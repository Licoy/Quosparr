package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
)

const (
	speedTestTitle   = "Fix flaky parser test"
	speedTestProject = "quosparr"
)

func speedTestStat(model, effort string, calls int, p50, p90 time.Duration, tokens int64, decode time.Duration) speed.Stat {
	return speed.Stat{Model: model, Effort: effort, Calls: calls, TTFTP50: p50, TTFTP90: p90, OutputTokens: tokens, Decode: decode}
}

// speedFixture has four models, so the card has one more than it shows.
func speedFixture() *speed.Report {
	now := time.Now()
	ms := time.Millisecond
	high := speedTestStat("gpt-5.5", "high", 12, 2400*ms, 4100*ms, 7200, 200*time.Second)
	medium := speedTestStat("gpt-5.5", "medium", 8, 1800*ms, 3000*ms, 3000, 60*time.Second)
	mini := speedTestStat("gpt-5.4-mini", "", 5, 800*ms, 1200*ms, 0, 0)
	low := speedTestStat("gpt-4.1", "low", 1, 500*ms, 500*ms, 900, 30*time.Second)
	return &speed.Report{
		Groups: []speed.Stat{high, medium, mini, low},
		Sessions: []speed.SessionStat{
			{ID: "s1", Title: speedTestTitle, ProjectName: speedTestProject, UpdatedAt: now.Add(-3 * time.Minute), Groups: []speed.Stat{high, medium}},
			{ID: "s2", Title: "Review the release notes", ProjectName: "docs", UpdatedAt: now.Add(-5 * time.Hour), Groups: []speed.Stat{mini, low}},
		},
		From: now.AddDate(0, 0, -6), To: now, CalculatedAt: now,
	}
}

// onlySpeedModule hides every module of the main panel but the speed analysis.
func onlySpeedModule(p *store.Preferences) {
	for _, m := range store.Modules() {
		if v := moduleShown(p, m); v != nil {
			*v = m == store.ModuleSpeedAnalysis
		}
	}
}

func speedPanel(t *testing.T, mutate func(*store.Preferences), snap runway.Snapshot, acts PanelActions) (*App, *ui.Tester) {
	t.Helper()
	return newPanel(t, func(p *store.Preferences) {
		onlySpeedModule(p)
		if mutate != nil {
			mutate(p)
		}
	}, snap, acts)
}

func speedSnapshot(r *speed.Report) runway.Snapshot {
	snap := panelFixture()
	snap.Codex.Speed = r
	return snap
}

func TestPanelSpeedCardShowsTopThreeGroupsWithAttribution(t *testing.T) {
	_, tt := speedPanel(t, nil, speedSnapshot(speedFixture()), PanelActions{})
	l := l10n.New(l10n.English)
	for _, text := range []string{
		tx(l, xSpeedTitle), "gpt-5.5 high", "gpt-5.5 medium", "gpt-5.4-mini", "2.4s", "1.8s", "0.8s", "36 tok/s", "50 tok/s", l.Text(l10n.KeyShowDetails),
		tx(l, xSpeedBasis, 2), tx(l, xSpeedLatest, speedTestTitle), runway.RelativePast(time.Now().Add(-3*time.Minute), time.Now(), l.Language(), false),
	} {
		if !hasExact(tt, text) {
			t.Fatalf("speed card missing %q; texts %q", text, tt.Texts())
		}
	}
	if tt.HasText("gpt-4.1") {
		t.Fatalf("speed card shows a fourth model; texts %q", tt.Texts())
	}
	if hasExact(tt, "high") || hasExact(tt, "medium") {
		t.Fatalf("the reasoning effort is drawn apart from the model name; texts %q", tt.Texts())
	}
	if !hasExact(tt, "—") {
		t.Fatal("a model without output samples shows no dash for its speed")
	}
}

func TestPanelSpeedCardSingleSessionBasis(t *testing.T) {
	r := speedFixture()
	r.Sessions = r.Sessions[:1]
	_, tt := speedPanel(t, nil, speedSnapshot(r), PanelActions{})
	if !hasExact(tt, tx(l10n.New(l10n.English), xSpeedBasisOne)) {
		t.Fatalf("single-session basis missing; texts %q", tt.Texts())
	}
}

