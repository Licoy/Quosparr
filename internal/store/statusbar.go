package store

import (
	"encoding/json"
	"slices"
)

// StatusBarWindow is which quota windows the status item shows. Only the Go
// app knows it, so the automatic choice is the empty value and is not written.
type StatusBarWindow string

// StatusBarWindow values.
const (
	// StatusBarWindowAuto shows the two main windows of a provider that has
	// a component of its own, and the main window of one sharing it.
	StatusBarWindowAuto StatusBarWindow = ""
	// StatusBarWindowFiveHour and StatusBarWindowWeekly show that window of
	// every provider, or its main window where it has none.
	StatusBarWindowFiveHour StatusBarWindow = "fiveHour"
	StatusBarWindowWeekly   StatusBarWindow = "weekly"
)

// StatusBarWindows returns every valid StatusBarWindow.
func StatusBarWindows() []StatusBarWindow {
	return []StatusBarWindow{StatusBarWindowAuto, StatusBarWindowFiveHour, StatusBarWindowWeekly}
}

// Valid reports whether v is a known value.
func (v StatusBarWindow) Valid() bool { return slices.Contains(StatusBarWindows(), v) }

// BatteryOrientation is how the "battery" status bar style stands. Only the
// Go app knows it, so lying flat is the empty value and is not written.
type BatteryOrientation string

// BatteryOrientation values.
const (
	BatteryHorizontal BatteryOrientation = ""
	BatteryVertical   BatteryOrientation = "vertical"
)

// BatteryOrientations returns every valid BatteryOrientation.
func BatteryOrientations() []BatteryOrientation {
	return []BatteryOrientation{BatteryHorizontal, BatteryVertical}
}

// Valid reports whether v is a known value.
func (v BatteryOrientation) Valid() bool { return slices.Contains(BatteryOrientations(), v) }

// Combines reports whether the style can stack two providers in one
// component; rings and text show every provider on its own.
func (v StatusBarStyle) Combines() bool {
	return v == StatusBarStyleMeters || v == StatusBarStyleCountdown || v == StatusBarStyleBattery
}

// MaxStatusBarGroup is how many providers one component stacks.
const MaxStatusBarGroup = 2

// StatusBarLayout is Go-only: what the status item shows beyond the settings
// the Swift app has. It is written as one object, left out while every field
// is at its default, and an unreadable value is the default.
type StatusBarLayout struct {
	// Groups are the custom providers in display order, one component each:
	// a provider on its own, or two stacked. Empty uses StatusBarProviders
	// as one component, as versions before it did.
	Groups    [][]Provider       `json:"groups,omitempty"`
	Window    StatusBarWindow    `json:"window,omitempty"`
	Battery   BatteryOrientation `json:"battery,omitempty"`
	HideIcons bool               `json:"hideIcons,omitempty"`
	// RingValues puts each window's value inside a ring of its own.
	RingValues bool `json:"ringValues,omitempty"`
}

func (l StatusBarLayout) clone() StatusBarLayout {
	l.Groups = cloneGroups(l.Groups)
	return l
}

func cloneGroups(groups [][]Provider) [][]Provider {
	if groups == nil {
		return nil
	}
	out := make([][]Provider, len(groups))
	for i, g := range groups {
		out[i] = slices.Clone(g)
	}
	return out
}

// normalize drops unknown values and keeps the groups canonical.
func (l *StatusBarLayout) normalize() {
	if !l.Window.Valid() {
		l.Window = StatusBarWindowAuto
	}
	if !l.Battery.Valid() {
		l.Battery = BatteryHorizontal
	}
	l.Groups = canonicalStatusBarGroups(l.Groups)
}

// canonicalStatusBarGroups keeps the order, drops unknown and repeated
// providers and empty groups, and moves a third provider of a group into a
// group of its own. Nothing left is nil.
func canonicalStatusBarGroups(groups [][]Provider) [][]Provider {
	var out [][]Provider
	seen := map[Provider]bool{}
	for _, g := range groups {
		var kept []Provider
		for _, p := range g {
			if !p.Valid() || seen[p] {
				continue
			}
			seen[p] = true
			if len(kept) == MaxStatusBarGroup {
				out = append(out, kept)
				kept = nil
			}
			kept = append(kept, p)
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	return out
}

// flatGroups lists the providers of groups in order.
func flatGroups(groups [][]Provider) []Provider {
	var out []Provider
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// legacyStatusBarProviders is what versions before the groups read: the
// first two providers.
func legacyStatusBarProviders(groups [][]Provider) []Provider {
	flat := flatGroups(groups)
	return canonicalStatusBarProviders(flat[:min(2, len(flat))])
}

// StatusBarGroups is the status item's providers by component. Nil means
// follow the main panel. Groups written by an older version, which left
// them behind when it changed the providers, are not used.
func (p Preferences) StatusBarGroups() [][]Provider {
	legacy := p.StatusBarSelection()
	if legacy == nil {
		return nil
	}
	if p.StatusBarProviderScope == ProviderScopeCustom && len(p.StatusBar.Groups) > 0 &&
		slices.Equal(legacyStatusBarProviders(p.StatusBar.Groups), legacy) {
		return cloneGroups(p.StatusBar.Groups)
	}
	return [][]Provider{legacy}
}

// SetStatusBarGroups chooses the custom components. The first two providers
// are kept in StatusBarProviders too, for the versions that read only them.
// No provider leaves the settings as they are.
func (p *Preferences) SetStatusBarGroups(groups [][]Provider) {
	groups = canonicalStatusBarGroups(groups)
	if len(groups) == 0 {
		return
	}
	p.StatusBarProviderScope = ProviderScopeCustom
	p.StatusBarProviders = legacyStatusBarProviders(groups)
	p.StatusBar.Groups = groups
}

// StatusBarQuotaWindow is the window the status item shows. Before the
// layout the battery style had a window of its own, so it keeps that one
// until a window is chosen.
func (p Preferences) StatusBarQuotaWindow() StatusBarWindow {
	if p.StatusBar.Window != StatusBarWindowAuto || p.StatusBarDisplayStyle != StatusBarStyleBattery {
		return p.StatusBar.Window
	}
	switch p.StatusBarBatteryScope {
	case BatteryScopeFiveHour:
		return StatusBarWindowFiveHour
	case BatteryScopeWeekly:
		return StatusBarWindowWeekly
	}
	return StatusBarWindowAuto
}

// SetStatusBarQuotaWindow chooses the window of every style, and the battery
// window of the versions that read only that.
func (p *Preferences) SetStatusBarQuotaWindow(w StatusBarWindow) {
	p.StatusBar.Window = w
	switch w {
	case StatusBarWindowFiveHour:
		p.StatusBarBatteryScope = BatteryScopeFiveHour
	case StatusBarWindowWeekly:
		p.StatusBarBatteryScope = BatteryScopeWeekly
	default:
		p.StatusBarBatteryScope = BatteryScopeBoth
	}
}

// readStatusBarLayout decodes the Go-only layout leniently, as
// readFloatingWindow: an unreadable object is the default.
func readStatusBarLayout(r *reader, key string) StatusBarLayout {
	raw, ok := r.take(key)
	var l StatusBarLayout
	if !ok || json.Unmarshal(raw, &l) != nil {
		return StatusBarLayout{}
	}
	l.normalize()
	return l
}
