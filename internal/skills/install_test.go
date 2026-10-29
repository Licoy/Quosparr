package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

type fakeGitHub struct {
	mu          sync.Mutex
	commit      string
	files       map[string]string // repository paths
	exec        map[string]bool
	trees       map[string]string // folder to tree SHA
	sizes       map[string]int64  // sizes the listing claims instead
	refs        map[string]bool   // refs that resolve; HEAD always does
	rateLimited bool
	truncated   bool // the listing is cut short
	noTree      bool // the listing fails
	hits        map[string]int
}

func newFakeGitHub(t *testing.T) (*fakeGitHub, GitHub) {
	t.Helper()
	f := &fakeGitHub{commit: strings.Repeat("a", 40), refs: map[string]bool{}, hits: map[string]int{},
		files: map[string]string{
			"README.md":                 "readme",
			"skills/pdf/SKILL.md":       importSkillText("pdf", "Fill PDF forms"),
			"skills/pdf/scripts/run.sh": "#!/bin/sh\n",
			"skills/word/SKILL.md":      importSkillText("word", "Edit documents"),
		},
		exec:  map[string]bool{"skills/pdf/scripts/run.sh": true},
		trees: map[string]string{"skills": "t-skills", "skills/pdf": "t-pdf-1", "skills/word": "t-word-1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, GitHub{Client: srv.Client(), API: srv.URL, Codeload: srv.URL}
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch p := r.URL.Path; {
	case strings.HasPrefix(p, "/repos/acme/tools/commits/"):
		f.hits["commits"]++
		ref := strings.TrimPrefix(p, "/repos/acme/tools/commits/")
		if f.rateLimited {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if ref != "HEAD" && !f.refs[ref] {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(f.commit))
	case p == "/repos/acme/tools/git/trees/"+f.commit:
		f.hits["trees"]++
		if f.noTree {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var body struct {
			SHA       string           `json:"sha"`
			Tree      []map[string]any `json:"tree"`
			Truncated bool             `json:"truncated"`
		}
		body.SHA, body.Truncated = "t-root", f.truncated
		for folder, sha := range f.trees {
			body.Tree = append(body.Tree, map[string]any{"path": folder, "mode": "040000", "type": "tree", "sha": sha})
		}
		for file, data := range f.files {
			size, ok := f.sizes[file]
			if !ok {
				size = int64(len(data))
			}
			body.Tree = append(body.Tree, map[string]any{"path": file, "mode": "100644", "type": "blob", "sha": "b", "size": size})
		}
		body.Tree = append(body.Tree, map[string]any{"path": "skills/pdf/escape.md", "mode": "120000", "type": "blob", "sha": "l", "size": 15})
		_ = json.NewEncoder(w).Encode(body)
	case p == "/acme/tools/tar.gz/"+f.commit:
		f.hits["tarball"]++
		_, _ = w.Write(f.tarball())
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeGitHub) tarball() []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	top := "tools-" + f.commit + "/"
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": f.commit}})
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: top, Mode: 0o755})
	paths := make([]string, 0, len(f.files))
	for p := range f.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		mode := int64(0o644)
		if f.exec[p] {
			mode = 0o755
		}
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: top + p, Mode: mode, Size: int64(len(f.files[p]))})
		_, _ = tw.Write([]byte(f.files[p]))
	}
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: top + "skills/pdf/escape.md", Linkname: "../../README.md"})
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

