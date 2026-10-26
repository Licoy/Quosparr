package app

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// costCustomIndex is the "custom range" tab after the four stored ranges.
const costCustomIndex = 4

// costDetailPage is the API cost page: the range tabs stay on top, the
// result scrolls under them.
func (a *App) costDetailPage(c *ui.Context, l l10n.Localizer, prefs store.Preferences, snap runway.Snapshot, v runway.CodexView, acts PanelActions) {
	s := &a.panelState.cost
	a.syncCostQuery(prefs, v, acts)
	ui.Column(c).Gap(14).Grow(1).Children(func() {
		a.costRangeControls(c, l, acts)
		if s.task.Running {
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() { costLoading(c, l, snap.Refresh.CostProgress) })
			return
		}
		ui.Scroll(c.Key("cost-detail-scroll")).Grow(1).Children(func() {
			ui.Column(c).Gap(14).Children(func() { costContent(c, l, s, v, prefs.APICostSummaryRange) })
		})
	})
}

// syncCostQuery starts the first query of the page and follows a new
// current-cycle summary that arrived while the page was open.
func (a *App) syncCostQuery(prefs store.Preferences, v runway.CodexView, acts PanelActions) {
	s := &a.panelState.cost
	ranges := store.CostRanges()
	if acts.PricingVersion != nil && s.query != nil && s.pricingVersion != acts.PricingVersion(prefs.SelectedProvider) {
		a.startCostQuery(*s.query, acts)
		return
	}
	if prefs.SelectedProvider == store.ProviderCodex && s.query != nil && s.query.Range == store.CostRangeCurrent && v.Cost != nil && v.Cost != s.sourceCost {
		if s.sourceCost != nil && !s.sourceCost.Window.Start.Equal(v.Cost.Window.Start) {
			// A different cycle must resolve its own window through the query.
			a.startCostQuery(*s.query, acts)
		} else {
			s.acceptCurrentCost(v.Cost)
		}
	}
	if s.initialized {
		return
	}
	s.initialized = true
	for i, r := range ranges {
		if r == prefs.APICostSummaryRange {
			s.selected = i
		}
	}
	s.end = time.Now()
	s.start = s.end.AddDate(0, 0, -6)
	if acts.QueryCost != nil {
		a.startCostQuery(CostQuery{Range: ranges[s.selected]}, acts)
	} else {
		s.summary = v.DisplayedCost
	}
}

func (a *App) costRangeControls(c *ui.Context, l l10n.Localizer, acts PanelActions) {
	s := &a.panelState.cost
	ui.Column(c).Gap(8).Children(func() {
		a.costRangeTabs(c, l, acts)
		if s.selected == costCustomIndex && !s.task.Running {
			a.customCostControls(c, l, acts)
		}
	})
}

// costRangeTabs is the one-row segmented picker: a sunken track with the
// chosen tab filled with the accent color.
func (a *App) costRangeTabs(c *ui.Context, l l10n.Localizer, acts PanelActions) {
	s := &a.panelState.cost
	t := c.Theme()
	cardColumn(c, surfaceSunkenKind, radiusRow+1).Padding(2).Children(func() {
		ui.Row(c).Gap(1).Children(func() {
			for i, label := range costRangeLabels(l) {
				tab := ui.ButtonBase(c.Key("cost-tab-"+strconv.Itoa(i))).Label(label).Tooltip(label).Grow(1).Basis(0).Padding(5, 4).Radius(radiusControl).
					AlignItems(ui.Center).Justify(ui.Center).Disabled(acts.QueryCost == nil).Cursor(ui.CursorPointer)
				weight, ink := 400, inkSecondary(t)
				switch {
				case i == s.selected:
					fill := t.Accent
					if tab.Pressed() {
						fill = t.AccentPressed
					} else if tab.Hovered() {
						fill = t.AccentHover
					}
					tab.Background(fill)
					weight, ink = 600, t.AccentText
				case tab.Hovered():
					tab.Background(t.Text.Alpha(0.05))
				}
				tab.Children(func() {
					ui.Text(c, label).FontSize(fsCallout).FontWeight(weight).TextColor(ink).SingleLine().Ellipsis("…")
				})
				if tab.Clicked() {
					a.selectCostRange(i, acts)
				}
			}
		})
	})
}

func costRangeLabels(l l10n.Localizer) []string {
	var out []string
	for _, r := range store.CostRanges() {
		out = append(out, l.Text(costRangeKey(r)))
	}
	return append(out, l.Text(l10n.KeyCustomRange))
}

