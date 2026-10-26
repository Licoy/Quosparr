package runway

import (
	"context"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/usage"
)

type priceStamp struct {
	generation uint64
	version    string
}

func (s *Service) priceStampLocked() priceStamp {
	version := s.pricingVersion
	if s.opts.PricingVersion != nil {
		version = s.opts.PricingVersion()
	}
	return priceStamp{s.pricingGeneration, version}
}
func (s *Service) pricesMatchLocked(stamp priceStamp) bool { return stamp == s.priceStampLocked() }
func (s *Service) applyPriced(auth codex.Auth, epoch uint64, stamp priceStamp, fn func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isCurrentLocked(auth, epoch) || !s.pricesMatchLocked(stamp) {
		return false
	}
	fn()
	s.publishLocked()
	return true
}

// PricingChanged invalidates only price-derived data, independently of account generations.
func (s *Service) PricingChanged(version string) {
	s.mu.Lock()
	if version == s.pricingVersion {
		s.mu.Unlock()
		return
	}
	s.pricingVersion = version
	s.pricingGeneration++
	s.pricingPending = true
	s.cost, s.disp = nil, nil
	s.lastCostDone = time.Time{}
	s.detail.clear()
	for _, f := range s.detailFlight {
		f.cancel()
	}
	s.detailFlight = map[string]*flight{}
	s.publishLocked()
	if s.pricingWorker {
		s.mu.Unlock()
		return
	}
	s.pricingWorker = true
	s.mu.Unlock()
	if !s.spawn(s.repriceLoop) {
		s.mu.Lock()
		s.pricingWorker = false
		s.mu.Unlock()
	}
}
func (s *Service) repriceLoop() {

	for {
		if s.root.Err() != nil {
			s.mu.Lock()
			s.pricingWorker = false
			s.mu.Unlock()
			return
		}
		s.mu.Lock()
		if !s.pricingPending {
			s.pricingWorker = false
			s.mu.Unlock()
			return
		}
		if s.allBusy || s.busy[SectionAPICost] || s.busy[SectionRecentSessions] {
			s.mu.Unlock()
			select {
			case <-s.root.Done():
				s.mu.Lock()
				s.pricingWorker = false
				s.mu.Unlock()
				return
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}
		s.pricingPending = false
		s.busy[SectionAPICost] = true
		stamp := s.priceStampLocked()
		epoch := s.epoch
		s.mu.Unlock()
		s.repriceLocal(s.root, stamp, epoch)
		s.mu.Lock()
		delete(s.busy, SectionAPICost)
		s.publishLocked()
		s.mu.Unlock()
		s.loadRecentSessions(s.root)
		s.mu.Lock()
		s.exportStatusLocked()
		s.mu.Unlock()
	}
}

// repriceLocal reuses known ranges and the local index, without requesting quota or credentials.
func (s *Service) repriceLocal(ctx context.Context, stamp priceStamp, epoch uint64) {
	s.mu.Lock()
	now := s.now()
	rng := s.prefs.APICostSummaryRange
	var current *usage.Window
	if s.fullWindow != nil {
		w := *s.fullWindow
		w.End = minTimeForPrice(w.End, now)
		current = &w
	}
	var selected *usage.CostRange
	if r, ok := selectedCostRange(rng, s.fullWindow, now, s.opts.Location); ok {
		selected = &r
	}
	queries := costQueries(current, selected)
	s.mu.Unlock()
	if len(queries) == 0 || s.src.ScanCost == nil {
		return
	}
	result, err := s.src.ScanCost(ctx, queries, now, usage.RefreshIfChanged, nil)
	s.mu.Lock()
	defer s.mu.Unlock()
	if epoch != s.epoch || !s.pricesMatchLocked(stamp) || ctx.Err() != nil {
		return
	}
	if err != nil {
		s.costErrText = s.costErrorTextLocked(err)
		s.publishLocked()
		return
	}
	if c := summaryPtr(result, queryCurrent); c != nil {
		s.applyCurrentLocked(*c)
		s.detail.put(detailCacheKey(usage.RangeForWindow(c.Window, s.opts.Location)), *c)
		if cache := s.costCache(); cache != nil {
			if err := cache.Save(*c); err != nil {
				s.costNote = s.costErrorTextLocked(err)
			}
		}
	}
	shown := summaryPtr(result, querySelected)
	if shown == nil {
		shown = summaryPtr(result, queryCurrent)
	}
	if shown != nil {
		s.disp = shown
		s.dispRange = rng
	}
	s.lastCostDone = now
	s.publishLocked()
}
func minTimeForPrice(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
