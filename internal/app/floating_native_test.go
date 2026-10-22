package app

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/store"
)

type nativeFloatingStates map[string]mygo.WindowState

func TestNativeFloatingWindowSizes(t *testing.T) {
	if os.Getenv("RUNWAY_NATIVE_FLOATING_HOST") != "1" {
		t.Skip("set RUNWAY_NATIVE_FLOATING_HOST=1 to exercise native window sizes")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native floating size host supports Windows and macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	home := t.TempDir()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"RUNWAY_NATIVE_FLOATING_CHILD=1",
		"RUNWAY_NATIVE_FLOATING_HOME="+home,
		"HOME="+home,
		"USERPROFILE="+home,
		"APPDATA="+filepath.Join(home, "appdata"),
		"LOCALAPPDATA="+filepath.Join(home, "localappdata"),
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_DATA_HOME="+filepath.Join(home, "data"),
		"CODEX_HOME="+filepath.Join(home, "codex"),
		"GROK_HOME="+filepath.Join(home, "grok"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(home, "claude"),
		"QUOSPARR_PREFS_DOMAIN=com.quosparr.app.native-floating-test",
	)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("native floating size child timed out: %v\n%s", ctx.Err(), out)
	}
	if err != nil {
		t.Fatalf("native floating size child failed: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func (s *nativeFloatingStates) LoadWindowStates() (map[string]mygo.WindowState, error) {
	return maps.Clone(*s), nil
}

func (s *nativeFloatingStates) SaveWindowStates(states map[string]mygo.WindowState) error {
	*s = maps.Clone(states)
	return nil
}

func runNativeFloatingHost() error {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return fmt.Errorf("native floating size check requires Windows or macOS")
	}
	home := os.Getenv("RUNWAY_NATIVE_FLOATING_HOME")
	if home == "" {
		return fmt.Errorf("native floating test home is missing")
	}
	mygo.App.SetName("Quosparr Floating Size Test")
	mygo.App.SetVersion("synthetic")
	mygo.App.SetPath(mygo.PathAppData, home)
	if err := DirectMygoPaths(mygo.App, home); err != nil {
		return err
	}
	states := nativeFloatingStates{}
	mygo.App.SetWindowStateStore(&states)
	s := NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, UnavailableCredentials{})
	if err := s.Load(); err != nil {
		return err
	}
	s.prefs.FloatingWindow = store.FloatingWindow{Enabled: true, Style: store.FloatingStyleRings, Providers: store.Providers()[:1], OverFullScreen: true}
	s.persisted = s.prefs.Clone()
	a := New(Options{Version: "synthetic", Settings: s})
	a.snap.Floating = floatingSample(a.L())
	a.snap.Floating.Providers = append(a.snap.Floating.Providers, a.snap.Floating.Providers[0])
	a.snap.Floating.Providers[2].Provider = store.ProviderGrok
	a.snap.Floating.Providers[2].Name = "Grok"
	for i := range a.snap.Floating.Providers {
		a.snap.Floating.Providers[i].Meters = a.snap.Floating.Providers[i].Meters[:1]
	}
	a.snap.Floating.Providers = a.snap.Floating.Providers[:1]
	host := &nativeFloatingHost{app: a, states: &states, err: fmt.Errorf("native floating size checks did not complete")}
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(host.run)
	if err := mygo.App.Run(); err != nil {
		return err
	}
	return host.err
}

type nativeFloatingHost struct {
	app    *App
	states *nativeFloatingStates
	err    error
}

// run keeps creation, resizing and restoration on one event-loop callback so
// every assertion observes the same native window and its saved state.
func (h *nativeFloatingHost) run() {
	quit := true
	defer func() {
		if quit {
			mygo.App.Quit()
		}
	}()
	a := h.app
	// A saved state wider than the content, on the right half of the
	// display, as Windows' minimum window size left it before. The states
	// load as the first window opens, once the displays are known.
	area := mygo.Screen.PrimaryDisplay().WorkArea
	saved := mygo.WindowState{X: area.X + area.Width - 480 - floatingMargin, Y: area.Y + 40, Width: 480, Height: 240}
	(*h.states)["floatingWindow"] = saved
	w, height := a.floatingWindowSize(a.settings.Prefs())
	if w >= saved.Width {
		h.err = fmt.Errorf("restored-size fixture width %d is not wider than requested width %d", saved.Width, w)
		return
	}
	a.openFloating(a.settings.Prefs(), w, height)
	if err := checkNativeFloatingSize(a, "restored rings with one provider and one meter", w, height); err != nil {
		h.err = err
		return
	}
	if err := checkNativeFloatingAnchor(a, saved); err != nil {
		h.err = err
		return
	}
	minH := nativeFloatingMinHeight(a)
	if minH >= 48 {
		h.err = fmt.Errorf("minimum content height = %d, want below 48, the shortest style but the capsule", minH)
		return
	}
	for _, style := range store.FloatingStyles() {
		a.settings.prefs.FloatingWindow.Style = style
		a.snap.Floating.Providers = a.snap.Floating.Providers[:3]
		w, height = a.floatingWindowSize(a.settings.Prefs())
		a.resizeFloating(w, height)
		if err := checkNativeFloatingSize(a, fmt.Sprintf("style %q with three providers", style), w, max(height, minH)); err != nil {
			h.err = err
			return
		}
		a.snap.Floating.Providers = a.snap.Floating.Providers[:1]
		w, height = a.floatingWindowSize(a.settings.Prefs())
		a.resizeFloating(w, height)
		if err := checkNativeFloatingSize(a, fmt.Sprintf("style %q with one provider", style), w, max(height, minH)); err != nil {
			h.err = err
			return
		}
	}
	a.settings.prefs.FloatingWindow.Style = store.FloatingStyleRings
	a.snap.Floating.Providers = a.snap.Floating.Providers[:1]
	w, height = a.floatingWindowSize(a.settings.Prefs())
	a.resizeFloating(w, height)
	a.closeFloating()
	w, height = a.floatingWindowSize(a.settings.Prefs())
	a.openFloating(a.settings.Prefs(), w, height)
	if err := checkNativeFloatingSize(a, "reopened with saved single-provider size", w, max(height, minH)); err != nil {
		h.err = err
		return
	}
	// The window server lists the window on screen once the event loop ran.
	quit = false
	go func() {
		err := checkNativeFloatingLowered(a)
		mygo.RunOnMain(func() {
			a.closeFloating()
			h.err = err
			mygo.App.Quit()
		})
	}()
}

