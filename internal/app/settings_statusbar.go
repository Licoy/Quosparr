package app

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// statusBarPage is the menu bar item tab: a preview of the current settings,
// the styles, the providers, then how they show. Where the OS has no such
// status item the settings stay, disabled and explained, with their values.
func (u *settingsUI) statusBarPage(c *ui.Context) {
	s, l := u.s, u.L()
	p := s.Prefs()
	supported := s.System().StatusBarStyles
	off := s.ReadOnly() || !supported
	hint := tx(l, xStatusBarPreviewHint)
	if !supported {
		hint = tx(l, xMacOnly)
	}
	settingsSection(c, tx(l, xStatusBarPreview), hint, nil, func() { statusBarPreview(c, l, p) })
	settingsSection(c, l.Text(l10n.KeyStatusBarStyle), "", nil, func() {
		ui.Row(c).Gap(10).Padding(12, 0).Children(func() {
			for _, st := range store.StatusBarStyles() {
				if statusBarStyleTile(c, l, p, st, off).Clicked() && st != p.StatusBarDisplayStyle {
					s.SetStatusBarStyle(st)
				}
			}
		})
	})
	custom := p.StatusBarGroups() != nil
	hint = tx(l, xStatusBarFollowHint)
	switch {
	case custom && p.StatusBarDisplayStyle.Combines():
		hint = tx(l, xStatusBarArrangementHint)
	case custom:
		hint = tx(l, xStatusBarSeparateHint)
	}
	settingsSection(c, l.Text(l10n.KeyStatusBarProviderScope), hint, func() {
		cur := store.ProviderScopeSelected
		if custom {
			cur = store.ProviderScopeCustom
		}
		scopes := []store.StatusBarProviderScope{store.ProviderScopeSelected, store.ProviderScopeCustom}
		if v, ok := choose(c, "sb-scope", l.Text(l10n.KeyStatusBarProviderScope), cur, scopes,
			func(v store.StatusBarProviderScope) string { return l.Text(providerScopeKey(v)) }, off); ok {
			s.SetStatusBarProviderScope(v)
		}
	}, func() {
		if custom {
			u.statusBarArrangement(c, p, off)
			return
		}
		// Following the panel: its provider, as it is now.
		prov := p.SelectedProvider
		if !prov.Valid() {
			prov = store.ProviderCodex
		}
		statusBarProviderLine(c, prov, l.Text(providerKey(prov)), true, nil)
	})
	settingsGroup(c, tx(l, xStatusBarOptions), "", nil, func(row rowFunc) { u.statusBarOptionRows(c, row, p, off) })
}

// statusBarOptionRows are the options of the chosen style, then those of
// every style.
func (u *settingsUI) statusBarOptionRows(c *ui.Context, row rowFunc, p store.Preferences, off bool) {
	s, l := u.s, u.L()
	switch p.StatusBarDisplayStyle {
	case store.StatusBarStyleMeters:
		row(l.Text(l10n.KeyStatusBarMetersDetailStyle), "", func() {
			if v, ok := choose(c, "sb-meters", l.Text(l10n.KeyStatusBarMetersDetailStyle), p.StatusBarMetersDetailStyle, store.MetersDetails(),
				func(v store.MetersDetail) string { return l.Text(metersDetailKey(v)) }, off); ok {
				s.SetStatusBarMetersDetail(v)
			}
		})
	case store.StatusBarStyleBattery:
		row(tx(l, xBatteryOrientation), tx(l, xBatteryOrientationHint), func() {
			if v, ok := pills(c, "sb-battery", p.StatusBar.Battery, store.BatteryOrientations(), func(v store.BatteryOrientation) string {
				if v == store.BatteryVertical {
					return tx(l, xBatteryVertical)
				}
				return tx(l, xBatteryHorizontal)
			}, off); ok {
				s.SetStatusBarLayout(func(b *store.StatusBarLayout) { b.Battery = v })
			}
		})
		row(tx(l, xBatteryText), "", func() {
			if v, ok := choose(c, "sb-bdetail", tx(l, xBatteryText), p.StatusBarBatteryDetailStyle, store.BatteryDetails(),
				func(v store.BatteryDetail) string { return l.Text(batteryDetailKey(v)) }, off); ok {
				s.SetStatusBarBatteryDetail(v)
			}
		})
	case store.StatusBarStyleRings:
		row(tx(l, xRingValues), tx(l, xRingValuesHint), func() {
			if v, ok := toggle(c, "sb-ring-values", tx(l, xRingValues), p.StatusBar.RingValues, off); ok {
				s.SetStatusBarLayout(func(b *store.StatusBarLayout) { b.RingValues = v })
			}
		})
	}
	row(tx(l, xStatusBarWindow), tx(l, xStatusBarWindowHint), func() {
		if v, ok := choose(c, "sb-window", tx(l, xStatusBarWindow), p.StatusBarQuotaWindow(), store.StatusBarWindows(),
			func(v store.StatusBarWindow) string { return statusBarWindowName(l, v) }, off); ok {
			s.Update(func(p *store.Preferences) { p.SetStatusBarQuotaWindow(v) })
		}
	})
	row(tx(l, xStatusBarIcons), tx(l, xStatusBarIconsHint), func() {
		if v, ok := toggle(c, "sb-icons", tx(l, xStatusBarIcons), !p.StatusBar.HideIcons, off); ok {
			s.SetStatusBarLayout(func(b *store.StatusBarLayout) { b.HideIcons = !v })
		}
	})
}

