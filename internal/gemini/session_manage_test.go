package gemini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/sessions"
)

func sessionWriteFile(t *testing.T, path string) {
	t.Helper()
	sessionMust(t, os.MkdirAll(filepath.Dir(path), 0o700))
	sessionMust(t, os.WriteFile(path, []byte("x"), 0o600))
}

// sessionManagedFixture is an App conversation with side files whose
// summary row is in both clients' summaries databases.
func sessionManagedFixture(t *testing.T) (sessionTestHome, SessionScanner) {
	t.Helper()
	h := sessionNewTestHome(t)
	sessionWriteConversation(t, h.app, "conv-a", sessionAppConversation(), false)
	sessionWriteConversation(t, h.app, "conv-b", sessionTestConversation{}, false)
	sessionWriteFile(t, filepath.Join(h.app, "brain", "conv-a", "task.md"))
	sessionWriteFile(t, filepath.Join(h.app, "brain", "conv-a", ".hidden", "deep", "image.png"))
	sessionWriteFile(t, filepath.Join(h.app, "annotations", "conv-a.pbtxt"))
	sessionWriteSummaries(t, h.app,
		sessionTestSummary{id: "conv-a", title: "Alpha", workspace: "/work/alpha", modified: "2026-11-11 09:00:10+00:00"},
		sessionTestSummary{id: "conv-b", title: "Beta", modified: "2026-11-01 09:00:00+00:00"},
	)
	sessionWriteSummaries(t, h.cli, sessionTestSummary{id: "conv-a", title: "Alpha", modified: "2026-11-11 09:00:10+00:00"})
	scanner := h.scanner(t)
	scanner.BackupDir = filepath.Join(filepath.Dir(h.home), "quosparr", "backups")
	return h, scanner
}

func sessionTrashInto(t *testing.T) (func(string) error, string) {
	dir := filepath.Join(t.TempDir(), "Trash")
	sessionMust(t, os.MkdirAll(dir, 0o700))
	n := 0
	return func(path string) error {
		n++
		return os.Rename(path, filepath.Join(dir, strconv.Itoa(n)+"-"+filepath.Base(path)))
	}, dir
}

func sessionFind(t *testing.T, list []sessions.ManagedSession, id string) sessions.ManagedSession {
	t.Helper()
	for _, session := range list {
		if session.ID == id {
			return session
		}
	}
	t.Fatalf("session %s not listed", id)
	return sessions.ManagedSession{}
}

func TestSessionListManaged(t *testing.T) {
	h, scanner := sessionManagedFixture(t)
	list, err := scanner.ListManaged(context.Background())
	sessionMust(t, err)
	if len(list) != 2 || list[0].ID != "conv-a" {
		t.Fatalf("list = %+v", list)
	}
	a := list[0]
	if a.Provider != sessions.ProviderGemini || a.Root != h.app || a.Path != filepath.Join(h.app, "conversations", "conv-a.db") || a.Problem != "" {
		t.Fatalf("session = %+v", a)
	}
	if a.Title != "Alpha" || a.ProjectName != "alpha" || a.State != sessions.StateRecent || a.Totals.TotalTokens != 3650 || a.EstimatedUSD == nil || a.Model == nil {
		t.Fatalf("item = %+v", a.Item)
	}
	var names []string
	for _, file := range a.Files {
		rel, _ := filepath.Rel(h.app, file.Path)
		names = append(names, filepath.ToSlash(rel))
	}
	want := "brain/conv-a/.hidden/deep/image.png brain/conv-a/task.md annotations/conv-a.pbtxt conversations/conv-a.db"
	if strings.Join(names, " ") != want {
		t.Fatalf("files = %v", names)
	}
}

func TestSessionDeleteRefusesWhileClientMayRun(t *testing.T) {
	_, scanner := sessionManagedFixture(t)
	list, err := scanner.ListManaged(context.Background())
	sessionMust(t, err)
	selected := []sessions.ManagedSession{sessionFind(t, list, "conv-a")}
	trash, trashDir := sessionTrashInto(t)
	for name, running := range map[string]func(string) bool{
		"no hook": nil,
		"cli running": func(root string) bool {
			return filepath.Base(root) == "antigravity-cli"
		},
	} {
		scanner.ClientRunning = running
		results, err := scanner.DeleteManaged(context.Background(), selected, trash, time.Now())
		if !errors.Is(err, ErrSessionClientRunning) || len(results) != 1 || !errors.Is(results[0].Err, ErrSessionClientRunning) || results[0].Removed != 0 {
			t.Fatalf("%s: results = %+v, err = %v", name, results, err)
		}
	}
	if entries, _ := os.ReadDir(trashDir); len(entries) != 0 {
		t.Fatalf("files moved while refused: %v", entries)
	}
	if _, err := os.Stat(scanner.BackupDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("backup written while refused")
	}
}

