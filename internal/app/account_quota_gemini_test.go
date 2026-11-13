package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// geminiQuotaFixture is one Pro account with a 5-hour and a weekly Gemini
// window and a credit balance, read through the bridge like the live app.
func geminiQuotaFixture(t *testing.T, l l10n.Localizer, now time.Time) (codex.ManagedAccount, runway.GeminiState) {
	t.Helper()
	mod := geminiTestModule(t)
	short, long := now.Add(2*time.Hour), now.Add(4*24*time.Hour)
	snap := gemini.QuotaSnapshot{
		Tier: gemini.Tier{Kind: gemini.TierPro},
		Buckets: []gemini.QuotaBucket{
			{ID: "g5", Family: gemini.FamilyGemini, Window: gemini.WindowFiveHour, RemainingFraction: 0.51, ResetAt: &short},
			{ID: "gw", Family: gemini.FamilyGemini, Window: gemini.WindowWeekly, RemainingFraction: 0.39, ResetAt: &long},
		},
		Credits:   []gemini.Credit{{Type: "GOOGLE_ONE_AI", Amount: 25}},
		UpdatedAt: now,
	}
	saveGeminiAccounts(t, mod, "one", gemini.ManagedAccount{
		ID: "one", Identity: gemini.Identity{Subject: "1", Email: "personal@example.com", Name: "Personal"}, LastQuotaAt: &now,
		CachedQuota: geminiSnapshotJSON(t, snap),
	})
	st := buildGeminiState(mod, l, nil)
	if len(st.Accounts.Sidebar) != 1 {
		t.Fatalf("sidebar: %+v", st.Accounts.Sidebar)
	}
	return st.Accounts.Sidebar[0], st
}

func TestGeminiAccountRowsShowQuotaAndControls(t *testing.T) {
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageTraditionalChinese, store.LanguageFrench, store.LanguageRussian, store.LanguageJapanese, store.LanguageKorean} {
		for _, panel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/panel=%t", lang, panel), func(t *testing.T) {
				u := cardSettingsUI(t, lang)
				l := u.L()
				a, _ := newPanel(t, nil, runway.Snapshot{}, PanelActions{})
				var acc codex.ManagedAccount
				var st runway.GeminiState
				var rt *liveApp
				width := settingsMinWidth - settingsSidebarWidth
				if panel {
					width = 320
				}
				tt := ui.NewTester(func(c *ui.Context) {
					applyTheme(c)
					if acc.ID == "" {
						acc, st = geminiQuotaFixture(t, l, c.Now())
						p := store.DefaultPreferences()
						p.ShowsQuotaEstimateSummary = false
						svc := runway.New(runway.Options{Preferences: p, Now: c.Now, Gemini: &geminiBridge{state: st}})
						svc.SetPreferences(p)
						t.Cleanup(svc.Close)
						rt = &liveApp{svc: svc}
					}
					ui.Column(c).Fill().Padding(16).Children(func() {
						if panel {
							a.geminiAccountCard(c, l, store.DefaultPreferences(), runway.Snapshot{Gemini: st}, acc, AccountActions{})
							return
						}
						renderGeminiAccountRow(c, u, &accountForm{}, rt, acc, rowPlace{}, "one", false)
					})
				}, width, 700)
				for _, want := range []string{"51% " + l.Text(l10n.KeyLeft), "39% " + l.Text(l10n.KeyLeft), "25", l.Text(l10n.KeyRefresh)} {
					if !hasExact(tt, want) {
						t.Fatalf("missing %q in %q", want, tt.Texts())
					}
				}
				if !panel {
					if !tt.HasText(accountQuotaUpdatedText(l, tt.Now())) {
						t.Fatalf("missing header updated time: %q", tt.Texts())
					}
					if _, ok := tt.Find(l.Text(l10n.KeyGeminiCurrentAccountCannotDelete)); !ok {
						t.Fatalf("the current account must explain why it cannot be removed: %q", tt.Texts())
					}
				}
				for _, s := range tt.Texts() {
					if r, _ := tt.Find(s); r.X < 0 || r.X+r.W > float32(width)+0.5 {
						t.Fatalf("%q exceeds width %d: %v", s, width, r)
					}
				}
			})
		}
	}
}
