package runway

import (
	"context"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/usage"
)

// DetailLine is one "title: value" row of a panel section.
type DetailLine struct {
	Title string
	Value string
}

// GrokAvailability is the state of the Grok panel (Swift GrokPanelAvailability).
type GrokAvailability int

// GrokAvailability values.
const (
	GrokLoading GrokAvailability = iota
	GrokReady
	GrokNotLoggedIn
	GrokReauthRequired
	GrokCLIUnavailable
	GrokCLITooOld
	GrokBillingParseFailed
	GrokFailed
)

// GrokState is what the coordinator needs to know about Grok to build the
// status bar and the widget snapshot. The Grok package owns the rest; Detail
// carries its panel payload untouched for the Grok UI to type-assert.
type GrokState struct {
	Availability GrokAvailability
	Accounts     AccountsView
	// AccountQuotas is the cached quota line of each managed account by id,
	// for the account list; accounts without a reading are absent.
	AccountQuotas map[string]GrokAccountQuota
	// Plan and UpdatedAt describe the quota reading; zero when none.
	Plan      string
	UpdatedAt time.Time
	// Meters are the quota bars, the overall included quota first.
	Meters     []Meter
	LastError  string
	Refreshing bool
	// Widget is the ready-made widget provider block, nil when Grok is unused.
	Widget         *export.ProviderSnapshot
	Detail         any
	Cost           *usage.Summary
	Heatmap        HeatmapData
	RecentSessions []sessions.Item
	// Speed is the model speed of the recent Grok sessions, nil until read;
	// SpeedError is the error of its last scan.
	Speed              *speed.Report
	SpeedError         string
	ResetCredits       *codexapi.ResetCreditsSnapshot
	ResetCreditSummary *ResetCreditSummary
}

// AccountResetGrant is a count whose availability has explicit time bounds.
// Only eligible, unpaused grants are projected; zero bounds are unrestricted.
type AccountResetGrant struct {
	Count            int
	StartsAt, EndsAt time.Time
}

// GrokAccountQuota contains the included quota, product contributions and reset cards.
// IncludedUSD is the "$used / $limit" allowance, or "" when unknown.
type GrokAccountQuota struct {
	Meters                []Meter
	Details               []DetailLine
	ResetGrants           []AccountResetGrant
	ResetCreditsKnown     bool
	ResetCreditsUpdatedAt time.Time
	Plan                  string
	Title                 string
	UsedPercent           int
	ResetsAt              time.Time
	IncludedUSD           string
	UpdatedAt             time.Time
}

// GrokProvider is the seam the Grok package plugs into. The Service never
// imports Grok code: an adapter in the application shell implements this.
//
// Locking: State is called while the Service holds its own lock, so it must
// be quick and must not call back into the Service. The onChange callback
// registered with SetOnChange must be invoked WITHOUT holding any lock that
// State takes, or the two will deadlock.
type GrokProvider interface {
	// State returns the current immutable state as of now.
	State(now time.Time) GrokState
	// Refresh refetches the Grok quota and accounts; it must honour ctx and
	// return when done.
	Refresh(ctx context.Context) error
	// RefreshLocal rescans the local sessions: cost, token chart, recent
	// sessions and speed. It never fetches the quota.
	RefreshLocal(ctx context.Context) error
	// SetOnChange registers the callback invoked after State changed.
	SetOnChange(fn func())
}

// ClaudeAvailability is the state of the Claude panel.
type ClaudeAvailability int

// ClaudeAvailability values.
const (
	ClaudeLoading ClaudeAvailability = iota
	ClaudeReady
	ClaudeNotLoggedIn
	ClaudeAPIKeyMode
	ClaudeTokenExpired
	ClaudeReauthRequired
	ClaudeRateLimited
	ClaudeCredentialStoreUnavailable
	ClaudeUsageParseFailed
	ClaudeFailed
)

