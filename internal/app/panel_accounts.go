package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// accountsPage is the multi-account page of the open provider: a fixed
// toolbar over the scrolling account cards.
func (a *App) accountsPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	a.panelState.accounts.page = prefs.SelectedProvider
	defer a.accountsNoticeDialog(c, l)
	switch prefs.SelectedProvider {
	case store.ProviderCursor:
		return
	case store.ProviderGrok:
		a.grokAccountsPage(c, l, prefs, snap, acts)
	case store.ProviderClaude:
		a.claudeAccountsPage(c, l, prefs, snap, acts)
	default:
		a.codexAccountsPage(c, l, prefs, snap, acts)
	}
}

// accountsList scrolls the cards under the toolbar.
func (a *App) accountsList(c *ui.Context, body func()) {
	ui.Scroll(c.Key("accounts-scroll")).Grow(1).TrackScroll(&a.panelState.detailScroll).Children(func() {
		ui.Column(c).Gap(8).Children(body)
	})
}

// accountsToolbar is the row over the list: refresh all, the eye, and the
// add control that the caller draws.
func (a *App) accountsToolbar(c *ui.Context, l l10n.Localizer, prefs store.Preferences, refresh func(), left, add func()) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		refresh()
		if left != nil {
			left()
		}
		ui.Spacer(c).Grow(1)
		if privacyButton(c, l, prefs.AccountPrivacyModeEnabled, 32, 26, radiusRow).Clicked() {
			a.settings.SetAccountPrivacyMode(!prefs.AccountPrivacyModeEnabled)
		}
		add()
	})
}

func (a *App) codexAccountsPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	view, ac := snap.Accounts, acts.Accounts
	st := &a.panelState.accounts
	ui.Column(c).Gap(10).Grow(1).Children(func() {
		a.accountsToolbar(c, l, prefs, func() {
			loading := snap.Refresh.AccountQuotas || st.refreshing == allRefreshing
			if ghostButton(c, icRefresh, l.Text(l10n.KeyAccountsRefreshAll), loading, ac.RefreshAll == nil).Clicked() && ac.RefreshAll != nil {
				a.accountsRun(allRefreshing, ac.RefreshAll)
			}
		}, nil, func() {
			if accentButton(c, icPlus, l.Text(l10n.KeyAccountsAdd), false).Clicked() {
				a.openAccountsSettings(accountKindCodex)
			}
		})
		a.accountsList(c, func() {
			if len(view.Sidebar) == 0 {
				accountEmpty(c, l.Text(l10n.KeyAccountsEmpty))
			}
			for _, acc := range view.Sidebar {
				a.codexAccountCard(c, l, prefs, view, acc, ac)
			}
			accountsNotice(c, []string{view.Message}, snap.Codex.LastError)
		})
	})
}

// codexAccountCard is AccountDetailCard.
func (a *App) codexAccountCard(c *ui.Context, l l10n.Localizer, prefs store.Preferences, view runway.AccountsView, acc codex.ManagedAccount, ac AccountActions) {
	current := view.ActiveID != "" && acc.ID == view.ActiveID
	priv := prefs.AccountPrivacyModeEnabled
	accountCard(c, current, func() {
		a.codexCardHeader(c, l, acc, current, priv, view, ac)
		switch {
		case acc.RequiresReauth:
			accountStatusLine(c, l.Text(l10n.KeyAccountsNeedsReauth), sys(c.Theme(), sysRed))
		case acc.LastError != "":
			accountStatusLine(c, accountPrivate(priv, acc.LastError), sys(c.Theme(), sysOrange))
		}
		codexQuotaBlock(c, l, acc, current)
	})
}

func (a *App) codexCardHeader(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Gap(4).Grow(1).Basis(0).MinWidth(0).Children(func() {
			name := acc.ResolvedDisplayName()
			ui.Text(c, accountPrivate(priv, name)).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…")
			codexCardTags(c, l, acc, name, current, priv)
			codexIdentityDetails(c, l, acc, priv)
		})
		a.codexCardActions(c, l, acc, current, priv, view, ac)
	})
}

func codexCardTags(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, name string, current, priv bool) {
	t := c.Theme()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if current {
			runwayTagPadded(c, l.Text(l10n.KeyAccountsCurrent), toneGreen, 1, 5)
		}
		tier := acc.Tier()
		tierBadgePadded(c, codexTierLook(t, tier), l.Text(accountTierKey(tier)), 1, 5)
		if acc.Email != "" && acc.Email != name {
			ui.Text(c, accountPrivate(priv, acc.Email)).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…").Shrink(1)
		}
	})
}

