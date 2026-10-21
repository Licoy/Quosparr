package app

import (
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// floatingPage is the floating window tab: the switch, the styles, then the
// options. Everything stays editable while the window is off, for when it
// is turned on.
func (u *settingsUI) floatingPage(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	f := p.FloatingWindow
	ro := s.ReadOnly()
	set := func(change func(*store.FloatingWindow)) { s.SetFloatingWindow(change) }
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		hint := tx(l, xFloatingEnabledHint)
		if runtime.GOOS == "linux" && os.Getenv("XDG_SESSION_TYPE") == "wayland" {
			hint += "\n" + tx(l, xFloatingWayland)
		}
		row(tx(l, xFloatingEnabled), hint, func() {
			if v, ok := toggle(c, "fw-enabled", tx(l, xFloatingEnabled), f.Enabled, ro); ok {
				set(func(f *store.FloatingWindow) { f.Enabled = v })
			}
		})
	})
	settingsSection(c, tx(l, xFloatingStyle), "", nil, func() {
		ui.Row(c).Gap(10).Padding(12, 0).Children(func() {
			for _, st := range store.FloatingStyles() {
				if floatingStyleTile(c, l, p, st, ro).Clicked() && st != f.Style {
					set(func(f *store.FloatingWindow) { f.Style = st })
				}
			}
		})
	})
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		u.floatingProviderRows(c, row, p, ro)
		row(tx(l, xFloatingOpacity), "", func() {
			if v, ok := choose(c, "fw-opacity", tx(l, xFloatingOpacity), f.OpacityPercent(), store.FloatingOpacityOptions(),
				func(v int) string { return strconv.Itoa(v) + "%" }, ro); ok {
				set(func(f *store.FloatingWindow) { f.Opacity = v })
			}
		})
		row(tx(l, xFloatingLocked), tx(l, xFloatingLockedHint), func() {
			if v, ok := toggle(c, "fw-locked", tx(l, xFloatingLocked), f.Locked, ro); ok {
				set(func(f *store.FloatingWindow) { f.Locked = v })
			}
		})
		row(tx(l, xFloatingFullScreen), tx(l, xFloatingFullScreenHint), func() {
			if v, ok := toggle(c, "fw-fullscreen", tx(l, xFloatingFullScreen), f.OverFullScreen, ro); ok {
				set(func(f *store.FloatingWindow) { f.OverFullScreen = v })
			}
		})
		u.floatingAppRows(c, row, f, ro)
	})
}

// floatingProviderRows choose the providers: the panel's, or up to all three.
func (u *settingsUI) floatingProviderRows(c *ui.Context, row rowFunc, p store.Preferences, ro bool) {
	s, l := u.s, u.L()
	custom := len(p.FloatingWindow.Providers) > 0
	cur := store.ProviderScopeSelected
	if custom {
		cur = store.ProviderScopeCustom
	}
	row(tx(l, xFloatingProviders), "", func() {
		scopes := []store.StatusBarProviderScope{store.ProviderScopeSelected, store.ProviderScopeCustom}
		if v, ok := choose(c, "fw-scope", tx(l, xFloatingProviders), cur, scopes,
			func(v store.StatusBarProviderScope) string { return l.Text(providerScopeKey(v)) }, ro); ok {
			s.SetFloatingWindow(func(f *store.FloatingWindow) {
				f.Providers = nil
				if v == store.ProviderScopeCustom {
					f.Providers = []store.Provider{p.FloatingSelection()[0]}
				}
			})
		}
	})
	if !custom {
		return
	}
	row(l.Text(l10n.KeyStatusBarProviders), "", func() {
		selected := map[store.Provider]bool{}
		for _, item := range p.FloatingWindow.Providers {
			selected[item] = true
		}
		ui.Row(c).Gap(12).Wrap().Children(func() {
			for _, prov := range store.Providers() {
				on := selected[prov]
				// Dropping the last one is not offered.
				box := ui.Checkbox(c.Key("fw-prov-"+string(prov)), &on, l.Text(providerKey(prov))).
					Disabled(ro || (on && len(selected) <= 1) || (!on && len(selected) >= store.MaxFloatingProviders))
				if !box.Changed() || ro {
					continue
				}
				selected[prov] = on
				next := make([]store.Provider, 0, len(selected))
				for _, item := range store.Providers() {
					if selected[item] {
						next = append(next, item)
					}
				}
				if len(next) > 0 {
					s.SetFloatingWindow(func(f *store.FloatingWindow) { f.Providers = next })
				}
			}
		})
	})
}

