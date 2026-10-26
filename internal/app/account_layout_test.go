package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

func TestDisplayAccountPrivacyPersistsAndRespectsReadOnly(t *testing.T) {
	for _, readonly := range []bool{false, true} {
		t.Run(fmt.Sprint(readonly), func(t *testing.T) {
			e := newEnv(t, func(d *failDomain) {
				if readonly {
					d.Put(store.KeyPreferences, []byte("{broken"))
				}
			})
			u, tt := openSettingsView(t, e, tabDisplay)
			l := u.L()
			if !hasExact(tt, tx(l, xPrivacyDisplaySection)) || !hasExact(tt, tx(l, xPrivacyDisplayDescription)) {
				t.Fatal("display privacy card or description missing")
			}
			clickAccountPrivacySwitch(t, u, tt)
			e.settle()
			if readonly {
				if e.s.Prefs().AccountPrivacyModeEnabled || e.mem.Writes() != 0 {
					t.Fatal("read-only privacy switch changed settings")
				}
				return
			}
			if !e.stored().AccountPrivacyModeEnabled {
				t.Fatal("privacy enable was not persisted")
			}
			reloaded := NewSettings(e.st, e.sys.system(), e.creds)
			if err := reloaded.Load(); err != nil || !reloaded.Prefs().AccountPrivacyModeEnabled {
				t.Fatalf("privacy preference did not survive reload: %v", err)
			}
			clickAccountPrivacySwitch(t, u, tt)
			e.settle()
			if e.stored().AccountPrivacyModeEnabled {
				t.Fatal("privacy disable was not persisted")
			}
		})
	}
}

// All account previews use public synthetic metadata, never the live runtime.
func accountLayoutFixture(t *testing.T, language store.Language, kind, width int, privacy bool, rows []codex.ManagedAccount) (*settingsUI, *ui.Tester) {
	t.Helper()
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	e.s.SetLanguage(language)
	e.s.SetAccountPrivacyMode(privacy)
	e.settle()
	u := newSettingsUI(e.s, SettingsOptions{Version: "1.0.0"})
	u.tab = tabAccounts
	f := accountFormOf(u)
	f.kind = kind
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		u.shell(c, func(c *ui.Context) {
			ui.Scroll(c).Grow(1).Padding(16).Gap(14).Children(func() {
				if u.tab == tabDisplay {
					u.displayPage(c)
					return
				}
				accountProviderHeader(c, u, f)
				accountToolbar(c, u, f, accountCountTitle(u.L(), len(rows)), accountFolder{u.text(l10n.KeyCodexFolder), "/codex"}, func(*ui.Menu) {}, func() string { return "" }, func() string { return "" })
				accountEditor(c, u, f)
				accountSelectionEditor(c, u, f, nil)
				for _, account := range rows {
					if f.kind == accountKindCodex {
						renderCodexSettingsRow(c, u, f, nil, account, "current", e.s.Prefs().AccountPrivacyModeEnabled)
					} else {
						renderGrokAccountRow(c, u, f, nil, account, "current", e.s.Prefs().AccountPrivacyModeEnabled)
					}
				}
			})
		})
	}, width, settingsHeight)
	tt.SetTitleBar(ui.TitleBar{Height: 40, Left: 80})
	return u, tt
}

func accountLayoutRows() []codex.ManagedAccount {
	weekly := 10080
	return []codex.ManagedAccount{
		{ID: "current", DisplayName: "Workspace account", Email: "workspace-with-a-long-email-address@example.com", CachedQuota: &codex.CachedAccountQuota{PrimaryUsedPercent: 32, PrimaryWindowMinutes: &weekly}},
		{ID: "secondary", DisplayName: "Personal account", Email: "personal@example.com"},
	}
}

