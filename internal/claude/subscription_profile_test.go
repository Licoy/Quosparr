package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestBrowserLoginProfileSubscriptionReachesManagedAccount(t *testing.T) {
	r := newSyncRig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/token" {
			fmt.Fprint(w, `{"access_token":"test-access","refresh_token":"test-refresh","expires_in":3600}`)
			return
		}
		fmt.Fprint(w, `{"account":{"uuid":"a"},"organization":{"uuid":"o","organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x"}}`)
	}))
	defer server.Close()
	result, err := (OAuthClient{HTTP: server.Client(), TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile"}).Exchange(context.Background(), "test-code", testLoginSession(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	account, err := r.mod.ImportBrowserLogin(context.Background(), result)
	if err != nil {
		t.Fatal(err)
	}
	if account.Tier() != TierMax20x {
		t.Fatalf("browser-imported subscription badge = %q, want Max 20x", account.Tier().DisplayName())
	}
}

func TestSubscriptionProfileUsesOnlyKnownOrganizationTypes(t *testing.T) {
	for kind, want := range map[string]string{"claude_pro": "pro", "claude_max": "max", "claude_team": "team", "claude_enterprise": "enterprise", "future_plan": "", "": ""} {
		raw := []byte(fmt.Sprintf(`{"organization":{"organization_type":%q,"rate_limit_tier":"tier"}}`, kind))
		subscription, tier, err := subscriptionFromProfile(raw)
		if err != nil || subscription != want || tier != "tier" {
			t.Fatalf("organization %q: subscription=%q tier=%q err=%v", kind, subscription, tier, err)
		}
	}
}

func TestUpsertSubscriptionDistinguishesMissingAndExplicitFields(t *testing.T) {
	for _, tc := range []struct {
		name, fields, subscription, tier string
	}{
		{"missing", ``, "max", "default_claude_max_20x"},
		{"empty", `,"subscriptionType":"","rateLimitTier":""`, "", ""},
		{"null", `,"subscriptionType":null,"rateLimitTier":null`, "", ""},
		{"changed", `,"subscriptionType":"pro"`, "pro", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newSyncRig(t)
			identity := CredentialIdentity{AccountUUID: "a", OrganizationUUID: "org"}
			initial := []byte(`{"claudeAiOauth":{"accessToken":"test","subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}}`)
			if _, err := r.store.Upsert(initial, identity, UpsertOptions{Now: r.now}); err != nil {
				t.Fatal(err)
			}
			next := []byte(`{"claudeAiOauth":{"accessToken":"test"` + tc.fields + `}}`)
			acc, err := r.store.Upsert(next, identity, UpsertOptions{Now: r.now})
			if err != nil || acc.SubscriptionType != tc.subscription || acc.RateLimitTier != tc.tier {
				t.Fatalf("subscription=%q tier=%q err=%v", acc.SubscriptionType, acc.RateLimitTier, err)
			}
		})
	}
}

func TestRefreshCompletesSubscriptionWithoutChangingCredentials(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(fmt.Sprint("current=", current), func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "official-access", "a")
			var account ManagedAccount
			var err error
			if current {
				id, syncErr := r.mod.Sync(context.Background())
				if syncErr != nil {
					t.Fatal(syncErr)
				}
				account, err = r.mod.reload(id)
			} else {
				account, err = r.mod.ImportBrowserLogin(context.Background(), browserResult("different", "managed-access"))
			}
			if err != nil {
				t.Fatal(err)
			}
			official, err := r.store.LoadOfficial(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			credential, err := r.store.LoadCredentialData(account.ID)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/profile" {
					calls++
					fmt.Fprintf(w, `{"account":{"uuid":%q},"organization":{"uuid":"org","organization_type":"claude_max","rate_limit_tier":"default_claude_max_20x"}}`, account.Identity.AccountUUID)
					return
				}
				fmt.Fprint(w, `{"five_hour":{"utilization":4}}`)
			}))
			defer server.Close()
			r.mod.Usage = &UsageClient{HTTP: server.Client(), Endpoint: server.URL + "/usage", ProfileEndpoint: server.URL + "/profile"}
			if err := r.mod.Refresh(context.Background(), account.ID, true); err != nil {
				t.Fatal(err)
			}
			acc, err := r.mod.reload(account.ID)
			if err != nil || acc.Tier() != TierMax20x || len(acc.CachedQuota) == 0 {
				t.Fatalf("refresh lost subscription or usage: tier=%q err=%v", acc.Tier().DisplayName(), err)
			}
			if _, err := r.store.Upsert(credential, account.Identity, UpsertOptions{MakeCurrent: current, Now: r.now}); err != nil {
				t.Fatal(err)
			}
			if err := r.mod.Refresh(context.Background(), account.ID, true); err != nil || calls != 1 {
				t.Fatalf("subscription was cleared or fetched again: calls=%d err=%v", calls, err)
			}
			after, err := r.store.LoadOfficial(context.Background())
			if err != nil || !bytes.Equal(official, after) {
				t.Fatal("subscription completion changed official credentials")
			}
			after, err = os.ReadFile(r.store.CredentialPath(account.ID))
			if err != nil || !jsonEqual(credential, after) {
				t.Fatal("subscription completion changed managed tokens")
			}
		})
	}
}

func TestSubscriptionProfileFailureRetainsUsageAndReportsError(t *testing.T) {
	for _, kind := range []string{"identity", "http", "rate-limit", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			r := newSyncRig(t)
			account, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("a", "managed-access"))
			if err != nil {
				t.Fatal(err)
			}
			hits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hits++
				if req.URL.Path == "/usage" {
					fmt.Fprint(w, `{"five_hour":{"utilization":4}}`)
					return
				}
				switch kind {
				case "http":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "rate-limit":
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(http.StatusTooManyRequests)
				case "identity":
					fmt.Fprint(w, `{"account":{"uuid":"other"},"organization":{"uuid":"org","organization_type":"claude_pro"}}`)
				default:
					fmt.Fprint(w, `{"account":{"uuid":"a"},"organization":{"uuid":"org","organization_type":"future_plan","rate_limit_tier":"default_claude_max_20x"}}`)
				}
			}))
			defer server.Close()
			r.mod.Usage = &UsageClient{HTTP: server.Client(), Endpoint: server.URL + "/usage", ProfileEndpoint: server.URL + "/profile"}
			err = r.mod.Refresh(context.Background(), account.ID, true)
			if (kind == "unknown") != (err == nil) {
				t.Fatalf("profile failure propagation: %v", err)
			}
			if kind == "identity" && !errors.Is(err, ErrIdentityRequired) {
				t.Fatal("identity mismatch was not rejected")
			}
			acc, loadErr := r.mod.reload(account.ID)
			if loadErr != nil || len(acc.CachedQuota) == 0 || acc.Tier() != TierUnknown || (kind != "unknown" && acc.LastError == "") {
				t.Fatalf("profile result lost usage, guessed a tier or hid error: tier=%q code=%q err=%v", acc.Tier().DisplayName(), acc.LastError, loadErr)
			}
			if kind == "rate-limit" {
				if err := r.mod.Refresh(context.Background(), account.ID, true); err == nil || hits != 2 {
					t.Fatalf("profile rate limit was bypassed: hits=%d err=%v", hits, err)
				}
			}
		})
	}
}
