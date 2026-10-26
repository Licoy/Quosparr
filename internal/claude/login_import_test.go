package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

func browserResult(account, access string) LoginResult {
	return LoginResult{Credential: []byte(`{"claudeAiOauth":{"accessToken":"` + access + `","refreshToken":"refresh-` + access + `","expiresAt":2000000000000}}`), Identity: CredentialIdentity{AccountUUID: account, OrganizationUUID: "org"}}
}

func TestBrowserImportNewAndDuplicatePreservesMetadata(t *testing.T) {
	r := newSyncRig(t)
	ctx := context.Background()
	result := browserResult("new", "browser-access-one")
	first, err := r.mod.ImportBrowserLogin(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	first.Alias, first.SortIndex, first.SubscriptionType, first.RateLimitTier = "my alias", 7, "pro", "known"
	first.Identity.Email = "known@example.com"
	first.CachedQuota = json.RawMessage(`{"five_hour":{"utilization":10}}`)
	if err := r.store.UpdateMetadata(first); err != nil {
		t.Fatal(err)
	}
	second, err := r.mod.ImportBrowserLogin(ctx, browserResult("new", "browser-access-two"))
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Alias != first.Alias || second.SortIndex != 7 || !second.CreatedAt.Equal(first.CreatedAt) || second.SubscriptionType != "pro" || second.RateLimitTier != "known" || second.Identity.Email != first.Identity.Email || !jsonEqual(second.CachedQuota, first.CachedQuota) {
		t.Fatal("duplicate discarded account metadata")
	}
	doc := readCred(t, r.store, first.ID)
	if doc.OAuth().AccessToken != "browser-access-two" || doc.OAuth().RefreshToken != "refresh-browser-access-two" || doc.ExpiresAt().UnixMilli() != 2000000000000 {
		t.Fatal("rotated credentials were not committed")
	}
	idx, err := r.store.LoadIndex()
	if err != nil || len(idx.Accounts) != 1 || idx.CurrentAccountID != "" {
		t.Fatal("browser import selected or duplicated account")
	}
	raw, err := os.ReadFile(r.store.IndexPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "browser-access") || rejectTokenKeys(raw) != nil {
		t.Fatal("index contains credentials")
	}
	for _, path := range []string{r.store.CredentialPath(first.ID), r.store.IndexPath()} {
		if err := platform.VerifyOwnerOnly(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := platform.VerifyOwnerOnly(r.store.AccountDir(first.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(CredentialsFilePath(r.config)); !os.IsNotExist(err) {
		t.Fatal("browser login wrote official credentials")
	}
}

func TestBrowserImportOfficialDuplicateIncludingAPISelection(t *testing.T) {
	for _, apiCurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "subscription", true: "api"}[apiCurrent], func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "official-old", "same")
			if _, err := r.mod.Sync(context.Background()); err != nil {
				t.Fatal(err)
			}
			if apiCurrent {
				api, err := r.mod.addAPI("relay", APIProfile{BaseURL: "https://relay.example.com"}, "test-api-key")
				if err != nil {
					t.Fatal(err)
				}
				if err := r.store.SetCurrentAccountID(api.ID, r.now); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(r.store.SettingsPath(), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://relay.example.com","ANTHROPIC_AUTH_TOKEN":"test-api-key"}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			paths := []string{r.store.IndexPath(), r.store.CredentialPath(browserResult("same", "x").Identity.StableID()), CredentialsFilePath(r.config), filepath.Join(r.home, ".claude.json"), r.store.SettingsPath()}
			before := map[string][]byte{}
			for _, path := range paths {
				before[path], _ = os.ReadFile(path)
			}
			if _, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("same", "browser-new")); !errors.Is(err, ErrLoginAlreadyOfficial) {
				t.Fatalf("duplicate = %v", err)
			}
			for _, path := range paths {
				after, _ := os.ReadFile(path)
				if !bytes.Equal(before[path], after) {
					t.Fatalf("duplicate changed %s", filepath.Base(path))
				}
			}
		})
	}
}

func TestBrowserImportExpiredOfficialKnownIdentity(t *testing.T) {
	r := newSyncRig(t)
	r.login(t, "expired", "old")
	if err := os.WriteFile(CredentialsFilePath(r.config), []byte(`{"claudeAiOauth":{"accessToken":"expired","expiresAt":1}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("different", "browser-new")); err != nil {
		t.Fatal(err)
	}
	if read, _ := os.ReadFile(CredentialsFilePath(r.config)); !strings.Contains(string(read), `"expiresAt":1`) {
		t.Fatal("official login changed")
	}
}

func TestBrowserImportRejectsUnknownOfficialIdentity(t *testing.T) {
	r := newSyncRig(t)
	r.login(t, "unknown", "")
	if _, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("new", "browser-new")); !errors.Is(err, ErrIdentityRequired) {
		t.Fatalf("unknown identity = %v", err)
	}
	if _, err := os.Stat(r.store.IndexPath()); !os.IsNotExist(err) {
		t.Fatal("failed login wrote index")
	}
}

func TestBrowserImportProofRecheckedAndCancelledBeforeCommit(t *testing.T) {
	for _, change := range []string{"cancel", "official", "identity", "settings"} {
		t.Run(change, func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "old", "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Network verification must run without Module.mu or Store.mu held.
				if _, err := r.mod.Load(context.Background()); err != nil {
					t.Error(err)
				}
				switch change {
				case "cancel":
					cancel()
				case "official":
					r.login(t, "changed", "")
				case "settings":
					if err := os.WriteFile(r.store.SettingsPath(), []byte(`{"env":{}}`), 0600); err != nil {
						t.Error(err)
					}
				case "identity":
					if err := os.WriteFile(filepath.Join(r.home, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"changed","organizationUuid":"org"}}`), 0600); err != nil {
						t.Error(err)
					}
				}
				_, _ = w.Write([]byte(`{"account":{"uuid":"old"},"organization":{"uuid":"org"}}`))
			}))
			defer srv.Close()
			r.mod.Usage = &UsageClient{HTTP: srv.Client(), ProfileEndpoint: srv.URL}
			_, err := r.mod.ImportBrowserLogin(ctx, browserResult("new", "browser-new"))
			if change == "cancel" && !errors.Is(err, context.Canceled) || change != "cancel" && !errors.Is(err, ErrCredentialChanged) {
				t.Fatalf("import = %v", err)
			}
			idx, e := r.store.LoadIndex()
			if e != nil || len(idx.Accounts) != 0 {
				t.Fatal("rejected login wrote an account")
			}
		})
	}
}

