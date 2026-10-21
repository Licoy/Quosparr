package runway

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

var zh = l10n.New(l10n.SimplifiedChinese)

func testMeter(title string, used, minutes int, src MeterSource) Meter {
	now := time.Now()
	return NewMeter(title, MeterWindow{UsedPercent: used, WindowMinutes: minutes,
		ResetsAt: now.Add(time.Duration(minutes) * time.Minute)}, now, DefaultMarkers(), src)
}

func cellPercents(comp StatusBarComponent) []string {
	var out []string
	for _, c := range comp.Cells {
		out = append(out, c.Percent)
	}
	return out
}

func TestStatusBarFollowsThePanelWithTwoWindows(t *testing.T) {
	five := testMeter("5小时", 20, FiveHourMinutes, SourceStandard)
	weekly := testMeter("每周", 11, WeeklyMinutes, SourceStandard)
	spark := testMeter("GPT-5.3-Codex-Spark", 0, WeeklyMinutes, SourceModelSpecific)
	v := BuildStatusBar(zh, StatusInputs{Prefs: store.DefaultPreferences(), Provider: store.ProviderCodex,
		CodexText: "6天23小时", CodexMeters: []Meter{weekly, spark, five}, Now: time.Now()})
	if len(v.Components) != 1 || v.Components[0].Provider != store.ProviderCodex {
		t.Fatalf("components = %+v", v.Components)
	}
	cells := v.Components[0].Cells
	if len(cells) != 2 || cells[0].Label != "5小时" || cells[1].Label != "每周" || cells[0].Percent != "80%" || cells[1].Percent != "89%" {
		t.Fatalf("cells = %+v, want the 5-hour window first and no model window", cells)
	}
	if cells[0].Countdown == "--" || cells[0].Remaining != 80 || cells[0].Health != five.Health {
		t.Errorf("first cell = %+v", cells[0])
	}
}

func TestStatusBarGroupsStackOrSeparateProviders(t *testing.T) {
	five := testMeter("5-hour", 20, FiveHourMinutes, SourceStandard)
	weekly := testMeter("Weekly", 60, WeeklyMinutes, SourceStandard)
	grokWeek := testMeter("周度包含额度", 42, WeeklyMinutes, SourceStandard)
	build := testMeter("Build", 10, WeeklyMinutes, SourceModelSpecific)
	p := store.DefaultPreferences()
	p.SetStatusBarGroups([][]store.Provider{{store.ProviderCodex, store.ProviderClaude}, {store.ProviderGrok}})
	in := StatusInputs{Prefs: p, Provider: store.ProviderCodex, CodexMeters: []Meter{weekly, five},
		Claude: ClaudeState{Availability: ClaudeReady, Meters: []Meter{weekly, five}},
		Grok:   GrokState{Availability: GrokReady, Meters: []Meter{grokWeek, build}}, Now: time.Now()}
	for _, st := range store.StatusBarStyles() {
		in.Prefs.StatusBarDisplayStyle = st
		v := BuildStatusBar(zh, in)
		if !st.Combines() {
			if len(v.Components) != 3 || v.Components[0].Provider != store.ProviderCodex || len(v.Components[0].Cells) != 2 {
				t.Errorf("%s shows every provider alone: %+v", st, v.Components)
			}
			continue
		}
		if len(v.Components) != 2 {
			t.Fatalf("%s components = %+v", st, v.Components)
		}
		pair, grok := v.Components[0], v.Components[1]
		if pair.Provider != "" || len(pair.Cells) != 2 || pair.Cells[0].Provider != store.ProviderCodex || pair.Cells[1].Provider != store.ProviderClaude {
			t.Errorf("%s pair = %+v", st, pair)
		}
		if !reflect.DeepEqual(cellPercents(pair), []string{"80%", "80%"}) || pair.Cells[0].Label != "" {
			t.Errorf("%s pair shows the main windows unlabeled: %+v", st, pair.Cells)
		}
		if grok.Provider != store.ProviderGrok || !reflect.DeepEqual(cellPercents(grok), []string{"58%"}) || grok.Cells[0].Label != "" {
			t.Errorf("%s grok = %+v", st, grok)
		}
	}
}

func TestStatusBarWindowChoosesOneWindowEach(t *testing.T) {
	five := testMeter("5-hour", 20, FiveHourMinutes, SourceStandard)
	weekly := testMeter("Weekly", 60, WeeklyMinutes, SourceStandard)
	monthly := testMeter("Monthly", 30, MonthlyMinutes, SourceStandard)
	p := store.DefaultPreferences()
	p.SetStatusBarGroups([][]store.Provider{{store.ProviderCodex}, {store.ProviderGrok}})
	in := StatusInputs{Prefs: p, CodexMeters: []Meter{five, weekly}, Grok: GrokState{Availability: GrokReady, Meters: []Meter{monthly}}, Now: time.Now()}
	for window, want := range map[store.StatusBarWindow][]string{
		store.StatusBarWindowAuto:     {"80%", "40%", "70%"},
		store.StatusBarWindowFiveHour: {"80%", "70%"},
		store.StatusBarWindowWeekly:   {"40%", "70%"},
	} {
		in.Prefs.StatusBar.Window = window
		var got []string
		for _, comp := range BuildStatusBar(zh, in).Components {
			got = append(got, cellPercents(comp)...)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("window %q = %v, want %v", window, got, want)
		}
	}
}

