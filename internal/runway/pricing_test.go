package runway

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func pricedResult(queries []usage.Query, version string, tokens int64) map[string]usage.Summary {
	out := map[string]usage.Summary{}
	usd := decimal.FromInt64(tokens)
	for _, q := range queries {
		out[q.ID] = usage.Summary{Source: usage.SourceLocalSessions, Confidence: usage.ConfidencePriced, Window: q.Window, Totals: usage.Totals{TotalTokens: tokens}, EstimatedUSD: &usd, PricingVersion: version, DailyRows: []usage.DailyRow{}, ModelRows: []usage.BreakdownRow{}, ProjectRows: []usage.BreakdownRow{}, ClientRows: []usage.BreakdownRow{}, Warnings: []string{}, CalculatedAt: time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)}
	}
	return out
}
func TestPriceChangeDropsOldDetailAndReusesNewCache(t *testing.T) {
	f := newFake(t)
	gate := newGate()
	var version atomic.Value
	version.Store(usage.PricingVersion + "+models-dev-v1")
	f.scanFn = func(_ context.Context, qs []usage.Query) (map[string]usage.Summary, error) {
		v := version.Load().(string)
		if v == usage.PricingVersion+"+models-dev-v1" {
			gate.wait()
		}
		return pricedResult(qs, v, 22), nil
	}
	s, _ := newService(t, f, func(o *Options) { o.PricingVersion = func() string { return version.Load().(string) } })
	rng := usage.Today(f.clk.Now(), time.UTC)
	oldDone := make(chan error, 1)
	go func() { _, err := s.QueryCost(context.Background(), rng); oldDone <- err }()
	gate.waitStarted(t)
	epoch := s.Epoch()
	version.Store(usage.PricingVersion + "+models-dev-v2")
	s.PricingChanged(usage.PricingVersion + "+models-dev-v2")
	fresh, err := s.QueryCost(context.Background(), rng)
	if err != nil || fresh.PricingVersion != usage.PricingVersion+"+models-dev-v2" {
		t.Fatalf("fresh detail %+v %v", fresh, err)
	}
	gate.open()
	if err := <-oldDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("old detail succeeded %v", err)
	}
	settle(t, s)
	before := f.count("scan")
	again, err := s.QueryCost(context.Background(), rng)
	if err != nil || again.PricingVersion != usage.PricingVersion+"+models-dev-v2" || f.count("scan") != before {
		t.Fatal("old flight poisoned cache")
	}
	if s.Epoch() != epoch {
		t.Fatal("price change changed account epoch")
	}
}
func TestPriceChangeWhileCostBusyEventuallyRepricesWithoutQuota(t *testing.T) {
	f := newFake(t)
	gate := newGate()
	var version atomic.Value
	version.Store(usage.PricingVersion + "+models-dev-v1")
	f.scanFn = func(_ context.Context, qs []usage.Query) (map[string]usage.Summary, error) {
		v := version.Load().(string)
		if v == usage.PricingVersion+"+models-dev-v1" {
			gate.wait()
		}
		return pricedResult(qs, v, 33), nil
	}
	path := t.TempDir() + "/cost.json"
	s, _ := newService(t, f, func(o *Options) {
		o.PricingVersion = func() string { return version.Load().(string) }
		o.CostCachePath = path
	})
	old := s.RefreshCost(usage.RefreshForce)
	gate.waitStarted(t)
	quotaCalls := f.count("quota")
	version.Store(usage.PricingVersion + "+models-dev-v2")
	s.PricingChanged(usage.PricingVersion + "+models-dev-v2")
	gate.open()
	wait(t, old)
	settle(t, s)
	got := s.Snapshot().Codex.Cost
	if got == nil || got.PricingVersion != usage.PricingVersion+"+models-dev-v2" {
		t.Fatalf("pending repricing lost %+v", got)
	}
	if f.count("quota") != quotaCalls {
		t.Fatal("repricing fetched quota")
	}
	cache := usage.CacheStore{Path: path}.Load()
	if cache.Summary.PricingVersion != usage.PricingVersion+"+models-dev-v2" {
		t.Fatalf("old cost saved in cache %+v", cache)
	}
}
func TestRepeatedPricingChangesLeaveNoPendingWorker(t *testing.T) {
	f := newFake(t)
	var version atomic.Value
	version.Store(usage.PricingVersion + "+models-dev-v0")
	f.scanFn = func(_ context.Context, qs []usage.Query) (map[string]usage.Summary, error) {
		return pricedResult(qs, version.Load().(string), 44), nil
	}
	s, _ := newService(t, f, func(o *Options) { o.PricingVersion = func() string { return version.Load().(string) } })
	s.mu.Lock()
	w := usage.Today(f.clk.Now(), time.UTC).Window
	s.fullWindow = &w
	s.mu.Unlock()
	for n := 1; n <= 100; n++ {
		v := usage.PricingVersion + "+models-dev-v" + strconv.Itoa(n)
		version.Store(v)
		s.PricingChanged(v)
		if n%5 == 0 {
			settle(t, s)
		}
	}
	settle(t, s)
	s.mu.Lock()
	pending, worker := s.pricingPending, s.pricingWorker
	s.mu.Unlock()
	if pending || worker {
		t.Fatalf("stranded pending=%v worker=%v", pending, worker)
	}
	if c := s.Snapshot().Codex.Cost; c == nil || c.PricingVersion != usage.PricingVersion+"+models-dev-v100" {
		t.Fatalf("latest update lost %+v", c)
	}
}
