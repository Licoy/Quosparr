package app

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// floatingMargin is how far from the right of the work area the window
// first shows, vertically centered.
const floatingMargin = 32

// floatingState is the desktop floating window: an always-on-top card with
// the quotas, which a click opens the panel from and a drag moves.
type floatingState struct {
	win  *mygo.Window
	w, h int
	// overFullScreen is the setting the window was set up with.
	overFullScreen bool
	stopWatch      func()
	offDisplays    func()
	// fullScreenHidden is true while another app is full screen, appHidden
	// while the app in front hides the window and appLowered while it takes
	// the window off the top; shown is what the window is.
	fullScreenHidden bool
	appHidden        bool
	appLowered       bool
	shown            floatingShown
	// followKey is the filter and the apps followed, empty when not.
	followKey  string
	stopFollow func()
	drag       floatingDrag
}

// floatingShown is whether the window is hidden and, shown, whether it is
// off the top.
type floatingShown struct{ hidden, lowered bool }

// floatingDrag moves the window from the cursor's screen position, so a
// press that does not move stays a click.
type floatingDrag struct {
	active, moved bool
	cursor        mygo.Point
	x, y          int
}

// SetFloatingWindow changes the floating window settings.
func (s *Settings) SetFloatingWindow(change func(*store.FloatingWindow)) bool {
	return s.Update(func(p *store.Preferences) { change(&p.FloatingWindow) })
}

// floatingContent is what the window shows: the coordinator's view, or the
// chosen providers waiting for it.
func (a *App) floatingContent(prefs store.Preferences) runway.FloatingView {
	v := a.Snapshot().Floating
	if len(v.Providers) > 0 {
		return v
	}
	l := a.L()
	for _, p := range prefs.FloatingSelection() {
		v.Providers = append(v.Providers, runway.FloatingProvider{Provider: p, Name: l.Text(providerKey(p)), Status: "--"})
	}
	return v
}

// syncFloating opens, updates or closes the window after the settings or the
// data changed. It runs on the main thread.
func (a *App) syncFloating() {
	prefs := a.settings.Prefs()
	f := prefs.FloatingWindow
	if !f.Enabled || a.tray == nil || a.startupErr != nil {
		a.closeFloating()
		return
	}
	w, h := a.floatingWindowSize(prefs)
	if a.floating.win == nil {
		a.openFloating(prefs, w, h)
		return
	}
	win := a.floating.win
	if a.floating.overFullScreen != f.OverFullScreen {
		// Changed in place: a new window would lose its place.
		a.setFloatingOverFullScreen(f.OverFullScreen)
	}
	a.syncFloatingFollow(f)
	if w != a.floating.w || h != a.floating.h {
		a.resizeFloating(w, h)
	}
	win.SetOpacity(float64(f.OpacityPercent()) / 100)
	win.Invalidate()
}

func (a *App) floatingWindowSize(prefs store.Preferences) (int, int) {
	w, h := floatingSize(prefs.FloatingWindow.Style, a.floatingContent(prefs), prefs.FontFamily)
	return int(math.Ceil(float64(w))), int(math.Ceil(float64(h)))
}

func (a *App) openFloating(prefs store.Preferences, w, h int) {
	f := prefs.FloatingWindow
	area := mygo.Screen.PrimaryDisplay().WorkArea
	win := mygo.NewWindow(mygo.WindowOptions{
		StateKey:          "floatingWindow",
		Title:             "Quosparr",
		Width:             w,
		Height:            h,
		UseContentSize:    true,
		X:                 area.X + area.Width - w - floatingMargin,
		Y:                 area.Y + (area.Height-h)/2,
		Hidden:            true,
		Frameless:         true,
		Transparent:       floatingTransparent(),
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		AlwaysOnTop:       true,
		SkipTaskbar:       true,
		Opacity:           float64(f.OpacityPercent()) / 100,
		Content:           ui.View(a.floatingView),
		// Below Windows' default minimum window size, which would widen
		// the narrow styles and leave the card empty on the right. Windows
		// still keeps it as tall as a caption, which frameless windows keep
		// for the system shadow: the capsule draws centered in that.
		MinWidth:  1,
		MinHeight: 1,
	})
	a.floating.win = win
	// The saved state brings back the last size too; the content decides
	// it, from the edge the window keeps when it resizes.
	a.resizeFloating(w, h)
	win.SetVisibleOnAllWorkspaces(true)
	// The opposite of the setting, so that this sets the window up.
	a.floating.overFullScreen = !f.OverFullScreen
	a.setFloatingOverFullScreen(f.OverFullScreen)
	a.syncFloatingFollow(f)
	a.keepFloatingOnScreen()
	win.OnClosed(func() {
		if a.floating.win == win {
			a.stopFloatingWatchers()
			a.floating = floatingState{}
		}
	})
	a.floating.offDisplays = mygo.Screen.OnDisplaysChanged(a.keepFloatingOnScreen)
	if a.floating.followKey != "" {
		// Hidden until the app in front is known.
		a.floating.appHidden, a.floating.shown.hidden = true, true
		return
	}
	win.ShowInactive()
}