func TestPanelSpeedFollowsTokenUsageInModuleOrder(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt := newPanel(t, nil, speedSnapshot(speedFixture()), PanelActions{})
	token, ok := tt.Find(l.Text(l10n.KeyTokenUsageHeatmap))
	speedTitle, ok2 := tt.Find(tx(l, xSpeedTitle))
	estimate, ok3 := tt.Find(l.Text(l10n.KeyQuotaEstimate))
	if !ok || !ok2 || !ok3 || token.Y >= speedTitle.Y || speedTitle.Y >= estimate.Y {
		t.Fatalf("speed card is not between token usage and the later modules: token=%+v speed=%+v estimate=%+v (%v %v %v)", token, speedTitle, estimate, ok, ok2, ok3)
	}
}

func TestPanelSpeedHiddenWhenPreferenceOff(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt := newPanel(t, func(p *store.Preferences) { p.ShowsSpeedAnalysis = false }, speedSnapshot(speedFixture()), PanelActions{})
	if tt.HasText(tx(l, xSpeedTitle)) || tt.HasText("gpt-5.5") {
		t.Fatalf("speed card still shown; texts %q", tt.Texts())
	}
	if !tt.HasText(l.Text(l10n.KeyQuota)) {
		t.Fatal("hiding the speed card removed the rest of the panel")
	}
}

func TestPanelSpeedGrokUsesGrokReport(t *testing.T) {
	codexOnly := &speed.Report{Groups: []speed.Stat{speedTestStat("codex-only-model", "high", 3, time.Second, time.Second, 100, time.Second)}}
	grokReport := &speed.Report{
		Groups:   []speed.Stat{speedTestStat("grok-code-fast-1", "medium", 4, 1500*time.Millisecond, 2*time.Second, 400, 10*time.Second)},
		Sessions: []speed.SessionStat{{Title: "Grok session", UpdatedAt: time.Now().Add(-time.Hour)}},
	}
	snap := speedSnapshot(codexOnly)
	snap.Grok = runway.GrokState{Availability: runway.GrokReady, Speed: grokReport}
	var refreshed []runway.Section
	_, tt := speedPanel(t, func(p *store.Preferences) { p.SelectedProvider = store.ProviderGrok }, snap, PanelActions{RefreshSection: func(s runway.Section) { refreshed = append(refreshed, s) }})
	for _, text := range []string{"grok-code-fast-1 medium", "1.5s", "40 tok/s", tx(l10n.New(l10n.English), xSpeedLatest, "Grok session")} {
		if !hasExact(tt, text) {
			t.Fatalf("Grok speed card missing %q; texts %q", text, tt.Texts())
		}
	}
	if tt.HasText("codex-only-model") {
		t.Fatal("the Grok panel shows the Codex speed report")
	}
	// The header has its own refresh button, so press the one on the speed title.
	title, _ := tt.Find(tx(l10n.New(l10n.English), xSpeedTitle))
	tt.ClickAt(panelWidth-16-12, title.Y+title.H/2)
	if len(refreshed) != 1 || refreshed[0] != runway.SectionSpeed {
		t.Fatalf("refresh sections = %v, want the speed section", refreshed)
	}
}

func TestPanelSpeedLoadingErrorAndEmptyStates(t *testing.T) {
	l := l10n.New(l10n.English)
	show := l.Text(l10n.KeyShowDetails)
	t.Run("not loaded", func(t *testing.T) {
		_, tt := speedPanel(t, nil, speedSnapshot(nil), PanelActions{})
		if !hasExact(tt, l.Text(l10n.KeyNotLoaded)) || hasExact(tt, show) {
			t.Fatalf("texts %q", tt.Texts())
		}
	})
	t.Run("calculating", func(t *testing.T) {
		snap := speedSnapshot(nil)
		snap.Refresh.Sections = []runway.Section{runway.SectionSpeed}
		_, tt := speedPanel(t, nil, snap, PanelActions{})
		if !hasExact(tt, tx(l, xPanelLoading)) {
			t.Fatalf("texts %q", tt.Texts())
		}
	})
	t.Run("grok calculating", func(t *testing.T) {
		snap := speedSnapshot(nil)
		snap.Grok = runway.GrokState{Refreshing: true}
		_, tt := speedPanel(t, func(p *store.Preferences) { p.SelectedProvider = store.ProviderGrok }, snap, PanelActions{})
		if !hasExact(tt, tx(l, xPanelLoading)) {
			t.Fatalf("texts %q", tt.Texts())
		}
	})
	t.Run("error", func(t *testing.T) {
		snap := speedSnapshot(nil)
		snap.Codex.SpeedError = "sessions folder unreadable"
		_, tt := speedPanel(t, nil, snap, PanelActions{})
		if !hasExact(tt, "sessions folder unreadable") || hasExact(tt, l.Text(l10n.KeyNotLoaded)) {
			t.Fatalf("texts %q", tt.Texts())
		}
	})
	t.Run("error beside stale data", func(t *testing.T) {
		snap := speedSnapshot(speedFixture())
		snap.Codex.SpeedError = "sessions folder unreadable"
		_, tt := speedPanel(t, nil, snap, PanelActions{})
		if !hasExact(tt, "sessions folder unreadable") || !hasExact(tt, "gpt-5.5 high") {
			t.Fatalf("texts %q", tt.Texts())
		}
	})
	t.Run("no recent data", func(t *testing.T) {
		_, tt := speedPanel(t, nil, speedSnapshot(&speed.Report{}), PanelActions{})
		if !hasExact(tt, tx(l, xSpeedEmpty)) || hasExact(tt, show) || hasExact(tt, l.Text(l10n.KeyNotLoaded)) {
			t.Fatalf("texts %q", tt.Texts())
		}
	})
}

