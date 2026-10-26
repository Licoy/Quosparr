package runway

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// Query ids of the cost scans.
const (
	queryCurrent  = "current-cycle"
	querySelected = "selected-range"
	queryDetail   = "detail-range"
	queryHeatmap  = "token-heatmap"
)

// detailCacheLimit bounds the per-range cost cache.
const detailCacheLimit = 6

// ErrUsageUnavailable means no cost could be resolved for a range.
var ErrUsageUnavailable = errors.New("usage analytics unavailable")

// costCycle identifies the cost cycle: when it changes the cost is rescanned
// even before the refresh interval elapses.
type costCycle struct {
	reset         time.Time
	windowMinutes int
}

func cycleOf(q codexapi.QuotaSnapshot) costCycle {
	if q.Secondary == nil {
		return costCycle{}
	}
	return costCycle{reset: q.Secondary.ResetsAt, windowMinutes: q.Secondary.WindowMinutes}
}

// shouldRefreshCost mirrors Swift shouldRefreshCost: every policy but
// ifChanged refreshes; ifChanged refreshes when the cycle changed or the
// refresh interval elapsed since the last completion.
func shouldRefreshCost(policy usage.RefreshPolicy, lastDone time.Time, lastCycle costCycle, q *codexapi.QuotaSnapshot, interval time.Duration, now time.Time) bool {
	if policy != usage.RefreshIfChanged || lastDone.IsZero() {
		return true
	}
	if q != nil && cycleOf(*q) != lastCycle {
		return true
	}
	return !now.Before(lastDone.Add(interval))
}

// shouldRefreshHeatmap is the same rule without the cycle.
func shouldRefreshHeatmap(policy usage.RefreshPolicy, lastDone time.Time, interval time.Duration, now time.Time) bool {
	if policy != usage.RefreshIfChanged || lastDone.IsZero() {
		return true
	}
	return !now.Before(lastDone.Add(interval))
}

// cycleWindows converts the quota cycle into usage windows.
func cycleWindows(q codexapi.QuotaSnapshot, now time.Time) (full, elapsed usage.Window, ok bool) {
	f, e, ok := q.CycleWindows(now)
	if !ok {
		return usage.Window{}, usage.Window{}, false
	}
	return usage.Window{Start: f.Start, End: f.End}, usage.Window{Start: e.Start, End: e.End}, true
}

// cycleFor is the cost cycle of the shown credential: the quota's own, or the
// calendar month for an API key, which has no quota cycle. Its "previous
// cycle" is the window of the same length before the month.
func cycleFor(auth codex.Auth, q codexapi.QuotaSnapshot, now time.Time, loc *time.Location) (full, elapsed usage.Window, ok bool) {
	if auth.IsAPIKey() {
		full, elapsed = monthWindows(now, loc)
		return full, elapsed, true
	}
	return cycleWindows(q, now)
}

// monthWindows is the calendar month of now (full) and its elapsed part.
func monthWindows(now time.Time, loc *time.Location) (full, elapsed usage.Window) {
	start := usage.ThisMonth(now, loc).Window.Start
	return usage.Window{Start: start, End: start.In(loc).AddDate(0, 1, 0)}, usage.Window{Start: start, End: now}
}

// selectedCostRange is the range the user's apiCostSummaryRange asks for
// besides the current cycle; ok=false when it cannot be determined.
func selectedCostRange(r store.CostRange, full *usage.Window, now time.Time, loc *time.Location) (usage.CostRange, bool) {
	switch r {
	case store.CostRangePrevious:
		if full == nil {
			return usage.CostRange{}, false
		}
		return usage.PreviousCycle(*full, loc), true
	case store.CostRangeToday:
		return usage.Today(now, loc), true
	case store.CostRangeThisMonth:
		return usage.ThisMonth(now, loc), true
	}
	return usage.CostRange{}, false
}

