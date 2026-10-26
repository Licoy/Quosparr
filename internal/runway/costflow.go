package runway

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// errHeatmap marks a handled profile failure inside the heatmap step.
var errHeatmap = errors.New("runway: token heatmap unavailable")

// ErrCycleUnavailable means the shown subscription did not report a usable
// billing window. Callers must not invent one for range-specific analysis.
var ErrCycleUnavailable = errors.New("runway: billing cycle unavailable")

func decimalFloat(d decimal.Decimal) float64 {
	f, _ := strconv.ParseFloat(d.String(), 64)
	return f
}

func summaryPtr(m map[string]usage.Summary, id string) *usage.Summary {
	if sum, ok := m[id]; ok {
		return &sum
	}
	return nil
}

// ---- progress ----------------------------------------------------------------

func (s *Service) beginProgress() {
	s.mu.Lock()
	s.progressCount++
	if s.progressCount == 1 {
		s.progress = usage.ProgressPreparing()
		s.publishLocked()
	}
	s.mu.Unlock()
}

func (s *Service) endProgress() {
	s.mu.Lock()
	if s.progressCount > 0 {
		s.progressCount--
	}
	if s.progressCount == 0 {
		s.progress = usage.ProgressIdle()
		s.publishLocked()
	}
	s.mu.Unlock()
}

func (s *Service) reportProgress(p usage.Progress) { s.progressReports.Report(p, true) }

// ---- scanning ---------------------------------------------------------------------

// localCost summarizes the local logs; scan errors other than cancellation
// fall back to "no local data" so the online source can still answer.
func (s *Service) localCost(ctx context.Context, queries []usage.Query, now time.Time, policy usage.RefreshPolicy, reporter *usage.ProgressReporter) (map[string]usage.Summary, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	m, err := s.src.ScanCost(ctx, queries, now, policy, reporter)
	if err != nil {
		if stopped(ctx, err) {
			return nil, context.Canceled
		}
		return nil, nil
	}
	return m, nil
}

// resolveCost picks the local summary when it has usage, otherwise asks the
// online analytics, falling back to whatever exists.
func (s *Service) resolveCost(ctx context.Context, local *usage.Summary, r usage.CostRange, auth codex.Auth, now time.Time) (usage.Summary, error) {
	if ctx.Err() != nil {
		return usage.Summary{}, context.Canceled
	}
	if local != nil && local.IsDisplayableCost() {
		return *local, nil
	}
	if auth.IsAPIKey() {
		// No online analytics exist for an API key: the local logs are all.
		if local != nil {
			return *local, nil
		}
		return usage.Summary{}, ErrUsageUnavailable
	}
	s.reportProgress(usage.ProgressFetchingOnline())
	sum, err := s.src.FetchDailyUsage(ctx, apiAuth(auth), r.APIStartDate, r.APIEndDate, r.Window, now)
	if stopped(ctx, err) && (err != nil || ctx.Err() != nil) {
		return usage.Summary{}, context.Canceled
	}
	if err != nil {
		if local != nil {
			return *local, nil
		}
		return usage.Summary{}, ErrUsageUnavailable
	}
	if sum.IsDisplayableCost() || local == nil {
		return sum, nil
	}
	return *local, nil
}

func (s *Service) resolveDisplayed(ctx context.Context, rng store.CostRange, sel *usage.CostRange, current, local *usage.Summary, auth codex.Auth, now time.Time) (usage.Summary, error) {
	if rng == store.CostRangeCurrent {
		if current == nil {
			return usage.Summary{}, ErrUsageUnavailable
		}
		return *current, nil
	}
	if rng == store.CostRangePrevious && current == nil {
		return usage.Summary{}, ErrUsageUnavailable
	}
	if sel == nil {
		return usage.Summary{}, ErrUsageUnavailable
	}
	return s.resolveCost(ctx, local, *sel, auth, now)
}

func (s *Service) costErrorTextLocked(err error) string {
	if errors.Is(err, ErrUsageUnavailable) {
		return s.lc.Text(l10n.KeyUsageAnalyticsUnavailable)
	}
	return s.errTextLocked(err)
}

