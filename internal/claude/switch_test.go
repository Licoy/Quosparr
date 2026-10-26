package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSwitchReplacesOAuthAndAccount(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".claude")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(filepath.Join(home, "accounts"), config, home)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	idA := seedAccount(t, store, "acct-a", "org-a", true, now.Add(time.Hour))
	idB := seedAccount(t, store, "acct-b", "org-b", false, now.Add(time.Hour))
	official := []byte(`{"mcpOAuth":{"server":{"token":"mcp-token"}},"theme":"dark","claudeAiOauth":{"accessToken":"test-access-token-new","refreshToken":"test-refresh-token-new","expiresAt":2000000000000,"subscriptionType":"pro"}}`)
	if err := osWrite(filepath.Join(config, ".credentials.json"), official); err != nil {
		t.Fatal(err)
	}
	claudeJSON := filepath.Join(home, ".claude.json")
	if err := os.WriteFile(claudeJSON, []byte(`{"projects":{"keep":true},"oauthAccount":{"accountUuid":"acct-a","organizationUuid":"org-a","emailAddress":"user@example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mod := NewModule(store)
	mod.Now = func() time.Time { return now }
	if _, err := mod.Apply(context.Background(), Command{Kind: CommandSwitch, ID: idB}); err != nil {
		t.Fatal(err)
	}
	copied := readCred(t, store, idA)
	if copied.OAuth().AccessToken != "test-access-token-new" {
		t.Fatal("current copy was not mirrored")
	}
	gotOfficial, err := os.ReadFile(filepath.Join(config, ".credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotOfficial), "mcp-token") || !strings.Contains(string(gotOfficial), `"theme"`) {
		t.Fatal("official lost sibling keys")
	}
	off, err := ParseAuthDocument(gotOfficial)
	if err != nil {
		t.Fatal(err)
	}
	if off.OAuth().AccessToken != "test-access-token" || off.OAuth().RefreshToken != "test-refresh-token" {
		t.Fatal("official oauth was not the target")
	}
	cfg, err := os.ReadFile(claudeJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), `"keep"`) || !strings.Contains(string(cfg), "acct-b") || strings.Contains(string(cfg), "acct-a") {
		t.Fatalf("oauthAccount %s", cfg)
	}
	// Windows has no POSIX mode bits to keep; Go reports 0666 there.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(claudeJSON)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Fatalf("mode %o", info.Mode().Perm())
		}
	}
	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if idx.CurrentAccountID != idB {
		t.Fatal("current")
	}
}

func TestOAuthAccountConflictRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(`{"projects":{"keep":1},"oauthAccount":{"accountUuid":"old"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n := 0
	replacer := OAuthAccountReplacer{Path: path, BeforeWrite: func(p string) error {
		n++
		if n > 1 {
			return nil
		}
		return os.WriteFile(p, []byte(`{"projects":{"keep":1},"oauthAccount":{"accountUuid":"raced"},"pad":"zzzzzzzzzz"}`), 0o644)
	}}
	if err := replacer.Replace(map[string]any{"accountUuid": "acct-b", "emailAddress": "user@example.com"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"keep"`) || !strings.Contains(string(body), "acct-b") {
		t.Fatalf("body %s", body)
	}
	always := 0
	replacer.BeforeWrite = func(p string) error {
		always++
		return os.WriteFile(p, []byte(`{"oauthAccount":{"accountUuid":"x"},"pad":"`+strings.Repeat("z", always)+`"}`), 0o644)
	}
	if err := replacer.Replace(map[string]any{"accountUuid": "acct-c"}); err == nil || !strings.Contains(err.Error(), "conflicted") {
		t.Fatal(err)
	}
}

func TestSwitchRefreshesExpiredTarget(t *testing.T) {
	srv := tokenServer(t)
	defer srv.Close()
	home := t.TempDir()
	config := filepath.Join(home, ".claude")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(filepath.Join(home, "accounts"), config, home)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	_ = seedAccount(t, store, "acct-a", "org-a", true, now.Add(time.Hour))
	idB := seedAccount(t, store, "acct-b", "org-b", false, now.Add(-time.Hour))
	if err := osWrite(filepath.Join(config, ".credentials.json"), []byte(`{"mcpOAuth":{"keep":true},"claudeAiOauth":{"accessToken":"test-access-token","refreshToken":"test-refresh-token","expiresAt":2000000000000}}`)); err != nil {
		t.Fatal(err)
	}
	mod := NewModule(store)
	mod.Now = func() time.Time { return now }
	mod.Tokens = RefreshClient{HTTP: srv.Client(), Primary: srv.URL + "/token", Fallback: srv.URL + "/nope"}
	if err := mod.Switch(context.Background(), idB, false); err != nil {
		t.Fatal(err)
	}
	doc := readCred(t, store, idB)
	if doc.OAuth().AccessToken != "test-access-token-2" || doc.OAuth().RefreshToken != "test-refresh-token-2" {
		t.Fatal("target was not refreshed before the switch")
	}
	off, err := os.ReadFile(filepath.Join(config, ".credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(off), "mcp") || !strings.Contains(string(off), "test-access-token-2") {
		t.Fatal("official credentials")
	}
}

func tokenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["grant_type"] != "refresh_token" || body["client_id"] != OAuthClientID {
			t.Fatal("refresh body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-access-token-2", "refresh_token": "test-refresh-token-2", "expires_in": 3600,
		})
	}))
}

func TestRunningProcessBlocksSwitch(t *testing.T) {
	home := t.TempDir()
	store := NewStoreAt(filepath.Join(home, "accounts"), filepath.Join(home, ".claude"), home)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	id := seedAccount(t, store, "acct-b", "org-b", false, now.Add(time.Hour))
	mod := NewModule(store)
	mod.Now = func() time.Time { return now }
	mod.RunningProcessIDs = func(context.Context) ([]int, error) { return []int{42}, nil }
	if _, err := mod.Apply(context.Background(), Command{Kind: CommandSwitch, ID: id}); err == nil {
		t.Fatal("expected running process error")
	}
}
