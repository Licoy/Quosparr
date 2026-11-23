package apiquota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	tokenBody  = `{"code":true,"data":{"total_available":750000,"total_used":250000,"total_granted":1000000,"unlimited_quota":false}}`
	statusBody = `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`
	grantsBody = `{"total_granted":100,"total_used":25,"total_available":75}`
)

func TestProbeUsesPublicFallbackPathsAndCorrectAuthHeaders(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/api/usage/token": {200, tokenBody},
		"/api/status":      {200, statusBody},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/newapi/v1", "sk-test", testNow)

	if q.Status != StatusAvailable || q.Source != SourceTokenUsage || q.Unit != "USD" {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "remaining", q.Remaining, 1.5)
	wantFloat(t, "used", q.Used, 0.5)
	wantFloat(t, "total", q.Total, 2)
	if q.Endpoint != site.url()+"/api/usage/token" {
		t.Fatal(q.Endpoint)
	}
	if !q.FetchedAt.Equal(testNow) {
		t.Fatal(q.FetchedAt)
	}

	reqs := site.all()
	if len(reqs) != 7 {
		t.Fatalf("%d requests:\n%s", len(reqs), site.paths())
	}
	token, _ := site.find("/api/usage/token")
	if token.auth != "Bearer sk-test" || token.accept != "application/json" {
		t.Fatalf("%+v", token)
	}
	status, _ := site.find("/api/status")
	if status.auth != "" {
		t.Fatal("status request must be unauthenticated")
	}
	for _, p := range []string{"/newapi/api/usage/token", "/newapi/api/status",
		"/newapi/v1/dashboard/billing/credit_grants", "/newapi/v1/dashboard/billing/subscription",
		"/newapi/v1/dashboard/billing/usage"} {
		if _, ok := site.find(p); !ok {
			t.Fatalf("missing %s in\n%s", p, site.paths())
		}
	}
	usage, _ := site.find("/newapi/v1/dashboard/billing/usage")
	if usage.query != "start_date=2026-08-01&end_date=2026-08-22" || usage.auth != "Bearer sk-test" {
		t.Fatalf("%+v", usage)
	}
}

func TestProbePrefersPathMountedPublicBase(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/newapi/api/usage/token": {200, tokenBody},
		"/newapi/api/status":      {200, statusBody},
		"/api/usage/token":        {200, `{"success":false,"message":"must not be asked"}`},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/newapi/v1/", " sk-test\n", testNow)
	if q.Source != SourceTokenUsage || q.Endpoint != site.url()+"/newapi/api/usage/token" {
		t.Fatalf("%+v", q)
	}
	if _, ok := site.find("/api/usage/token"); ok {
		t.Fatal("root token endpoint asked after the mounted one succeeded")
	}
	if r, _ := site.find("/newapi/api/usage/token"); r.auth != "Bearer sk-test" {
		t.Fatalf("key not trimmed: %q", r.auth)
	}
}

func TestProbeSkipsRootTokenWhenRootStatusIsNotValid(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/api/status": {200, "<html>welcome</html>"},
	})
	Probe(context.Background(), site.client(), site.url()+"/newapi/v1", "sk-test", testNow)
	if _, ok := site.find("/api/usage/token"); ok {
		t.Fatalf("key sent to unrelated site root:\n%s", site.paths())
	}
}

func TestProbeFallsBackToOriginWithoutV1(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/dashboard/billing/credit_grants": {200, grantsBody},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Status != StatusAvailable || q.Source != SourceCreditGrants || q.Unit != "USD" {
		t.Fatalf("%+v", q)
	}
	if q.Endpoint != site.url()+"/dashboard/billing/credit_grants" {
		t.Fatal(q.Endpoint)
	}
	wantFloat(t, "remaining", q.Remaining, 75)
	// first round: 3 billing + status + token, second round: 3 billing.
	if n := len(site.all()); n != 8 {
		t.Fatalf("%d requests:\n%s", n, site.paths())
	}
}