// ClaudeState is the Claude view used by the status bar, widget and panel.
// ExtraUsage is the raw used/limit text.
type ClaudeState struct {
	Availability   ClaudeAvailability
	Accounts       AccountsView
	AccountQuotas  map[string]ClaudeAccountQuota
	Plan           string
	UpdatedAt      time.Time
	Meters         []Meter
	LastError      string
	Refreshing     bool
	Widget         *export.ProviderSnapshot
	Detail         any
	Cost           *usage.Summary
	Heatmap        HeatmapData
	RecentSessions []sessions.Item
	// Speed is the model speed of recent Claude sessions; SpeedError is the
	// error of its last scan.
	Speed      *speed.Report
	SpeedError string
	// ExtraUsage is the unconverted extra-usage line, "" when absent.
	ExtraUsage string
	// Gateway is set when settings.json names a gateway env key and a
	// subscription credential is also present.
	Gateway bool
	// API is set when the current account is an API relay account; Plan is
	// then "API" and Meters holds the balance meter when the relay reports one.
	API *ClaudeAPIView
	// ResetCredits are the reset grants as reset credits, nil when the
	// account takes no part in the reset grant program.
	ResetCredits       *codexapi.ResetCreditsSnapshot
	ResetCreditSummary *ResetCreditSummary
}

// ClaudeAPIView is what the panel shows of a current API relay account. It
// never holds the key.
type ClaudeAPIView struct {
	BaseURL string
	// Quota is the last balance reading, nil before the first one.
	Quota *apiquota.Quota
	// ErrorCode is the stored failure code of the last read ("" when none):
	// "api_unauthorized" when the relay rejected the key, else "api_failed".
	ErrorCode string
}

// ClaudeAccountQuota is the cached usage line of one managed account.
// ExtraUsage keeps the raw used / monthly limit text and currency.
type ClaudeAccountQuota struct {
	Meters                []Meter
	ResetGrants           []AccountResetGrant
	ResetCreditsUpdatedAt time.Time
	Plan                  string
	Title                 string
	UsedPercent           int
	ResetsAt              time.Time
	ExtraUsage            string
	UpdatedAt             time.Time
	// Balance is "remaining / total" (or the single known amount) of an API
	// relay account, "" for a subscription account or without a reading.
	Balance string
}

// ClaudeProvider is the Claude seam. Locking matches GrokProvider: State must
// be quick and must not call back into the Service, and the onChange callback
// must run without holding any lock State takes.
type ClaudeProvider interface {
	State(now time.Time) ClaudeState
	// Refresh refetches the Claude usage and accounts. force is a manual
	// refresh, which may read the usage sooner than the automatic spacing
	// allows.
	Refresh(ctx context.Context, force bool) error
	// RefreshLocal rescans the local sessions, like GrokProvider.RefreshLocal.
	RefreshLocal(ctx context.Context) error
	SetOnChange(fn func())
}

// ResetTodayState is the display state of the "rate limit reset today" card.
type ResetTodayState struct {
	// Text is the one-line summary, Lines the detail rows.
	Text  string
	Lines []DetailLine
	// Widget is the widget block, nil without a successful reading.
	Widget *export.ResetTodaySnapshot
	// HasData reports whether a reading exists.
	HasData bool
	// Detail carries the reset-today package payload for its UI.
	Detail any
}

// ResetTodayProvider is the seam of the reset-today feature. The locking
// rules of GrokProvider apply: State must not block or call into the Service,
// and onChange must be called without holding State's lock.
type ResetTodayProvider interface {
	State(now time.Time) ResetTodayState
	// Refresh fetches the status; force skips the provider's own throttling.
	Refresh(ctx context.Context, force bool) error
	// LastFetch is when Refresh last finished (success or failure), zero if never.
	LastFetch() time.Time
	// PendingAlerts returns the alerts that are due at now; the Service
	// dedupes them through the alert store.
	PendingAlerts(now time.Time) []alerts.Alert
	SetOnChange(fn func())
}
