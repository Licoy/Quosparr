package grok

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseClientVersion(t *testing.T) {
	cases := []struct {
		raw, want string
	}{
		{"grok 0.2.114\n", "0.2.114"},
		{"0.2.93", "0.2.93"},
		{"banner\ngrok 1.2.3-beta.1 extra", "1.2.3-beta.1"},
		{"", ""},
		{"no version here", ""},
	}
	for _, tc := range cases {
		if got := parseClientVersion(tc.raw); got != tc.want {
			t.Errorf("parse %q = %q, want %q", tc.raw, got, tc.want)
		}
	}
	if got := normalizedClientVersion("  "); got != fallbackClientVersion {
		t.Errorf("empty normalized = %q", got)
	}
	if got := normalizedClientVersion("not-a-version"); got != "not-a-version" {
		t.Errorf("unparsed normalized = %q", got)
	}
}

func TestBillingFetchMergesPlanMoneyAndReset(t *testing.T) {
	token := syntheticTierToken(1)
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		assertIdentity(t, r, token, "9.9.9")
		switch {
		case r.URL.Path == "/v1/billing" && r.URL.Query().Get("format") == "credits":
			writeJSON(w, `{"config":{"creditUsagePercent":10,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_MONTHLY","start":"2026-06-01T00:00:00Z","end":"2026-07-01T00:00:00Z"}}}`)
		case r.URL.Path == "/v1/billing":
			writeJSON(w, `{"config":{"monthlyLimit":{"val":15000},"used":{"val":277}}}`)
		case r.URL.Path == "/v1/settings":
			writeJSON(w, `{"subscription_tier_display":"SuperGrok Heavy"}`)
		case r.URL.Path == "/reset" && r.Method == http.MethodPost:
			if r.Header.Get("Content-Type") != "application/grpc-web+proto" || r.Header.Get("X-Grpc-Web") != "1" {
				t.Errorf("reset headers content-type %q grpc %q", r.Header.Get("Content-Type"), r.Header.Get("X-Grpc-Web"))
			}
			writeJSON(w, `{"tokens":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	home := t.TempDir()
	writeAuth(t, home, token)
	client := &BillingClient{
		HTTP: srv.Client(), BaseURL: srv.URL + "/v1", ResetCreditsURL: srv.URL + "/reset", SubscriptionsURL: srv.URL + "/subscriptions",
		Version: func() string { return "9.9.9" },
	}
	raw, when, err := client.Fetch(context.Background(), home)
	if err != nil || when.IsZero() {
		t.Fatalf("fetch: %v when %v", err, when)
	}
	snap, err := ParseBillingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Plan == nil || *snap.Plan != "SuperGrok Heavy" || snap.IncludedUsagePercent != 10 {
		t.Fatalf("plan/percent = %v %v", snap.Plan, snap.IncludedUsagePercent)
	}
	if snap.IncludedLimitCents == nil || *snap.IncludedLimitCents != 15000 || snap.IncludedUsedCents == nil || *snap.IncludedUsedCents != 277 {
		t.Fatalf("money %v %v", snap.IncludedLimitCents, snap.IncludedUsedCents)
	}
	if snap.ResetCredits == nil {
		t.Fatal("reset credits were not merged")
	}
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(seen, "\n")
	for _, want := range []string{"GET /v1/billing?format=credits", "GET /v1/billing", "GET /v1/settings", "POST /reset"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %s", want, joined)
		}
	}
}

func TestBillingFetchUsesJWTWhenSettingsOmitPlan(t *testing.T) {
	token := syntheticTierToken(1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/settings" || r.URL.Path == "/reset" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, `{"config":{"creditUsagePercent":1,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","start":"2026-08-01T00:00:00Z","end":"2026-08-08T00:00:00Z"}}}`)
	}))
	defer srv.Close()
	home := t.TempDir()
	writeAuth(t, home, token)
	client := &BillingClient{
		HTTP: srv.Client(), BaseURL: srv.URL + "/v1", ResetCreditsURL: srv.URL + "/reset", SubscriptionsURL: srv.URL + "/subscriptions",
		Version: func() string { return "0.2.93" },
	}
	raw, _, err := client.Fetch(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := ParseBillingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Plan == nil || *snap.Plan != "SuperGrok" {
		t.Fatalf("plan = %v", snap.Plan)
	}
}

func TestBillingFetchAuthStatusOmitsBody(t *testing.T) {
	token := syntheticTierToken(1)
	const secret = "secret-body-must-not-leak"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(secret))
	}))
	defer srv.Close()
	home := t.TempDir()
	writeAuth(t, home, token)
	client := &BillingClient{
		HTTP: srv.Client(), BaseURL: srv.URL + "/v1", ResetCreditsURL: srv.URL + "/reset", SubscriptionsURL: srv.URL + "/subscriptions",
		Version: func() string { return "9.9.9" },
	}
	_, _, err := client.Fetch(context.Background(), home)
	if !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("auth error included the response body")
	}
}

func TestBillingFetchMissingAuthDoesNotUseHome(t *testing.T) {
	home := t.TempDir()
	client := &BillingClient{Version: func() string { return "9.9.9" }, HTTP: http.DefaultClient}
	_, _, err := client.Fetch(context.Background(), home)
	if !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, "auth.json")); !os.IsNotExist(statErr) {
		t.Fatalf("auth.json appeared: %v", statErr)
	}
}

func syntheticTierToken(tier int) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"tier":` + itoa(tier) + `}`))
	return "e30." + payload + ".c2ln"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func writeAuth(t *testing.T, home, token string) {
	t.Helper()
	data := credSpec{email: "synthetic@example.com", user: "synthetic", access: token, refresh: "synthetic-refresh"}.json()
	if err := os.WriteFile(filepath.Join(home, "auth.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func assertIdentity(t *testing.T, r *http.Request, token, version string) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer "+token {
		t.Errorf("%s authorization mismatch", r.URL.Path)
	}
	if r.Header.Get(tokenAuthHeader) != tokenAuthValue ||
		r.Header.Get(clientVersionHeader) != version ||
		r.Header.Get(clientIDHeader) != clientIDValue ||
		r.Header.Get(clientModeHeader) != clientModeValue ||
		r.Header.Get("User-Agent") != userAgentPrefix+version ||
		r.Header.Get("Accept") == "" {
		t.Errorf("%s identity headers incomplete", r.URL.Path)
	}
}