func TestProbeSkipsFallbackAfterStrongSource(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/credit_grants": {200, grantsBody},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Source != SourceCreditGrants {
		t.Fatalf("%+v", q)
	}
	if n := len(site.all()); n != 5 {
		t.Fatalf("%d requests:\n%s", n, site.paths())
	}
}

func TestProbeFallbackImprovesWeakSource(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/subscription": {200, `{"hard_limit_usd":100}`},
		"/dashboard/billing/credit_grants":   {200, grantsBody},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Source != SourceCreditGrants {
		t.Fatalf("%+v", q)
	}
}

func TestProbeKeepsWeakSourceWhenFallbackFinds404(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/subscription": {200, `{"hard_limit_usd":100,"access_until":1767225600}`},
		"/v1/dashboard/billing/usage":        {200, `{"total_usage":2500}`},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Source != SourceSubscriptionUsage {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "used", q.Used, 25)
	wantFloat(t, "remaining", q.Remaining, 75)
	if q.ExpiresAt == nil || q.ExpiresAt.Unix() != 1767225600 {
		t.Fatalf("%v", q.ExpiresAt)
	}
}

func TestProbeUnauthorized(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/credit_grants": {401, `{"error":"bad key sk-test"}`},
		"/api/usage/token":                    {200, `{"success":false,"message":"Invalid token"}`},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Status != StatusUnauthorized || q.Error != "" {
		t.Fatalf("%+v", q)
	}
}

func TestProbeUpstreamErrorIsShortAndSanitized(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/credit_grants": {502, "bad gateway for sk-test"},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Status != StatusError || q.Error != "HTTP 502" {
		t.Fatalf("%+v", q)
	}
}

func TestProbeHTMLSiteIsUnsupported(t *testing.T) {
	site := newFakeSite(t, nil)
	site.routes = map[string]reply{}
	for _, p := range []string{"/v1/dashboard/billing/credit_grants", "/v1/dashboard/billing/subscription",
		"/v1/dashboard/billing/usage", "/api/status", "/api/usage/token"} {
		site.routes[p] = reply{200, "<!doctype html><html></html>"}
	}
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Status != StatusUnsupported || q.Error != "" {
		t.Fatalf("%+v", q)
	}
}

func TestProbeEmptyKeyIsUnsupportedWithoutRequests(t *testing.T) {
	site := newFakeSite(t, nil)
	for _, key := range []string{"", "  \n"} {
		q := Probe(context.Background(), site.client(), site.url(), key, testNow)
		if q.Status != StatusUnsupported || q.Source != "" || !q.FetchedAt.Equal(testNow) {
			t.Fatalf("%+v", q)
		}
	}
	if n := len(site.all()); n != 0 {
		t.Fatalf("%d requests", n)
	}
}

func TestProbeInvalidInputs(t *testing.T) {
	if q := Probe(context.Background(), nil, "https://a.example.com", "sk-test", testNow); q.Status != StatusError {
		t.Fatalf("%+v", q)
	}
	site := newFakeSite(t, nil)
	q := Probe(context.Background(), site.client(), "ftp://a.example.com", "sk-test", testNow)
	if q.Status != StatusError || q.Error != "invalid base URL" {
		t.Fatalf("%+v", q)
	}
	if q := Probe(context.Background(), site.client(), "", "sk-test", testNow); q.Status != StatusError || q.Error != "empty base URL" {
		t.Fatalf("%+v", q)
	}
	if n := len(site.all()); n != 0 {
		t.Fatalf("%d requests", n)
	}
}

func TestProbeZeroNowUsesClock(t *testing.T) {
	q := Probe(context.Background(), nil, "https://a.example.com", "sk-test", time.Time{})
	if time.Since(q.FetchedAt) > time.Minute {
		t.Fatal(q.FetchedAt)
	}
}

func TestProbeBodyIsCapped(t *testing.T) {
	// Valid JSON that starts after the 64 KiB cap is never read.
	late := strings.Repeat(" ", maxBodyBytes+10) + grantsBody
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/credit_grants": {200, late},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if q.Status != StatusUnsupported {
		t.Fatalf("%+v", q)
	}
	// Valid JSON that ends inside the cap is read.
	early := grantsBody + strings.Repeat(" ", 2*maxBodyBytes)
	site = newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/credit_grants": {200, early},
	})
	if q := Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow); q.Source != SourceCreditGrants {
		t.Fatalf("%+v", q)
	}
}

