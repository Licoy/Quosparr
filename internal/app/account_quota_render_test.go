package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func quotaAccountRowTester(t *testing.T, lang store.Language, provider store.Provider, width int, panel bool) (*ui.Tester, l10n.Localizer) {
	t.Helper()
	u := cardSettingsUI(t, lang)
	l := u.L()
	a, _ := newPanel(t, nil, runway.Snapshot{}, PanelActions{})
	var acc codex.ManagedAccount
	var cq runway.ClaudeAccountQuota
	var gq runway.GrokAccountQuota
	var rt *liveApp
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		if acc.ID == "" {
			acc, cq, gq = quotaAccountFixture(t, l, c.Now())
			acc.DisplayName = "Personal workspace account with a long name"
			p := store.DefaultPreferences()
			p.ShowsQuotaEstimateSummary = false
			svc := runway.New(runway.Options{Preferences: p, Now: c.Now, Claude: &claudeBridge{state: runway.ClaudeState{AccountQuotas: map[string]runway.ClaudeAccountQuota{"one": cq}}}, Grok: &grokBridge{state: runway.GrokState{AccountQuotas: map[string]runway.GrokAccountQuota{"one": gq}}}})
			svc.SetPreferences(p)
			t.Cleanup(svc.Close)
			rt = &liveApp{svc: svc}
		}
		ui.Column(c).Fill().Padding(16).Children(func() {
			if !panel {
				f := &accountForm{}
				switch provider {
				case store.ProviderCodex:
					renderCodexSettingsRow(c, u, f, nil, acc, rowPlace{}, "one", false)
				case store.ProviderClaude:
					renderClaudeAccountRow(c, u, f, rt, acc, rowPlace{}, "one", false)
				case store.ProviderGrok:
					renderGrokAccountRow(c, u, f, rt, acc, rowPlace{}, "one", false)
				}
				return
			}
			snap := runway.Snapshot{Accounts: runway.AccountsView{ActiveID: "one"}, Claude: runway.ClaudeState{AccountQuotas: map[string]runway.ClaudeAccountQuota{"one": cq}}, Grok: runway.GrokState{AccountQuotas: map[string]runway.GrokAccountQuota{"one": gq}}}
			switch provider {
			case store.ProviderCodex:
				a.codexAccountCard(c, l, store.DefaultPreferences(), snap.Accounts, acc, AccountActions{})
			case store.ProviderClaude:
				a.claudeAccountCard(c, l, store.DefaultPreferences(), snap, acc, AccountActions{})
			case store.ProviderGrok:
				a.grokAccountCard(c, l, store.DefaultPreferences(), snap, acc, grokCapabilities{}, AccountActions{})
			}
		})
	}, width, 700)
	return tt, l
}

func TestSettingsAccountUpdateControlsStayCentered(t *testing.T) {
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageTraditionalChinese, store.LanguageFrench, store.LanguageRussian, store.LanguageJapanese, store.LanguageKorean} {
		for _, provider := range store.Providers() {
			for _, width := range []int{settingsMinWidth - settingsSidebarWidth, settingsWidth - settingsSidebarWidth} {
				t.Run(fmt.Sprintf("%s/%s/%d", lang, provider, width), func(t *testing.T) {
					tt, l := quotaAccountRowTester(t, lang, provider, width, false)
					text := accountQuotaUpdatedText(l, tt.Now())
					updated, ok := tt.Find(text)
					if !ok {
						t.Fatalf("missing header updated time: %q", tt.Texts())
					}
					refresh, ok := tt.Find(l.Text(l10n.KeyRefresh))
					if !ok {
						t.Fatal("missing refresh button")
					}
					actions, ok := tt.Find(accountRemoveLabel(l, provider))
					if !ok {
						t.Fatalf("missing remove button: %q", tt.Texts())
					}
					if updated.X+updated.W > refresh.X || refresh.X+refresh.W > actions.X {
						t.Fatalf("update/refresh/actions order: %v %v %v", updated, refresh, actions)
					}
					for _, r := range []ui.Rect{updated, refresh} {
						if math.Abs(float64(r.Y+r.H/2-actions.Y-actions.H/2)) > 1 {
							t.Fatalf("controls not centered: %v %v %v", updated, refresh, actions)
						}
					}
					count := 0
					for _, s := range tt.Texts() {
						if s == text {
							count++
						}
						r, _ := tt.Find(s)
						if r.X < 0 || r.X+r.W > float32(width)+0.5 {
							t.Fatalf("%q exceeds settings width: %v", s, r)
						}
					}
					if count != 1 {
						t.Fatalf("updated time repeated %d times", count)
					}
					for _, s := range tt.Texts() {
						if strings.HasSuffix(s, " "+l.Text(l10n.KeyLeft)) {
							r, _ := tt.Find(s)
							if r.Y < actions.Y+actions.H {
								t.Fatalf("quota rises into the header: %v %v", actions, r)
							}
						}
					}
					if !tt.HasText(accountResetExpiryText(l, tt.Now().Add(2*time.Hour))) {
						t.Fatalf("missing exact reset-card expiry: %q", tt.Texts())
					}
				})
			}
		}
	}
}

// accountRemoveLabel is the last icon button of a settings account row.
func accountRemoveLabel(l l10n.Localizer, provider store.Provider) string {
	if provider == store.ProviderGrok {
		return l.Text(l10n.KeyGrokAccountsRemove)
	}
	return l.Text(l10n.KeyAccountsDelete)
}

func TestExportAccountQuotaReview(t *testing.T) {
	dir := os.Getenv("QUOSPARR_ACCOUNT_QUOTA_QA")
	if dir == "" {
		t.Skip("set QUOSPARR_ACCOUNT_QUOTA_QA to export synthetic account previews")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		for _, dark := range []bool{false, true} {
			for _, provider := range store.Providers() {
				for _, panel := range []bool{false, true} {
					width := settingsMinWidth - settingsSidebarWidth
					if panel {
						width = 320
					}
					tt, _ := quotaAccountRowTester(t, lang, provider, width, panel)
					tt.SetDark(dark)
					exportReviewPNG(t, tt, filepath.Join(dir, fmt.Sprintf("%s-%s-panel-%t-dark-%t.png", provider, lang, panel, dark)))
				}
			}
		}
	}
}