func TestPanelSpeedDetailOpensWithTableAndSessions(t *testing.T) {
	l := l10n.New(l10n.English)
	a, tt := speedPanel(t, nil, speedSnapshot(speedFixture()), PanelActions{})
	if err := tt.Click(l.Text(l10n.KeyShowDetails)); err != nil {
		t.Fatal(err)
	}
	if a.panelState.detail != store.ModuleSpeedAnalysis {
		t.Fatalf("detail = %q, want the speed analysis page", a.panelState.detail)
	}
	r := speedFixture()
	for _, text := range []string{
		tx(l, xSpeedTitle), l.Text(l10n.KeyBack), tx(l, xSpeedByModel), tx(l, xSpeedModel), "P50", "P90", "tok/s", tx(l, xSpeedCalls),
		"gpt-4.1 low", "0.5s", "12", "8", "5", "1",
		l.Text(l10n.KeyRecentSessions), speedTestTitle, "Review the release notes",
		tx(l, xSpeedRange, r.From.Local().Format(time.DateOnly), r.To.Local().Format(time.DateOnly)),
		tx(l, xSpeedInfoTTFT), tx(l, xSpeedInfoOutput), tx(l, xSpeedInfoSource),
	} {
		if !hasExact(tt, text) {
			t.Fatalf("speed detail missing %q; texts %q", text, tt.Texts())
		}
	}
	for _, text := range []string{speedTestProject + " · " + runway.RelativePast(time.Now().Add(-3*time.Minute), time.Now(), l.Language(), false), "docs · " + runway.RelativePast(time.Now().Add(-5*time.Hour), time.Now(), l.Language(), false)} {
		if !hasExact(tt, text) {
			t.Fatalf("session line missing %q; texts %q", text, tt.Texts())
		}
	}
	if err := tt.Click(l.Text(l10n.KeyBack)); err != nil {
		t.Fatal(err)
	}
	if a.panelState.detail != "" {
		t.Fatal("Back did not leave the speed page")
	}
}

func TestPanelSpeedDetailEmptyAndErrorStates(t *testing.T) {
	l := l10n.New(l10n.English)
	for name, test := range map[string]struct {
		report *speed.Report
		err    string
		want   string
	}{
		"not loaded": {nil, "", l.Text(l10n.KeyNotLoaded)},
		"error":      {nil, "sessions folder unreadable", "sessions folder unreadable"},
		"empty":      {&speed.Report{}, "", tx(l, xSpeedEmpty)},
	} {
		t.Run(name, func(t *testing.T) {
			snap := speedSnapshot(test.report)
			snap.Codex.SpeedError = test.err
			a, tt := speedPanel(t, nil, snap, PanelActions{})
			a.panelState.detail = store.ModuleSpeedAnalysis
			tt.Frame()
			if !hasExact(tt, test.want) || !hasExact(tt, tx(l, xSpeedTitle)) {
				t.Fatalf("texts %q", tt.Texts())
			}
		})
	}
	snap := speedSnapshot(&speed.Report{})
	snap.Codex.SpeedError = "sessions folder unreadable"
	a, tt := speedPanel(t, nil, snap, PanelActions{})
	a.panelState.detail = store.ModuleSpeedAnalysis
	tt.Frame()
	if !hasExact(tt, tx(l, xSpeedEmpty)) || !hasExact(tt, snap.Codex.SpeedError) {
		t.Fatalf("empty report detail dropped its error; texts %q", tt.Texts())
	}
}

