package app

import (
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// grokAccountsPage is GrokAccountsDetailView.
func (a *App) grokAccountsPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	view, ac := snap.Grok.Accounts, acts.Accounts
	st := &a.panelState.accounts
	ui.Column(c).Gap(10).Grow(1).Children(func() {
		a.accountsToolbar(c, l, prefs, func() {
			loading := snap.Grok.Refreshing || st.refreshing == allRefreshing
			if ghostButton(c, icRefresh, l.Text(l10n.KeyGrokAccountsRefreshAll), loading, ac.GrokRefreshAll == nil).Clicked() && ac.GrokRefreshAll != nil {
				a.accountsRun(allRefreshing, ac.GrokRefreshAll)
			}
		}, func() {
			if st.loggingIn() && ghostButton(c, icClose, l.Text(l10n.KeyCancel), false, false).Clicked() {
				a.accountsCancelLogin()
			}
		}, func() { a.grokAddMenu(c, l, ac) })
		a.accountsList(c, func() {
			if len(view.Sidebar) == 0 {
				accountEmpty(c, l.Text(l10n.KeyGrokAccountsEmpty))
			}
			for _, acc := range grokDisplayOrder(view) {
				a.grokAccountCard(c, l, prefs, snap, acc, grokCaps(view.Sidebar, acc.ID, view.ActiveID), ac)
			}
			accountsNotice(c, []string{view.Message}, snap.Grok.LastError)
		})
	})
}

// grokDisplayOrder pins the current account first; the rest keep their
// stored order.
func grokDisplayOrder(view runway.AccountsView) []codex.ManagedAccount {
	out := append([]codex.ManagedAccount(nil), view.Sidebar...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID == view.ActiveID && out[j].ID != view.ActiveID
	})
	return out
}

// grokCapabilities is GrokAccountDetailCapabilities: moves follow the stored
// order, not the display order.
type grokCapabilities struct{ up, down, delete bool }

func grokCaps(stored []codex.ManagedAccount, id, activeID string) grokCapabilities {
	index := 0
	for i, acc := range stored {
		if acc.ID == id {
			index = i
		}
	}
	return grokCapabilities{up: index > 0, down: index+1 < len(stored), delete: id != activeID}
}

// grokAddMenu is the "Add account" menu and the paste popover it opens.
func (a *App) grokAddMenu(c *ui.Context, l l10n.Localizer, ac AccountActions) {
	st := &a.panelState.accounts
	add := accentButton(c, icPlus, l.Text(l10n.KeyAccountsAdd), st.busy)
	add.Menu(func(m *ui.Menu) {
		if m.Item(l.Text(l10n.KeyGrokAccountsAddOAuth)).Chosen() {
			a.accountsLogin(ac)
		}
		if m.Item(l.Text(l10n.KeyGrokAccountsAddPaste)).Chosen() {
			st.pasteOpen, st.paste, st.pasteErr = true, "", ""
		}
		if m.Item(l.Text(l10n.KeyGrokAccountsImportOfficial)).Chosen() && ac.GrokImportOfficial != nil {
			a.accountsRun("", ac.GrokImportOfficial)
		}
	})
	panelPopover(c, add, &st.pasteOpen, func() { a.grokPasteBody(c, l, ac) })
}

// grokPasteBody is the paste-import sheet: the hint, the text, and the
// import that stays open with its error until it succeeds.
func (a *App) grokPasteBody(c *ui.Context, l l10n.Localizer, ac AccountActions) {
	st := &a.panelState.accounts
	ui.Column(c).Gap(12).Width(320).Padding(4).Children(func() {
		ui.Text(c, l.Text(l10n.KeyGrokAccountsAddPaste)).FontSize(fsBody).FontWeight(600)
		dimText(c, l.Text(l10n.KeyGrokAccountsPasteHint))
		ui.TextArea(c.Key("grok-paste"), &st.paste).Lines(6, 10).Label(l.Text(l10n.KeyGrokAccountsAddPaste))
		ui.Row(c).Children(func() {
			if PillButton(c, nil, l.Text(l10n.KeyAccountsPasteFromClipboard), st.busy).Clicked() {
				if clip := mygo.Clipboard.ReadText(); clip != "" {
					st.paste = clip
				}
			}
		})
		if st.pasteErr != "" {
			accountStatusLine(c, st.pasteErr, sys(c.Theme(), sysRed))
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyCancel), st.busy).Clicked() {
				st.pasteOpen, st.pasteErr = false, ""
			}
			empty := strings.TrimSpace(st.paste) == ""
			if PillButton(c, nil, l.Text(l10n.KeyAccountsAdd), st.busy || empty || ac.GrokPaste == nil).Clicked() {
				a.grokPaste(ac, st.paste)
			}
		})
	})
}

