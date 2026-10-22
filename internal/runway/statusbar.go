package runway

import (
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// Preferred window lengths of a provider's main window.
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

func statusTextFor(lc l10n.Localizer, in StatusInputs, p store.Provider) string {
	switch p {
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

func metersOf(in StatusInputs, p store.Provider) []Meter {
	switch p {
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
	case store.ProviderGrok:
		return l10n.KeyProviderGrok
	case store.ProviderClaude:
		return l10n.KeyProviderClaude
	default:
		return l10n.KeyProviderCodex
	}
}

// StatusBarCell is one quota reading of a status bar component.
type StatusBarCell struct {
	Provider store.Provider
	// Label is the short window name where a provider shows two windows.
	Label string
	// Remaining is the percentage left, Health its color.
	Remaining int
	Health    Health
	// Empty is a provider without a reading; its texts are "--".
	Empty bool
	// Percent is "NN%" or the balance amount, Countdown the time left until
	// the reset (the amount of a balance).
	Percent   string
	Countdown string
	// MeterText and BatteryText are the details the meters and the battery
	// styles are set to show.
	MeterText   string
	BatteryText string
	// RingText is the value inside a ring: the percentage without its sign,
	// empty for a balance, whose amount does not fit.
	RingText string
}

// StatusBarComponent is one component of the status item: a provider on its
// own, or two stacked.
type StatusBarComponent struct {
	// Provider is the provider shown alone, whose windows the cells are; it
	// is empty where two providers share the component, a cell each.
	Provider store.Provider
	Cells    []StatusBarCell
}

// StatusBarView is the data side of the status item: what to draw, no pixels.
type StatusBarView struct {
	Style      store.StatusBarStyle
	Battery    store.BatteryOrientation
	HideIcons  bool
	RingValues bool
	Components []StatusBarComponent
	Tooltip    string
}

// BuildStatusBar computes the status item content for the preferences.
func BuildStatusBar(lc l10n.Localizer, in StatusInputs) StatusBarView {
	layout := in.Prefs.StatusBar
	v := StatusBarView{Style: in.Prefs.StatusBarDisplayStyle, Battery: layout.Battery, HideIcons: layout.HideIcons, RingValues: layout.RingValues}
	tips := []string{"Quosparr"}
	window := in.Prefs.StatusBarQuotaWindow()
	for _, group := range statusGroups(in) {
		var comp StatusBarComponent
		if len(group) == 1 {
			comp.Provider = group[0]
		}
		for _, p := range group {
			picked := statusMeters(metersOf(in, p), preferredOf(p), window, len(group) == 1)
			comp.Cells = append(comp.Cells, statusCells(lc, in, p, picked)...)
			tips = append(tips, statusTip(lc, in, p, picked))
		}
		v.Components = append(v.Components, comp)
	}
	v.Tooltip = strings.Join(tips, "\n")
	return v
}

// statusGroups are the providers of each component. Following the panel is
// its provider alone; rings and text show every provider on its own.
func statusGroups(in StatusInputs) [][]store.Provider {
	groups := in.Prefs.StatusBarGroups()
	if groups == nil {
		p := in.Provider
		if !p.Valid() {
			p = store.ProviderCodex
		}
		return [][]store.Provider{{p}}
	}
	if in.Prefs.StatusBarDisplayStyle.Combines() {
		return groups
	}
	var out [][]store.Provider
	for _, g := range groups {
		for _, p := range g {
			out = append(out, []store.Provider{p})
		}
	}
	return out
}

// statusMeters picks the windows a provider shows: the chosen window, else
// its main window; automatically, up to two main windows when it has the
// component to itself.
func statusMeters(meters []Meter, preferred []int, window store.StatusBarWindow, alone bool) []Meter {
	switch {
	case window == store.StatusBarWindowFiveHour:
		preferred = []int{FiveHourMinutes}
	case window == store.StatusBarWindowWeekly:
		preferred = []int{WeeklyMinutes}
	case alone:
		return floatingMeters(meters, preferred)
	}
	if m, ok := PrimaryStandardMeter(meters, preferred); ok {
		return []Meter{m}
	}
	return nil
}

func statusCells(lc l10n.Localizer, in StatusInputs, p store.Provider, picked []Meter) []StatusBarCell {
	if len(picked) == 0 {
		return []StatusBarCell{{Provider: p, Empty: true, Percent: "--", Countdown: "--", MeterText: "--", BatteryText: "--", RingText: "--"}}
	}
	lang := lc.Language()
	cells := make([]StatusBarCell, len(picked))
	for i, m := range picked {
		cells[i] = StatusBarCell{
			Provider: p, Remaining: m.RemainingPercent, Health: m.Health,
			Percent:     percentText(m),
			Countdown:   CountdownDetail(m, in.Now, lang),
			MeterText:   MeterDetail(m, in.Prefs.StatusBarMetersDetailStyle, in.Now, lang),
			BatteryText: BatteryDetail(m, in.Prefs.StatusBarBatteryDetailStyle, in.Now, lang),
		}
		if !m.Balance {
			cells[i].RingText = strconv.Itoa(m.RemainingPercent)
		}
		if len(picked) > 1 {
			cells[i].Label = meterTitle(lc, m)
		}
	}
	return cells
}

// statusTip is the provider's tooltip line: every window shown, with the
// time left, or why there is none.
func statusTip(lc l10n.Localizer, in StatusInputs, p store.Provider, picked []Meter) string {
	name := lc.Text(providerStatusKey(p))
	if len(picked) == 0 {
		return name + ": " + floatingStatus(lc, in, p)
	}
	parts := make([]string, len(picked))
	for i, m := range picked {
		parts[i] = meterTitle(lc, m) + " " + percentText(m)
		if m.HasReset() && !m.Balance {
			parts[i] += " (" + resetCountdown(m, in.Now, lc.Language()) + ")"
		}
	}
	return name + ": " + strings.Join(parts, " · ")
}

func resetCountdown(m Meter, now time.Time, lang l10n.Language) string {
	if !m.HasReset() {
		return "--"
	}
	return DurationLocalized(m.ResetsAt.Sub(now).Seconds(), lang, false)
}

// shortCountdown is the time left until the reset in short units.
func shortCountdown(m Meter, now time.Time, lang l10n.Language) string {
	if !m.HasReset() {
		return "--"
	}
	return ShortDuration(m.ResetsAt.Sub(now).Seconds(), lang)
}

// CountdownDetail is the text of the "countdown" style: the time left until
// the reset, or the amount of a balance.
func CountdownDetail(m Meter, now time.Time, lang l10n.Language) string {
	if m.Balance {
		return percentText(m)
	}
	return shortCountdown(m, now, lang)
}

// BatteryDetail is the detail text of the "battery" style.
func BatteryDetail(m Meter, style store.BatteryDetail, now time.Time, lang l10n.Language) string {
	if style == store.BatteryDetailRemainingPercent || m.Balance {
		return percentText(m)
	}
	return shortCountdown(m, now, lang)
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
