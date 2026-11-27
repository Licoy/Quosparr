package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type cloudCall struct {
	method, path, ua, apiClient, auth string
	body                              map[string]any
}

type cloudServer struct {
	*httptest.Server
	mu     sync.Mutex
	calls  []cloudCall
	routes map[string]func(w http.ResponseWriter, call cloudCall)
}

func newCloudServer(t *testing.T, routes map[string]func(w http.ResponseWriter, call cloudCall)) *cloudServer {
	c := &cloudServer{routes: routes}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		call := cloudCall{method: r.Method, path: r.URL.Path, ua: r.Header.Get("User-Agent"), apiClient: r.Header.Get("x-goog-api-client"), auth: r.Header.Get("Authorization")}
		_ = json.Unmarshal(raw, &call.body)
		c.mu.Lock()
		c.calls = append(c.calls, call)
		handler := c.routes[r.URL.Path]
		c.mu.Unlock()
		if handler == nil {
			http.NotFound(w, r)
			return
		}
		handler(w, call)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *cloudServer) paths() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, call := range c.calls {
		out = append(out, call.method+" "+call.path)
	}
	return out
}

func reply(body string) func(http.ResponseWriter, cloudCall) {
	return func(w http.ResponseWriter, _ cloudCall) { _, _ = w.Write([]byte(body)) }
}

const summaryBody = `{"groups":[{"displayName":"Claude","buckets":[
 {"bucketId":"claude:5h","displayName":"Claude 5h","remainingFraction":0.25,"resetTime":"2026-11-13T12:00:00Z"},
 {"bucketId":"3p-weekly","remainingFraction":1}]},
 {"buckets":[{"bucketId":"gemini-5h"},{"bucketId":"gemini-argon-wtu-weekly","remainingFraction":0.5},
 {"bucketId":"mystery-weekly","displayName":"Mystery","remainingFraction":0.75}]}]}`

func testQuotaClient(base string) *QuotaClient {
	return &QuotaClient{
		BaseURL: base, ClientVersion: func() string { return "2.31.2" }, GOOS: "darwin", GOARCH: "arm64",
		Now: func() time.Time { return testNow },
	}
}

