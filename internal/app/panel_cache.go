package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func panelCache(c *ui.Context, l l10n.Localizer, _ runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	a := st.cacheOwner
	if a == nil {
		return
	}
	s := &st.cache.summary
	sectionBlock(c, first, func() {
		var refresh func()
		if acts.QueryCache != nil {
			refresh = func() { a.startCacheSummary(acts) }
		}
		drawSectionHead(c, l, st, sectionSpec{
			title: tx(l, xCacheTitle), refreshing: s.task.Running,
			refresh: refresh,
		})
		switch {
		case s.task.Running && s.report == nil:
			sectionEmpty(c, l, true)
		case s.task.Err != nil:
			ui.Text(c, detailErrorText(l, s.task.Err)).FontSize(fsCallout).TextColor(sys(c.Theme(), sysRed))
		case s.report != nil:
			cacheSummaryContent(c, l, *s.report)
		default:
			calloutDim(c, tx(l, xCacheNoData))
		}
		if disclosureRow(c, nil, tx(l, xCacheViewDetails), true).Clicked() {
			st.detail = store.ModuleCacheAnalysis
		}
	})
}

func cacheSummaryContent(c *ui.Context, l l10n.Localizer, r cachehit.Report) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
		rate := ui.Text(c, cachePercent(r.Totals)).FontSize(fsTitle3).FontWeight(600).FontFeatures("tnum")
		if h, ok := cacheHealth(r.Totals); ok {
			rate.TextColor(healthInk(t, h))
		}
		calloutDim(c, tx(l, xCacheHitRate))
	})
	ui.Text(c, tx(l, xCacheTokenRatio, tokenCompact(int(r.Totals.CachedInputTokens)), tokenCompact(int(r.Totals.InputTokens)))).
		FontSize(fsCallout).TextColor(inkSecondary(t)).FontFeatures("tnum")
	ui.Text(c, cacheRangeText(l, r)).FontSize(fsCaption).TextColor(inkTertiary(t)).SingleLine().Ellipsis("…")
	cacheGapsText(c, l, r)
}

func cachePercent(t cachehit.Totals) string {
	p, ok := t.Percent()
	if !ok {
		return "—"
	}
	if p == float64(int(p)) {
		return fmt.Sprintf("%.0f%%", p)
	}
	return fmt.Sprintf("%.1f%%", p)
}

// cacheHealth grades a hit rate: above 90% green, from 80% yellow, below
// red. ok is false without input, where the rate is a dash.
func cacheHealth(t cachehit.Totals) (runway.Health, bool) {
	p, ok := t.Percent()
	switch {
	case !ok:
		return runway.HealthGreen, false
	case p > 90:
		return runway.HealthGreen, true
	case p >= 80:
		return runway.HealthYellow, true
	}
	return runway.HealthRed, true
}

// sessionCacheTotals is a session's whole input, of which the cached part
// was read from the cache.
func sessionCacheTotals(t sessions.Totals) cachehit.Totals {
	return cachehit.Totals{InputTokens: int64(t.UncachedInputTokens) + int64(t.CachedInputTokens), CachedInputTokens: int64(t.CachedInputTokens)}
}

func cacheRangeText(l l10n.Localizer, r cachehit.Report) string {
	if r.Start.IsZero() || r.End.IsZero() {
		return tx(l, xCacheLocalSource)
	}
	rangeValue := usage.RangeForWindow(usage.Window{Start: r.Start, End: r.End}, time.Local)
	if rangeValue.APIStartDate == rangeValue.APIEndDate {
		return tx(l, xCacheSingleDaySource, rangeValue.APIStartDate)
	}
	return tx(l, xCacheRangeSource, rangeValue.APIStartDate, rangeValue.APIEndDate)
}

// cacheGapsText notes the records left out of the report: those without cache
// data and the log lines that could not be read.
func cacheGapsText(c *ui.Context, l l10n.Localizer, r cachehit.Report) {
	if n := r.Totals.MissingSamples; n > 0 {
		ui.Text(c, tx(l, xCacheMissingSamples, n)).FontSize(fsCaption).TextColor(sys(c.Theme(), sysOrange))
	}
	if n := r.UnreadLines; n > 0 {
		ui.Text(c, tx(l, xCacheUnreadLines, n)).FontSize(fsCaption).TextColor(sys(c.Theme(), sysOrange))
	}
}