func TestSessionDeleteBacksUpAndRemovesRows(t *testing.T) {
	h, scanner := sessionManagedFixture(t)
	scanner.ClientRunning = func(string) bool { return false }
	list, err := scanner.ListManaged(context.Background())
	sessionMust(t, err)
	selected := []sessions.ManagedSession{sessionFind(t, list, "conv-a")}
	trash, trashDir := sessionTrashInto(t)
	results, err := scanner.DeleteManaged(context.Background(), selected, trash, time.Date(2026, 11, 13, 1, 2, 3, 4, time.UTC))
	sessionMust(t, err)
	if len(results) != 1 || results[0].Err != nil || results[0].Removed != 4 || results[0].Total != 4 {
		t.Fatalf("results = %+v", results)
	}
	if moved, _ := os.ReadDir(trashDir); len(moved) != 4 || !strings.HasSuffix(moved[len(moved)-1].Name(), "conv-a.db") {
		t.Fatalf("trash = %v", moved)
	}
	if _, err := os.Stat(filepath.Join(h.app, "brain", "conv-a")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("emptied brain folder kept")
	}
	for _, root := range []string{h.app, h.cli} {
		path := filepath.Join(root, sessionSummariesFile)
		for _, table := range []string{"conversation_summaries", "conversation_parents", "conversations_fts"} {
			if n := sessionCount(t, path, table, "conv-a"); n != 0 {
				t.Fatalf("%s %s still has %d rows", filepath.Base(root), table, n)
			}
		}
	}
	if n := sessionCount(t, filepath.Join(h.app, sessionSummariesFile), "conversation_summaries", "conv-b"); n != 1 {
		t.Fatal("unselected row removed")
	}
	backups, err := os.ReadDir(scanner.BackupDir)
	sessionMust(t, err)
	if len(backups) != 2 {
		t.Fatalf("backups = %v", backups)
	}
	for _, backup := range backups {
		path := filepath.Join(scanner.BackupDir, backup.Name())
		if !strings.HasPrefix(backup.Name(), "gemini-antigravity") || !strings.HasSuffix(backup.Name(), "-conversation-summaries-20261113T010203.000000004Z.db") {
			t.Fatalf("backup name = %s", backup.Name())
		}
		if info, _ := os.Stat(path); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("backup mode = %v", info.Mode().Perm())
		}
		if n := sessionCount(t, path, "conversation_summaries", "conv-a"); n != 1 {
			t.Fatalf("backup %s lacks the deleted row", backup.Name())
		}
	}
	after, err := scanner.ListManaged(context.Background())
	sessionMust(t, err)
	if len(after) != 1 || after[0].ID != "conv-b" {
		t.Fatalf("after = %+v", after)
	}
}

func TestSessionDeleteRejectsForeignSelection(t *testing.T) {
	_, scanner := sessionManagedFixture(t)
	scanner.ClientRunning = func(string) bool { return false }
	list, err := scanner.ListManaged(context.Background())
	sessionMust(t, err)
	session := sessionFind(t, list, "conv-a")
	session.ID = "conv-b"
	trash, _ := sessionTrashInto(t)
	if _, err := scanner.DeleteManaged(context.Background(), []sessions.ManagedSession{session}, trash, time.Now()); !errors.Is(err, ErrSessionSelection) {
		t.Fatalf("err = %v", err)
	}
	stale := sessionFind(t, list, "conv-b")
	sessionMust(t, os.WriteFile(stale.Path, []byte("changed after the scan"), 0o600))
	results, err := scanner.DeleteManaged(context.Background(), []sessions.ManagedSession{stale}, trash, time.Now())
	if err == nil || results[0].Removed != 0 {
		t.Fatalf("stale delete = %+v, %v", results, err)
	}
	if n := sessionCount(t, filepath.Join(scanner.SessionRoots()[0], sessionSummariesFile), "conversation_summaries", "conv-b"); n != 1 {
		t.Fatal("row removed although the files stayed")
	}
}
