package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/cachehit"
	"github.com/licoy/quosparr/internal/cursor"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestCursorPanelIsQuotaOnlyAndEnablesExplicitly(t *testing.T) {
	l := l10n.New(l10n.English)
	enabled := false
	queries := 0
	refreshes := 0
	a, tt := newPanel(t, func(p *store.Preferences) { p.SelectedProvider = store.ProviderCursor }, panelFixture(), PanelActions{
		SetCursorMonitoring: func(v bool) { enabled = v },
		Refresh:             func() { t.Error("Cursor invoked all-provider refresh") },
		RefreshSection: func(section runway.Section) {
			if section != runway.SectionQuota {
				t.Error("Cursor requested a non-quota refresh")
			}
			refreshes++
		},
		QueryCache: func(context.Context, store.Provider, CostQuery) (cachehit.Report, error) {
			queries++
			return cachehit.Report{}, nil
		},
	})
	for _, text := range []string{"Ada Lovelace", l.Text(l10n.KeySessionRepair), "quota-headline"} {
		if tt.HasText(text) {
			t.Fatal("Cursor rendered another provider's module")
		}
	}
	if !tt.HasText(l.Text(l10n.KeyCursorReadOnlyHint)) {
		t.Fatal("opt-in explanation missing")
	}
	if err := tt.Click(l.Text(l10n.KeyCursorEnable)); err != nil {
		t.Fatal(err)
	}
	if !enabled || queries != 0 {
		t.Fatal("opt-in or quota-only boundary failed")
	}
	if a.panelState.detail != "" {
		t.Fatal("Cursor opened account management")
	}
	refreshEverything(a, a.PanelActions())
	if queries != 0 || refreshes != 1 {
		t.Fatal("header refresh crossed the quota-only boundary")
	}
}

func TestCursorPanelFourTabsFitAndUsageLinkWorks(t *testing.T) {
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageTraditionalChinese, store.LanguageKorean, store.LanguageJapanese, store.LanguageRussian, store.LanguageFrench} {
		for _, dark := range []bool{false, true} {
			snap := panelFixture()
			now := time.Now()
			snap.Cursor = runway.CursorState{Availability: runway.CursorReady, Plan: "test plan", Email: "tester@example.invalid", UpdatedAt: now}
			auto, api, used, limit, extra := 25.0, 67.0, int64(1340), int64(2000), int64(250)
			cursorQuotaView(&snap.Cursor, cursor.Snapshot{
				Pools: []cursor.Pool{
					{Kind: "auto", UsedPercent: &auto, ResetsAt: now.Add(48 * time.Hour)},
					{Kind: "api", UsedPercent: &api, UsedCents: &used, LimitCents: &limit, ResetsAt: now.Add(48 * time.Hour)},
				},
				OnDemandCents: &extra,
				Credits:       []cursor.Credit{{RemainingCents: 500, ExpiresAt: now.Add(7 * 24 * time.Hour)}},
			}, l10n.ForPreference(string(lang), nil), now)
			a, tt := newPanel(t, func(p *store.Preferences) {
				p.SelectedProvider = store.ProviderCursor
				p.CursorMonitoringEnabled = true
				p.Language = lang
			}, snap, PanelActions{})
			tt.SetDark(dark)
			for _, p := range store.Providers() {
				label := a.L().Text(providerKey(p))
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.X < 0 || r.X+r.W > panelWidth {
					t.Fatalf("provider tab out of bounds for %s", lang)
				}
			}
			if err := tt.Click(a.L().Text(l10n.KeyCursorOpenUsage)); err != nil {
				t.Fatal(err)
			}
			if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != "https://cursor.com/dashboard?tab=usage" {
				t.Fatal("usage link missing")
			}
			if dir := os.Getenv("QUOSPARR_DEV_CURSOR_QA_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				exportReviewPNG(t, tt, filepath.Join(dir, string(lang)+map[bool]string{false: "-light.png", true: "-dark.png"}[dark]))
			}
		}
	}
}