func statusBarWindowName(l l10n.Localizer, v store.StatusBarWindow) string {
	switch v {
	case store.StatusBarWindowFiveHour:
		return runway.ShortWindowTitle(l, runway.FiveHourMinutes)
	case store.StatusBarWindowWeekly:
		return runway.ShortWindowTitle(l, runway.WeeklyMinutes)
	}
	return tx(l, xStatusBarWindowAuto)
}

// statusBarArrangement lists the custom providers in order, a line each:
// shown ones first, stacked under the one before or not, then the others.
func (u *settingsUI) statusBarArrangement(c *ui.Context, p store.Preferences, off bool) {
	s, l := u.s, u.L()
	t := c.Theme()
	slots := statusBarSlots(p.StatusBarGroups())
	combine := p.StatusBarDisplayStyle.Combines()
	apply := func(next []statusBarSlot) {
		if !off {
			s.SetStatusBarGroups(statusBarGroupsOf(next))
		}
	}
	order := make([]store.Provider, 0, len(store.Providers()))
	for _, slot := range slots {
		order = append(order, slot.prov)
	}
	for _, prov := range store.Providers() {
		if !slices.Contains(order, prov) {
			order = append(order, prov)
		}
	}
	for i, prov := range order {
		shown := i < len(slots)
		name := l.Text(providerKey(prov))
		if i > 0 {
			ui.Box(c).Height(1).Background(pal(t).Line)
		}
		statusBarProviderLine(c, prov, name, shown, func() {
			if shown && combine && i > 0 {
				on := slots[i].joined
				box := ui.Checkbox(c.Key("sb-join-"+string(prov)), &on, tx(l, xStatusBarJoin)).
					Label(tx(l, xStatusBarJoinProvider, name)).Disabled(off || !on && !canJoinStatusBarSlot(slots, i))
				if box.Changed() {
					next := slices.Clone(slots)
					next[i].joined = on
					apply(next)
				}
			}
			if shown {
				if IconButton(c.Key("sb-up-"+string(prov)), icChevUp, l.Text(l10n.KeyAccountsMoveUp), off || i == 0).Clicked() {
					apply(swapStatusBarSlots(slots, i, i-1))
				}
				if IconButton(c.Key("sb-down-"+string(prov)), icChevDown, l.Text(l10n.KeyAccountsMoveDown), off || i+1 >= len(slots)).Clicked() {
					apply(swapStatusBarSlots(slots, i, i+1))
				}
			}
			// Hiding the last one is not offered.
			if v, ok := toggle(c, "sb-show-"+string(prov), tx(l, xStatusBarShowProvider, name), shown, off || shown && len(slots) <= 1); ok {
				if v {
					apply(append(slices.Clone(slots), statusBarSlot{prov: prov}))
				} else {
					apply(withoutStatusBarSlot(slots, i))
				}
			}
		})
	}
}

