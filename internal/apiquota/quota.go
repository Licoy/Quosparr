// Package apiquota queries the balance and the model list of third-party API
// relay sites from a base URL and an API key. It understands new-api / one-api
// style token endpoints, OpenAI-compatible billing dashboards and the official
// DeepSeek balance endpoint that is not part of the OpenAI-compatible API.
//
// The package is pure business logic: no UI, no localization, no file access
// and no global state. HTTP goes through a caller supplied Doer so proxy and
// TLS policy stay with the caller. The API key is only ever sent to the site it
// belongs to and is redacted from every error and result field.
package apiquota

import "time"

// Status is the outcome class of one quota probe.
type Status string

const (
	StatusAvailable    Status = "available"
	StatusUnsupported  Status = "unsupported"
	StatusUnauthorized Status = "unauthorized"
	StatusInvalidData  Status = "invalidData"
	StatusError        Status = "error"
)

// Source names the endpoint family a quota was read from.
type Source string

const (
	SourceTokenUsage        Source = "tokenUsage"
	SourceDeepSeekBalance   Source = "deepSeekBalance"
	SourceCreditGrants      Source = "creditGrants"
	SourceSubscriptionUsage Source = "subscriptionUsage"
	SourceSubscriptionOnly  Source = "subscriptionOnly"
	SourceUsageOnly         Source = "usageOnly"
)

// Units used by Quota.Unit. A site may also report a custom currency symbol.
const (
	UnitUSD      = "USD"
	UnitCNY      = "CNY"
	UnitTokens   = "TOKENS"
	UnitRawQuota = "RAW_QUOTA"
)

// Quota is one probe result. It is JSON-serializable with stable camelCase
// names and never contains the API key.
type Quota struct {
	Status    Status     `json:"status"`
	Remaining *float64   `json:"remaining,omitempty"`
	Used      *float64   `json:"used,omitempty"`
	Total     *float64   `json:"total,omitempty"`
	Unlimited bool       `json:"unlimited,omitempty"`
	Unit      string     `json:"unit,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Source    Source     `json:"source,omitempty"`
	// Endpoint is the URL that produced the result; it never contains the key.
	Endpoint  string    `json:"endpoint,omitempty"`
	FetchedAt time.Time `json:"fetchedAt"`
	// Error is a short sanitized message; it never holds the key or a response body.
	Error string `json:"error,omitempty"`
}

// UsedFraction returns used/total clamped to 0...1 when both are known, the
// quota is not unlimited and total is positive. ok is false otherwise.
func (q Quota) UsedFraction() (float64, bool) {
	if q.Unlimited || q.Used == nil || q.Total == nil || !(*q.Total > 0) {
		return 0, false
	}
	f := *q.Used / *q.Total
	if f != f {
		return 0, false
	}
	return min(max(f, 0), 1), true
}

func (s Source) rank() int {
	switch s {
	case SourceDeepSeekBalance:
		return 6
	case SourceTokenUsage:
		return 5
	case SourceCreditGrants:
		return 4
	case SourceSubscriptionUsage:
		return 3
	case SourceSubscriptionOnly:
		return 2
	case SourceUsageOnly:
		return 1
	}
	return 0
}

func (s Status) rank() int {
	switch s {
	case StatusInvalidData:
		return 4
	case StatusUnauthorized:
		return 3
	case StatusError:
		return 2
	case StatusUnsupported:
		return 1
	}
	return 0
}
