package main

import (
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"
	"time"
)

// The hero shows the app's main panel as live HTML instead of a screenshot.
// Every value is sample data, generated from fixed seeds so a rebuild of the
// same release renders the same page; formatting follows internal/runway.

type Panel struct {
	Providers []PanelProvider
}

type PanelProvider struct {
	Key, Name string
	Remaining int // shortest window, drives the tab bar
	Health    string
	Tier      string
	TierClass string
	Account   string
	Gilded    bool // Codex paints the account name like its tier badge
	Expiry    *PanelExpiry
	Official  string
	QuotaHead string
	Rows      []QuotaRow
	Slim      []SlimRow
	Extra     string
	Chart     PanelChart
	TTFT      bool
	Speed     []SpeedRow
	Basis     string
	Cost      string
}

type PanelExpiry struct {
	Date, Left string
	Dash       string // stroke-dasharray of the 30-day countdown ring
}

type QuotaRow struct {
	Title, Left, Health string
	Pct                 int
	Pace                string
	ResetRel, ResetAbs  string
}

type SlimRow struct {
	Title, Used, Health string
	Pct                 int
}

type SpeedRow struct {
	Model, Effort, TTFT, Rate string
}

type PanelChart struct {
	Cols   int
	Cells  []int // heatmap level 0–4, -1 after today
	Months []MonthLabel
	Bars   template.HTML
	Line   template.HTML
	Weekly [3]string // y-axis: max, half, zero
	Total  [3]string
}

type MonthLabel struct {
	Col  int // 1-based CSS grid column
	Text string
}

const (
	chartCols   = 42
	chartHeight = 78
)

type usageSpec struct {
	seed      uint32
	startCol  int
	perDay    float64
	idle      float64 // chance of an unused weekday
	usdPerTok float64
}

// panelNow anchors the sample to the release date so dates stay plausible.
func panelNow(rel Release) time.Time {
	if t, err := time.Parse("2006-01-02", rel.Date); err == nil {
		return t.Add(10*time.Hour + 47*time.Minute + 55*time.Second)
	}
	y, m, d := time.Now().UTC().Date()
	return time.Date(y, m, d, 10, 47, 55, 0, time.UTC)
}

