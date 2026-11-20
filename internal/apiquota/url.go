package apiquota

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// Errors returned by NormalizeBaseURL. They never contain the input.
var (
	ErrEmptyBaseURL   = errors.New("empty base URL")
	ErrInvalidBaseURL = errors.New("invalid base URL")
)

// URLs is the normalized form of a user-entered base URL.
type URLs struct {
	// OpenAIBase ends with /v1; used for provider base_url and billing paths.
	OpenAIBase string
	// AnthropicBase is the base without a trailing /v1 segment: Claude Code
	// appends /v1/messages itself, so a base ending in /v1 would request
	// /v1/v1/messages.
	AnthropicBase string
	// ModelsURL is the model list endpoint.
	ModelsURL string
}

// NormalizeBaseURL turns user input into the URLs used by Probe and
// FetchModels. Only http and https are accepted. A query, a fragment, trailing
// slashes and a /messages or /v1/messages suffix are removed; the Anthropic
// form also drops a trailing /v1.
func NormalizeBaseURL(raw string) (URLs, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return URLs{}, ErrEmptyBaseURL
	}
	if strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return URLs{}, ErrInvalidBaseURL
	}
	if i := strings.IndexAny(s, "#?"); i >= 0 {
		s = s[:i]
	}
	if !validBaseURL(s) {
		return URLs{}, ErrInvalidBaseURL
	}
	base := strings.TrimRight(s, "/")
	switch lower := asciiLower(base); {
	case strings.HasSuffix(lower, "/v1/messages"):
		base = base[:len(base)-len("/v1/messages")] + "/v1"
	case strings.HasSuffix(lower, "/messages"):
		base = base[:len(base)-len("/messages")]
	}
	base = strings.TrimRight(base, "/")

	if strings.HasSuffix(asciiLower(base), "/v1") {
		root := strings.TrimRight(base[:len(base)-len("/v1")], "/")
		return URLs{OpenAIBase: base, AnthropicBase: root, ModelsURL: base + "/models"}, nil
	}
	return URLs{OpenAIBase: base + "/v1", AnthropicBase: base, ModelsURL: base + "/v1/models"}, nil
}

// validBaseURL requires an http(s) URL with a host and no embedded
// credentials, so the key can never end up in a probed URL through userinfo.
func validBaseURL(s string) bool {
	lower := asciiLower(s)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Hostname() != "" && u.User == nil
}

// asciiLower lowercases A-Z only, so byte offsets stay valid for suffix cuts.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// originWithoutV1 cuts a trailing /v1 from an http(s) base URL.
func originWithoutV1(openAIBase string) (string, bool) {
	trimmed := strings.TrimRight(openAIBase, "/")
	lower := asciiLower(trimmed)
	if !strings.HasSuffix(lower, "/v1") {
		return "", false
	}
	origin := strings.TrimRight(trimmed[:len(trimmed)-len("/v1")], "/")
	ol := asciiLower(origin)
	if ol == "http://" || ol == "https://" || len(origin) < 8 {
		return "", false
	}
	if !strings.HasPrefix(ol, "http://") && !strings.HasPrefix(ol, "https://") {
		return "", false
	}
	return origin, true
}

// siteOrigin returns scheme://host[:port] of an http(s) URL, dropping a
// default port.
func siteOrigin(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if port == "80" && u.Scheme == "http" || port == "443" && u.Scheme == "https" {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return u.Scheme + "://" + host, true
}

// publicAPIBases lists the roots probed for the unauthenticated status and the
// token usage endpoints: the path-mounted root first, then the site origin.
func publicAPIBases(openAIBase string) []string {
	var bases []string
	if b, ok := originWithoutV1(openAIBase); ok {
		bases = append(bases, b)
	}
	if b, ok := siteOrigin(openAIBase); ok && (len(bases) == 0 || bases[0] != b) {
		bases = append(bases, b)
	}
	return bases
}

type billingURLs struct {
	creditGrants string
	subscription string
	usage        string
}

// usageDateRange is the UTC month start through tomorrow, as dashboard usage
// queries expect.
func usageDateRange(now time.Time) (start, end string) {
	t := now.UTC()
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.Format(time.DateOnly), t.AddDate(0, 0, 1).Format(time.DateOnly)
}

func newBillingURLs(apiRoot string, now time.Time) billingURLs {
	root := strings.TrimRight(apiRoot, "/")
	start, end := usageDateRange(now)
	return billingURLs{
		creditGrants: root + "/dashboard/billing/credit_grants",
		subscription: root + "/dashboard/billing/subscription",
		usage:        root + "/dashboard/billing/usage?start_date=" + start + "&end_date=" + end,
	}
}

func tokenUsageURL(origin string) string { return strings.TrimRight(origin, "/") + "/api/usage/token" }

func quotaStatusURL(origin string) string { return strings.TrimRight(origin, "/") + "/api/status" }

// deepSeekBalanceURL is the official DeepSeek balance endpoint when the base
// URL points at api.deepseek.com. DeepSeek serves that endpoint from the site
// origin, not from /v1, so both its OpenAI-compatible base and its Anthropic
// path map to the same URL.
func deepSeekBalanceURL(openAIBase string) (string, bool) {
	origin, ok := siteOrigin(openAIBase)
	if !ok {
		return "", false
	}
	u, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(u.Hostname(), "api.deepseek.com") {
		return "", false
	}
	return origin + "/user/balance", true
}