// statusBarProviderLine is a provider's line: its logo and name, dimmed
// while it is not shown, then the controls.
func statusBarProviderLine(c *ui.Context, prov store.Provider, name string, shown bool, controls func()) {
	t := c.Theme()
	ui.Row(c).Gap(10).Padding(8, 0).MinHeight(44).AlignItems(ui.Center).Children(func() {
		icon := ProviderIcon(c, prov, 18)
		col := t.Text
		if !shown {
			icon.Opacity(0.45)
			col = t.TextMuted
		}
		ui.Text(c, name).FontSize(13).TextColor(col).Grow(1).Basis(0).SingleLine()
		if controls != nil {
			controls()
		}
	})
}

// statusBarSlot is a custom provider in display order, stacked under the one
// before it or starting a component.
type statusBarSlot struct {
	prov   store.Provider
	joined bool
}

func statusBarSlots(groups [][]store.Provider) []statusBarSlot {
	var out []statusBarSlot
	for _, g := range groups {
		for i, p := range g {
			out = append(out, statusBarSlot{prov: p, joined: i > 0})
		}
	}
	return out
}

func statusBarGroupsOf(slots []statusBarSlot) [][]store.Provider {
	var out [][]store.Provider
	for i, s := range slots {
		if i > 0 && s.joined && len(out[len(out)-1]) < store.MaxStatusBarGroup {
			out[len(out)-1] = append(out[len(out)-1], s.prov)
			continue
		}
		out = append(out, []store.Provider{s.prov})
	}
	return out
}

// canJoinStatusBarSlot reports whether slot i can stack under the one before
// it: neither is already stacked with another.
func canJoinStatusBarSlot(slots []statusBarSlot, i int) bool {
	return i > 0 && !slots[i-1].joined && (i+1 >= len(slots) || !slots[i+1].joined)
}

// swapStatusBarSlots swaps two providers; the components stay in place.
func swapStatusBarSlots(slots []statusBarSlot, i, j int) []statusBarSlot {
	next := slices.Clone(slots)
	next[i].prov, next[j].prov = next[j].prov, next[i].prov
	return next
}

// withoutStatusBarSlot leaves a provider out; one stacked under it starts
// the component instead.
func withoutStatusBarSlot(slots []statusBarSlot, i int) []statusBarSlot {
	next := slices.Clone(slots)
	if i+1 < len(next) && !next[i].joined {
		next[i+1].joined = false
	}
	return slices.Delete(next, i, i+1)
}

// statusBarPreview draws the status item of the settings, over sample
// quotas, on a strip of menu bar.
func statusBarPreview(c *ui.Context, l l10n.Localizer, p store.Preferences) {
	t := c.Theme()
	r := statusBarSample(l, p)
	ui.Box(c).Height(72).Margin(10, 0).Draw(func(pt *ui.Painter, rect ui.Rect) {
		w := trayContentWidth(r)
		scale := min(2, (rect.W-24)/w)
		bar := ui.Rect{X: rect.X, Y: rect.Y + (rect.H-(trayContentHeight+6)*scale)/2, W: rect.W, H: (trayContentHeight + 6) * scale}
		drawStatusBarStrip(pt, t, bar, r, scale)
	})
}

// drawStatusBarStrip draws r centered on a strip of menu bar.
func drawStatusBarStrip(pt *ui.Painter, t *ui.Theme, bar ui.Rect, r TrayRender, scale float32) {
	fill, ink := ui.RGBA(0, 0, 0, 0.05), ui.RGB(0, 0, 0)
	if t.Dark {
		fill, ink = ui.RGBA(255, 255, 255, 0.07), ui.RGB(255, 255, 255)
	}
	pt.Fill(bar, fill, 7)
	w := trayContentWidth(r)
	x, y := bar.X+(bar.W-w*scale)/2, bar.Y+(bar.H-trayContentHeight*scale)/2
	pt.Transform(ui.Scaling(scale, scale).Then(ui.Translation(x, y)), 0, 0, func() {
		// As the menu bar composes them: the colors over the ink.
		drawTrayContent(pt, r, trayPaint{ink: ink})
		drawTrayContent(pt, r, trayPaint{ink: ui.Transparent, colorOpacity: 1})
	})
}