// A reading is loadCodeAssist and the summary, nothing else: no onboarding
// for an account without a project, and no model list.
func TestQuotaFetchSendsTwoRequestsAndParsesBuckets(t *testing.T) {
	server := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist": reply(`{"cloudaicompanionProject":"aicode-consumers","currentTier":{"id":"free-tier","name":"Free"},
			"paidTier":{"id":"g1-ultra-tier","name":"Ultra","availableCredits":[{"creditType":"GOOGLE_ONE_AI","creditAmount":"1500","minimumCreditAmountForUsage":"10"}]},
			"allowedTiers":[{"id":"legacy-tier"},{"id":"free-tier","isDefault":true}]}`),
		"/v1internal:onboardUser":              reply(`{"done":true}`),
		"/v1internal:retrieveUserQuotaSummary": reply(summaryBody),
		"/v1internal:fetchAvailableModels":     reply(`{"models":{}}`),
	})
	snap, err := testQuotaClient(server.URL).Fetch(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(server.paths(), ","); got != "POST /v1internal:loadCodeAssist,POST /v1internal:retrieveUserQuotaSummary" {
		t.Fatalf("calls = %s", got)
	}
	if snap.Project != "" || snap.Source != SourceSummary || snap.Tier != (Tier{Kind: TierUltra, ID: "g1-ultra-tier", Name: "Ultra"}) {
		t.Fatalf("snapshot = %+v", snap)
	}
	if len(snap.Credits) != 1 || snap.Credits[0].Amount != 1500 || snap.Credits[0].MinimumForUsage != 10 {
		t.Fatalf("credits = %+v", snap.Credits)
	}
	want := []struct{ id, family, window string }{
		{"claude:5h", FamilyClaude, WindowFiveHour}, {"3p-weekly", FamilyClaude, WindowWeekly},
		{"gemini-5h", FamilyGemini, WindowFiveHour}, {"gemini-argon-wtu-weekly", FamilyArgon, WindowWeekly},
		{"mystery-weekly", FamilyOther, WindowWeekly},
	}
	if len(snap.Buckets) != len(want) {
		t.Fatalf("buckets = %+v", snap.Buckets)
	}
	for i, w := range want {
		b := snap.Buckets[i]
		if b.ID != w.id || b.Family != w.family || b.Window != w.window {
			t.Errorf("bucket %d = %+v", i, b)
		}
	}
	if snap.Buckets[0].RemainingFraction != 0.25 || snap.Buckets[0].ResetAt == nil || snap.Buckets[0].Group != "Claude" {
		t.Fatal("claude bucket details")
	}
	if snap.Buckets[2].RemainingFraction != 0 {
		t.Fatal("an omitted fraction reads as exhausted")
	}
	if snap.Buckets[4].DisplayName != "Mystery" {
		t.Fatal("unknown bucket keeps its display name")
	}
	if len(snap.Models) != 0 {
		t.Fatalf("models = %+v", snap.Models)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	load := server.calls[0]
	if load.ua != "antigravity/2.31.2 darwin/arm64 google-api-nodejs-client/10.3.0" || load.apiClient != "gl-node/22.21.1" || load.auth != "Bearer tok" {
		t.Fatalf("load headers: %q %q", load.ua, load.apiClient)
	}
	meta := load.body["metadata"].(map[string]any)
	if load.body["mode"] != "FULL_ELIGIBILITY_CHECK" || meta["platform"] != "DARWIN_ARM64" || meta["ideType"] != "ANTIGRAVITY" ||
		meta["pluginType"] != "GEMINI" || meta["ideVersion"] != "2.31.2" || meta["updateChannel"] != "stable" {
		t.Fatalf("metadata = %v", meta)
	}
	summary := server.calls[1]
	if summary.ua != "antigravity/hub/2.31.2 (aidev_client; os_type=darwin; arch=arm64)" || len(summary.body) != 0 {
		t.Fatalf("quota call without project: %q %v", summary.ua, summary.body)
	}
}

func TestQuotaFetchUsesProductionForGcpTerms(t *testing.T) {
	prod := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:retrieveUserQuotaSummary": func(w http.ResponseWriter, call cloudCall) {
			if call.body["project"] != "gcp-proj" {
				t.Errorf("project = %v", call.body["project"])
			}
			_, _ = w.Write([]byte(`{"groups":[]}`))
		},
	})
	daily := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist": reply(`{"cloudaicompanionProject":{"id":"gcp-proj"},"currentTier":{"id":"standard-tier"},"allowedTiers":[{"id":"standard-tier","usesGcpTos":true}]}`),
	})
	c := testQuotaClient("")
	c.DailyURL, c.ProdURL = daily.URL, prod.URL
	t.Setenv(CloudCodeURLEnv, "")
	snap, err := c.Fetch(context.Background(), "tok")
	if err != nil || snap.Tier.Kind != TierFree {
		t.Fatalf("fetch: %+v %v", snap, err)
	}
	if got := strings.Join(prod.paths(), ","); got != "POST /v1internal:retrieveUserQuotaSummary" {
		t.Fatalf("prod calls = %s", got)
	}
}

func TestQuotaFetchEnvironmentOverride(t *testing.T) {
	server := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist":           reply(`{"cloudaicompanionProject":"p","currentTier":{"id":"free-tier"}}`),
		"/v1internal:retrieveUserQuotaSummary": reply(`{"groups":[]}`),
	})
	t.Setenv(CloudCodeURLEnv, server.URL)
	c := testQuotaClient("")
	c.DailyURL, c.ProdURL = "http://127.0.0.1:1", "http://127.0.0.1:1"
	if _, err := c.Fetch(context.Background(), "tok"); err != nil {
		t.Fatal(err)
	}
}

func TestQuotaErrorsValidationUnauthorizedRateLimit(t *testing.T) {
	forbidden := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist": func(w http.ResponseWriter, _ cloudCall) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/verify?continue=x&access_token=leak"}}]}}`))
		},
	})
	_, err := testQuotaClient(forbidden.URL).Fetch(context.Background(), "tok")
	var v *ValidationRequiredError
	if !errors.As(err, &v) || v.Reason != "VALIDATION_REQUIRED" || v.URL != "https://accounts.google.com/verify?continue=x" {
		t.Fatalf("validation: %v %+v", err, v)
	}
	if ErrorCode(err) != ErrorCodeValidationRequired {
		t.Fatal("validation code")
	}
	if ErrorCode(&ValidationRequiredError{Reason: "SUBSCRIPTION_REQUIRED"}) != ErrorCodeSubscriptionRequired {
		t.Fatal("subscription code")
	}

	unauthorized := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist": func(w http.ResponseWriter, _ cloudCall) { w.WriteHeader(http.StatusUnauthorized) },
	})
	if _, err := testQuotaClient(unauthorized.URL).Fetch(context.Background(), "tok"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401: %v", err)
	}

	limited := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist": reply(`{"cloudaicompanionProject":"p"}`),
		"/v1internal:retrieveUserQuotaSummary": func(w http.ResponseWriter, _ cloudCall) {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(http.StatusTooManyRequests)
		},
	})
	_, err = testQuotaClient(limited.URL).Fetch(context.Background(), "tok")
	var rl *RateLimitedError
	if !errors.As(err, &rl) || !rl.HasRetryAfter || rl.RetryAfter != 2*time.Minute {
		t.Fatalf("429: %v", err)
	}
	if got := len(limited.paths()); got != 2 {
		t.Fatalf("a 429 is not retried: %d calls", got)
	}
}