// codexIdentityDetails is AccountIdentityDetailsLabel: the user id, and for
// organization plans the workspace id and name.
func codexIdentityDetails(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, priv bool) {
	rows := identityRows(l, acc)
	if len(rows) == 0 {
		return
	}
	ui.Column(c).Gap(2).Children(func() {
		for _, r := range rows {
			identityRow(c, r.label, accountPrivate(priv, r.value), r.mono)
		}
	})
}

type identityLine struct {
	label, value string
	mono         bool
}

func identityRows(l l10n.Localizer, acc codex.ManagedAccount) []identityLine {
	var rows []identityLine
	if id := acc.IdentityMarkerUserID(); id != "" {
		rows = append(rows, identityLine{l.Text(l10n.KeyAccountsUserIdLabel), id, true})
	}
	if !acc.DisplaysWorkspaceIdentity() {
		return rows
	}
	if id := acc.IdentityMarkerWorkspaceID(); id != "" {
		rows = append(rows, identityLine{l.Text(l10n.KeyAccountsWorkspaceIdLabel), id, true})
		if name := strings.TrimSpace(acc.WorkspaceName); name != "" {
			rows = append(rows, identityLine{l.Text(l10n.KeyAccountsWorkspaceNameLabel), name, false})
		}
	}
	return rows
}

func identityRow(c *ui.Context, label, value string, mono bool) {
	ink := inkSecondary(c.Theme())
	font := ""
	if mono {
		font = "monospace"
	}
	ui.RichText(c,
		ui.Span{Text: label, Size: fsCaption, Color: ink},
		ui.Span{Text: value, Size: fsCaption, Color: ink, Font: font},
	).Tooltip(label + value)
}

// codexCardActions are the tick, refresh and more controls of a card.
func (a *App) codexCardActions(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current, priv bool, view runway.AccountsView, ac AccountActions) {
	st := &a.panelState.accounts
	busy := view.Switching || st.busy
	refreshing := accountRefreshing(view.RefreshingIDs, acc.ID) || st.refreshing == acc.ID || st.refreshing == allRefreshing
	name := accountPrivate(priv, acc.ResolvedDisplayName())
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		title, icon := l.Text(l10n.KeyAccountsMakeCurrent), icCheckCircle
		if current {
			title, icon = l.Text(l10n.KeyAccountsIsCurrentLogin), icCheckCircleFill
		}
		tick := accountIconButton(c, icon, title, current || busy || refreshing || ac.Switch == nil, false, current)
		if tick.Clicked() {
			st.switchID, st.restart = acc.ID, codexRestartSupported
		}
		a.codexSwitchConfirm(c, l, tick, acc.ID, name, ac)

		if accountIconButton(c, icRefresh, l.Text(l10n.KeyRefresh), refreshing || ac.Refresh == nil, refreshing, false).Clicked() {
			id := acc.ID
			a.accountsRun(id, func() string { return ac.Refresh(id) })
		}
		more := accountIconButton(c, icMore, l.Text(l10n.KeyAccountsActions), busy || refreshing || ac.Delete == nil, false, false)
		more.Menu(func(m *ui.Menu) {
			if m.Item(l.Text(l10n.KeyAccountsDelete)).Chosen() {
				st.deleteID = acc.ID
			}
		})
		a.codexDeleteConfirm(c, l, more, acc.ID, name, ac)
	})
}

// codexSwitchConfirm is AccountSwitchConfirmSheet: the message, the restart
// choice and the confirmation.
func (a *App) codexSwitchConfirm(c *ui.Context, l l10n.Localizer, anchor ui.Element, id, name string, ac AccountActions) {
	st := &a.panelState.accounts
	open := st.switchID == id
	confirmPopover(c, l, anchor, &open, l.Text(l10n.KeyAccountsSwitchConfirmTitle), l.Textf(l10n.KeyAccountsSwitchConfirmMessage, name),
		l.Text(l10n.KeyAccountsMakeCurrent), func() { restartChoice(c, l, st) }, func() {
			restart := st.restart && codexRestartSupported
			a.accountsRun("", func() string { return ac.Switch(id, restart) })
		})
	if !open && st.switchID == id {
		st.switchID = ""
	}
}

