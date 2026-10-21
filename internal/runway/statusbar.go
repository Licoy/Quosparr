package runway

import (
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// RenderPlan says which meters the status item shows and how they stack
// (Swift StatusBarRenderPlan).
type RenderPlan struct {
	Meters        []Meter
	RowsPerColumn int
}

// Columns chunks the meters into columns of RowsPerColumn.
func (p RenderPlan) Columns() [][]Meter {
	if p.RowsPerColumn <= 0 {
		return nil
	}
	var cols [][]Meter
	for start := 0; start < len(p.Meters); start += p.RowsPerColumn {
		cols = append(cols, p.Meters[start:min(start+p.RowsPerColumn, len(p.Meters))])
	}
	return cols
}

// MakeRenderPlan filters the meters for the style and picks the row count.
func MakeRenderPlan(style store.StatusBarStyle, scope store.BatteryScope, meters []Meter) RenderPlan {
	visible := meters
	if style == store.StatusBarStyleBattery {
		visible = batteryMeters(scope, meters)
	}
	rows := 1
	if (style == store.StatusBarStyleBattery || style == store.StatusBarStyleMeters) && len(visible) > 1 {
		rows = 2
	}
	return RenderPlan{Meters: visible, RowsPerColumn: rows}
}

func batteryMeters(scope store.BatteryScope, meters []Meter) []Meter {
	if scope == store.BatteryScopeBoth {
		return meters
	}
	var standard, specific []Meter
	for _, m := range meters {
		if m.Source == SourceStandard {
			standard = append(standard, m)
		} else {
			specific = append(specific, m)
		}
	}
	want := WeeklyMinutes
	if scope == store.BatteryScopeFiveHour {
		want = FiveHourMinutes
	}
	var picked *Meter
	for i := range standard {
		if standard[i].WindowMinutes == want {
			picked = &standard[i]
			break
		}
	}
	if picked == nil && len(standard) > 0 {
		picked = &standard[0]
	}
	if picked == nil {
		return specific
	}
	return append([]Meter{*picked}, specific...)
}

// Preferred window lengths when one meter per provider is chosen.
var (
	codexPreferredWindows  = []int{FiveHourMinutes, WeeklyMinutes}
	grokPreferredWindows   = []int{WeeklyMinutes, MonthlyMinutes}
	claudePreferredWindows = []int{FiveHourMinutes, WeeklyMinutes}
)

// PrimaryStandardMeter picks the first standard meter matching a preferred
// window length, else the first standard meter, else the first meter.
func PrimaryStandardMeter(meters []Meter, preferred []int) (Meter, bool) {
	var standard []Meter
	for _, m := range meters {
		if m.Source == SourceStandard {
			standard = append(standard, m)
		}
	}
	if len(standard) == 0 {
		if len(meters) == 0 {
			return Meter{}, false
		}
		return meters[0], true
	}
	for _, minutes := range preferred {
		for _, m := range standard {
			if m.WindowMinutes == minutes {
				return m, true
			}
		}
	}
	return standard[0], true
}

// ShortestWindowMeter picks the standard meter with the shortest known
// window, else the first standard meter, else the first meter.
func ShortestWindowMeter(meters []Meter) (Meter, bool) {
	var picked *Meter
	for i := range meters {
		m := &meters[i]
		if m.Source != SourceStandard {
			continue
		}
		if picked == nil || m.WindowMinutes > 0 && (picked.WindowMinutes <= 0 || m.WindowMinutes < picked.WindowMinutes) {
			picked = m
		}
	}
	if picked != nil {
		return *picked, true
	}
	if len(meters) == 0 {
		return Meter{}, false
	}
	return meters[0], true
}

// WithShortTitle retitles a meter with the short window name.
func WithShortTitle(lc l10n.Localizer, m Meter) Meter {
	m.Title = meterTitle(lc, m)
	return m
}

// DualProviderMeters picks one meter per provider, Codex first, titled
// "Codex · 5h" style.
func DualProviderMeters(lc l10n.Localizer, codex, grok []Meter, codexLabel, grokLabel string) []Meter {
	var out []Meter
	if m, ok := PrimaryStandardMeter(codex, codexPreferredWindows); ok {
		m.Title = codexLabel + " · " + meterTitle(lc, m)
		out = append(out, m)
	}
	var g Meter
	ok := false
	if len(grok) > 0 {
		g, ok = PrimaryStandardMeter(grok[:1], grokPreferredWindows)
	}
	if !ok && len(grok) > 0 {
		g, ok = grok[0], true
	}
	if ok {
		g.Title = grokLabel + " · " + meterTitle(lc, g)
		out = append(out, g)
	}
	return out
}

// StatusInputs is everything the status item selection reads.
type StatusInputs struct {
	Prefs    store.Preferences
	Provider store.Provider
	// Codex side.
	CodexText   string
	CodexMeters []Meter
	// Grok side; the zero state reads as loading.
	Grok GrokState
	// Claude side. The preferred status window is 5 hours, then 7 days.
	Claude ClaudeState
	Cursor CursorState
	Now    time.Time
}

// GrokStatusText is the status text of Grok: the countdown of the first
// meter, its remaining percentage, or the availability fallback.
func GrokStatusText(lc l10n.Localizer, g GrokState, now time.Time) string {
	if len(g.Meters) == 0 {
		switch g.Availability {
		case GrokNotLoggedIn, GrokReauthRequired:
			return lc.Text(l10n.KeyStatusLogin)
		case GrokLoading:
			return lc.Text(l10n.KeyStatusWait)
		case GrokReady:
			return lc.Text(l10n.KeyStatusUnknown)
		}
		return lc.Text(l10n.KeyStatusError)
	}
	m := g.Meters[0]
	if m.HasReset() {
		return DurationLocalized(m.ResetsAt.Sub(now).Seconds(), lc.Language(), false)
	}
	return percentText(m)
}

// statusProviders is the ordered status-bar selection. An empty custom list
// and the selected scope follow the main panel.
func statusProviders(in StatusInputs) []store.Provider {
	if sel := in.Prefs.StatusBarSelection(); len(sel) > 0 {
		return sel
	}
	if in.Provider.Valid() {
		return []store.Provider{in.Provider}
	}
	return []store.Provider{store.ProviderCodex}
}

func statusTextFor(lc l10n.Localizer, in StatusInputs, p store.Provider) string {
	switch p {
	case store.ProviderCursor:
		return CursorStatusText(lc, in.Cursor)
	case store.ProviderGrok:
		return GrokStatusText(lc, in.Grok, in.Now)
	case store.ProviderClaude:
		return ClaudeStatusText(lc, in.Claude, in.Now)
	default:
		return in.CodexText
	}
}

// ClaudeStatusText is the status text of Claude. The first meter is the
// 5-hour window when one exists.
func ClaudeStatusText(lc l10n.Localizer, g ClaudeState, now time.Time) string {
	if len(g.Meters) == 0 {
		switch g.Availability {
		case ClaudeNotLoggedIn, ClaudeReauthRequired, ClaudeAPIKeyMode:
			return lc.Text(l10n.KeyStatusLogin)
		case ClaudeLoading:
			return lc.Text(l10n.KeyStatusWait)
		case ClaudeReady:
			return lc.Text(l10n.KeyStatusUnknown)
		}
		return lc.Text(l10n.KeyStatusError)
	}
	m := g.Meters[0]
	for _, item := range g.Meters {
		if item.WindowMinutes == FiveHourMinutes {
			m = item
			break
		}
	}
	if m.HasReset() {
		return DurationLocalized(m.ResetsAt.Sub(now).Seconds(), lc.Language(), false)
	}
	return percentText(m)
}

// SelectedStatusText is the status item text for the chosen provider scope.
func SelectedStatusText(lc l10n.Localizer, in StatusInputs) string {
	picked := statusProviders(in)
	if len(picked) == 2 && !statusHasMeters(in, picked[0]) {
		return statusTextFor(lc, in, picked[1])
	}
	return statusTextFor(lc, in, picked[0])
}

// statusHasMeters reports whether a provider has a reading to show; a pair
// falls back to the second provider's text otherwise.
func statusHasMeters(in StatusInputs, p store.Provider) bool {
	if p == store.ProviderCodex {
		return len(in.CodexMeters) > 0 && in.CodexText != ""
	}
	return len(metersOf(in, p)) > 0
}

// SelectedMeters are the meters the status item draws.
func SelectedMeters(lc l10n.Localizer, in StatusInputs) []Meter {
	picked := statusProviders(in)
	if len(picked) == 2 && picked[0] == store.ProviderCodex && picked[1] == store.ProviderGrok {
		return DualProviderMeters(lc, in.CodexMeters, in.Grok.Meters,
			lc.Text(l10n.KeyProviderCodex), lc.Text(l10n.KeyProviderGrok))
	}
	if len(picked) == 2 {
		return combinedMeters(lc, in, picked[0], picked[1])
	}
	switch picked[0] {
	case store.ProviderCursor:
		m := in.Cursor.LiveMeters()
		return m[:min(2, len(m))]
	case store.ProviderCodex:
		return in.CodexMeters
	case store.ProviderGrok:
		if len(in.Grok.Meters) == 0 {
			return nil
		}
		return []Meter{WithShortTitle(lc, in.Grok.Meters[0])}
	case store.ProviderClaude:
		return claudeStatusMeters(lc, in.Claude.Meters)
	default:
		return nil
	}
}

// claudeStatusMeters are the 5-hour and weekly meters with short titles;
// per-model windows stay in the panel.
func claudeStatusMeters(lc l10n.Localizer, meters []Meter) []Meter {
	var out []Meter
	for _, m := range meters {
		if m.Source == SourceStandard {
			out = append(out, WithShortTitle(lc, m))
		}
	}
	if len(out) == 0 && len(meters) > 0 {
		out = append(out, WithShortTitle(lc, meters[0]))
	}
	return out
}

func combinedMeters(lc l10n.Localizer, in StatusInputs, a, b store.Provider) []Meter {
	var out []Meter
	if m, ok := labeledMeter(lc, metersOf(in, a), preferredOf(a), lc.Text(providerStatusKey(a))); ok {
		out = append(out, m)
	}
	if m, ok := labeledMeter(lc, metersOf(in, b), preferredOf(b), lc.Text(providerStatusKey(b))); ok {
		out = append(out, m)
	}
	return out
}

func metersOf(in StatusInputs, p store.Provider) []Meter {
	switch p {
	case store.ProviderCursor:
		return in.Cursor.LiveMeters()
	case store.ProviderGrok:
		return in.Grok.Meters
	case store.ProviderClaude:
		return in.Claude.Meters
	default:
		return in.CodexMeters
	}
}

func preferredOf(p store.Provider) []int {
	switch p {
	case store.ProviderCursor:
		return nil
	case store.ProviderGrok:
		return grokPreferredWindows
	case store.ProviderClaude:
		return claudePreferredWindows
	default:
		return codexPreferredWindows
	}
}

func providerStatusKey(p store.Provider) l10n.Key {
	switch p {
	case store.ProviderCursor:
		return l10n.KeyProviderCursor
	case store.ProviderGrok:
		return l10n.KeyProviderGrok
	case store.ProviderClaude:
		return l10n.KeyProviderClaude
	default:
		return l10n.KeyProviderCodex
	}
}

func labeledMeter(lc l10n.Localizer, meters []Meter, preferred []int, label string) (Meter, bool) {
	m, ok := PrimaryStandardMeter(meters, preferred)
	if !ok && len(meters) > 0 {
		m, ok = meters[0], true
	}
	if !ok {
		return Meter{}, false
	}
	m.Title = label + " · " + meterTitle(lc, m)
	return m, true
}

// StatusBarItem is the text data of one meter in the status item.
type StatusBarItem struct {
	Meter Meter
	// Ring is the remaining percentage ("--" without a meter).
	Ring          string
	Countdown     string
	Battery       string
	BatteryDetail string
	MeterText     string
}

// StatusBarView is the data side of the status item: what to draw, no pixels.
type StatusBarView struct {
	Style store.StatusBarStyle
	Text  string
	Plan  RenderPlan
	Items []StatusBarItem
	// TextCaptions are the captions of the "text" style ("NN%" or "--").
	TextCaptions []string
	Tooltip      string
}

// BuildStatusBar computes the status item content for the preferences.
func BuildStatusBar(lc l10n.Localizer, in StatusInputs) StatusBarView {
	text := SelectedStatusText(lc, in)
	meters := SelectedMeters(lc, in)
	plan := MakeRenderPlan(in.Prefs.StatusBarDisplayStyle, in.Prefs.StatusBarBatteryScope, meters)
	view := StatusBarView{Style: in.Prefs.StatusBarDisplayStyle, Text: text, Plan: plan}
	lang := lc.Language()
	for _, m := range plan.Meters {
		view.Items = append(view.Items, StatusBarItem{
			Meter:         m,
			Ring:          RingText(&m),
			Countdown:     CountdownCaption(m, in.Now, lang),
			Battery:       BatteryCaption(m, in.Prefs.StatusBarBatteryDetailStyle, in.Now, lang),
			BatteryDetail: BatteryDetail(m, in.Prefs.StatusBarBatteryDetailStyle, in.Now, lang),
			MeterText:     MeterCaption(m, in.Prefs.StatusBarMetersDetailStyle, in.Now, lang),
		})
		view.TextCaptions = append(view.TextCaptions, percentText(m))
	}
	if len(plan.Meters) == 0 {
		view.TextCaptions = []string{RingText(nil)}
	}
	view.Tooltip = Tooltip(text, meters)
	return view
}

// Tooltip is "Quosparr · text" plus one "title: NN%" line.
func Tooltip(text string, meters []Meter) string {
	head := "Quosparr · " + text
	if len(meters) == 0 {
		return head
	}
	parts := make([]string, len(meters))
	for i, m := range meters {
		parts[i] = m.Title + ": " + percentText(m)
	}
	return head + "\n" + strings.Join(parts, " · ")
}

// RingText is the remaining percentage, the balance amount, or "--".
func RingText(m *Meter) string {
	if m == nil {
		return "--"
	}
	if m.ValueText != "" {
		return m.ValueText
	}
	return strconv.Itoa(m.RemainingPercent)
}

func resetCountdown(m Meter, now time.Time, lang l10n.Language) string {
	if !m.HasReset() {
		return "--"
	}
	return DurationLocalized(m.ResetsAt.Sub(now).Seconds(), lang, false)
}

func titled(m Meter, detail string) string {
	if m.Source == SourceModelSpecific {
		return detail + " " + m.Title
	}
	return m.Title + " " + detail
}

// CountdownCaption is the caption of the "countdown" style.
func CountdownCaption(m Meter, now time.Time, lang l10n.Language) string {
	if m.Balance {
		return titled(m, percentText(m))
	}
	return titled(m, resetCountdown(m, now, lang))
}

// BatteryDetail is the detail text of the "battery" style.
func BatteryDetail(m Meter, style store.BatteryDetail, now time.Time, lang l10n.Language) string {
	if style == store.BatteryDetailRemainingPercent || m.Balance {
		return percentText(m)
	}
	return resetCountdown(m, now, lang)
}

// BatteryCaption is the caption of the "battery" style.
func BatteryCaption(m Meter, style store.BatteryDetail, now time.Time, lang l10n.Language) string {
	return titled(m, BatteryDetail(m, style, now, lang))
}

// MeterDetail is the detail text of the "meters" style.
func MeterDetail(m Meter, style store.MetersDetail, now time.Time, lang l10n.Language) string {
	reset := "--"
	if m.HasReset() {
		reset = ResetShortLabel(m.ResetsAt, now, nil)
	}
	switch {
	case m.Balance:
		return percentText(m)
	case style == store.MetersDetailResetTime:
		return reset
	case style == store.MetersDetailBoth:
		return percentText(m) + " · " + reset
	}
	return percentText(m)
}

// MeterCaption is the caption of the "meters" style.
func MeterCaption(m Meter, style store.MetersDetail, now time.Time, lang l10n.Language) string {
	return titled(m, MeterDetail(m, style, now, lang))
}
