package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

const heatmapWeekStart = 1

// Layout of the grid, from HeatmapLayoutMetrics.
const (
	heatMaxGrid   float32 = 364
	heatMinCell   float32 = 5
	heatMaxCell   float32 = 11
	heatSpacing   float32 = 2
	heatLabelGap  float32 = 6
	heatLabelH    float32 = 14
	trendPlotH    float32 = 78
	trendYAxisW   float32 = 36
	trendLabelGap float32 = 6
)

// panelToken is the token usage section: title with the official as-of
// line, the info/style/refresh controls, the mode picker and the chart.
func panelToken(c *ui.Context, l l10n.Localizer, style store.ChartStyle, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	heat := snap.Codex.Heatmap
	refreshing := sectionRefreshing(snap, runway.SectionTokenHeatmap)
	sectionBlock(c, first, func() {
		tokenHeader(c, l, style, heat, !localProvider(snap.Provider), refreshing, sectionRefresh(acts, runway.SectionTokenHeatmap), st)
		panelChart(c, l, style, snap.Provider, heat, refreshing, st)
	})
}

// officialAsOfText is "Official as of <date>" from the profile statistics.
func officialAsOfText(l l10n.Localizer, heat runway.HeatmapData) string {
	day := strings.TrimSpace(heat.OfficialStatsAsOf)
	if day == "" && !heat.OfficialGeneratedAt.IsZero() {
		day = heat.OfficialGeneratedAt.UTC().Format("2006-01-02")
	}
	if day == "" {
		return ""
	}
	return l.Textf(l10n.KeyHeatmapOfficialAsOf, day)
}

func tokenHeader(c *ui.Context, l l10n.Localizer, style store.ChartStyle, heat runway.HeatmapData, official, refreshing bool, refresh func(), st *panelState) {
	asOf := ""
	if official {
		asOf = officialAsOfText(l, heat)
	}
	disclosure := l.Text(l10n.KeyHeatmapSourceDisclosure)
	if !official {
		disclosure = l.Text(l10n.KeyGrokLocalUsageHint)
	}
	paragraphs := []string{disclosure}
	if asOf != "" {
		paragraphs = append(paragraphs, asOf)
	}
	title := l.Text(l10n.KeyTokenUsageHeatmap)
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Gap(1).Grow(1).Basis(0).Children(func() {
				ui.Text(c, title).FontSize(fsHeadline).FontWeight(600).SingleLine()
				if asOf != "" {
					ui.Text(c, asOf).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine()
				}
			})
			infoButton(c, st, sectionInfo{id: "token-usage", help: disclosure, icon: icInfo, body: popoverBody(c, l, st, title, paragraphs...)})
			chartStyleMenu(c, l, style, st)
			refreshButton(c, l, refreshing, refresh)
		})
		modePicker(c, l, style, st)
	})
}

// chartStyleMenu is the 34pt pop-up of the chart style: the glyph of the
// current style and a chevron.
func chartStyleMenu(c *ui.Context, l l10n.Localizer, style store.ChartStyle, st *panelState) {
	t := c.Theme()
	help := l.Text(l10n.KeyTokenUsageChartStyle)
	b := ui.ButtonBase(c).Label(help).Tooltip(help).Size(34, 24).Radius(radiusControl).Gap(2).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(hoverNeutral(t))
	}
	b.Children(func() {
		ui.Icon(c, chartStyleIcon(style)).Size(14, 14).TextColor(t.Text)
		ui.Icon(c, icChevDown).Size(9, 9).TextColor(inkSecondary(t))
	})
	b.Menu(func(m *ui.Menu) {
		for _, s := range []store.ChartStyle{store.ChartStyleHeatmap, store.ChartStyleLine, store.ChartStyleBar} {
			if m.Item(chartStyleTitle(l, s)).Checked(s == style).Chosen() && s != style {
				if st.setChartStyle != nil {
					st.setChartStyle(s)
				}
			}
		}
	})
}

func chartStyleTitle(l l10n.Localizer, s store.ChartStyle) string {
	switch s {
	case store.ChartStyleLine:
		return l.Text(l10n.KeyTokenUsageChartLine)
	case store.ChartStyleBar:
		return l.Text(l10n.KeyTokenUsageChartBar)
	}
	return l.Text(l10n.KeyTokenUsageChartHeatmap)
}

func chartStyleIcon(s store.ChartStyle) *ui.SVG {
	switch s {
	case store.ChartStyleLine:
		return icLineChart
	case store.ChartStyleBar:
		return icBarChart
	}
	return icGridDots
}

var chartModes = []usage.HeatmapMode{usage.HeatmapDaily, usage.HeatmapWeekly, usage.HeatmapCumulative}

