package app

import (
	"context"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func (rt *liveApp) pricesChanged(provider string) {
	switch provider {
	case "openai":
		rt.svc.PricingChanged(usage.SnapshotPrices(rt.pricing.manager.Snapshot).PriceBook(context.Background()).Version())
	case "xai":
		if g := rt.grok; g != nil {
			g.mu.Lock()
			g.pricingGeneration++
			g.local = nil
			g.state.Cost = nil
			g.state.RecentSessions = nil
			g.mu.Unlock()
			rt.svc.RefreshProviderLocal(store.ProviderGrok)
		}
	case "anthropic":
		if g := rt.claudeB; g != nil {
			g.mu.Lock()
			g.pricingGeneration++
			g.local = nil
			g.state.Cost = nil
			g.state.RecentSessions = nil
			g.mu.Unlock()
			rt.svc.RefreshProviderLocal(store.ProviderClaude)
		}
		// Antigravity also runs Claude models.
		rt.geminiPricesChanged()
	case "google":
		rt.geminiPricesChanged()
	}
}

func (rt *liveApp) geminiPricesChanged() {
	g := rt.geminiB
	if g == nil {
		return
	}
	g.mu.Lock()
	g.pricingGeneration++
	g.local = nil
	g.state.Cost = nil
	g.state.RecentSessions = nil
	g.mu.Unlock()
	if rt.svc != nil {
		rt.svc.RefreshProviderLocal(store.ProviderGemini)
	}
}