func TestAccountRowsKeepCurrentTagByNameAndMaskIdentity(t *testing.T) {
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			for _, kind := range []int{accountKindCodex, accountKindGrok} {
				t.Run(fmt.Sprintf("%s/%d/%d", language, width, kind), func(t *testing.T) {
					var actionX, refreshX float32
					for i, account := range accountLayoutRows() {
						u, tt := accountLayoutFixture(t, language, kind, width, false, []codex.ManagedAccount{account})
						actions, _ := tt.Find(u.L().Text(l10n.KeyAccountsActions))
						refresh, _ := tt.Find(u.L().Text(l10n.KeyRefresh))
						if i == 0 {
							actionX, refreshX = actions.X, refresh.X
							current, ok := tt.Find(u.L().Text(l10n.KeyAccountsCurrent))
							name, _ := tt.Find(account.DisplayName)
							gap := current.X - name.X - name.W
							baseline := current.Y + current.H/2 - name.Y - name.H/2
							if !ok || gap < 5 || gap > 7 || baseline < -1 || baseline > 1 {
								t.Fatalf("current tag must immediately follow account name: name=%+v current=%+v", name, current)
							}
							if kind == accountKindCodex {
								quota, found := tt.Find(u.L().Text(l10n.KeyWeeklyUsage))
								if !found || quota.Y <= name.Y+name.H || quota.X != name.X {
									t.Fatalf("quota must be below identity: %v", quota)
								}
							}
						} else if actions.X != actionX || refresh.X != refreshX {
							t.Fatalf("missing quota/current shifts buttons: actions=%v/%v refresh=%v/%v", actions.X, actionX, refresh.X, refreshX)
						}
						for _, text := range tt.Texts() {
							r, _ := tt.Find(text)
							if r.X < 0 || r.X+r.W > float32(width) {
								t.Fatalf("%q outside %d window: %+v", text, width, r)
							}
						}
						if err := tt.Click(tabDisplay.title(u.L())); err != nil {
							t.Fatal(err)
						}
						clickAccountPrivacySwitch(t, u, tt)
						if err := tt.Click(tabAccounts.title(u.L())); err != nil {
							t.Fatal(err)
						}
						if tt.HasText(account.DisplayName) || tt.HasText(account.Email) || !hasExact(tt, maskAccount(true, account.Email)) {
							t.Fatal("privacy toggle left unmasked account labels or did not redraw")
						}
						if err := tt.Click(tabDisplay.title(u.L())); err != nil {
							t.Fatal(err)
						}
						clickAccountPrivacySwitch(t, u, tt)
						if err := tt.Click(tabAccounts.title(u.L())); err != nil {
							t.Fatal(err)
						}
						if !tt.HasText(account.DisplayName) || !tt.HasText(account.Email) {
							t.Fatal("privacy disable did not restore account labels")
						}
					}
				})
			}
		}
	}
}

func TestAccountPrivacyPreservesDiagnosticText(t *testing.T) {
	for _, kind := range []int{accountKindCodex, accountKindGrok} {
		for _, message := range []string{"Quota unavailable", "Sign-in failed for private@example.com"} {
			account := codex.ManagedAccount{ID: "current", DisplayName: "Private account", Email: "private@example.com", LastError: message}
			_, tt := accountLayoutFixture(t, store.LanguageEnglish, kind, settingsMinWidth, true, []codex.ManagedAccount{account})
			if !hasExact(tt, codex.MaskEmailsIn(message)) || tt.HasText(account.Email) {
				t.Fatalf("privacy dropped diagnostic text or exposed email: %q", tt.Texts())
			}
		}
	}
}

func TestExportAccountLayoutReview(t *testing.T) {
	if os.Getenv("RUNWAY_ACCOUNT_LAYOUT_QA") == "" {
		t.Skip("set RUNWAY_ACCOUNT_LAYOUT_QA=1 to export offscreen PNGs")
	}
	base := filepath.Join("..", "..", ".build", "v1-settings-layout-qa")
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			for _, kind := range []int{accountKindCodex, accountKindGrok} {
				for _, privacy := range []bool{false, true} {
					_, tt := accountLayoutFixture(t, language, kind, width, privacy, accountLayoutRows())
					for _, dark := range []bool{false, true} {
						appearance := "light"
						if dark {
							appearance = "dark"
						}
						dir := filepath.Join(base, "accounts-"+appearance)
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						tt.SetDark(dark)
						name := fmt.Sprintf("%s-%d-provider-%d-privacy-%t.png", language, width, kind, privacy)
						exportReviewPNG(t, tt, filepath.Join(dir, name))
					}
				}
			}
		}
	}
}
