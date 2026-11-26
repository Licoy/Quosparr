package claude

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseUsageLimitsAndFlat(t *testing.T) {
	flat := []byte(`{
		"five_hour":{"utilization":12.5,"resets_at":"2026-10-14T12:00:00Z"},
		"seven_day":null,
		"seven_day_opus":{"utilization":3,"resets_at":null},
		"seven_day_cowork":{"utilization":1,"resets_at":"2026-10-15T00:00:00Z"},
		"extra_usage":{"is_enabled":true,"monthly_limit":1234,"used_credits":10,"utilization":1.5,"currency":"USD"}
	}`)
	snap, err := ParseUsageSnapshot(flat)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Extra == nil || snap.Extra.MonthlyLimit == nil || snap.Extra.MonthlyLimit.Text != "1234" || snap.Extra.UsedCredits.Text != "10" || snap.Extra.Utilization.Text != "1.5" || snap.Extra.Currency != "USD" || snap.Extra.IsEnabled == nil || !*snap.Extra.IsEnabled {
		t.Fatalf("extra %+v", snap.Extra)
	}
	byKey := map[string]UsageWindow{}
	for _, w := range snap.Windows {
		byKey[w.Key] = w
	}
	if byKey["five_hour"].Kind != WindowFiveHour || byKey["five_hour"].UsedPercent == nil || *byKey["five_hour"].UsedPercent != 12.5 {
		t.Fatal("five hour")
	}
	if !byKey["seven_day"].Null || byKey["seven_day"].Kind != WindowSevenDay {
		t.Fatal("null seven day")
	}
	if byKey["seven_day_opus"].Kind != WindowSevenDayModel || byKey["seven_day_opus"].Model != "opus" {
		t.Fatal("opus")
	}
	if byKey["seven_day_cowork"].Kind != WindowOther || byKey["seven_day_cowork"].Key != "seven_day_cowork" {
		t.Fatal("a product window must not be a model window")
	}
	if got := snap.Extra.FormatAmount(snap.Extra.MonthlyLimit) + "/" + snap.Extra.FormatAmount(snap.Extra.UsedCredits); got != "12.34/0.10" {
		t.Fatalf("minor units %s", got)
	}
	yen := ExtraUsage{Currency: "JPY"}
	if got := yen.FormatAmount(&RawNumber{Text: "500"}); got != "500" {
		t.Fatalf("zero-decimal currency %s", got)
	}
	three := 3
	if got := (ExtraUsage{DecimalPlaces: &three}).FormatAmount(&RawNumber{Text: "5"}); got != "0.005" {
		t.Fatalf("decimal places %s", got)
	}
	both := []byte(`{"limits":[{"key":"five_hour","utilization":9,"resets_at":"2026-10-14T12:00:00Z"}],"five_hour":{"utilization":1,"resets_at":null},"extra_usage":{"monthly_limit":50}}`)
	snap, err = ParseUsageSnapshot(both)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Windows) != 1 || snap.Windows[0].UsedPercent == nil || *snap.Windows[0].UsedPercent != 9 {
		t.Fatal("limits should win")
	}
	if snap.Extra == nil || snap.Extra.MonthlyLimit.Text != "50" {
		t.Fatal("extra from limits response")
	}
	if _, err := ParseUsageSnapshot([]byte(`not-json`)); !errors.Is(err, ErrUsageParse) {
		t.Fatal(err)
	}
}