func TestBrowserImportPreCancelledDoesNotCreateLibrary(t *testing.T) {
	r := newSyncRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.mod.ImportBrowserLogin(ctx, browserResult("new", "browser-new")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.store.Root()); !os.IsNotExist(err) {
		t.Fatal("cancelled import created library")
	}
}

func TestBrowserImportPreventsInflightRefreshOverwrite(t *testing.T) {
	r := newSyncRig(t)
	ctx := context.Background()
	result := browserResult("new", "browser-old")
	acc, err := r.mod.ImportBrowserLogin(ctx, result)
	if err != nil {
		t.Fatal(err)
	}
	original, err := r.store.credentialIfRefreshable(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.mod.ImportBrowserLogin(ctx, browserResult("new", "browser-new")); err != nil {
		t.Fatal(err)
	}
	updated, err := ApplyTokenRefresh(original, "refresh-access", "refresh-secret", time.Now().Add(time.Hour), true)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.store.storeRefreshed(acc.ID, original, updated); !errors.Is(err, ErrCredentialChanged) {
		t.Fatal("old refresh overwrote browser login")
	}
}

func jsonEqual(a, b []byte) bool {
	var first, second bytes.Buffer
	return json.Compact(&first, a) == nil && json.Compact(&second, b) == nil && bytes.Equal(first.Bytes(), second.Bytes())
}

func TestBrowserImportAndSyncKeepSeparateCredentialCopies(t *testing.T) {
	r := newSyncRig(t)
	r.login(t, "official-access", "official")
	ctx := context.Background()
	acc, err := r.mod.ImportBrowserLogin(ctx, browserResult("new", "browser-access"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := r.mod.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id == acc.ID || readCred(t, r.store, acc.ID).OAuth().AccessToken != "browser-access" || readCred(t, r.store, id).OAuth().AccessToken != "official-access" {
		t.Fatal("sync mixed browser and official credentials")
	}
}

func TestBrowserImportRejectsIncompleteResult(t *testing.T) {
	for _, kind := range []string{"account", "org", "tokens", "expiry"} {
		t.Run(kind, func(t *testing.T) {
			r := newSyncRig(t)
			result := browserResult("new", "browser-new")
			switch kind {
			case "account":
				result.Identity.AccountUUID = ""
			case "org":
				result.Identity.OrganizationUUID = ""
			case "tokens":
				result.Credential = []byte(`{"claudeAiOauth":{"accessToken":"only-access","expiresAt":2000000000000}}`)
			case "expiry":
				result.Credential = []byte(`{"claudeAiOauth":{"accessToken":"access","refreshToken":"refresh"}}`)
			}
			if _, err := r.mod.ImportBrowserLogin(context.Background(), result); err == nil {
				t.Fatal("incomplete result accepted")
			}
			if _, err := os.Stat(r.store.Root()); !os.IsNotExist(err) {
				t.Fatal("invalid result touched library")
			}
		})
	}
}

func TestBrowserImportOfficialMissingLibraryMirrorsOfficialOnly(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "expired"}[expired], func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "official-authority", "same")
			if expired {
				if err := os.WriteFile(CredentialsFilePath(r.config), []byte(`{"claudeAiOauth":{"accessToken":"official-authority","expiresAt":1}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			official, err := os.ReadFile(CredentialsFilePath(r.config))
			if err != nil {
				t.Fatal(err)
			}
			acc, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("same", "browser-new"))
			if !errors.Is(err, ErrLoginAlreadyOfficial) || acc.ID == "" || acc.RequiresReauth != expired {
				t.Fatalf("official mirror state: %s reauth=%v error=%v", acc.ID, acc.RequiresReauth, err)
			}
			if readCred(t, r.store, acc.ID).OAuth().AccessToken != "official-authority" {
				t.Fatal("browser result replaced official authoritative credential")
			}
			after, err := os.ReadFile(CredentialsFilePath(r.config))
			if err != nil || !bytes.Equal(after, official) {
				t.Fatal("official credentials changed")
			}
			idx, err := r.store.LoadIndex()
			if err != nil || len(idx.Accounts) != 1 || idx.CurrentAccountID != acc.ID {
				t.Fatal("mirror did not follow actual official login")
			}
		})
	}
}