// floatingStyleTile is a choice of style, previewing it with sample quotas.
func floatingStyleTile(c *ui.Context, l l10n.Localizer, p store.Preferences, st store.FloatingStyle, ro bool) ui.Element {
	t := c.Theme()
	name := floatingStyleName(l, st)
	chosen := p.FloatingWindow.Style == st
	sample := floatingSample(l)
	ink := floatingInkFor(t)
	family := p.FontFamily
	b := ui.ButtonBase(c.Key("fw-style-" + string(st))).Label(name).Tooltip(name).Disabled(ro).
		Grow(1).Basis(0).MinWidth(0).Height(floatingTileHeight).Cursor(ui.CursorPointer)
	// The tile draws its own chosen look: a frame, not the selected fill.
	b.Role(ui.RoleRadio).Selected(chosen).Background(ui.Transparent)
	hovered := b.Hovered() && !ro
	return b.Draw(func(pt *ui.Painter, r ui.Rect) {
		border, width := pal(t).Line, float32(1)
		if chosen {
			border, width = t.Accent, 2
		}
		if chosen {
			pt.Fill(r, t.Accent.Alpha(0.08), 10)
		} else if hovered {
			pt.Fill(r, pal(t).Hover, 10)
		}
		pt.Stroke(inset(r, width/2), border, 10, width)
		label := floatingFont(12, 500, family)
		labelH := float32(26)
		preview := ui.Rect{X: r.X + 8, Y: r.Y + 8, W: r.W - 16, H: r.H - 8 - labelH}
		w, h := floatingSize(st, sample, family)
		scale := min(1, preview.W/w, preview.H/h)
		x, y := preview.X+(preview.W-w*scale)/2, preview.Y+(preview.H-h*scale)/2
		pt.Transform(ui.Scaling(scale, scale), x, y, func() {
			drawFloating(pt, ui.Rect{X: x, Y: y, W: w, H: h}, st, sample, ink, family, floatingStyleRadius(st, h))
		})
		col := t.TextMuted
		if chosen {
			col = t.Text
		}
		tw := min(floatingTextWidth(name, label), r.W-16)
		floatingText(pt, name, label, r.X+(r.W-tw)/2, r.Y+r.H-labelH/2-2, r.W-16, col)
	})
}

// floatingTileHeight fits the tallest preview, two providers on a card, at
// about half its size above the name.
const floatingTileHeight = 150

// floatingSample are made-up quotas of two providers for the previews.
func floatingSample(l l10n.Localizer) runway.FloatingView {
	reset := func(d time.Duration) string { return runway.DurationLocalized(d.Seconds(), l.Language(), false) }
	five, week := runway.ShortWindowTitle(l, runway.FiveHourMinutes), runway.ShortWindowTitle(l, runway.WeeklyMinutes)
	return runway.FloatingView{Providers: []runway.FloatingProvider{
		{Provider: store.ProviderCodex, Name: l.Text(l10n.KeyProviderCodex), Meters: []runway.FloatingMeter{
			{Label: five, RemainingPercent: 73, Health: runway.HealthGreen, Reset: reset(2*time.Hour + 15*time.Minute)},
			{Label: week, RemainingPercent: 41, Health: runway.HealthYellow, Reset: reset(3*24*time.Hour + 5*time.Hour)},
		}},
		{Provider: store.ProviderClaude, Name: l.Text(l10n.KeyProviderClaude), Meters: []runway.FloatingMeter{
			{Label: five, RemainingPercent: 18, Health: runway.HealthRed, Reset: reset(48 * time.Minute)},
			{Label: week, RemainingPercent: 64, Health: runway.HealthGreen, Reset: reset(5 * 24 * time.Hour)},
		}},
	}}
}
