package apiquota

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name, in, openAI, anthropic, models string
	}{
		{"bare host", "https://api.example.com",
			"https://api.example.com/v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"with v1", "https://api.example.com/v1",
			"https://api.example.com/v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"strip v1 messages", "https://relay.example.com/v1/messages",
			"https://relay.example.com/v1", "https://relay.example.com", "https://relay.example.com/v1/models"},
		{"strip messages", "https://relay.example.com/anthropic/messages",
			"https://relay.example.com/anthropic/v1", "https://relay.example.com/anthropic", "https://relay.example.com/anthropic/v1/models"},
		{"anthropic path", "https://relay.example.com/anthropic",
			"https://relay.example.com/anthropic/v1", "https://relay.example.com/anthropic", "https://relay.example.com/anthropic/v1/models"},
		{"trailing slashes", "https://api.example.com/v1///",
			"https://api.example.com/v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"query and fragment", "https://api.example.com/v1/?key=1#top",
			"https://api.example.com/v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"fragment before query", "https://api.example.com#a?b",
			"https://api.example.com/v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"case insensitive messages suffix", "https://api.example.com/V1/Messages/",
			"https://api.example.com/v1", "https://api.example.com", "https://api.example.com/v1/models"},
		{"case insensitive v1", "https://api.example.com/V1",
			"https://api.example.com/V1", "https://api.example.com", "https://api.example.com/V1/models"},
		{"outer whitespace", "  https://127.0.0.1:8080/api \n",
			"https://127.0.0.1:8080/api/v1", "https://127.0.0.1:8080/api", "https://127.0.0.1:8080/api/v1/models"},
		{"path-mounted v1", "https://relay.example.com/claude/v1/",
			"https://relay.example.com/claude/v1", "https://relay.example.com/claude", "https://relay.example.com/claude/v1/models"},
		{"uppercase scheme", "HTTPS://api.example.com",
			"HTTPS://api.example.com/v1", "HTTPS://api.example.com", "HTTPS://api.example.com/v1/models"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			want := URLs{OpenAIBase: tt.openAI, AnthropicBase: tt.anthropic, ModelsURL: tt.models}
			if got != want {
				t.Fatalf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestNormalizeBaseURLRejects(t *testing.T) {
	for _, in := range []string{"", "   ", "api.example.com", "http://api.example.com", "http://127.0.0.1:8080", "ftp://api.example.com", "https://",
		"https:///v1", "https://exa mple.com", "https://a.com/x y", "https://user:pass@a.com", "https://a.com:port",
		"?https://a.com", "#https://a.com"} {
		if _, err := NormalizeBaseURL(in); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
	if _, err := NormalizeBaseURL(" "); !errors.Is(err, ErrEmptyBaseURL) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := NormalizeBaseURL("ftp://x"); !errors.Is(err, ErrInvalidBaseURL) {
		t.Fatalf("scheme: %v", err)
	}
}

func TestBillingURLsFromBareAndV1(t *testing.T) {
	for _, in := range []string{"https://api.example.com", "https://api.example.com/v1"} {
		urls, err := NormalizeBaseURL(in)
		if err != nil {
			t.Fatal(err)
		}
		b := newBillingURLs(urls.OpenAIBase, testNow)
		if b.creditGrants != "https://api.example.com/v1/dashboard/billing/credit_grants" ||
			b.subscription != "https://api.example.com/v1/dashboard/billing/subscription" ||
			b.usage != "https://api.example.com/v1/dashboard/billing/usage?start_date=2026-08-01&end_date=2026-08-22" {
			t.Fatalf("%s: %+v", in, b)
		}
	}
	if got := tokenUsageURL("https://api.example.com/"); got != "https://api.example.com/api/usage/token" {
		t.Fatal(got)
	}
	if got := quotaStatusURL("https://api.example.com"); got != "https://api.example.com/api/status" {
		t.Fatal(got)
	}
}

func TestOriginWithoutV1(t *testing.T) {
	tests := map[string]string{
		"https://api.example.com/v1":          "https://api.example.com",
		"https://api.example.com/V1/":         "https://api.example.com",
		"https://relay.example.com/openai/v1": "https://relay.example.com/openai",
		"https://api.example.com":             "",
		"https://v1":                          "",
		"v1":                                  "",
		"ftp://host/v1":                       "",
	}
	for in, want := range tests {
		got, ok := originWithoutV1(in)
		if got != want || ok != (want != "") {
			t.Fatalf("originWithoutV1(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestSiteOriginIgnoresOpenAIPath(t *testing.T) {
	tests := map[string]string{
		"https://relay.example.com/openai/v1":      "https://relay.example.com",
		"https://relay.example.com:8443/openai/v1": "https://relay.example.com:8443",
		"https://Relay.Example.com:443/v1":         "https://relay.example.com",
		"http://relay.example.com:80/v1":           "http://relay.example.com",
		"http://[::1]:8080/v1":                     "http://[::1]:8080",
		"ftp://relay.example.com/v1":               "",
	}
	for in, want := range tests {
		got, ok := siteOrigin(in)
		if got != want || ok != (want != "") {
			t.Fatalf("siteOrigin(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestPublicAPIBasesSupportPathMountedAndRootDeployments(t *testing.T) {
	got := publicAPIBases("https://relay.example.com/newapi/v1")
	if want := []string{"https://relay.example.com/newapi", "https://relay.example.com"}; !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
	got = publicAPIBases("https://relay.example.com/v1")
	if want := []string{"https://relay.example.com"}; !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestUsageDateRangeIsMonthStartToTomorrow(t *testing.T) {
	tests := []struct {
		now        time.Time
		start, end string
	}{
		{testNow, "2026-08-01", "2026-08-22"},
		{time.Date(2026, 1, 31, 23, 59, 0, 0, time.UTC), "2026-01-01", "2026-02-01"},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "2026-12-01", "2027-01-01"},
		// 2026-03-01 01:00 at +08:00 is still 2026-02-28 in UTC.
		{time.Date(2026, 3, 1, 1, 0, 0, 0, time.FixedZone("x", 8*3600)), "2026-02-01", "2026-03-01"},
	}
	for _, tt := range tests {
		start, end := usageDateRange(tt.now)
		if start != tt.start || end != tt.end {
			t.Fatalf("%v: got %s..%s, want %s..%s", tt.now, start, end, tt.start, tt.end)
		}
	}
}

func TestDeepSeekBalanceURL(t *testing.T) {
	tests := map[string]string{
		"https://api.deepseek.com":                 "https://api.deepseek.com/user/balance",
		"https://api.deepseek.com/v1":              "https://api.deepseek.com/user/balance",
		"https://api.deepseek.com/anthropic/v1":    "https://api.deepseek.com/user/balance",
		"https://api.deepseek.com:8443/v1/":        "https://api.deepseek.com:8443/user/balance",
		"https://API.DeepSeek.com/v1":              "https://api.deepseek.com/user/balance",
		"https://api.deepseek.com.evil.example/v1": "",
		"https://deepseek.com/v1":                  "",
		"https://relay.example.com/v1":             "",
		"ftp://api.deepseek.com/v1":                "",
		"api.deepseek.com":                         "",
	}
	for in, want := range tests {
		got, ok := deepSeekBalanceURL(in)
		if got != want || ok != (want != "") {
			t.Fatalf("deepSeekBalanceURL(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestNormalizeStoredBaseURLRecognizesLegacyHTTP(t *testing.T) {
	got, err := NormalizeStoredBaseURL("http://relay.example.test/v1/messages")
	if err != nil || got.AnthropicBase != "http://relay.example.test" {
		t.Fatalf("NormalizeStoredBaseURL() = %v, %v", got, err)
	}
}