// modePicker is the Daily / Weekly / Cumulative (Monthly for line and bar)
// segmented control.
func modePicker(c *ui.Context, l l10n.Localizer, style store.ChartStyle, st *panelState) {
	third := l10n.KeyHeatmapCumulative
	if style != store.ChartStyleHeatmap {
		third = l10n.KeyHeatmapMonthly
	}
	labels := []string{l.Text(l10n.KeyHeatmapDaily), l.Text(l10n.KeyHeatmapWeekly), l.Text(third)}
	selected := 0
	for i, m := range chartModes {
		if st.mode() == m {
			selected = i
		}
	}
	ui.Row(c).Children(func() {
		if blueSegmented(c.Key("chart-mode"), &selected, false, labels...) {
			st.chartMode = chartModes[selected]
		}
	})
}

func panelChart(c *ui.Context, l l10n.Localizer, style store.ChartStyle, provider store.Provider, heat runway.HeatmapData, refreshing bool, st *panelState) {
	empty := len(heat.AllDevices) == 0 && len(heat.Local) == 0
	if empty {
		sectionEmptyKey(c, l, refreshing, l10n.KeyHeatmapEmpty)
		return
	}
	at := heat.CalculatedAt
	if at.IsZero() {
		at = time.Now()
	}
	loc := time.UTC
	if provider == store.ProviderGrok || provider == store.ProviderGemini {
		loc = time.Local
	}
	input := usage.HeatmapInput{AllDevices: heat.AllDevices, Local: heat.Local, Mode: st.mode(), Now: at, FirstWeekday: heatmapWeekStart, Loc: loc}
	if style == store.ChartStyleHeatmap {
		grid := usage.BuildHeatmap(input)
		if len(grid.Weeks) == 0 {
			sectionEmptyKey(c, l, refreshing, l10n.KeyHeatmapEmpty)
			return
		}
		panelHeatmap(c, l, grid, provider)
		return
	}
	series := usage.BuildChartSeries(input)
	if len(series.Points) == 0 {
		sectionEmptyKey(c, l, refreshing, l10n.KeyHeatmapEmpty)
		return
	}
	panelTrend(c, l, series, style == store.ChartStyleBar, provider)
}

// heatCellSize is HeatmapLayoutMetrics.make: square cells filling the width
// within the cell caps.
func heatCellSize(width float32, weeks int) float32 {
	weeks = max(1, weeks)
	avail := min(max(0, width), heatMaxGrid)
	raw := (avail - heatSpacing*float32(weeks-1)) / float32(weeks)
	return min(heatMaxCell, max(heatMinCell, raw))
}

func heatLevelColor(t *ui.Theme, cell usage.HeatmapCell) ui.Color {
	if !cell.InRange {
		return mono(t, 0.018, 0.018)
	}
	accent := t.Accent
	switch min(4, max(0, cell.Level)) {
	case 0:
		return mono(t, 0.022, 0.022)
	case 1:
		return accent.Alpha(0.28)
	case 2:
		return accent.Alpha(0.45)
	case 3:
		return accent.Alpha(0.68)
	}
	return accent.Alpha(0.92)
}

func panelHeatmap(c *ui.Context, l l10n.Localizer, grid usage.HeatmapSnapshot, provider store.Provider) {
	t := c.Theme()
	width, _ := c.Size()
	cell := heatCellSize(width-32, len(grid.Weeks))
	step := cell + heatSpacing
	corner := max(1.5, cell*0.22)
	ui.Column(c).Children(func() {
		ui.Row(c).Gap(heatSpacing).Children(func() {
			for _, week := range grid.Weeks {
				ui.Column(c).Gap(heatSpacing).Children(func() {
					for _, day := range week {
						box := ui.Box(c.Key(day.DayKey)).Size(cell, cell).Radius(corner).Background(heatLevelColor(t, day)).Label(day.DayKey)
						chartHoverTip(c, box, heatTooltip(l, provider, day.DayKey, day.Tokens, day.AllDevicesTokens, day.LocalTokens))
					}
				})
			}
		})
		ui.Box(c).Height(heatLabelH).Margin(heatLabelGap, 0, 0, 0).Draw(func(p *ui.Painter, r ui.Rect) {
			for _, month := range grid.MonthLabels {
				p.Text(r.X+float32(month.WeekIndex)*step, r.Y, runway.MonthTitle(month.Month, l.Language()), fsCaption, inkSecondary(t))
			}
		})
	})
}

func heatTooltip(l l10n.Localizer, provider store.Provider, day string, tokens, all, local int64) string {
	if provider == store.ProviderGrok || provider == store.ProviderGemini {
		return fmt.Sprintf("%s\n%s %s %s", day, l.Text(l10n.KeyHeatmapLocalShort), usage.CompactTokenCount(local, l.Language()), l.Text(l10n.KeyTokens))
	}
	return fmt.Sprintf("%s\n%s %s\n%s: %s\n%s: %s", day, usage.CompactTokenCount(tokens, l.Language()), l.Text(l10n.KeyTokens), l.Text(l10n.KeyHeatmapAllDevices), usage.CompactTokenCount(all, l.Language()), l.Text(l10n.KeyHeatmapLocalShort), usage.CompactTokenCount(local, l.Language()))
}

