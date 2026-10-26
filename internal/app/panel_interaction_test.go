package app

import (
	"bytes"
	"image/png"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func TestPanelDetailNavigationAndProviderIsolation(t *testing.T) {
	snap := panelFixture()
	snap.Grok = runway.GrokState{Availability: runway.GrokReady, Plan: "SuperGrok", Accounts: runway.AccountsView{Display: codex.AccountDisplay{DisplayName: "Grok identity"}}}
	a, tt := newPanel(t, nil, snap, PanelActions{})
	a.SetPanelActions(PanelActions{SelectProvider: func(p store.Provider) { a.settings.SetSelectedProvider(p) }})
	tt.Frame()
	l := a.L()
	if err := tt.Click(l.Text(l10n.KeyAccounts)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panelState.detail != detailAccounts || !hasExact(tt, l.Text(l10n.KeyBack)) {
		t.Fatalf("detail did not open: %q", tt.Texts())
	}
	if err := tt.Click(l.Text(l10n.KeyBack)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panelState.detail != "" || !tt.HasText("Ada Lovelace") {
		t.Fatal("back did not restore panel")
	}
	if err := tt.Click(l.Text(l10n.KeyProviderGrok)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panelState.detail != "" || tt.HasText("Ada Lovelace") || tt.HasText("5-hour") || tt.HasText("70% left") || !tt.HasText("Grok identity") {
		t.Fatalf("provider leaked Codex state: %q", tt.Texts())
	}
	// Changes from the settings page must also clear an unsupported detail.
	if err := tt.Click(l.Text(l10n.KeyShowDetails)); err != nil {
		t.Fatal(err)
	}
	if a.panelState.detail != store.ModuleAPICost {
		t.Fatal("Grok cost detail did not open")
	}
	a.settings.SetSelectedProvider(store.ProviderCodex)
	tt.Frame()
	if a.panelState.detail != "" || tt.HasText("Grok identity") {
		t.Fatal("external provider change retained detail or identity")
	}
}

func TestPanelChartModesChangeActualSeries(t *testing.T) {
	heat := uiReviewSnapshot().Codex.Heatmap
	state := &panelState{}
	l := l10n.New(l10n.English)
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Children(func() {
			first := true
			panelToken(c, l, store.ChartStyleLine, runway.Snapshot{Provider: store.ProviderCodex, Codex: runway.CodexView{Heatmap: heat}}, PanelActions{}, &first, state)
		})
	}, 368, 280)
	var before, after bytes.Buffer
	if err := png.Encode(&before, tt.Image()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(l.Text(l10n.KeyHeatmapWeekly)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if state.mode() != usage.HeatmapWeekly {
		t.Fatal("weekly mode not applied")
	}
	if err := png.Encode(&after, tt.Image()); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before.Bytes(), after.Bytes()) {
		t.Fatal("weekly selection did not change rendered series")
	}
	if err := tt.Click(l.Text(l10n.KeyHeatmapMonthly)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if state.mode() != usage.HeatmapCumulative {
		t.Fatal("cumulative mode not applied")
	}
}

func TestPanelModuleSettingChangesVisiblePage(t *testing.T) {
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	a.settings.SetModuleShown(store.ModuleTokenUsage, false)
	tt.Frame()
	if hasExact(tt, a.L().Text(l10n.KeyTokenUsageHeatmap)) {
		t.Fatal("hidden token module still rendered")
	}
	if !hasExact(tt, a.L().Text(l10n.KeyQuota)) || !tt.HasText("5-hour") || !tt.HasText("70% left") {
		t.Fatal("hiding token module affected quota")
	}
}

func TestPanelPrivacyChangeUpdatesIdentity(t *testing.T) {
	a, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	a.settings.SetAccountPrivacyMode(true)
	tt.Frame()
	if tt.HasText("Ada Lovelace") || tt.HasText("ada@example.com") || !tt.HasText("A***") {
		t.Fatalf("privacy not applied: %q", tt.Texts())
	}
	if err := tt.Click(a.L().Text(l10n.KeyAccounts)); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Ada Lovelace") || tt.HasText("ada@example.com") || !tt.HasText("a***@***.com") || !tt.HasText("A***") {
		t.Fatal("account detail exposed identity or failed to show masked email")
	}
}

func TestAccountAliasButtonOpensEditorAndCancelClearsSensitiveDraft(t *testing.T) {
	s := NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, nil)
	s.prefs = store.DefaultPreferences()
	s.prefs.Language = store.LanguageEnglish
	s.applyLocale()
	u := newSettingsUI(s, SettingsOptions{})
	f := accountFormOf(u)
	rt := &liveApp{svc: runway.New(runway.Options{Preferences: s.Prefs()})}
	t.Cleanup(rt.svc.Close)
	account := codex.ManagedAccount{ID: "fixture", DisplayName: "Private account", Email: "private@example.com", Alias: "Personal"}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(16).Children(func() { accountEditor(c, u, f); renderCodexRow(c, u, f, rt, account, true) })
	}, 620, 320)
	if tt.HasText("Private account") || tt.HasText("private@example.com") {
		t.Fatal("account identity was not masked")
	}
	if err := tt.Click(u.L().Text(l10n.KeyAlias)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.mode != "alias" || f.editID != "fixture" || f.alias != "Personal" || !hasExact(tt, u.L().Text(l10n.KeyCancel)) {
		t.Fatalf("alias editor did not open: %#v", f)
	}
	f.paste = "sensitive draft"
	f.api.key = "sensitive key"
	if err := tt.Click(u.L().Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if f.mode != "" || f.paste != "" || f.api.key != "" {
		t.Fatal("cancel retained sensitive draft")
	}
}

func TestPanelBackgroundOnlyUsesAvailableMaterial(t *testing.T) {
	var got ui.Color
	style := store.PanelBackgroundTranslucent
	tt := ui.NewTester(func(c *ui.Context) {
		got = applyPanelTheme(c, style, "", "").Background
		ui.Box(c).Fill().Background(got)
	}, 100, 100)
	tt.SetVibrancy(true)
	if got != ui.Transparent {
		t.Fatal("translucent material was painted opaque")
	}
	if px := tt.Image().RGBAAt(50, 50); px.A != 0 {
		t.Fatalf("material canvas center alpha=%d, want 0", px.A)
	}
	style = store.PanelBackgroundOpaque
	tt.Frame()
	if got == ui.Transparent {
		t.Fatal("opaque preference did not paint a background")
	}
	if px := tt.Image().RGBAAt(50, 50); px.A != 255 {
		t.Fatalf("opaque canvas alpha=%d, want 255", px.A)
	}
	style = store.PanelBackgroundTranslucent
	tt.SetVibrancy(false)
	if got == ui.Transparent {
		t.Fatal("unsupported material rendered a transparent window")
	}
}

func TestLineAndBarDrawDifferentMarks(t *testing.T) {
	l := l10n.New(l10n.English)
	at := uiReviewSnapshot().Codex.Heatmap.CalculatedAt
	points := []usage.ChartPoint{{DayKey: "2026-10-10", Tokens: 10}, {DayKey: "2026-10-11", Tokens: 40}, {DayKey: "2026-10-12", Tokens: 20}}
	render := func(bar bool) []byte {
		tt := ui.NewTester(func(c *ui.Context) {
			applyTheme(c)
			panelTrend(c, l, usage.ChartSeries{Points: points, CalculatedAt: at}, bar, store.ProviderCodex)
		}, 368, 130)
		var b bytes.Buffer
		if err := png.Encode(&b, tt.Image()); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	if bytes.Equal(render(true), render(false)) {
		t.Fatal("line and bar produced identical images")
	}
}

func TestViewsKeepTranslatedControlsWithinWindow(t *testing.T) {
	for _, lang := range l10n.Languages() {
		t.Run(string(lang), func(t *testing.T) {
			a, tt := newPanel(t, func(p *store.Preferences) { p.Language = store.Language(lang) }, panelFixture(), PanelActions{})
			tt.SetSize(400, 584)
			// Header actions must remain inside the compact native panel in every language.
			for _, key := range []l10n.Key{l10n.KeyRefresh, l10n.KeySettings, l10n.KeyQuit} {
				r, ok := tt.Find(a.L().Text(key))
				if !ok || r.X < 0 || r.X+r.W > 400 {
					t.Fatalf("toolbar %s overflow: %+v", key, r)
				}
			}
			u := newSettingsUI(a.settings, SettingsOptions{})
			settings := ui.NewTester(u.view, settingsWidth, settingsHeight)
			for _, tab := range settingsTabs {
				r, ok := settings.Find(tab.title(u.L()))
				if !ok || r.X < 0 || r.X+r.W > settingsWidth {
					t.Fatalf("settings tab %s overflow: %+v", tab.title(u.L()), r)
				}
			}
		})
	}
}

func TestPanelGrokUsesEnabledSupportedModules(t *testing.T) {
	snap := uiReviewDetails(panelFixture())
	snap.Grok = runway.GrokState{Availability: runway.GrokReady, Plan: "SuperGrok", Heatmap: snap.Codex.Heatmap, Cost: snap.Codex.DisplayedCost, ResetCredits: snap.Codex.ResetCredits, ResetCreditSummary: snap.Codex.ResetCreditSummary}
	a, tt := newPanel(t, func(p *store.Preferences) {
		p.SelectedProvider = store.ProviderGrok
		p.ShowsRecentSessions = true
		p.APICostSummaryRange = store.CostRangeToday
	}, snap, PanelActions{})
	for _, key := range []l10n.Key{l10n.KeyTokenUsageHeatmap, l10n.KeyApiCost, l10n.KeyResetCredits, l10n.KeyRecentSessions} {
		if !hasExact(tt, a.L().Text(key)) {
			t.Fatalf("Grok module %s missing", key)
		}
	}
	if hasExact(tt, a.L().Text(l10n.KeyQuotaEstimate)) || hasExact(tt, a.L().Text(l10n.KeySessionRepair)) {
		t.Fatal("unsupported Codex modules shown on Grok")
	}
	adapted := providerView(a.L(), runway.Snapshot{Provider: store.ProviderGrok, Grok: snap.Grok}, store.CostRangeToday)
	if adapted.DisplayedRange != store.CostRangeToday {
		t.Fatal("Grok cost ignored selected range")
	}
	if !tt.HasText(a.L().Text(l10n.KeyToday)) {
		t.Fatal("selected Grok cost range was not rendered")
	}
	a.settings.SetModuleShown(store.ModuleTokenUsage, false)
	tt.Frame()
	if hasExact(tt, a.L().Text(l10n.KeyTokenUsageHeatmap)) {
		t.Fatal("Grok module ignored visibility setting")
	}
}

func TestResetEvidenceOpensSnapshotURL(t *testing.T) {
	l := l10n.New(l10n.English)
	now := time.Now()
	source := resettoday.NewStateSnapshot(resettoday.StateYes, now)
	source.Events = []resettoday.Event{{Kind: resettoday.KindResetCompleted, ResetType: resettoday.ResetGlobal, AnnouncedAt: now}}
	rc := newResetCard(l, runway.ResetTodayState{Text: "Reset evidence", Detail: source}, false)
	url, line := rc.evidenceURL(), rc.evidenceLine()
	if url == "" || line == "" {
		t.Fatal("typed source has no evidence URL or line")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Children(func() { rc.evidence(c) })
	}, 368, 240)
	if err := tt.Click(line); err != nil {
		t.Fatal(err)
	}
	if got := tt.OpenedURLs(); len(got) != 1 || got[0] != url {
		t.Fatalf("evidence opened %q, want %q", got, url)
	}
}

func TestChartHoverTipInkStaysOpaqueOverMaterial(t *testing.T) {
	const detail = "Oct 12 · 1.2M tokens"
	var fill ui.Color
	tt := ui.NewTester(func(c *ui.Context) {
		th := applyPanelTheme(c, store.PanelBackgroundTranslucent, "", "")
		fill, _ = inverseColors(th)
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			chartHoverTip(c, ui.Box(c).Size(12, 12).Background(th.Accent), detail)
		})
	}, 240, 120)
	tt.SetVibrancy(true)
	tt.Move(120, 60)
	tt.Frame()
	r, ok := tt.Find(detail)
	if !ok {
		t.Fatalf("tooltip not shown: %q", tt.Texts())
	}
	img := tt.Image()
	scale := float32(img.Bounds().Dx()) / 240
	inked := 0
	for y := int(r.Y * scale); y < int((r.Y+r.H)*scale); y++ {
		for x := int(r.X * scale); x < int((r.X+r.W)*scale); x++ {
			px := img.RGBAAt(x, y)
			if px.A == 255 && colorDistance(px.R, px.G, px.B, fill) > 96 {
				inked++
			}
		}
	}
	if inked == 0 {
		t.Fatal("tooltip text is invisible over native material")
	}
}

func colorDistance(r, g, b uint8, c ui.Color) int {
	d := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	return d(r, c.R) + d(g, c.G) + d(b, c.B)
}