func TestUsageClientHeadersAndStatus(t *testing.T) {
	var sawQuery bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "cedar_ember=1" || strings.Contains(r.URL.RequestURI(), "skip_spend") {
			sawQuery = true
		}
		wantUA := "claude-cli/2.1.299 (external, cli)"
		if r.URL.Path == "/usage" {
			wantUA = "claude-cli/1.2.3 (external, cli)"
		}
		if r.Header.Get("Authorization") != "Bearer test-access-token" ||
			r.Header.Get("anthropic-beta") != "oauth-2025-04-20" ||
			r.Header.Get("anthropic-version") != "2023-06-01" ||
			r.Header.Get("x-app") != "cli" ||
			r.Header.Get("User-Agent") != wantUA {
			t.Errorf("beta=%q version=%q app=%q ua=%q path=%s", r.Header.Get("anthropic-beta"), r.Header.Get("anthropic-version"), r.Header.Get("x-app"), r.Header.Get("User-Agent"), r.URL.Path)
		}
		switch r.URL.Path {
		case "/limited":
			w.Header().Set("Retry-After", "12")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/limited-bare":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/denied":
			w.WriteHeader(http.StatusUnauthorized)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/boom":
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`{"five_hour":{"utilization":1,"resets_at":null}}`))
		}
	}))
	defer srv.Close()
	client := &UsageClient{HTTP: srv.Client(), Version: "1.2.3", Endpoint: srv.URL + "/usage"}
	body, _, err := client.Fetch(context.Background(), "test-access-token")
	if err != nil || sawQuery {
		t.Fatal(err, sawQuery)
	}
	if _, err := ParseUsageSnapshot(body); err != nil {
		t.Fatal(err)
	}
	client.Version = ""
	client.Endpoint = srv.URL + "/limited"
	_, _, err = client.Fetch(context.Background(), "test-access-token")
	var limited *RateLimitedError
	if !errors.As(err, &limited) || !limited.HasRetryAfter || limited.RetryAfter != 12*time.Second {
		t.Fatal(err)
	}
	client.Endpoint = srv.URL + "/limited-bare"
	_, _, err = client.Fetch(context.Background(), "test-access-token")
	limited = nil
	if !errors.As(err, &limited) || limited.HasRetryAfter || limited.RetryAfter != 0 {
		t.Fatal(err)
	}
	client.Endpoint = srv.URL + "/denied"
	_, _, err = client.Fetch(context.Background(), "test-access-token")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	client.Endpoint = srv.URL + "/forbidden"
	_, _, err = client.Fetch(context.Background(), "test-access-token")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	client.Endpoint = srv.URL + "/boom"
	_, _, err = client.Fetch(context.Background(), "test-access-token")
	if !errors.Is(err, ErrUsageStatus) || errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	ua := UserAgent("")
	if ua != "claude-cli/2.1.299 (external, cli)" {
		t.Fatal(ua)
	}
}

func TestRefreshSavesBothTokensAndInvalidGrant(t *testing.T) {
	var primary, fallback int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["grant_type"] != "refresh_token" || body["client_id"] != OAuthClientID {
			t.Fatal("refresh body")
		}
		switch r.URL.Path {
		case "/primary-miss":
			primary++
			if body["refresh_token"] != "test-refresh-token" {
				t.Fatal("refresh token")
			}
			http.NotFound(w, r)
		case "/fallback":
			fallback++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-access-token-2", "refresh_token": "test-refresh-token-2", "expires_in": 3600,
			})
		case "/invalid":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
		case "/half":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "test-access-token-2"})
		case "/primary-down":
			primary++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()
	home := t.TempDir()
	config := home + "/.claude"
	store := NewStoreAt(home+"/accounts", config, home)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	id := seedAccount(t, store, "acct-b", "org-b", false, now.Add(-time.Hour))
	client := RefreshClient{HTTP: srv.Client(), Primary: srv.URL + "/primary-miss", Fallback: srv.URL + "/fallback"}
	if err := store.RefreshManaged(context.Background(), id, client, now); err != nil {
		t.Fatal(err)
	}
	if primary != 1 || fallback != 1 {
		t.Fatalf("hits %d %d", primary, fallback)
	}
	doc := readCred(t, store, id)
	if doc.OAuth().AccessToken != "test-access-token-2" || doc.OAuth().RefreshToken != "test-refresh-token-2" {
		t.Fatal("both tokens were not saved")
	}
	if !doc.ExpiresAt().Equal(now.Add(time.Hour)) {
		t.Fatal("expiry")
	}
	current := seedAccount(t, store, "acct-a", "org-a", true, now.Add(time.Hour))
	client.Primary = srv.URL + "/invalid"
	if err := store.RefreshManaged(context.Background(), current, client, now); !errors.Is(err, ErrCurrentAccountNotRefreshed) {
		t.Fatal(err)
	}
	id = seedAccount(t, store, "acct-c", "org-c", false, now.Add(-time.Hour))
	client.Primary = srv.URL + "/invalid"
	if err := store.RefreshManaged(context.Background(), id, client, now); !errors.Is(err, ErrInvalidGrant) {
		t.Fatal(err)
	}
	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	acc, _ := idx.Account(id)
	if !acc.RequiresReauth || acc.LastError != ErrorCodeReauth {
		t.Fatal("reauth flag")
	}
	if readCred(t, store, id).OAuth().RefreshToken != "test-refresh-token" {
		t.Fatal("old refresh token was dropped")
	}
	id = seedAccount(t, store, "acct-d", "org-d", false, now.Add(-time.Hour))
	client.Primary = srv.URL + "/half"
	if err := store.RefreshManaged(context.Background(), id, client, now); !errors.Is(err, ErrIncompleteRefresh) {
		t.Fatal(err)
	}
	if readCred(t, store, id).OAuth().AccessToken != "test-access-token" {
		t.Fatal("partial refresh was saved")
	}
	primary = 0
	client.Primary = srv.URL + "/primary-down"
	client.Fallback = srv.URL + "/fallback"
	id = seedAccount(t, store, "acct-e", "org-e", false, now.Add(-time.Hour))
	if err := store.RefreshManaged(context.Background(), id, client, now); !errors.Is(err, ErrRefreshFailed) || primary != 1 || fallback != 1 {
		t.Fatal(err, primary, fallback)
	}
}