func buildPanel(l Locale, now time.Time) Panel {
	p := l.T.Panel
	pct := func(format string, n int) string { return strings.ReplaceAll(format, "{n}", strconv.Itoa(n)) }
	exhausts := func(secs int) string {
		return p.BurnRate + ": " + p.ExhaustsIn + " " + duration(l.Code, secs, false)
	}
	row := func(title string, left int, pace string, reset time.Duration) QuotaRow {
		at := now.Add(reset)
		return QuotaRow{
			Title: title, Pct: left, Left: pct(p.Left, left), Health: health(left), Pace: pace,
			ResetRel: p.NextReset + duration(l.Code, int(reset.Seconds()), true),
			ResetAbs: p.NextReset + resetExact(l.Code, at, now),
		}
	}
	slim := func(title string, used int) SlimRow {
		return SlimRow{Title: title, Pct: used, Used: pct(p.Used, used), Health: health(100 - used)}
	}
	basis := strings.ReplaceAll(p.Basis, "{n}", "10")

	expires := now.Add(9*24*time.Hour + 21*time.Hour + 30*time.Minute)
	codex := PanelProvider{
		Key: "codex", Name: "Codex", Tier: "Pro 200", TierClass: "black-gold", Account: "m***@***.com", Gilded: true,
		Expiry: &PanelExpiry{
			Date: expiryDate(l.Code, expires),
			Left: duration(l.Code, int(expires.Sub(now).Seconds()), false),
			Dash: ringDash(expires.Sub(now).Hours() / (30 * 24)),
		},
		Official:  strings.ReplaceAll(p.Official, "{date}", now.AddDate(0, 0, -2).Format("2006-01-02")),
		QuotaHead: p.Quota,
		Rows: []QuotaRow{
			row(p.FiveHour, 82, p.BurnRate+": "+p.Projected+" 64%", 3*time.Hour+12*time.Minute+5*time.Second),
			row(p.Weekly, 57, exhausts(42*3600), 4*24*time.Hour+6*time.Hour+10*time.Minute),
		},
		TTFT: true,
		Speed: []SpeedRow{
			{"gpt-6-astra", "ultra", "2.2s", "37 tok/s"},
			{"gpt-5.6-sol", "medium", "2.5s", "47 tok/s"},
			{"codex-auto-review", "low", "1.5s", "149 tok/s"},
		},
		Basis: basis,
	}
	claude := PanelProvider{
		Key: "claude", Name: "Claude", Tier: "Max 20x", TierClass: "black-gold", Account: "m***@***.com",
		QuotaHead: "Claude",
		Rows:      []QuotaRow{row(p.FiveHour, 34, "", 1*time.Hour+48*time.Minute+20*time.Second)},
		Slim:      []SlimRow{slim(p.Weekly, 41), slim("Sonnet", 18), slim("Opus", 63)},
		Extra:     "12.50 / 100.00 USD",
		Speed: []SpeedRow{
			{"claude-opus-5-5", "", "", "62 tok/s"},
			{"claude-sonnet-5", "", "", "88 tok/s"},
			{"claude-haiku-4-5", "", "", "141 tok/s"},
		},
		Basis: basis,
	}
	gemini := PanelProvider{
		Key: "gemini", Name: "Gemini", Tier: "Pro", TierClass: "gemini", Account: "m***@***.com",
		QuotaHead: "Gemini",
		Rows:      []QuotaRow{row("Gemini · "+p.FiveHour, 71, "", 2*time.Hour+26*time.Minute+40*time.Second)},
		Slim: []SlimRow{
			slim("Gemini · "+p.Weekly, 38), slim("Claude · "+p.FiveHour, 22),
			slim("Claude · "+p.Weekly, 47), slim("Argon · "+p.Weekly, 9),
		},
		TTFT: true,
		Speed: []SpeedRow{
			{"gemini-3.5-pro", "", "2.4s", "71 tok/s"},
			{"gemini-3.5-flash", "", "0.8s", "164 tok/s"},
			{"claude-sonnet-5", "", "1.9s", "83 tok/s"},
		},
		Basis: basis,
	}
	grok := PanelProvider{
		Key: "grok", Name: "Grok", Tier: "SuperGrok", TierClass: "silver", Account: "m***@***.com",
		QuotaHead: p.GrokIncluded,
		Rows:      []QuotaRow{row(p.GrokWeekly, 12, "", 2*24*time.Hour+9*time.Hour+40*time.Minute)},
		Slim:      []SlimRow{slim("Grok Build", 61), slim("Grok Imagine", 19), slim("Grok Chat", 8)},
		TTFT:      true,
		Speed: []SpeedRow{
			{"grok-code-fast-1", "", "0.9s", "176 tok/s"},
			{"grok-4", "", "1.8s", "64 tok/s"},
		},
		Basis: basis,
	}
	specs := []usageSpec{
		{seed: 0x9e3779b9, startCol: 10, perDay: 9e6, idle: 0.12, usdPerTok: 0.36e-6},
		{seed: 0x85ebca6b, startCol: 17, perDay: 6e6, idle: 0.2, usdPerTok: 0.58e-6},
		{seed: 0x27d4eb2f, startCol: 21, perDay: 3.2e6, idle: 0.3, usdPerTok: 0.31e-6},
		{seed: 0xc2b2ae35, startCol: 27, perDay: 1.6e6, idle: 0.42, usdPerTok: 0.27e-6},
	}
	providers := []PanelProvider{codex, claude, gemini, grok}
	for i := range providers {
		pr := &providers[i]
		pr.Remaining = pr.Rows[0].Pct
		pr.Health = health(pr.Remaining)
		var total float64
		pr.Chart, total = buildChart(l.Code, now, specs[i])
		pr.Cost = fmt.Sprintf("$%.2f %s · %s Tokens", total*specs[i].usdPerTok, p.APIEquivalent, compact(total))
	}
	return Panel{Providers: providers}
}

// ringDash draws share of the expiry ring, whose circle has r = 4.5.
func ringDash(share float64) string {
	c := 2 * math.Pi * 4.5
	return fmt.Sprintf("%.2f %.2f", min(1, share)*c, c)
}

// health is the app's quota color by remaining share.
func health(left int) string {
	switch {
	case left >= 50:
		return "ok"
	case left >= 20:
		return "warn"
	}
	return "low"
}

