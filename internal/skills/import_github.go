package skills

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ParseGitHubSource accepts owner/repo, owner/repo/folder, and github.com
// repository, tree or blob URLs (a blob URL may end in SKILL.md).
func ParseGitHubSource(raw string) (Source, error) {
	raw = strings.TrimSpace(raw)
	bad := importError(ImportInvalidSource, "")
	rest, isURL := raw, false
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "https://"):
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || u.Port() != "" {
			return Source{}, bad
		}
		if host := strings.ToLower(u.Hostname()); host != "github.com" && host != "www.github.com" {
			return Source{}, bad
		}
		rest, isURL = u.Path, true
	case strings.HasPrefix(lower, "github.com/"):
		rest, isURL = raw[len("github.com/"):], true
		if i := strings.IndexAny(rest, "?#"); i >= 0 {
			rest = rest[:i]
		}
	case strings.Contains(raw, ":") || strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "."):
		return Source{}, bad
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		return Source{}, bad
	}
	src := Source{Kind: SourceGitHub, Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}
	if !validGitHubName(src.Owner) || !validGitHubName(src.Repo) {
		return Source{}, bad
	}
	tail := parts[2:]
	if isURL && len(tail) > 0 {
		if tail[0] != "tree" && tail[0] != "blob" || len(tail) < 2 {
			return Source{}, bad
		}
		if tail[0] == "blob" && tail[len(tail)-1] == "SKILL.md" {
			tail = tail[:len(tail)-1]
		}
		src.treePath = strings.Join(tail[1:], "/")
		src.Ref, src.Subpath = tail[1], strings.Join(tail[2:], "/")
	} else if len(tail) > 0 {
		src.Subpath = strings.Join(tail, "/")
	}
	if src.Subpath != "" {
		if _, ok := cleanImportPath(src.Subpath); !ok {
			return Source{}, bad
		}
	}
	if src.treePath != "" {
		if _, ok := cleanImportPath(src.treePath); !ok {
			return Source{}, bad
		}
	}
	return src, nil
}

func validGitHubName(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > 100 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// GitHub reads public repositories. Client is the application's proxied
// HTTP client, so requests never fall back to a direct connection; only
// the API and codeload hosts are contacted, redirects included.
type GitHub struct {
	Client   *http.Client
	API      string // https://api.github.com unless a test overrides it
	Codeload string // https://codeload.github.com
	// Progress, when set, hears about a download every MiB or so: the
	// bytes unpacked and roughly how many there are, 0 when unknown.
	Progress func(done, total int64)
}

const (
	gitHubAPI      = "https://api.github.com"
	gitHubCodeload = "https://codeload.github.com"
	maxTreeBytes   = 32 << 20
)

func (g GitHub) bases() (string, string) {
	api, codeload := strings.TrimRight(g.API, "/"), strings.TrimRight(g.Codeload, "/")
	if api == "" {
		api = gitHubAPI
	}
	if codeload == "" {
		codeload = gitHubCodeload
	}
	return api, codeload
}

func (g GitHub) client() (*http.Client, error) {
	if g.Client == nil {
		return nil, importError(ImportNetwork, "")
	}
	allowed := map[string]string{}
	for _, base := range []string{g.API, g.Codeload, gitHubAPI, gitHubCodeload} {
		if u, err := url.Parse(base); err == nil && u.Host != "" {
			allowed[u.Host] = u.Scheme
		}
	}
	c := *g.Client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || allowed[req.URL.Host] != req.URL.Scheme {
			return importError(ImportNetwork, "")
		}
		return nil
	}
	return &c, nil
}

func (g GitHub) get(ctx context.Context, rawURL, accept string) (*http.Response, error) {
	client, err := g.client()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, importError(ImportInvalidSource, "")
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "Quosparr")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var detail *ImportError
		if errors.As(err, &detail) {
			return nil, detail
		}
		return nil, importError(ImportNetwork, "")
	}
	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return nil, importError(ImportRateLimited, "")
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnprocessableEntity,
		resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, importError(ImportNotFound, "")
	}
	return nil, importError(ImportHTTP, strconv.Itoa(resp.StatusCode))
}