func (s *Service) clearDisplayedLocked(text string) {
	s.disp, s.dispRange, s.costNote, s.costErrText = nil, "", "", text
}

// applyCurrentLocked stores the current-cycle summary; a new pricing version
// drops the detail cache.
func (s *Service) applyCurrentLocked(sum usage.Summary) {
	if s.cost != nil && s.cost.PricingVersion != sum.PricingVersion {
		s.detail.clear()
	}
	s.cost = &sum
}

func (s *Service) saveCostCache(sum usage.Summary) {
	if c := s.costCache(); c != nil && sum.IsDisplayableCost() {
		_ = c.Save(sum)
	}
}

// scanCostAndHeatmap is Swift scanCostAndHeatmap: local summaries first, then
// the online sources; every step is dropped when the account changed.
func (s *Service) scanCostAndHeatmap(ctx context.Context, quota codexapi.QuotaSnapshot, auth codex.Auth, gen uint64, policy usage.RefreshPolicy, includeCost, includeHeat bool) {
	if !s.current(auth, gen) {
		return
	}
	now := s.now()
	s.mu.Lock()
	rng, loc := s.prefs.APICostSummaryRange, s.opts.Location
	price := s.priceStampLocked()
	s.mu.Unlock()

	var fullP, elapsedP *usage.Window
	if f, e, ok := cycleFor(auth, quota, now, loc); ok {
		fullP, elapsedP = &f, &e
	}
	s.mu.Lock()
	if s.isCurrentLocked(auth, gen) {
		s.fullWindow = fullP
	}
	s.mu.Unlock()
	var sel *usage.CostRange
	if r, ok := selectedCostRange(rng, fullP, now, loc); ok {
		sel = &r
	}
	var queries []usage.Query
	if includeCost {
		queries = costQueries(elapsedP, sel)
	}
	if includeHeat {
		queries = append(queries, usage.Query{ID: queryHeatmap, Window: usage.UTCYearToDateWindow(now), DayBoundary: usage.DayUTC})
	}
	if len(queries) == 0 {
		return
	}
	s.beginProgress()
	defer s.endProgress()
	s.reportProgress(usage.ProgressPreparing())

	err := s.scanSteps(ctx, auth, gen, now, scanPlan{
		pricing: price, policy: policy, queries: queries, rng: rng, loc: loc, elapsed: elapsedP, sel: sel,
		cost: includeCost, heat: includeHeat,
	})
	if err == nil || stopped(ctx, err) || errors.Is(err, errHeatmap) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isCurrentLocked(auth, gen) {
		return
	}
	if includeCost {
		text := s.costErrorTextLocked(err)
		if s.disp != nil && s.dispRange == rng {
			s.costNote = text
		} else {
			s.clearDisplayedLocked(text)
		}
	}
	if includeHeat {
		if len(s.heat.AllDevices) == 0 && len(s.heat.Local) == 0 {
			s.heat.CalculatedAt = now
		}
		s.lastErr = s.lc.Text(l10n.KeyTokenUsageHeatmapUnavailable)
	}
	s.publishLocked()
}

type scanPlan struct {
	pricing    priceStamp
	policy     usage.RefreshPolicy
	queries    []usage.Query
	rng        store.CostRange
	loc        *time.Location
	elapsed    *usage.Window
	sel        *usage.CostRange
	cost, heat bool
}

func (s *Service) scanSteps(ctx context.Context, auth codex.Auth, gen uint64, now time.Time, p scanPlan) error {
	local, err := s.localCost(ctx, p.queries, now, p.policy, s.progressReports)
	if err != nil {
		return err
	}
	if !s.current(auth, gen) {
		return context.Canceled
	}
	if p.cost {
		if err := s.costStep(ctx, auth, gen, now, p, local); err != nil {
			return err
		}
	}
	if p.heat {
		if ctx.Err() != nil {
			return context.Canceled
		}
		lm := map[string]int64{}
		if sum := summaryPtr(local, queryHeatmap); sum != nil {
			lm = dailyTokenMap(*sum)
		}
		return s.heatmapStep(ctx, auth, gen, lm, now)
	}
	return nil
}

