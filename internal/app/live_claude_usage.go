package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func (g *claudeBridge) scanLocal(ctx context.Context) error {
	if g.mod == nil || g.mod.Store == nil {
		return nil
	}
	g.mu.Lock()
	p, gen, st := g.prefs.Clone(), g.generation, g.state
	priceGen, pricingSnapshot := g.pricingGeneration, g.pricingSnapshot
	g.mu.Unlock()
	now := time.Now()
	opts := claude.DefaultSessionScanOptions(now)
	opts.CostWindow = claudeCostWindow(p.APICostSummaryRange, st.Detail, now)
	if !p.ShowsRecentSessions {
		opts.RecentLimit = 0
	}
	scanner := claude.SessionScanner{ConfigDir: g.mod.Store.ConfigDir(), Home: g.mod.Store.HomeDir(), Location: time.Local, Cache: &g.sessions}
	snapshot := pricing.Snapshot{}
	if pricingSnapshot != nil {
		snapshot = pricingSnapshot()
		scanner.PricingSnapshot = func() pricing.Snapshot { return snapshot }
	}
	local, err := scanner.Scan(ctx, opts)
	report, speedErr := scanClaudeSpeed(ctx, scanner, p, now)
	g.mu.Lock()
	defer g.mu.Unlock()
	if gen != g.generation || priceGen != g.pricingGeneration || (pricingSnapshot != nil && snapshot.ProviderVersion("anthropic") != pricingSnapshot().ProviderVersion("anthropic")) || ctx.Err() != nil {
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

func scanClaudeSpeed(ctx context.Context, scanner claude.SessionScanner, p store.Preferences, now time.Time) (*speed.Report, error) {
	if !p.ShowsSpeedAnalysis {
		return nil, nil
	}
	report, err := scanner.ScanSpeed(ctx, now)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// claudeCostWindow is the current cycle when a 7-day window has resets_at:
// the cycle starts seven days before that instant. Otherwise, or when the
// cached reset has already passed, it is the last seven days. Previous is the
// seven days before that.
func claudeCostWindow(r store.CostRange, detail any, now time.Time) usage.Window {
	switch r {
	case store.CostRangeToday:
		return usage.Today(now, time.Local).Window
	case store.CostRangeThisMonth:
		return usage.ThisMonth(now, time.Local).Window
	}
	if snap, ok := detail.(*claude.UsageSnapshot); ok {
		for _, w := range snap.Windows {
			if w.Kind == claude.WindowSevenDay && w.ResetsAt != nil && w.ResetsAt.After(now) {
				end := *w.ResetsAt
				start := end.Add(-7 * 24 * time.Hour)
				if r == store.CostRangePrevious {
					return usage.Window{Start: start.Add(-7 * 24 * time.Hour), End: start}
				}
				return usage.Window{Start: start, End: minTime(end, now)}
			}
		}
	}
	start := now.Add(-7 * 24 * time.Hour)
	if r == store.CostRangePrevious {
		return usage.Window{Start: start.Add(-7 * 24 * time.Hour), End: start}
	}
	return usage.Window{Start: start, End: now}
}

func applyClaudeLocal(st *runway.ClaudeState, local *claude.SessionUsageSummary) {
	if local == nil {
		return
	}
	cost := local.CostSummary
	st.Cost = &cost
	st.Heatmap = runway.HeatmapData{Local: local.DailyTokens, CalculatedAt: local.CalculatedAt}
	for _, item := range local.RecentItems {
		row := item.SessionsItem()
		if row.State == "" {
			row.State = sessions.StateRecent
		}
		st.RecentSessions = append(st.RecentSessions, row)
	}
}
