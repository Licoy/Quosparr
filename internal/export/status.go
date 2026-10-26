package export

import (
	"io/fs"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/sessions"
)

// StatusFileName is the status file inside ~/.quosparr.
const StatusFileName = "status.json"

// DefaultStatusPath is ~/.quosparr/status.json under home.
func DefaultStatusPath(home string) string {
	return filepath.Join(platform.RunwayDir(home), StatusFileName)
}

// Window is a quota window as far as the status file goes.
type Window struct {
	UsedPercent int
	ResetsAt    *time.Time
}

// NamedWindow is an additional quota window (for example model specific).
type NamedWindow struct {
	Name string
	Window
}

// QuotaInput is the quota snapshot the status file is built from.
type QuotaInput struct {
	Plan           *string
	Primary        Window
	Secondary      *Window
	Additional     []NamedWindow
	CreditsBalance *float64
	UpdatedAt      time.Time
}

// StatusWindow is one window of the status file.
type StatusWindow struct {
	Name        string     `json:"name"`
	UsedPercent int        `json:"usedPercent"`
	ResetsAt    *time.Time `json:"resetsAt,omitempty"`
}

// StatusQuota is the quota block of the status file. It carries no account or
// token data.
type StatusQuota struct {
	Plan              *string        `json:"plan,omitempty"`
	Primary           StatusWindow   `json:"primary"`
	Secondary         *StatusWindow  `json:"secondary,omitempty"`
	AdditionalWindows []StatusWindow `json:"additionalWindows"`
	CreditsBalance    *float64       `json:"creditsBalance,omitempty"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

// NewStatusQuota names the windows "5-hour" and "Weekly" as the Swift app does.
func NewStatusQuota(q QuotaInput) StatusQuota {
	out := StatusQuota{
		Plan:              q.Plan,
		Primary:           StatusWindow{"5-hour", q.Primary.UsedPercent, q.Primary.ResetsAt},
		AdditionalWindows: make([]StatusWindow, 0, len(q.Additional)),
		CreditsBalance:    q.CreditsBalance,
		UpdatedAt:         q.UpdatedAt,
	}
	if q.Secondary != nil {
		out.Secondary = &StatusWindow{"Weekly", q.Secondary.UsedPercent, q.Secondary.ResetsAt}
	}
	for _, w := range q.Additional {
		out.AdditionalWindows = append(out.AdditionalWindows, StatusWindow{w.Name, w.UsedPercent, w.ResetsAt})
	}
	return out
}

// Interval is a Foundation DateInterval: a start and a duration in seconds.
type Interval struct {
	Start    time.Time `json:"start"`
	Duration float64   `json:"duration"`
}

// CostDailyRow is one day of the API-equivalent cost.
type CostDailyRow struct {
	Date            string             `json:"date"`
	Totals          sessions.Totals    `json:"totals"`
	EstimatedUSD    *swiftjson.Decimal `json:"estimatedUSD,omitempty"`
	UnpricedTokens  int64              `json:"unpricedTokens,omitempty,omitzero"`
	RawCredits      float64            `json:"rawCredits"`
	CreditsReported *bool              `json:"creditsReported,omitempty"`
	TotalsReported  *bool              `json:"totalsReported,omitempty"`
}

// CostBreakdownRow is a model, project or client line of the cost.
type CostBreakdownRow struct {
	Name           string             `json:"name"`
	Totals         sessions.Totals    `json:"totals"`
	EstimatedUSD   *swiftjson.Decimal `json:"estimatedUSD,omitempty"`
	UnpricedTokens int64              `json:"unpricedTokens,omitempty,omitzero"`
	RawCredits     float64            `json:"rawCredits"`
}

// CostSummary mirrors the Swift ApiEquivalentSummary, the cost block of the
// status file. Source is localSessions, onlineAnalytics or unavailable;
// Confidence is priced, partiallyPriced, tokensOnly or unavailable.
type CostSummary struct {
	Source         string             `json:"source"`
	Confidence     string             `json:"confidence"`
	Window         Interval           `json:"window"`
	EstimatedUSD   *swiftjson.Decimal `json:"estimatedUSD,omitempty"`
	UnpricedTokens int64              `json:"unpricedTokens,omitempty,omitzero"`
	Totals         sessions.Totals    `json:"totals"`
	DailyRows      []CostDailyRow     `json:"dailyRows"`
	ModelRows      []CostBreakdownRow `json:"modelRows"`
	ProjectRows    []CostBreakdownRow `json:"projectRows"`
	ClientRows     []CostBreakdownRow `json:"clientRows"`
	RawCredits     float64            `json:"rawCredits"`
	Warnings       []string           `json:"warnings"`
	PricingVersion string             `json:"pricingVersion"`
	CalculatedAt   time.Time          `json:"calculatedAt"`
}

// StatusSnapshot is the content of status.json.
type StatusSnapshot struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	Quota       *StatusQuota      `json:"quota,omitempty"`
	Cost        *CostSummary      `json:"cost,omitempty"`
	Sessions    *sessions.Summary `json:"sessions,omitempty"`
}

// StatusExporter writes status.json.
type StatusExporter struct {
	Path string
}

// Encode is the status file content: pretty printed, keys sorted.
func (s StatusSnapshot) Encode() ([]byte, error) { return swiftjson.Marshal(s, true) }

// Save writes the file atomically with mode 0644 (it holds no secrets).
func (e StatusExporter) Save(s StatusSnapshot) error {
	data, err := s.Encode()
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(e.Path, data, fs.FileMode(0o644))
}
