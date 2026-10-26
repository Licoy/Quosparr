package app

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/store"
)

type livePricing struct {
	manager       *pricing.Manager
	mu            sync.Mutex
	prefs         store.ModelPricing
	displayPrefs  store.Preferences
	langs         []string
	catalogTarget string
	now           func() time.Time
	ctx           context.Context
	cancel        context.CancelFunc
	wake          chan struct{}
	wg            sync.WaitGroup
	closed        bool
	running       int
	versions      map[string]string
	rows          []PricingRow
	dataVersion   string
	changed       func(string)
	invalidate    func()
}

func newLivePricing(home string, p store.Preferences, fetch pricing.Fetcher, now func() time.Time) *livePricing {
	if now == nil {
		now = time.Now
	}
	m := pricing.NewManager(pricing.Config{CachePath: filepath.Join(platform.RunwayDir(home), pricing.CacheFileName), Fetch: fetch, Now: now})
	ctx, cancel := context.WithCancel(context.Background())
	pstate := &livePricing{manager: m, prefs: p.ModelPricing, displayPrefs: p.Clone(), langs: l10n.SystemPreferredLanguages(), now: now, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), versions: map[string]string{}}
	for _, id := range []string{"openai", "anthropic", "xai"} {
		pstate.versions[id] = m.Snapshot().ProviderVersion(id)
	}
	pstate.rows = pricingRows(pricing.Snapshot{}, l10n.ForPreference(string(p.Language), pstate.langs))
	return pstate
}

// pricingDue bases failure retries on LastAttempt, and cache freshness on persisted success.
func pricingDue(p store.ModelPricing, status pricing.Status, now time.Time) (time.Time, bool) {
	if !p.AutoSync {
		return time.Time{}, false
	}
	base := status.LastAttempt
	if status.LastSuccess.After(base) {
		base = status.LastSuccess
	}
	next := base.Add(time.Duration(p.SyncIntervalSeconds) * time.Second)
	if base.IsZero() {
		next = now
	}
	return next, !status.Running && !now.Before(next)
}
func (p *livePricing) start(ctx context.Context) {
	stop := context.AfterFunc(ctx, p.cancel)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer stop()
		p.prepareRows()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			p.mu.Lock()
			prefs := p.prefs
			p.mu.Unlock()
			_, due := pricingDue(prefs, p.manager.Status(), p.now())
			if due {
				_ = p.refresh(p.ctx)
			}
			select {
			case <-ctx.Done():
				p.cancel()
				return
			case <-p.ctx.Done():
				return
			case <-ticker.C:
			case <-p.wake:
			}
		}
	}()
}
func (p *livePricing) configure(prefs store.Preferences) {
	p.mu.Lock()
	p.prefs = prefs.ModelPricing
	p.displayPrefs = prefs.Clone()
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
	p.prepareRows()
	p.notify()
}
func (p *livePricing) refresh(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return context.Canceled
	}
	p.wg.Add(1)
	p.running++
	p.mu.Unlock()
	defer p.wg.Done()
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.ctx, cancel)
	defer stop()
	defer cancel()
	p.notify()
	err := p.manager.Refresh(work)
	p.prepareRows()
	snapshot := p.manager.Snapshot()
	var changed []string
	p.mu.Lock()
	p.running--
	for _, id := range []string{"openai", "anthropic", "xai"} {
		v := snapshot.ProviderVersion(id)
		if p.versions[id] != v {
			p.versions[id] = v
			changed = append(changed, id)
		}
	}
	p.mu.Unlock()
	for _, id := range changed {
		if p.changed != nil {
			p.changed(id)
		}
	}
	p.notify()
	return err
}
func (p *livePricing) notify() {
	if p.invalidate != nil {
		p.invalidate()
	}
}
func (p *livePricing) close() {
	p.mu.Lock()
	p.closed = true
	p.cancel()
	p.mu.Unlock()
	p.manager.Close()
	p.wg.Wait()
}
func (rt *liveApp) pricingVersion(provider store.Provider) string {
	if rt.pricing == nil {
		return ""
	}
	id := "openai"
	if provider == store.ProviderGrok {
		id = "xai"
	} else if provider == store.ProviderClaude {
		id = "anthropic"
	}
	return rt.pricing.manager.Snapshot().ProviderVersion(id)
}
func (rt *liveApp) pricingActions() PricingActions {
	if rt == nil || rt.pricing == nil {
		return PricingActions{}
	}
	return PricingActions{Refresh: rt.pricing.refresh, Snapshot: func() PricingSnapshot { return rt.pricing.snapshot(rt.svc.Preferences()) }}
}
