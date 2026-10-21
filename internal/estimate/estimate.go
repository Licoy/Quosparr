package estimate

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// WindowMode is how the cycle of the estimate is chosen.
type WindowMode string

const (
	// ModeAuto uses the official cycle of the weekly window, else the
	// secondary window, else the primary one.
	ModeAuto WindowMode = "auto"
	// ModeRollingWeek uses the seven UTC days ending at the latest daily row.
	ModeRollingWeek WindowMode = "rollingWeek"
)

// ParseWindowMode decodes a stored mode; anything unknown is ModeAuto, as in
// the Swift decoder.
func ParseWindowMode(raw string) WindowMode {
	if raw == string(ModeRollingWeek) {
		return ModeRollingWeek
	}
	return ModeAuto
}

const (
	// USDPerCredit is the unofficial ChatGPT Credit conversion (1000 Credits
	// are about $40). It is not an official price list.
	USDPerCredit = 0.04
	// PricingVersion labels the conversion above.
	PricingVersion = "credits-usd-2026-08-26"
	// SignificantChangePercent is the change against the previous cycle that
	// counts as a real increase or cut.
	SignificantChangePercent = 10.0
	// MaxHistorySamples is how many cycles per account are kept (twelve weeks).
	MaxHistorySamples = 12
	// AnalyticsLookbackDays is how far back the daily rows are requested.
	AnalyticsLookbackDays = 30
)

// ChangeKind compares this cycle's estimate with the previous one.
type ChangeKind string

const (
	ChangeDecreased ChangeKind = "decreased"
	ChangeIncreased ChangeKind = "increased"
	ChangeSimilar   ChangeKind = "similar"
)

// Window is the part of a quota window the estimate reads (the Swift
// RateWindow). UsedPercentExact is the fractional percentage; when the API only
// gave a whole number it equals UsedPercent.
type Window struct {
	UsedPercent      int
	UsedPercentExact float64
	WindowMinutes    *int
	ResetsAt         *time.Time
}

// NewWindow builds a Window whose exact percentage is the whole one.
func NewWindow(usedPercent int, windowMinutes *int, resetsAt *time.Time) Window {
	return Window{UsedPercent: usedPercent, UsedPercentExact: float64(usedPercent), WindowMinutes: windowMinutes, ResetsAt: resetsAt}
}

// Quota is the primary and optional secondary window of a quota snapshot.
type Quota struct {
	Primary   Window
	Secondary *Window
}

// DailyRow is one day of API-equivalent analytics (the Swift
// ApiEquivalentDailyRow, reduced to what the estimate uses).
type DailyRow struct {
	// Date is the UTC day, "2026-06-29".
	Date       string
	RawCredits float64
	// TotalTokens and Turns come from the day's totals.
	TotalTokens int
	Turns       int
	// EstimatedUSD is the priced API-equivalent cost, nil when not priceable.
	EstimatedUSD *float64
	// CreditsReported and TotalsReported say the server actually sent
	// the day's credits / totals; legacy cached rows have neither.
	CreditsReported bool
	TotalsReported  bool
}

// Row is a DailyRow as shown in the estimate.
type Row struct {
	Date            string
	Credits         float64
	Tokens          int
	Turns           int
	USD             *float64
	CreditsReported bool
	TotalsReported  bool
}

// USDForCredits converts Credits to the reference USD amount.
func USDForCredits(credits float64) float64 { return credits * USDPerCredit }

func (r Row) hasUsage() bool { return r.Tokens > 0 || r.Turns > 0 || r.Credits > 0 }

// Snapshot is the result of Make.
type Snapshot struct {
	WindowMode       WindowMode
	CycleStartDate   string
	UsedPercent      float64
	UsedCredits      float64
	UsedUSD          float64
	DayCount         int
	EstimatedCredits *float64
	EstimatedUSD     *float64
	CanExtrapolate   bool
	HasWeeklyWindow  bool
	CurrentRows      []Row
	PreviousEstimate *HistorySample
	ChangePercent    *float64
	ChangeKind       *ChangeKind
	History          []HistorySample
	// LatestStatsDate is the latest day of all daily rows, "" without any.
	LatestStatsDate string
	CalculatedAt    time.Time
	// UnavailableReason is nil when the estimate could be extrapolated.
	UnavailableReason *UnavailableReason
}

// IsWeekly reports a window of at least six days.
func IsWeekly(w Window) bool {
	return w.WindowMinutes != nil && *w.WindowMinutes > 0 && *w.WindowMinutes >= 6*24*60
}

// IsFiveHour reports a window of at most twelve hours.
func IsFiveHour(w Window) bool {
	return w.WindowMinutes != nil && *w.WindowMinutes > 0 && *w.WindowMinutes <= 12*60
}

// WeeklyWindow is the secondary window if weekly, else the primary if weekly.
func WeeklyWindow(q Quota) *Window {
	if q.Secondary != nil && IsWeekly(*q.Secondary) {
		return q.Secondary
	}
	if IsWeekly(q.Primary) {
		p := q.Primary
		return &p
	}
	return nil
}

// AutoMeter is the window ModeAuto reads: weekly, else secondary, else primary.
func AutoMeter(q Quota) Window {
	if w := WeeklyWindow(q); w != nil {
		return *w
	}
	if q.Secondary != nil {
		return *q.Secondary
	}
	return q.Primary
}

