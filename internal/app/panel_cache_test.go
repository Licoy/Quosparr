package app

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
)

func cacheReportFixture() cachehit.Report {
	return cachehit.Report{
		Start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local), End: time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local),
		CalculatedAt: time.Date(2026, 10, 20, 12, 1, 0, 0, time.Local),
		Totals:       cachehit.Totals{InputTokens: 100_000, CachedInputTokens: 80_000, Samples: 12, MissingSamples: 2},
		Models: []cachehit.Row{
			{ID: "gpt-5", Name: "gpt-5", Totals: cachehit.Totals{InputTokens: 75_000, CachedInputTokens: 65_000, Samples: 8}},
			{ID: "gpt-5-mini", Name: "gpt-5-mini", Totals: cachehit.Totals{InputTokens: 25_000, CachedInputTokens: 15_000, Samples: 4}},
			{ID: "unknown", Totals: cachehit.Totals{MissingSamples: 2}},
		},
		Sessions: []cachehit.Row{{ID: "session-1", Name: "A very long local session name that must be ellipsized", Totals: cachehit.Totals{InputTokens: 100_000, CachedInputTokens: 80_000, Samples: 12}}},
	}
}

func TestCacheSummaryLoadsOnceAndTodayDetailReusesIt(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan CostQuery, 4)
	report := cacheReportFixture()
	acts := PanelActions{QueryCache: func(_ context.Context, _ store.Provider, q CostQuery) (cachehit.Report, error) {
		queries <- q
		return report, nil
	}}
	a, tt := newPanel(t, nil, panelFixture(), acts)
	a.syncCacheSummary(acts)
	q := receiveCost(t, queries)
	if q.Range != store.CostRangeToday || q.Custom != nil {
		t.Fatalf("summary query = %#v", q)
	}
	finishDetail(t, queue)
	tt.Frame()
	if !tt.HasText("80%") || !tt.HasText("Excluded 2 records without cache token data") {
		t.Fatalf("cache summary missing: %q", tt.Texts())
	}
	tt.Frame()
	select {
	case q := <-queries:
		t.Fatalf("repaint repeated cache query: %#v", q)
	default:
	}
	if err := tt.Click(tx(a.L(), xCacheViewDetails)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	select {
	case q := <-queries:
		t.Fatalf("today detail did not reuse summary: %#v", q)
	default:
	}
	if !tt.HasText("gpt-5") || !tt.HasText("65.00K / 75.00K") {
		t.Fatalf("model rows missing: %q", tt.Texts())
	}
	if err := tt.Click(tx(a.L(), xCacheSessions)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("A very long local session name") || tt.HasText("gpt-5") {
		t.Fatalf("session switch failed: %q", tt.Texts())
	}
	select {
	case <-queries:
		t.Fatal("model/session switch queried logs")
	default:
	}
}

func TestCacheTodayDetailWaitsForPendingSummary(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan CostQuery, 2)
	release := make(chan struct{})
	acts := PanelActions{QueryCache: func(_ context.Context, _ store.Provider, q CostQuery) (cachehit.Report, error) {
		queries <- q
		<-release
		return cacheReportFixture(), nil
	}}
	a, tt := newPanel(t, nil, panelFixture(), acts)
	a.syncCacheSummary(acts)
	<-queries
	a.panelState.detail = store.ModuleCacheAnalysis
	tt.Frame()
	select {
	case <-queries:
		t.Fatal("today detail duplicated the pending summary scan")
	default:
	}
	close(release)
	finishDetail(t, queue)
	tt.Frame()
	if a.panelState.cache.detail.report == nil || !tt.HasText("gpt-5") {
		t.Fatal("today detail did not adopt the completed summary")
	}
}