func restartChoice(c *ui.Context, l l10n.Localizer, st *accountsState) {
	ui.Checkbox(c.Key("restart-after-switch"), &st.restart, l.Text(l10n.KeyAccountsRestartCodexAfterSwitch)).Disabled(!codexRestartSupported)
	if !codexRestartSupported {
		dimText(c, tx(l, xCodexRestartMac))
	}
}

func (a *App) codexDeleteConfirm(c *ui.Context, l l10n.Localizer, anchor ui.Element, id, name string, ac AccountActions) {
	st := &a.panelState.accounts
	open := st.deleteID == id
	confirmPopover(c, l, anchor, &open, l.Text(l10n.KeyAccountsDeleteConfirmTitle), name+"\n\n"+l.Text(l10n.KeyAccountsDeleteConfirmMessage),
		l.Text(l10n.KeyAccountsDelete), nil, func() {
			a.accountsRun("", func() string { return ac.Delete(id) })
		})
	if !open && st.deleteID == id {
		st.deleteID = ""
	}
}

// codexQuotaBlock is the meters of a card, or what stands in for them.
func codexQuotaBlock(c *ui.Context, l l10n.Localizer, acc codex.ManagedAccount, current bool) {
	if acc.IsRelay() {
		apiBalanceBlock(c, l, acc.APIQuota, current)
		return
	}
	if q := acc.CachedQuota; q != nil {
		ui.Column(c).Gap(8).Children(func() {
			for _, row := range q.MeterRows() {
				accountMeter(c, l, cachedMeter(l, row, current))
			}
		})
		return
	}
	if acc.AuthMode == codex.AuthModeAPIKey {
		dimText(c, l.Text(l10n.KeyAccountsAPIKeyHint)).MaxLines(2)
		return
	}
	calloutDim(c, l.Text(l10n.KeyNotLoaded)).FontSize(fsCaption)
}

func cachedMeter(l l10n.Localizer, row codex.CachedQuotaMeterRow, current bool) meterSpec {
	m := meterSpec{title: cachedRowTitle(l, row), remaining: row.RemainingPercent(), used: row.UsedPercent, current: current}
	if row.ResetsAt != nil {
		m.resetsAt, m.hasReset = *row.ResetsAt, true
	}
	return m
}

// cachedRowTitle follows the main panel: a window's own name first, then
// its length.
func cachedRowTitle(l l10n.Localizer, row codex.CachedQuotaMeterRow) string {
	if row.Name != "" {
		return row.Name
	}
	if row.WindowMinutes != nil {
		return runway.WindowTitle(l, *row.WindowMinutes)
	}
	return l.Text(l10n.KeyQuota)
}

func accountTierKey(tier codex.Tier) l10n.Key {
	switch tier {
	case codex.TierFree:
		return l10n.KeyPlanFree
	case codex.TierPlus:
		return l10n.KeyPlanPlus
	case codex.TierPro100:
		return l10n.KeyPlanPro100
	case codex.TierPro200:
		return l10n.KeyPlanPro200
	case codex.TierPro500:
		return l10n.KeyPlanPro500
	case codex.TierBusiness:
		return l10n.KeyPlanBusiness
	case codex.TierBusinessPremium:
		return l10n.KeyPlanBusinessPremium
	case codex.TierTeam:
		return l10n.KeyPlanTeam
	case codex.TierEnterprise:
		return l10n.KeyPlanEnterprise
	case codex.TierEdu:
		return l10n.KeyPlanEdu
	case codex.TierAPI:
		return l10n.KeyPlanAPI
	default:
		return l10n.KeyPlanUnknown
	}
}

func accountRefreshing(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// panelAccountStatus is the operation message of the settings accounts
// page: a spinner while a switch runs, else the message.
func panelAccountStatus(c *ui.Context, l l10n.Localizer, view runway.AccountsView) {
	t := c.Theme()
	if view.Switching {
		msg := view.Message
		if msg == "" {
			msg = l.Text(l10n.KeyAccountsSwitching)
		}
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 0, 6, 0).Children(func() {
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, msg).FontSize(11).TextColor(t.TextMuted)
		})
		return
	}
	if view.Message != "" {
		ui.Text(c, view.Message).FontSize(11).TextColor(t.TextMuted).Padding(0, 0, 6, 0)
	}
}