// statusBarStyleTile is a choice of style, previewing it with Codex alone.
func statusBarStyleTile(c *ui.Context, l l10n.Localizer, p store.Preferences, st store.StatusBarStyle, off bool) ui.Element {
	t := c.Theme()
	name := statusBarStyleName(l, st)
	chosen := p.StatusBarDisplayStyle == st
	p.StatusBarDisplayStyle = st
	p.SetStatusBarGroups([][]store.Provider{{store.ProviderCodex}})
	r := statusBarSample(l, p)
	b := ui.ButtonBase(c.Key("sb-style-" + string(st))).Label(name).Tooltip(name).Disabled(off).
		Grow(1).Basis(0).MinWidth(0).Height(statusBarTileHeight).Cursor(ui.CursorPointer)
	// The tile draws its own chosen look: a frame, not the selected fill.
	b.Role(ui.RoleRadio).Selected(chosen).Background(ui.Transparent)
	hovered := b.Hovered() && !off
	return b.Draw(func(pt *ui.Painter, rect ui.Rect) {
		border, width := pal(t).Line, float32(1)
		if chosen {
			border, width = t.Accent, 2
			pt.Fill(rect, t.Accent.Alpha(0.08), 10)
		} else if hovered {
			pt.Fill(rect, pal(t).Hover, 10)
		}
		pt.Stroke(inset(rect, width/2), border, 10, width)
		labelH := float32(26)
		bar := ui.Rect{X: rect.X + 8, Y: rect.Y + 10, W: rect.W - 16, H: rect.H - 10 - labelH}
		scale := min(1.25, (bar.W-8)/trayContentWidth(r))
		bar.H = min(bar.H, (trayContentHeight+6)*scale)
		bar.Y = rect.Y + 10 + (rect.H-10-labelH-bar.H)/2
		drawStatusBarStrip(pt, t, bar, r, scale)
		col := t.TextMuted
		if chosen {
			col = t.Text
		}
		f := floatingFont(12, 500, "")
		tw := min(floatingTextWidth(name, f), rect.W-16)
		floatingText(pt, name, f, rect.X+(rect.W-tw)/2, rect.Y+rect.H-labelH/2-2, rect.W-16, col)
	})
}

const statusBarTileHeight = 92

// statusBarStyleName names a style; the battery shows more than a countdown.
func statusBarStyleName(l l10n.Localizer, st store.StatusBarStyle) string {
	if st == store.StatusBarStyleBattery {
		return tx(l, xStatusBarBattery)
	}
	return l.Text(statusBarStyleKey(st))
}

// statusBarSample is the status item of p over made-up quotas, two windows
// for every provider.
func statusBarSample(l l10n.Localizer, p store.Preferences) TrayRender {
	now := time.Now()
	meter := func(minutes, left int, h runway.Health, reset time.Duration) runway.Meter {
		return runway.Meter{Title: runway.ShortWindowTitle(l, minutes), WindowMinutes: minutes, RemainingPercent: left, Health: h, ResetsAt: now.Add(reset)}
	}
	five, week, month := runway.FiveHourMinutes, runway.WeeklyMinutes, runway.MonthlyMinutes
	return TrayFromStatus(runway.BuildStatusBar(l, runway.StatusInputs{Prefs: p, Provider: p.SelectedProvider, Now: now,
		CodexMeters: []runway.Meter{meter(five, 73, runway.HealthGreen, 2*time.Hour+15*time.Minute), meter(week, 41, runway.HealthYellow, 77*time.Hour)},
		Claude: runway.ClaudeState{Availability: runway.ClaudeReady, Meters: []runway.Meter{
			meter(five, 18, runway.HealthRed, 48*time.Minute), meter(week, 64, runway.HealthGreen, 120*time.Hour)}},
		Grok: runway.GrokState{Availability: runway.GrokReady, Meters: []runway.Meter{
			meter(week, 85, runway.HealthGreen, 30*time.Hour), meter(month, 52, runway.HealthGreen, 400*time.Hour)}},
	}))
}