func (a *App) grokPaste(ac AccountActions, text string) {
	st := &a.panelState.accounts
	st.pasteErr = ""
	a.accountsRun("", func() string {
		msg, ok := ac.GrokPaste(text)
		runOnMain(func() {
			if ok {
				st.pasteOpen, st.paste = false, ""
			} else {
				st.pasteErr = msg
			}
		})
		if ok {
			return msg
		}
		return ""
	})
}

// grokAccountCard is GrokAccountDetailCard.
func (a *App) grokAccountCard(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acc codex.ManagedAccount, caps grokCapabilities, ac AccountActions) {
	view := snap.Grok.Accounts
	current := acc.ID == view.ActiveID
	priv := prefs.AccountPrivacyModeEnabled
	quota := snap.Grok.AccountQuotas[acc.ID]
	accountCard(c, current, func() {
		a.grokCardHeader(c, l, acc, quota, current, priv, view, ac)
		a.grokAliasEditor(c, l, acc.ID, ac)
		switch {
		case acc.RequiresReauth:
			accountStatusLine(c, l.Text(l10n.KeyGrokReauthenticationRequired), sys(c.Theme(), sysRed))
		case acc.LastError != "":
			accountStatusLine(c, acc.LastError, sys(c.Theme(), sysOrange))
		}
		grokQuotaBlock(c, l, &quota, panelQuotaOptions(prefs, current))
		a.grokManagement(c, l, acc, caps, priv, view, ac)
	})
}

func (a *App) grokCardHeader(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, q runway.GrokAccountQuota, current, priv bool, view runway.AccountsView, ac AccountActions) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Basis(0).MinWidth(0).Children(func() {
			name := acc.ResolvedDisplayName()
			ui.Text(c, accountPrivate(priv, name)).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…")
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if current {
					runwayTagPadded(c, l.Text(l10n.KeyAccountsCurrent), toneGreen, 1, 5)
				}
				if q.Plan != "" {
					kind := grok.TierResolve(q.Plan)
					tierBadgePadded(c, grokTierLook(t, kind), kind.DisplayName(), 1, 5)
				}
				if acc.Email != "" && acc.Email != name {
					ui.Text(c, accountPrivate(priv, acc.Email)).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…").Shrink(1)
				}
			})
		})
		a.grokCardActions(c, l, acc, current, priv, view, ac)
	})
}

// grokCardActions are the tick and refresh controls of a card.
func (a *App) grokCardActions(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		title, icon := l.Text(l10n.KeyGrokAccountsMakeCurrent), icCheckCircle
		if current {
			title, icon = l.Text(l10n.KeyGrokAccountsCurrent), icCheckCircleFill
		}
		tick := accountIconButton(c, icon, title, current || st.busy || refreshing || ac.GrokSwitch == nil, false, current)
		if tick.Clicked() {
			st.switchID = acc.ID
		}
		a.grokSwitchConfirm(c, l, tick, acc.ID, name, ac)
		if accountIconButton(c, icRefresh, l.Text(l10n.KeyRefresh), refreshing || ac.GrokRefresh == nil, refreshing, false).Clicked() {
			id := acc.ID
			a.accountsRun(id, func() string { return ac.GrokRefresh(id) })
		}
	})
}