func (f *fakeGitHub) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func installFromFake(t *testing.T, env Env, g GitHub, raw string, target InstallTarget) InstallResult {
	t.Helper()
	src, err := ParseGitHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := g.Load(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareInstall(env, target, nil, snap, found)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Install(context.Background(), plan, env.Now)
	if err != nil || result.RecordErr != nil {
		t.Fatalf("install: %v, %v", err, result.RecordErr)
	}
	for _, item := range result.Items {
		if item.Err != nil {
			t.Fatalf("install %s: %v", item.Name, item.Err)
		}
	}
	return result
}

func TestGitHubLoadResolvesTreeRef(t *testing.T) {
	f, g := newFakeGitHub(t)
	f.refs["feature/x"] = true
	src, err := ParseGitHubSource("https://github.com/acme/tools/tree/feature/x/skills")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := g.Load(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Source.Ref != "feature/x" || snap.Source.Subpath != "skills" || snap.Commit != f.commit {
		t.Fatalf("source = %+v, commit %s", snap.Source, snap.Commit)
	}
	found, err := Discover(snap)
	if err != nil || len(found) != 2 {
		t.Fatalf("found = %v, %v", importNames(found), err)
	}
	pdf := importFind(t, found, "pdf")
	if pdf.Folder != "pdf" || pdf.TreeSHA != "t-pdf-1" || !strings.Contains(strings.Join(pdf.Skipped, ","), "escape.md") {
		t.Fatalf("pdf = %+v", pdf)
	}
	if snap.has("README.md") {
		t.Fatal("files outside the folder were kept")
	}

	f.set(func() { f.rateLimited = true })
	if _, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools"}); importCode(err) != ImportRateLimited {
		t.Fatalf("rate limit: %v", err)
	}
	f.set(func() { f.rateLimited = false })
	if _, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools", Subpath: "missing"}); importCode(err) != ImportNotFound {
		t.Fatalf("missing folder: %v", err)
	}
	if _, err := (GitHub{}).Load(context.Background(), src); importCode(err) != ImportNetwork {
		t.Fatalf("no client: %v", err)
	}
}

func TestGitHubLoadKeepsOnlyTheSkills(t *testing.T) {
	f, g := newFakeGitHub(t)
	f.files["docs/video.bin"] = strings.Repeat("x", 3<<20)
	var done, total int64
	g.Progress = func(d, t int64) { done, total = d, t }
	snap, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools"})
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil || len(found) != 2 {
		t.Fatalf("found = %v, %v", importNames(found), err)
	}
	if snap.has("README.md") || snap.has("docs/video.bin") || snap.dirs["docs"] || !snap.has("skills/pdf/scripts/run.sh") {
		t.Fatalf("kept %v", snap.order)
	}
	if done < 3<<20 || total < done {
		t.Fatalf("progress = %d of %d", done, total)
	}

	// Without a complete listing the whole folder is read, as before.
	for _, cut := range []func(){func() { f.truncated = true }, func() { f.truncated, f.noTree = false, true }} {
		f.set(cut)
		snap, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools"})
		if err != nil || !snap.has("README.md") || !snap.has("docs/video.bin") {
			t.Fatalf("unlisted: %v", err)
		}
	}
}

func TestGitHubLoadChecksTheListingFirst(t *testing.T) {
	f, g := newFakeGitHub(t)
	f.sizes = map[string]int64{"skills/pdf/SKILL.md": MaxImportFileBytes + 1}
	_, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools"})
	if importCode(err) != ImportTooLarge || !strings.Contains(err.Error(), "skills/pdf/SKILL.md") {
		t.Fatalf("large file: %v", err)
	}
	f.set(func() {
		f.sizes = map[string]int64{"skills/pdf/a": MaxImportBytes / 2, "skills/word/b": MaxImportBytes / 2}
		f.files["skills/pdf/a"], f.files["skills/word/b"] = "a", "b"
	})
	if _, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools"}); importCode(err) != ImportTooLarge {
		t.Fatalf("large skills: %v", err)
	}
	f.set(func() { f.sizes = nil })
	if _, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools", Subpath: "docs"}); importCode(err) != ImportNotFound {
		t.Fatalf("missing folder: %v", err)
	}
	f.set(func() { f.files["docs/notes.md"], f.trees["docs"] = "notes", "t-docs" })
	if _, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools", Subpath: "docs"}); importCode(err) != ImportNoSkills {
		t.Fatalf("no skills: %v", err)
	}
	if f.hits["tarball"] != 0 {
		t.Fatalf("downloaded %d times", f.hits["tarball"])
	}
}