func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// Latest is the commit ref points at; "" is the default branch.
func (g GitHub) Latest(ctx context.Context, owner, repo, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	api, _ := g.bases()
	resp, err := g.get(ctx, fmt.Sprintf("%s/repos/%s/%s/commits/%s", api, owner, repo, escapeRef(ref)), "application/vnd.github.sha")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	sha := strings.TrimSpace(string(data))
	if err != nil || !validCommit(sha) {
		return "", importError(ImportHTTP, "commit")
	}
	return strings.ToLower(sha), nil
}

func validCommit(sha string) bool {
	if len(sha) != 40 && len(sha) != 64 {
		return false
	}
	for _, r := range strings.ToLower(sha) {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// resolve finds the commit of a source; a pasted tree URL tries longer
// refs when the first part alone is not a branch or tag.
func (g GitHub) resolve(ctx context.Context, src Source) (Source, string, error) {
	if src.treePath == "" {
		commit, err := g.Latest(ctx, src.Owner, src.Repo, src.Ref)
		return src, commit, err
	}
	parts := strings.Split(src.treePath, "/")
	var last error
	for i := 1; i <= len(parts) && i <= 5; i++ {
		commit, err := g.Latest(ctx, src.Owner, src.Repo, strings.Join(parts[:i], "/"))
		var detail *ImportError
		if errors.As(err, &detail) && detail.Code == ImportNotFound {
			last = err
			continue
		}
		if err != nil {
			return src, "", err
		}
		src.Ref, src.Subpath, src.treePath = strings.Join(parts[:i], "/"), strings.Join(parts[i:], "/"), ""
		return src, commit, nil
	}
	return src, "", last
}

// Load reads the source's folder at its current commit into memory, with
// the tree SHA of each folder when the API provides them. The repository's
// listing comes first, so that the tarball is streamed through and only
// the skills' folders are kept; without a complete listing everything in
// the folder is kept, within the limits.
func (g GitHub) Load(ctx context.Context, src Source) (*Snapshot, error) {
	if src.Kind != SourceGitHub {
		return nil, importError(ImportInvalidSource, "")
	}
	src, commit, err := g.resolve(ctx, src)
	if err != nil {
		return nil, err
	}
	snap := newSnapshot(src)
	snap.Commit = commit
	tree, treeErr := g.trees(ctx, src.Owner, src.Repo, commit)
	switch {
	case treeErr != nil && ctx.Err() != nil:
		return nil, ctx.Err()
	case treeErr == nil && tree.complete && src.Subpath != "" && tree.folders[src.Subpath] == "":
		return nil, importError(ImportNotFound, src.Subpath)
	case treeErr == nil:
		snap.trees = tree.within(src.Subpath)
		if tree.complete {
			if err := snap.plan(tree.listing(src.Subpath)); err != nil {
				return nil, err
			}
		}
	}
	if err := g.download(ctx, snap, tree.unpacked); err != nil {
		if code := importCode(err); treeErr != nil && (code == ImportTooLarge || code == ImportTooManyFiles) {
			return nil, treeErr // the listing would have kept only the skills
		}
		return nil, err
	}
	return snap, snap.finish()
}

// gitTree is a commit's tree as the API lists it.
type gitTree struct {
	folders  map[string]string // tree SHA by folder, "" the root
	entries  []importEntry     // files and links, by path in the repository
	unpacked int64             // roughly what the tarball unpacks to
	complete bool              // false when GitHub cut the listing short
}

// trees lists the folders, files and links at commit.
func (g GitHub) trees(ctx context.Context, owner, repo, commit string) (gitTree, error) {
	api, _ := g.bases()
	resp, err := g.get(ctx, fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", api, owner, repo, commit), "application/vnd.github+json")
	if err != nil {
		return gitTree{}, err
	}
	defer resp.Body.Close()
	var body struct {
		SHA  string `json:"sha"`
		Tree []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size int64  `json:"size"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxTreeBytes)).Decode(&body); err != nil || body.SHA == "" {
		return gitTree{}, importError(ImportHTTP, "tree")
	}
	tree := gitTree{folders: map[string]string{"": body.SHA}, complete: !body.Truncated}
	for _, entry := range body.Tree {
		tree.unpacked += max(entry.Size, 0) + 1024 // a tar header and padding
		switch entry.Type {
		case "tree":
			tree.folders[entry.Path] = entry.SHA
		case "blob":
			tree.entries = append(tree.entries, importEntry{path: entry.Path, size: entry.Size, link: entry.Mode == "120000"})
		}
	}
	return tree, nil
}

// within is the tree SHA of each folder in sub, relative to it.
func (t gitTree) within(sub string) map[string]string {
	trees := map[string]string{}
	for p, sha := range t.folders {
		switch {
		case p == sub:
			trees[""] = sha
		case sub == "" || strings.HasPrefix(p, sub+"/"):
			trees[relInFolder(sub, p)] = sha
		}
	}
	return trees
}

// listing is the files and links in sub, relative to it; paths that do
// not clean are left to the download to reject.
func (t gitTree) listing(sub string) []importEntry {
	var out []importEntry
	for _, e := range t.entries {
		if sub != "" && !strings.HasPrefix(e.path, sub+"/") {
			continue
		}
		if p, ok := cleanImportPath(relInFolder(sub, e.path)); ok {
			e.path = p
			out = append(out, e)
		}
	}
	return out
}

// FolderTree is the tree SHA of folder ("" the root) at commit, or "".
func (g GitHub) FolderTree(ctx context.Context, owner, repo, commit, folder string) (string, error) {
	tree, err := g.trees(ctx, owner, repo, commit)
	if err != nil {
		return "", err
	}
	return tree.folders[folder], nil
}

// limitedReader fails with code once more than n bytes are read.
type limitedReader struct {
	r    io.Reader
	n    int64
	code string
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.n < 0 {
		return 0, importError(l.code, "")
	}
	if int64(len(p)) > l.n+1 {
		p = p[:l.n+1]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	if l.n < 0 {
		return n, importError(l.code, "")
	}
	return n, err
}

// progressReader passes on the bytes read through it about once a MiB.
type progressReader struct {
	r            io.Reader
	report       func(done, total int64)
	total        int64
	done, posted int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.done-p.posted >= 1<<20 {
		p.posted = p.done
		p.report(p.done, p.total)
	}
	return n, err
}

// download streams the commit's tarball into snap, keeping what it wants.
func (g GitHub) download(ctx context.Context, snap *Snapshot, unpacked int64) error {
	_, codeload := g.bases()
	src := snap.Source
	resp, err := g.get(ctx, fmt.Sprintf("%s/%s/%s/tar.gz/%s", codeload, src.Owner, src.Repo, snap.Commit), "application/x-gzip")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	zr, err := gzip.NewReader(&limitedReader{r: resp.Body, n: MaxImportDownload, code: ImportTooLarge})
	if err != nil {
		return archiveError(ctx, err)
	}
	var body io.Reader = &limitedReader{r: zr, n: MaxImportExpanded, code: ImportTooLarge}
	if g.Progress != nil {
		body = &progressReader{r: body, report: g.Progress, total: unpacked}
	}
	matched := false
	tr := tar.NewReader(body)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return archiveError(ctx, err)
		}
		name, ok := tarballPath(hdr.Name, src.Subpath)
		if !ok {
			continue
		}
		matched = true
		if !snap.wants(name) {
			continue
		}
		if err := addTarEntry(snap, tr, hdr, name); err != nil {
			return archiveError(ctx, err)
		}
	}
	if !matched {
		return importError(ImportNotFound, src.Subpath)
	}
	return nil
}

// tarballPath strips the tarball's top folder and the source folder; ok
// is false outside the folder and for the folder itself.
func tarballPath(name, sub string) (string, bool) {
	i := strings.IndexByte(name, '/')
	if i < 0 {
		return "", false
	}
	name = strings.TrimSuffix(name[i+1:], "/")
	if sub == "" {
		return name, name != ""
	}
	if !strings.HasPrefix(name, sub+"/") {
		return "", false
	}
	return name[len(sub)+1:], true
}

func addTarEntry(snap *Snapshot, tr *tar.Reader, hdr *tar.Header, name string) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return snap.addDir(name)
	case tar.TypeSymlink:
		return snap.addLink(name, hdr.Linkname)
	case tar.TypeReg:
		if importSkipped(name, false) {
			return nil
		}
		if hdr.Size > MaxImportFileBytes {
			return importError(ImportTooLarge, name)
		}
		data, err := io.ReadAll(io.LimitReader(tr, MaxImportFileBytes+1))
		if err != nil {
			return err
		}
		return snap.addFile(name, data, hdr.Mode&0o111 != 0)
	}
	return nil // hard links, devices and submodules are not kept
}

func archiveError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var detail *ImportError
	if errors.As(err, &detail) {
		return detail
	}
	return importError(ImportInvalidArchive, "")
}