func (a *App) selectCostRange(i int, acts PanelActions) {
	s := &a.panelState.cost
	s.selected = i
	if i < costCustomIndex {
		a.startCostQuery(CostQuery{Range: store.CostRanges()[i]}, acts)
		return
	}
	// A custom range stays idle until the user picks dates and calculates.
	if s.cancel != nil {
		s.cancel()
	}
	s.generation++
	s.summary, s.query, s.task = nil, nil, Task{}
}

func (a *App) customCostControls(c *ui.Context, l l10n.Localizer, acts PanelActions) {
	s := &a.panelState.cost
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
		costDateField(c, l.Text(l10n.KeyStartDate), "cost-start", &s.start)
		costDateField(c, l.Text(l10n.KeyEndDate), "cost-end", &s.end)
		disabled := acts.QueryCost == nil
		calc := ui.ButtonBase(c).Label(l.Text(l10n.KeyCalculate)).Padding(7, 14).MinWidth(68).Radius(radiusRow).AlignItems(ui.Center).Justify(ui.Center).
			Disabled(disabled).Cursor(ui.CursorPointer)
		fill, ink := t.Accent, t.AccentText
		switch {
		case disabled:
			fill, ink = fill.Alpha(0.45), ink.Alpha(0.7)
		case calc.Pressed():
			fill = t.AccentPressed
		case calc.Hovered():
			fill = t.AccentHover
		}
		calc.Background(fill)
		calc.Children(func() {
			ui.Text(c, l.Text(l10n.KeyCalculate)).FontSize(fsCallout).FontWeight(600).TextColor(ink).SingleLine()
		})
		if calc.Clicked() {
			a.calculateCustomCost(l, acts)
		}
	})
}

func costDateField(c *ui.Context, title, key string, date *time.Time) {
	ui.Column(c).Gap(3).Grow(1).Basis(0).Children(func() {
		dimText(c, title)
		ui.DateInput(c.Key(key), date).MinWidth(0)
	})
}

func (a *App) calculateCustomCost(l l10n.Localizer, acts PanelActions) {
	s := &a.panelState.cost
	r, ok := usage.CustomRange(s.start, s.end, time.Local)
	if !ok || s.end.After(time.Now()) {
		if s.cancel != nil {
			s.cancel()
		}
		s.generation++
		s.summary = nil
		s.task = Task{Err: errors.New(l.Text(l10n.KeyInvalidDateRange))}
		return
	}
	a.startCostQuery(CostQuery{Custom: &r}, acts)
}

// costLoading is the centered scan progress.
func costLoading(c *ui.Context, l l10n.Localizer, p usage.Progress) {
	t := c.Theme()
	ui.Column(c).Gap(12).AlignItems(ui.Center).Children(func() {
		if f, ok := p.Fraction(); ok {
			ui.Progress(c, float64(f)).Width(220)
		} else {
			ui.Spinner(c).Size(20, 20)
		}
		ui.Text(c, costProgressTitle(l, p)).FontSize(fsCallout).FontWeight(500).TextColor(inkSecondary(t)).TextAlign(ui.Center)
		if sub := costProgressSubtitle(l, p); sub != "" {
			ui.Text(c, sub).FontSize(fsCaption).TextColor(inkTertiary(t)).MaxLines(2).TextAlign(ui.Center)
		}
	})
}

func costProgressTitle(l l10n.Localizer, p usage.Progress) string {
	switch p.Phase {
	case usage.PhasePreparing:
		return l.Text(l10n.KeyCostScanPreparing)
	case usage.PhaseRefreshingIndex:
		return l.Text(l10n.KeyCostScanIndexing)
	case usage.PhaseAggregating:
		return l.Text(l10n.KeyCostScanAggregating)
	case usage.PhaseFetchingOnline:
		return l.Text(l10n.KeyCostScanFetchingOnline)
	case usage.PhaseFailed:
		if p.Message != "" {
			return p.Message
		}
		return l.Text(l10n.KeyCostScanFailed)
	}
	return l.Text(l10n.KeyCalculating)
}

func costProgressSubtitle(l l10n.Localizer, p usage.Progress) string {
	var parts []string
	if p.TotalUnits != nil && *p.TotalUnits > 0 {
		parts = append(parts, l10n.Format(l.Text(l10n.KeyCostScanProgressFiles), p.CompletedUnits, *p.TotalUnits))
	}
	if p.Detail != "" {
		parts = append(parts, p.Detail)
	}
	return joinParts(parts, " · ")
}

