package runway

import (
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestEstimateUnavailableReasonsAreDistinct(t *testing.T) {
	zeroC := EstimateUnavailableText(zh, estimate.ReasonZeroCreditsWithUsage)
	missing := EstimateUnavailableText(zh, estimate.ReasonMissingCredits)
	noUsage := EstimateUnavailableText(zh, estimate.ReasonNoUsage)
	if !strings.Contains(zeroC, "Token") || !strings.Contains(zeroC, "Credits 返回 0") || strings.Contains(zeroC, "用量不足") {
		t.Errorf("zero credits: %q", zeroC)
	}
	if !strings.Contains(missing, "未提供完整 Credits") {
		t.Errorf("missing: %q", missing)
	}
	if !strings.Contains(noUsage, "尚无用量") {
		t.Errorf("no usage: %q", noUsage)
	}
	if zeroC == missing || missing == noUsage || zeroC == noUsage {
		t.Error("reasons must differ")
	}
	if notSynced := EstimateUnavailableText(zh, estimate.ReasonStatsNotSynced); !strings.Contains(notSynced, "尚未包含") || notSynced == noUsage {
		t.Errorf("stats not synced: %q", notSynced)
	}
}

func TestEstimateDataTimeShowsStatisticsDateAndFetchTime(t *testing.T) {
	fetched := time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC)
	snap := estimate.Snapshot{CalculatedAt: fetched, CurrentRows: []estimate.Row{{Date: "2026-09-01"}, {Date: "2026-09-02"}}}
	en := l10n.New(l10n.English)
	text := EstimateDataTimeText(en, snap)
	if !strings.Contains(text, "Statistics through 2026-09-02") {
		t.Errorf("statistics date: %q", text)
	}
	if !strings.Contains(text, "Updated "+ResetCreditUpdatedAt(fetched, l10n.English, nil)) {
		t.Errorf("fetch time: %q", text)
	}
	noRows := EstimateDataTimeText(en, estimate.Snapshot{CalculatedAt: fetched})
	if strings.Contains(noRows, "Statistics through") {
		t.Errorf("no rows: %q", noRows)
	}
}

func TestEstimateRefreshErrorKeepsErrorVisible(t *testing.T) {
	en := l10n.New(l10n.English)
	text := EstimateRefreshErrorText(en, "The request timed out")
	if !strings.Contains(text, "previously fetched data") || !strings.Contains(text, "The request timed out") {
		t.Errorf("text: %q", text)
	}
	if EstimateRefreshErrorText(en, "") != "" {
		t.Error("empty error must yield no text")
	}
}

func TestExpiryDuration(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	if ExpiryDuration(nil, l10n.English) != "--" {
		t.Error("nil is --")
	}
	for in, want := range map[float64]string{0: "0 seconds", 3661: "1 hour 1 minute", 25 * 86400: "25 days"} {
		if got := ExpiryDuration(f(in), l10n.English); got != want {
			t.Errorf("%v: %q want %q", in, got, want)
		}
	}
}

func TestExpiryHelpEveryLanguage(t *testing.T) {
	updated := time.Unix(1_786_601_000, 0).UTC()
	expiry := updated.Add(25 * 24 * time.Hour)
	for _, lang := range l10n.Languages() {
		lc := l10n.New(lang)
		known := ExpiryHelp(lc, expiry, updated, time.UTC)
		unknown := ExpiryHelp(lc, time.Time{}, updated, time.UTC)
		if !strings.HasPrefix(known, ResetCreditExpiresAt(expiry, lang, time.UTC)) {
			t.Errorf("%s known prefix: %q", lang, known)
		}
		if !strings.HasPrefix(unknown, lc.Text(l10n.KeyResetExpiryUnavailable)) || strings.HasPrefix(unknown, lc.Text(l10n.KeyNoExpiry)) {
			t.Errorf("%s unknown prefix: %q", lang, unknown)
		}
		for _, h := range []string{known, unknown} {
			if !strings.Contains(h, ResetCreditUpdatedAt(updated, lang, time.UTC)) || !strings.Contains(h, lc.Text(l10n.KeyResetExpirySummaryHelp)) {
				t.Errorf("%s missing parts: %q", lang, h)
			}
		}
	}
}

