package claude

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/licoy/quosparr/internal/platform"
)

func TestBrowserOfficialImportProtectsExpiredCurrentRefresh(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "stale-current"}[existing], func(t *testing.T) {
			r := newSyncRig(t)
			ctx := context.Background()
			r.login(t, "official-expired", "same")
			official := []byte(`{"claudeAiOauth":{"accessToken":"official-expired","refreshToken":"official-refresh","expiresAt":1}}`)
			if err := platform.WriteFileAtomic(CredentialsFilePath(r.config), official, 0600); err != nil {
				t.Fatal(err)
			}
			var before []byte
			result := browserResult("same", "browser-new")
			if existing {
				if _, err := r.store.Upsert(official, result.Identity, UpsertOptions{Now: r.now}); err != nil {
					t.Fatal(err)
				}
				var err error
				before, err = r.store.LoadCredentialData(result.Identity.StableID())
				if err != nil {
					t.Fatal(err)
				}
			}
			acc, err := r.mod.ImportBrowserLogin(ctx, result)
			if !errors.Is(err, ErrLoginAlreadyOfficial) {
				t.Fatal(err)
			}
			idx, err := r.store.LoadIndex()
			if err != nil || idx.CurrentAccountID != acc.ID {
				t.Fatal("official current marker was not synchronized")
			}
			if existing {
				after, e := r.store.LoadCredentialData(acc.ID)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatal("current-marker repair rewrote managed credential")
				}
			}
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"access_token":"unexpected","refresh_token":"unexpected"}`))
			}))
			defer srv.Close()
			r.mod.Tokens = RefreshClient{HTTP: srv.Client(), Primary: srv.URL, Fallback: srv.URL}
			if err := r.mod.Refresh(ctx, acc.ID, true); !errors.Is(err, ErrTokenExpired) {
				t.Fatalf("refresh = %v", err)
			}
			if calls.Load() != 0 {
				t.Fatal("official current refresh token was rotated")
			}
			after, err := os.ReadFile(CredentialsFilePath(r.config))
			if err != nil || !bytes.Equal(after, official) {
				t.Fatal("official credential changed")
			}
		})
	}
}

func TestBrowserOfficialImportFollowsActualAPISettings(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-api-index", true: "active-api-settings"}[active], func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "official-access", "same")
			api, err := r.mod.AddAPIAccount("relay", APIProfile{BaseURL: "https://relay.example.com"}, "test-relay-key")
			if err != nil {
				t.Fatal(err)
			}
			if err := r.store.SetCurrentAccountID(api.ID, r.now); err != nil {
				t.Fatal(err)
			}
			if active {
				if err := platform.WriteFileAtomic(r.store.SettingsPath(), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://relay.example.com/v1","ANTHROPIC_API_KEY":"test-relay-key"}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			settings, _, hadSettings, err := readOptional(r.store.SettingsPath())
			if err != nil {
				t.Fatal(err)
			}
			acc, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("same", "browser-new"))
			if !errors.Is(err, ErrLoginAlreadyOfficial) {
				t.Fatal(err)
			}
			want := acc.ID
			if active {
				want = api.ID
			}
			idx, err := r.store.LoadIndex()
			if err != nil || idx.CurrentAccountID != want {
				t.Fatal("index current marker did not follow actual settings")
			}
			after, _, hasSettings, err := readOptional(r.store.SettingsPath())
			if err != nil || hadSettings != hasSettings || !bytes.Equal(after, settings) {
				t.Fatal("browser import wrote settings")
			}
		})
	}
}
