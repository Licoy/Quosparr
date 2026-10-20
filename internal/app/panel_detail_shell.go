package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// detailTitle is the heading of a detail page.
func detailTitle(l l10n.Localizer, m store.Module) string {
	switch m {
	case detailAccounts:
		return l.Text(l10n.KeyAccounts)
	case store.ModuleResetCredits:
		return l.Text(l10n.KeyResetCreditDetails)
	case store.ModuleAPICost:
		return l.Text(l10n.KeyApiCost)
	case store.ModuleCacheAnalysis:
		return tx(l, xCacheTitle)
	case store.ModuleQuotaEstimate:
		return l.Text(l10n.KeyQuotaEstimate)
	case store.ModuleSpeedAnalysis:
		return tx(l, xSpeedTitle)
	case store.ModuleSkills:
		return tx(l, xSkillsTitle)
	}
	return ""
}

// closeDetail leaves the open detail page and cancels the work it started.
func (a *App) closeDetail() {
	a.cancelPanelDetails()
	a.panelState.accounts.closePage()
	a.panelState.detail = ""
}

// panelDetailPage replaces the whole main page: the back header, a
// hairline and the page body. Unknown pages fall back to the main page.
func (a *App) panelDetailPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	switch a.panelState.detail {
	case detailAccounts, store.ModuleResetCredits, store.ModuleCacheAnalysis, store.ModuleAPICost, store.ModuleQuotaEstimate, store.ModuleSpeedAnalysis, store.ModuleSkills:
	default:
		a.panelState.detail = ""
		a.panelMainPage(c, l, prefs, snap, acts)
		return
	}
	detailHeader(c, a, l)
	hairline(c)
	ui.Column(c).Grow(1).Padding(12, 16).Children(func() {
		a.panelDetailBody(c, l, prefs, snap, acts)
	})
}

// panelDetailBody is the page under the header. The accounts, skills and API cost
// pages keep their toolbar fixed and scroll only their list.
func (a *App) panelDetailBody(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, acts PanelActions) {
	snap.Provider = prefs.SelectedProvider
	switch m := a.panelState.detail; m {
	case detailAccounts:
		a.accountsPage(c, l, prefs, snap, acts)
	case store.ModuleSkills:
		a.skillsPage(c, l, prefs, acts)
	case store.ModuleAPICost:
		a.costDetailPage(c, l, prefs, snap, providerView(l, snap, prefs.APICostSummaryRange), acts)
	case store.ModuleCacheAnalysis:
		a.cacheDetailPage(c, l, acts)
	default:
		v := providerView(l, snap, prefs.APICostSummaryRange)
		ui.Scroll(c.Key("panel-detail-scroll")).Grow(1).TrackScroll(&a.panelState.detailScroll).Children(func() {
			ui.Column(c).Padding(4, 0).Children(func() { panelDetail(c, l, m, v) })
		})
	}
}

// detailHeader is the "back" header of a detail page: the chevron and
// label link, and the bold page title.
func detailHeader(c *ui.Context, a *App, l l10n.Localizer) {
	ui.Row(c).Padding(14, 16, 10, 16).Gap(8).AlignItems(ui.Center).Children(func() {
		if detailBackButton(c, l.Text(l10n.KeyBack)).Clicked() {
			if s := &a.panelState.skills; a.panelState.detail == store.ModuleSkills && s.open != "" {
				s.closeItem() // a skill page goes back to the list
				return
			}
			a.closeDetail()
			return
		}
		ui.Text(c, detailTitle(l, a.panelState.detail)).FontSize(fsTitle3).FontWeight(600).SingleLine()
	})
}

// detailBackButton is a quiet chevron + label button, primary on hover.
func detailBackButton(c *ui.Context, title string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(title).Padding(4, 8).Gap(4).Radius(radiusRow).AlignItems(ui.Center).Margin(0, 0, 0, -8).Cursor(ui.CursorPointer)
	ink := inkSecondary(t)
	if b.Hovered() {
		ink = t.Text
		b.Background(hoverNeutral(t))
	}
	return b.Children(func() {
		ui.Icon(c, icChevLeft).Size(11, 11).TextColor(ink)
		ui.Text(c, title).FontSize(fsCallout).FontWeight(500).TextColor(ink).SingleLine()
	})
}