func TestProbeTimeoutAndCancel(t *testing.T) {
	blocked := doerFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: r.Context().Err()}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	q := Probe(ctx, blocked, "https://a.example.com/v1", "sk-test", testNow)
	if q.Status != StatusError || q.Error != "request timed out" {
		t.Fatalf("%+v", q)
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	q = Probe(ctx, blocked, "https://a.example.com/v1", "sk-test", testNow)
	if q.Status != StatusError || q.Error != "request canceled" {
		t.Fatalf("%+v", q)
	}
}

func TestProbeNetworkTimeoutError(t *testing.T) {
	slow := doerFunc(func(r *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: timeoutErr{}}
	})
	q := Probe(context.Background(), slow, "https://a.example.com/v1", "sk-test", testNow)
	if q.Status != StatusError || q.Error != "request timed out" {
		t.Fatalf("%+v", q)
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestProbeTransportErrorIsRedactedAndShort(t *testing.T) {
	const key = "sk-live-0123456789abcdef"
	failing := doerFunc(func(r *http.Request) (*http.Response, error) {
		msg := fmt.Sprintf("proxy rejected credentials %s\n%s", key, strings.Repeat("x", 500))
		return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: errors.New(msg)}
	})
	q := Probe(context.Background(), failing, "https://a.example.com/v1", key, testNow)
	if q.Status != StatusError {
		t.Fatalf("%+v", q)
	}
	if strings.Contains(q.Error, key) || !strings.Contains(q.Error, "<redacted>") {
		t.Fatalf("not redacted: %q", q.Error)
	}
	if strings.Contains(q.Error, "https://") || len([]rune(q.Error)) > maxErrorRune+3 {
		t.Fatalf("not short: %q", q.Error)
	}
}

// TestProbeNeverLeaksKey runs hostile and echoing sites and checks that the
// marshalled result (the form cached in account files) never contains the key.
// The short key fits the unit length cap, so it exercises the unit redaction.
func TestProbeNeverLeaksKey(t *testing.T) {
	for _, key := range []string{"sk-Secret0123456789abcdefXYZ", "sk-Short1234"} {
		for name, routes := range leakRoutes(key) {
			t.Run(name+"/"+key, func(t *testing.T) {
				site := newFakeSite(t, routes)
				for _, base := range []string{site.url() + "/v1", site.url() + "/" + key + "/v1"} {
					q := Probe(context.Background(), site.client(), base, key, testNow)
					out := strings.ToLower(marshal(t, q))
					if strings.Contains(out, strings.ToLower(key)) {
						t.Fatalf("key leaked for %s: %s", base, out)
					}
				}
			})
		}
	}
}

func leakRoutes(key string) map[string]map[string]reply {
	echo := `"` + key + `"`
	return map[string]map[string]reply{
		"error body echo": {
			"/v1/dashboard/billing/credit_grants": {500, "internal error for " + key},
			"/v1/dashboard/billing/subscription":  {401, `{"error":` + echo + `}`},
			"/api/usage/token":                    {200, `{"success":false,"message":` + echo + `}`},
		},
		"upstream failure echo": {
			"/v1/dashboard/billing/credit_grants": {200, `{"error":` + echo + `}`},
		},
		"unit echo": {
			"/api/usage/token": {200, `{"code":true,"data":{"display":{"remaining":1,"used":1,"total":2,"unit":` + echo + `}}}`},
		},
		"custom symbol echo": {
			"/api/usage/token": {200, `{"code":true,"data":{"total_available":1,"total_used":1,"total_granted":2}}`},
			"/api/status":      {200, `{"success":true,"data":{"quota_per_unit":1,"quota_display_type":"CUSTOM","custom_currency_symbol":` + echo + `,"custom_currency_exchange_rate":1}}`},
		},
		"invalid data echo": {
			"/api/usage/token": {200, `{"code":true,"data":{"total_available":"` + key + `"}}`},
		},
	}
}