func joinParts(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

// costContent is the result under the range tabs.
func costContent(c *ui.Context, l l10n.Localizer, s *panelCostState, v runway.CodexView, shown store.CostRange) {
	t := c.Theme()
	note := costScanNote(l, s, v)
	switch {
	case s.summary != nil && s.summary.IsDisplayableCost():
		costResult(c, l, s, *s.summary)
		note(c)
	case s.selected == costCustomIndex && s.summary == nil && s.task.Err == nil:
		// Idle custom tab: ask for dates instead of showing "unavailable".
		calloutDim(c, l.Text(l10n.KeyCustomRangePrompt)).FontSize(fsBody)
		note(c)
	case s.summary != nil:
		ui.Text(c, l.Text(l10n.KeyUsageAnalyticsEmpty)).FontSize(fsBody).TextColor(inkSecondary(t))
		note(c)
	default:
		ui.Text(c, l.Text(l10n.KeyUsageAnalyticsUnavailable)).FontSize(fsBody).TextColor(inkSecondary(t))
		note(c)
	}
}

// costScanNote is the orange scan failure line: the page's own error first,
// else the main panel's note on the current cycle tab.
func costScanNote(l l10n.Localizer, s *panelCostState, v runway.CodexView) func(*ui.Context) {
	text := ""
	switch {
	case s.task.Err != nil:
		text = detailErrorText(l, s.task.Err)
	case s.selected == 1:
		text = v.CostScanNote
	}
	return func(c *ui.Context) {
		if text != "" {
			ui.Text(c, l.Text(l10n.KeyCostScanFailed)+": "+text).FontSize(fsCaption).TextColor(sys(c.Theme(), sysOrange))
		}
	}
}

func costResult(c *ui.Context, l l10n.Localizer, s *panelCostState, sum usage.Summary) {
	title := l.Text(l10n.KeyApiCost)
	figure := costMoney(sum.EstimatedUSD)
	if claudePartialCost(sum) {
		title = l.Pricing("partial")
	}
	if sum.EstimatedUSD == nil {
		figure = l.Text(l10n.KeyTokensOnly)
	}
	pageSummaryRow(c, title, costMeta(l, sum), figure)
	ui.Text(c, l.Pricing("version")+": "+pricingShortVersion(sum.PricingVersion)).Tooltip(sum.PricingVersion).FontSize(fsCaption).TextColor(inkSecondary(c.Theme()))
	ui.Text(c, l.Pricing("currentEstimate")).FontSize(fsCaption).TextColor(inkSecondary(c.Theme()))
	costPricingNotes(c, l, sum)
	costStats(c, l, sum)
	costTokenParts(c, l, sum.Totals)
	costDailyTable(c, l, sum.DailyRows, costRangeTitle(l, s))
	costBreakdown(c, l, l.Text(l10n.KeyProjectBreakdown), sum.ProjectRows)
	costBreakdown(c, l, l.Text(l10n.KeyModelBreakdown), sum.ModelRows)
	costBreakdown(c, l, l.Text(l10n.KeyApiCostSource), sum.ClientRows)
	if sum.Source == usage.SourceOnlineAnalytics {
		dimText(c, l.Text(l10n.KeyRawAnalyticsCredits)+": "+strconv.FormatFloat(sum.RawCredits, 'f', 3, 64))
	}
}

func costRangeTitle(l l10n.Localizer, s *panelCostState) string {
	if s.selected >= costCustomIndex {
		return l.Text(l10n.KeyCustomRange)
	}
	return l.Text(costRangeKey(store.CostRanges()[s.selected]))
}

func costFigure(l l10n.Localizer, sum usage.Summary) string {
	if sum.EstimatedUSD == nil {
		return l.Text(l10n.KeyTokensOnly)
	}
	return costAmount(l, sum.EstimatedUSD, claudePartialCost(sum))
}
func costAmount(l l10n.Localizer, d *decimal.Decimal, partial bool) string {
	if d == nil {
		return "—"
	}
	out := runway.Money(*d)
	if partial {
		out += " · " + l.Pricing("partial")
	}
	return out
}

func costMeta(l l10n.Localizer, sum usage.Summary) string {
	window := mediumDateTime(l, sum.Window.Start) + " – " + mediumDateTime(l, sum.Window.End)
	return window + " · " + l.Text(l10n.KeyCalculatedAt) + " " + mediumDateTime(l, sum.CalculatedAt) + " · " + costSourceName(l, sum.Source)
}

// mediumDateTime is a medium date and medium time in the user's language.
func mediumDateTime(l l10n.Localizer, t time.Time) string {
	return runway.ResetCreditUpdatedAt(t, l.Language(), nil) + t.Local().Format(":05")
}

func costMoney(d *decimal.Decimal) string {
	if d == nil {
		return "—"
	}
	return runway.Money(*d)
}

func costStats(c *ui.Context, l l10n.Localizer, sum usage.Summary) {
	t := c.Theme()
	tot := sum.Totals
	parts := tokenCompact(int(tot.UncachedInputTokens)) + " / " + tokenCompact(int(tot.CachedInputTokens)) + " / " + tokenCompact(int(tot.OutputTokens))
	statGrid(c,
		func() {
			title := l.Text(l10n.KeyEstimatedAPICost)
			if claudePartialCost(sum) {
				title = l.Pricing("partial")
			}
			statCard(c, title, costMoney(sum.EstimatedUSD), sys(t, sysGreen))
		},
		func() { statCard(c, l.Text(l10n.KeyTokens), tokenCompact(int(tot.TotalTokens)), sys(t, sysBlue)) },
		func() { statCard(c, l.Text(l10n.KeyInputCachedOutput), parts, sys(t, sysTeal)) },
		func() { statCard(c, l.Text(l10n.KeyTurns), strconv.FormatInt(tot.Turns, 10), sys(t, sysOrange)) },
	)
}

func costTokenParts(c *ui.Context, l l10n.Localizer, tot usage.Totals) {
	t := c.Theme()
	ui.Row(c).Gap(8).Children(func() {
		statCard(c, l.Text(l10n.KeyNonCachedInput), tokenCompact(int(tot.UncachedInputTokens)), sys(t, sysBlue))
		statCard(c, l.Text(l10n.KeyCachedInput), tokenCompact(int(tot.CachedInputTokens)), sys(t, sysGreen))
		statCard(c, l.Text(l10n.KeyOutputTokens), tokenCompact(int(tot.OutputTokens)), sys(t, sysOrange))
	})
}

const (
	costColDate  float32 = 78
	costColTurns float32 = 42
	costColMoney float32 = 82
)

func costDailyTable(c *ui.Context, l l10n.Localizer, rows []usage.DailyRow, title string) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(fsHeadline).FontWeight(600)
		if len(rows) == 0 {
			calloutDim(c, l.Text(l10n.KeyNotLoaded))
			return
		}
		tableContainer(c, func() {
			ui.Row(c).Gap(8).Padding(7, 10).Grow(1).Children(func() {
				head := func(s string) ui.Element {
					return ui.Text(c, s).FontSize(fsCaption).FontWeight(600).TextColor(inkSecondary(t)).SingleLine()
				}
				head(l.Text(l10n.KeyDate)).Width(costColDate)
				head(l.Text(l10n.KeyTokens)).Grow(1).Basis(0).TextAlign(ui.End)
				head(l.Text(l10n.KeyEstimatedAPICost)).Grow(1).Basis(0).TextAlign(ui.End)
				head(l.Text(l10n.KeyTurns)).Width(costColTurns).TextAlign(ui.End)
			})
		}, func() {
			for i := len(rows) - 1; i >= 0; i-- {
				costDailyRow(c, l, rows[i], i == len(rows)-1)
			}
		})
	})
}