// costQueries lists the local scans for the current cycle and the selection.
func costQueries(current *usage.Window, selected *usage.CostRange) []usage.Query {
	var qs []usage.Query
	if current != nil {
		qs = append(qs, usage.Query{ID: queryCurrent, Window: *current})
	}
	if selected != nil {
		qs = append(qs, usage.Query{ID: querySelected, Window: selected.Window})
	}
	return qs
}

func detailCacheKey(r usage.CostRange) string {
	f := func(t time.Time) string { return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 3, 64) }
	return r.APIStartDate + "|" + r.APIEndDate + "|" + f(r.Window.Start) + "|" + f(r.Window.End)
}

// detailCache is the FIFO cache of range summaries behind the cost detail page.
type detailCache struct {
	items map[string]usage.Summary
	order []string
}

func (c *detailCache) get(key string) (usage.Summary, bool) {
	s, ok := c.items[key]
	return s, ok
}

// put stores a displayable summary; empty or failed scans are never pinned.
func (c *detailCache) put(key string, s usage.Summary) {
	if !s.IsDisplayableCost() {
		return
	}
	if c.items == nil {
		c.items = map[string]usage.Summary{}
	}
	if _, ok := c.items[key]; !ok {
		c.order = append(c.order, key)
	}
	c.items[key] = s
	for len(c.order) > detailCacheLimit {
		delete(c.items, c.order[0])
		c.order = c.order[1:]
	}
}

func (c *detailCache) clear() { c.items, c.order = nil, nil }

// dailyTokenMap sums the displayed tokens per day.
func dailyTokenMap(s usage.Summary) map[string]int64 {
	m := make(map[string]int64, len(s.DailyRows))
	for _, r := range s.DailyRows {
		m[r.Date] += max(0, r.Totals.TotalTokens)
	}
	return m
}

// costSourceText localizes where a cost came from.
func costSourceText(lc l10n.Localizer, s usage.Source) string {
	switch s {
	case usage.SourceLocalSessions:
		return lc.Text(l10n.KeySourceLocalSessions)
	case usage.SourceOnlineAnalytics:
		return lc.Text(l10n.KeySourceOnlineSupplement)
	}
	return lc.Text(l10n.KeyUsageAnalyticsEmpty)
}

func costRangeText(lc l10n.Localizer, r store.CostRange) string {
	switch r {
	case store.CostRangeToday:
		return lc.Text(l10n.KeyToday)
	case store.CostRangePrevious:
		return lc.Text(l10n.KeyPreviousCycle)
	case store.CostRangeThisMonth:
		return lc.Text(l10n.KeyThisMonth)
	}
	return lc.Text(l10n.KeyCurrentCycle)
}

// CostSubtitle is "range · pricing · N turns · calculated <ago>". The age has
// minute granularity so the string changes once a minute.
func CostSubtitle(lc l10n.Localizer, s usage.Summary, r store.CostRange, now time.Time) string {
	pricing := lc.Text(l10n.KeyApiTokenPricing)
	if s.EstimatedUSD == nil || s.Confidence == usage.ConfidenceTokensOnly {
		pricing = lc.Text(l10n.KeyTokensOnly)
	} else if s.Confidence == usage.ConfidencePartiallyPriced || s.UnpricedTokens > 0 {
		pricing = lc.Pricing("partial")
	}
	calculated := RelativePast(s.CalculatedAt, now, lc.Language(), false)
	return costRangeText(lc, r) + " · " + pricing + " · " + strconv.FormatInt(s.Totals.Turns, 10) + " " +
		lc.Text(l10n.KeyTurns) + " · " + lc.Text(l10n.KeyCalculatedAt) + " " + calculated
}

// CostDisplay is the text form of the displayed cost.
type CostDisplay struct {
	Text     string
	Subtitle string
	Lines    []DetailLine
}