func TestProbeKeyInBasePathIsRedactedInEndpoint(t *testing.T) {
	const key = "sk-pathkey-12345"
	site := newFakeSite(t, map[string]reply{
		"/" + key + "/v1/dashboard/billing/credit_grants": {200, grantsBody},
	})
	q := Probe(context.Background(), site.client(), site.url()+"/"+key+"/v1", key, testNow)
	if q.Source != SourceCreditGrants || strings.Contains(q.Endpoint, key) || !strings.Contains(q.Endpoint, "<redacted>") {
		t.Fatalf("%+v", q)
	}
}

func TestProbeIsRaceFreeAcrossCalls(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/api/usage/token": {200, tokenBody},
		"/api/status":      {200, statusBody},
	})
	done := make(chan Quota, 8)
	for i := 0; i < cap(done); i++ {
		go func() { done <- Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow) }()
	}
	for i := 0; i < cap(done); i++ {
		if q := <-done; q.Source != SourceTokenUsage {
			t.Fatalf("%+v", q)
		}
	}
}

const deepSeekBalanceBody = `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"72.50","granted_balance":"2.50","topped_up_balance":"70.00"}]}`

func TestProbeDeepSeekBalanceUsesTheOfficialEndpoint(t *testing.T) {
	for _, base := range []string{
		"https://api.deepseek.com",
		"https://api.deepseek.com/v1",
		"https://api.deepseek.com/anthropic/v1",
	} {
		t.Run(base, func(t *testing.T) {
			var requests []*http.Request
			client := doerFunc(func(r *http.Request) (*http.Response, error) {
				requests = append(requests, r)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(deepSeekBalanceBody))}, nil
			})
			q := Probe(context.Background(), client, base, " sk-deep\n", testNow)
			if q.Status != StatusAvailable || q.Source != SourceDeepSeekBalance || q.Unit != UnitCNY {
				t.Fatalf("%+v", q)
			}
			wantFloat(t, "remaining", q.Remaining, 72.5)
			wantNil(t, "used", q.Used)
			wantNil(t, "total", q.Total)
			if q.Endpoint != "https://api.deepseek.com/user/balance" || !q.FetchedAt.Equal(testNow) {
				t.Fatalf("endpoint %q at %v", q.Endpoint, q.FetchedAt)
			}
			if len(requests) != 1 {
				t.Fatalf("%d requests", len(requests))
			}
			got := requests[0]
			if got.Method != http.MethodGet || got.URL.String() != "https://api.deepseek.com/user/balance" {
				t.Fatalf("%s %s", got.Method, got.URL)
			}
			if got.Header.Get("Authorization") != "Bearer sk-deep" || got.Header.Get("Accept") != "application/json" {
				t.Fatalf("headers: %v", got.Header)
			}
		})
	}
}

func TestProbeDeepSeekBalanceFailures(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus Status
		wantError  string
		wantURL    string
	}{
		{"unauthorized", 401, `{"error":"bad key"}`, StatusUnauthorized, "", ""},
		{"empty balances", 200, `{"is_available":false,"balance_infos":[]}`, StatusInvalidData, msgInvalidQuota, "https://api.deepseek.com/user/balance"},
		{"unrelated json", 200, `{"hello":"world"}`, StatusUnsupported, "", ""},
		{"server error", 502, "down", StatusError, "HTTP 502", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tt.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			q := Probe(context.Background(), client, "https://api.deepseek.com/v1", "sk-deep", testNow)
			if q.Status != tt.wantStatus || q.Error != tt.wantError || q.Endpoint != tt.wantURL {
				t.Fatalf("%+v", q)
			}
		})
	}
}

func TestProbeDoesNotAskOtherSitesForTheDeepSeekEndpoint(t *testing.T) {
	site := newFakeSite(t, map[string]reply{
		"/v1/dashboard/billing/credit_grants": {200, grantsBody},
	})
	Probe(context.Background(), site.client(), site.url()+"/v1", "sk-test", testNow)
	if _, ok := site.find("/user/balance"); ok {
		t.Fatalf("asked an unrelated site:\n%s", site.paths())
	}
}