func (s *Service) costStep(ctx context.Context, auth codex.Auth, gen uint64, now time.Time, p scanPlan, local map[string]usage.Summary) error {
	var current *usage.Summary
	var currentKey string
	if p.elapsed != nil {
		cr := usage.RangeForWindow(*p.elapsed, p.loc)
		currentKey = detailCacheKey(cr)
		sum, err := s.resolveCost(ctx, summaryPtr(local, queryCurrent), cr, auth, now)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err == nil {
			current = &sum
		}
	}
	if !s.current(auth, gen) {
		return context.Canceled
	}
	if current != nil {
		sum := *current
		if !s.applyPriced(auth, gen, p.pricing, func() {
			s.applyCurrentLocked(sum)
			s.detail.put(currentKey, sum)
		}) {
			return context.Canceled
		}
		s.mu.Lock()
		if s.pricesMatchLocked(p.pricing) {
			s.saveCostCache(sum)
		}
		s.mu.Unlock()
	}
	shown, err := s.resolveDisplayed(ctx, p.rng, p.sel, current, summaryPtr(local, querySelected), auth, now)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return context.Canceled
	}
	if !s.applyPriced(auth, gen, p.pricing, func() {
		s.disp, s.dispRange, s.costNote, s.costErrText = &shown, p.rng, "", ""
		if p.sel != nil {
			s.detail.put(detailCacheKey(*p.sel), shown)
		}
	}) {
		return context.Canceled
	}
	return nil
}

// heatmapStep applies the local series, then the account-wide profile series.
// A profile failure keeps the previous grid and is reported as errHeatmap.
func (s *Service) heatmapStep(ctx context.Context, auth codex.Auth, gen uint64, local map[string]int64, now time.Time) error {
	if !s.apply(auth, gen, func() {
		s.heat = HeatmapData{
			AllDevices: s.heat.AllDevices, Local: local, CalculatedAt: now,
			OfficialStatsAsOf: s.heat.OfficialStatsAsOf, OfficialGeneratedAt: s.heat.OfficialGeneratedAt,
		}
	}) {
		return context.Canceled
	}
	if auth.IsAPIKey() {
		return nil // no account-wide series exists for an API key
	}
	s.reportProgress(usage.ProgressFetchingOnline())
	prof, err := s.src.FetchProfileTokens(ctx, apiAuth(auth))
	if err != nil && stopped(ctx, err) {
		return context.Canceled
	}
	if err == nil && ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		s.apply(auth, gen, func() {
			s.lastErr = s.lc.Text(l10n.KeyTokenUsageHeatmapUnavailable)
			if len(s.heat.AllDevices) == 0 && len(s.heat.Local) == 0 {
				s.heat.CalculatedAt = now
			}
		})
		return errHeatmap
	}
	if !s.apply(auth, gen, func() {
		s.heat = HeatmapData{
			AllDevices: prof.DailyTokens, Local: local, CalculatedAt: now,
			OfficialStatsAsOf: prof.StatsAsOf, OfficialGeneratedAt: prof.GeneratedAt,
		}
	}) {
		return context.Canceled
	}
	return nil
}

// ---- standalone cost and heatmap refreshes -------------------------------------

// RefreshCost rescans the API-equivalent cost alone.
func (s *Service) RefreshCost(policy usage.RefreshPolicy) <-chan struct{} {
	s.mu.Lock()
	skip := s.allBusy || s.busy[SectionAPICost] ||
		!shouldRefreshCost(policy, s.lastCostDone, s.lastCostCycle, nil, s.refreshInterval(), s.now())
	s.mu.Unlock()
	if skip {
		return closedChan()
	}
	return s.runAccountScoped(func(ctx context.Context) {
		busyEpoch := s.Epoch()
		if !s.enter(SectionAPICost) {
			return
		}
		defer s.leaveAccount(busyEpoch, SectionAPICost)
		s.refreshCostNow(ctx, policy)
		s.mu.Lock()
		s.exportStatusLocked()
		s.mu.Unlock()
	})
}

