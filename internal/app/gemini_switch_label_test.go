package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func TestGeminiSwitchUsesAntigravityLabel(t *testing.T) {
	const label = "Use in Antigravity"
	s := NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, nil)
	s.prefs = store.DefaultPreferences()
	s.prefs.Language = store.LanguageEnglish
	s.applyLocale()
	u := newSettingsUI(s, SettingsOptions{})
	l := u.L()
	acc := codex.ManagedAccount{ID: "fixture", DisplayName: "Fixture"}
	f := accountFormOf(u)
	actions := geminiRowActions(u, f, nil, acc, rowPlace{}, acc.DisplayName, false)
	if actions[0].label != label {
		t.Errorf("settings row action = %q, want %q", actions[0].label, label)
	}
	actions[0].run()
	editor := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		accountEditor(c, u, f)
	}, 620, 360)
	if !editor.HasText(label) || editor.HasText(l.Text(l10n.KeyAccountsMakeCurrent)) {
		t.Error("Gemini settings confirmation does not use the Antigravity label")
	}

	a := New(Options{Settings: s})
	panel := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Padding(16).Children(func() {
			a.geminiCardActions(c, l, acc, false, false, runway.AccountsView{},
				AccountActions{GeminiSwitch: func(string, bool, bool) (bool, string) { return false, "" }})
		})
	}, 620, 360)
	if err := panel.Click(label); err != nil {
		t.Fatal(err)
	}
	if !panel.HasText(l.Text(l10n.KeyAccountsSwitchConfirmTitle)) ||
		!panel.HasText(label) || panel.HasText(l.Text(l10n.KeyAccountsMakeCurrent)) {
		t.Error("Gemini panel confirmation does not use the Antigravity label")
	}
}
