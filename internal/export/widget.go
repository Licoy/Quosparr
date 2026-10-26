package export

import (
	"math"
	"sort"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
)

// SchemaVersion is the widget snapshot schema this package reads and writes.
const SchemaVersion = 1

// StaleAfter is how old a snapshot may be before the widget calls it stale.
const StaleAfter = time.Hour

// QuotaSource separates the account-wide windows from model-specific ones.
type QuotaSource string

const (
	QuotaStandard      QuotaSource = "standard"
	QuotaModelSpecific QuotaSource = "modelSpecific"
)

// TokenSource says whether the token numbers cover all devices or this Mac.
type TokenSource string

const (
	TokensAllDevices TokenSource = "allDevices"
	TokensThisMac    TokenSource = "thisMac"
)

// Availability is the state of a provider's data.
type Availability string

const (
	Available      Availability = "available"
	NotLoggedIn    Availability = "notLoggedIn"
	CLIUnavailable Availability = "cliUnavailable"
	Unavailable    Availability = "unavailable"
)

// WidgetQuota is one quota row of a provider.
type WidgetQuota struct {
	Title            string      `json:"title"`
	WindowMinutes    *int        `json:"windowMinutes,omitempty"`
	Source           QuotaSource `json:"source"`
	UsedPercent      int         `json:"usedPercent"`
	RemainingPercent int         `json:"remainingPercent"`
	ResetsAt         *time.Time  `json:"resetsAt,omitempty"`
	// ValueText is the balance amount of an API relay row, shown instead of
	// the percentage.
	ValueText *string `json:"valueText,omitempty"`
}

func clamp(v, lo, hi int) int { return max(lo, min(hi, v)) }

// NewWidgetQuota clamps both percentages to 0..100.
func NewWidgetQuota(title string, windowMinutes *int, source QuotaSource, used, remaining int, resetsAt *time.Time) WidgetQuota {
	return WidgetQuota{title, windowMinutes, source, clamp(used, 0, 100), clamp(remaining, 0, 100), resetsAt, nil}
}

// DailyTokens is the token count of one day ("2026-08-06").
type DailyTokens struct {
	Date   string `json:"date"`
	Tokens int    `json:"tokens"`
}

// NewDailyTokens clamps negative counts to zero.
func NewDailyTokens(date string, tokens int) DailyTokens { return DailyTokens{date, max(0, tokens)} }

// ResetCredits counts a provider's reset credits.
type ResetCredits struct {
	AvailableCount int `json:"availableCount"`
	ExpiringCount  int `json:"expiringCount"`
}

// NewResetCredits clamps negative counts to zero.
func NewResetCredits(available, expiring int) ResetCredits {
	return ResetCredits{max(0, available), max(0, expiring)}
}

// ProviderSnapshot is everything a widget shows about one provider.
type ProviderSnapshot struct {
	Provider     Provider           `json:"provider"`
	Availability Availability       `json:"availability"`
	Plan         *string            `json:"plan,omitempty"`
	UpdatedAt    *time.Time         `json:"updatedAt,omitempty"`
	Quota        []WidgetQuota      `json:"quota"`
	BalanceUSD   *swiftjson.Decimal `json:"balanceUSD,omitempty"`
	// BalanceText is the preformatted API relay balance in its own unit;
	// the widget prefers it over BalanceUSD.
	BalanceText              *string            `json:"balanceText,omitempty"`
	APIEquivalentCostUSD     *swiftjson.Decimal `json:"apiEquivalentCostUSD,omitempty"`
	APIEquivalentCostPartial bool               `json:"apiEquivalentCostPartial,omitempty,omitzero"`
	TokenSource              TokenSource        `json:"tokenSource"`
	DailyTokens              []DailyTokens      `json:"dailyTokens"`
	ResetCredits             *ResetCredits      `json:"resetCredits,omitempty"`
}

// NewProviderSnapshot orders the quota rows (SortedQuota) and the daily tokens
// (by date), copying both slices.
func NewProviderSnapshot(p ProviderSnapshot) ProviderSnapshot {
	p.Quota = SortedQuota(p.Quota)
	days := append([]DailyTokens{}, p.DailyTokens...)
	sort.SliceStable(days, func(i, j int) bool { return days[i].Date < days[j].Date })
	p.DailyTokens = days
	return p
}

// SortedQuota puts standard windows before model-specific ones and orders by
// window length, a missing length last.
func SortedQuota(quota []WidgetQuota) []WidgetQuota {
	out := append([]WidgetQuota{}, quota...)
	length := func(q WidgetQuota) int {
		if q.WindowMinutes == nil {
			return math.MaxInt
		}
		return *q.WindowMinutes
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Source == QuotaStandard && b.Source == QuotaModelSpecific:
			return true
		case a.Source == QuotaModelSpecific && b.Source == QuotaStandard:
			return false
		}
		return length(a) < length(b)
	})
	return out
}

