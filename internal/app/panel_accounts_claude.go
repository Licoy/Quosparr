package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// claudeAccountsPage lists the managed Claude accounts with their cached
// 5-hour usage. Adding opens the Claude accounts tab of the settings.
func (a *App) claudeAccountsPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	view, ac := snap.Claude.Accounts, acts.Accounts
	st := &a.panelState.accounts
	ui.Column(c).Gap(10).Grow(1).Children(func() {
		a.accountsToolbar(c, l, prefs, func() {
			loading := snap.Claude.Refreshing || st.refreshing == allRefreshing
			if ghostButton(c, icRefresh, l.Text(l10n.KeyAccountsRefreshAll), loading, ac.ClaudeRefreshAll == nil).Clicked() && ac.ClaudeRefreshAll != nil {
				a.accountsRun(allRefreshing, ac.ClaudeRefreshAll)
			}
		}, nil, func() {
			if accentButton(c, icPlus, l.Text(l10n.KeyAccountsAdd), false).Clicked() {
				a.openAccountsSettings(accountKindClaude)
			}
		})
		desktopRecoveryButton(c, l, &st.desktop, currentRuntime())
		renderDesktopDialog(c, l, &st.desktop, currentRuntime(), func(fn func()) { runOnMain(func() { fn(); a.redrawPanel() }) })
		a.accountsList(c, func() {
			if len(view.Sidebar) == 0 {
				accountEmpty(c, l.Text(l10n.KeyClaudeAccountsEmpty))
			}
			for _, acc := range grokDisplayOrder(view) {
				a.claudeAccountCard(c, l, prefs, snap, acc, ac)
			}
			accountsNotice(c, []string{view.Message}, snap.Claude.LastError)
		})
	})
}

func (a *App) claudeAccountCard(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acc codex.ManagedAccount, ac AccountActions) {
	view := snap.Claude.Accounts
	current := acc.ID == view.ActiveID
	priv := prefs.AccountPrivacyModeEnabled
	quota, hasQuota := snap.Claude.AccountQuotas[acc.ID]
	accountCard(c, current, func() {
		a.claudeCardHeader(c, l, acc, quota, current, priv, view, ac)
		switch {
		case acc.RequiresReauth:
			accountStatusLine(c, l.Text(l10n.KeyClaudeReauthRequired), sys(c.Theme(), sysRed))
		case acc.LastError != "":
			accountStatusLine(c, privateSiteText(priv, acc.LastError, acc.APIBaseURL), sys(c.Theme(), sysOrange))
		}
		if acc.IsRelay() {
			apiBalanceBlock(c, l, acc.APIQuota, current)
		} else if hasQuota && quota.Title != "" {
			accountMeter(c, l, claudeAccountMeter(quota, current))
		} else {
			calloutDim(c, l.Text(l10n.KeyClaudeNoQuotaData)).FontSize(fsCaption)
		}
		if quota.ExtraUsage != "" {
			dimText(c, quota.ExtraUsage).FontSize(fsCaption)
		}
		a.claudeManagement(c, l, acc, current, priv, view, ac)
	})
}

func claudeAccountMeter(q runway.ClaudeAccountQuota, current bool) meterSpec {
	m := meterSpec{title: q.Title, remaining: max(0, 100-q.UsedPercent), used: q.UsedPercent, current: current}
	if !q.ResetsAt.IsZero() {
		m.resetsAt, m.hasReset = q.ResetsAt, true
	}
	return m
}

func (a *App) claudeCardHeader(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, q runway.ClaudeAccountQuota, current, priv bool, view runway.AccountsView, ac AccountActions) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Basis(0).MinWidth(0).Children(func() {
			name := acc.ResolvedDisplayName()
			ui.Text(c, accountPrivate(priv, name)).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…")
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if current {
					runwayTagPadded(c, l.Text(l10n.KeyAccountsCurrent), toneGreen, 1, 5)
				}
				if acc.IsRelay() {
					apiTierBadge(c, l)
				} else if q.Plan != "" {
					tierBadgePadded(c, claudeTierLook(t, claude.TierResolve(q.Plan)), q.Plan, 1, 5)
				}
				if acc.Email != "" && acc.Email != name {
					ui.Text(c, accountPrivate(priv, acc.Email)).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…").Shrink(1)
				}
			})
		})
		a.claudeCardActions(c, l, acc, current, priv, view, ac)
	})
}

