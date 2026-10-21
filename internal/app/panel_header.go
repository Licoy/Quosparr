package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// panelHeader is the fixed header of the main page: the title, the header
// buttons, the provider strip, the identity row and the subscription chip.
func panelHeader(c *ui.Context, a *App, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	grokView := prefs.SelectedProvider == store.ProviderGrok
	ui.Column(c).Padding(14, 16, 10, 16).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				// Center on the cap height rather than the line box, which reserves
				// room for descenders below the glyphs.
				ui.Image(c, appIconBitmap()).Size(20, 20).Margin(2, 0, 0, 0)
				ui.Text(c, "Quosparr").FontSize(fsTitle3).FontWeight(600).SingleLine()
			})
			ui.Spacer(c).Grow(1)
			panelHeaderButtons(c, a, l, prefs, acts, panelBusy(snap, grokView))
		})
		if providers := prefs.PanelSelection(); len(providers) > 1 {
			providerStrip(c, a, l, snap, providers, prefs.SelectedProvider, acts.SelectProvider)
		}
		if prefs.SelectedProvider == store.ProviderCursor {
			cursorIdentityRow(c, l, prefs, snap.Cursor)
			return
		}
		if grokView {
			grokIdentityRow(c, a, l, prefs, snap)
			return
		}
		if prefs.SelectedProvider == store.ProviderClaude {
			claudeIdentityRow(c, a, l, prefs, snap)
			return
		}
		codexIdentityRow(c, a, l, prefs, snap)
		if exp := snap.Accounts.Display.SubscriptionExpiresAt; exp != nil {
			ui.Row(c).Padding(6, 0, 0, 0).Children(func() { expiryBadge(c, l, *exp, time.Now()) })
		}
	})
}

func panelBusy(snap runway.Snapshot, grokView bool) bool {
	if snap.Provider == store.ProviderCursor {
		return snap.Cursor.Refreshing
	}
	if grokView && snap.Grok.Refreshing {
		return true
	}
	if snap.Provider == store.ProviderClaude && snap.Claude.Refreshing {
		return true
	}
	return snap.Refresh.Refreshing()
}

// providerStrip splits the width evenly among the shown providers, and is
// left out when only one is shown: each tab is the
// logo and name over the remaining share of the account's shortest quota
// window. The selected tab sits on a gray fill in full color; the others are
// monochrome and gray. With no SelectProvider action the tabs stay disabled.
func providerStrip(c *ui.Context, a *App, l l10n.Localizer, snap runway.Snapshot, providers []store.Provider, cur store.Provider, selectFn func(store.Provider)) {
	t := c.Theme()
	ui.Row(c).Gap(6).Padding(10, 0, 0, 0).Children(func() {
		for _, p := range providers {
			name := l.Text(providerKey(p))
			selected := p == cur
			tab := ui.ButtonBase(c.Key("provider-tab-"+string(p))).Label(name).Disabled(selectFn == nil).
				Grow(1).Basis(0).Padding(6, 10, 7, 10).Radius(radiusRow).Cursor(ui.CursorPointer)
			switch {
			case selected && tab.Hovered():
				tab.Background(mono(t, 0.10, 0.15))
			case selected:
				tab.Background(mono(t, 0.07, 0.11))
			case tab.Hovered():
				tab.Background(hoverNeutral(t))
			}
			tab.Children(func() {
				ui.Column(c).Grow(1).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						if selected {
							ProviderIcon(c, p, 16).TextColor(t.Text)
							ui.Text(c, name).FontSize(fsCallout).FontWeight(600).SingleLine()
							return
						}
						ProviderMonoIcon(c, p, 16).TextColor(inkTertiary(t))
						ui.Text(c, name).FontSize(fsCallout).TextColor(inkTertiary(t)).SingleLine()
					})
					meter, ok := runway.ShortestWindowMeter(providerMeters(snap, p))
					stripBar(c, meter, ok, selected)
				})
			})
			if tab.Clicked() && selectFn != nil && !selected {
				a.panelState.detail = ""
				selectFn(p)
			}
		}
	})
}

func providerMeters(snap runway.Snapshot, p store.Provider) []runway.Meter {
	switch p {
	case store.ProviderCursor:
		return snap.Cursor.LiveMeters()
	case store.ProviderGrok:
		return snap.Grok.Meters
	case store.ProviderClaude:
		return snap.Claude.Meters
	default:
		return snap.Codex.Meters
	}
}

