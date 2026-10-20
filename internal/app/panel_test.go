package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func newPanel(t *testing.T, mutate func(*store.Preferences), snap runway.Snapshot, acts PanelActions) (*App, *ui.Tester) {
	t.Helper()
	s := NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, nil)
	s.prefs = store.DefaultPreferences()
	s.prefs.Language = store.LanguageEnglish
	if mutate != nil {
		mutate(&s.prefs)
	}
	s.prefs.Normalize()
	s.applyLocale()
	a := New(Options{Version: "test", Settings: s})
	a.snapMu.Lock()
	a.snap = snap
	a.snapMu.Unlock()
	a.SetPanelActions(acts)
	// Tall enough that every module of the default panel is on screen, so a
	// click on a card row never lands on the footer below the fold.
	return a, ui.NewTester(a.panelView, panelWidth, 1600)
}

func panelFixture() runway.Snapshot {
	day := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	return runway.Snapshot{
		Accounts: runway.AccountsView{
			ActiveID: "ada",
			Sidebar: []codex.ManagedAccount{{
				ID: "ada", DisplayName: "Ada Lovelace", Email: "ada@example.com",
			}},
		},
		Codex: runway.CodexView{
			QuotaText: "quota-headline",
			Meters: []runway.Meter{{
				Title: "5-hour", RemainingPercent: 70, UsedPercent: 30, Health: runway.HealthGreen,
			}},
			Heatmap: runway.HeatmapData{
				AllDevices:   map[string]int64{"2026-10-11": 20, "2026-10-12": 80},
				CalculatedAt: day,
			},
		},
		ResetToday: runway.ResetTodayState{Text: "reset-today-headline"},
	}
}

func hasExact(tt *ui.Tester, s string) bool {
	for _, text := range tt.Texts() {
		if text == s {
			return true
		}
	}
	return false
}

func TestPanelRendersQuotaAccountAndResetToday(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	if !hasExact(tt, l.Text(l10n.KeyQuota)) {
		t.Fatalf("quota title missing; texts %q", tt.Texts())
	}
	if !tt.HasText("Ada Lovelace") {
		t.Fatalf("account label missing; texts %q", tt.Texts())
	}
	if !tt.HasText(l.Text(l10n.KeyRateLimitResetTodayUnavailableHint)) {
		t.Fatalf("reset-today text missing; texts %q", tt.Texts())
	}
}

func TestPanelHidesQuotaWhenPreferenceOff(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt := newPanel(t, func(p *store.Preferences) {
		p.ShowsQuotaSummary = false
	}, panelFixture(), PanelActions{})
	if hasExact(tt, l.Text(l10n.KeyQuota)) || tt.HasText("5-hour") || tt.HasText("70% left") {
		t.Fatalf("quota section still shown; texts %q", tt.Texts())
	}
	if !tt.HasText(l.Text(l10n.KeyRateLimitResetTodayUnavailableHint)) {
		t.Fatal("hiding quota removed the rest of the panel")
	}
}

func TestPanelGrokShowsPlanNotCodexQuota(t *testing.T) {
	snap := panelFixture()
	limit, used := int64(15000), int64(277)
	snap.Grok = runway.GrokState{Availability: runway.GrokReady, Plan: "SuperGrok", Meters: []runway.Meter{{
		Title: "weekly", RemainingPercent: 40, Health: runway.HealthYellow,
	}}, Detail: &grok.BillingSnapshot{
		IncludedLimitCents: &limit, IncludedUsedCents: &used,
		ResetCredits: &grok.ResetCreditSnapshot{AvailableCount: 3},
	}, ResetCreditSummary: &runway.ResetCreditSummary{AvailableCount: 3, StableAvailableCount: 3, TotalCount: 3}}
	_, tt := newPanel(t, func(p *store.Preferences) {
		p.SelectedProvider = store.ProviderGrok
	}, snap, PanelActions{})
	if !tt.HasText("SuperGrok") {
		t.Fatalf("Grok plan missing; texts %q", tt.Texts())
	}
	if tt.HasText("5-hour") || tt.HasText("70% left") {
		t.Fatal("Codex quota text is shown on the Grok panel")
	}
	if !hasExact(tt, "40% left") || !hasExact(tt, "3") || !hasExact(tt, "Available / 3 Total") {
		t.Fatalf("Grok allowance or reset credits missing; texts %q", tt.Texts())
	}
	if err := tt.Click(l10n.New(l10n.English).Text(l10n.KeyGrokBillingPeriod)); err != nil {
		t.Fatal(err)
	}
	if !hasExact(tt, "$2.77 / $150.00") {
		t.Fatal("Grok billing details lost spent/limit values")
	}
}

func TestPanelPrivacyMasksAccountEmail(t *testing.T) {
	a, tt := newPanel(t, func(p *store.Preferences) {
		p.AccountPrivacyModeEnabled = true
	}, panelFixture(), PanelActions{})
	if !hasExact(tt, "A***") || tt.HasText("Ada Lovelace") {
		t.Fatal("main identity is not masked")
	}
	if err := tt.Click(a.L().Text(l10n.KeyAccounts)); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("ada@example.com") {
		t.Fatal("privacy mode left the email visible")
	}
	if !tt.HasText("a***@***.com") {
		t.Fatalf("masked email missing; texts %q", tt.Texts())
	}
}

func TestPanelRefreshClick(t *testing.T) {
	l := l10n.New(l10n.English)
	calls := 0
	_, tt := newPanel(t, nil, panelFixture(), PanelActions{Refresh: func() { calls++ }})
	if err := tt.Click(l.Text(l10n.KeyRefresh)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("refresh calls = %d, want 1", calls)
	}
}

func TestPanelNilRefreshStaysDisabled(t *testing.T) {
	l := l10n.New(l10n.English)
	_, tt := newPanel(t, nil, panelFixture(), PanelActions{})
	if err := tt.Click(l.Text(l10n.KeyRefresh)); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(joined, "5-hour") || !strings.Contains(joined, "70% left") {
		t.Fatal("panel did not render")
	}
}