// A failed summary is the reading's error: no model-list fallback, no retry.
func TestQuotaSummaryFailureIsNotRetried(t *testing.T) {
	server := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
		"/v1internal:loadCodeAssist": reply(`{"cloudaicompanionProject":"p","currentTier":{"id":"g1-pro-tier"}}`),
		"/v1internal:retrieveUserQuotaSummary": func(w http.ResponseWriter, _ cloudCall) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
		"/v1internal:fetchAvailableModels": reply(`{"models":{}}`),
	})
	_, err := testQuotaClient(server.URL).Fetch(context.Background(), "tok")
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("err = %v", err)
	}
	if got := strings.Join(server.paths(), ","); got != "POST /v1internal:loadCodeAssist,POST /v1internal:retrieveUserQuotaSummary" {
		t.Fatalf("calls = %s", got)
	}
}

func TestResolveTier(t *testing.T) {
	cases := []struct {
		paid, current string
		want          TierKind
	}{
		{"g1-ultra-tier", "free-tier", TierUltra},
		{"g1-pro-tier", "free-tier", TierPro},
		{"", "standard-tier", TierFree},
		{"", "free-tier", TierFree},
		{"", "", TierUnknown},
	}
	for _, c := range cases {
		if got := ResolveTier(c.paid, "", c.current, "").Kind; got != c.want {
			t.Errorf("%q/%q = %s", c.paid, c.current, got)
		}
	}
}

func TestQuotaSnapshotRoundTrip(t *testing.T) {
	f := 0.5
	in := QuotaSnapshot{Tier: Tier{Kind: TierPro, ID: "g1-pro-tier"}, Buckets: []QuotaBucket{{ID: "gemini:5h", Family: FamilyGemini, RemainingFraction: 0.5}},
		Models: []ModelQuota{{ID: "m", RemainingFraction: &f}}, UpdatedAt: testNow}
	raw, err := MarshalQuotaSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ParseQuotaSnapshot(raw)
	if err != nil || out.Tier != in.Tier || len(out.Buckets) != 1 || *out.Models[0].RemainingFraction != 0.5 || !out.UpdatedAt.Equal(testNow) {
		t.Fatalf("round trip: %+v %v", out, err)
	}
	if _, err := ParseQuotaSnapshot([]byte(`[1]`)); !errors.Is(err, ErrQuotaSnapshotJSON) {
		t.Fatal("non-object snapshot")
	}
	marked, _ := withValidation(raw, Validation{Reason: "VALIDATION_REQUIRED", URL: "https://x"}, testNow)
	if s, _ := ParseQuotaSnapshot(marked); s.Validation == nil || len(s.Buckets) != 1 {
		t.Fatal("validation keeps the previous reading")
	}
}