func (s *Service) refreshCostNow(ctx context.Context, policy usage.RefreshPolicy) {
	auth, gen, err := s.loadValidAuth(ctx, true)
	if err != nil {
		return
	}
	q, ok := s.cycleQuota(ctx, auth, gen)
	if !ok {
		return
	}
	s.scanCostAndHeatmap(ctx, q, auth, gen, policy, true, false)
	s.mu.Lock()
	if s.isCurrentLocked(auth, gen) {
		s.lastCostDone, s.lastCostCycle, s.lastErr = s.now(), cycleOf(q), ""
		s.publishLocked()
	}
	s.mu.Unlock()
}

// cycleQuota is the quota the cost cycle comes from: the one on hand, else a
// fetched one. An API key has no quota (and no request is made): its cost uses
// the calendar month. ok is false when the cost refresh must stop.
func (s *Service) cycleQuota(ctx context.Context, auth codex.Auth, gen uint64) (codexapi.QuotaSnapshot, bool) {
	if auth.IsAPIKey() {
		return codexapi.QuotaSnapshot{}, true
	}
	s.mu.Lock()
	cached := s.quota
	s.mu.Unlock()
	if cached != nil {
		return *cached, true
	}
	q, err := s.src.FetchQuota(ctx, apiAuth(auth))
	if err != nil {
		if !stopped(ctx, err) {
			s.fail(&auth, gen, err, func() {
				text := s.errTextLocked(err)
				if s.disp != nil {
					s.costNote = text
				} else {
					s.clearDisplayedLocked(s.lc.Text(l10n.KeyUsageAnalyticsUnavailable))
				}
				s.lastErr = text
			})
		}
		return q, false
	}
	return q, s.apply(auth, gen, func() { s.quota, s.quotaErr, s.statusFail = &q, "", false })
}

// RefreshTokenHeatmap rescans the token usage chart alone. It can be
// cancelled by an account switch.
func (s *Service) RefreshTokenHeatmap(policy usage.RefreshPolicy) <-chan struct{} {
	s.mu.Lock()
	if !(s.prefs.ShowsTokenUsageHeatmap || s.widgetReqs.Has(export.RequireTokenTrend)) ||
		s.allBusy || s.busy[SectionTokenHeatmap] ||
		!shouldRefreshHeatmap(policy, s.lastHeatDone, s.refreshInterval(), s.now()) {
		s.mu.Unlock()
		return closedChan()
	}
	ctx, cancel := context.WithCancel(s.root)
	s.heatID++
	id := s.heatID
	done := make(chan struct{})
	s.heatCancel, s.heatDone = cancel, done
	s.busy[SectionTokenHeatmap] = true
	s.publishLocked()
	s.mu.Unlock()

	finish := func() {
		cancel()
		s.mu.Lock()
		if s.heatID == id {
			s.heatCancel, s.heatDone = nil, nil
			delete(s.busy, SectionTokenHeatmap)
			s.publishLocked()
		}
		s.mu.Unlock()
	}
	if !s.spawn(func() {
		defer close(done)
		s.refreshHeatmapNow(ctx, policy)
		finish()
	}) {
		finish()
		close(done)
	}
	return done
}

func (s *Service) refreshHeatmapNow(ctx context.Context, policy usage.RefreshPolicy) {
	auth, gen, err := s.loadValidAuth(ctx, true)
	if err != nil {
		if !stopped(ctx, err) && !IsAuthFailure(err) {
			s.markHeatmapFailed(gen)
		}
		return
	}
	now := s.now()
	local, err := s.localCost(ctx, []usage.Query{{ID: queryHeatmap, Window: usage.UTCYearToDateWindow(now), DayBoundary: usage.DayUTC}}, now, policy, s.progressReports)
	if err != nil {
		return
	}
	lm := map[string]int64{}
	if sum := summaryPtr(local, queryHeatmap); sum != nil {
		lm = dailyTokenMap(*sum)
	}
	if !s.current(auth, gen) || ctx.Err() != nil {
		return
	}
	err = s.heatmapStep(ctx, auth, gen, lm, now)
	if err != nil {
		return // cancelled, or already reported by the step
	}
	s.mu.Lock()
	if s.isCurrentLocked(auth, gen) {
		s.lastHeatDone, s.lastErr = s.now(), ""
		s.publishLocked()
	}
	s.mu.Unlock()
}

