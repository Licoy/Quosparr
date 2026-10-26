package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

func ptr(v float64) *float64 { return &v }

func TestAPIBalanceSummaryStates(t *testing.T) {
	l := l10n.New(l10n.English)
	at := time.Date(2026, 10, 15, 9, 30, 0, 0, time.Local)
	cases := []struct {
		q    *apiquota.Quota
		want string
	}{
		{nil, ""},
		{&apiquota.Quota{Status: apiquota.StatusUnsupported}, tx(l, xAPIBalanceNone)},
		{&apiquota.Quota{Status: apiquota.StatusUnauthorized}, tx(l, xAPIKeyRejected)},
		{&apiquota.Quota{Status: apiquota.StatusError, Error: "timed out"}, tx(l, xAPIBalanceFailed)},
		{&apiquota.Quota{Status: apiquota.StatusAvailable, Remaining: ptr(3), Total: ptr(10), Unit: "USD", FetchedAt: at},
			"Balance $3.00 / $10.00 · " + l.Text(l10n.KeyLastUpdated) + " 09:30"},
		{&apiquota.Quota{Status: apiquota.StatusAvailable, Unlimited: true}, "Balance " + tx(l, xAPIUnlimited)},
		{&apiquota.Quota{Status: apiquota.StatusAvailable, Used: ptr(2), Unit: "CNY"}, "Balance " + tx(l, xAPIBalanceUsed, "¥2.00")},
	}
	for i, c := range cases {
		if got := apiBalanceSummary(l, c.q); got != c.want {
			t.Errorf("case %d: %q, want %q", i, got, c.want)
		}
	}
}

func TestAPIErrorTextMapsBothProviders(t *testing.T) {
	l := l10n.New(l10n.English)
	cases := map[error]textKey{
		claude.ErrAPIKeyRequired:                                                xAPIKeyRequired,
		codex.ErrRelayKeyRequired:                                               xAPIKeyRequired,
		fmt.Errorf("wrap: %w", claude.ErrAPIBaseURL):                            xAPIBaseURLInvalid,
		fmt.Errorf("%w: %w", codex.ErrRelayBaseURL, apiquota.ErrInvalidBaseURL): xAPIBaseURLInvalid,
		codex.ErrRelayDuplicate:                                                 xAPIDuplicate,
		apiquota.ErrUnauthorized:                                                xAPIKeyRejected,
	}
	for err, key := range cases {
		if got := apiErrorText(l, err); got != tx(l, key) {
			t.Errorf("%v: %q", err, got)
		}
	}
	if got := apiModelsErrorText(l, apiquota.ErrModelsNotFound); got != accountFailure(l, tx(l, xAPIModelsFailed), apiquota.ErrModelsNotFound) {
		t.Errorf("models fallback = %q", got)
	}
	if got := claudePublicError(l, claude.ErrAPIUnauthorized); got != tx(l, xAPIKeyRejected) {
		t.Errorf("claude public error = %q", got)
	}
}

func TestAPIAccountFormFields(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	tt := ui.NewTester(func(c *ui.Context) { accountEditor(c, u, f) }, 700, 900)
	l := u.L()

	f.mode = modeClaudeAPIAdd
	tt.Frame()
	for _, k := range []textKey{xAPIAdd, xAPIHintClaude, xAPIFetchModels, xAPIModelOpus, xAPIModelSonnet, xAPIModelHaiku, xAPIModelFable} {
		if !tt.HasText(tx(l, k)) {
			t.Errorf("Claude form misses %q", tx(l, k))
		}
	}
	if !apiEditorIncomplete(f) {
		t.Error("a new Claude account needs a base URL and a key")
	}
	f.api.baseURL, f.api.key = "https://relay.example", "sk-test"
	if apiEditorIncomplete(f) || !apiCanFetchModels(f) {
		t.Error("filled Claude form should be submittable and fetch models")
	}
	f.closeEditor()

	f.mode, f.editID = modeCodexAPIEdit, "api-x"
	tt.Frame()
	if tt.HasText(tx(l, xAPIModelOpus)) || !tt.HasText(tx(l, xAPIKey)) {
		t.Error("Codex edit form shows Claude mapping or lacks the replaceable key")
	}
	if !tt.HasText(tx(l, xAPIEdit)) || apiEditorIncomplete(f) || !apiCanFetchModels(f) {
		t.Error("Codex edit form should use the stored key")
	}
	f.closeEditor()
	if f.api.key != "" || f.api.baseURL != "" {
		t.Error("closing kept the draft key")
	}
}

