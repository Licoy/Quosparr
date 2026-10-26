package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
)

func (u *settingsUI) pricingBreadcrumb(c *ui.Context, r PricingRow) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		if detailBackButton(c, l.Text(l10n.KeyBack)).Label(l.Pricing("back")).Tooltip(l.Pricing("back")).Clicked() {
			s.open = ""
			return
		}
		ProviderIcon(c, s.provider, 14).Margin(0, 0, 0, 4)
		ui.Text(c, l.Text(providerKey(s.provider))).FontSize(13).TextColor(t.TextMuted).SingleLine()
		ui.Text(c, "/").FontSize(13).TextColor(t.TextMuted)
		ui.Text(c, r.Name).FontSize(15).FontWeight(650).SingleLine().Ellipsis("…").Shrink(1)
	})
}
func (u *settingsUI) pricingDetail(c *ui.Context, r PricingRow, snap PricingSnapshot, a PricingActions) {
	l := u.L()
	u.pricingModelOverview(c, r, snap, a)
	settingsSection(c, l.Pricing("tokenPrices"), l.Pricing("shortUnit"), nil, func() {
		for i, f := range []struct{ key, value string }{{"input", pricingValue(r.Input)}, {"cacheRead", pricingValue(r.CacheRead)}, {"output", pricingValue(r.Output)}, {"cacheWrite", pricingValue(r.CacheWrite)}} {
			settingsRule(c, i)
			pricingFact(c, l.Pricing(f.key), f.value, "")
		}
		ui.Text(c, l.Pricing("missing")).FontSize(11).TextColor(c.Theme().TextMuted).Padding(8, 0, 0, 0)
	})
	settingsSection(c, l.Pricing("pricingOrigin"), "", nil, func() {
		pricingFact(c, l.Pricing("source"), pricingSource(u, r), "")
		settingsRule(c, 1)
		pricingFact(c, l.Pricing("version"), pricingShortVersion(r.Version), r.Version)
	})
	settingsSection(c, l.Pricing("pricingRules"), l.Pricing("unit"), nil, func() {
		if r.Details != "" {
			ui.Text(c, r.Details).FontSize(12).LineHeight(1.45).Selectable()
		} else {
			ui.Text(c, l.Pricing("currentEstimate")).FontSize(12).TextColor(c.Theme().TextMuted)
		}
	})
}
func (u *settingsUI) pricingModelOverview(c *ui.Context, r PricingRow, snap PricingSnapshot, a PricingActions) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	Panel(c).Padding(16).Gap(12).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Start).Children(func() {
			ui.Row(c).Size(44, 44).Radius(10).Background(t.Text.Alpha(0.05)).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ProviderIcon(c, s.provider, 28) })
			ui.Column(c).Gap(5).Grow(1).Basis(0).Children(func() {
				ui.Text(c, r.Name).FontSize(18).FontWeight(700).Selectable()
				ui.Text(c, r.ID).FontSize(12).Font("monospace").TextColor(t.TextMuted).SingleLine().Ellipsis("…").Tooltip(r.ID).Selectable()
				ui.Row(c).Gap(6).Wrap().Children(func() {
					state, col := l.Pricing("reference"), t.TextMuted
					if r.Available {
						state, col = l.Pricing("available"), t.Success
					}
					settingsBadge(c, nil, state, col)
					if r.Input == nil || r.Output == nil {
						settingsBadge(c, icWarnCircle, l.Pricing("incomplete"), t.Warning)
					}
					if r.Builtin {
						settingsBadge(c, nil, l.Pricing("builtin"), t.TextMuted)
					}
				})
			})
		})
		ui.Box(c).Height(1).Background(pal(t).Line)
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			if source := pricingSafeSourceURL(r.SourceURL); source != "" {
				if SmallPillButton(c, icExternal, l.Pricing("openSource"), false).Clicked() {
					c.OpenURL(source)
				}
			}
			u.pricingSyncButton(c, snap, a, false)
			if IconButton(c, icSliders, l.Pricing("syncSettings"), false).Clicked() {
				s.syncSettings = true
			}
		})
	})
}
func pricingFact(c *ui.Context, label, value, tooltip string) {
	ui.Row(c).Gap(14).Padding(8, 0).MinHeight(34).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(118).Shrink(0).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Text(c, pricingOrDash(value)).Tooltip(tooltip).FontSize(12).Grow(1).Basis(0).Selectable()
	})
}