func TestClassifyAndSummarizeCredits(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	mk := func(id, status string, remaining time.Duration) codexapi.ResetCredit {
		c := codexapi.ResetCredit{ID: id, Status: status}
		if remaining > 0 {
			c.ExpiresAt = now.Add(remaining)
			c.RemainingSeconds = remaining.Seconds()
		}
		return c
	}
	credits := []codexapi.ResetCredit{
		mk("long", "available", 20*24*time.Hour),
		mk("soon", "available", 3*24*time.Hour),
		mk("never", "available", 0),
		mk("used", "used", 2*24*time.Hour),
	}
	want := []CreditRisk{CreditAvailable, CreditExpiring, CreditAvailable, CreditUnavailable}
	for i, c := range credits {
		if got := ClassifyCredit(c); got != want[i] {
			t.Errorf("%s: %v want %v", c.ID, got, want[i])
		}
	}
	sum := SummarizeCredits(codexapi.ResetCreditsSnapshot{AvailableCount: 3, Credits: credits, UpdatedAt: now})
	if sum.TotalCount != 4 || sum.StableAvailableCount != 2 || sum.ExpiringCount != 1 || sum.UnavailableCount != 1 {
		t.Errorf("counts: %+v", sum)
	}
	if !sum.NextExpiry.Equal(now.Add(3*24*time.Hour)) || !sum.LatestExpiry.Equal(now.Add(20*24*time.Hour)) {
		t.Errorf("expiries: %v %v", sum.NextExpiry, sum.LatestExpiry)
	}
	sorted := SortedCreditsByExpiry(credits)
	order := []string{"used", "soon", "long", "never"}
	for i, id := range order {
		if sorted[i].ID != id {
			t.Fatalf("sorted[%d]=%s want %s", i, sorted[i].ID, id)
		}
	}
	if credits[0].ID != "long" {
		t.Error("input was reordered")
	}
	details := CreditDetails(zh, codexapi.ResetCreditsSnapshot{Credits: credits})
	if len(details) != 4 || details[3].RemainingProgress != 1 || details[2].RemainingProgress >= 1 {
		t.Errorf("details: %+v", details)
	}
}

func TestMeterHealthAndProjection(t *testing.T) {
	for used, want := range map[int]Health{0: HealthGreen, 50: HealthGreen, 51: HealthYellow, 80: HealthYellow, 81: HealthRed, 100: HealthRed} {
		if got := HealthForUsedPercent(used); got != want {
			t.Errorf("used %d: %v want %v", used, got, want)
		}
	}
	now := time.Unix(1_000_000, 0)
	// Half the window elapsed, 30% used: projects 60% and never exhausts.
	w := MeterWindow{UsedPercent: 30, WindowMinutes: 100, ResetsAt: now.Add(50 * time.Minute)}
	p := MakeBurnProjection(w, now)
	if p == nil || p.ProjectedUsedPercentAtReset != 60 || !p.ExhaustsAt.IsZero() {
		t.Fatalf("projection: %+v", p)
	}
	// 80% used at the half: exhausts before the reset.
	w.UsedPercent = 80
	p = MakeBurnProjection(w, now)
	if p == nil || p.ExhaustsAt.IsZero() || !p.ExhaustsAt.Before(w.ResetsAt) || p.ProjectedUsedPercentAtReset != 100 {
		t.Fatalf("exhausting projection: %+v", p)
	}
	if MakeBurnProjection(MeterWindow{UsedPercent: 0, WindowMinutes: 100, ResetsAt: now.Add(time.Hour)}, now) != nil {
		t.Error("nothing used has no projection")
	}
	if MakeBurnProjection(MeterWindow{UsedPercent: 10, WindowMinutes: 100}, now) != nil {
		t.Error("no reset has no projection")
	}
	if p := MakeBurnProjection(MeterWindow{UsedPercent: 100, WindowMinutes: 100, ResetsAt: now.Add(time.Hour)}, now); p == nil || !p.ExhaustsAt.Equal(now) {
		t.Errorf("exhausted: %+v", p)
	}
}

func TestQuotaMetersAndMenuBarText(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	weekly := codexapi.RateWindow{UsedPercent: 11, WindowMinutes: WeeklyMinutes, ResetsAt: now.Add(26 * time.Hour)}
	five := codexapi.RateWindow{UsedPercent: 100, WindowMinutes: FiveHourMinutes, ResetsAt: now.Add(90 * time.Minute)}
	q := codexapi.QuotaSnapshot{
		Primary: five, Secondary: &weekly, UpdatedAt: now,
		AdditionalWindows: []codexapi.NamedRateWindow{{Name: "Spark", Window: weekly}},
	}
	if got := QuotaMeters(zh, q, false); len(got) != 2 || got[0].Title != "5小时" || got[1].Title != "每周" {
		t.Errorf("meters: %+v", got)
	}
	if got := QuotaMeters(zh, q, true); len(got) != 3 || got[2].Source != SourceModelSpecific {
		t.Errorf("with model-specific: %+v", got)
	}
	if got := MenuBarText(zh, q, now); !strings.HasPrefix(got, zh.Text(l10n.KeyStatusWait)+" ") {
		t.Errorf("used up: %q", got)
	}
	q.Primary.UsedPercent = 40
	if got := MenuBarText(l10n.New(l10n.English), q, now); got != "1 hour 30 minutes" {
		t.Errorf("countdown: %q", got)
	}
	q.Primary.ResetsAt = time.Time{}
	if got := MenuBarText(zh, q, now); got != "40%" {
		t.Errorf("no reset: %q", got)
	}
}