func (a *App) cacheDetailPage(c *ui.Context, l l10n.Localizer, acts PanelActions) {
	a.syncCacheDetail(acts)
	s := &a.panelState.cache
	ui.Column(c).Gap(12).Grow(1).Children(func() {
		a.cacheRangeTabs(c, l, acts)
		if s.selected == costCustomIndex && !s.detail.task.Running {
			a.customCacheControls(c, l, acts)
		}
		if s.detail.task.Running {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() { sectionEmpty(c, l, true) })
			return
		}
		ui.Scroll(c.Key("cache-detail-scroll")).Grow(1).Children(func() {
			ui.Column(c).Gap(14).Children(func() { cacheDetailContent(c, l, s) })
		})
	})
}

func (a *App) cacheRangeTabs(c *ui.Context, l l10n.Localizer, acts PanelActions) {
	s := &a.panelState.cache
	t := c.Theme()
	cardColumn(c, surfaceSunkenKind, radiusRow+1).Padding(2).Children(func() {
		ui.Row(c).Gap(1).Children(func() {
			for i, label := range costRangeLabels(l) {
				tab := ui.ButtonBase(c.Key("cache-tab-"+strconv.Itoa(i))).Label(label).Tooltip(label).Grow(1).Basis(0).Padding(5, 4).Radius(radiusControl).
					AlignItems(ui.Center).Justify(ui.Center).Disabled(acts.QueryCache == nil).Cursor(ui.CursorPointer)
				weight, ink := 400, inkSecondary(t)
				if i == s.selected {
					tab.Background(t.Accent)
					weight, ink = 600, t.AccentText
				} else if tab.Hovered() {
					tab.Background(t.Text.Alpha(0.05))
				}
				tab.Children(func() {
					ui.Text(c, label).FontSize(fsCallout).FontWeight(weight).TextColor(ink).SingleLine().Ellipsis("…")
				})
				if tab.Clicked() {
					a.selectCacheRange(i, acts)
				}
			}
		})
	})
}

func (a *App) customCacheControls(c *ui.Context, l l10n.Localizer, acts PanelActions) {
	s := &a.panelState.cache
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
		costDateField(c, l.Text(l10n.KeyStartDate), "cache-start", &s.start)
		costDateField(c, l.Text(l10n.KeyEndDate), "cache-end", &s.end)
		calc := ui.ButtonBase(c).Label(l.Text(l10n.KeyCalculate)).Padding(7, 14).MinWidth(68).Radius(radiusRow).AlignItems(ui.Center).Justify(ui.Center).
			Disabled(acts.QueryCache == nil).Cursor(ui.CursorPointer).Background(t.Accent)
		calc.Children(func() {
			ui.Text(c, l.Text(l10n.KeyCalculate)).FontSize(fsCallout).FontWeight(600).TextColor(t.AccentText).SingleLine()
		})
		if calc.Clicked() {
			a.calculateCustomCache(acts)
		}
	})
}

func cacheDetailContent(c *ui.Context, l l10n.Localizer, s *panelCacheState) {
	q := &s.detail
	if q.task.Err != nil {
		ui.Text(c, detailErrorText(l, q.task.Err)).FontSize(fsCallout).TextColor(sys(c.Theme(), sysRed))
		return
	}
	if q.report == nil {
		calloutDim(c, tx(l, xCacheNoData))
		return
	}
	r := *q.report
	cacheDetailSummary(c, l, r)
	cacheBreakdownTabs(c, l, s)
	rows := r.Models
	title := tx(l, xCacheByModel)
	if s.showSessions {
		rows, title = r.Sessions, tx(l, xCacheBySession)
	}
	cacheRows(c, l, title, rows)
}