// checkNativeFloatingLowered: an app off the allow list takes the window off
// the top but leaves it on screen, below the window in front, also once
// shown again after hiding, and a listed app puts it back on top. It runs
// off the main thread, the event loop running between the steps.
func checkNativeFloatingLowered(a *App) error {
	var err error
	step := func(fn func() error) {
		if err == nil {
			time.Sleep(200 * time.Millisecond)
			waitOnMain(func() { err = fn() })
		}
	}
	var win *mygo.Window
	waitOnMain(func() { win = a.floating.win })
	check := func(name string, onTop bool) error {
		if win.IsAlwaysOnTop() != onTop || !win.IsVisible() {
			return fmt.Errorf("%s: on top %v, visible %v; want on top %v, visible", name, win.IsAlwaysOnTop(), win.IsVisible(), onTop)
		}
		if onTop {
			return nil
		}
		below, known := nativeFloatingBelowFront(win)
		if known && !below {
			return fmt.Errorf("%s: the window stays above the window in front", name)
		}
		fmt.Printf("native floating %s: below the window in front = %v (known %v)\n", name, below, known)
		return nil
	}
	set := func(lowered, hidden bool) func() error {
		return func() error {
			a.floating.appLowered, a.floating.fullScreenHidden = lowered, hidden
			a.showFloatingIfDue()
			return nil
		}
	}
	step(set(true, false))
	step(func() error { return check("lowered", false) })
	step(set(true, true))
	step(func() error {
		if win.IsVisible() {
			return fmt.Errorf("lowered window still shows while hidden")
		}
		return nil
	})
	step(set(true, false))
	step(func() error { return check("shown again lowered", false) })
	step(set(false, false))
	step(func() error { return check("raised", true) })
	return err
}

func checkNativeFloatingSize(a *App, name string, wantW, wantH int) error {
	gotW, gotH := a.floating.win.ContentSize()
	if absInt(gotW-wantW) > 1 || absInt(gotH-wantH) > 1 {
		return fmt.Errorf("%s content size = %dx%d, want %dx%d (1 DIP tolerance)", name, gotW, gotH, wantW, wantH)
	}
	fmt.Printf("native floating size: %s = %dx%d, want %dx%d\n", name, gotW, gotH, wantW, wantH)
	return nil
}

// nativeFloatingMinHeight is the least content height the window takes.
// Windows keeps a window with a caption, which frameless windows keep for
// the system shadow, as tall as the caption: taller than the capsule, which
// draws centered in it. Widths have no such floor.
func nativeFloatingMinHeight(a *App) int {
	win := a.floating.win
	w, h := win.ContentSize()
	win.SetContentSize(w, 1)
	_, minH := win.ContentSize()
	win.SetContentSize(w, h)
	fmt.Printf("native floating minimum content height = %d\n", minH)
	return minH
}

// checkNativeFloatingAnchor: a window restored on the right half keeps the
// right edge and the vertical center of its saved bounds.
func checkNativeFloatingAnchor(a *App, saved mygo.WindowState) error {
	b := a.floating.win.Bounds()
	right, cy := saved.X+saved.Width, saved.Y+saved.Height/2
	if absInt(b.X+b.Width-right) > 1 || absInt(b.Y+b.Height/2-cy) > 1 {
		return fmt.Errorf("restored bounds = %+v, want right edge %d and vertical center %d (1 DIP tolerance)", b, right, cy)
	}
	fmt.Printf("native floating anchor: restored bounds = %+v, right edge %d, vertical center %d\n", b, right, cy)
	return nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