// markHeatmapFailed notes a heatmap failure that happened before a
// credential was available.
func (s *Service) markHeatmapFailed(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.epoch {
		return
	}
	if len(s.heat.AllDevices) == 0 && len(s.heat.Local) == 0 {
		s.heat.CalculatedAt = s.now()
	}
	s.lastErr = s.lc.Text(l10n.KeyTokenUsageHeatmapUnavailable)
	s.publishLocked()
}

// ---- detail queries (cost detail page) ---------------------------------------------

// QueryCost returns the cost of a range, cached (FIFO, 6 ranges) and shared
// between concurrent callers. The scan itself is not cancelled when a caller
// gives up, so its result still lands in the cache.
func (s *Service) QueryCost(ctx context.Context, r usage.CostRange) (usage.Summary, error) {
	key := detailCacheKey(r)
	s.mu.Lock()
	stamp := s.priceStampLocked()
	if sum, ok := s.detail.get(key); ok && (s.opts.PricingVersion == nil || sum.Source != usage.SourceLocalSessions || sum.PricingVersion == stamp.version) {
		s.mu.Unlock()
		return sum, nil
	}
	f := s.detailFlight[key]
	if f == nil || f.epoch != s.epoch || f.pricing != stamp {
		workCtx, cancel := context.WithCancel(s.root)
		f = &flight{done: make(chan struct{}), epoch: s.epoch, cancel: cancel, pricing: stamp}
		s.detailFlight[key] = f
		epoch := s.epoch
		if !s.spawn(func() { s.runDetail(workCtx, key, r, f, epoch) }) {
			delete(s.detailFlight, key)
			cancel()
			s.mu.Unlock()
			return usage.Summary{}, context.Canceled
		}
	}
	s.mu.Unlock()
	select {
	case <-f.done:
		return f.sum, f.err
	case <-ctx.Done():
		return usage.Summary{}, ctx.Err()
	}
}

func (s *Service) runDetail(ctx context.Context, key string, r usage.CostRange, f *flight, epoch uint64) {
	defer f.cancel()
	sum, err := s.performDetail(ctx, r)
	s.mu.Lock()
	if s.detailFlight[key] == f {
		delete(s.detailFlight, key)
	}
	if epoch != s.epoch || !s.pricesMatchLocked(f.pricing) || ctx.Err() != nil {
		sum, err = usage.Summary{}, context.Canceled
	}
	if err == nil && epoch == s.epoch && s.pricesMatchLocked(f.pricing) {
		s.detail.put(key, sum)
	}
	f.sum, f.err = sum, err
	close(f.done)
	s.mu.Unlock()
}

func (s *Service) performDetail(ctx context.Context, r usage.CostRange) (usage.Summary, error) {
	if !r.Window.End.After(r.Window.Start) {
		return usage.Summary{}, ErrUsageUnavailable
	}
	s.beginProgress()
	defer s.endProgress()
	auth, _, err := s.loadValidAuth(ctx, true)
	if err != nil {
		return usage.Summary{}, err
	}
	now := s.now()
	q := usage.Query{ID: queryDetail, Window: r.Window}
	m, err := s.localCost(ctx, []usage.Query{q}, now, usage.RefreshIfChanged, s.progressReports)
	if err != nil {
		return usage.Summary{}, err
	}
	return s.resolveCost(ctx, summaryPtr(m, queryDetail), r, auth, now)
}