func TestInstallWritesRecordsAndRefusesExisting(t *testing.T) {
	env := claudeTestEnv(t)
	f, g := newFakeGitHub(t)
	installFromFake(t, env, g, "acme/tools", TargetClaude)

	dir := filepath.Join(env.ClaudeDir, "skills", "pdf")
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil || !strings.Contains(string(data), "Fill PDF forms") {
		t.Fatalf("SKILL.md = %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "scripts", "run.sh"))
		if err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("script mode = %v, %v", info, err)
		}
		if info, err := os.Stat(recordsPath(env.Home)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("record mode = %v, %v", info, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "escape.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a link out of the skill was installed")
	}
	records, err := LoadRecords(env.Home)
	if err != nil {
		t.Fatal(err)
	}
	rec := records[recordKey(dir)]
	if rec.Repository != "https://github.com/acme/tools" || rec.SkillPath != "skills/pdf" || rec.Commit != f.commit || rec.TreeSHA != "t-pdf-1" || rec.ContentHash == "" {
		t.Fatalf("record = %+v", rec)
	}

	sum, err := ScanClaude(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	pdf := claudeFindSkill(t, sum, "pdf", "")
	if pdf.Origin.From != OriginQuosparr || pdf.Origin.Installer != "github" || pdf.Origin.GitHub != "acme/tools" || !Updatable(pdf) {
		t.Fatalf("origin = %+v", pdf.Origin)
	}

	snap, err := g.Load(context.Background(), Source{Kind: SourceGitHub, Owner: "acme", Repo: "tools"})
	if err != nil {
		t.Fatal(err)
	}
	found, _ := Discover(snap)
	plan, err := PrepareInstall(env, TargetClaude, []Summary{sum}, snap, found)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range plan.Items {
		if item.Status != InstallInstalled {
			t.Fatalf("%s status = %s", item.Name, item.Status)
		}
	}
	commonWriteFile(t, filepath.Join(env.Home, ".agents", "skills", "pdf", "notes.txt"), "mine")
	plan, err = PrepareInstall(env, TargetShared, []Summary{sum}, snap, found)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Items[0].Name != "pdf" || plan.Items[0].Status != InstallExists || plan.Items[1].Status != InstallAdd {
		t.Fatalf("items = %+v", plan.Items)
	}
	result, err := Install(context.Background(), plan, env.Now)
	if err != nil || len(result.Items) != 1 || result.Items[0].Name != "word" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if data, _ := os.ReadFile(filepath.Join(env.Home, ".agents", "skills", "pdf", "notes.txt")); string(data) != "mine" {
		t.Fatal("an existing folder was changed")
	}
}

func TestPrepareInstallChecksTargets(t *testing.T) {
	env := claudeTestEnv(t)
	dir := t.TempDir()
	commonWriteFile(t, filepath.Join(dir, "synced", "SKILL.md"), importSkillText("synced", "Reserved by Claude"))
	snap, err := LoadFolder(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(snap)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PrepareInstall(env, TargetClaude, nil, snap, found)
	if err != nil || plan.Items[0].Status != InstallInvalid {
		t.Fatalf("plan = %+v, %v", plan.Items, err)
	}
	commonWriteFile(t, filepath.Join(env.CodexHome, "skills", "other", "SKILL.md"), importSkillText("synced", "Same name"))
	codex, err := ScanCodex(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = PrepareInstall(env, TargetShared, []Summary{codex}, snap, found)
	if err != nil || plan.Items[0].Status != InstallAdd || len(plan.Items[0].Clashes) != 1 || plan.Readers[0] != ProviderCodex {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	link := filepath.Join(env.Home, ".grok", "skills")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareInstall(env, TargetGrok, nil, snap, found); importCode(err) != ImportUnsafeRoot {
		t.Fatalf("linked root: %v", err)
	}
	plan, err = PrepareInstall(env, TargetCodex, nil, snap, found)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(env.CodexHome, "skills")
	if err := os.Rename(root, root+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), plan, env.Now); importCode(err) != ImportChanged {
		t.Fatalf("replaced root: %v", err)
	}
}

func TestUpdateSkillFromGitHub(t *testing.T) {
	env := claudeTestEnv(t)
	f, g := newFakeGitHub(t)
	installFromFake(t, env, g, "acme/tools", TargetClaude)
	pdfDir := filepath.Join(env.ClaudeDir, "skills", "pdf")
	wordDir := filepath.Join(env.ClaudeDir, "skills", "word")
	ctx := context.Background()

	checks := g.CheckUpdates(ctx, env.Home, []string{pdfDir, wordDir, t.TempDir()})
	if checks[0].State != UpdateCurrent || checks[1].State != UpdateCurrent || importCode(checks[2].Err) != ImportNotUpdatable {
		t.Fatalf("checks = %+v", checks)
	}
	f.set(func() {
		f.commit = strings.Repeat("b", 40)
		f.files["skills/pdf/SKILL.md"] = importSkillText("pdf", "Fill PDF forms, version two")
		f.trees["skills/pdf"] = "t-pdf-2"
		f.hits = map[string]int{}
	})
	checks = g.CheckUpdates(ctx, env.Home, []string{pdfDir, wordDir})
	if checks[0].State != UpdateAvailable || checks[1].State != UpdateCurrent || f.hits["commits"] != 1 || f.hits["trees"] != 1 {
		t.Fatalf("checks = %+v, hits %v", checks, f.hits)
	}

	sum, err := ScanClaude(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	noTrash := func(string) error { return errors.New("system trash not expected") }
	result, err := UpdateSkill(ctx, env, g, UpdateRequest{Skill: claudeFindSkill(t, sum, "pdf", ""), Roots: sum.Roots}, noTrash)
	if err != nil || result.Unchanged || result.OldInSystemTrash {
		t.Fatalf("update = %+v, %v", result, err)
	}
	if data, _ := os.ReadFile(filepath.Join(pdfDir, "SKILL.md")); !strings.Contains(string(data), "version two") {
		t.Fatalf("SKILL.md = %q", data)
	}
	entries, err := NewTrash(filepath.Join(env.Home, ".quosparr"), env.Home).List(ctx)
	if err != nil || len(entries) != 1 || entries[0].Original != pdfDir {
		t.Fatalf("trash = %+v, %v", entries, err)
	}
	staged, _ := filepath.Glob(filepath.Join(env.ClaudeDir, "skills", ".quosparr-update-*"))
	if len(staged) != 0 {
		t.Fatalf("staging left behind: %v", staged)
	}

	result, err = UpdateSkill(ctx, env, g, UpdateRequest{Skill: claudeFindSkill(t, sum, "word", ""), Roots: sum.Roots}, noTrash)
	if err != nil || !result.Unchanged {
		t.Fatalf("unchanged update = %+v, %v", result, err)
	}
	records, _ := LoadRecords(env.Home)
	if rec := records[recordKey(wordDir)]; rec.Commit != f.commit {
		t.Fatalf("record not moved on: %+v", rec)
	}

	commonWriteFile(t, filepath.Join(wordDir, "local.md"), "edited here")
	f.set(func() { f.files["skills/word/SKILL.md"] = importSkillText("word", "Edit documents, version two") })
	sum, err = ScanClaude(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	word := claudeFindSkill(t, sum, "word", "")
	if _, err := UpdateSkill(ctx, env, g, UpdateRequest{Skill: word, Roots: sum.Roots}, noTrash); importCode(err) != ImportModified {
		t.Fatalf("modified copy: %v", err)
	}
	if _, err := UpdateSkill(ctx, env, g, UpdateRequest{Skill: word, Roots: sum.Roots, Force: true}, noTrash); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wordDir, "local.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("forced update kept the local file")
	}
}
