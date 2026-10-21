package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// CostQuery is a detail-only selection; it never changes summary preferences.
type CostQuery struct {
	Range  store.CostRange
	Custom *usage.CostRange
}

var (
	errDetailUnavailable    = errors.New("detail service unavailable")
	errGrokCycleUnavailable = errors.New("Grok billing cycle unavailable")
	errInvalidCostRange     = errors.New("invalid cost range")
)

func (rt *liveApp) queryDetailCost(ctx context.Context, provider store.Provider, q CostQuery) (usage.Summary, error) {
	now := time.Now()
	switch provider {
	case store.ProviderCursor:
		return usage.Summary{}, errDetailUnavailable
	case store.ProviderGrok:
		return rt.queryGrokDetailCost(ctx, q, now)
	case store.ProviderClaude:
		return rt.queryClaudeDetailCost(ctx, q, now)
	}
	if rt.svc == nil {
		return usage.Summary{}, errDetailUnavailable
	}
	if q.Custom != nil {
		return rt.svc.QueryCost(ctx, *q.Custom)
	}
	switch q.Range {
	case store.CostRangeCurrent:
		return rt.svc.QueryCurrentCycleCost(ctx)
	case store.CostRangePrevious:
		r, err := rt.svc.ResolvePreviousCycleRange(ctx)
		if err != nil {
			return usage.Summary{}, err
		}
		return rt.svc.QueryCost(ctx, r)
	case store.CostRangeToday:
		return rt.svc.QueryCost(ctx, usage.Today(now, time.Local))
	case store.CostRangeThisMonth:
		return rt.svc.QueryCost(ctx, usage.ThisMonth(now, time.Local))
	default:
		return usage.Summary{}, errInvalidCostRange
	}
}

func (rt *liveApp) queryGrokDetailCost(ctx context.Context, q CostQuery, now time.Time) (usage.Summary, error) {
	if rt.grokMod == nil || rt.grokMod.Store == nil || rt.grok == nil {
		return usage.Summary{}, errDetailUnavailable
	}
	var window usage.Window
	if q.Custom != nil {
		window = q.Custom.Window
	} else {
		rt.grok.mu.Lock()
		detail := rt.grok.state.Detail
		rt.grok.mu.Unlock()
		// A billing cycle must be reported; never silently invent a seven-day cycle.
		if q.Range == store.CostRangeCurrent || q.Range == store.CostRangePrevious {
			billing, ok := detail.(*grok.BillingSnapshot)
			if !ok || billing.Period == nil || billing.Period.StartsAt == nil || billing.Period.ResetsAt == nil || !billing.Period.ResetsAt.After(*billing.Period.StartsAt) {
				return usage.Summary{}, errGrokCycleUnavailable
			}
		}
		if !q.Range.Valid() {
			return usage.Summary{}, errInvalidCostRange
		}
		window = grokCostWindow(q.Range, detail, now)
	}
	if !window.End.After(window.Start) {
		return usage.Summary{}, errInvalidCostRange
	}
	return (grok.SessionScanner{GrokHome: rt.grokMod.Store.OfficialHome(), Location: time.Local}).ScanCost(ctx, window, now)
}

// queryClaudeDetailCost scans the Claude Code logs for one range. The
// current and previous cycles follow the cached 7-day window.
func (rt *liveApp) queryClaudeDetailCost(ctx context.Context, q CostQuery, now time.Time) (usage.Summary, error) {
	g := rt.claudeB
	if rt.claudeMod == nil || rt.claudeMod.Store == nil || g == nil {
		return usage.Summary{}, errDetailUnavailable
	}
	var window usage.Window
	switch {
	case q.Custom != nil:
		window = q.Custom.Window
	case q.Range == store.CostRangeToday, q.Range == store.CostRangeThisMonth, q.Range == store.CostRangeCurrent, q.Range == store.CostRangePrevious:
		g.mu.Lock()
		detail := g.state.Detail
		g.mu.Unlock()
		window = claudeCostWindow(q.Range, detail, now)
	default:
		return usage.Summary{}, errInvalidCostRange
	}
	if !window.End.After(window.Start) {
		return usage.Summary{}, errInvalidCostRange
	}
	scanner := claude.SessionScanner{ConfigDir: rt.claudeMod.Store.ConfigDir(), Home: rt.claudeMod.Store.HomeDir(), Location: time.Local, Cache: &g.sessions}
	return scanner.ScanCost(ctx, window, now)
}

func installDetailActions(rt *liveApp, a *PanelActions) {
	a.QueryCost = rt.queryDetailCost
	a.QueryCache = rt.queryCache
	client := &resettoday.ReactionClient{Store: resettoday.DefaultReactionStore()}
	a.QueryReaction = client.Fetch
	a.React = client.Click
	a.RepairSessions = func() { rt.queuePreferences(livePreferenceRequest{repair: true}) }
	installSkillActions(rt, &a.Skills)
	installSessionActions(rt, &a.Sessions)
	installMCPActions(rt, &a.MCP)
}