// DisplayCost renders a cost summary (Swift applyDisplayedCost). scanNote is
// the failure of the latest rescan that left the previous summary in place.
func DisplayCost(lc l10n.Localizer, s usage.Summary, r store.CostRange, scanNote string, now time.Time) CostDisplay {
	src := costSourceText(lc, s.Source)
	d := CostDisplay{Subtitle: CostSubtitle(lc, s, r, now)}
	if !s.IsDisplayableCost() {
		d.Text = lc.Text(l10n.KeyUsageAnalyticsEmpty)
		d.Lines = []DetailLine{
			{lc.Text(l10n.KeyApiCost), lc.Text(l10n.KeyUsageAnalyticsEmpty)},
			{lc.Text(l10n.KeyApiCostSource), src},
		}
	} else {
		amount := "--"
		if s.EstimatedUSD != nil {
			amount = Money(*s.EstimatedUSD)
			if s.Confidence == usage.ConfidencePartiallyPriced || s.UnpricedTokens > 0 {
				amount += " · " + lc.Pricing("partial")
			}
		}
		t := s.Totals
		d.Text = amount + " " + lc.Text(l10n.KeyApiEquivalent) + " · " + CompactNumber(t.TotalTokens) + " " + lc.Text(l10n.KeyTokens) + " · " + src
		d.Lines = []DetailLine{
			{lc.Text(l10n.KeyEstimatedAPICost), amount},
			{lc.Text(l10n.KeyTokens), CompactNumber(t.TotalTokens)},
			{lc.Text(l10n.KeyInputCachedOutput), CompactNumber(t.UncachedInputTokens) + " / " + CompactNumber(t.CachedInputTokens) + " / " + CompactNumber(t.OutputTokens)},
			{lc.Text(l10n.KeyTurns), strconv.FormatInt(t.Turns, 10)},
			{lc.Text(l10n.KeyApiCostSource), src},
			{lc.Text(l10n.KeyPricingVersion), s.PricingVersion},
		}
		if s.EstimatedUSD == nil {
			d.Text = lc.Text(l10n.KeyTokensOnly) + " · " + CompactNumber(t.TotalTokens) + " " + lc.Text(l10n.KeyTokens) + " · " + src
		}
		if s.UnpricedTokens > 0 {
			d.Lines = append(d.Lines, DetailLine{lc.Pricing("unpriced"), lc.PricingRule("unpricedCount", CompactNumber(s.UnpricedTokens))})
		}
		if s.Source == usage.SourceLocalSessions {
			d.Lines = append(d.Lines, DetailLine{lc.Text(l10n.KeyEstimatedAPICost), lc.Pricing("currentEstimate")})
		}
		for _, warning := range s.Warnings {
			if strings.HasPrefix(warning, "price-source:") {
				continue
			}
			kind, model, hasModel := strings.Cut(warning, ":")
			key := ""
			switch kind {
			case "unknown-model":
				key = "unknownModel"
			case "fast-unpriced":
				key = "fastUnpriced"
			case "missing-rate", "missing-price-rule", "missing-long-rule", "ambiguous-tier":
				key = "missingPrice"
			}
			if key != "" && hasModel {
				d.Lines = append(d.Lines, DetailLine{lc.Pricing("pricingIssues"), lc.PricingRule(key, model)})
			} else {
				d.Lines = append(d.Lines, DetailLine{lc.Pricing("scanIssues"), warning})
			}
		}

		if s.Source == usage.SourceOnlineAnalytics {
			d.Lines = append(d.Lines, DetailLine{lc.Text(l10n.KeyRawAnalyticsCredits), strconv.FormatFloat(s.RawCredits, 'f', 1, 64)})
		}
	}
	if scanNote != "" {
		d.Lines = append(d.Lines, DetailLine{lc.Text(l10n.KeyCostScanFailed), scanNote})
	}
	return d
}

// ClearedCost is the display of a failed first scan: just the message.
func ClearedCost(lc l10n.Localizer, text string) CostDisplay {
	return CostDisplay{Text: text, Lines: []DetailLine{{lc.Text(l10n.KeyApiCost), text}}}
}