// claudeCardActions are the tick and refresh controls of a card.
func (a *App) claudeCardActions(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		title, icon := l.Text(l10n.KeyAccountsMakeCurrent), icCheckCircle
		if current {
			title, icon = l.Text(l10n.KeyAccountsCurrent), icCheckCircleFill
		}
		tick := accountIconButton(c, icon, title, current || st.busy || refreshing || ac.ClaudeSwitch == nil, false, current)
		if tick.Clicked() {
			st.switchID = acc.ID
			st.desktopSwitch = false
		}
		open := st.switchID == acc.ID
		id := acc.ID
		confirmPopover(c, l, tick, &open, l.Text(l10n.KeyAccountsSwitchConfirmTitle),
			l.Textf(l10n.KeyClaudeSwitchConfirmMessage, name)+"\n\n"+claudeSwitchNote(l, acc),
			l.Text(l10n.KeyAccountsMakeCurrent), func() {
				if acc.AuthMode != codex.AuthModeAPIKey {
					desktopSwitchOption(c, l, currentRuntime(), id, &st.desktopSwitch, st.busy || ac.ClaudeSwitchDesktop == nil, func() {
						st.switchID = ""
						st.desktop = newDesktopDialog(currentRuntime(), id, priv)
					})
				}
			}, func() {
				desktop := st.desktopSwitch
				a.accountsRun("", func() string {
					if ac.ClaudeSwitchDesktop != nil {
						return ac.ClaudeSwitchDesktop(id, desktop)
					}
					if desktop {
						return tx(l, xDesktopFailed)
					}
					return ac.ClaudeSwitch(id)
				})
			})
		if !open && st.switchID == acc.ID {
			st.switchID = ""
		}
		if accountIconButton(c, icRefresh, l.Text(l10n.KeyRefresh), refreshing || ac.ClaudeRefresh == nil, refreshing, false).Clicked() {
			a.accountsRun(id, func() string { return ac.ClaudeRefresh(id) })
		}
	})
}

// claudeManagement is the remove control; the current account cannot be
// removed. Aliases and order are edited in the settings.
func (a *App) claudeManagement(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		if acc.AuthMode != codex.AuthModeAPIKey {
			if PillButton(c, nil, tx(l, xDesktopBind), st.busy).Clicked() {
				st.desktop = newDesktopDialog(currentRuntime(), acc.ID, priv)
			}
			if PillButton(c, nil, tx(l, xDesktopOpen), st.busy || !desktopBound(currentRuntime(), acc.ID)).Clicked() {
				st.desktop = newDesktopDialog(currentRuntime(), acc.ID, priv)
				st.desktop.openOnly = true
			}
		}
		ui.Spacer(c).Grow(1)
		title := l.Text(l10n.KeyAccountsDelete)
		if current {
			title = l.Text(l10n.KeyClaudeCurrentAccountCannotDelete)
		}
		trash := accountIconButton(c, icTrash, title, current || st.busy || refreshing || ac.ClaudeDelete == nil, false, false)
		if trash.Clicked() {
			st.deleteID = acc.ID
		}
		open := st.deleteID == acc.ID
		id := acc.ID
		confirmPopover(c, l, trash, &open, l.Text(l10n.KeyAccountsDeleteConfirmTitle), name+"\n\n"+l.Text(l10n.KeyClaudeDeleteConfirmMessage),
			l.Text(l10n.KeyAccountsDelete), nil, func() {
				a.accountsRun("", func() string { return ac.ClaudeDelete(id) })
			})
		if !open && st.deleteID == acc.ID {
			st.deleteID = ""
		}
	})
}

// claudeSwitchNote is the warning under a switch confirmation.
func claudeSwitchNote(l l10n.Localizer, acc codex.ManagedAccount) string {
	if acc.IsRelay() {
		return tx(l, xAPIClaudeSwitchNote)
	}
	return l.Text(l10n.KeyClaudeSwitchRisk)
}
