package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// panelView renders the published snapshot. It does not refresh, scan, or
// write preferences. A nil PanelActions func disables that control.
//
// The main page is a fixed header, a scrolling body, the status lines and a
// fixed footer. A detail page replaces the header with its back header and
// drops the footer.
func (a *App) panelView(c *ui.Context) {
	prefs := a.settings.Prefs()
	if !prefs.ShowsCacheAnalysis && (a.panelState.cache.summary.cancel != nil || a.panelState.cache.detail.cancel != nil || a.panelState.detail == store.ModuleCacheAnalysis) {
		a.panelState.cache.cancelAll()
		if a.panelState.detail == store.ModuleCacheAnalysis {
			a.panelState.detail = ""
		}
	}
	a.syncPanelContext(prefs)
	t := applyPanelTheme(c, prefs.MainPanelBackgroundStyle, prefs.ThemeColor, prefs.FontFamily)
	l := a.L()
	snap := a.Snapshot()
	acts := a.PanelActions()
	snap.Provider = prefs.SelectedProvider
	a.syncPanelAccount(snap, prefs)
	if a.panel != nil && prefs.ShowsCacheAnalysis && prefs.SelectedProvider != store.ProviderCursor {
		a.syncCacheSummary(acts)
	}
	a.panelState.setChartStyle = func(s store.ChartStyle) { a.settings.SetTokenUsageChartStyle(s) }
	a.panelState.cacheOwner = a
	a.panelState.setResetTimeFormat = func(f store.ResetTimeFormat) { a.settings.SetQuotaResetTimeFormat(f) }
	a.panelState.startReaction = a.startReaction
	a.panelState.syncSkills = a.syncSkills
	a.panelState.refreshSkills = a.startSkillsQuery
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		if a.panelState.detail != "" {
			a.panelDetailPage(c, l, prefs, snap, acts)
		} else {
			a.panelMainPage(c, l, prefs, snap, acts)
		}
		panelResizeHandle(c, a)
	})
}

// syncPanelContext drops view state that belongs to another provider.
func (a *App) syncPanelContext(prefs store.Preferences) {
	if a.panelState.provider != prefs.SelectedProvider {
		a.cancelPanelDetails()
		a.panelState.provider = prefs.SelectedProvider
		a.panelState.detail = ""
	}
	if a.panelState.scrollDetail != a.panelState.detail {
		a.panelState.detailScroll = ui.ScrollState{}
		a.panelState.scrollDetail = a.panelState.detail
	}
}

// syncPanelAccount cancels detail work when the account or its state generation changed.
func (a *App) syncPanelAccount(snap runway.Snapshot, prefs store.Preferences) {
	account := activePanelAccount(snap, prefs.SelectedProvider)
	generation := activePanelGeneration(snap, prefs.SelectedProvider)
	if a.panelState.account != account || a.panelState.accountGeneration != generation {
		a.cancelPanelDetails()
		a.panelState.account = account
		a.panelState.accountGeneration = generation
	}
}

func (a *App) panelMainPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	panelHeader(c, a, l, prefs, snap, acts)
	hairline(c)
	ui.Scroll(c.Key("panel-main-scroll")).Grow(1).TrackScroll(&a.panelState.mainScroll).Children(func() {
		ui.Column(c).Padding(4, 16).Children(func() {
			panelStartup(c, a, l)
			panelBody(c, l, prefs, snap, acts, &a.panelState)
		})
	})
	panelStatusLines(c, l, prefs, snap)
	hairline(c)
	panelFooter(c, a, l)
}

// panelStatusLines shows the operation message and the last error of the
// selected provider above the footer.
func panelStatusLines(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) {
	message, failure := snap.Accounts.Message, snap.Codex.LastError
	switch prefs.SelectedProvider {
	case store.ProviderCursor:
		message, failure = "", snap.Cursor.LastError
	case store.ProviderGrok:
		message, failure = snap.Grok.Accounts.Message, snap.Grok.LastError
	case store.ProviderClaude:
		message, failure = snap.Claude.Accounts.Message, snap.Claude.LastError
	}
	if message == "" && failure == "" {
		return
	}
	t := c.Theme()
	ui.Column(c).Gap(3).Padding(0, 16, 6, 16).Children(func() {
		if message != "" {
			ui.Text(c, message).FontSize(fsCaption).TextColor(inkSecondary(t))
		}
		if failure != "" {
			ui.Text(c, failure).FontSize(fsCaption).TextColor(sys(t, sysRed))
		}
	})
}

// panelStartup keeps the damaged-startup notice: the settings or proxy
// error in the user's language, and no control that rewrites them.
func panelStartup(c *ui.Context, a *App, l l10n.Localizer) {
	t := c.Theme()
	s := a.settings
	shown := false
	if err := s.LoadError(); err != nil {
		shown = true
		ui.Text(c, tx(l, xSettingsUnreadable)).FontSize(13).FontWeight(600).TextColor(t.Danger)
		ui.Text(c, err.Error()).FontSize(11).TextColor(t.Danger)
		ui.Text(c, tx(l, xSettingsReadOnly)).FontSize(11).TextColor(t.TextMuted)
	}
	if err := s.ProxyError(); err != nil {
		shown = true
		ui.Text(c, err.Error()).FontSize(11).TextColor(t.Danger)
		ui.Text(c, tx(l, xProxyBlocked)).FontSize(11).TextColor(t.TextMuted)
	}
	for _, p := range a.opts.Report.Problems {
		if p.Step == "config" || p.Step == "proxy credentials" {
			continue
		}
		shown = true
		ui.Text(c, p.String()).FontSize(12).TextColor(t.Danger)
	}
	if shown {
		ui.Box(c).Height(8)
	}
}
