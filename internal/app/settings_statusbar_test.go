package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

func TestStatusBarTabShowsEveryStyleAndItsOptions(t *testing.T) {
	u := cardSettingsUI(t, store.LanguageEnglish)
	l := u.L()
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight*2)
	if err := tt.Click(tabStatusBar.title(l)); err != nil {
		t.Fatal(err)
	}
	if u.tab != tabStatusBar || !tt.HasText(tx(l, xStatusBarPreview)) {
		t.Fatal("menu bar item page not shown")
	}
	for _, st := range store.StatusBarStyles() {
		if _, ok := tt.Find(statusBarStyleName(l, st)); !ok {
			t.Fatalf("style %q missing", st)
		}
	}
	if tt.HasText(tx(l, xBatteryOrientation)) {
		t.Fatal("battery options shown for another style")
	}
	if err := tt.Click(statusBarStyleName(l, store.StatusBarStyleBattery)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.s.Prefs().StatusBarDisplayStyle != store.StatusBarStyleBattery || !tt.HasText(tx(l, xBatteryOrientation)) {
		t.Fatal("battery style or its options missing")
	}
	if err := tt.Click(tx(l, xBatteryVertical)); err != nil {
		t.Fatal(err)
	}
	if got := u.s.Prefs().StatusBar.Battery; got != store.BatteryVertical {
		t.Fatalf("battery = %q", got)
	}
	if tt.HasText(tx(l, xRingValues)) {
		t.Fatal("ring options shown for another style")
	}
	if err := tt.Click(statusBarStyleName(l, store.StatusBarStyleRings)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// The switch sits at the end of its row, beside the title and the hint.
	title, ok := tt.Find(tx(l, xRingValues))
	hint, hintOK := tt.Find(tx(l, xRingValuesHint))
	if !ok || !hintOK {
		t.Fatal("ring options missing")
	}
	tt.ClickAt(float32(tt.Image().Bounds().Dx()-48), (title.Y+hint.Y+hint.H)/2)
	if !u.s.Prefs().StatusBar.RingValues {
		t.Fatal("values inside rings not saved")
	}
	// The appearance page no longer carries the status item.
	u.tab = tabDisplay
	tt.Frame()
	if tt.HasText(l.Text(l10n.KeyStatusBarStyle)) {
		t.Fatal("appearance page still shows the status bar style")
	}
}

func TestStatusBarPreviewFollowsTheSettings(t *testing.T) {
	l := l10n.New(l10n.English)
	p := store.DefaultPreferences()
	p.StatusBarDisplayStyle = store.StatusBarStyleRings
	p.SetStatusBarGroups([][]store.Provider{{store.ProviderCodex, store.ProviderClaude}, {store.ProviderGrok}})
	r := statusBarSample(l, p)
	if r.Style != store.StatusBarStyleRings || len(r.Components) != 3 {
		t.Fatalf("sample = %+v", r)
	}
	p.StatusBarDisplayStyle = store.StatusBarStyleMeters
	if r = statusBarSample(l, p); len(r.Components) != 2 || len(r.Components[0].Cells) != 2 {
		t.Fatalf("sample = %+v", r)
	}
}

// TestExportStatusBarPageReview writes the page in each style, both themes,
// to check the previews by eye; it never opens a window or saves.
func TestExportStatusBarPageReview(t *testing.T) {
	dir := os.Getenv("RUNWAY_STATUSBAR_QA_DIR")
	if dir == "" {
		t.Skip("set RUNWAY_STATUSBAR_QA_DIR to export offscreen PNGs")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		u := cardSettingsUI(t, language)
		u.tab = tabStatusBar
		u.s.SetStatusBarGroups([][]store.Provider{{store.ProviderCodex, store.ProviderClaude}, {store.ProviderGrok}})
		for _, st := range store.StatusBarStyles() {
			u.s.SetStatusBarStyle(st)
			for _, dark := range []bool{false, true} {
				view := ui.NewTester(u.view, settingsWidth, settingsHeight*2)
				view.SetDark(dark)
				name := fmt.Sprintf("%s-%s-%v.png", language, st, dark)
				exportReviewPNG(t, view, filepath.Join(dir, name))
			}
		}
	}
}
