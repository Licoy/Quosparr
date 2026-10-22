package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestStatusBarLayoutIsGoOnlyAndOmittedByDefault(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "statusBarLayout") {
		t.Errorf("the default layout is written: %s", enc)
	}
	p := DefaultPreferences()
	p.SetStatusBarGroups([][]Provider{{ProviderGrok}, {ProviderClaude, ProviderCodex}})
	p.StatusBar.Window = StatusBarWindowWeekly
	p.StatusBar.Battery = BatteryVertical
	p.StatusBar.HideIcons = true
	p.StatusBar.RingValues = true
	if enc, err = EncodePreferences(p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"statusBarProviders":["claude","grok"]`) || !strings.Contains(string(enc), `"statusBarProviderScope":"custom"`) {
		t.Errorf("older versions lose the first two providers: %s", enc)
	}
	got := mustDecode(t, string(enc))
	if !reflect.DeepEqual(got.StatusBar, p.StatusBar) {
		t.Errorf("round trip = %+v, want %+v", got.StatusBar, p.StatusBar)
	}
	want := [][]Provider{{ProviderGrok}, {ProviderClaude, ProviderCodex}}
	if g := got.StatusBarGroups(); !reflect.DeepEqual(g, want) {
		t.Errorf("groups = %v, want %v", g, want)
	}
}

func TestStatusBarLayoutDecodesLeniently(t *testing.T) {
	for raw, want := range map[string]StatusBarLayout{
		`12`:                                {},
		`{"window":"daily","battery":"up"}`: {},
		`{"hideIcons":"yes"}`:               {},
		`{"ringValues":1}`:                  {},
		`{"groups":[["codex","codex","x"],[],["claude","grok","codex"]]}`: {Groups: [][]Provider{{ProviderCodex}, {ProviderClaude, ProviderGrok}}},
		`{"groups":[["codex","claude","grok"]]}`:                          {Groups: [][]Provider{{ProviderCodex, ProviderClaude}, {ProviderGrok}}},
		`{"groups":[]}`:                                                   {},
	} {
		if got := mustDecode(t, `{"statusBarLayout":`+raw+`}`).StatusBar; !reflect.DeepEqual(got, want) {
			t.Errorf("statusBarLayout %s decoded to %+v, want %+v", raw, got, want)
		}
	}
}

func TestStatusBarGroupsFollowTheLegacySelection(t *testing.T) {
	if g := DefaultPreferences().StatusBarGroups(); g != nil {
		t.Errorf("following the panel = %v", g)
	}
	both := mustDecode(t, `{"statusBarProviderScope":"both","statusBarLayout":{"groups":[["claude"]]}}`)
	if g := both.StatusBarGroups(); !reflect.DeepEqual(g, [][]Provider{{ProviderCodex, ProviderGrok}}) {
		t.Errorf("legacy both = %v", g)
	}
	pair := mustDecode(t, `{"statusBarProviderScope":"custom","statusBarProviders":["codex","claude"]}`)
	if g := pair.StatusBarGroups(); !reflect.DeepEqual(g, [][]Provider{{ProviderCodex, ProviderClaude}}) {
		t.Errorf("legacy pair = %v", g)
	}
	// An older version changed the providers and kept the groups it does
	// not know: its choice wins.
	stale := mustDecode(t, `{"statusBarProviderScope":"custom","statusBarProviders":["grok"],"statusBarLayout":{"groups":[["codex"],["claude"]]}}`)
	if g := stale.StatusBarGroups(); !reflect.DeepEqual(g, [][]Provider{{ProviderGrok}}) {
		t.Errorf("stale groups = %v", g)
	}
	three := DefaultPreferences()
	three.SetStatusBarGroups([][]Provider{{ProviderCodex}, {ProviderClaude}, {ProviderGrok}})
	if g := three.StatusBarGroups(); len(g) != 3 {
		t.Errorf("three components = %v", g)
	}
	three.SetStatusBarGroups(nil)
	if g := three.StatusBarGroups(); len(g) != 3 {
		t.Errorf("an empty choice changed the groups: %v", g)
	}
}

func TestStatusBarLayoutIsCloned(t *testing.T) {
	p := DefaultPreferences()
	p.SetStatusBarGroups([][]Provider{{ProviderCodex, ProviderGrok}})
	c := p.Clone()
	c.StatusBar.Groups[0][0] = ProviderClaude
	if p.StatusBar.Groups[0][0] != ProviderCodex {
		t.Error("clone shares the groups")
	}
	g := p.StatusBarGroups()
	g[0][0] = ProviderClaude
	if p.StatusBar.Groups[0][0] != ProviderCodex {
		t.Error("StatusBarGroups shares the groups")
	}
}

func TestStatusBarStylesThatCombine(t *testing.T) {
	for _, st := range StatusBarStyles() {
		want := st != StatusBarStyleRings && st != StatusBarStyleText
		if st.Combines() != want {
			t.Errorf("%s combines = %v", st, st.Combines())
		}
	}
}

func TestStatusBarBatteryKeepsItsWindowUntilOneIsChosen(t *testing.T) {
	p := DefaultPreferences()
	p.StatusBarDisplayStyle = StatusBarStyleBattery
	for scope, want := range map[BatteryScope]StatusBarWindow{
		BatteryScopeFiveHour: StatusBarWindowFiveHour,
		BatteryScopeWeekly:   StatusBarWindowWeekly,
		BatteryScopeBoth:     StatusBarWindowAuto,
	} {
		p.StatusBarBatteryScope = scope
		if got := p.StatusBarQuotaWindow(); got != want {
			t.Errorf("battery %s window = %q, want %q", scope, got, want)
		}
	}
	p.StatusBarBatteryScope = BatteryScopeWeekly
	p.StatusBarDisplayStyle = StatusBarStyleMeters
	if got := p.StatusBarQuotaWindow(); got != StatusBarWindowAuto {
		t.Errorf("meters follow the battery window: %q", got)
	}
	// Choosing a window sets the battery one too, so that every style and
	// older versions agree, and automatic survives a round trip.
	p.StatusBarDisplayStyle = StatusBarStyleBattery
	for _, w := range StatusBarWindows() {
		p.SetStatusBarQuotaWindow(w)
		enc, err := EncodePreferences(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := mustDecode(t, string(enc)).StatusBarQuotaWindow(); got != w {
			t.Errorf("window %q read back as %q", w, got)
		}
	}
}