func TestStatusBarProviderWithoutReading(t *testing.T) {
	five := testMeter("5-hour", 20, FiveHourMinutes, SourceStandard)
	p := store.DefaultPreferences()
	p.SetStatusBarGroups([][]store.Provider{{store.ProviderGrok}, {store.ProviderClaude}})
	v := BuildStatusBar(zh, StatusInputs{Prefs: p, Grok: GrokState{Availability: GrokNotLoggedIn},
		Claude: ClaudeState{Availability: ClaudeReady, Meters: []Meter{five}}, Now: time.Now()})
	grok := v.Components[0]
	if len(grok.Cells) != 1 || !grok.Cells[0].Empty || grok.Cells[0].Percent != "--" || grok.Cells[0].Countdown != "--" {
		t.Fatalf("grok = %+v", grok)
	}
	lines := strings.Split(v.Tooltip, "\n")
	if len(lines) != 3 || lines[0] != "Quosparr" || lines[1] != "Grok: "+zh.Text(l10n.KeyStatusLogin) || !strings.HasPrefix(lines[2], "Claude: 5小时 80% (") {
		t.Errorf("tooltip = %q", v.Tooltip)
	}
}

func TestStatusBarDetailsReachTheCells(t *testing.T) {
	five := testMeter("5-hour", 20, FiveHourMinutes, SourceStandard)
	p := store.DefaultPreferences()
	p.StatusBarMetersDetailStyle = store.MetersDetailBoth
	p.StatusBarBatteryDetailStyle = store.BatteryDetailRemainingPercent
	now := time.Now()
	c := BuildStatusBar(zh, StatusInputs{Prefs: p, Provider: store.ProviderCodex, CodexMeters: []Meter{five}, Now: now}).Components[0].Cells[0]
	if c.MeterText != MeterDetail(five, store.MetersDetailBoth, now, zh.Language()) || !strings.Contains(c.MeterText, " · ") {
		t.Errorf("meter text = %q", c.MeterText)
	}
	if c.BatteryText != "80%" || c.Countdown != CountdownDetail(five, now, zh.Language()) {
		t.Errorf("cell = %+v", c)
	}
}

func TestStatusBarFollowsTheSelectedProvider(t *testing.T) {
	week := testMeter("7-day", 11, WeeklyMinutes, SourceStandard)
	v := BuildStatusBar(zh, StatusInputs{Prefs: store.DefaultPreferences(), Provider: store.ProviderClaude,
		Claude: ClaudeState{Availability: ClaudeReady, Meters: []Meter{week}}, Now: time.Now()})
	if len(v.Components) != 1 || v.Components[0].Provider != store.ProviderClaude || v.Components[0].Cells[0].Label != "" {
		t.Errorf("components = %+v", v.Components)
	}
}

func TestCodexPrefersFiveHourOverWeekly(t *testing.T) {
	five := testMeter("5h", 20, 300, SourceStandard)
	weekly := testMeter("weekly", 40, 10080, SourceStandard)
	pick, ok := PrimaryStandardMeter([]Meter{weekly, five}, codexPreferredWindows)
	if !ok || pick.Title != "5h" {
		t.Fatalf("pick = %+v", pick)
	}
}

func TestCodexFallsBackToWeekly(t *testing.T) {
	weekly := testMeter("weekly", 40, 10080, SourceStandard)
	model := testMeter("gpt", 10, 10080, SourceModelSpecific)
	pick, _ := PrimaryStandardMeter([]Meter{model, weekly}, codexPreferredWindows)
	if pick.Title != "weekly" || pick.Source != SourceStandard {
		t.Fatalf("pick = %+v", pick)
	}
}

func TestGrokPrefersWeeklyOverMonthly(t *testing.T) {
	weekly := testMeter("week", 18, 10080, SourceStandard)
	monthly := testMeter("month", 30, 30*24*60, SourceStandard)
	pick, _ := PrimaryStandardMeter([]Meter{monthly, weekly}, grokPreferredWindows)
	if pick.Title != "week" {
		t.Fatalf("pick = %+v", pick)
	}
}

func TestShortWindowTitlesMatchCodexWording(t *testing.T) {
	for minutes, want := range map[int]string{300: "5小时", 10080: "每周", 30 * 24 * 60: "每月"} {
		if got := ShortWindowTitle(zh, minutes); got != want {
			t.Errorf("%d min = %q, want %q", minutes, got, want)
		}
	}
	long := testMeter("周度包含额度", 27, 10080, SourceStandard)
	if WithShortTitle(zh, long).Title != "每周" {
		t.Error("long title not shortened")
	}
}

func TestShortestWindowMeterPrefersShortestStandardWindow(t *testing.T) {
	weekly := testMeter("weekly", 10, WeeklyMinutes, SourceStandard)
	five := testMeter("5h", 40, FiveHourMinutes, SourceStandard)
	model := testMeter("model", 90, 60, SourceModelSpecific)
	unknown := testMeter("unknown", 20, 0, SourceStandard)
	cases := []struct {
		in   []Meter
		want string
	}{
		{[]Meter{weekly, five, model}, "5h"},
		{[]Meter{unknown, weekly}, "weekly"},
		{[]Meter{unknown}, "unknown"},
		{[]Meter{model}, "model"},
	}
	for _, tc := range cases {
		if m, ok := ShortestWindowMeter(tc.in); !ok || m.Title != tc.want {
			t.Errorf("got %q, want %q", m.Title, tc.want)
		}
	}
	if _, ok := ShortestWindowMeter(nil); ok {
		t.Error("empty meters picked a meter")
	}
}
