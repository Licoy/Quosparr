package app

import (
	"cmp"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func (u *settingsUI) pricingPage(c *ui.Context) {
	s, a := u.pricingState(), u.pricingActions()
	snap := PricingSnapshot{}
	if a.Snapshot != nil {
		snap = a.Snapshot()
	}

	detail, open := s.detailRow(snap)
	if s.open != "" && !open {
		s.open = ""
	}
	key := "model-pricing-list-" + pricingProviderID(s.provider)
	if open {
		key = "model-pricing-detail-" + s.open
	}
	rows := s.cachedList(snap) // first, for the counts over the list
	splitPage(c, key, func() {
		if open {
			u.pricingBreadcrumb(c, detail)
			u.pricingNotice(c, snap)
			return
		}
		u.pricingProviderTabs(c, snap, a)
		u.pricingFilters(c, snap)
		u.pricingListHeader(c)
	}, func() {
		if open {
			u.pricingDetail(c, detail, snap, a)
			return
		}

		page := s.pageRows(rows)
		if len(rows) == 0 {
			skillsEmptyBox(c, u.L().Pricing("empty"))
			return
		}
		listPanel(c).Children(func() {
			for _, r := range page {
				u.pricingRow(c, r)
			}
		})
		ui.Row(c).Gap(12).Children(func() {
			if len(rows) <= 100 {
				ui.Text(c, u.L().PricingRule("modelCount", len(rows))).FontSize(12).TextColor(c.Theme().TextMuted)
				return
			}
			if ui.Button(c, u.L().Pricing("previous")).Disabled(s.page == 0).Clicked() {
				s.page--
			}
			ui.Text(c, fmt.Sprintf("%d–%d / %d", s.page*100+1, s.page*100+len(page), len(rows))).FontSize(12)
			if ui.Button(c, u.L().Pricing("next")).Disabled((s.page+1)*100 >= len(rows)).Clicked() {
				s.page++
			}
		})
	})
}
func (u *settingsUI) savePricingInterval(raw string) bool {
	s := u.pricingState()
	n, err := strconv.Atoi(raw)
	if err != nil || n < 15 || n > 10080 {
		s.validation = u.L().Pricing("invalid")
		return false
	}
	s.validation = ""
	return u.s.Update(func(p *store.Preferences) { p.ModelPricing.SyncIntervalSeconds = n * 60 })
}
func pricingTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}
func pricingOrDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// pricingFilters is the search and order, then the state filters with
// their counts and the sync state.
func (u *settingsUI) pricingFilters(c *ui.Context, snap PricingSnapshot) {
	s, l := u.pricingState(), u.L()
	Panel(c).Padding(8, 10).Gap(8).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("pricing-search"), &s.search).Placeholder(l.Pricing("search")).Label(l.Pricing("search")).
				FontSize(12).Height(listControlHeight).Grow(1).Basis(0).MinWidth(160)
			order := s.sort
			if order == "" {
				order = "name"
			}
			listMenuButton(c, "pricing-sort", icUpDown, l.Pricing(order), l.Pricing(order), false, func(m *ui.Menu) {
				for _, id := range []string{"name", "input", "output"} {
					pricingMenuItem(m, l.Pricing(id), order == id, func() { s.sort = id; s.page = 0 })
				}
			})
		})
		ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
			u.pricingStates(c)
			ui.Spacer(c).Grow(1)
			u.pricingStatus(c, snap, true)
		})
		u.pricingSyncError(c, snap)
	})
}

// pricingStates is a segmented row of the state filters, each with its
// count under the search.
func (u *settingsUI) pricingStates(c *ui.Context) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	ui.Row(c).Padding(2).Gap(2).Radius(7).Background(t.Text.Alpha(0.06)).AlignItems(ui.Center).Children(func() {
		for i, id := range pricingFilters {
			name := l.Pricing(cmp.Or(id, "all"))
			selected := s.filter == id
			b := ui.ButtonBase(c.Key("pricing-state-"+cmp.Or(id, "all"))).Label(name).Role(ui.RoleTab).Selected(selected).
				Padding(4, 9).Gap(5).Radius(5).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			col := t.TextMuted
			switch {
			case selected:
				col = t.Text
				b.Background(pal(t).Inset).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12))
			case b.Hovered():
				b.Background(t.Text.Alpha(0.06))
			}
			b.Children(func() {
				ui.Text(c, name).FontSize(12).FontWeight(500).TextColor(col).SingleLine()
				ui.Text(c, fmt.Sprint(s.counts[i])).FontSize(11).FontWeight(600).FontFeatures("tnum").TextColor(pricingStateColor(t, id)).SingleLine()
			})
			if b.Clicked() {
				s.filter = id
				s.page = 0
			}
		}
	})
}

