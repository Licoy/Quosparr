package app

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
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

		rows := s.cachedList(snap)
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
func (u *settingsUI) pricingFilters(c *ui.Context, snap PricingSnapshot) {
	s, l := u.pricingState(), u.L()
	Panel(c).Padding(8, 10).Gap(6).Children(func() {
		ui.TextInput(c.Key("pricing-search"), &s.search).Placeholder(l.Pricing("search")).Label(l.Pricing("search")).Height(listControlHeight)
		ui.Row(c).Gap(8).Children(func() {
			filter := s.filter
			if filter == "" {
				filter = "all"
			}
			listMenuButton(c, "pricing-filter", nil, l.Pricing(filter), l.Pricing(filter), false, func(m *ui.Menu) {
				for _, id := range []string{"all", "available", "reference", "incomplete", "builtin"} {
					pricingMenuItem(m, l.Pricing(id), filter == id, func() {
						s.filter = id
						if id == "all" {
							s.filter = ""
						}
						s.page = 0
					})
				}
			})
			order := s.sort
			if order == "" {
				order = "name"
			}
			listMenuButton(c, "pricing-sort", nil, l.Pricing(order), l.Pricing(order), false, func(m *ui.Menu) {
				for _, id := range []string{"name", "input", "output"} {
					pricingMenuItem(m, l.Pricing(id), order == id, func() { s.sort = id; s.page = 0 })
				}
			})
			ui.Spacer(c).Grow(1)
			ui.Text(c, l.Pricing("shortUnit")).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		u.pricingNotice(c, snap)
	})
}

const pricingNumberWidth float32 = 76

func (u *settingsUI) pricingListHeader(c *ui.Context) {
	l := u.L()
	listBar(c).Children(func() {
		ui.Text(c, l.Pricing("model")).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1).Basis(0)
		for _, key := range []string{"input", "cacheRead", "output"} {
			ui.Text(c, l.Pricing(key)).FontSize(11).TextColor(c.Theme().TextMuted).Width(pricingNumberWidth).TextAlign(ui.End)
		}
	})
}
func (u *settingsUI) pricingRow(c *ui.Context, r PricingRow) {
	key := r.ProviderID + "/" + r.ID
	listRow(c, "pricing-row-"+key, false).Children(func() {
		b := ui.ButtonBase(c.Key("pricing-open-" + key)).Label(r.Name).Grow(1).Basis(0).Gap(10).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		b.Children(func() {
			ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
				ui.Text(c, r.Name).FontSize(13).SingleLine().Ellipsis("…")
				ui.Text(c, r.ID).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…")
			})
			for _, value := range []string{pricingValue(r.Input), pricingValue(r.CacheRead), pricingValue(r.Output)} {
				ui.Text(c, value).FontSize(12).FontFeatures("tnum").Width(pricingNumberWidth).TextAlign(ui.End).SingleLine().Ellipsis("…")
			}
		})
		if b.Clicked() {
			u.pricingState().open = key
		}
	})
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