func TestCostCadence(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	reset := now.Add(24 * time.Hour)
	q := codexapi.QuotaSnapshot{Secondary: &codexapi.RateWindow{WindowMinutes: WeeklyMinutes, ResetsAt: reset}}
	cyc := cycleOf(q)
	interval := 300 * time.Second
	last := now.Add(-100 * time.Second)

	if !shouldRefreshCost(usage.RefreshForce, last, cyc, &q, interval, now) {
		t.Error("force always refreshes")
	}
	if !shouldRefreshCost(usage.RefreshIfChanged, time.Time{}, cyc, &q, interval, now) {
		t.Error("never completed refreshes")
	}
	if shouldRefreshCost(usage.RefreshIfChanged, last, cyc, &q, interval, now) {
		t.Error("recent and same cycle must not refresh")
	}
	if !shouldRefreshCost(usage.RefreshIfChanged, last, cyc, &q, interval, last.Add(interval)) {
		t.Error("interval elapsed must refresh")
	}
	q2 := codexapi.QuotaSnapshot{Secondary: &codexapi.RateWindow{WindowMinutes: WeeklyMinutes, ResetsAt: reset.Add(7 * 24 * time.Hour)}}
	if !shouldRefreshCost(usage.RefreshIfChanged, last, cyc, &q2, interval, now) {
		t.Error("new cycle must refresh early")
	}
	if shouldRefreshHeatmap(usage.RefreshIfChanged, last, interval, now) || !shouldRefreshHeatmap(usage.RefreshIfChanged, last, interval, last.Add(interval)) {
		t.Error("heatmap cadence")
	}
	if !shouldRefreshHeatmap(usage.RefreshForce, last, interval, now) {
		t.Error("heatmap force")
	}
}

func displayable(tokens int64) usage.Summary {
	return usage.Summary{Confidence: usage.ConfidencePriced, Totals: usage.Totals{TotalTokens: tokens}}
}

func TestDetailCacheIsBoundedFIFO(t *testing.T) {
	var c detailCache
	for i := 0; i < detailCacheLimit+2; i++ {
		c.put(string(rune('a'+i)), displayable(int64(i+1)))
	}
	if _, ok := c.get("a"); ok {
		t.Error("oldest entry must be evicted")
	}
	if _, ok := c.get("b"); ok {
		t.Error("second oldest must be evicted")
	}
	if s, ok := c.get("h"); !ok || s.Totals.TotalTokens != 8 {
		t.Error("newest entry missing")
	}
	if len(c.items) != detailCacheLimit {
		t.Errorf("size %d", len(c.items))
	}
	c.put("empty", usage.Summary{})
	if _, ok := c.get("empty"); ok {
		t.Error("empty scan must not be cached")
	}
	c.clear()
	if _, ok := c.get("h"); ok {
		t.Error("clear")
	}
}

func TestDisplayCostAndSubtitle(t *testing.T) {
	en := l10n.New(l10n.English)
	now := time.Unix(10_000, 0)
	usd, _ := decimal.Parse("12.5")
	s := displayable(1_500_000)
	s.EstimatedUSD = &usd
	s.Source = usage.SourceLocalSessions
	s.CalculatedAt = now.Add(-2 * time.Minute)
	s.Totals.Turns = 7
	d := DisplayCost(en, s, store.CostRangeCurrent, "", now)
	if !strings.HasPrefix(d.Text, "$12.5000 ") || !strings.Contains(d.Text, "1.50M") {
		t.Errorf("text: %q", d.Text)
	}
	if !strings.Contains(d.Subtitle, "7 ") || !strings.Contains(d.Subtitle, "2 minutes ago") {
		t.Errorf("subtitle: %q", d.Subtitle)
	}
	failed := DisplayCost(en, s, store.CostRangeCurrent, "disk full", now)
	if got := failed.Lines[len(failed.Lines)-1]; got.Value != "disk full" {
		t.Errorf("scan note line: %+v", got)
	}
	empty := DisplayCost(en, usage.Summary{}, store.CostRangeToday, "", now)
	if empty.Text != en.Text(l10n.KeyUsageAnalyticsEmpty) {
		t.Errorf("empty: %q", empty.Text)
	}
	if c := ClearedCost(en, "boom"); c.Text != "boom" || len(c.Lines) != 1 {
		t.Errorf("cleared: %+v", c)
	}
}
