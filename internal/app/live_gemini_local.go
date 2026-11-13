package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// geminiPriceProviders are the snapshot providers of the Antigravity
// sessions: Google for Gemini models, Anthropic for Claude models.
var geminiPriceProviders = []string{"google", "anthropic"}

// geminiPriceVersion joins the versions of geminiPriceProviders.
func geminiPriceVersion(snapshot pricing.Snapshot) string {
	version := ""
	for _, id := range geminiPriceProviders {
		version += id + ":" + snapshot.ProviderVersion(id) + "|"
	}
	return version
}

// sessionScanner reads the conversations under the store's ~/.gemini with
// the bridge's cache and price snapshot.
func (g *geminiBridge) sessionScanner() (gemini.SessionScanner, bool) {
	if g == nil || g.mod == nil || g.mod.Store == nil || g.mod.Store.GeminiHome() == "" {
		return gemini.SessionScanner{}, false
	}
	return gemini.SessionScanner{GeminiHome: g.mod.Store.GeminiHome(), Location: time.Local, Cache: &g.sessions, PricingSnapshot: g.pricingSnapshot}, true
}

func (g *geminiBridge) scanLocal(ctx context.Context) error {
	scanner, ok := g.sessionScanner()
	if !ok {
		return nil
	}
	g.mu.Lock()
	p, gen, st := g.prefs.Clone(), g.generation, g.state
	priceGen, pricingSnapshot := g.pricingGeneration, g.pricingSnapshot
	g.mu.Unlock()
	now := time.Now()
	opts := gemini.DefaultSessionScanOptions(now)
	opts.CostWindow = geminiCostWindow(p.APICostSummaryRange, st.Detail, now)
	if !p.ShowsRecentSessions {
		opts.RecentLimit = 0
	}
	snapshot := pricing.Snapshot{}
	if pricingSnapshot != nil {
		snapshot = pricingSnapshot()
		scanner.PricingSnapshot = func() pricing.Snapshot { return snapshot }
	}
	local, err := scanner.Scan(ctx, opts)
	report, speedErr := scanGeminiSpeed(ctx, scanner, p, now)
	g.mu.Lock()
	defer g.mu.Unlock()
	if gen != g.generation || priceGen != g.pricingGeneration || (pricingSnapshot != nil && geminiPriceVersion(snapshot) != geminiPriceVersion(pricingSnapshot())) || ctx.Err() != nil {
		return ctx.Err()
	}
	g.localErr = err
	if err == nil {
		g.local = &local
		g.localRange = p.APICostSummaryRange
	}
	g.speedErr = speedErr
	if speedErr == nil {
		g.speed = report
	}
	return errors.Join(err, speedErr)
}

// scanGeminiSpeed is the model speed report of the newest conversations,
// nil without an error when the module is hidden.
func scanGeminiSpeed(ctx context.Context, scanner gemini.SessionScanner, p store.Preferences, now time.Time) (*speed.Report, error) {
	if !p.ShowsSpeedAnalysis {
		return nil, nil
	}
	list, err := scanner.ScanSpeed(ctx, now)
	if err != nil {
		return nil, err
	}
	report := speed.Build(list, now)
	return &report, nil
}

// geminiCostWindow has no billing cycle to follow, so the current cycle is
// the weekly Gemini quota window when the cached reading has a future reset:
// it starts seven days before that reset. Otherwise, or when the reset has
// passed, it is the last seven days. Previous is the seven days before that.
func geminiCostWindow(r store.CostRange, detail any, now time.Time) usage.Window {
	switch r {
	case store.CostRangeToday:
		return usage.Today(now, time.Local).Window
	case store.CostRangeThisMonth:
		return usage.ThisMonth(now, time.Local).Window
	}
	start, end := now.Add(-7*24*time.Hour), now
	if snap, ok := detail.(*gemini.QuotaSnapshot); ok {
		for _, b := range snap.Buckets {
			if b.Family == gemini.FamilyGemini && b.Window == gemini.WindowWeekly && b.ResetAt != nil && b.ResetAt.After(now) {
				start, end = b.ResetAt.Add(-7*24*time.Hour), now
				break
			}
		}
	}
	if r == store.CostRangePrevious {
		return usage.Window{Start: start.Add(-7 * 24 * time.Hour), End: start}
	}
	return usage.Window{Start: start, End: end}
}

// applyLocalLocked puts the last scan on a freshly built state.
func (g *geminiBridge) applyLocalLocked(st *runway.GeminiState) {
	applyGeminiLocal(st, g.local)
	if g.prefs.ShowsSpeedAnalysis {
		st.Speed = g.speed
		if g.speedErr != nil {
			st.SpeedError = g.speedErr.Error()
		}
	}
	if g.localRange != g.prefs.APICostSummaryRange {
		st.Cost = nil
	}
	if g.localErr != nil {
		st.LocalError = g.lc.Text(l10n.KeyStatusError) + ": " + g.localErr.Error()
	}
}

func applyGeminiLocal(st *runway.GeminiState, local *gemini.SessionUsageSummary) {
	if local == nil {
		return
	}
	cost := local.CostSummary
	st.Cost = &cost
	st.Heatmap = runway.HeatmapData{Local: local.DailyTokens, CalculatedAt: local.CalculatedAt}
	for _, item := range local.RecentItems {
		row := sessions.Item{UnpricedTokens: item.Totals.UnpricedTokens, ID: item.ID, Title: item.Title, ProjectName: item.ProjectName, Cwd: item.CWD, UpdatedAt: item.UpdatedAt, State: sessions.StateRecent, Model: item.Model}
		totals := item.Totals.APITotals()
		row.Totals = sessions.Totals{TotalTokens: int(totals.TotalTokens), UncachedInputTokens: int(totals.UncachedInputTokens), CachedInputTokens: int(totals.CachedInputTokens), OutputTokens: int(totals.OutputTokens), Turns: int(totals.Turns), Threads: int(totals.Threads)}
		if item.Totals.EstimatedUSD != nil {
			if d, err := swiftjson.ParseDecimal(item.Totals.EstimatedUSD.String()); err == nil {
				row.EstimatedUSD = &d
			}
		}
		st.RecentSessions = append(st.RecentSessions, row)
	}
}