func TestCursorDoesNotEnterCLIManagementTabs(t *testing.T) {
	if len(store.Providers()) != 4 || len(store.CLIProviders()) != 3 || skillsTrashTab != len(store.CLIProviders()) {
		t.Fatal("management providers conflict with trash")
	}
	for _, p := range store.CLIProviders() {
		if p == store.ProviderCursor {
			t.Fatal("quota-only provider entered CLI management")
		}
	}
	l := l10n.New(l10n.English)
	tt := ui.NewTester(func(c *ui.Context) { selected := 0; ProviderTabs(c, &selected, false, providerTabLabels(l)...) }, 400, 60)
	if tt.HasText("Cursor") {
		t.Fatal("CLI management offers Cursor")
	}
}

func TestCursorAccountTabShowsReadOnlyIdentityAndRefresh(t *testing.T) {
	u := cardSettingsUI(t, store.LanguageEnglish)
	f := accountFormOf(u)
	f.mode, f.paste = "paste", "discarded input"
	u.s.SetCursorMonitoringEnabled(true)
	u.s.SetAccountPrivacyMode(true)
	s := runway.CursorState{
		Availability: runway.CursorReady, Email: "tester@example.invalid", Plan: "Team",
		Meters: []runway.Meter{{Title: "Auto", RemainingPercent: 75}},
	}
	refreshes := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(16).Gap(14).Children(func() {
			accountProviderHeader(c, u, f)
			if f.kind == accountKindCursor {
				renderCursorAccount(c, u, s, func() { refreshes++ })
			}
		})
	}, settingsMinWidth-settingsSidebarWidth, 640)
	if err := tt.Click("Cursor"); err != nil {
		t.Fatal(err)
	}
	if f.kind != accountKindCursor || f.mode != "" || f.paste != "" {
		t.Fatal("Cursor tab did not open or retained another provider's editor")
	}
	if tt.HasText(s.Email) || !tt.HasText(accountPrivate(true, s.Email)) || !tt.HasText(s.Plan) || !tt.HasText("Auto · 75%") {
		t.Fatal("current Cursor identity, privacy or quota missing")
	}
	for _, key := range []l10n.Key{l10n.KeyAccountsActions, l10n.KeyAccountsMakeCurrent, l10n.KeyAccountsAddOAuth} {
		if tt.HasText(u.text(key)) {
			t.Fatal("Cursor offered unsupported account actions")
		}
	}
	if err := tt.Click(u.text(l10n.KeyRefresh)); err != nil || refreshes != 1 {
		t.Fatal("account refresh did not reach Cursor")
	}
	s.LastError, s.Stale = u.text(l10n.KeyCursorReauth), true
	tt.Frame()
	if !tt.HasText(s.LastError) || tt.HasText("Auto · 75%") {
		t.Fatal("stale quota hid the sign-in failure")
	}
	u.s.SetCursorMonitoringEnabled(false)
	tt.Frame()
	if tt.HasText(accountPrivate(true, s.Email)) || !tt.HasText(u.text(l10n.KeyCursorReadOnlyHint)) {
		t.Fatal("disabled monitoring still showed the cached account")
	}
}

func TestCursorIdentityObeysPrivacyAndCannotOpenAccountManagement(t *testing.T) {
	snap := panelFixture()
	snap.Cursor = runway.CursorState{Availability: runway.CursorReady, Email: "tester@example.com", Plan: "test plan"}
	a, tt := newPanel(t, func(p *store.Preferences) {
		p.SelectedProvider = store.ProviderCursor
		p.CursorMonitoringEnabled = true
		p.AccountPrivacyModeEnabled = true
	}, snap, PanelActions{})
	if tt.HasText(snap.Cursor.Email) || !tt.HasText(accountPrivate(true, snap.Cursor.Email)) {
		t.Fatal("Cursor email ignored privacy mode")
	}
	if r, ok := tt.Find(accountPrivate(true, snap.Cursor.Email)); ok {
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	if a.panelState.detail != "" {
		t.Fatal("read-only Cursor identity opened account management")
	}
}