func costDailyRow(c *ui.Context, l l10n.Localizer, r usage.DailyRow, first bool) {
	tableRowRule(c, first)
	ui.Row(c).Gap(8).Padding(6, 10).Children(func() {
		cell := func(s string) ui.Element { return ui.Text(c, s).FontSize(fsCaption).FontFeatures("tnum").SingleLine() }
		cell(r.Date).Width(costColDate)
		cell(tokenCompact(int(r.Totals.TotalTokens))).Grow(1).Basis(0).TextAlign(ui.End)
		ui.Column(c).Grow(1).Basis(0).AlignItems(ui.End).Children(func() { costAmountCell(c, l, r.EstimatedUSD, r.UnpricedTokens > 0) })
		cell(strconv.FormatInt(r.Totals.Turns, 10)).Width(costColTurns).TextAlign(ui.End)
	})
}

// costBreakdown is a sunken card of at most eight name / tokens / cost rows.
func costBreakdown(c *ui.Context, l l10n.Localizer, title string, rows []usage.BreakdownRow) {
	if len(rows) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(fsHeadline).FontWeight(600)
		cardColumn(c, surfaceSunkenKind, radiusCard).Children(func() {
			for i, r := range rows[:min(8, len(rows))] {
				tableRowRule(c, i == 0)
				ui.Row(c).Gap(8).Padding(6, 10).Children(func() {
					ui.Text(c, r.Name).FontSize(fsCaption).FontFeatures("tnum").SingleLine().Ellipsis("…").Grow(1).Basis(0)
					ui.Text(c, tokenCompact(int(r.Totals.TotalTokens))).FontSize(fsCaption).FontFeatures("tnum").TextColor(inkSecondary(t)).SingleLine()
					ui.Column(c).Width(110).AlignItems(ui.End).Children(func() { costAmountCell(c, l, r.EstimatedUSD, r.UnpricedTokens > 0) })
				})
			}
		})
	})
}