// stripBar is the bare quota bar of a provider tab: the remaining share in
// the health color, gray when not selected, or the empty track while there
// is no reading.
func stripBar(c *ui.Context, m runway.Meter, ok, selected bool) {
	t := c.Theme()
	fill := mono(t, 0.22, 0.28)
	if selected {
		fill = healthFill(t, m.Health)
	}
	ui.Box(c).Height(4).AlignSelf(ui.Stretch).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(r, surfaceSunken(t), r.H/2)
		if !ok {
			return
		}
		w := max(4, r.W*float32(m.RemainingPercent)/100)
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: min(w, r.W), H: r.H}, fill, r.H/2)
	})
}

func panelHeaderButtons(c *ui.Context, a *App, l l10n.Localizer, prefs store.Preferences, acts PanelActions, busy bool) {
	ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
		privacy := prefs.AccountPrivacyModeEnabled
		if privacyButton(c, l, privacy, 26, 24, radiusControl).Clicked() {
			a.settings.SetAccountPrivacyMode(!privacy)
		}
		if a.settings.System().PanelBackground {
			bg := prefs.MainPanelBackgroundStyle
			next := pick(bg == store.PanelBackgroundOpaque, store.PanelBackgroundTranslucent, store.PanelBackgroundOpaque)
			title := l.Text(l10n.KeyMainPanelBackground) + " · " + l.Text(panelBackgroundKey(bg))
			if toolButton(c, pick(bg == store.PanelBackgroundOpaque, icOpaque, icTranslucent), title, 26, 24, 14, false).Clicked() {
				a.settings.SetPanelBackground(next)
			}
		}
		floating := prefs.FloatingWindow.Enabled
		if toggleIconButton(c, icFloating, tx(l, xFloatingEnabled), floating, 26, 24, radiusControl).Clicked() {
			a.settings.SetFloatingWindow(func(f *store.FloatingWindow) { f.Enabled = !floating })
		}
		check := a.opts.Actions.CheckForUpdates
		if toolButton(c, icCloudDown, l.Text(l10n.KeyCheckForUpdates), 26, 24, 14, check == nil).Clicked() && check != nil {
			check()
		}
		if toolButton(c, icGithub, "GitHub", 26, 24, 14, false).Clicked() {
			c.OpenURL(githubURL)
		}
		if busy {
			ui.Row(c).Size(26, 24).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c).Size(14, 14) })
			return
		}
		if toolButton(c, icRefresh, l.Text(l10n.KeyRefresh), 26, 24, 14, acts.Refresh == nil).Clicked() && acts.Refresh != nil {
			refreshEverything(a, acts)
		}
	})
}

// refreshEverything is the header refresh: all sections, plus the open
// cost query or reaction when a page that shows them is open.
func refreshEverything(a *App, acts PanelActions) {
	if a.settings.Prefs().SelectedProvider == store.ProviderCursor {
		if acts.RefreshSection != nil {
			acts.RefreshSection(runway.SectionQuota)
		}
		return
	}
	acts.Refresh()
	if a.settings.Prefs().ShowsCacheAnalysis && acts.QueryCache != nil {
		a.startCacheSummary(acts)
		if a.panelState.detail == store.ModuleCacheAnalysis && !a.panelState.cache.detail.isToday() {
			a.startCacheDetail(a.panelState.cache.detail.queryValue(), acts)
		}
	}
	if a.panelState.detail == store.ModuleAPICost && a.panelState.cost.query != nil {
		a.startCostQuery(*a.panelState.cost.query, acts)
	}
}

// headerIdentity is the name of the signed-in account; privacy mode masks
// it as soon as there is a real identity to mask.
func headerIdentity(l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) string {
	if prefs.SelectedProvider == store.ProviderGrok {
		return grokIdentity(l, prefs, snap)
	}
	if prefs.SelectedProvider == store.ProviderClaude {
		return claudeIdentity(l, prefs, snap)
	}
	name := snap.Accounts.Display.DisplayName
	if name == "" {
		name = activeManagedName(snap.Accounts)
	}
	if name == "" {
		if snap.Accounts.Display.Authenticated {
			return l.Text(l10n.KeyUnknownAccount)
		}
		return l.Text(l10n.KeyNotLoggedIn)
	}
	if prefs.AccountPrivacyModeEnabled {
		return codex.MaskAccountText(name)
	}
	return name
}