func TestCacheRangeSelectionWinsOverPendingTodaySummary(t *testing.T) {
	queue := detailMainQueue(t)
	todayStarted := make(chan struct{})
	todayRelease := make(chan struct{})
	monthStarted := make(chan struct{})
	acts := PanelActions{QueryCache: func(_ context.Context, _ store.Provider, q CostQuery) (cachehit.Report, error) {
		report := cacheReportFixture()
		if q.Range == store.CostRangeToday {
			close(todayStarted)
			<-todayRelease
			return report, nil
		}
		if q.Range == store.CostRangeThisMonth {
			close(monthStarted)
			report.Models[0].Name = "month-result"
			return report, nil
		}
		return cachehit.Report{}, errors.New("unexpected cache range")
	}}
	a, tt := newPanel(t, nil, panelFixture(), acts)
	a.syncCacheSummary(acts)
	<-todayStarted
	a.panelState.detail = store.ModuleCacheAnalysis
	tt.Frame()
	if err := tt.Click(a.L().Text(costRangeKey(store.CostRangeThisMonth))); err != nil {
		t.Fatal(err)
	}
	<-monthStarted
	finishDetail(t, queue)
	tt.Frame()
	if a.panelState.cache.selected != 3 || a.panelState.cache.detail.queryValue().Range != store.CostRangeThisMonth || !tt.HasText("month-result") {
		t.Fatalf("month selection was not retained: selected=%d query=%#v texts=%q", a.panelState.cache.selected, a.panelState.cache.detail.queryValue(), tt.Texts())
	}
	close(todayRelease)
	finishDetail(t, queue)
	tt.Frame()
	if a.panelState.cache.selected != 3 || a.panelState.cache.detail.queryValue().Range != store.CostRangeThisMonth || !tt.HasText("month-result") {
		t.Fatal("completed Today summary overwrote the selected month result")
	}
}

func TestCacheRangesAndInvalidCustomRange(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan CostQuery, 5)
	acts := PanelActions{QueryCache: func(_ context.Context, _ store.Provider, q CostQuery) (cachehit.Report, error) {
		queries <- q
		return cacheReportFixture(), nil
	}}
	a, tt := newPanel(t, nil, panelFixture(), acts)
	a.syncCacheSummary(acts)
	<-queries
	finishDetail(t, queue)
	tt.Frame()
	if err := tt.Click(tx(a.L(), xCacheViewDetails)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(a.L().Text(costRangeKey(store.CostRangeThisMonth))); err != nil {
		t.Fatal(err)
	}
	if q := <-queries; q.Range != store.CostRangeThisMonth {
		t.Fatalf("range query = %#v", q)
	}
	finishDetail(t, queue)
	tt.Frame()
	if err := tt.Click(a.L().Text(storeRangeCustomKey())); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.panelState.cache.start = time.Now()
	a.panelState.cache.end = time.Now().AddDate(0, 0, -1)
	tt.Frame()
	if err := tt.Click(a.L().Text(calculateKey())); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(a.L().Text(invalidRangeKey())) {
		t.Fatalf("invalid custom range hidden: %q", tt.Texts())
	}
}

// Small wrappers keep this test tied to the same localized controls as the cost page.
func storeRangeCustomKey() l10n.Key { return l10n.KeyCustomRange }
func calculateKey() l10n.Key        { return l10n.KeyCalculate }
func invalidRangeKey() l10n.Key     { return l10n.KeyInvalidDateRange }

func TestCacheDropsChangedContextResult(t *testing.T) {
	for _, change := range []string{"account", "provider", "generation"} {
		t.Run(change, func(t *testing.T) {
			queue := detailMainQueue(t)
			started := make(chan struct{})
			release := make(chan struct{})
			acts := PanelActions{QueryCache: func(context.Context, store.Provider, CostQuery) (cachehit.Report, error) {
				close(started)
				<-release
				return cacheReportFixture(), nil
			}}
			a, _ := newPanel(t, nil, panelFixture(), acts)
			a.syncCacheSummary(acts)
			<-started
			switch change {
			case "account":
				a.snap.Accounts.ActiveID = "other"
			case "provider":
				a.settings.SetSelectedProvider(store.ProviderGrok)
			case "generation":
				a.snap.Codex.Generation++
			}
			close(release)
			finishDetail(t, queue)
			if a.panelState.cache.summary.report != nil {
				t.Fatal("stale context result applied")
			}
		})
	}
}