// grokSwitchConfirm is GrokSwitchConfirmSheet, then the running-process
// alert when Grok is in use and needs a second confirmation.
func (a *App) grokSwitchConfirm(c *ui.Context, l l10n.Localizer, anchor ui.Element, id, name string, ac AccountActions) {
	st := &a.panelState.accounts
	open := st.switchID == id
	confirmPopover(c, l, anchor, &open, l.Text(l10n.KeyGrokSwitchConfirmTitle), l.Textf(l10n.KeyGrokSwitchConfirmMessage, name),
		l.Text(l10n.KeyGrokAccountsMakeCurrent), nil, func() { a.grokSwitch(id, false, ac) })
	if !open && st.switchID == id {
		st.switchID = ""
	}
	busy := st.runningID == id
	confirmPopover(c, l, anchor, &busy, l.Text(l10n.KeyGrokSwitchRunningTitle), l.Text(l10n.KeyGrokSwitchRunningMessage),
		l.Text(l10n.KeyGrokAccountsMakeCurrent), nil, func() { a.grokSwitch(id, true, ac) })
	if !busy && st.runningID == id {
		st.runningID = ""
	}
}

func (a *App) grokSwitch(id string, force bool, ac AccountActions) {
	st := &a.panelState.accounts
	st.runningID = ""
	a.accountsRun("", func() string {
		running, msg := ac.GrokSwitch(id, force)
		if running {
			runOnMain(func() { st.runningID = id })
		}
		return msg
	})
}

// grokAliasEditor edits a card alias without changing the account list layout.
func (a *App) grokAliasEditor(c *ui.Context, l l10n.Localizer, id string, ac AccountActions) {
	st := &a.panelState.accounts
	if st.aliasID != id {
		return
	}
	open := true
	ui.Modal(c.Key("grok-alias-dialog"), &open, func() {
		ui.Text(c, l.Text(l10n.KeyAlias)).FontSize(16).FontWeight(600)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("grok-alias"), &st.alias).Placeholder(l.Text(l10n.KeyAlias)).Label(l.Text(l10n.KeyAlias)).Grow(1).Basis(0)
			if PillButton(c, nil, l.Text(l10n.KeyOk), ac.GrokAlias == nil).Clicked() {
				alias := st.alias
				st.aliasID, st.alias = "", ""
				a.accountsRun("", func() string { return ac.GrokAlias(id, alias) })
			}
			if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
				st.aliasID, st.alias = "", ""
			}
		})
	}).Width(340)
	if !open {
		st.aliasID, st.alias = "", ""
	}
}

// grokManagement is the row of rename, move and remove controls.
func (a *App) grokManagement(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, caps grokCapabilities, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	blocked := st.busy || refreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c).Grow(1)
		if accountIconButton(c, icPencil, l.Text(l10n.KeyAlias), blocked || ac.GrokAlias == nil, false, false).Clicked() {
			st.aliasID, st.alias = acc.ID, acc.Alias
		}
		if accountIconButton(c, icArrowUp, l.Text(l10n.KeyAccountsMoveUp), !caps.up || blocked || ac.GrokMove == nil, false, false).Clicked() {
			a.grokMove(acc.ID, -1, ac)
		}
		if accountIconButton(c, icArrowDown, l.Text(l10n.KeyAccountsMoveDown), !caps.down || blocked || ac.GrokMove == nil, false, false).Clicked() {
			a.grokMove(acc.ID, 1, ac)
		}
		title := l.Text(l10n.KeyGrokAccountsRemove)
		if !caps.delete {
			title = l.Text(l10n.KeyGrokCurrentAccountCannotDelete)
		}
		trash := accountIconButton(c, icTrash, title, !caps.delete || blocked || ac.GrokDelete == nil, false, false)
		if trash.Clicked() {
			st.deleteID = acc.ID
		}
		a.grokDeleteConfirm(c, l, trash, acc.ID, name, ac)
	})
}

func (a *App) grokMove(id string, direction int, ac AccountActions) {
	a.accountsRun("", func() string { return ac.GrokMove(id, direction) })
}

func (a *App) grokDeleteConfirm(c *ui.Context, l l10n.Localizer, anchor ui.Element, id, name string, ac AccountActions) {
	st := &a.panelState.accounts
	open := st.deleteID == id
	confirmPopover(c, l, anchor, &open, l.Text(l10n.KeyGrokAccountsDeleteConfirmTitle), name+"\n\n"+l.Text(l10n.KeyGrokAccountsDeleteConfirmMessage),
		l.Text(l10n.KeyGrokAccountsRemove), nil, func() {
			a.accountsRun("", func() string { return ac.GrokDelete(id) })
		})
	if !open && st.deleteID == id {
		st.deleteID = ""
	}
}
