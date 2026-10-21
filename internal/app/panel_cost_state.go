package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

type panelCostState struct {
	selected    int
	initialized bool
	start, end  time.Time
	cancel      context.CancelFunc
	generation  uint64
	task        Task
	summary     *usage.Summary
	sourceCost  *usage.Summary
	query       *CostQuery
}

func (a *App) cancelPanelDetails() {
	s := &a.panelState
	if s.cost.cancel != nil {
		s.cost.cancel()
	}
	if s.reaction.cancel != nil {
		s.reaction.cancel()
	}
	s.cost = panelCostState{generation: s.cost.generation + 1}
	s.cache.cancelAll()
	s.reaction = panelReactionState{generation: s.reaction.generation + 1}
	s.skills.closeItem() // the scan stays cached; it covers every provider
}

func (a *App) redrawPanel() {
	if a.panel != nil {
		a.panel.Invalidate()
	}
}

func (a *App) startCostQuery(q CostQuery, acts PanelActions) {
	s := &a.panelState.cost
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	gen := s.generation
	s.summary, s.task = nil, Task{Running: true}
	query := q
	s.query = &query
	if acts.QueryCost == nil {
		s.task = Task{Err: errors.New(tx(a.L(), xDetailUnavailable))}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	provider, account := a.panelState.provider, a.panelState.account
	if provider == store.ProviderCodex {
		s.sourceCost = a.Snapshot().Codex.Cost
	}
	accountGeneration := a.panelState.accountGeneration
	go func() {
		sum, err := acts.QueryCost(ctx, provider, q)
		runOnMain(func() {
			current := a.Snapshot()
			if ctx.Err() != nil || a.panelState.cost.generation != gen || a.settings.Prefs().SelectedProvider != provider || activePanelAccount(current, provider) != account || activePanelGeneration(current, provider) != accountGeneration {
				return
			}
			s.task = Task{Err: err}
			if err == nil {
				copy := sum.Clone()
				s.summary = &copy
			}
			a.redrawPanel()
		})
	}()
}

func activePanelAccount(s runway.Snapshot, p store.Provider) string {
	switch p {
	case store.ProviderCursor:
		return s.Cursor.IdentityID
	case store.ProviderGrok:
		return s.Grok.Accounts.ActiveID
	case store.ProviderClaude:
		return s.Claude.Accounts.ActiveID
	default:
		return s.Accounts.ActiveID
	}
}

func activePanelGeneration(s runway.Snapshot, p store.Provider) uint64 {
	if p == store.ProviderCodex {
		return s.Codex.Generation
	}
	return 0
}

// Core's cycle summary can change after the header's immediate detail query
// reused the previous value; cancel that result before showing the new one.
func (s *panelCostState) acceptCurrentCost(cost *usage.Summary) {
	if cost == nil || cost == s.sourceCost {
		return
	}
	s.sourceCost = cost
	if !cost.IsDisplayableCost() || !cost.Window.End.After(cost.Window.Start) {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	s.task = Task{}
	fresh := cost.Clone()
	s.summary = &fresh
}

func detailErrorText(l l10n.Localizer, err error) string {
	switch {
	case errors.Is(err, errDetailUnavailable):
		return tx(l, xDetailUnavailable)
	case errors.Is(err, errGrokCycleUnavailable):
		return tx(l, xGrokCycleUnavailable)
	case errors.Is(err, runway.ErrCycleUnavailable):
		return tx(l, xCacheCodexCycleMissing)
	case errors.Is(err, errClaudeCycleUnavailable):
		return tx(l, xCacheClaudeCycleMissing)
	case errors.Is(err, errInvalidCacheProvider):
		return tx(l, xCacheProviderUnavailable)
	case errors.Is(err, errInvalidCostRange):
		return l.Text(l10n.KeyInvalidDateRange)
	default:
		return err.Error()
	}
}