func buildChart(code string, now time.Time, s usageSpec) (PanelChart, float64) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	first := today.AddDate(0, 0, -int(today.Weekday())-7*(chartCols-1))
	rng := s.seed
	next := func() float64 {
		rng ^= rng << 13
		rng ^= rng >> 17
		rng ^= rng << 5
		return float64(rng) / float64(math.MaxUint32)
	}
	days := make([]float64, chartCols*7)
	var peak float64
	for i := range days {
		col := i / 7
		day := first.AddDate(0, 0, i)
		if day.After(today) {
			days[i] = -1
			continue
		}
		idle := s.idle
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			idle += 0.3
		}
		if col < s.startCol || next() < idle {
			continue
		}
		growth := 0.55 + 0.9*float64(col-s.startCol)/float64(chartCols-s.startCol)
		days[i] = s.perDay * growth * (0.3 + 1.4*next())
		peak = max(peak, days[i])
	}
	c := PanelChart{Cols: chartCols, Cells: make([]int, len(days))}
	weekly := make([]float64, chartCols)
	cumulative := make([]float64, chartCols)
	var total float64
	for i, v := range days {
		switch {
		case v < 0:
			c.Cells[i] = -1
		case v > 0:
			c.Cells[i] = min(4, int(math.Ceil(v/peak*4)))
			weekly[i/7] += v
			total += v
		}
		cumulative[i/7] = total
	}
	c.Months = monthLabels(code, first)
	c.Bars, c.Weekly = barsSVG(weekly)
	c.Line, c.Total = lineSVG(cumulative)
	return c, total
}

// monthLabels marks the week column holding each month's first day, plus the
// partial first month when it has room before the next label.
func monthLabels(code string, first time.Time) []MonthLabel {
	var labels []MonthLabel
	for col := range chartCols {
		for d := range 7 {
			day := first.AddDate(0, 0, col*7+d)
			if day.Day() == 1 {
				labels = append(labels, MonthLabel{Col: col + 1, Text: monthTitle(code, day.Month())})
			}
		}
	}
	if len(labels) == 0 || labels[0].Col > 4 {
		labels = append([]MonthLabel{{Col: 1, Text: monthTitle(code, first.AddDate(0, 0, 6).Month())}}, labels...)
	}
	return labels
}

