package app

import (
	"math"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
)

func TestPanelResizeDragPersistsOnRelease(t *testing.T) {
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	tt.SetSize(panelWidth, int(store.DefaultMainPanelHeight))
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	changes := 0
	a.settings.OnChange(func() { changes++ })
	r, ok := tt.Find("panel-resize-handle")
	if !ok {
		t.Fatal("main panel has no bottom resize handle")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	tt.Move(x, y+80)
	tt.Move(x, y+120)
	if changes != 0 || a.settings.Prefs().MainPanelHeight != store.DefaultMainPanelHeight {
		t.Fatal("drag persisted before mouse-up")
	}
	tt.Release(x, y+120)
	if changes != 1 || a.settings.Prefs().MainPanelHeight != store.DefaultMainPanelHeight+120 {
		t.Fatalf("release changes=%d height=%v", changes, a.settings.Prefs().MainPanelHeight)
	}
	tt.Frame()
	tt.Frame()
	if changes != 1 {
		t.Fatal("release persisted more than once")
	}
	a.settings.Flush()
}

func TestPanelResizeClickDoesNotPersist(t *testing.T) {
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	tt.SetSize(panelWidth, int(store.DefaultMainPanelHeight))
	changes := 0
	a.settings.OnChange(func() { changes++ })
	if err := tt.Click("panel-resize-handle"); err != nil {
		t.Fatal(err)
	}
	if changes != 0 {
		t.Fatal("click without movement persisted height")
	}
}

func TestPanelResizeDragClampsHeight(t *testing.T) {
	for _, target := range []struct {
		delta        float32
		screenHeight int
		top          bool
		height       float64
	}{{1000, 2160, false, 1584}, {3000, 2160, false, 1620}, {-3000, 2160, true, 1620}, {1000, 1080, false, 810}, {-1000, 1080, false, store.MinMainPanelHeight}} {
		a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
		tt.SetSize(panelWidth, int(store.DefaultMainPanelHeight))
		a.panelResize.top = target.top
		tt.Frame()
		r, ok := tt.Find("panel-resize-handle")
		if !ok {
			t.Fatal("resize handle missing")
		}
		x, y := r.X+r.W/2, r.Y+r.H/2
		tt.Press(x, y)
		// The native drag reads this from the panel's display on mouse-down.
		a.panelResize.screenHeight = target.screenHeight
		a.panelResize.available = target.screenHeight - 40
		tt.Move(x, y+target.delta)
		tt.Release(x, y+target.delta)
		if got := a.settings.Prefs().MainPanelHeight; got != target.height {
			t.Fatalf("height %v, want %v", got, target.height)
		}
		a.settings.Flush()
	}
}

func TestPanelHeightClampsToUsableScreen(t *testing.T) {
	for _, tc := range []struct {
		proposed                float64
		screenHeight, available int
		want                    float64
	}{
		{900, 1080, 1040, 810}, {2000, 1440, 1400, 1080}, {3000, 2160, 2120, 1620},
		{900, 1080, 700, 684}, {100, 1080, 700, 360}, {900, 400, 300, 360},
		{900, 2160, 2000, 900}, {2000, 1081, 1040, 810},
		{math.NaN(), 1080, 500, 484}, {math.Inf(1), 0, 0, store.DefaultMainPanelHeight}, {584.6, 0, 0, 585},
	} {
		if got := clampPanelHeight(tc.proposed, tc.screenHeight, tc.available); got != tc.want {
			t.Fatalf("clamp(%v,%d,%d)=%v, want %v", tc.proposed, tc.screenHeight, tc.available, got, tc.want)
		}
	}
}

func TestPanelOpeningSizeFitsScreenWithoutChangingPreference(t *testing.T) {
	a, _ := newPanel(t, func(p *store.Preferences) { p.MainPanelHeight = 1500 }, panelFixture(), PanelActions{})
	for _, tc := range []struct{ screen, available, want int }{{1080, 1040, 810}, {1440, 1400, 1080}, {2160, 2120, 1500}, {1080, 700, 684}, {400, 300, 360}, {0, 0, 1500}} {
		width, height := a.panelSizeWithin(tc.screen, tc.available)
		if width != panelWidth || height != tc.want {
			t.Fatalf("screen %d: panel %dx%d, want %dx%d", tc.screen, width, height, panelWidth, tc.want)
		}
	}
	if a.settings.Prefs().MainPanelHeight != 1500 {
		t.Fatal("fitting the panel overwrote the preferred height")
	}
}

func TestSettingsPanelHeightUsesDisplayLimit(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	display := mygo.Display{Bounds: mygo.Rectangle{Height: 2160}, WorkArea: mygo.Rectangle{Height: 2120}}
	u := newSettingsUI(e.s, SettingsOptions{PanelDisplay: func() mygo.Display { return display }})
	tt := ui.NewTester(u.appearanceGroup, settingsWidth, settingsHeight)
	focusSettingsAction(t, tt, tx(u.L(), xPanelHeight))
	tt.Key(0, ui.KeyEnd)
	if got := e.s.Prefs().MainPanelHeight; got != 1620 {
		t.Fatalf("large screen height = %v, want 1620", got)
	}
	display.Bounds.Height, display.WorkArea.Height = 1080, 1040
	tt.Frame()
	if got := e.s.Prefs().MainPanelHeight; got != 1620 {
		t.Fatalf("display change overwrote preferred height: %v", got)
	}
	tt.Key(0, ui.KeyEnd)
	if got := e.s.Prefs().MainPanelHeight; got != 810 {
		t.Fatalf("small screen height = %v, want 810", got)
	}
	e.settle()
}

func TestPanelResizeCoalescedReleaseIncludesFinalMovement(t *testing.T) {
	testPanelResizeFinalMovement(t, false)
}

func TestPanelResizeReleaseIncludesMovementAfterLastFrame(t *testing.T) {
	testPanelResizeFinalMovement(t, true)
}

func testPanelResizeFinalMovement(t *testing.T, moveBeforeRelease bool) {
	t.Helper()
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	tt.SetSize(panelWidth, int(store.DefaultMainPanelHeight))
	changes := 0
	a.settings.OnChange(func() { changes++ })
	r, ok := tt.Find("panel-resize-handle")
	if !ok {
		t.Fatal("resize handle missing")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	if moveBeforeRelease {
		tt.Move(x, y-40)
	}
	tt.Release(x, y-80) // A real native frame can see release before any held move frame.
	if changes != 1 || a.settings.Prefs().MainPanelHeight != store.DefaultMainPanelHeight-80 {
		t.Fatalf("coalesced release changes=%d height=%v, want one save at %v", changes, a.settings.Prefs().MainPanelHeight, store.DefaultMainPanelHeight-80)
	}
	a.settings.Flush()
}

func TestPanelResizeTopEdgeGrowsUpward(t *testing.T) {
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	tt.SetSize(panelWidth, int(store.DefaultMainPanelHeight))
	a.panelResize.top = true
	tt.Frame()
	r, ok := tt.Find("panel-resize-handle")
	if !ok {
		t.Fatal("main panel has no top resize handle")
	}
	if r.Y != 0 {
		t.Fatalf("top handle at y=%v, want 0", r.Y)
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	tt.Move(x, y-60)
	tt.Release(x, y-100)
	if got := a.settings.Prefs().MainPanelHeight; got != store.DefaultMainPanelHeight+100 {
		t.Fatalf("height %v, want %v", got, store.DefaultMainPanelHeight+100)
	}
	if !a.panelResize.top {
		t.Fatal("drag dropped the top edge")
	}
	a.settings.Flush()
}
