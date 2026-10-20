package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

const cacheSummaryTTL = 5 * time.Minute

type panelCacheQueryState struct {
	cancel            context.CancelFunc
	generation        uint64
	task              Task
	report            *cachehit.Report
	query             *CostQuery
	provider          store.Provider
	account           string
	accountGeneration uint64
	day               string
	loadedAt          time.Time
}

type panelCacheState struct {
	summary      panelCacheQueryState
	detail       panelCacheQueryState
	selected     int
	initialized  bool
	start, end   time.Time
	showSessions bool
}

func (s *panelCacheQueryState) isToday() bool {
	return s.query != nil && s.query.Custom == nil && s.query.Range == store.CostRangeToday
}

func (s *panelCacheQueryState) queryValue() CostQuery {
	if s.query == nil {
		return CostQuery{Range: store.CostRangeToday}
	}
	return *s.query
}

func (s *panelCacheState) cancelAll() {
	if s.summary.cancel != nil {
		s.summary.cancel()
	}
	s.summary.cancel = nil
	s.summary.generation++
	s.summary.task.Running = false
	if s.detail.cancel != nil {
		s.detail.cancel()
	}
	s.detail = panelCacheQueryState{generation: s.detail.generation + 1}
	s.initialized = false
}

func (a *App) cacheContextMatches(s *panelCacheQueryState) bool {
	p := a.settings.Prefs().SelectedProvider
	snap := a.Snapshot()
	return s.provider == p && s.account == activePanelAccount(snap, p) && s.accountGeneration == activePanelGeneration(snap, p)
}

func (a *App) startCacheQuery(s *panelCacheQueryState, q CostQuery, acts PanelActions) {
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	gen := s.generation
	s.report, s.task = nil, Task{Running: true}
	query := q
	s.query = &query
	if acts.QueryCache == nil {
		s.task = Task{Err: errors.New(tx(a.L(), xDetailUnavailable))}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.provider = a.panelState.provider
	s.account = a.panelState.account
	s.accountGeneration = a.panelState.accountGeneration
	if q.Custom == nil && q.Range == store.CostRangeToday {
		s.day = time.Now().In(time.Local).Format("2006-01-02")
	}
	provider, account, accountGeneration := s.provider, s.account, s.accountGeneration
	go func() {
		report, err := acts.QueryCache(ctx, provider, q)
		runOnMain(func() {
			current := s
			snap := a.Snapshot()
			if ctx.Err() != nil || current.generation != gen || a.settings.Prefs().SelectedProvider != provider || activePanelAccount(snap, provider) != account || activePanelGeneration(snap, provider) != accountGeneration {
				return
			}
			current.cancel = nil
			current.task = Task{Err: err}
			if err == nil {
				copy := report.Clone()
				current.report = &copy
				current.loadedAt = time.Now()
			}
			a.redrawPanel()
		})
	}()
}

func (a *App) startCacheSummary(acts PanelActions) {
	a.startCacheQuery(&a.panelState.cache.summary, CostQuery{Range: store.CostRangeToday}, acts)
}

func (a *App) startCacheDetail(q CostQuery, acts PanelActions) {
	a.startCacheQuery(&a.panelState.cache.detail, q, acts)
}

func (a *App) syncCacheSummary(acts PanelActions) {
	s := &a.panelState.cache.summary
	today := time.Now().In(time.Local).Format("2006-01-02")
	if s.task.Running {
		return
	}
	if s.query != nil && s.day == today && a.cacheContextMatches(s) {
		if s.task.Err != nil || s.report != nil && !s.loadedAt.IsZero() && time.Since(s.loadedAt) < cacheSummaryTTL {
			return
		}
	}
	if acts.QueryCache != nil {
		a.startCacheSummary(acts)
	}
}

func (a *App) syncCacheDetail(acts PanelActions) {
	s := &a.panelState.cache
	if s.initialized {
		return
	}
	s.selected = 0
	s.end = time.Now()
	s.start = s.end.AddDate(0, 0, -6)
	if summary := &s.summary; summary.isToday() && summary.day == time.Now().In(time.Local).Format("2006-01-02") && a.cacheContextMatches(summary) {
		switch {
		case summary.task.Running:
			s.detail.task.Running = true
			s.detail.query = &CostQuery{Range: store.CostRangeToday}
			return
		case summary.report != nil && !summary.loadedAt.IsZero() && time.Since(summary.loadedAt) < cacheSummaryTTL:
			copy := summary.report.Clone()
			s.detail = panelCacheQueryState{report: &copy, query: &CostQuery{Range: store.CostRangeToday}, provider: summary.provider, account: summary.account, accountGeneration: summary.accountGeneration, day: summary.day, loadedAt: summary.loadedAt}
			s.initialized = true
			return
		case summary.task.Err != nil:
			s.detail = panelCacheQueryState{task: Task{Err: summary.task.Err}, query: &CostQuery{Range: store.CostRangeToday}, provider: summary.provider, account: summary.account, accountGeneration: summary.accountGeneration, day: summary.day}
			s.initialized = true
			return
		}
	}
	s.initialized = true
	if acts.QueryCache != nil {
		a.startCacheDetail(CostQuery{Range: store.CostRangeToday}, acts)
	}
}

func (a *App) selectCacheRange(i int, acts PanelActions) {
	s := &a.panelState.cache
	// A range click is an explicit detail choice. Stop the initial wait for a
	// pending Today summary so its completion cannot select Today again.
	s.initialized = true
	s.selected = i
	if i < costCustomIndex {
		a.startCacheDetail(CostQuery{Range: store.CostRanges()[i]}, acts)
		return
	}
	if s.detail.cancel != nil {
		s.detail.cancel()
	}
	s.detail = panelCacheQueryState{generation: s.detail.generation + 1}
}

func (a *App) calculateCustomCache(acts PanelActions) {
	s := &a.panelState.cache
	r, ok := usage.CustomRange(s.start, s.end, time.Local)
	if !ok || s.end.After(time.Now()) {
		if s.detail.cancel != nil {
			s.detail.cancel()
		}
		s.detail = panelCacheQueryState{generation: s.detail.generation + 1, task: Task{Err: errInvalidCostRange}}
		return
	}
	a.startCacheDetail(CostQuery{Custom: &r}, acts)
}