func windowMatches(got, want usage.Window) bool {
	return abs(got.Start.Sub(want.Start)) < 60*time.Second && abs(got.End.Sub(want.End)) < 120*time.Second
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// QueryCurrentCycleCost returns the cost of the current quota cycle so far,
// reusing the in-memory summary when its window still matches.
func (s *Service) QueryCurrentCycleCost(ctx context.Context) (usage.Summary, error) {
	_, elapsed, err := s.ensureCycleWindows(ctx, s.now())
	if err != nil {
		return usage.Summary{}, err
	}
	r := usage.RangeForWindow(elapsed, s.opts.Location)
	s.mu.Lock()
	if c := s.cost; c != nil && c.IsDisplayableCost() && windowMatches(c.Window, r.Window) && (s.opts.PricingVersion == nil || c.Source != usage.SourceLocalSessions || c.PricingVersion == s.priceStampLocked().version) {
		sum := *c
		s.mu.Unlock()
		return sum, nil
	}
	s.mu.Unlock()
	return s.QueryCost(ctx, r)
}

// PreviousCycleRange is the previous quota cycle when the current one is known.
func (s *Service) PreviousCycleRange() (usage.CostRange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fullWindow == nil {
		return usage.CostRange{}, false
	}
	return usage.PreviousCycle(*s.fullWindow, s.opts.Location), true
}

// ResolvePreviousCycleRange is PreviousCycleRange, fetching the quota first
// when the cycle is not known yet.
func (s *Service) ResolvePreviousCycleRange(ctx context.Context) (usage.CostRange, error) {
	if r, ok := s.PreviousCycleRange(); ok {
		return r, nil
	}
	full, _, err := s.ensureCycleWindows(ctx, s.now())
	if err != nil {
		return usage.CostRange{}, err
	}
	return usage.PreviousCycle(full, s.opts.Location), nil
}

// ResolveCycleRange returns the reported current cycle so far, or the full
// preceding cycle. API-key accounts use their established calendar-month
// cycle. Unlike cost display refreshes, this never invents a seven-day range.
func (s *Service) ResolveCycleRange(ctx context.Context, previous bool) (usage.CostRange, error) {
	full, elapsed, err := s.resolveCycleWindows(ctx, s.now(), false)
	if err != nil {
		return usage.CostRange{}, err
	}
	if previous {
		s.mu.Lock()
		apiKey := s.keyModeLocked()
		s.mu.Unlock()
		if apiKey {
			start := full.Start.In(s.opts.Location)
			previousStart := start.AddDate(0, -1, 0)
			return usage.RangeForWindow(usage.Window{Start: previousStart, End: start}, s.opts.Location), nil
		}
		return usage.PreviousCycle(full, s.opts.Location), nil
	}
	return usage.RangeForWindow(elapsed, s.opts.Location), nil
}

// ensureCycleWindows returns the quota cycle, fetching the quota when needed
// and falling back to the last 7 days when the quota has no cycle.
func (s *Service) ensureCycleWindows(ctx context.Context, now time.Time) (full, elapsed usage.Window, err error) {
	return s.resolveCycleWindows(ctx, now, true)
}

func (s *Service) resolveCycleWindows(ctx context.Context, now time.Time, allowFallback bool) (full, elapsed usage.Window, err error) {
	s.mu.Lock()
	if s.keyModeLocked() {
		f, e := monthWindows(now, s.opts.Location)
		s.fullWindow = &f
		s.mu.Unlock()
		return f, e, nil
	}
	if s.quota != nil {
		if f, e, ok := cycleWindows(*s.quota, now); ok {
			s.fullWindow = &f
			s.mu.Unlock()
			return f, e, nil
		}
	}
	s.mu.Unlock()
	s.beginProgress()
	defer s.endProgress()
	auth, gen, err := s.loadValidAuth(ctx, true)
	if err != nil {
		return usage.Window{}, usage.Window{}, err
	}
	if auth.IsAPIKey() {
		f, e := monthWindows(now, s.opts.Location)
		s.mu.Lock()
		s.fullWindow = &f
		s.mu.Unlock()
		return f, e, nil
	}
	q, err := s.src.FetchQuota(ctx, apiAuth(auth))
	if err != nil {
		if !stopped(ctx, err) {
			s.fail(&auth, gen, err, nil)
		}
		return usage.Window{}, usage.Window{}, err
	}
	if !s.apply(auth, gen, func() { s.quota, s.quotaErr, s.statusFail = &q, "", false }) {
		return usage.Window{}, usage.Window{}, context.Canceled
	}
	f, e, ok := cycleWindows(q, now)
	if !ok {
		if !allowFallback {
			return usage.Window{}, usage.Window{}, ErrCycleUnavailable
		}
		f = usage.Window{Start: now.Add(-7 * 24 * time.Hour), End: now}
		e = f
	}
	s.mu.Lock()
	s.fullWindow = &f
	s.mu.Unlock()
	return f, e, nil
}
