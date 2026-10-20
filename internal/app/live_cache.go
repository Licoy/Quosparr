package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

var (
	errInvalidCacheProvider   = errors.New("invalid cache analysis provider")
	errClaudeCycleUnavailable = errors.New("Claude billing cycle unavailable")
)

// queryCache scans local logs independently of the cost module and online
// account statistics.
func (rt *liveApp) queryCache(ctx context.Context, provider store.Provider, q CostQuery) (cachehit.Report, error) {
	if !provider.Valid() {
		return cachehit.Report{}, errInvalidCacheProvider
	}
	now := time.Now()
	window, err := rt.cacheWindow(ctx, provider, q, now)
	if err != nil {
		return cachehit.Report{}, err
	}
	switch provider {
	case store.ProviderCodex:
		if rt.codex == nil {
			return cachehit.Report{}, errDetailUnavailable
		}
		return (usage.Scanner{CodexHome: rt.codexFolder(), Location: time.Local, Cache: &rt.codexCache}).ScanCache(ctx, window)
	case store.ProviderGrok:
		if rt.grokMod == nil || rt.grokMod.Store == nil {
			return cachehit.Report{}, errDetailUnavailable
		}
		return (grok.SessionScanner{GrokHome: rt.grokMod.Store.OfficialHome(), Location: time.Local, Cache: &rt.grokCache}).ScanCache(ctx, window, now)
	case store.ProviderClaude:
		if rt.claudeMod == nil || rt.claudeMod.Store == nil || rt.claudeB == nil {
			return cachehit.Report{}, errDetailUnavailable
		}
		return (claude.SessionScanner{
			ConfigDir: rt.claudeMod.Store.ConfigDir(), Home: rt.claudeMod.Store.HomeDir(),
			Location: time.Local, Cache: &rt.claudeB.sessions,
		}).ScanCache(ctx, window, now)
	}
	return cachehit.Report{}, errInvalidCacheProvider
}

func (rt *liveApp) cacheWindow(ctx context.Context, provider store.Provider, q CostQuery, now time.Time) (usage.Window, error) {
	if q.Custom != nil {
		return validCacheWindow(q.Custom.Window)
	}
	switch q.Range {
	case store.CostRangeToday:
		return usage.Today(now, time.Local).Window, nil
	case store.CostRangeThisMonth:
		return usage.ThisMonth(now, time.Local).Window, nil
	case store.CostRangeCurrent, store.CostRangePrevious:
	default:
		return usage.Window{}, errInvalidCostRange
	}
	previous := q.Range == store.CostRangePrevious
	switch provider {
	case store.ProviderCodex:
		if rt.svc == nil {
			return usage.Window{}, errDetailUnavailable
		}
		r, err := rt.svc.ResolveCycleRange(ctx, previous)
		if err != nil {
			return usage.Window{}, err
		}
		return validCacheWindow(r.Window)
	case store.ProviderGrok:
		return rt.grokCacheWindow(previous, now)
	case store.ProviderClaude:
		return rt.claudeCacheWindow(previous, now)
	default:
		return usage.Window{}, errInvalidCacheProvider
	}
}

func (rt *liveApp) grokCacheWindow(previous bool, now time.Time) (usage.Window, error) {
	if rt.grok == nil {
		return usage.Window{}, errDetailUnavailable
	}
	rt.grok.mu.Lock()
	detail := rt.grok.state.Detail
	rt.grok.mu.Unlock()
	billing, ok := detail.(*grok.BillingSnapshot)
	if !ok || billing.Period == nil || billing.Period.StartsAt == nil || billing.Period.ResetsAt == nil ||
		billing.Period.StartsAt.After(now) || !billing.Period.ResetsAt.After(now) {
		return usage.Window{}, errGrokCycleUnavailable
	}
	full := usage.Window{Start: *billing.Period.StartsAt, End: *billing.Period.ResetsAt}
	if previous {
		return usage.PreviousCycle(full, time.Local).Window, nil
	}
	full.End = minTime(full.End, now)
	return validCacheWindow(full)
}

func (rt *liveApp) claudeCacheWindow(previous bool, now time.Time) (usage.Window, error) {
	if rt.claudeB == nil {
		return usage.Window{}, errDetailUnavailable
	}
	rt.claudeB.mu.Lock()
	detail := rt.claudeB.state.Detail
	rt.claudeB.mu.Unlock()
	if snap, ok := detail.(*claude.UsageSnapshot); ok {
		for _, w := range snap.Windows {
			if w.Kind != claude.WindowSevenDay || w.ResetsAt == nil || !w.ResetsAt.After(now) {
				continue
			}
			end := *w.ResetsAt
			start := end.Add(-7 * 24 * time.Hour)
			if previous {
				return usage.Window{Start: start.Add(-7 * 24 * time.Hour), End: start}, nil
			}
			return validCacheWindow(usage.Window{Start: start, End: minTime(end, now)})
		}
	}
	return usage.Window{}, errClaudeCycleUnavailable
}

func validCacheWindow(window usage.Window) (usage.Window, error) {
	if !window.End.After(window.Start) {
		return usage.Window{}, errInvalidCostRange
	}
	return window, nil
}