func TestCacheZeroInputIsUnknown(t *testing.T) {
	if got := cachePercent(cachehit.Totals{CachedInputTokens: 50}); got != "—" {
		t.Fatalf("zero input displayed as %q", got)
	}
}

func TestCacheHealthThresholds(t *testing.T) {
	for _, tc := range []struct {
		cached int64
		want   runway.Health
	}{
		{9_010, runway.HealthGreen}, {9_000, runway.HealthYellow}, {8_000, runway.HealthYellow}, {7_999, runway.HealthRed},
	} {
		if got, ok := cacheHealth(cachehit.Totals{InputTokens: 10_000, CachedInputTokens: tc.cached}); !ok || got != tc.want {
			t.Errorf("cached %d of 10000: health %v, %v; want %v", tc.cached, got, ok, tc.want)
		}
	}
	if _, ok := cacheHealth(cachehit.Totals{}); ok {
		t.Error("zero input graded")
	}
}

func TestSessionCacheTotalsCountsWholeInput(t *testing.T) {
	got := sessionCacheTotals(sessions.Totals{UncachedInputTokens: 500, CachedInputTokens: 400, OutputTokens: 300})
	if got.InputTokens != 900 || got.CachedInputTokens != 400 || cachePercent(got) != "44.4%" {
		t.Fatalf("session cache totals = %+v (%s)", got, cachePercent(got))
	}
}

func TestCacheTableColumnsAlign(t *testing.T) {
	l := l10n.New(l10n.English)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(16).Children(func() { cacheRows(c, l, "By model", cacheReportFixture().Models[:1]) })
	}, 368, 240)
	header, ok := tt.Find("Hit rate")
	if !ok {
		t.Fatal("hit-rate header missing")
	}
	value, ok := tt.Find("86.7%")
	if !ok || math.Abs(float64(header.X-value.X)) > 0.5 || math.Abs(float64(header.W-value.W)) > 0.5 {
		t.Fatalf("hit-rate column misaligned: header=%+v value=%+v", header, value)
	}
}

func TestCacheFailureIsLatchedUntilRefresh(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan struct{}, 2)
	acts := PanelActions{QueryCache: func(context.Context, store.Provider, CostQuery) (cachehit.Report, error) {
		queries <- struct{}{}
		return cachehit.Report{}, errors.New("scan failed")
	}}
	a, tt := newPanel(t, nil, panelFixture(), acts)
	a.syncCacheSummary(acts)
	<-queries
	finishDetail(t, queue)
	tt.Frame()
	tt.Frame()
	select {
	case <-queries:
		t.Fatal("failed query repeated on repaint")
	default:
	}
	a.startCacheSummary(acts)
	<-queries
	finishDetail(t, queue)
}

func TestCacheTodayExpiresAcrossLocalDay(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan struct{}, 1)
	acts := PanelActions{QueryCache: func(context.Context, store.Provider, CostQuery) (cachehit.Report, error) {
		queries <- struct{}{}
		return cacheReportFixture(), nil
	}}
	a, _ := newPanel(t, nil, panelFixture(), PanelActions{})
	a.panelState.cache.summary = cacheQueryFixture(a, cacheReportFixture(), CostQuery{Range: store.CostRangeToday})
	a.panelState.cache.summary.day = "2000-01-01"
	a.syncCacheSummary(acts)
	<-queries
	finishDetail(t, queue)
}

