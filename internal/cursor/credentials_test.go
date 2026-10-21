package cursor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/plugins/sqlite"
	"github.com/licoy/quosparr/internal/sqliteruntime"
)

func TestMissingCredentialsDoNotCreateDatabase(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadAuth(context.Background(), dir); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatal("missing store not classified")
	}
	items, _ := os.ReadDir(dir)
	if len(items) != 0 {
		t.Fatal("read created files")
	}
}

func TestReadOnlyCredentialsAndUnsupportedModes(t *testing.T) {
	if err := sqliteruntime.Load(); err != nil {
		if os.Getenv("USAGE_REQUIRE_SQLITE") == "1" {
			t.Fatal(err)
		}
		t.Skip("SQLite runtime unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "User", "globalStorage", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(context.Background(), path, sqlite.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Execute(context.Background(), "CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value BLOB)"); err != nil {
		t.Fatal(err)
	}
	put := func(k, v string) {
		t.Helper()
		if _, err := db.Execute(context.Background(), "INSERT OR REPLACE INTO ItemTable VALUES (?, ?)", k, v); err != nil {
			t.Fatal(err)
		}
	}
	put("cursorAuth/accessToken", "synthetic-local-token")
	put("cursorAuth/cachedEmail", "tester@example.invalid")
	put("cursorAuth/refreshToken", "must-not-be-read")
	before, _ := os.ReadFile(path)
	a, err := ReadAuth(context.Background(), dir)
	if err != nil || a.Token != "synthetic-local-token" || a.Email != "tester@example.invalid" {
		t.Fatal("login read failed")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("official database modified")
	}
	put("cursorAuth/accessToken", "cursor-safe-storage:v1:encrypted")
	if _, err := ReadAuth(context.Background(), dir); !errors.Is(err, ErrUnsupportedLogin) {
		t.Fatal("encrypted token accepted")
	}
	put("cursorAuth/accessToken", "synthetic-local-token")
	put("cursorAuth/activeDeploymentId", "isolated")
	if _, err := ReadAuth(context.Background(), dir); !errors.Is(err, ErrUnsupportedLogin) {
		t.Fatal("isolated deployment accepted")
	}
	put("cursorAuth/activeDeploymentId", string(bytes.Repeat([]byte("x"), 16385)))
	if _, err := ReadAuth(context.Background(), dir); !errors.Is(err, ErrCredentials) {
		t.Fatal("oversize deployment was ignored")
	}
	put("cursorAuth/activeDeploymentId", "0")
	if _, err := db.Execute(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	put("cursorAuth/accessToken", "synthetic-wal-token")
	a, err = ReadAuth(context.Background(), dir)
	if err != nil || a.Token != "synthetic-wal-token" {
		t.Fatal("committed WAL login not observed")
	}
}

func TestUserDataPaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "test-home")
	empty := func(string) string { return "" }
	if UserDataDir("darwin", home, empty) != filepath.Join(home, "Library", "Application Support", "Cursor") {
		t.Fatal("macOS path")
	}
	if UserDataDir("linux", home, empty) != filepath.Join(home, ".config", "Cursor") {
		t.Fatal("Linux path")
	}
	if UserDataDir("windows", home, empty) != filepath.Join(home, "AppData", "Roaming", "Cursor") {
		t.Fatal("Windows fallback")
	}
	root := t.TempDir()
	if got := UserDataDir("linux", home, func(k string) string {
		if k == "XDG_CONFIG_HOME" {
			return root
		}
		return ""
	}); got != filepath.Join(root, "Cursor") {
		t.Fatal("XDG override")
	}
}