func TestAPIModelFieldListsFetchedModelsOnClick(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	f.mode = modeCodexAPIAdd
	f.api.models = []string{"gpt-test-mini", "gpt-test", "other-model"}
	tt := ui.NewTester(func(c *ui.Context) { applyTheme(c); accountEditor(c, u, f) }, 700, 900)
	label, ok := tt.Find(tx(u.L(), xAPIModel))
	if !ok {
		t.Fatal("model field missing")
	}
	tt.ClickAt(label.X+40, label.Y+label.H+22)
	tt.Frame()
	for _, m := range f.api.models {
		if !tt.HasText(m) {
			t.Fatalf("an empty field must list every model on a click, missing %q", m)
		}
	}
	if err := tt.Click("other-model"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.api.model != "other-model" {
		t.Fatalf("model = %q", f.api.model)
	}
	if got := matchingModels(f.api.models, "TEST", true); len(got) != 2 || got[0] != "gpt-test-mini" {
		t.Fatalf("typed filter = %v", got)
	}
}

func TestAPIEditOfTheAccountInUseAsksBeforeWriting(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	l := u.L()
	tt := ui.NewTester(func(c *ui.Context) { applyTheme(c); accountEditor(c, u, f) }, 700, 900)
	f.mode, f.editID, f.alias = modeCodexAPIEdit, "api-x", "Old"
	f.api = apiDraft{baseURL: "https://relay.example/v1", inUse: true}
	f.api.initial = f.api.profileFields()

	f.alias = "New"
	if apiNeedsApplyChoice(f) {
		t.Fatal("an alias does not touch the config files")
	}
	f.api.key = "sk-new"
	if !apiNeedsApplyChoice(f) {
		t.Fatal("a new key of the account in use needs a choice")
	}
	f.api.key, f.api.model = "", "gpt-new"
	if !apiNeedsApplyChoice(f) {
		t.Fatal("a new model of the account in use needs a choice")
	}
	f.api.inUse = false
	if apiNeedsApplyChoice(f) {
		t.Fatal("an account not in use saves without asking")
	}

	f.api.inUse = true
	tt.Frame()
	if err := tt.Click(l.Text(l10n.KeyOk)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !f.api.askApply || f.busy || !tt.HasText(tx(l, xAPIApplyTitle)) || !tt.HasText(tx(l, xAPISaveAndApply)) {
		t.Fatal("saving the account in use did not ask first")
	}
	if err := tt.Click(tx(l, xAPISaveOnly)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if f.api.askApply || !f.busy {
		t.Fatal("choosing an answer did not close the question and save")
	}
}

func TestAPISavedTextTellsWhatChanged(t *testing.T) {
	l := l10n.New(l10n.English)
	if got := apiSavedText(l, modeCodexAPIEdit, apiApplyNone); got != tx(l, xAPISaved) {
		t.Errorf("none = %q", got)
	}
	if got := apiSavedText(l, modeClaudeAPIEdit, apiApplySaveOnly); got != tx(l, xAPISavedNotApplied) {
		t.Errorf("save only = %q", got)
	}
	for mode, product := range map[string]string{modeCodexAPIEdit: "Codex", modeClaudeAPIEdit: "Claude Code"} {
		if got := apiSavedText(l, mode, apiApplyWrite); got != tx(l, xAPISavedApplied, product) || !strings.Contains(got, "Restart "+product) {
			t.Errorf("%s applied = %q", mode, got)
		}
	}
}

func TestPanelAPIBalanceMasksTheSiteInPrivacyMode(t *testing.T) {
	l := l10n.New(l10n.English)
	q := &apiquota.Quota{Status: apiquota.StatusUnsupported}
	failure := "dial tcp: lookup private.example.com: no such host"
	for _, private := range []bool{false, true} {
		tt := ui.NewTester(func(c *ui.Context) {
			applyTheme(c)
			panelAPIBalance(c, l, "https://private.example.com:8443/v1", q, failure, false, private)
		}, 600, 300)
		leaked := tt.HasText("private.example.com:8443") || tt.HasText(failure)
		if private == leaked {
			t.Fatalf("private=%v shows the site: %v", private, leaked)
		}
	}
	if got := privateSiteText(true, failure, "https://private.example.com/v1"); strings.Contains(got, "private.example.com") {
		t.Fatalf("masked error %q", got)
	}
	if got := privateSiteText(false, failure, "https://private.example.com/v1"); got != failure {
		t.Fatalf("privacy off changed %q", got)
	}
}

func TestAnOutdatedConfigOffersToApplyWithoutChanges(t *testing.T) {
	f := &accountForm{mode: modeClaudeAPIEdit}
	f.api = apiDraft{baseURL: "https://relay.example", inUse: true}
	f.api.initial = f.api.profileFields()
	if apiNeedsApplyChoice(f) {
		t.Fatal("nothing changed and the config is current")
	}
	f.api.outdated = true
	if !apiNeedsApplyChoice(f) {
		t.Fatal("an outdated config of the account in use should offer to apply")
	}
}

func TestClaudeAPIContext1MForm(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	f.mode = modeClaudeAPIEdit
	f.api.inUse = true
	f.api.initial = f.api.profileFields()
	tt := ui.NewTester(func(c *ui.Context) { applyTheme(c); accountEditor(c, u, f) }, 700, 900)
	if f.api.context1M || apiNeedsApplyChoice(f) {
		t.Fatal("1M should start disabled")
	}
	if err := tt.Click(tx(u.L(), xAPIContext1M)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !f.api.context1M || !apiNeedsApplyChoice(f) {
		t.Fatal("1M edit needs apply choice")
	}
	if !tt.HasText(tx(u.L(), xAPIContext1MHint)) {
		t.Fatal("1M help missing")
	}
	for _, key := range []textKey{xAPIContext1M, xAPIContext1MHint} {
		for i, text := range extraTexts[key] {
			if text == "" {
				t.Fatalf("missing language %d", i)
			}
		}
	}
}

func TestClaudeAPIContext1MModalBounds(t *testing.T) {
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageTraditionalChinese, store.LanguageKorean, store.LanguageJapanese, store.LanguageRussian, store.LanguageFrench} {
		t.Run(string(language), func(t *testing.T) {
			u, tt := accountLayoutFixture(t, language, accountKindClaude, settingsMinWidth, false, nil)
			f := accountFormOf(u)
			f.mode = modeClaudeAPIAdd
			tt.Frame()
			for _, text := range []string{tx(u.L(), xAPIContext1M), u.L().Text(l10n.KeyCancel), u.L().Text(l10n.KeyOk)} {
				rect, ok := tt.Find(text)
				if !ok || rect.Y < 0 || rect.Y+rect.H > float32(settingsHeight) {
					t.Fatalf("control outside modal viewport: %q %+v", text, rect)
				}
			}
		})
	}
}

func TestClaudeAPIContext1MModalScrollReachesMappings(t *testing.T) {
	for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageTraditionalChinese, store.LanguageKorean, store.LanguageJapanese, store.LanguageRussian, store.LanguageFrench} {
		t.Run(string(language), func(t *testing.T) {
			u, tt := accountLayoutFixture(t, language, accountKindClaude, settingsMinWidth, false, nil)
			tt.SetSize(settingsMinWidth, settingsMinHeight)
			f := accountFormOf(u)
			f.mode = modeClaudeAPIEdit
			f.api.inUse, f.api.context1M = true, true
			tt.Frame()
			checkbox, ok := tt.Find(tx(u.L(), xAPIContext1M))
			if !ok {
				t.Fatal("context checkbox missing")
			}
			tt.Scroll(checkbox.X+checkbox.W/2, checkbox.Y+checkbox.H/2, 0, 1000)
			for _, field := range []struct {
				key   textKey
				value *string
				want  string
			}{{xAPIModelHaiku, &f.api.haiku, "relay-haiku"}, {xAPIModelFable, &f.api.fable, "relay-fable"}} {
				label, ok := tt.Find(tx(u.L(), field.key))
				if !ok || label.Y < 0 || label.Y+label.H+40 > float32(settingsMinHeight) {
					t.Fatalf("mapping input is unreachable after scrolling: %+v", label)
				}
				tt.ClickAt(label.X+40, label.Y+label.H+22)
				tt.Type(field.want)
				if *field.value != field.want {
					t.Fatalf("scrolled mapping did not accept input: %q", *field.value)
				}
			}
			if err := tt.Click(u.L().Text(l10n.KeyOk)); err != nil {
				t.Fatal(err)
			}
			if !f.api.askApply {
				t.Fatal("confirmation button could not be clicked after scrolling")
			}
			if err := tt.Click(u.L().Text(l10n.KeyCancel)); err != nil {
				t.Fatal(err)
			}
			if f.api.askApply || f.mode == "" {
				t.Fatal("cancel did not return from the apply choice to the editor")
			}
			if err := tt.Click(u.L().Text(l10n.KeyCancel)); err != nil {
				t.Fatal(err)
			}
			if f.mode != "" {
				t.Fatal("cancel button could not close the scrolled editor")
			}
		})
	}
}