func activeManagedName(v runway.AccountsView) string {
	for _, acc := range v.Sidebar {
		if acc.ID == v.ActiveID {
			return acc.ResolvedDisplayName()
		}
	}
	return ""
}

func grokIdentity(l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) string {
	name := snap.Grok.Accounts.Display.DisplayName
	if name == "" {
		return l.Text(l10n.KeyGrokNotLoggedIn)
	}
	if prefs.AccountPrivacyModeEnabled {
		return codex.MaskAccountText(name)
	}
	return name
}

// identityPill is the hover pill around the identity row; the negative
// margin keeps the badge on the 16pt grid while the pill breathes.
func identityPill(c *ui.Context, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Padding(3, 6).Gap(6).Radius(radiusRow).AlignItems(ui.Center).Justify(ui.Start).Margin(8, -6, 0, -6).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(hoverNeutral(t))
	}
	return b
}

func identityChevron(c *ui.Context) {
	ui.Icon(c, icChevRight).Size(10, 10).TextColor(inkTertiary(c.Theme()))
}

func codexIdentityRow(c *ui.Context, a *App, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) {
	tier := snap.Accounts.Display.Tier
	look := codexTierLook(c.Theme(), tier)
	name := headerIdentity(l, prefs, snap)
	row := identityPill(c, l.Text(l10n.KeyAccounts))
	row.Children(func() {
		tierBadge(c, look, l.Text(accountTierKey(tier)))
		tierText(c, look, name).MaxWidth(200)
		identityChevron(c)
	})
	if row.Clicked() {
		a.panelState.detail = detailAccounts
	}
}

func claudeIdentity(l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) string {
	name := snap.Claude.Accounts.Display.DisplayName
	if name == "" {
		return l.Text(l10n.KeyClaudeNotLoggedIn)
	}
	if prefs.AccountPrivacyModeEnabled {
		return codex.MaskAccountText(name)
	}
	return name
}

func claudeIdentityRow(c *ui.Context, a *App, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) {
	name := claudeIdentity(l, prefs, snap)
	t := c.Theme()
	row := ui.ButtonBase(c).Label(name).Tooltip(name).Padding(3, 6).Gap(6).Radius(radiusRow).AlignItems(ui.Center).Justify(ui.Start).Margin(8, -6, 0, -6).Cursor(ui.CursorPointer)
	if row.Hovered() {
		row.Background(hoverNeutral(t))
	}
	row.Children(func() {
		claudePlanBadge(c, l, snap.Claude.Plan)
		ui.Text(c, name).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…").MaxWidth(220)
		identityChevron(c)
		ui.Spacer(c).Grow(1)
	})
	if row.Clicked() {
		a.panelState.detail = detailAccounts
	}
}

func claudePlanBadge(c *ui.Context, l l10n.Localizer, plan string) {
	if plan == "" {
		tierBadge(c, plainLook(c.Theme()), l.Text(l10n.KeyProviderClaude))
		return
	}
	tierBadge(c, claudeTierLook(c.Theme(), claude.TierResolve(plan)), plan)
}

func grokIdentityRow(c *ui.Context, a *App, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot) {
	row := identityPill(c, l.Text(l10n.KeyGrokAccountsTitle))
	row.Children(func() {
		grokPlanBadge(c, l, snap.Grok.Plan)
		ui.Text(c, headerIdentity(l, prefs, snap)).FontSize(fsCallout).FontWeight(600).SingleLine().Ellipsis("…").MaxWidth(220)
		identityChevron(c)
	})
	if row.Clicked() {
		a.panelState.detail = detailAccounts
	}
}

// grokPlanBadge is the tier plate of a Grok plan, or the neutral provider
// tag while the plan is unknown.
func grokPlanBadge(c *ui.Context, l l10n.Localizer, plan string) {
	if plan == "" {
		tierBadge(c, plainLook(c.Theme()), l.Text(l10n.KeyProviderGrok))
		return
	}
	kind := grok.TierResolve(plan)
	tierBadge(c, grokTierLook(c.Theme(), kind), kind.DisplayName())
}