// setFloatingOverFullScreen lets the window show above full-screen apps,
// or makes it give way to them.
func (a *App) setFloatingOverFullScreen(over bool) {
	win := a.floating.win
	if win == nil || a.floating.overFullScreen == over {
		return
	}
	a.floating.overFullScreen = over
	setupFloatingWindow(win, over)
	if a.floating.stopWatch != nil {
		a.floating.stopWatch()
		a.floating.stopWatch = nil
	}
	if !over {
		a.floating.stopWatch = watchFullScreen(a.floatingFullScreen)
		return
	}
	a.floating.fullScreenHidden = false
	a.showFloatingIfDue()
}

// floatingFollowKey is what the window follows the app in front by, or
// empty when it does not.
func floatingFollowKey(f store.FloatingWindow) string {
	if f.AppFilter == store.FloatingAppFilterOff || !foregroundSupported() {
		return ""
	}
	var b strings.Builder
	b.WriteString(string(f.AppFilter))
	if floatingFollowLowers(f) {
		b.WriteString(" lower")
	}
	for _, app := range f.Apps {
		b.WriteString("\n" + app.ID + "\x00" + app.Path)
	}
	return b.String()
}

// floatingFollowLowers reports whether an app the window does not show for
// takes it off the top rather than hiding it: the allow list's, unless set
// to hide.
func floatingFollowLowers(f store.FloatingWindow) bool {
	return f.AppFilter == store.FloatingAppFilterAllow && !f.HideOffList
}

// syncFloatingFollow starts, restarts or stops following the app in front
// as the filter or the apps change.
func (a *App) syncFloatingFollow(f store.FloatingWindow) {
	key := floatingFollowKey(f)
	if key == a.floating.followKey {
		return
	}
	if a.floating.stopFollow != nil {
		a.floating.stopFollow()
		a.floating.stopFollow = nil
	}
	a.floating.followKey = key
	if key == "" {
		a.floating.appHidden, a.floating.appLowered = false, false
		a.showFloatingIfDue()
		return
	}
	f.Apps = slices.Clone(f.Apps)
	lowers := floatingFollowLowers(f)
	a.floating.stopFollow = watchForeground(f, func(visible bool) {
		a.floating.appHidden = !visible && !lowers
		a.floating.appLowered = !visible && lowers
		a.showFloatingIfDue()
	})
}

// showFloatingIfDue hides the window while another app is full screen or the
// app in front hides it, and shows it again after; shown, the app in front
// may take it off the top.
func (a *App) showFloatingIfDue() {
	win := a.floating.win
	if win == nil {
		return
	}
	was := a.floating.shown
	due := floatingShown{hidden: a.floating.fullScreenHidden || a.floating.appHidden, lowered: a.floating.appLowered}
	if due == was {
		return
	}
	a.floating.shown = due
	switch {
	case due.hidden:
		if !was.hidden {
			win.Hide()
		}
		return
	case was.hidden:
		win.ShowInactive()
	}
	// Set either way once shown again, which puts the window in front.
	if due.lowered {
		lowerFloating(win)
	} else {
		win.SetAlwaysOnTop(true)
	}
}

// resizeFloating fits the window to new content, keeping its vertical
// center and the edge nearer the side of the display it is on.
func (a *App) resizeFloating(w, h int) {
	win := a.floating.win
	b := win.Bounds()
	area := mygo.Screen.DisplayMatching(b).WorkArea
	a.floating.w, a.floating.h = w, h
	win.SetContentSize(w, h)
	nb := win.Bounds()
	x, y := floatingAnchor(b, nb.Width, nb.Height, area)
	nb.X, nb.Y = clampFloating(mygo.Rectangle{X: x, Y: y, Width: nb.Width, Height: nb.Height}, area)
	win.SetPosition(nb.X, nb.Y)
}

// floatingAnchor is where a window at b goes when it becomes w by h: on
// the right half of area it keeps its right edge, else its left edge, and
// it keeps its vertical center.
func floatingAnchor(b mygo.Rectangle, w, h int, area mygo.Rectangle) (int, int) {
	x := b.X
	if b.X+b.Width/2 > area.X+area.Width/2 {
		x = b.X + b.Width - w
	}
	return x, b.Y + (b.Height-h)/2
}

func (a *App) stopFloatingWatchers() {
	if a.floating.stopWatch != nil {
		a.floating.stopWatch()
	}
	if a.floating.stopFollow != nil {
		a.floating.stopFollow()
	}
	if a.floating.offDisplays != nil {
		a.floating.offDisplays()
	}
}

func (a *App) closeFloating() {
	win := a.floating.win
	if win == nil {
		return
	}
	a.stopFloatingWatchers()
	a.floating = floatingState{}
	win.Close()
}

// floatingFullScreen hides the window while another app is full screen
// (Windows; macOS keeps full-screen apps in Spaces of their own).
func (a *App) floatingFullScreen(busy bool) {
	if a.floating.win == nil {
		return
	}
	a.floating.fullScreenHidden = busy
	a.showFloatingIfDue()
}