func TestPanelSpeedGrokDetailUsesGrokReport(t *testing.T) {
	snap := speedSnapshot(&speed.Report{Groups: []speed.Stat{speedTestStat("codex-only-model", "high", 3, time.Second, time.Second, 100, time.Second)}})
	snap.Grok = runway.GrokState{Availability: runway.GrokReady, Speed: speedFixture()}
	a, tt := speedPanel(t, func(p *store.Preferences) { p.SelectedProvider = store.ProviderGrok }, snap, PanelActions{})
	a.panelState.detail = store.ModuleSpeedAnalysis
	tt.Frame()
	if !tt.HasText("gpt-4.1") || tt.HasText("codex-only-model") {
		t.Fatalf("Grok detail does not show the Grok report; texts %q", tt.Texts())
	}
}

func TestProviderSpeedErrorsFollowTheSelectedProvider(t *testing.T) {
	l := l10n.New(l10n.English)
	snap := speedSnapshot(nil)
	snap.Grok.SpeedError = "grok sessions unreadable"
	snap.Claude.SpeedError = "claude sessions unreadable"
	snap.Provider = store.ProviderGrok
	if got := providerView(l, snap, store.CostRangeCurrent).SpeedError; got != snap.Grok.SpeedError {
		t.Fatalf("Grok speed error = %q", got)
	}
	snap.Provider = store.ProviderClaude
	if got := providerView(l, snap, store.CostRangeCurrent).SpeedError; got != snap.Claude.SpeedError {
		t.Fatalf("Claude speed error = %q", got)
	}
}

// Claude logs have no first-token time: the card, the table and the session
// rows leave the TTFT and call count columns out, and the notes say why.
func TestPanelSpeedClaudeHidesTTFT(t *testing.T) {
	l := l10n.New(l10n.English)
	snap := speedSnapshot(nil)
	snap.Claude = runway.ClaudeState{Speed: speedFixture()}
	a, tt := speedPanel(t, func(p *store.Preferences) { p.SelectedProvider = store.ProviderClaude }, snap, PanelActions{})
	if !hasExact(tt, "36 tok/s") || hasExact(tt, tx(l, xSpeedTTFT)) {
		t.Fatalf("Claude speed card shows TTFT or misses the rate; texts %q", tt.Texts())
	}
	a.panelState.detail = store.ModuleSpeedAnalysis
	tt.Frame()
	for _, text := range []string{tx(l, xSpeedTTFT), "P50", "P90", tx(l, xSpeedCalls), tx(l, xSpeedInfoTTFT)} {
		if hasExact(tt, text) {
			t.Fatalf("Claude speed detail shows %q; texts %q", text, tt.Texts())
		}
	}
	if !hasExact(tt, "36") || !tt.HasText(tx(l, xSpeedInfoNoTTFT)) {
		t.Fatalf("Claude speed detail misses the rate or the note; texts %q", tt.Texts())
	}
	if !tt.HasText(tx(l, xSpeedInfoOutput)) {
		t.Fatalf("Claude speed detail misses its end-to-end timing formula; texts %q", tt.Texts())
	}
}

func TestSpeedFigures(t *testing.T) {
	ms := time.Millisecond
	for _, test := range []struct {
		d     time.Duration
		calls int
		want  string
	}{{2400 * ms, 3, "2.4s"}, {800 * ms, 1, "0.8s"}, {0, 0, "—"}, {time.Second, 0, "—"}, {125 * time.Second, 2, "125s"}} {
		if got := speedTTFT(test.d, test.calls); got != test.want {
			t.Errorf("speedTTFT(%v, %d) = %q, want %q", test.d, test.calls, got, test.want)
		}
	}
	for _, test := range []struct {
		s    speed.Stat
		unit bool
		want string
	}{
		{speedTestStat("m", "", 1, 0, 0, 7200, 200*time.Second), true, "36 tok/s"},
		{speedTestStat("m", "", 1, 0, 0, 25, 10*time.Second), true, "2.5 tok/s"},
		{speedTestStat("m", "", 1, 0, 0, 7200, 200*time.Second), false, "36"},
		{speedTestStat("m", "", 1, 0, 0, 0, 0), true, "—"},
	} {
		if got := speedRate(test.s, test.unit); got != test.want {
			t.Errorf("speedRate(%+v, %v) = %q, want %q", test.s, test.unit, got, test.want)
		}
	}
}

