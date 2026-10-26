package usage

import (
	"time"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

// Source says where a Summary came from.
type Source string

// Summary sources.
const (
	SourceLocalSessions   Source = "localSessions"
	SourceOnlineAnalytics Source = "onlineAnalytics"
	SourceUnavailable     Source = "unavailable"
)

// Confidence says how much of a Summary is priced.
type Confidence string

// Summary confidence levels.
const (
	ConfidencePriced          Confidence = "priced"
	ConfidencePartiallyPriced Confidence = "partiallyPriced"
	ConfidenceTokensOnly      Confidence = "tokensOnly"
	ConfidenceUnavailable     Confidence = "unavailable"
)

// TokenUsage is a plain token triple. InputTokens includes the cached part.
type TokenUsage struct {
	InputTokens       int64
	CachedInputTokens int64
	OutputTokens      int64
}

// Totals are the counters of one row of a Summary. UncachedInputTokens and
// CachedInputTokens add up to the input side; TotalTokens includes output.
type Totals struct {
	TotalTokens         int64 `json:"totalTokens"`
	UncachedInputTokens int64 `json:"uncachedInputTokens"`
	CachedInputTokens   int64 `json:"cachedInputTokens"`
	OutputTokens        int64 `json:"outputTokens"`
	Turns               int64 `json:"turns"`
	Threads             int64 `json:"threads"`
}

// HasTokenParts reports whether any of the priced counters is non-zero.
func (t Totals) HasTokenParts() bool {
	return t.UncachedInputTokens > 0 || t.CachedInputTokens > 0 || t.OutputTokens > 0
}

// DailyRow is one calendar day of a Summary. EstimatedUSD is nil when the day
// has no priced usage.
type DailyRow struct {
	Date           string
	Totals         Totals
	EstimatedUSD   *decimal.Decimal
	UnpricedTokens int64
	RawCredits     float64
	// CreditsReported and TotalsReported are nil for local or legacy data
	// whose reporting status is unknown.
	CreditsReported *bool
	TotalsReported  *bool
}

// BreakdownRow is one model, project or client of a Summary.
type BreakdownRow struct {
	Name           string
	Totals         Totals
	EstimatedUSD   *decimal.Decimal
	UnpricedTokens int64
	RawCredits     float64
}

// Window is a closed time interval [Start, End].
type Window struct {
	Start time.Time
	End   time.Time
}

// Duration returns End - Start.
func (w Window) Duration() time.Duration { return w.End.Sub(w.Start) }

// Contains reports whether the Unix-seconds timestamp ts lies inside the
// window, bounds included, comparing as floating point seconds the way the
// index query does.
func (w Window) Contains(ts float64) bool {
	return ts >= unixSeconds(w.Start) && ts <= unixSeconds(w.End)
}

// Summary is the API-equivalent cost of a window. A Summary returned by this
// package is immutable by convention: callers must not modify its slices.
type Summary struct {
	Source         Source
	Confidence     Confidence
	Window         Window
	EstimatedUSD   *decimal.Decimal
	UnpricedTokens int64
	Totals         Totals
	DailyRows      []DailyRow
	ModelRows      []BreakdownRow
	ProjectRows    []BreakdownRow
	ClientRows     []BreakdownRow
	RawCredits     float64
	Warnings       []string
	PricingVersion string
	CalculatedAt   time.Time
}

// IsDisplayableCost reports whether the summary has anything worth showing.
func (s Summary) IsDisplayableCost() bool {
	return s.Confidence != ConfidenceUnavailable && s.Totals.TotalTokens > 0
}

// Clone returns a deep copy of s.
func (s Summary) Clone() Summary {
	c := s
	c.EstimatedUSD = cloneDecimal(s.EstimatedUSD)
	c.DailyRows = make([]DailyRow, len(s.DailyRows))
	for i, r := range s.DailyRows {
		r.EstimatedUSD = cloneDecimal(r.EstimatedUSD)
		r.CreditsReported = cloneBool(r.CreditsReported)
		r.TotalsReported = cloneBool(r.TotalsReported)
		c.DailyRows[i] = r
	}
	c.ModelRows = cloneRows(s.ModelRows)
	c.ProjectRows = cloneRows(s.ProjectRows)
	c.ClientRows = cloneRows(s.ClientRows)
	c.Warnings = append([]string{}, s.Warnings...)
	return c
}

func cloneRows(rows []BreakdownRow) []BreakdownRow {
	out := make([]BreakdownRow, len(rows))
	for i, r := range rows {
		r.EstimatedUSD = cloneDecimal(r.EstimatedUSD)
		out[i] = r
	}
	return out
}

func cloneDecimal(d *decimal.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v := *d
	return &v
}

func cloneBool(b *bool) *bool {
	if b == nil {
		return nil
	}
	v := *b
	return &v
}

// UnavailableSummary is the empty summary for a window with no data.
func UnavailableSummary(w Window, warning string, calculatedAt time.Time) Summary {
	s := Summary{
		Source:         SourceUnavailable,
		Confidence:     ConfidenceUnavailable,
		Window:         w,
		DailyRows:      []DailyRow{},
		ModelRows:      []BreakdownRow{},
		ProjectRows:    []BreakdownRow{},
		ClientRows:     []BreakdownRow{},
		Warnings:       []string{},
		PricingVersion: PricingVersion,
		CalculatedAt:   calculatedAt,
	}
	if warning != "" {
		s.Warnings = []string{warning}
	}
	return s
}

// ModelCostBreakdown is one model of a UsageCostSummary.
type ModelCostBreakdown struct {
	Model        string
	Usage        TokenUsage
	EstimatedUSD decimal.Decimal
}

// CostSummary is the result of the index-free token scan (Scanner.Scan).
type CostSummary struct {
	Window         Window
	Totals         TokenUsage
	ModelBreakdown []ModelCostBreakdown
	EstimatedUSD   decimal.Decimal
	PricingVersion string
	UnknownModels  []string
}

// unixSeconds converts t to Unix seconds as float64 the way Foundation's
// Date.timeIntervalSince1970 does: through the 2001 reference date.
func unixSeconds(t time.Time) float64 {
	x := float64(t.Unix()) + float64(t.Nanosecond())/1e9
	return swiftDate(x)
}

const referenceOffset = 978307200.0

// swiftDate reproduces the Date(timeIntervalSince1970:) /
// timeIntervalSince1970 round trip, which goes through seconds since 2001 and
// is the identity for timestamps after 2004.
func swiftDate(x float64) float64 {
	ref := float64(x - referenceOffset)
	return float64(ref + referenceOffset)
}

// timeFromUnixSeconds converts float seconds back to a time.Time.
func timeFromUnixSeconds(x float64) time.Time {
	sec := int64(x)
	frac := x - float64(sec)
	return time.Unix(sec, int64(frac*1e9)).UTC()
}