func TestCacheTodayExpiresAfterTTL(t *testing.T) {
	queue := detailMainQueue(t)
	queries := make(chan struct{}, 1)
	acts := PanelActions{QueryCache: func(context.Context, store.Provider, CostQuery) (cachehit.Report, error) {
		queries <- struct{}{}
		return cacheReportFixture(), nil
	}}
	a, _ := newPanel(t, nil, panelFixture(), PanelActions{})
	a.panelState.cache.summary = cacheQueryFixture(a, cacheReportFixture(), CostQuery{Range: store.CostRangeToday})
	a.panelState.cache.summary.loadedAt = time.Now().Add(-cacheSummaryTTL - time.Second)
	a.syncCacheSummary(acts)
	<-queries
	finishDetail(t, queue)
}

func TestExportCacheAnalysisUI(t *testing.T) {
	if os.Getenv("RUNWAY_UI_QA") == "" {
		t.Skip("set RUNWAY_UI_QA to export cache analysis PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-ui-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	report := cacheReportFixture()
	queue := detailMainQueue(t)
	for _, dark := range []bool{false, true} {
		acts := PanelActions{QueryCache: func(_ context.Context, _ store.Provider, q CostQuery) (cachehit.Report, error) {
			out := report.Clone()
			if q.Custom != nil {
				out.Start, out.End = q.Custom.Window.Start, q.Custom.Window.End
			}
			return out, nil
		}}
		a, tt := newPanel(t, func(p *store.Preferences) {
			p.Language = store.LanguageSimplifiedChinese
			p.ShowsQuotaSummary = false
			p.ShowsTokenUsageHeatmap = false
			p.ShowsSpeedAnalysis = false
			p.ShowsRateLimitResetToday = false
			p.ShowsQuotaEstimateSummary = false
			p.ShowsResetCreditsSummary = false
			p.ShowsSessionRepairSummary = false
			p.ShowsRecentSessions = false
			p.ShowsSkills = false
			p.ShowsCacheAnalysis = true
			p.ShowsCostSummary = true
		}, panelFixture(), acts)
		a.panelState.cache.summary = cacheQueryFixture(a, report, CostQuery{Range: store.CostRangeToday})
		tt.SetSize(400, 584)
		tt.SetDark(dark)
		tt.Frame()
		theme := map[bool]string{false: "light", true: "dark"}[dark]
		exportReviewPNG(t, tt, filepath.Join(dir, "cache-summary-"+theme+".png"))
		a.panelState.detail = store.ModuleCacheAnalysis
		a.panelState.cache.initialized = true
		a.panelState.cache.detail = cacheQueryFixture(a, report, CostQuery{Range: store.CostRangeToday})
		tt.Frame()
		exportReviewPNG(t, tt, filepath.Join(dir, "cache-detail-models-"+theme+".png"))
		a.panelState.cache.showSessions = true
		tt.Frame()
		exportReviewPNG(t, tt, filepath.Join(dir, "cache-detail-sessions-"+theme+".png"))
		if err := tt.Click(a.L().Text(l10n.KeyCustomRange)); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		a.panelState.cache.start = time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
		a.panelState.cache.end = time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
		tt.Frame()
		if err := tt.Click(a.L().Text(l10n.KeyCalculate)); err != nil {
			t.Fatal(err)
		}
		finishDetail(t, queue)
		tt.Frame()
		exportReviewPNG(t, tt, filepath.Join(dir, "cache-detail-custom-"+theme+".png"))
	}
}

func cacheQueryFixture(a *App, report cachehit.Report, q CostQuery) panelCacheQueryState {
	copy := report.Clone()
	p := a.settings.Prefs().SelectedProvider
	day := ""
	if q.Custom == nil && q.Range == store.CostRangeToday {
		day = time.Now().In(time.Local).Format("2006-01-02")
	}
	return panelCacheQueryState{report: &copy, query: &q, provider: p, account: activePanelAccount(a.Snapshot(), p), accountGeneration: activePanelGeneration(a.Snapshot(), p), day: day, loadedAt: time.Now()}
}