// Make computes the estimate. history is the stored samples of the account.
func Make(q Quota, daily []DailyRow, mode WindowMode, history []HistorySample, now time.Time) Snapshot {
	weekly := WeeklyWindow(q)
	var meter *Window
	switch mode {
	case ModeRollingWeek:
		meter = weekly
	default:
		m := AutoMeter(q)
		meter = &m
	}
	cycleStart := cycleStartDate(mode, meter, daily, now)
	var rows []Row
	for _, d := range daily {
		if d.Date >= cycleStart {
			rows = append(rows, rowFrom(d))
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	used := 0.0
	for _, r := range rows {
		used += r.Credits
	}
	usedPercent := 0.0
	if meter != nil {
		usedPercent = meter.UsedPercentExact
	}
	reason := evaluate(meter, rows)
	canExtrapolate := reason == nil
	var estimated, estimatedUSD *float64
	if canExtrapolate {
		v := used / (usedPercent / 100)
		estimated = &v
		u := USDForCredits(v)
		estimatedUSD = &u
	}
	previous, change, kind := compare(estimated, cycleStart, history)
	return Snapshot{
		WindowMode:        mode,
		CycleStartDate:    cycleStart,
		UsedPercent:       usedPercent,
		UsedCredits:       used,
		UsedUSD:           USDForCredits(used),
		DayCount:          len(rows),
		EstimatedCredits:  estimated,
		EstimatedUSD:      estimatedUSD,
		CanExtrapolate:    canExtrapolate,
		HasWeeklyWindow:   weekly != nil,
		CurrentRows:       rows,
		PreviousEstimate:  previous,
		ChangePercent:     change,
		ChangeKind:        kind,
		History:           Normalized(history),
		LatestStatsDate:   latestDate(daily),
		CalculatedAt:      now,
		UnavailableReason: reason,
	}
}

// SampleFrom is the history sample of a snapshot, nil when it has no estimate.
func SampleFrom(s Snapshot) *HistorySample {
	if !s.CanExtrapolate || s.EstimatedCredits == nil {
		return nil
	}
	return &HistorySample{
		CycleStartDate:   s.CycleStartDate,
		EstimatedCredits: *s.EstimatedCredits,
		UsedPercent:      s.UsedPercent,
		UsedCredits:      s.UsedCredits,
		RecordedAt:       s.CalculatedAt,
	}
}

func rowFrom(d DailyRow) Row {
	return Row{
		Date:            d.Date,
		Credits:         d.RawCredits,
		Tokens:          d.TotalTokens,
		Turns:           d.Turns,
		USD:             d.EstimatedUSD,
		CreditsReported: d.CreditsReported,
		TotalsReported:  d.TotalsReported,
	}
}

func cycleStartDate(mode WindowMode, meter *Window, daily []DailyRow, now time.Time) string {
	if mode == ModeRollingWeek {
		if latest := latestDate(daily); latest != "" {
			return AddUTCDays(latest, -6)
		}
		return AddUTCDays(UTCDay(now), -6)
	}
	minutes := 10_080
	if meter != nil && meter.WindowMinutes != nil {
		minutes = *meter.WindowMinutes
	}
	seconds := int64(minutes) * 60
	if meter != nil && meter.ResetsAt != nil && seconds > 0 {
		return UTCDay(meter.ResetsAt.Add(-time.Duration(seconds) * time.Second))
	}
	return AddUTCDays(UTCDay(now), -6)
}

func latestDate(daily []DailyRow) string {
	latest := ""
	for _, d := range daily {
		if d.Date > latest {
			latest = d.Date
		}
	}
	return latest
}

func compare(estimated *float64, cycleStart string, history []HistorySample) (*HistorySample, *float64, *ChangeKind) {
	if estimated == nil {
		return nil, nil, nil
	}
	var previous *HistorySample
	for i := range history {
		h := history[i]
		if h.CycleStartDate == cycleStart {
			continue
		}
		// The last of the cycles sorted by start date; ties keep the later
		// entry of the stored order.
		if previous == nil || h.CycleStartDate >= previous.CycleStartDate {
			previous = &h
		}
	}
	if previous == nil || previous.EstimatedCredits <= 0 {
		return previous, nil, nil
	}
	change := (*estimated - previous.EstimatedCredits) / previous.EstimatedCredits * 100
	var kind ChangeKind
	switch {
	case math.Abs(change) < SignificantChangePercent:
		kind = ChangeSimilar
	case change < 0:
		kind = ChangeDecreased
	default:
		kind = ChangeIncreased
	}
	return previous, &change, &kind
}

// CompactTokens formats a token count: 12, 1.50K, 2.25M.
func CompactTokens(value int) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.2fK", float64(value)/1_000)
	}
	return fmt.Sprintf("%d", value)
}

const dayLayout = "2006-01-02"

// UTCDay is the UTC calendar day of t, "2026-06-29".
func UTCDay(t time.Time) string { return t.UTC().Format(dayLayout) }

// AddUTCDays shifts a "yyyy-MM-dd" day; text that is not a day is returned
// unchanged.
func AddUTCDays(ymd string, delta int) string {
	day, err := time.Parse(dayLayout, ymd)
	if err != nil {
		return ymd
	}
	return UTCDay(day.AddDate(0, 0, delta))
}

// AnalyticsRange is the day range requested from the analytics endpoint: from
// 30 days before today (UTC) to tomorrow, plus the same range as an interval.
func AnalyticsRange(now time.Time) (start, end string, from, to time.Time) {
	today := now.UTC().Truncate(24 * time.Hour)
	from = today.Add(-AnalyticsLookbackDays * 24 * time.Hour)
	to = today.Add(24 * time.Hour)
	return UTCDay(from), UTCDay(to), from, to
}
