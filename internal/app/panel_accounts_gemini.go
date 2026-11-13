package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// geminiAccountsPage lists the managed Gemini accounts with their cached
// quota windows. Adding opens the Gemini accounts tab of the settings.
func (a *App) geminiAccountsPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	view, ac := snap.Gemini.Accounts, acts.Accounts
	st := &a.panelState.accounts
	ui.Column(c).Gap(10).Grow(1).Children(func() {
		a.accountsToolbar(c, l, prefs, func() {
			loading := snap.Gemini.Refreshing || st.refreshing == allRefreshing
			if ghostButton(c, icRefresh, l.Text(l10n.KeyAccountsRefreshAll), loading, ac.GeminiRefreshAll == nil).Clicked() && ac.GeminiRefreshAll != nil {
				a.accountsRun(allRefreshing, ac.GeminiRefreshAll)
			}
		}, nil, func() {
			if accentButton(c, icPlus, l.Text(l10n.KeyAccountsAdd), false).Clicked() {
				a.openAccountsSettings(accountKindGemini)
			}
		})
		a.accountsList(c, func() {
			if len(view.Sidebar) == 0 {
				accountEmpty(c, l.Text(l10n.KeyGeminiAccountsEmpty))
			}
			for _, acc := range grokDisplayOrder(view) {
				a.geminiAccountCard(c, l, prefs, snap, acc, ac)
			}
			accountsNotice(c, []string{view.Message}, snap.Gemini.LastError)
		})
	})
}

func (a *App) geminiAccountCard(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acc codex.ManagedAccount, ac AccountActions) {
	view := snap.Gemini.Accounts
	current := acc.ID == view.ActiveID
	priv := prefs.AccountPrivacyModeEnabled
	quota := snap.Gemini.AccountQuotas[acc.ID]
	accountCard(c, current, func() {
		a.geminiCardHeader(c, l, acc, quota, current, priv, view, ac)
		switch {
		case acc.RequiresReauth:
			accountStatusLine(c, l.Text(l10n.KeyGeminiReauthRequired), sys(c.Theme(), sysRed))
		case acc.LastError != "":
			accountStatusLine(c, acc.LastError, sys(c.Theme(), sysOrange))
		}
		geminiQuotaBlock(c, l, &quota, panelQuotaOptions(prefs, current))
		a.geminiManagement(c, l, acc, current, priv, view, ac)
	})
}

func (a *App) geminiCardHeader(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, q runway.GeminiAccountQuota, current, priv bool, view runway.AccountsView, ac AccountActions) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Basis(0).MinWidth(0).Children(func() {
			name := acc.ResolvedDisplayName()
			ui.Text(c, accountPrivate(priv, name)).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…")
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if current {
					runwayTagPadded(c, l.Text(l10n.KeyAccountsCurrent), toneGreen, 1, 5)
				}
				if q.Plan != "" || q.TierKind != "" {
					geminiPlanBadgePadded(c, l, q.TierKind, q.Plan, 1, 5)
				}
				if acc.Email != "" && acc.Email != name {
					ui.Text(c, accountPrivate(priv, acc.Email)).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…").Shrink(1)
				}
			})
		})
		a.geminiCardActions(c, l, acc, current, priv, view, ac)
	})
}

// geminiCardActions are the tick and refresh controls of a card.
func (a *App) geminiCardActions(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	id := acc.ID
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		title, icon := l.Text(l10n.KeyAccountsMakeCurrent), icCheckCircle
		if current {
			title, icon = l.Text(l10n.KeyAccountsCurrent), icCheckCircleFill
		}
		tick := accountIconButton(c, icon, title, current || st.busy || refreshing || ac.GeminiSwitch == nil, false, current)
		if tick.Clicked() {
			st.switchID, st.restart = id, codexRestartDefault
		}
		a.geminiSwitchConfirm(c, l, tick, id, name, ac)
		if accountIconButton(c, icRefresh, l.Text(l10n.KeyRefresh), refreshing || ac.GeminiRefresh == nil, refreshing, false).Clicked() {
			a.accountsRun(id, func() string { return ac.GeminiRefresh(id) })
		}
	})
}

// geminiSwitchConfirm asks before the switch, with the choice to restart
// the desktop app, then again when Antigravity or agy is still running.
func (a *App) geminiSwitchConfirm(c *ui.Context, l l10n.Localizer, anchor ui.Element, id, name string, ac AccountActions) {
	st := &a.panelState.accounts
	open := st.switchID == id
	confirmPopover(c, l, anchor, &open, l.Text(l10n.KeyAccountsSwitchConfirmTitle), name+"\n\n"+l.Text(l10n.KeyGeminiSwitchNote),
		l.Text(l10n.KeyAccountsMakeCurrent), func() { geminiRestartChoice(c, l, &st.restart) }, func() { a.geminiSwitch(id, st.restart, false, ac) })
	if !open && st.switchID == id {
		st.switchID = ""
	}
	busy := st.runningID == id
	confirmPopover(c, l, anchor, &busy, l.Text(l10n.KeyGeminiRunningTitle), l.Text(l10n.KeyGeminiRunningMessage),
		l.Text(l10n.KeyAccountsForceCurrent), nil, func() { a.geminiSwitch(id, st.restart, true, ac) })
	if !busy && st.runningID == id {
		st.runningID = ""
	}
}

// geminiRestartChoice is the restart checkbox; elsewhere than macOS the user
// restarts Antigravity themselves.
func geminiRestartChoice(c *ui.Context, l l10n.Localizer, restart *bool) {
	if !geminiRestartSupported {
		dimText(c, l.Text(l10n.KeyGeminiRestartUnsupported))
		return
	}
	ui.Checkbox(c.Key("gemini-restart-after-switch"), restart, l.Text(l10n.KeyGeminiRestartApp))
	dimText(c, l.Text(l10n.KeyGeminiRestartHint))
}

func (a *App) geminiSwitch(id string, restart, force bool, ac AccountActions) {
	st := &a.panelState.accounts
	st.runningID = ""
	restart = restart && geminiRestartSupported
	a.accountsRun("", func() string {
		running, msg := ac.GeminiSwitch(id, restart, force)
		if running {
			runOnMain(func() { st.runningID = id })
		}
		return msg
	})
}

// geminiManagement is the remove control; the current account cannot be
// removed. Aliases and order are edited in the settings.
func (a *App) geminiManagement(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	id := acc.ID
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c).Grow(1)
		title := l.Text(l10n.KeyAccountsDelete)
		if current {
			title = l.Text(l10n.KeyGeminiCurrentAccountCannotDelete)
		}
		trash := accountIconButton(c, icTrash, title, current || st.busy || refreshing || ac.GeminiDelete == nil, false, false)
		if trash.Clicked() {
			st.deleteID = id
		}
		open := st.deleteID == id
		confirmPopover(c, l, trash, &open, l.Text(l10n.KeyAccountsDeleteConfirmTitle), name+"\n\n"+l.Text(l10n.KeyGeminiDeleteConfirmMessage),
			l.Text(l10n.KeyAccountsDelete), nil, func() {
				a.accountsRun("", func() string { return ac.GeminiDelete(id) })
			})
		if !open && st.deleteID == id {
			st.deleteID = ""
		}
	})
}