// costSourceName localizes where a cost came from.
func costSourceName(l l10n.Localizer, s usage.Source) string {
	switch s {
	case usage.SourceLocalSessions:
		return l.Text(l10n.KeySourceLocalSessions)
	case usage.SourceOnlineAnalytics:
		return l.Text(l10n.KeySourceOnlineSupplement)
	}
	return l.Text(l10n.KeyUsageAnalyticsEmpty)
}

// costPricingNotes keeps the full pricing issue list independent of the compact
// eight-row breakdowns; source metadata never makes a price incomplete.
func costPricingNotes(c *ui.Context, l l10n.Localizer, sum usage.Summary) {
	issues, sources, scan := costIssueLists(l, sum)
	if sum.UnpricedTokens > 0 {
		ui.Text(c, l.PricingRule("unpricedCount", tokenCompact(int(sum.UnpricedTokens)))).FontSize(fsCaption).TextColor(sys(c.Theme(), sysOrange))
	}
	for _, group := range []struct {
		title string
		lines []string
	}{{l.Pricing("pricingIssues"), issues}, {l.Pricing("source"), sources}, {l.Pricing("scanIssues"), scan}} {
		if len(group.lines) == 0 {
			continue
		}
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, group.title).FontSize(fsCallout).FontWeight(600)
			for _, line := range group.lines {
				ui.Text(c, line).FontSize(fsCaption).Selectable()
			}
		})
	}
}
func costIssueLists(l l10n.Localizer, sum usage.Summary) (issues, sources, scan []string) {
	seen := map[string]bool{}
	add := func(dst *[]string, text string) {
		if !seen[text] {
			*dst = append(*dst, text)
			seen[text] = true
		}
	}
	for _, warning := range sum.Warnings {
		kind, value, ok := strings.Cut(warning, ":")
		if !ok {
			scan = append(scan, warning)
			continue
		}
		switch kind {
		case "price-source":
			source, model, _ := strings.Cut(value, ":")
			if source == "builtin" {
				source = l.Pricing("builtin")
			}
			add(&sources, l.PricingRule("pricingSourceModel", model, source))
		case "unknown-model":
			add(&issues, l.PricingRule("unknownModel", value))
		case "fast-unpriced":
			add(&issues, l.PricingRule("fastUnpriced", value))
		case "ambiguous-tier":
			add(&issues, l.PricingRule("ambiguousTier", value))
		case "missing-price-rule", "missing-input-price", "missing-output-price", "missing-cache-price":
			add(&issues, l.PricingRule("missingPrice", value))
		case "malformed-jsonl-lines":
			add(&scan, l.PricingRule("scanMalformed", value))
		default:
			if strings.HasPrefix(kind, "missing-") {
				add(&issues, l.PricingRule("missingPrice", value))
			} else {
				add(&scan, warning)
			}
		}
	}
	for _, row := range sum.ModelRows {
		if row.UnpricedTokens > 0 {
			add(&issues, l.PricingRule("modelUnpriced", row.Name, tokenCompact(int(row.UnpricedTokens))))
		}
	}
	return
}

func costAmountCell(c *ui.Context, l l10n.Localizer, d *decimal.Decimal, partial bool) {
	ui.Text(c, costMoney(d)).FontSize(fsCaption).FontFeatures("tnum").SingleLine()
	if partial && d != nil {
		ui.Text(c, l.Pricing("partial")).FontSize(fsCaption).TextColor(sys(c.Theme(), sysOrange)).TextAlign(ui.End)
	}
}