// panelTrend is the line / bar chart: a y axis of compact token counts, the
// plot with its hover areas and the month labels.
func panelTrend(c *ui.Context, l l10n.Localizer, series usage.ChartSeries, bar bool, provider store.Provider) {
	t := c.Theme()
	maxTokens := int64(0)
	for _, point := range series.Points {
		maxTokens = max(maxTokens, point.Tokens)
	}
	months := trendMonthLabels(l, series.Points)
	outer := trendPlotH + trendLabelGap + heatLabelH
	ui.Box(c).Height(outer).Draw(func(p *ui.Painter, r ui.Rect) {
		plot := ui.Rect{X: r.X + trendYAxisW, Y: r.Y, W: max(1, r.W-trendYAxisW), H: trendPlotH}
		drawTrendAxis(p, l, plot, maxTokens, inkSecondary(t))
		drawTrendGrid(p, plot, t)
		drawTrendSeries(p, plot, series.Points, maxTokens, t.Accent, bar)
		for _, m := range months {
			p.Text(plot.X+trendX(plot.W, len(series.Points), m.index)-8, plot.Y+plot.H+trendLabelGap, m.title, fsCaption, inkSecondary(t))
		}
	}).Children(func() {
		ui.Row(c).Fill().Padding(0, 0, trendLabelGap+heatLabelH, trendYAxisW).Children(func() {
			for _, point := range series.Points {
				ui.Box(c).Grow(1).Basis(0).Height(trendPlotH).Label(point.DayKey).Tooltip(heatTooltip(l, provider, point.DayKey, point.Tokens, point.AllDevicesTokens, point.LocalTokens))
			}
		})
	})
}

type trendMonth struct {
	index int
	title string
}

func trendMonthLabels(l l10n.Localizer, points []usage.ChartPoint) []trendMonth {
	var out []trendMonth
	last := 0
	for i, p := range points {
		if m := int(p.Date.Month()); m != last {
			out = append(out, trendMonth{index: i, title: runway.MonthTitle(m, l.Language())})
			last = m
		}
	}
	return out
}

// trendX is the x of point i: points span the plot edge to edge.
func trendX(width float32, count, i int) float32 {
	if count <= 1 {
		return width / 2
	}
	return width * float32(i) / float32(count-1)
}

func trendY(plot ui.Rect, tokens, maxTokens int64) float32 {
	if maxTokens <= 0 {
		return plot.H
	}
	ratio := min(1, max(0, float32(tokens)/float32(maxTokens)))
	return plot.H * (1 - ratio)
}

func drawTrendAxis(p *ui.Painter, l l10n.Localizer, plot ui.Rect, maxTokens int64, ink ui.Color) {
	label := func(s string, y float32) {
		w, h := p.MeasureText(0, ui.Span{Text: s, Size: 9, Font: "monospace"})
		p.RichText(plot.X-4-w, y-h/2, 0, ui.Span{Text: s, Size: 9, Font: "monospace", Color: ink})
	}
	label(usage.CompactTokenCount(maxTokens, l.Language()), plot.Y+4)
	if maxTokens > 0 {
		label(usage.CompactTokenCount(maxTokens/2, l.Language()), plot.Y+plot.H/2)
	}
	label("0", plot.Y+plot.H-4)
}

func drawTrendGrid(p *ui.Painter, plot ui.Rect, t *ui.Theme) {
	grid := mono(t, 0.035, 0.035)
	p.Fill(ui.Rect{X: plot.X, Y: plot.Y + plot.H - 1, W: plot.W, H: 1}, grid, 0)
	p.Fill(ui.Rect{X: plot.X, Y: plot.Y + plot.H/2, W: plot.W, H: 1}, grid, 0)
}

func drawTrendSeries(p *ui.Painter, plot ui.Rect, points []usage.ChartPoint, maxTokens int64, blue ui.Color, bar bool) {
	if len(points) == 0 {
		return
	}
	if bar {
		slot := plot.W / float32(len(points))
		width := min(12, max(1.5, slot*0.62))
		for i, point := range points {
			y := trendY(plot, point.Tokens, maxTokens)
			h := max(plot.H-y, 0)
			if point.Tokens > 0 {
				h = max(h, 1)
			}
			if h > 0 {
				p.Fill(ui.Rect{X: plot.X + float32(i)*slot + (slot-width)/2, Y: plot.Y + plot.H - h, W: width, H: h}, blue.Alpha(0.78), min(2, width/2))
			}
		}
		return
	}
	var line, area ui.Path
	for i, point := range points {
		x, y := plot.X+trendX(plot.W, len(points), i), plot.Y+trendY(plot, point.Tokens, maxTokens)
		if i == 0 {
			line.MoveTo(x, y)
			area.MoveTo(x, plot.Y+plot.H)
			area.LineTo(x, y)
			continue
		}
		line.LineTo(x, y)
		area.LineTo(x, y)
	}
	area.LineTo(plot.X+trendX(plot.W, len(points), len(points)-1), plot.Y+plot.H)
	p.FillPath(&area, blue.Alpha(0.16))
	p.StrokePath(&line, 1.75, blue)
}