func barsSVG(weekly []float64) (template.HTML, [3]string) {
	top := axisTop(weekly)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="plot" viewBox="0 0 %d %d" preserveAspectRatio="none" aria-hidden="true">`, len(weekly)*10, chartHeight)
	for i, v := range weekly {
		if h := v / top * chartHeight; h > 0 {
			fmt.Fprintf(&b, `<rect x="%d" y="%.1f" width="7" height="%.1f"/>`, i*10+1, chartHeight-h, h)
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()), axisLabels(top)
}

func lineSVG(cumulative []float64) (template.HTML, [3]string) {
	top := axisTop(cumulative)
	var pts []string
	for i, v := range cumulative {
		pts = append(pts, fmt.Sprintf("%d,%.1f", i*10+5, chartHeight-v/top*chartHeight))
	}
	line := strings.Join(pts, " L")
	area := fmt.Sprintf("M5,%d L%s L%d,%d Z", chartHeight, line, (len(cumulative)-1)*10+5, chartHeight)
	svg := fmt.Sprintf(`<svg class="plot" viewBox="0 0 %d %d" preserveAspectRatio="none" aria-hidden="true"><path class="area" d="%s"/><path class="stroke" d="M%s" vector-effect="non-scaling-stroke"/></svg>`,
		len(cumulative)*10, chartHeight, area, line)
	return template.HTML(svg), axisLabels(top)
}

// axisTop rounds the peak up to two significant digits.
func axisTop(values []float64) float64 {
	var peak float64
	for _, v := range values {
		peak = max(peak, v)
	}
	if peak <= 0 {
		return 1
	}
	unit := math.Pow(10, math.Floor(math.Log10(peak))-1)
	return math.Ceil(peak/unit) * unit
}

func axisLabels(top float64) [3]string {
	return [3]string{compact(top), compact(top / 2), "0"}
}

func compact(v float64) string {
	unit, suffix := 1.0, ""
	switch {
	case v >= 1e9:
		unit, suffix = 1e9, "B"
	case v >= 1e6:
		unit, suffix = 1e6, "M"
	case v >= 1e3:
		unit, suffix = 1e3, "K"
	}
	s := strconv.FormatFloat(v/unit, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + suffix
}

// The helpers below mirror internal/runway/format.go for the seven site
// languages, keyed by locale code.

var monthShort = map[string][12]string{
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"ru": {"янв.", "февр.", "март", "апр.", "май", "июнь", "июль", "авг.", "сент.", "окт.", "нояб.", "дек."},
	"fr": {"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
}

func monthTitle(code string, m time.Month) string {
	switch code {
	case "zh-hans", "zh-hant", "ja":
		return fmt.Sprintf("%d月", m)
	case "ko":
		return fmt.Sprintf("%d월", m)
	case "ru", "fr":
		return monthShort[code][m-1]
	}
	return monthShort["en"][m-1]
}

func expiryDate(code string, t time.Time) string {
	switch code {
	case "zh-hans", "zh-hant", "ja", "ko":
		return fmt.Sprintf("%d/%d/%d", t.Year(), t.Month(), t.Day())
	case "ru", "fr":
		return fmt.Sprintf("%d %s %d", t.Day(), monthTitle(code, t.Month()), t.Year())
	}
	return fmt.Sprintf("%s %d, %d", monthTitle(code, t.Month()), t.Day(), t.Year())
}

func resetExact(code string, at, now time.Time) string {
	clock := at.Format("15:04")
	if at.YearDay() == now.YearDay() && at.Year() == now.Year() {
		return clock
	}
	switch code {
	case "zh-hans", "zh-hant", "ja":
		return fmt.Sprintf("%d月%d日 %s", at.Month(), at.Day(), clock)
	case "ko":
		return fmt.Sprintf("%d월 %d일 %s", at.Month(), at.Day(), clock)
	case "ru", "fr":
		return fmt.Sprintf("%d %s %s", at.Day(), monthTitle(code, at.Month()), clock)
	}
	return fmt.Sprintf("%s %d %s", monthTitle(code, at.Month()), at.Day(), clock)
}

// duration shows days and hours, or hours, minutes and optionally seconds.
func duration(code string, secs int, withSeconds bool) string {
	days, hours, minutes, s := secs/86400, secs%86400/3600, secs%3600/60, secs%60
	var parts []string
	if days > 0 {
		parts = append(parts, unit(code, days, 0))
		if hours > 0 {
			parts = append(parts, unit(code, hours, 1))
		}
	} else {
		if hours > 0 {
			parts = append(parts, unit(code, hours, 1))
		}
		if minutes > 0 || hours > 0 {
			parts = append(parts, unit(code, minutes, 2))
		}
		if withSeconds || len(parts) == 0 {
			parts = append(parts, unit(code, s, 3))
		}
	}
	switch code {
	case "zh-hans", "zh-hant", "ja", "ko":
		return strings.Join(parts, "")
	}
	return strings.Join(parts, " ")
}

var unitWords = map[string][4]string{
	"zh-hans": {"天", "小时", "分钟", "秒"},
	"zh-hant": {"天", "小時", "分鐘", "秒"},
	"ja":      {"日", "時間", "分", "秒"},
	"ko":      {"일", "시간", "분", "초"},
}

// unit is a number with its unit word; u is day, hour, minute, second.
func unit(code string, v, u int) string {
	n := strconv.Itoa(v)
	if w, ok := unitWords[code]; ok {
		return n + w[u]
	}
	switch code {
	case "ru":
		forms := [...][3]string{{"день", "дня", "дней"}, {"час", "часа", "часов"}, {"минута", "минуты", "минут"}, {"секунда", "секунды", "секунд"}}[u]
		w := forms[2]
		switch m10, m100 := v%10, v%100; {
		case m10 == 1 && m100 != 11:
			w = forms[0]
		case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
			w = forms[1]
		}
		return n + " " + w
	case "fr":
		w := [...][2]string{{"jour", "jours"}, {"heure", "heures"}, {"minute", "minutes"}, {"seconde", "secondes"}}[u]
		if v <= 1 {
			return n + " " + w[0]
		}
		return n + " " + w[1]
	}
	w := [...][2]string{{"day", "days"}, {"hour", "hours"}, {"minute", "minutes"}, {"second", "seconds"}}[u]
	if v == 1 {
		return n + " " + w[0]
	}
	return n + " " + w[1]
}