func pricingStateColor(t *ui.Theme, filter string) ui.Color {
	switch filter {
	case "":
		return t.Accent
	case "available":
		return t.Success
	case "incomplete":
		return t.Warning
	}
	return t.TextMuted
}

// pricingColumn is a price column: its label, icon and the icon's color,
// the colors of the same token kinds on the cost page.
type pricingColumn struct {
	key   string
	icon  *ui.SVG
	color ui.Color
}

func pricingColumns(t *ui.Theme) []pricingColumn {
	return []pricingColumn{{"input", icTokenIn, sys(t, sysBlue)}, {"cacheRead", icDatabase, sys(t, sysGreen)}, {"output", icTokenOut, sys(t, sysOrange)}}
}

const (
	pricingNumberWidth  float32 = 92
	pricingTileSize     float32 = 28
	pricingChevronWidth float32 = 12
)

func (u *settingsUI) pricingListHeader(c *ui.Context) {
	l, t := u.L(), c.Theme()
	listBar(c).Children(func() {
		ui.Row(c).Gap(5).Grow(1).Basis(0).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icCpu).Size(12, 12).TextColor(t.TextMuted)
			ui.Text(c, l.Pricing("model")).FontSize(11).FontWeight(600).TextColor(t.TextMuted).SingleLine().Shrink(0)
			ui.Text(c, "· "+l.Pricing("shortUnit")).FontSize(11).TextColor(t.TextMuted.Alpha(0.75)).SingleLine().Ellipsis("…").Shrink(1).Tooltip(l.Pricing("unit"))
		})
		for _, col := range pricingColumns(t) {
			ui.Row(c).Gap(4).Width(pricingNumberWidth).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, col.icon).Size(12, 12).TextColor(col.color)
				name := l.Pricing(col.key)
				ui.Text(c, name).FontSize(11).FontWeight(600).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1).Tooltip(name)
			})
		}
		ui.Box(c).Width(pricingChevronWidth)
	})
}
func (u *settingsUI) pricingRow(c *ui.Context, r PricingRow) {
	s, l, t := u.pricingState(), u.L(), c.Theme()
	key := r.ProviderID + "/" + r.ID
	listRow(c, "pricing-row-"+key, false).Children(func() {
		b := ui.ButtonBase(c.Key("pricing-open-" + key)).Label(r.Name).Grow(1).Basis(0).Gap(10).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		b.Children(func() {
			ui.Row(c).Size(pricingTileSize, pricingTileSize).Radius(7).Background(t.Text.Alpha(0.05)).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
				Children(func() { ProviderIcon(c, s.provider, 16) })
			ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
				ui.Text(c, r.Name).FontSize(13).FontWeight(500).SingleLine().Ellipsis("…")
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, r.ID).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1)
					pricingRowBadges(c, l, r)
				})
			})
			for _, v := range []*decimal.Decimal{r.Input, r.CacheRead, r.Output} {
				col := t.Text
				if v == nil {
					col = t.TextMuted
				}
				ui.Text(c, pricingValue(v)).FontSize(12).FontWeight(500).FontFeatures("tnum").TextColor(col).Width(pricingNumberWidth).TextAlign(ui.End).SingleLine().Ellipsis("…")
			}
			ui.Icon(c, icChevRight).Size(pricingChevronWidth, pricingChevronWidth).TextColor(t.TextMuted)
		})
		if b.Clicked() {
			s.open = key
		}
	})
}

// pricingRowBadges marks a row that is not simply available: missing
// prices, reference only, and built-in prices.
func pricingRowBadges(c *ui.Context, l l10n.Localizer, r PricingRow) {
	t := c.Theme()
	switch {
	case r.Input == nil || r.Output == nil:
		settingsBadge(c, icWarnCircle, l.Pricing("incomplete"), t.Warning)
	case !r.Available:
		settingsBadge(c, nil, l.Pricing("reference"), t.TextMuted)
	}
	if r.Builtin {
		settingsBadge(c, nil, l.Pricing("builtin"), t.TextMuted)
	}
}
func pricingSource(u *settingsUI, r PricingRow) string {
	if r.Source == "builtin" || r.Builtin {
		return u.L().Pricing("builtin")
	}
	return r.Source
}

func pricingMenuItem(m *ui.Menu, label string, checked bool, fn func()) {
	if m.Item(label).Checked(checked).Chosen() {
		fn()
	}
}

func pricingIntervalLabel(u *settingsUI, seconds int) string {
	if seconds%3600 == 0 {
		return u.L().PricingRule("everyHours", seconds/3600)
	}
	return u.L().PricingRule("everyMinutes", seconds/60)
}
func pricingShortVersion(version string) string {
	if len(version) > 32 {
		return version[:24] + "…"
	}
	return pricingOrDash(version)
}

func pricingSafeSourceURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.String()
}
