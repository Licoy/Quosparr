package sessions

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	ProviderCodex  = "codex"
	ProviderClaude = "claude"
	ProviderGrok   = "grok"
	ProviderGemini = "gemini"
)

// ManagedFile is a file that belonged to a session when it was scanned.
// Destructive callers must re-check that Path is still a regular file below
// Root and compare Info with os.SameFile before acting on it.
type ManagedFile struct {
	Path    string
	Size    int64
	ModTime time.Time
	Info    fs.FileInfo
}

// ManagedSession is one complete session and the exact files in its group.
// Files is a trusted snapshot, not permission to enumerate or remove siblings.
type ManagedSession struct {
	Item
	Provider string
	Path     string
	Root     string
	Files    []ManagedFile
	Archived bool
	Problem  string
}

type ManagedMessage struct {
	Role string
	Text string
	At   time.Time
}

type ManagedDetail struct {
	Session   ManagedSession
	Messages  []ManagedMessage
	Truncated bool
}

// SnapshotManagedFile validates and snapshots one regular file without
// following a final symlink.
func SnapshotManagedFile(root, path string) (ManagedFile, error) {
	inside, err := pathWithinRoot(root, path)
	if err != nil {
		return ManagedFile{}, err
	}
	if !inside {
		return ManagedFile{}, fmt.Errorf("session file is outside its root")
	}
	if err := managedParentChain(root, path); err != nil {
		return ManagedFile{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return ManagedFile{}, err
	}
	if !info.Mode().IsRegular() {
		return ManagedFile{}, fmt.Errorf("session file is not regular: %s", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return ManagedFile{}, err
	}
	// File.Stat captures Windows identity now; Lstat defers it until SameFile.
	info, statErr := f.Stat()
	closeErr := f.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		return ManagedFile{}, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return ManagedFile{}, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return ManagedFile{}, fmt.Errorf("session file identity changed: %s", filepath.Base(path))
	}
	return ManagedFile{Path: path, Size: info.Size(), ModTime: info.ModTime(), Info: info}, nil
}

// VerifyManagedFileIdentity rejects a path that no longer names the regular
// file captured by SnapshotManagedFile. Growth on the same file is allowed.
func VerifyManagedFileIdentity(root string, file ManagedFile) error {
	current, err := SnapshotManagedFile(root, file.Path)
	if err != nil {
		return err
	}
	if file.Info == nil || !os.SameFile(file.Info, current.Info) {
		return fmt.Errorf("session file identity changed: %s", filepath.Base(file.Path))
	}
	return nil
}

func managedParentChain(root, path string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("session root is not a regular directory")
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Dir(filepath.Clean(path)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("session file parent is outside its root")
	}
	current := filepath.Clean(root)
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("session file parent is not a regular directory")
		}
	}
	return nil
}

func pathWithinRoot(root, path string) (bool, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

// ListManaged returns every Codex session under sessions and
// archived_sessions. It does not apply the recent-list candidate limit.
func (s Scanner) ListManaged(ctx context.Context) ([]ManagedSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index, err := readIndex(s.CodexHome)
	if err != nil {
		return nil, err
	}
	titles := make(map[string]string, len(index))
	for _, entry := range index {
		titles[entry.ID] = entry.ThreadName
	}
	var result []ManagedSession
	for _, folder := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(s.CodexHome, folder)
		found, err := s.listManagedRoot(ctx, root, folder == "archived_sessions", titles)
		if err != nil {
			return nil, err
		}
		result = append(result, found...)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

func (s Scanner) listManagedRoot(ctx context.Context, root string, archived bool, titles map[string]string) ([]ManagedSession, error) {
	st, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("session root is not a directory: %s", filepath.Base(root))
	}
	var result []ManagedSession
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(d.Name()) != ".jsonl" {
			return nil
		}
		file, err := SnapshotManagedFile(root, path)
		if err != nil {
			return err
		}
		item, ok, err := s.parseSession(path)
		if err != nil {
			return err
		}
		problem, err := probeProblem(path, file.Size)
		if err != nil {
			return err
		}
		if !ok {
			id := sessionID(filepath.Base(path))
			if id == "" {
				id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			}
			item = Item{ID: id, Title: untitled, ProjectName: UnknownProject, UpdatedAt: file.ModTime, State: StateNeedsAttention}
			if problem == "" {
				problem = "unrecognized session log"
			}
		}
		if title := cleanTitle(titles[item.ID]); title != "" {
			item.Title = title
		}
		model, err := managedLatestModel(path, file.Size)
		if err != nil {
			return err
		}
		if model != "" {
			item.Model = &model
		}
		if problem != "" {
			item.State = StateNeedsAttention
		}
		result = append(result, ManagedSession{Item: item, Provider: ProviderCodex, Path: path, Root: root, Files: []ManagedFile{file}, Archived: archived, Problem: problem})
		return nil
	})
	return result, err
}

func managedLatestModel(path string, size int64) (string, error) {
	lines, err := probeLines(path, size)
	if err != nil {
		return "", err
	}
	model := ""
	for _, line := range lines {
		obj, ok := decodeLine(line)
		if !ok {
			continue
		}
		payload := objectOf(obj["payload"])
		if value, ok := stringOf(objectOf(obj["turn_context"]), "model"); ok && strings.TrimSpace(value) != "" {
			model = value
		} else if value, ok := stringOf(payload, "model"); ok && strings.TrimSpace(value) != "" {
			model = value
		}
	}
	return model, nil
}

func probeProblem(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	limit := min(int64(headProbeBytes), size)
	reader := bufio.NewReader(io.LimitReader(f, limit))
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF && int64(len(line)) > 0 && size > limit {
			return "", nil
		}
		if strings.TrimSpace(line) != "" {
			if _, ok := decodeLine(strings.TrimRight(line, "\r\n")); !ok {
				return "malformed session record", nil
			}
		}
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if line == "" {
			continue
		}
	}
}