func speedLongModelReport() *speed.Report {
	r := speedFixture()
	long := speedTestStat("claude-sonnet-4-5-20250929-extended-thinking-preview", "xhigh", 99, 12300*time.Millisecond, 98*time.Second, 123456, 40*time.Second)
	r.Groups = append([]speed.Stat{long}, r.Groups...)
	r.Sessions[0].Title = "A very long conversation title that cannot possibly fit on a single line of the panel"
	r.Sessions[0].Groups = append([]speed.Stat{long}, r.Sessions[0].Groups...)
	return r
}

// speedRowRects are the cells of the first card row, left to right: the
// model and effort label, the TTFT label, the TTFT figure and the speed.
func speedRowRects(t *testing.T, tt *ui.Tester, l l10n.Localizer, label, ttft, rate string) []ui.Rect {
	t.Helper()
	var rects []ui.Rect
	for _, text := range []string{label, tx(l, xSpeedTTFT), ttft, rate} {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("row cell %q missing; texts %q", text, tt.Texts())
		}
		rects = append(rects, r)
	}
	return rects
}

// checkSpeedRow checks that the cells are in order on one line and that the
// figures end inside the card, however much of the name had to go.
func checkSpeedRow(t *testing.T, cells []ui.Rect, width int) {
	t.Helper()
	rowEnd := float32(width - 16 - 10)
	for i := 1; i < len(cells); i++ {
		if cells[i].X < cells[i-1].X+cells[i-1].W-0.5 {
			t.Fatalf("row cells overlap or run out of order at %d: %+v", i, cells)
		}
	}
	if last := cells[len(cells)-1]; last.X+last.W > rowEnd+0.5 || cells[0].W <= 0 {
		t.Fatalf("row runs out of the card (ends at %v of %v): %+v", last.X+last.W, rowEnd, cells)
	}
	for i, cell := range cells {
		if d := cell.Y + cell.H/2 - (cells[0].Y + cells[0].H/2); d > 6 || d < -6 {
			t.Fatalf("row cell %d is off the model line by %v: %+v", i, d, cells)
		}
	}
}

// The model with its effort and the two figures of the first row sit on one
// line in order, inside the card, in every language and for a long name.
func TestPanelSpeedCardRowLayoutInEveryLanguage(t *testing.T) {
	for _, lang := range l10n.Languages() {
		for _, long := range []bool{false, true} {
			for _, dark := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/long=%v/dark=%v", lang, long, dark), func(t *testing.T) {
					r := speedFixture()
					label, ttft, rate := "gpt-5.5 high", "2.4s", "36 tok/s"
					if long {
						r = speedLongModelReport()
						label, ttft, rate = r.Groups[0].Model+" xhigh", "12.3s", "3086 tok/s"
					}
					l := l10n.New(lang)
					_, tt := speedPanel(t, func(p *store.Preferences) { p.Language = store.Language(lang) }, speedSnapshot(r), PanelActions{})
					tt.SetDark(dark)
					tt.Frame()
					checkSpeedRow(t, speedRowRects(t, tt, l, label, ttft, rate), panelWidth)
					attribution, ok := tt.Find(tx(l, xSpeedBasis, 2))
					if !ok || attribution.X+attribution.W > panelWidth-16 {
						t.Fatalf("attribution leaves the panel: %+v", attribution)
					}
					for _, text := range tt.Texts() {
						if r, _ := tt.Find(text); r.X < 0 || r.X+r.W > panelWidth {
							t.Fatalf("text %q exceeds the panel: %+v", text, r)
						}
					}
				})
			}
		}
	}
}

// Narrower than the panel, only the name gives way: the figures stay whole
// and visible, in every language.
func TestPanelSpeedCardKeepsFiguresWhenNarrow(t *testing.T) {
	for _, lang := range l10n.Languages() {
		for _, width := range []int{280, 320, 360} {
			t.Run(fmt.Sprintf("%s/%d", lang, width), func(t *testing.T) {
				l := l10n.New(lang)
				r := speedLongModelReport()
				snap := speedSnapshot(r)
				tt := ui.NewTester(func(c *ui.Context) {
					applyTheme(c)
					ui.Column(c).Fill().Padding(16).Children(func() {
						first := true
						panelSpeed(c, l, snap, PanelActions{}, &first, &panelState{})
					})
				}, width, 300)
				cells := speedRowRects(t, tt, l, r.Groups[0].Model+" xhigh", "12.3s", "3086 tok/s")
				checkSpeedRow(t, cells, width)
				if cells[0].W < 40 {
					t.Fatalf("the model name is squeezed to %v: %+v", cells[0].W, cells)
				}
			})
		}
	}
}

