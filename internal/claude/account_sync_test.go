package claude

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type syncRig struct {
	home, config string
	store        *Store
	mod          *Module
	now          time.Time
}

func newSyncRig(t *testing.T) *syncRig {
	t.Helper()
	home := t.TempDir()
	config := filepath.Join(home, ".claude")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	r := &syncRig{home: home, config: config, now: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	r.store = NewStoreAt(filepath.Join(home, "accounts"), config, home)
	r.mod = NewModule(r.store)
	r.mod.Now = func() time.Time { return r.now }
	return r
}

func (r *syncRig) login(t *testing.T, access, account string) {
	t.Helper()
	body := `{"claudeAiOauth":{"accessToken":"` + access + `","refreshToken":"r-` + access + `","expiresAt":2000000000000}}`
	if err := osWrite(filepath.Join(r.config, ".credentials.json"), []byte(body)); err != nil {
		t.Fatal(err)
	}
	cfg := `{"projects":{}}`
	if account != "" {
		cfg = `{"oauthAccount":{"accountUuid":"` + account + `","organizationUuid":"org","emailAddress":"` + account + `@example.com"}}`
	}
	if err := os.WriteFile(filepath.Join(r.home, ".claude.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncImportsAndFollowsTheOfficialLogin(t *testing.T) {
	r := newSyncRig(t)
	ctx := context.Background()
	if id, err := r.mod.Sync(ctx); err != nil || id != "" {
		t.Fatalf("no login: %q %v", id, err)
	}
	r.login(t, "test-access-a", "acct-a")
	first, err := r.mod.Sync(ctx)
	if err != nil || first == "" {
		t.Fatalf("first run must import the login: %q %v", first, err)
	}
	r.login(t, "test-access-b", "acct-b")
	second, err := r.mod.Sync(ctx)
	if err != nil || second == first {
		t.Fatalf("an external switch must move the current account: %q %v", second, err)
	}
	if readCred(t, r.store, first).OAuth().AccessToken != "test-access-a" {
		t.Fatal("the previous account's copy was overwritten")
	}
	idx, _ := r.store.LoadIndex()
	if idx.CurrentAccountID != second || len(idx.Accounts) != 2 {
		t.Fatalf("index %+v", idx)
	}
	if err := os.Remove(filepath.Join(r.config, ".credentials.json")); err != nil {
		t.Fatal(err)
	}
	if id, err := r.mod.Sync(ctx); err != nil || id != "" {
		t.Fatalf("logout must clear the current id: %q %v", id, err)
	}
}

func TestSyncAsksProfileWhenOAuthAccountIsMissing(t *testing.T) {
	r := newSyncRig(t)
	calls := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"account":{"uuid":"acct-p","email":"p@example.com"},"organization":{"uuid":"org-p"}}`))
	}))
	defer srv.Close()
	r.mod.Usage = &UsageClient{HTTP: srv.Client(), ProfileEndpoint: srv.URL, Endpoint: srv.URL}
	r.login(t, "test-access-p", "")
	ctx := context.Background()
	id, err := r.mod.Sync(ctx)
	want := CredentialIdentity{AccountUUID: "acct-p", OrganizationUUID: "org-p"}.StableID()
	if err != nil || id != want {
		t.Fatalf("profile identity: %q %v", id, err)
	}
	if _, err := r.mod.Sync(ctx); err != nil || calls != 1 {
		t.Fatalf("profile must be asked once per token: %d %v", calls, err)
	}
	if _, err := r.mod.Apply(ctx, Command{Kind: CommandImportOfficial}); err != nil {
		t.Fatal(err)
	}
}