// VerdictReason is why the reset-today widget shows its answer.
type VerdictReason string

const (
	ReasonCompleted          VerdictReason = "completed"
	ReasonUpcoming           VerdictReason = "upcoming"
	ReasonGrace              VerdictReason = "grace"
	ReasonExpiredUnconfirmed VerdictReason = "expiredUnconfirmed"
	ReasonNone               VerdictReason = "none"
	ReasonUnavailable        VerdictReason = "unavailable"
)

// ResetState is the widget's yes / no / unknown answer.
type ResetState string

const (
	ResetYes     ResetState = "yes"
	ResetNo      ResetState = "no"
	ResetUnknown ResetState = "unknown"
)

// ConfidenceBand is how sure the verdict is.
type ConfidenceBand string

const (
	BandOK   ConfidenceBand = "ok"
	BandWarn ConfidenceBand = "warn"
)

// ResetTodayEntry is the precomputed presentation from EffectiveAt on.
type ResetTodayEntry struct {
	EffectiveAt            time.Time             `json:"effectiveAt"`
	Reason                 VerdictReason         `json:"reason"`
	State                  ResetState            `json:"state"`
	ResetType              *alerts.ResetType     `json:"resetType,omitempty"`
	NextScheduledAt        *time.Time            `json:"nextScheduledAt,omitempty"`
	NextScheduledResetType *alerts.ResetType     `json:"nextScheduledResetType,omitempty"`
	ScheduleBasis          *alerts.ScheduleBasis `json:"scheduleBasis,omitempty"`
	ConfidencePercent      *int                  `json:"confidencePercent,omitempty"`
	ConfidenceBand         *ConfidenceBand       `json:"confidenceBand,omitempty"`
}

// ResetTodaySnapshot is the widget's copy of the reset-today verdict. Timeline
// is nil on legacy snapshots, which makes the widget ask the app to refresh;
// a nil Timeline is omitted from the JSON, an empty one is written as [].
type ResetTodaySnapshot struct {
	State                  ResetState        `json:"state"`
	ResetType              *alerts.ResetType `json:"resetType,omitempty"`
	NextScheduledAt        *time.Time        `json:"nextScheduledAt,omitempty"`
	NextScheduledResetType *alerts.ResetType `json:"nextScheduledResetType,omitempty"`
	LastSuccessfulCheckAt  *time.Time        `json:"lastSuccessfulCheckAt,omitempty"`
	FetchedAt              time.Time         `json:"fetchedAt"`
	ConfidencePercent      *int              `json:"confidencePercent,omitempty"`
	ConfidenceBand         *ConfidenceBand   `json:"confidenceBand,omitempty"`
	Timeline               []ResetTodayEntry `json:"timeline,omitnil"`
}

// Presentation is the entry in force at date: the last one that took effect,
// else the first. nil when there is no timeline.
func (s ResetTodaySnapshot) Presentation(date time.Time) *ResetTodayEntry {
	if len(s.Timeline) == 0 {
		return nil
	}
	for i := len(s.Timeline) - 1; i >= 0; i-- {
		if !s.Timeline[i].EffectiveAt.After(date) {
			return &s.Timeline[i]
		}
	}
	return &s.Timeline[0]
}

// TransitionDates are the entry times after date.
func (s ResetTodaySnapshot) TransitionDates(date time.Time) []time.Time {
	var out []time.Time
	for _, e := range s.Timeline {
		if e.EffectiveAt.After(date) {
			out = append(out, e.EffectiveAt)
		}
	}
	return out
}

// Snapshot is the content of widget-snapshot.json.
type Snapshot struct {
	SchemaVersion int                 `json:"schemaVersion"`
	GeneratedAt   time.Time           `json:"generatedAt"`
	Language      l10n.Language       `json:"language"`
	Providers     []ProviderSnapshot  `json:"providers"`
	ResetToday    *ResetTodaySnapshot `json:"resetToday,omitempty"`
}

// NewSnapshot builds a current-schema snapshot with providers ordered by name.
func NewSnapshot(generatedAt time.Time, language l10n.Language, providers []ProviderSnapshot, resetToday *ResetTodaySnapshot) Snapshot {
	list := append([]ProviderSnapshot{}, providers...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Provider < list[j].Provider })
	return Snapshot{SchemaVersion, generatedAt, language, list, resetToday}
}

// IsStale reports a snapshot older than StaleAfter.
func (s Snapshot) IsStale(now time.Time) bool { return now.Sub(s.GeneratedAt) > StaleAfter }

// ProviderOf finds a provider's snapshot.
func (s Snapshot) ProviderOf(p Provider) *ProviderSnapshot {
	for i := range s.Providers {
		if s.Providers[i].Provider == p {
			return &s.Providers[i]
		}
	}
	return nil
}

// Encode is the file content: compact, keys sorted.
func (s Snapshot) Encode() ([]byte, error) { return swiftjson.Marshal(s, false) }