// A tiered report labels each known tier and explains the split.
func TestPanelSpeedDetailMarksTiers(t *testing.T) {
	l := l10n.New(l10n.English)
	plain := speedFixture()
	_, tt := speedPanel(t, nil, speedSnapshot(plain), PanelActions{})
	if err := tt.Click(l.Text(l10n.KeyShowDetails)); err != nil {
		t.Fatal(err)
	}
	if tt.HasText(tx(l, xSpeedInfoTier)) || hasExact(tt, "gpt-5.5 high · Fast") {
		t.Fatalf("untiered report shows tiers; texts %q", tt.Texts())
	}

	r := speedFixture()
	r.Tiered = true
	r.Groups[0].Tier = speed.TierFast
	r.Groups[1].Tier = speed.TierStandard
	r.Groups[2].Tier = speed.TierUltrafast
	r.Sessions[0].Groups[0].Tier = speed.TierUltrafast
	_, tt = speedPanel(t, nil, speedSnapshot(r), PanelActions{})
	if !hasExact(tt, "gpt-5.4-mini · Ultrafast") {
		t.Fatalf("speed card missing Ultrafast; texts %q", tt.Texts())
	}
	if err := tt.Click(l.Text(l10n.KeyShowDetails)); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"gpt-5.5 high · Fast", "gpt-5.5 medium · Standard", "gpt-5.4-mini · Ultrafast", "gpt-5.5 high · Ultrafast", tx(l, xSpeedInfoTier)} {
		if !hasExact(tt, text) {
			t.Fatalf("tiered detail missing %q; texts %q", text, tt.Texts())
		}
	}
}

// The (i) button explains TTFT, the output speed and where they come from,
// and the explanation fits the panel in every language.
func TestPanelSpeedInfoPopoverExplainsTheFigures(t *testing.T) {
	for _, lang := range l10n.Languages() {
		t.Run(string(lang), func(t *testing.T) {
			l := l10n.New(lang)
			_, tt := speedPanel(t, func(p *store.Preferences) { p.Language = store.Language(lang) }, speedSnapshot(speedFixture()), PanelActions{})
			title, _ := tt.Find(tx(l, xSpeedTitle))
			// The info button is the second control from the right of the title row.
			tt.ClickAt(panelWidth-16-24-6-12, title.Y+title.H/2)
			for _, key := range []textKey{xSpeedInfoTTFT, xSpeedInfoOutput, xSpeedInfoSource} {
				r, ok := tt.Find(tx(l, key))
				if !ok || r.X < 0 || r.X+r.W > panelWidth || r.Y < 0 {
					t.Fatalf("info text %q missing or outside the panel: %+v; texts %q", key, r, tt.Texts())
				}
			}
			if err := tt.Click(l.Text(l10n.KeyOk)); err != nil {
				t.Fatal(err)
			}
			if tt.HasText(tx(l, xSpeedInfoTTFT)) {
				t.Fatal("OK did not close the info popover")
			}
		})
	}
}

// The attribution line keeps its basis and age whole and shortens only the
// title of the newest session.
func TestPanelSpeedAttributionShortensOnlyTheTitle(t *testing.T) {
	for _, lang := range l10n.Languages() {
		t.Run(string(lang), func(t *testing.T) {
			l := l10n.New(lang)
			r := speedLongModelReport()
			_, tt := speedPanel(t, func(p *store.Preferences) { p.Language = store.Language(lang) }, speedSnapshot(r), PanelActions{})
			basis, ok := tt.Find(tx(l, xSpeedBasis, 2))
			latest, ok2 := tt.Find(tx(l, xSpeedLatest, r.Sessions[0].Title))
			age, ok3 := tt.Find(runway.RelativePast(r.Sessions[0].UpdatedAt, time.Now(), lang, false))
			if !ok || !ok2 || !ok3 {
				t.Fatalf("attribution parts missing (%v %v %v); texts %q", ok, ok2, ok3, tt.Texts())
			}
			if latest.X < basis.X+basis.W || age.X < latest.X+latest.W || age.X+age.W > panelWidth-16+0.5 {
				t.Fatalf("attribution parts overlap or overflow: basis=%+v latest=%+v age=%+v", basis, latest, age)
			}
			// Fallback fonts can give the shortened title a slightly different
			// line height, so compare the centres of the parts.
			mid := func(r ui.Rect) float64 { return float64(r.Y + r.H/2) }
			if math.Abs(mid(basis)-mid(latest)) > 0.5 || math.Abs(mid(latest)-mid(age)) > 0.5 {
				t.Fatalf("attribution wraps: basis=%+v latest=%+v age=%+v", basis, latest, age)
			}
		})
	}
}

