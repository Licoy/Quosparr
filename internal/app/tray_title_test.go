package app

import (
	"bytes"
	"image/png"
	"strconv"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

var trayTestNow = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

// trayTestInputs has two windows for every provider.
func trayTestInputs(prefs store.Preferences) runway.StatusInputs {
	now := trayTestNow
	meter := func(title string, minutes, left int, health runway.Health, reset time.Duration) runway.Meter {
		return runway.Meter{Title: title, WindowMinutes: minutes, RemainingPercent: left, ResetsAt: now.Add(reset), Health: health}
	}
	return runway.StatusInputs{Prefs: prefs, Provider: store.ProviderCodex, Now: now,
		CodexMeters: []runway.Meter{meter("5h", 300, 80, runway.HealthGreen, 3*time.Hour), meter("Weekly", 10080, 35, runway.HealthYellow, 48*time.Hour)},
		Claude: runway.ClaudeState{Availability: runway.ClaudeReady, Meters: []runway.Meter{
			meter("5h", 300, 18, runway.HealthRed, 72*time.Minute), meter("Weekly", 10080, 64, runway.HealthGreen, 100*time.Hour)}},
		Grok: runway.GrokState{Availability: runway.GrokReady, Meters: []runway.Meter{
			meter("Weekly", 10080, 85, runway.HealthGreen, 30*time.Hour), meter("Monthly", 43200, 52, runway.HealthGreen, 400*time.Hour)}},
	}
}

func trayTestStatus(style store.StatusBarStyle, customize func(*store.Preferences)) runway.StatusBarView {
	prefs := store.DefaultPreferences()
	prefs.StatusBarDisplayStyle = style
	if customize != nil {
		customize(&prefs)
	}
	return runway.BuildStatusBar(l10n.New(l10n.English), trayTestInputs(prefs))
}

func trayGroups(groups ...[]store.Provider) func(*store.Preferences) {
	return func(p *store.Preferences) { p.SetStatusBarGroups(groups) }
}

func TestTrayStatusRendersFiveDifferentStyles(t *testing.T) {
	seen := map[string]store.StatusBarStyle{}
	for _, style := range store.StatusBarStyles() {
		r := TrayFromStatus(trayTestStatus(style, nil))
		// The battery keeps the 5-hour window it had before the layout.
		cells := 2
		if style == store.StatusBarStyleBattery {
			cells = 1
		}
		if r.Style != style || len(r.Components) != 1 || len(r.Components[0].Cells) != cells {
			t.Fatalf("style/layout lost: %+v", r)
		}
		data, err := TrayImage(r, true, false)
		if err != nil {
			t.Fatal(err)
		}
		if prior, ok := seen[string(data)]; ok {
			t.Fatalf("%s and %s rendered identically", prior, style)
		}
		seen[string(data)] = style
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dy() != 44 {
			t.Fatalf("unexpected tray image bounds %v", img.Bounds())
		}
		if _, _, _, alpha := img.At(0, 0).RGBA(); alpha != 0 {
			t.Fatalf("style %s image has an opaque background", style)
		}
	}
}

func TestDesktopTrayKeepsApplicationLogoForEveryStatus(t *testing.T) {
	states := []TrayRender{{}}
	for _, style := range store.StatusBarStyles() {
		states = append(states, TrayRender{Style: style}, TrayFromStatus(trayTestStatus(style, nil)))
	}
	for _, state := range states {
		for _, light := range []bool{false, true} {
			data, err := TrayImage(state, light, true)
			if err != nil || !bytes.Equal(data, desktopTrayIconPNG) {
				t.Fatalf("desktop tray replaced logo for style %q, light=%v: %v", state.Style, light, err)
			}
		}
	}
}

func TestTrayVariantsRenderDifferently(t *testing.T) {
	render := func(style store.StatusBarStyle, customize func(*store.Preferences)) string {
		t.Helper()
		data, err := TrayImage(TrayFromStatus(trayTestStatus(style, customize)), true, false)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	variants := map[string]string{}
	add := func(name, image string) {
		t.Helper()
		for other, prior := range variants {
			if prior == image {
				t.Fatalf("%s renders like %s", name, other)
			}
		}
		variants[name] = image
	}
	for _, detail := range []store.MetersDetail{store.MetersDetailRemainingPercent, store.MetersDetailResetTime, store.MetersDetailBoth} {
		add("meters "+string(detail), render(store.StatusBarStyleMeters, func(p *store.Preferences) { p.StatusBarMetersDetailStyle = detail }))
	}
	for _, detail := range []store.BatteryDetail{store.BatteryDetailRemainingPercent, store.BatteryDetailCountdown} {
		add("battery "+string(detail), render(store.StatusBarStyleBattery, func(p *store.Preferences) { p.StatusBarBatteryDetailStyle = detail }))
	}
	add("battery vertical", render(store.StatusBarStyleBattery, func(p *store.Preferences) { p.StatusBar.Battery = store.BatteryVertical }))
	add("meters weekly", render(store.StatusBarStyleMeters, func(p *store.Preferences) { p.StatusBar.Window = store.StatusBarWindowWeekly }))
	add("meters no icons", render(store.StatusBarStyleMeters, func(p *store.Preferences) { p.StatusBar.HideIcons = true }))
	add("rings", render(store.StatusBarStyleRings, nil))
	add("rings values", render(store.StatusBarStyleRings, func(p *store.Preferences) { p.StatusBar.RingValues = true }))
}

func TestTrayRingValuesKeepBalancesBesideTheRing(t *testing.T) {
	cell := runway.StatusBarCell{Provider: store.ProviderCodex, Remaining: 30, Percent: "30%", RingText: "30"}
	r := TrayRender{Style: store.StatusBarStyleRings, Components: []runway.StatusBarComponent{{Provider: store.ProviderCodex, Cells: []runway.StatusBarCell{cell}}}}
	beside := trayContentWidth(r)
	r.RingValues = true
	if inside := trayContentWidth(r); inside >= beside {
		t.Errorf("value inside the ring is %v wide, beside it %v", inside, beside)
	}
	cell.Percent, cell.RingText = "$30.00", ""
	r.Components[0].Cells[0] = cell
	withValues := trayContentWidth(r)
	r.RingValues = false
	if withValues != trayContentWidth(r) {
		t.Errorf("a balance is drawn inside its ring: %v", withValues)
	}
}

func TestTrayRingValuesKeepClearOfTheRing(t *testing.T) {
	radius := trayValueRingRadius - trayValueRingWidth/2
	two := ringTextSize("88", radius)
	for v := 0; v <= 100; v++ {
		s := strconv.Itoa(v)
		size := ringTextSize(s, radius)
		if w := trayTextWidth(s, trayFontWeight(size, 600)); w > 2*radius-5+0.01 {
			t.Errorf("%s is %v wide inside a ring of radius %v", s, w, radius)
		}
		if v >= 10 && v < 100 && size != two {
			t.Errorf("%s is drawn at %v, other two digits at %v", s, size, two)
		}
	}
	if big := ringTextSize("100", radius); big >= two {
		t.Errorf("three digits are drawn at %v, two at %v", big, two)
	}
}

func TestTrayComponentsWidenTheItem(t *testing.T) {
	pair := trayGroups([]store.Provider{store.ProviderCodex, store.ProviderClaude}, []store.Provider{store.ProviderGrok})
	apart := trayGroups([]store.Provider{store.ProviderCodex}, []store.Provider{store.ProviderClaude}, []store.Provider{store.ProviderGrok})
	for _, style := range store.StatusBarStyles() {
		one := TrayFromStatus(trayTestStatus(style, trayGroups([]store.Provider{store.ProviderGrok})))
		two := TrayFromStatus(trayTestStatus(style, pair))
		three := TrayFromStatus(trayTestStatus(style, apart))
		if len(three.Components) != 3 || style.Combines() && len(two.Components) != 2 {
			t.Fatalf("%s components = %d / %d", style, len(two.Components), len(three.Components))
		}
		if !(trayContentWidth(one) < trayContentWidth(two) && trayContentWidth(two) <= trayContentWidth(three)) {
			t.Errorf("%s widths %v %v %v", style, trayContentWidth(one), trayContentWidth(two), trayContentWidth(three))
		}
	}
}

func TestTrayDrawsEmptyProviders(t *testing.T) {
	prefs := store.DefaultPreferences()
	prefs.SetStatusBarGroups([][]store.Provider{{store.ProviderGrok}})
	for _, style := range store.StatusBarStyles() {
		prefs.StatusBarDisplayStyle = style
		r := TrayFromStatus(runway.BuildStatusBar(l10n.New(l10n.English), runway.StatusInputs{Prefs: prefs, Now: trayTestNow}))
		if !r.Components[0].Cells[0].Empty {
			t.Fatalf("%s cells = %+v", style, r.Components[0].Cells)
		}
		if _, err := TrayImage(r, false, false); err != nil {
			t.Fatal(err)
		}
	}
	if w := trayContentWidth(TrayRender{Style: store.StatusBarStyleMeters}); w <= 2*trayHorizontalPadding {
		t.Errorf("no components drew nothing: %v", w)
	}
}

func TestTrayRendererRejectsUnsupportedStyle(t *testing.T) {
	if _, err := TrayImage(TrayRender{Style: "unknown"}, true, false); err == nil {
		t.Fatal("unsupported style must fail visibly")
	}
}

func TestTrayContentCacheDetectsChanges(t *testing.T) {
	a := TrayFromStatus(trayTestStatus(store.StatusBarStyleMeters, nil))
	b := TrayFromStatus(trayTestStatus(store.StatusBarStyleMeters, nil))
	if !sameTrayRender(a, b) {
		t.Fatal("unchanged snapshot must reuse its tray content")
	}
	b.Components[0].Cells[0].MeterText = "changed detail"
	if sameTrayRender(a, b) {
		t.Fatal("detail changes must redraw")
	}
	b = TrayFromStatus(trayTestStatus(store.StatusBarStyleText, nil))
	if sameTrayRender(a, b) {
		t.Fatal("style changes must redraw")
	}
	rememberTray(nil, a, true)
	a.Components[0].Cells[0].Percent = "changed"
	if sameTrayRender(lastAppliedTray.render, a) {
		t.Fatal("the remembered draw shares its cells")
	}
	lastAppliedTray.render = TrayRender{}
}

func TestTrayFitKeepsFittingText(t *testing.T) {
	for _, label := range []string{"Weekly 35%", "每周 35%", "Недельная квота 35%"} {
		f := trayFontWeight(8.5, 600)
		width := trayTextWidth(label, f)
		// A float32 add/subtract from the layout may shrink the width by a
		// fraction of a device pixel; it must still keep the last glyph.
		if got := trayFit(label, f, (width+44)-44); got != label {
			t.Fatalf("fitting label was truncated by float rounding: %q", got)
		}
		if got := trayFit(label, f, width/2); got == label || trayTextWidth(got, f) > width/2 {
			t.Fatalf("long label not shortened: %q", got)
		}
	}
}
