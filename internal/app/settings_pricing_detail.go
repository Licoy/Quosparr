package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/usage/decimal"
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
	l, t := u.L(), c.Theme()
	u.pricingModelOverview(c, r, snap, a)
	settingsSection(c, l.Pricing("tokenPrices"), l.Pricing("unit"), nil, func() {
		pricingPriceStrip(c, l, r)
		ui.Box(c).Height(1).Background(pal(t).Line)
		ui.Text(c, l.Pricing("missing")).FontSize(11).TextColor(t.TextMuted).Padding(8, 0)
	})
	settingsSection(c, l.Pricing("pricingOrigin"), "", nil, func() {
		pricingFact(c, icLink, l.Pricing("source"), pricingSource(u, r), "")
		settingsRule(c, 1)
		pricingFact(c, icPriceTag, l.Pricing("version"), pricingShortVersion(r.Version), r.Version)
	})
	if r.Details != "" {
		settingsSection(c, l.Pricing("pricingRules"), "", nil, func() {
			ui.Text(c, r.Details).FontSize(12).LineHeight(1.45).Padding(8, 0).Selectable()
		})
	}
}

// pricingPriceStrip is the four token prices side by side, each under its
// column's icon.
func pricingPriceStrip(c *ui.Context, l l10n.Localizer, r PricingRow) {
	t := c.Theme()
	cols := append(pricingColumns(t), pricingColumn{"cacheWrite", icSave, sys(t, sysPurple)})
	values := []*decimal.Decimal{r.Input, r.CacheRead, r.Output, r.CacheWrite}
	ui.Row(c).Padding(10, 0).AlignItems(ui.Center).Children(func() {
		for i, col := range cols {
			if i > 0 {
				ui.Box(c).Size(1, 34).Margin(0, 12).Background(pal(t).Line)
			}
			ui.Column(c).Gap(6).Grow(1).Basis(0).Children(func() {
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, col.icon).Size(13, 13).TextColor(col.color)
					ui.Text(c, l.Pricing(col.key)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1)
				})
				value := ui.Text(c, pricingValue(values[i])).FontSize(17).FontWeight(650).FontFeatures("tnum").SingleLine().Selectable()
				if values[i] == nil {
					value.TextColor(t.TextMuted)
				}
			})
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
					state, col, icon := l.Pricing("reference"), t.TextMuted, (*ui.SVG)(nil)
					if r.Available {
						state, col, icon = l.Pricing("available"), t.Success, icCheckCircle
					}
					settingsBadge(c, icon, state, col)
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
func pricingFact(c *ui.Context, icon *ui.SVG, label, value, tooltip string) {
	t := c.Theme()
	ui.Row(c).Gap(14).Padding(8, 0).MinHeight(34).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(6).Width(118).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icon).Size(13, 13).TextColor(t.TextMuted)
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1)
		})
		ui.Text(c, pricingOrDash(value)).Tooltip(tooltip).FontSize(12).Grow(1).Basis(0).Selectable()
	})
}
