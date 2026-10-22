package store

import (
	"reflect"
	"strings"
	"testing"
)

func TestFloatingWindowIsGoOnlyAndOmittedByDefault(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "floatingWindow") {
		t.Errorf("the default floating window is written: %s", enc)
	}
	p := DefaultPreferences()
	p.FloatingWindow = FloatingWindow{Enabled: true, Style: FloatingStyleRings, Providers: []Provider{ProviderGrok, ProviderCodex}, Opacity: 80, Locked: true, OverFullScreen: true,
		AppFilter: FloatingAppFilterAllow, Apps: []FloatingApp{{ID: "com.openai.codex", Name: "Codex", Path: "/Applications/Codex.app"}}, HideOffList: true}
	if enc, err = EncodePreferences(p); err != nil {
		t.Fatal(err)
	}
	got := mustDecode(t, string(enc))
	want := p.FloatingWindow
	want.Providers = []Provider{ProviderCodex, ProviderGrok}
	if !reflect.DeepEqual(got.FloatingWindow, want) {
		t.Errorf("round trip = %+v, want %+v", got.FloatingWindow, want)
	}
}

func TestFloatingWindowDecodesLeniently(t *testing.T) {
	for raw, want := range map[string]FloatingWindow{
		`12`:                              {},
		`{"enabled":"yes"}`:               {},
		`{"style":"nope","enabled":true}`: {Enabled: true},
		`{"opacity":5}`:                   {Opacity: MinFloatingOpacity},
		`{"opacity":100}`:                 {},
		`{"opacity":150}`:                 {},
		`{"providers":["x","claude","claude","codex","grok"]}`: {Providers: []Provider{ProviderCodex, ProviderClaude, ProviderGrok}},
		`{"providers":[]}`:      {},
		`{"appFilter":"maybe"}`: {},
		`{"appFilter":"allow","apps":[{"id":""},{"id":"a","name":"A"},{"id":"a"},{"id":"b"}]}`: {
			AppFilter: FloatingAppFilterAllow, Apps: []FloatingApp{{ID: "a", Name: "A"}, {ID: "b"}},
		},
		`{"apps":[]}`:           {},
		`{"hideOffList":"yes"}`: {},
		`{"hideOffList":false}`: {},
	} {
		if got := mustDecode(t, `{"floatingWindow":`+raw+`}`).FloatingWindow; !reflect.DeepEqual(got, want) {
			t.Errorf("floatingWindow %s decoded to %+v, want %+v", raw, got, want)
		}
	}
}

func TestFloatingSelectionFollowsThePanel(t *testing.T) {
	p := DefaultPreferences()
	p.SelectedProvider = ProviderClaude
	if got := p.FloatingSelection(); !reflect.DeepEqual(got, []Provider{ProviderClaude}) {
		t.Errorf("selection = %v", got)
	}
	p.FloatingWindow.Providers = []Provider{ProviderGrok, ProviderCodex}
	if got := p.FloatingSelection(); !reflect.DeepEqual(got, []Provider{ProviderCodex, ProviderGrok}) {
		t.Errorf("custom selection = %v", got)
	}
	if got := p.FloatingWindow.OpacityPercent(); got != 100 {
		t.Errorf("default opacity = %d", got)
	}
}

func TestFloatingAppsAreCappedAndCloned(t *testing.T) {
	var f FloatingWindow
	for i := range MaxFloatingApps + 5 {
		f.Apps = append(f.Apps, FloatingApp{ID: string(rune('a'+i%26)) + strings.Repeat("x", i)})
	}
	f.normalize()
	if len(f.Apps) != MaxFloatingApps {
		t.Errorf("kept %d apps, want %d", len(f.Apps), MaxFloatingApps)
	}
	c := f.clone()
	c.Apps[0].ID = "changed"
	if f.Apps[0].ID == "changed" || !f.HasApp(f.Apps[1].ID) || f.HasApp("changed") {
		t.Errorf("clone shares the apps: %v", f.Apps[0])
	}
}
