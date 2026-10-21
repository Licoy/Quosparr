package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// The static row label and the switch share their accessible name.
func clickAccountPrivacySwitch(t *testing.T, u *settingsUI, tt *ui.Tester) {
	t.Helper()
	label := u.L().Text(l10n.KeyAccountPrivacyMode)
	row := func() ui.Rect {
		r, ok := tt.Find(label)
		detail, detailOK := tt.Find(tx(u.L(), xPrivacyDisplayDescription))
		if !ok || !detailOK {
			t.Fatal("privacy switch row missing")
		}
		// The switch is centered beside both label and description, which
		// wraps differently with each platform's native fonts.
		r.H = detail.Y + detail.H - r.Y
		return r
	}
	r := row()
	size := tt.Image().Bounds()
	if r.Y+r.H > float32(size.Dy()-40) {
		tt.Scroll(float32(size.Dx()-80), float32(size.Dy()-80), 0, r.Y+r.H-float32(size.Dy())+80)
		r = row()
	}
	if r.Y < 0 || r.Y+r.H > float32(size.Dy()) {
		t.Fatalf("privacy switch row is outside the window: %+v", r)
	}
	tt.ClickAt(float32(size.Dx()-48), r.Y+r.H/2)
}

func TestPrivacyDisplayCardOrderAndTranslatedLayout(t *testing.T) {
	for _, language := range l10n.Languages() {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			u := cardSettingsUI(t, store.Language(language))
			u.tab = tabDisplay
			tt := ui.NewTester(u.view, width, settingsHeight)
			appearance, ok := tt.Find(u.L().Text(l10n.KeyDisplayAppearanceSection))
			privacy, privacyOK := tt.Find(tx(u.L(), xPrivacyDisplaySection))
			if !ok || !privacyOK || appearance.Y >= privacy.Y {
				t.Fatalf("language %s privacy card order is wrong: %+v %+v", language, appearance, privacy)
			}
			for _, text := range tt.Texts() {
				r, _ := tt.Find(text)
				if r.X < 0 || r.X+r.W > float32(width) {
					t.Fatalf("language %s text %q outside %d window: %+v", language, text, width, r)
				}
			}
		}
	}
}

func TestAccountsProviderHeaderHasNoOuterTrackOrPrivacyControl(t *testing.T) {
	u := cardSettingsUI(t, store.LanguageEnglish)
	for _, dark := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) {
			applyTheme(c)
			ui.Column(c).Fill().Padding(16).Children(func() { accountProviderHeader(c, u, accountFormOf(u)) })
		}, 700, 240)
		tt.SetDark(dark)
		grok, ok := tt.Find(u.L().Text(l10n.KeyProviderGrok))
		if !ok {
			t.Fatal("account provider tabs missing")
		}
		image := tt.Image()
		if image.RGBAAt(650, int(grok.Y+grok.H/2)) != image.RGBAAt(650, 200) {
			t.Fatal("account provider header has an extra full-width track or privacy button")
		}
		if hasExact(tt, u.L().Text(l10n.KeyAccountPrivacyMode)) || hasExact(tt, tx(u.L(), xPrivacyDisplaySection)) {
			t.Fatal("account provider header still contains privacy controls")
		}
	}
}

func TestDisplayPrivacySwitchUpdatesBothProviderPanels(t *testing.T) {
	for _, provider := range store.CLIProviders() {
		snap := panelFixture()
		snap.Grok.Accounts = snap.Accounts
		snap.Grok.Accounts.Display.DisplayName = "Ada Lovelace"
		snap.Claude.Accounts = snap.Accounts
		snap.Claude.Accounts.Display.DisplayName = "Ada Lovelace"
		snap.Claude.Accounts.Display.Email = "ada@example.com"
		a, panel := newPanel(t, func(p *store.Preferences) { p.SelectedProvider = provider }, snap, PanelActions{})
		label := a.L().Text(l10n.KeyAccounts)
		if provider == store.ProviderGrok {
			label = a.L().Text(l10n.KeyGrokAccountsTitle)
		}
		if provider == store.ProviderClaude {
			label = "Ada Lovelace"
		}
		if !panel.HasText("Ada Lovelace") {
			t.Fatal("main identity missing")
		}
		if err := panel.Click(label); err != nil {
			t.Fatal(err)
		}
		u := newSettingsUI(a.settings, SettingsOptions{})
		u.tab = tabDisplay
		display := ui.NewTester(u.view, settingsWidth, settingsHeight)
		if !panel.HasText("Ada Lovelace") || !panel.HasText("ada@example.com") {
			t.Fatalf("provider %s identity missing before privacy switch", provider)
		}
		clickAccountPrivacySwitch(t, u, display)
		panel.Frame()
		if panel.HasText("Ada Lovelace") || panel.HasText("ada@example.com") || !panel.HasText("a***@***.com") {
			t.Fatalf("provider %s privacy switch did not mask panel: %q", provider, panel.Texts())
		}
		clickAccountPrivacySwitch(t, u, display)
		panel.Frame()
		if !panel.HasText("Ada Lovelace") || !panel.HasText("ada@example.com") {
			t.Fatalf("provider %s privacy switch did not restore panel identity", provider)
		}
		a.settings.Flush()
	}
}

func TestExportPrivacyDisplayReview(t *testing.T) {
	if os.Getenv("RUNWAY_PRIVACY_DISPLAY_QA") == "" {
		t.Skip("set RUNWAY_PRIVACY_DISPLAY_QA=1 to export offscreen PNGs")
	}
	base := filepath.Join("..", "..", ".build", "v1-settings-response-qa")
	for _, page := range []string{"accounts", "display-privacy"} {
		if err := os.MkdirAll(filepath.Join(base, page), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			for _, dark := range []bool{false, true} {
				name := fmt.Sprintf("%s-%d-dark-%t", language, width, dark)
				u := cardSettingsUI(t, language)
				u.tab = tabDisplay
				display := ui.NewTester(u.view, width, settingsHeight)
				display.SetTitleBar(ui.TitleBar{Height: 40, Left: 80})
				display.SetDark(dark)
				exportReviewPNG(t, display, filepath.Join(base, "display-privacy", name+".png"))
				for _, kind := range []int{0, 1} {
					for _, privacy := range []bool{false, true} {
						_, accounts := accountLayoutFixture(t, language, kind, width, privacy, accountLayoutRows())
						accounts.SetDark(dark)
						accountName := fmt.Sprintf("%s-provider-%d-privacy-%t.png", name, kind, privacy)
						exportReviewPNG(t, accounts, filepath.Join(base, "accounts", accountName))
					}
				}
			}
		}
	}
}
