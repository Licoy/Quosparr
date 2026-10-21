package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// sectionBlock is the ruled section rhythm of the main page: an inset
// hairline above every section except the first, then 16pt of air (8pt on
// top of the first section).
func sectionBlock(c *ui.Context, first *bool, body func()) {
	top := float32(16)
	if *first {
		top, *first = 8, false
	} else {
		hairline(c)
	}
	ui.Column(c).Gap(8).Padding(top, 0, 16, 0).Children(body)
}

// panelBody follows the saved module order and provider support.
func panelBody(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions, state *panelState) {
	if prefs.SelectedProvider == store.ProviderCursor {
		first := true
		panelCursorQuota(c, l, prefs, snap.Cursor, acts, &first, state)
		return
	}
	snap.Provider = prefs.SelectedProvider
	snap.Codex = providerView(l, snap, prefs.APICostSummaryRange)
	grok := prefs.SelectedProvider == store.ProviderGrok
	claude := prefs.SelectedProvider == store.ProviderClaude
	local := grok || claude
	first := true
	for _, m := range store.NormalizeModuleOrder(prefs.MainPanelModuleOrder) {
		if local && m == store.ModuleResetCredits && snap.Codex.ResetCreditSummary == nil {
			continue
		}
		if panelModuleOn(m, prefs, local) {
			panelModule(c, l, m, prefs, snap, acts, &first, state)
		}
	}
}

func panelModuleOn(m store.Module, p store.Preferences, grok bool) bool {
	if grok && (m == store.ModuleQuotaEstimate || m == store.ModuleSessionRepair || m == store.ModuleRateLimitResetToday) {
		return false
	}
	switch m {
	case store.ModuleQuota:
		return p.ShowsQuotaSummary
	case store.ModuleTokenUsage:
		return p.ShowsTokenUsageHeatmap
	case store.ModuleSpeedAnalysis:
		return p.ShowsSpeedAnalysis
	case store.ModuleRateLimitResetToday:
		return p.ShowsRateLimitResetToday
	case store.ModuleQuotaEstimate:
		return p.ShowsQuotaEstimateSummary
	case store.ModuleResetCredits:
		return p.ShowsResetCreditsSummary
	case store.ModuleCacheAnalysis:
		return p.ShowsCacheAnalysis
	case store.ModuleAPICost:
		return p.ShowsCostSummary
	case store.ModuleSessionRepair:
		return p.ShowsSessionRepairSummary
	case store.ModuleRecentSessions:
		return p.ShowsRecentSessions
	case store.ModuleSkills:
		return p.ShowsSkills
	default:
		return false
	}
}

func panelModule(c *ui.Context, l l10n.Localizer, m store.Module, prefs store.Preferences, snap runway.Snapshot, acts PanelActions, first *bool, state *panelState) {
	grok := prefs.SelectedProvider == store.ProviderGrok
	claude := prefs.SelectedProvider == store.ProviderClaude
	switch m {
	case store.ModuleQuota:
		switch {
		case claude:
			panelClaudeQuota(c, l, snap, acts, first, state, state.resetSwitch(prefs.QuotaResetTimeFormat), prefs.AccountPrivacyModeEnabled)
		case grok:
			panelGrokQuota(c, l, snap, acts, first, state, state.resetSwitch(prefs.QuotaResetTimeFormat))
		default:
			panelCodexQuota(c, l, snap, acts, first, state, state.resetSwitch(prefs.QuotaResetTimeFormat), prefs.AccountPrivacyModeEnabled, prefs.ShowsCreditsBalance)
		}
	case store.ModuleTokenUsage:
		panelToken(c, l, prefs.TokenUsageChartStyle, snap, acts, first, state)
	case store.ModuleSpeedAnalysis:
		panelSpeed(c, l, snap, acts, first, state)
	case store.ModuleRateLimitResetToday:
		panelResetToday(c, l, snap, acts, first, state)
	case store.ModuleQuotaEstimate:
		panelEstimate(c, l, snap, acts, first, state)
	case store.ModuleResetCredits:
		panelCredits(c, l, snap, acts, first, state)
	case store.ModuleCacheAnalysis:
		panelCache(c, l, snap, acts, first, state)
	case store.ModuleAPICost:
		panelCost(c, l, snap, acts, first, state)
	case store.ModuleSessionRepair:
		panelRepair(c, l, snap, acts, first, state)
	case store.ModuleRecentSessions:
		panelRecent(c, l, snap, grok || claude, acts, first, state)
	case store.ModuleSkills:
		panelSkills(c, l, snap, acts, first, state)
	}
}

func sectionRefresh(acts PanelActions, sec runway.Section) func() {
	if acts.RefreshSection == nil {
		return nil
	}
	fn := acts.RefreshSection
	return func() { fn(sec) }
}

// sectionRefreshing includes the wait for a full refresh to reach a module.
// Single-section Codex work must not mark another provider as refreshing.
func sectionRefreshing(snap runway.Snapshot, section runway.Section) bool {
	if snap.Refresh.All {
		return true
	}
	switch snap.Provider {
	case store.ProviderCursor:
		return snap.Cursor.Refreshing
	case store.ProviderGrok:
		return snap.Grok.Refreshing
	case store.ProviderClaude:
		return snap.Claude.Refreshing
	default:
		return snap.Refresh.Has(section)
	}
}

// sectionEmpty shows a progress indicator while loading, then the idle text.
func sectionEmpty(c *ui.Context, l l10n.Localizer, refreshing bool) {
	sectionEmptyKey(c, l, refreshing, l10n.KeyNotLoaded)
}

func sectionEmptyKey(c *ui.Context, l l10n.Localizer, refreshing bool, key l10n.Key) {
	if refreshing {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(14, 14).Label(tx(l, xPanelLoading))
			calloutDim(c, tx(l, xPanelLoading))
		})
		return
	}
	calloutDim(c, l.Text(key))
}