// The detail page fits the panel in every language, with a long model name
// and title, and its figure columns keep their order.
func TestPanelSpeedDetailLayoutInEveryLanguage(t *testing.T) {
	for _, lang := range l10n.Languages() {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%v", lang, dark), func(t *testing.T) {
				l := l10n.New(lang)
				a, tt := speedPanel(t, func(p *store.Preferences) { p.Language = store.Language(lang) }, speedSnapshot(speedLongModelReport()), PanelActions{})
				tt.SetDark(dark)
				a.panelState.detail = store.ModuleSpeedAnalysis
				tt.Frame()
				for _, text := range tt.Texts() {
					if r, _ := tt.Find(text); r.X < 0 || r.X+r.W > panelWidth {
						t.Fatalf("text %q exceeds the panel: %+v", text, r)
					}
				}
				p50, _ := tt.Find("P50")
				p90, _ := tt.Find("P90")
				out, _ := tt.Find("tok/s")
				calls, ok := tt.Find(tx(l, xSpeedCalls))
				if !ok || p50.X+p50.W > p90.X || p90.X+p90.W > out.X || out.X+out.W > calls.X {
					t.Fatalf("table head columns overlap: p50=%+v p90=%+v out=%+v calls=%+v", p50, p90, out, calls)
				}
				cell, _ := tt.Find("12.3s")
				share, _ := tt.Find("99")
				if share.Y != cell.Y || share.X < cell.X+cell.W {
					t.Fatalf("table row columns overlap: ttft=%+v calls=%+v", cell, share)
				}
				model, _ := tt.Find(tx(l, xSpeedModel))
				if model.X+model.W > p50.X {
					t.Fatalf("model head overlaps TTFT head: %+v %+v", model, p50)
				}
			})
		}
	}
}

// The module title and description are real text in every language.
func TestSpeedModuleTextsInEverySettingsLanguage(t *testing.T) {
	for _, lang := range l10n.Languages() {
		title, subtitle := moduleTexts(l10n.New(lang), store.ModuleSpeedAnalysis)
		if title == "" || subtitle == "" || title == string(xSpeedTitle) || subtitle == string(xSpeedModuleDescription) {
			t.Errorf("%s: speed module texts %q %q", lang, title, subtitle)
		}
	}
	if info := moduleInfoOf(store.ModuleSpeedAnalysis); !info.both || info.icon == nil || info.config {
		t.Errorf("speed module info = %+v", info)
	}
}

// Fixtures stay in tests; exporting never opens a native window or reads credentials.
func TestExportSpeedReview(t *testing.T) {
	if os.Getenv("RUNWAY_UI_QA") == "" {
		t.Skip("set RUNWAY_UI_QA to export speed analysis PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-ui-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range l10n.Languages() {
		for _, dark := range []bool{false, true} {
			theme := "light"
			if dark {
				theme = "dark"
			}
			a, tt := speedPanel(t, func(p *store.Preferences) { p.Language = store.Language(lang) }, speedSnapshot(speedLongModelReport()), PanelActions{})
			tt.SetSize(panelWidth, 430)
			tt.SetDark(dark)
			tt.Frame()
			exportReviewPNG(t, tt, filepath.Join(dir, fmt.Sprintf("speed-card-%s-%s.png", lang, theme)))
			a.panelState.detail = store.ModuleSpeedAnalysis
			tt.SetSize(panelWidth, 900)
			tt.Frame()
			exportReviewPNG(t, tt, filepath.Join(dir, fmt.Sprintf("speed-detail-%s-%s.png", lang, theme)))
		}
	}
}
