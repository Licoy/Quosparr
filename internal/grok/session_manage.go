package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/sessions"
)

var managedGrokFiles = []string{"summary.json", "events.jsonl", "updates.jsonl"}

// ListManaged returns every Grok session summary without applying the normal
// recent/cost scan cap.
func (s SessionScanner) ListManaged(ctx context.Context) ([]sessions.ManagedSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot := s.pricingSnapshot()
	s.PricingSnapshot = func() pricing.Snapshot { return snapshot }
	root := filepath.Join(s.GrokHome, "sessions")
	st, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return []sessions.ManagedSession{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Grok sessions root is not a directory")
	}
	var result []sessions.ManagedSession
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "summary.json" || d.IsDir() {
			return nil
		}
		entry, err := s.managedFromSummary(root, path)
		if err != nil {
			return err
		}
		result = append(result, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].Path < result[j].Path
	})
	return result, nil
}

func (s SessionScanner) managedFromSummary(root, summaryPath string) (sessions.ManagedSession, error) {
	dir := filepath.Dir(summaryPath)
	files, err := grokManagedSnapshots(root, dir)
	if err != nil {
		return sessions.ManagedSession{}, err
	}
	return s.managedFromCapturedSummary(root, summaryPath, files)
}

func (s SessionScanner) managedFromCapturedSummary(root, summaryPath string, files []sessions.ManagedFile) (sessions.ManagedSession, error) {
	dir := filepath.Dir(summaryPath)
	var info sessions.ManagedFile
	for _, file := range files {
		if file.Path == summaryPath {
			info = file
			break
		}
	}
	if info.Info == nil {
		return sessions.ManagedSession{}, fmt.Errorf("Grok session summary was not captured")
	}
	sum, ok, err := parseSessionSummary(sessionCandidate{path: summaryPath, mtime: info.ModTime})
	if err != nil {
		return sessions.ManagedSession{}, err
	}
	problem := ""
	item := sessions.Item{ID: filepath.Base(dir), Title: "Untitled session", ProjectName: sessions.UnknownProject, UpdatedAt: info.ModTime, State: sessions.StateNeedsAttention}
	if !ok {
		problem = "malformed session summary"
	} else {
		totals := SessionUsageTotals{}
		updatesPath := filepath.Join(dir, "updates.jsonl")
		if managedGrokFileCaptured(files, updatesPath) {
			parsed, err := parseSessionUpdates(updatesPath, sum.updatedAt, sum.model, s.loc(), s.pricingSnapshot())
			if err != nil {
				return sessions.ManagedSession{}, err
			}
			for _, day := range parsed.byDay {
				totals.Merge(day)
			}
			if bad, err := grokMalformedJSONL(updatesPath); err != nil {
				return sessions.ManagedSession{}, err
			} else if bad > 0 {
				problem = fmt.Sprintf("%d malformed session record(s)", bad)
			}
		}
		item = grokManagedItem(sum, totals)
		if problem != "" {
			item.State = sessions.StateNeedsAttention
		}
	}
	for _, file := range files {
		if err := sessions.VerifyManagedFileIdentity(root, file); err != nil {
			return sessions.ManagedSession{}, err
		}
	}
	return sessions.ManagedSession{Item: item, Provider: sessions.ProviderGrok, Path: dir, Root: root, Files: files, Problem: problem}, nil
}

func managedGrokFileCaptured(files []sessions.ManagedFile, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func grokManagedSnapshots(root, dir string) ([]sessions.ManagedFile, error) {
	files := make([]sessions.ManagedFile, 0, len(managedGrokFiles))
	for _, name := range managedGrokFiles {
		path := filepath.Join(dir, name)
		file, err := sessions.SnapshotManagedFile(root, path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func grokManagedItem(sum sessionSummaryFile, totals SessionUsageTotals) sessions.Item {
	item := sessions.Item{ID: sum.id, Title: sum.title, ProjectName: sum.projectName, Cwd: sum.cwd, UpdatedAt: sum.updatedAt, State: sessions.StateRecent, Model: sum.model}
	api := totals.APITotals()
	item.Totals = sessions.Totals{TotalTokens: int(api.TotalTokens), UncachedInputTokens: int(api.UncachedInputTokens), CachedInputTokens: int(api.CachedInputTokens), OutputTokens: int(api.OutputTokens), Turns: int(api.Turns), Threads: int(api.Threads)}
	if totals.EstimatedUSD != nil {
		if value, err := swiftjson.ParseDecimal(totals.EstimatedUSD.String()); err == nil {
			item.EstimatedUSD = &value
		}
	}
	return item
}

func grokMalformedJSONL(path string) (int, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	bad := 0
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var value map[string]any
		if json.Unmarshal(line, &value) != nil || value == nil {
			bad++
		}
	}
	return bad, nil
}
