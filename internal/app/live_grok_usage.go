package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func initialWidgetRequirements(s *Settings) export.Requirements {
	if s != nil && s.System().Widgets {
		return export.RequireAll
	}
	return 0
}

func (g *grokBridge) scanLocal(ctx context.Context) error {
	if g.mod == nil || g.mod.Store == nil {
		return nil
	}
	g.mu.Lock()
	p, gen, st := g.prefs.Clone(), g.generation, g.state
	priceGen, pricingSnapshot := g.pricingGeneration, g.pricingSnapshot
	g.mu.Unlock()
	opts := grok.DefaultSessionScanOptions(time.Now())
	opts.CostWindow = grokCostWindow(p.APICostSummaryRange, st.Detail, opts.Now)
	if !p.ShowsRecentSessions {
		opts.RecentLimit = 0
	}
	scanner := grok.SessionScanner{GrokHome: g.mod.Store.OfficialHome(), Location: time.Local}
	snapshot := pricing.Snapshot{}
	if pricingSnapshot != nil {
		snapshot = pricingSnapshot()
		scanner.PricingSnapshot = func() pricing.Snapshot { return snapshot }
	}
	local, err := scanner.Scan(ctx, opts)
	report, speedErr := scanGrokSpeed(ctx, scanner, p, opts.Now)
	g.mu.Lock()
	defer g.mu.Unlock()
	if gen != g.generation || priceGen != g.pricingGeneration || (pricingSnapshot != nil && snapshot.ProviderVersion("xai") != pricingSnapshot().ProviderVersion("xai")) || ctx.Err() != nil {
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

// scanGrokSpeed is the model speed report of the newest Grok sessions, nil
// without an error when the module is hidden.
func scanGrokSpeed(ctx context.Context, scanner grok.SessionScanner, p store.Preferences, now time.Time) (*speed.Report, error) {
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

func grokCostWindow(r store.CostRange, detail any, now time.Time) usage.Window {
	switch r {
	case store.CostRangeToday:
		return usage.Today(now, time.Local).Window
	case store.CostRangeThisMonth:
		return usage.ThisMonth(now, time.Local).Window
	}
	if snap, ok := detail.(*grok.BillingSnapshot); ok && snap.Period != nil && snap.Period.StartsAt != nil && snap.Period.ResetsAt != nil && snap.Period.ResetsAt.After(*snap.Period.StartsAt) {
		full := usage.Window{Start: *snap.Period.StartsAt, End: *snap.Period.ResetsAt}
		if r == store.CostRangePrevious {
			return usage.PreviousCycle(full, time.Local).Window
		}
		full.End = minTime(full.End, now)
		return full
	}
	start := now.Add(-7 * 24 * time.Hour)
	if r == store.CostRangePrevious {
		return usage.Window{Start: start.Add(-7 * 24 * time.Hour), End: start}
	}
	return usage.Window{Start: start, End: now}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func applyGrokLocal(st *runway.GrokState, local *grok.SessionUsageSummary) {
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
			d, err := swiftjson.ParseDecimal(item.Totals.EstimatedUSD.String())
			if err == nil {
				row.EstimatedUSD = &d
			}
		}
		st.RecentSessions = append(st.RecentSessions, row)
	}
}