// keepFloatingOnScreen moves the window back into the work area of the
// display it is mostly on, after a drag or a display change.
func (a *App) keepFloatingOnScreen() {
	win := a.floating.win
	if win == nil {
		return
	}
	b := win.Bounds()
	x, y := clampFloating(b, mygo.Screen.DisplayMatching(b).WorkArea)
	if x != b.X || y != b.Y {
		win.SetPosition(x, y)
	}
}

// clampFloating is the position that keeps b inside area, by the margin
// where it fits.
func clampFloating(b, area mygo.Rectangle) (int, int) {
	if area.Width <= 0 || area.Height <= 0 {
		return b.X, b.Y
	}
	clamp := func(v, lo, size, span int) int {
		hi := lo + span - size
		if hi < lo {
			return lo
		}
		return max(lo, min(hi, v))
	}
	return clamp(b.X, area.X, b.Width, area.Width), clamp(b.Y, area.Y, b.Height, area.Height)
}

// floatingView draws the window and handles the pointer: a click opens the
// panel, a drag moves the window, the secondary button opens a menu.
func (a *App) floatingView(c *ui.Context) {
	prefs := a.settings.Prefs()
	t := applyThemeColor(c, prefs.ThemeColor, prefs.FontFamily)
	c.Root().Background(ui.Transparent)
	v := a.floatingContent(prefs)
	style, family := prefs.FloatingWindow.Style, prefs.FontFamily
	ink := floatingInkFor(t)
	root := ui.Box(c).Fill().Role(ui.RoleButton).Label(floatingTooltip(v)).Tooltip(floatingTooltip(v)).
		Draw(func(p *ui.Painter, r ui.Rect) {
			drawFloating(p, r, style, v, ink, family, floatingCornerRadius(style, r.H))
		})
	root.ContextMenu(func(m *ui.Menu) { a.floatingMenu(m, prefs) })
	// Dragged holds until the button goes up, unlike Pressed, which ends
	// as soon as the pointer gets ahead of the moving window.
	_, _, held := root.Dragged()
	clicked := root.Clicked()
	a.trackFloatingDrag(held, prefs.FloatingWindow.Locked)
	if clicked && !a.floating.drag.moved {
		a.floatingClicked()
	}
	if !held {
		a.floating.drag = floatingDrag{}
	}
}

// trackFloatingDrag follows a press: past a few DIPs it is a drag, which
// moves the window unless its position is locked.
func (a *App) trackFloatingDrag(held, locked bool) {
	d, win := &a.floating.drag, a.floating.win
	if win == nil {
		return
	}
	if !held {
		if d.moved && !locked {
			a.keepFloatingOnScreen()
		}
		return
	}
	cursor := mygo.Screen.CursorScreenPoint()
	if !d.active {
		x, y := win.Position()
		*d = floatingDrag{active: true, cursor: cursor, x: x, y: y}
		return
	}
	dx, dy := cursor.X-d.cursor.X, cursor.Y-d.cursor.Y
	if !d.moved && dx*dx+dy*dy < 9 {
		return
	}
	d.moved = true
	if !locked {
		win.SetPosition(d.x+dx, d.y+dy)
	}
}

// floatingClicked opens the panel, or closes it. The panel closes itself as
// soon as the press takes the focus away, so a click right after that is
// the one that closed it.
func (a *App) floatingClicked() {
	if a.panel != nil {
		a.ClosePanel()
		return
	}
	if time.Since(a.panelClosedAt) < 400*time.Millisecond {
		return
	}
	a.OpenPanel()
}

func (a *App) floatingMenu(m *ui.Menu, prefs store.Preferences) {
	l := a.L()
	if m.Item(l.Text(l10n.KeyOpenControlPanel)).Chosen() {
		a.OpenPanel()
	}
	if m.Item(l.Text(l10n.KeySettings)).Chosen() {
		a.OpenSettingsTab(int(tabFloating))
	}
	m.Separator()
	m.Submenu(tx(l, xFloatingStyle), func(sub *ui.Menu) {
		for _, st := range store.FloatingStyles() {
			if sub.Item(floatingStyleName(l, st)).Checked(prefs.FloatingWindow.Style == st).Chosen() {
				a.settings.SetFloatingWindow(func(f *store.FloatingWindow) { f.Style = st })
			}
		}
	})
	if m.Item(tx(l, xFloatingLocked)).Checked(prefs.FloatingWindow.Locked).Chosen() {
		a.settings.SetFloatingWindow(func(f *store.FloatingWindow) { f.Locked = !prefs.FloatingWindow.Locked })
	}
	m.Separator()
	if m.Item(tx(l, xFloatingHide)).Chosen() {
		a.settings.SetFloatingWindow(func(f *store.FloatingWindow) { f.Enabled = false })
	}
}

func floatingStyleName(l l10n.Localizer, st store.FloatingStyle) string {
	switch st {
	case store.FloatingStyleCard:
		return tx(l, xFloatingStyleCard)
	case store.FloatingStyleRings:
		return tx(l, xFloatingStyleRings)
	case store.FloatingStyleBattery:
		return tx(l, xFloatingStyleBattery)
	}
	return tx(l, xFloatingStyleCapsule)
}