func TestCurrentAccountIsNotTokenRefreshed(t *testing.T) {
	tokenHits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "token") {
			tokenHits++
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":4,"resets_at":null}}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	store := NewStoreAt(home+"/accounts", home+"/.claude", home)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	id := seedAccount(t, store, "acct-a", "org-a", true, now.Add(time.Hour))
	if err := osWrite(store.official().(FileStore).Path, mustManaged(t, "test-access-token", "test-refresh-token", now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	mod := NewModule(store)
	mod.Now = func() time.Time { return now }
	mod.Tokens = RefreshClient{HTTP: srv.Client(), Primary: srv.URL + "/token", Fallback: srv.URL + "/token"}
	mod.Usage = &UsageClient{HTTP: srv.Client(), Endpoint: srv.URL + "/usage", Now: mod.Now}
	if err := mod.Refresh(context.Background(), id, false); err != nil {
		t.Fatal(err)
	}
	if tokenHits != 0 {
		t.Fatal("current account refreshed")
	}
	idx, _ := store.LoadIndex()
	acc, _ := idx.Account(id)
	if acc.LastError != "" || len(acc.CachedQuota) == 0 {
		t.Fatal("usage cache")
	}
	expired := seedAccount(t, store, "acct-old", "org-old", true, now.Add(-time.Hour))
	_ = store.SetCurrentAccountID(expired, now)
	if err := osWrite(store.official().(FileStore).Path, mustManaged(t, "test-access-token", "test-refresh-token", now.Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := mod.Refresh(context.Background(), expired, true); !errors.Is(err, ErrTokenExpired) || tokenHits != 0 {
		t.Fatal(err, tokenHits)
	}
}

func seedAccount(t *testing.T, store *Store, account, org string, current bool, exp time.Time) string {
	t.Helper()
	body := mustManaged(t, "test-access-token", "test-refresh-token", exp)
	acc, err := store.Upsert(body, CredentialIdentity{
		AccountUUID: account, OrganizationUUID: org, Email: "user@example.com",
	}, UpsertOptions{MakeCurrent: current, Now: time.Unix(1_700_000_000, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	return acc.ID
}

func mustManaged(t *testing.T, access, refresh string, exp time.Time) []byte {
	t.Helper()
	raw := []byte(`{"claudeAiOauth":{"accessToken":"` + access + `","refreshToken":"` + refresh + `","expiresAt":` + itoa(exp.UnixMilli()) + `,"scopes":["user:inference"],"subscriptionType":"pro","rateLimitTier":"default_claude_max_5x"}}`)
	out, err := ManagedDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func readCred(t *testing.T, store *Store, id string) AuthDocument {
	t.Helper()
	data, err := store.LoadCredentialData(id)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ParseAuthDocument(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func itoa(n int64) string {
	return json.Number(jsonNumber(n)).String()
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func osWrite(path string, data []byte) error {
	return FileStore{Path: path}.Save(context.Background(), data)
}

func TestSpacedUsageReadKeepsCacheWithoutError(t *testing.T) {
	hits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":4,"resets_at":null}}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	store := NewStoreAt(home+"/accounts", home+"/.claude", home)
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	id := seedAccount(t, store, "acct-a", "org-a", true, now.Add(time.Hour))
	if err := osWrite(store.official().(FileStore).Path, mustManaged(t, "test-access-token", "test-refresh-token", now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	mod := NewModule(store)
	mod.Now = func() time.Time { return now }
	mod.Usage = &UsageClient{HTTP: srv.Client(), Endpoint: srv.URL + "/usage", Now: mod.Now}
	if err := mod.Refresh(context.Background(), id, false); err != nil || hits != 1 {
		t.Fatal(err, hits)
	}
	now = now.Add(time.Minute)
	if err := mod.Refresh(context.Background(), id, false); err != nil || hits != 1 {
		t.Fatal("spaced read", err, hits)
	}
	idx, _ := store.LoadIndex()
	if acc, _ := idx.Account(id); acc.LastError != "" || len(acc.CachedQuota) == 0 {
		t.Fatalf("spaced read marked the account: %q", acc.LastError)
	}
	if err := mod.Refresh(context.Background(), id, true); err != nil || hits != 2 {
		t.Fatal("forced read", err, hits)
	}
	mod.throttle().ObserveRateLimit("test-access-token", now, time.Hour, true)
	var limited *RateLimitedError
	if err := mod.Refresh(context.Background(), id, true); !errors.As(err, &limited) || hits != 2 {
		t.Fatal("a 429 block must still be reported", err)
	}
}
