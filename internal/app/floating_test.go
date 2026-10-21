package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

func TestClampFloatingKeepsTheWindowInTheWorkArea(t *testing.T) {
	area := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	cases := []struct {
		name   string
		b      mygo.Rectangle
		wx, wy int
	}{
		{"inside", mygo.Rectangle{X: 100, Y: 100, Width: 200, Height: 40}, 100, 100},
		{"off the right", mygo.Rectangle{X: 1400, Y: 100, Width: 200, Height: 40}, 1240, 100},
		{"above the menu bar", mygo.Rectangle{X: 100, Y: 0, Width: 200, Height: 40}, 100, 25},
		{"off the bottom left", mygo.Rectangle{X: -50, Y: 990, Width: 200, Height: 40}, 0, 860},
		{"larger than the area", mygo.Rectangle{X: 300, Y: 300, Width: 2000, Height: 1000}, 0, 25},
	}
	for _, tc := range cases {
		x, y := clampFloating(tc.b, area)
		if x != tc.wx || y != tc.wy {
			t.Errorf("%s: (%d, %d), want (%d, %d)", tc.name, x, y, tc.wx, tc.wy)
		}
	}
	if x, y := clampFloating(cases[1].b, mygo.Rectangle{}); x != 1400 || y != 100 {
		t.Errorf("no work area moved the window to (%d, %d)", x, y)
	}
}

func TestFloatingAnchorKeepsTheNearEdgeAndTheCenter(t *testing.T) {
	area := mygo.Rectangle{X: 0, Y: 25, Width: 1440, Height: 875}
	right := mygo.Rectangle{X: 1200, Y: 400, Width: 200, Height: 40}
	if x, y := floatingAnchor(right, 240, 140, area); x != 1160 || y != 350 {
		t.Errorf("right half: (%d, %d), want (1160, 350)", x, y)
	}
	left := mygo.Rectangle{X: 40, Y: 400, Width: 200, Height: 40}
	if x, y := floatingAnchor(left, 240, 140, area); x != 40 || y != 350 {
		t.Errorf("left half: (%d, %d), want (40, 350)", x, y)
	}
}

func TestFloatingStylesHaveTheirOwnSizes(t *testing.T) {
	l := l10n.New(l10n.English)
	v := floatingSample(l)
	one := v
	one.Providers = v.Providers[:1]
	seen := map[[2]float32]store.FloatingStyle{}
	for _, st := range store.FloatingStyles() {
		w, h := floatingSize(st, v, "")
		w1, h1 := floatingSize(st, one, "")
		if w <= 0 || h <= 0 || w > 600 || h > 300 {
			t.Errorf("%q: size %vx%v", st, w, h)
		}
		if w1 > w || h1 > h || (w1 == w && h1 == h) {
			t.Errorf("%q: one provider %vx%v is not smaller than two %vx%v", st, w1, h1, w, h)
		}
		if prev, ok := seen[[2]float32{w, h}]; ok {
			t.Errorf("%q has the same size as %q", st, prev)
		}
		seen[[2]float32{w, h}] = st
	}
}

func TestFloatingTooltipNamesEveryMeter(t *testing.T) {
	l := l10n.New(l10n.English)
	tip := floatingTooltip(floatingSample(l))
	for _, want := range []string{"Codex", "Claude", "73%", "41%", "18%", "64%"} {
		if !strings.Contains(tip, want) {
			t.Errorf("tooltip %q misses %q", tip, want)
		}
	}
}

func TestFloatingSettingsIsATabAfterAppearance(t *testing.T) {
	if i := slices.Index(settingsTabs, tabFloating); i < 2 || settingsTabs[i-2] != tabDisplay || settingsTabs[i-1] != tabStatusBar {
		t.Fatalf("floating window tab at %d, want after appearance and the menu bar item", i)
	}
	u := cardSettingsUI(t, store.LanguageEnglish)
	l := u.L()
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight*2)
	if err := tt.Click(tabFloating.title(l)); err != nil {
		t.Fatal(err)
	}
	if u.tab != tabFloating || !tt.HasText(tx(l, xFloatingEnabledHint)) {
		t.Fatal("floating window page not shown")
	}
	// The styles can be chosen before the window is turned on.
	for _, st := range store.FloatingStyles() {
		if !tt.HasText(floatingStyleName(l, st)) {
			t.Fatalf("style %q missing", st)
		}
	}
	if err := tt.Click(tx(l, xFloatingStyleRings)); err != nil {
		t.Fatal(err)
	}
	if got := u.s.Prefs().FloatingWindow.Style; got != store.FloatingStyleRings {
		t.Fatalf("style = %q, want rings", got)
	}
	u.tab = tabDisplay
	tt.Frame()
	if tt.HasText(tx(l, xFloatingEnabledHint)) {
		t.Error("the appearance page still shows the floating window")
	}
}

// The previews never open a native window or save preferences.
func TestExportFloatingStylesReview(t *testing.T) {
	if os.Getenv("RUNWAY_FLOATING_QA") == "" {
		t.Skip("set RUNWAY_FLOATING_QA=1 to export offscreen PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-floating-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		u := cardSettingsUI(t, language)
		v := floatingSample(u.L())
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			for _, st := range store.FloatingStyles() {
				w, h := floatingSize(st, v, "")
				tt := ui.NewTester(func(c *ui.Context) {
					ink := floatingInkFor(c.Theme())
					ui.Box(c).Fill().Padding(20).Children(func() {
						ui.Box(c).Width(w).Height(h).Draw(func(p *ui.Painter, r ui.Rect) {
							drawFloating(p, r, st, v, ink, "", floatingStyleRadius(st, r.H))
						})
					})
				}, int(w)+40, int(h)+40)
				tt.SetScale(2)
				tt.SetDark(dark)
				name := string(st)
				if name == "" {
					name = "capsule"
				}
				exportReviewPNG(t, tt, filepath.Join(dir, string(language)+"-"+name+"-"+appearance+".png"))
			}
		}
		u.tab = tabFloating
		u.s.SetFloatingWindow(func(f *store.FloatingWindow) { f.Enabled = true; f.Style = store.FloatingStyleCard })
		tt := ui.NewTester(u.view, settingsWidth, settingsHeight*3)
		exportReviewPNG(t, tt, filepath.Join(dir, string(language)+"-settings.png"))
	}
}