func cacheDetailSummary(c *ui.Context, l l10n.Localizer, r cachehit.Report) {
	t := c.Theme()
	h, _ := cacheHealth(r.Totals)
	statGrid(c,
		func() { statCard(c, tx(l, xCacheHitRate), cachePercent(r.Totals), healthFill(t, h)) },
		func() {
			statCard(c, tx(l, xCacheCachedInput), tokenCompact(int(r.Totals.CachedInputTokens)), sys(t, sysBlue))
		},
		func() { statCard(c, tx(l, xCacheAllInput), tokenCompact(int(r.Totals.InputTokens)), sys(t, sysTeal)) },
		func() { statCard(c, tx(l, xCacheSamples), strconv.Itoa(r.Totals.Samples), sys(t, sysOrange)) },
	)
	ui.Text(c, cacheRangeText(l, r)).FontSize(fsCaption).TextColor(inkTertiary(t)).SingleLine().Ellipsis("…")
	cacheGapsText(c, l, r)
}

func cacheBreakdownTabs(c *ui.Context, l l10n.Localizer, s *panelCacheState) {
	t := c.Theme()
	labels := []string{tx(l, xCacheModels), tx(l, xCacheSessions)}
	ui.Row(c).Gap(6).Children(func() {
		for i, label := range labels {
			selected := (i == 1) == s.showSessions
			b := ui.ButtonBase(c.Key("cache-group-"+strconv.Itoa(i))).Label(label).Padding(6, 12).Radius(radiusControl).Cursor(ui.CursorPointer)
			if selected {
				b.Background(t.Accent)
			} else if b.Hovered() {
				b.Background(hoverNeutral(t))
			}
			ink := t.Text
			if selected {
				ink = t.AccentText
			}
			b.Children(func() { ui.Text(c, label).FontSize(fsCallout).FontWeight(500).TextColor(ink).SingleLine() })
			if b.Clicked() {
				s.showSessions = i == 1
			}
		}
	})
}

func cacheRows(c *ui.Context, l l10n.Localizer, title string, rows []cachehit.Row) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(fsHeadline).FontWeight(600)
		if len(rows) == 0 {
			calloutDim(c, tx(l, xCacheNoData))
			return
		}
		tableContainer(c, func() {
			ui.Row(c).Gap(8).Padding(7, 10).Grow(1).Children(func() {
				ui.Text(c, tx(l, xCacheName)).FontSize(fsCaption).FontWeight(600).TextColor(inkSecondary(t)).Grow(1).Basis(0).SingleLine()
				ui.Text(c, tx(l, xCacheHitRate)).FontSize(fsCaption).FontWeight(600).TextColor(inkSecondary(t)).Width(58).TextAlign(ui.End).SingleLine()
				ui.Text(c, tx(l, xCacheCachedAll)).FontSize(fsCaption).FontWeight(600).TextColor(inkSecondary(t)).Width(100).TextAlign(ui.End).SingleLine()
			})
		}, func() {
			for i, row := range rows {
				tableRowRule(c, i == 0)
				name := cacheRowName(l, row)
				ui.Row(c).Gap(8).Padding(7, 10).Grow(1).Children(func() {
					ui.Text(c, name).FontSize(fsCaption).Grow(1).Basis(0).SingleLine().Ellipsis("…").Tooltip(name)
					rate := cachePercent(row.Totals)
					if row.Totals.MissingSamples > 0 {
						rate += " *"
					}
					rateText := ui.Text(c, rate).FontSize(fsCaption).FontFeatures("tnum").Width(58).TextAlign(ui.End).SingleLine().
						Tooltip(cacheRowRateHelp(l, row.Totals))
					if h, ok := cacheHealth(row.Totals); ok {
						rateText.TextColor(healthInk(t, h))
					}
					ratio := tokenCompact(int(row.Totals.CachedInputTokens)) + " / " + tokenCompact(int(row.Totals.InputTokens))
					ui.Text(c, ratio).FontSize(fsCaption).FontFeatures("tnum").TextColor(inkSecondary(t)).Width(100).TextAlign(ui.End).SingleLine()
				})
			}
		})
	})
}

func cacheRowName(l l10n.Localizer, row cachehit.Row) string {
	name := row.Name
	if name == "" {
		name = row.ID
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "unknown", "unknown model":
		return tx(l, xCacheUnknown)
	}
	return name
}

func cacheRowRateHelp(l l10n.Localizer, totals cachehit.Totals) string {
	if totals.MissingSamples > 0 {
		return tx(l, xCacheMissingSamples, totals.MissingSamples)
	}
	return tx(l, xCacheHitRate)
}
