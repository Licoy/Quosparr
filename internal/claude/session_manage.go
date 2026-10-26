package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/sessions"
)

type managedClaudeGroup struct {
	id      string
	root    string
	path    string
	files   []sessions.ManagedFile
	sink    *scanSink
	problem string
	hasMain bool
}

// ListManaged returns every Claude project session, grouping the main log and
// subagent/sidecar JSONL files that declare the same session ID.
func (s SessionScanner) ListManaged(ctx context.Context) ([]sessions.ManagedSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot := s.pricingSnapshot()
	groups := map[string]*managedClaudeGroup{}
	for _, root := range ProjectsDirs(s.ConfigDir, s.Home) {
		st, err := os.Lstat(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Claude projects root is not a directory: %s", filepath.Base(root))
		}
		files, err := walkProjects(ctx, root)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			snapshot, err := sessions.SnapshotManagedFile(root, file.path)
			if err != nil {
				return nil, err
			}
			part, err := readManagedClaudePart(ctx, root, file.path, snapshot)
			if err != nil {
				return nil, err
			}
			ids := claudeMetaIDs(part)
			id := strings.TrimSuffix(filepath.Base(file.path), filepath.Ext(file.path))
			problem := ""
			switch len(ids) {
			case 0:
				problem = "unrecognized session log"
			case 1:
				id = ids[0]
			default:
				problem = "multiple session IDs in one log"
				id = ids[0]
			}
			projectRoot := claudeProjectRoot(root, file.path)
			key := projectRoot + "\x00" + id
			if len(ids) != 1 {
				key += "\x00" + file.path
			}
			group := groups[key]
			if group == nil {
				group = &managedClaudeGroup{id: id, root: root, path: file.path, sink: newScanSink("")}
				groups[key] = group
			}
			group.files = append(group.files, snapshot)
			group.sink.merge(part)
			if part.bad > 0 {
				problem = fmt.Sprintf("%d malformed session record(s)", part.bad)
			}
			if problem != "" {
				group.problem = problem
			}
			isMain := strings.TrimSuffix(filepath.Base(file.path), filepath.Ext(file.path)) == id
			if isMain || (!group.hasMain && file.path < group.path) {
				group.path = file.path
			}
			group.hasMain = group.hasMain || isMain
		}
	}

	result := make([]sessions.ManagedSession, 0, len(groups))
	for _, group := range groups {
		item := claudeManagedItem(group, snapshot)
		if group.problem != "" {
			item.State = sessions.StateNeedsAttention
		}
		sort.Slice(group.files, func(i, j int) bool { return group.files[i].Path < group.files[j].Path })
		result = append(result, sessions.ManagedSession{
			Item: item, Provider: sessions.ProviderClaude, Path: group.path,
			Root: group.root, Files: group.files, Problem: group.problem,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

func readManagedClaudePart(ctx context.Context, root, path string, snapshot sessions.ManagedFile) (*scanSink, error) {
	part := newScanSink(path)
	if err := part.readFile(ctx, path, snapshot.ModTime); err != nil {
		return nil, err
	}
	if err := sessions.VerifyManagedFileIdentity(root, snapshot); err != nil {
		return nil, err
	}
	return part, nil
}

func claudeProjectRoot(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return root
	}
	first := strings.Split(rel, string(filepath.Separator))[0]
	return filepath.Join(root, first)
}

func claudeMetaIDs(sink *scanSink) []string {
	ids := make([]string, 0, len(sink.metas))
	for id := range sink.metas {
		if id != "unknown" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func claudeManagedItem(group *managedClaudeGroup, snapshots ...pricing.Snapshot) sessions.Item {
	summary := group.sink.summary(time.Local, SessionScanOptions{RecentLimit: 1}, time.Now(), snapshots...)
	meta := group.sink.metas[group.id]
	if len(summary.RecentItems) > 0 {
		item := summary.RecentItems[0].SessionsItem()
		fillClaudeManagedMeta(&item, meta)
		return item
	}
	updated := time.Time{}
	for _, file := range group.files {
		if file.ModTime.After(updated) {
			updated = file.ModTime
		}
	}
	item := sessions.Item{ID: group.id, Title: "Untitled session", ProjectName: sessions.UnknownProject, UpdatedAt: updated, State: sessions.StateRecent}
	fillClaudeManagedMeta(&item, meta)
	return item
}

func fillClaudeManagedMeta(item *sessions.Item, meta *sessionMeta) {
	if meta != nil {
		if meta.title != "" {
			item.Title = meta.title
		}
		if meta.cwd != "" {
			cwd := meta.cwd
			item.Cwd = &cwd
			item.ProjectName = sessions.ProjectName(&cwd)
		}
		if meta.updated.After(item.UpdatedAt) {
			item.UpdatedAt = meta.updated
		}
	}
	if item.Title == "" {
		item.Title = "Untitled session"
	}
}
