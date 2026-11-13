package migration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/platform"
)

func put(t *testing.T, root, path, value string) {
	t.Helper()
	if err := platform.WriteFileAtomic(filepath.Join(root, path), []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func get(t *testing.T, root, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func indexFixture(t *testing.T, provider, active string, ids ...string) string {
	t.Helper()
	var data []byte
	var err error
	if provider == "grok-index.json" {
		idx := grok.AccountIndex{Version: 1, CurrentAccountID: active}
		for i, id := range ids {
			idx.Accounts = append(idx.Accounts, grok.ManagedAccount{ID: id, SortIndex: i, Identity: grok.CredentialIdentity{Kind: grok.AuthKindOAuth, Scope: "test-scope"}, CreatedAt: time.Unix(1700000000, 0)})
		}
		data, err = grok.MarshalAccountIndex(idx)
	} else if provider == "claude-index.json" {
		idx := claude.AccountIndex{Version: 1, CurrentAccountID: active}
		for i, id := range ids {
			idx.Accounts = append(idx.Accounts, claude.ManagedAccount{ID: id, SortIndex: i, Identity: claude.CredentialIdentity{AccountUUID: id, OrganizationUUID: "org"}, CreatedAt: time.Unix(1700000000, 0).UTC()})
		}
		data, err = json.Marshal(idx)
	} else if provider == "gemini-index.json" {
		idx := gemini.AccountIndex{Version: 1, CurrentAccountID: active}
		for i, id := range ids {
			idx.Accounts = append(idx.Accounts, gemini.ManagedAccount{ID: id, SortIndex: i, Identity: gemini.Identity{Subject: id}, CreatedAt: time.Unix(1700000000, 0).UTC()})
		}
		data, err = json.Marshal(idx)
	} else {
		idx := codex.AccountIndex{Version: 1, ActiveAccountID: active}
		for i, id := range ids {
			idx.Accounts = append(idx.Accounts, codex.ManagedAccount{ID: id, SortIndex: i, DisplayName: id, AuthMode: codex.AuthModeOAuth, CreatedAt: time.Unix(1700000000, 0)})
		}
		data, err = codex.MarshalIndex(idx)
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
func TestMigrationMergesBothLibrariesWithoutChangingNewSelection(t *testing.T) {
	for _, provider := range []string{"index.json", "grok-index.json", "claude-index.json", "gemini-index.json"} {
		t.Run(provider, func(t *testing.T) {
			home := t.TempDir()
			old := filepath.Join(home, ".codex-runway")
			current := platform.RunwayDir(home)
			put(t, old, "accounts/"+provider, indexFixture(t, provider, "older", "same", "older"))
			put(t, current, "accounts/"+provider, indexFixture(t, provider, "newer", "newer", "same"))
			put(t, old, "accounts/same/auth.json", "old same credential")
			put(t, old, "accounts/older/auth.json", "older credential")
			put(t, current, "accounts/same/auth.json", "new same credential")
			if _, err := Run(home); err != nil {
				t.Fatal(err)
			}
			var idx struct {
				Accounts []struct {
					ID   string `json:"id"`
					Sort int    `json:"sortIndex"`
				}
				Active  string `json:"activeAccountId"`
				Current string `json:"currentAccountID"`
			}
			if err := json.Unmarshal([]byte(get(t, current, "accounts/"+provider)), &idx); err != nil {
				t.Fatal(err)
			}
			if len(idx.Accounts) != 3 || idx.Accounts[2].ID != "older" || idx.Accounts[2].Sort != 2 || (idx.Active != "newer" && idx.Current != "newer") {
				t.Fatalf("wrong merged index: %+v", idx)
			}
			if get(t, current, "accounts/same/auth.json") != "new same credential" || get(t, current, "accounts/older/auth.json") != "older credential" {
				t.Fatal("credentials overwritten or missing")
			}
			// A removed migrated account must not resurrect on the next launch.
			if err := os.RemoveAll(filepath.Join(current, "accounts/older")); err != nil {
				t.Fatal(err)
			}
			if copied, err := Run(home); copied || err != nil {
				t.Fatalf("migration repeated: %v %v", copied, err)
			}
			if _, err := os.Stat(filepath.Join(current, "accounts/older")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("deleted account resurrected")
			}
		})
	}
}
func TestMigrationDoesNotMixSQLiteGenerations(t *testing.T) {
	for _, newFile := range []string{"usage.sqlite", "usage.sqlite-wal", "usage.sqlite-shm"} {
		t.Run(newFile, func(t *testing.T) {
			home := t.TempDir()
			old := filepath.Join(home, ".codex-runway")
			current := platform.RunwayDir(home)
			for _, name := range []string{"usage.sqlite", "usage.sqlite-wal", "usage.sqlite-shm", "another.sqlite", "another.sqlite-wal"} {
				put(t, old, name, "old")
			}
			put(t, current, newFile, "new")
			if _, err := Run(home); err != nil {
				t.Fatal(err)
			}
			if get(t, current, newFile) != "new" || get(t, current, "another.sqlite-wal") != "old" {
				t.Fatal("database data overwritten/lost")
			}
			for _, name := range []string{"usage.sqlite", "usage.sqlite-wal", "usage.sqlite-shm"} {
				if name != newFile {
					if _, err := os.Stat(filepath.Join(current, name)); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("mixed database generations")
					}
				}
			}
		})
	}
}
func TestMigrationConflictLeavesCurrentTreeUntouched(t *testing.T) {
	for _, failure := range []string{"type conflict", "invalid index", "unknown version"} {
		t.Run(failure, func(t *testing.T) {
			home := t.TempDir()
			old := filepath.Join(home, ".codex-runway")
			current := platform.RunwayDir(home)
			put(t, current, "keep", "new")
			put(t, old, "first", "would otherwise copy")
			switch failure {
			case "type conflict":
				put(t, current, "accounts", "file")
				put(t, old, "accounts/sample/auth.json", "old")
			case "invalid index":
				put(t, old, "accounts/index.json", "bad JSON")
			case "unknown version":
				put(t, old, "accounts/index.json", `{"version":99,"accounts":[]}`)
			}
			if _, err := Run(home); err == nil {
				t.Fatal("invalid migration succeeded")
			}
			if get(t, current, "keep") != "new" {
				t.Fatal("new data changed")
			}
			if _, err := os.Stat(filepath.Join(current, "first")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("partial data installed")
			}
		})
	}
}
func TestMigrationRecoversEachInterruptedPhase(t *testing.T) {
	for _, phase := range []string{"preparing", "old renamed", "committed"} {
		t.Run(phase, func(t *testing.T) {
			home := t.TempDir()
			old := filepath.Join(home, ".codex-runway")
			current := platform.RunwayDir(home)
			work := filepath.Join(home, ".quosparr-migration-v2")
			put(t, old, "missing", "old")
			put(t, current, "keep", "new")
			put(t, work, "owner", signature)
			put(t, work, "new/incomplete", "partial")
			if phase != "preparing" {
				if err := os.Rename(current, filepath.Join(work, "previous")); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "committed" {
				put(t, current, "keep", "committed new")
				put(t, current, marker, signature)
			}
			if _, err := Run(home); err != nil {
				t.Fatal(err)
			}
			want := "new"
			if phase == "committed" {
				want = "committed new"
			} else if get(t, current, "missing") != "old" {
				t.Fatal("missing recovered data")
			}
			if get(t, current, "keep") != want {
				t.Fatal("recovery lost new data")
			}
			if _, err := os.Stat(work); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("journal not cleaned")
			}
		})
	}
}
func TestMigrationRejectsActiveCurrentAppAndUnknownJournal(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, ".codex-runway"), "old", "old")
	lock, err := platform.AcquireLock(platform.LockPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(home); !errors.Is(err, platform.ErrLocked) {
		t.Fatalf("active app not excluded: %v", err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(home, ".quosparr-migration-v2")
	put(t, work, "unrelated", "retain me")
	if _, err := Run(home); err == nil {
		t.Fatal("unknown staging area removed")
	}
	if get(t, work, "unrelated") != "retain me" {
		t.Fatal("unowned data removed")
	}
}
