package app

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
)

func TestSettingsQuotaThresholdValidationAndRollback(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]int{nil, {}, {-1}, {101}, {10, 200}} {
		before, writes := e.s.Prefs(), e.mem.Writes()
		if err := e.s.SetQuotaAlertThresholds(invalid); err == nil {
			t.Fatalf("accepted %v", invalid)
		}
		e.settle()
		if !reflect.DeepEqual(before, e.s.Prefs()) || e.mem.Writes() != writes {
			t.Fatal("invalid edit changed preferences")
		}
	}
	input := []int{100, 0, 50, 50}
	if err := e.s.SetQuotaAlertThresholds(input); err != nil {
		t.Fatal(err)
	}
	input[0] = 1
	e.s.SetQuotaAlerts(true)
	e.settle()
	e.s.SetQuotaAlerts(false)
	e.settle()
	want := []int{0, 50, 100}
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, want) {
		t.Fatalf("saved thresholds = %v", e.stored().QuotaAlertThresholds)
	}
	e.mem.fail.Store(true)
	if err := e.s.SetQuotaAlertThresholds([]int{25}); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if e.s.SaveError() == nil || !reflect.DeepEqual(e.s.Prefs().QuotaAlertThresholds, want) {
		t.Fatal("failed save did not report error and roll back")
	}
}

func TestSettingsQuotaThresholdReadOnly(t *testing.T) {
	e := newEnv(t, func(d *failDomain) { d.Put(store.KeyPreferences, []byte("{broken")) })
	_ = e.s.Load()
	if !errors.Is(e.s.SetQuotaAlertThresholds([]int{20}), ErrReadOnly) {
		t.Fatal("readonly edit accepted")
	}
	if e.mem.Writes() != 0 {
		t.Fatal("readonly edit wrote preferences")
	}
}

func chooseQuotaThreshold(t *testing.T, tt *ui.Tester, u *settingsUI, query string) {
	t.Helper()
	if err := tt.Click(tx(u.L(), xQuotaThresholdAdd)); err != nil {
		t.Fatal(err)
	}
	tt.Type(query)
	tt.Key(0, ui.KeyEnter)
}

func TestQuotaThresholdSearchAndImmediateActions(t *testing.T) {
	e := newEnv(t, nil)
	u, tt := openSettingsView(t, e, tabNotifications)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	e.s.SetQuotaAlerts(true)
	e.settle()
	l := u.L()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	writes := e.mem.Writes()
	click(tx(l, xQuotaThresholdAdd))
	tt.Type("50")
	e.settle()
	if !hasExact(tt, "50%") || hasExact(tt, "49%") {
		t.Fatal("search did not filter native choices")
	}
	if e.mem.Writes() != writes || !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{80, 95, 100}) {
		t.Fatal("typing changed preferences")
	}
	if dir := os.Getenv("RUNWAY_QUOTA_UI_QA"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		exportReviewPNG(t, tt, filepath.Join(dir, "search-tags-filtered-dropdown.png"))
		tt.SetDark(true)
		exportReviewPNG(t, tt, filepath.Join(dir, "search-tags-filtered-dropdown-dark.png"))
		tt.SetDark(false)
	}
	tt.Key(0, ui.KeyEnter)
	e.settle()
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{50, 80, 95, 100}) {
		t.Fatalf("selection not immediately saved: %v", e.stored().QuotaAlertThresholds)
	}
	chooseQuotaThreshold(t, tt, u, "60")
	e.settle()
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{50, 60, 80, 95, 100}) {
		t.Fatal("consecutive selection failed")
	}
	click(tx(l, xQuotaThresholdAdd))
	tt.Type("50")
	count := 0
	for _, text := range tt.Texts() {
		if text == "50%" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("selected threshold remains available as a duplicate")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEscape)
	e.settle()
	if len(e.stored().QuotaAlertThresholds) != 5 {
		t.Fatal("duplicate selection changed thresholds")
	}
	chooseQuotaThreshold(t, tt, u, "0")
	e.settle()
	if e.stored().QuotaAlertThresholds[0] != 0 {
		t.Fatal("0% was not added")
	}
	for _, value := range []int{0, 50, 60, 80, 95} {
		click(tx(l, xQuotaThresholdRemove, value))
		e.settle()
	}
	click(tx(l, xQuotaThresholdRemove, 100))
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{100}) {
		t.Fatal("last threshold removed or deletion did not save")
	}
	click(tx(l, xQuotaThresholdDefaults))
	e.settle()
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, store.DefaultQuotaAlertThresholds()) {
		t.Fatal("defaults not immediately saved")
	}
}

func TestQuotaThresholdComboboxReaches100AtMinimumSize(t *testing.T) {
	e := newEnv(t, nil)
	u, tt := openSettingsView(t, e, tabNotifications)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	e.s.SetQuotaAlerts(true)
	e.settle()
	tt.SetSize(settingsMinWidth, settingsMinHeight)
	if err := tt.Click(tx(u.L(), xQuotaThresholdRemove, 100)); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if err := tt.Click(tx(u.L(), xQuotaThresholdAdd)); err != nil {
		t.Fatal(err)
	}
	first, ok := tt.Find("0%")
	if !ok {
		t.Fatal("combobox has no 0% option")
	}
	tt.Move(first.X+first.W/2, first.Y+first.H/2)
	for range 30 {
		tt.Scroll(first.X+first.W/2, first.Y+first.H/2, 0, 120)
	}
	r, ok := tt.Find("100%")
	if !ok || r.Y < 0 || r.Y+r.H > settingsMinHeight {
		t.Fatalf("wheel cannot reach100%%: %+v", r)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyUp)
	r, ok = tt.Find("100%")
	if !ok || r.Y < 0 || r.Y+r.H > settingsMinHeight {
		t.Fatalf("Up cannot reach100%%: %+v", r)
	}
	tt.Key(0, ui.KeyEnter)
	e.settle()
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{80, 95, 100}) {
		t.Fatal("100% selection not applied")
	}
}

func TestQuotaThresholdEditorReadOnlyControls(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	e.s.prefs.QuotaAlertsEnabled = true
	e.s.loadErr = errors.New("unreadable settings")
	u := newSettingsUI(e.s, SettingsOptions{})
	tt := ui.NewTester(u.quotaThresholdEditor, 600, 650)
	for _, label := range []string{tx(u.L(), xQuotaThresholdAdd), tx(u.L(), xQuotaThresholdRemove, 80), tx(u.L(), xQuotaThresholdDefaults)} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(e.s.Prefs().QuotaAlertThresholds, []int{80, 95, 100}) || e.mem.Writes() != 0 {
		t.Fatal("readonly controls changed thresholds")
	}
	if hasExact(tt, "0%") {
		t.Fatal("readonly combobox opened")
	}
}

func TestQuotaThresholdEditorReflectsRollbackAndExternalUpdates(t *testing.T) {
	e := newEnv(t, nil)
	u, tt := openSettingsView(t, e, tabNotifications)
	e.s.SetQuotaAlerts(true)
	e.settle()
	e.mem.fail.Store(true)
	chooseQuotaThreshold(t, tt, u, "50")
	e.settle()
	tt.Frame()
	if e.s.SaveError() == nil || hasExact(tt, "50%") || !tt.HasText("disk full") {
		t.Fatal("failed save did not restore tags and show error")
	}
	e.mem.fail.Store(false)
	chooseQuotaThreshold(t, tt, u, "50")
	e.settle()
	tt.Frame()
	if !hasExact(tt, "50%") || e.s.SaveError() != nil {
		t.Fatal("selection retry failed")
	}
	if err := e.s.SetQuotaAlertThresholds([]int{20, 50}); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if !hasExact(tt, "20%") || hasExact(tt, "80%") {
		t.Fatal("tags ignored external settings")
	}
	e.s.SetQuotaAlerts(false)
	e.settle()
	e.s.SetQuotaAlerts(true)
	e.settle()
	if !reflect.DeepEqual(e.stored().QuotaAlertThresholds, []int{20, 50}) {
		t.Fatal("toggle discarded thresholds")
	}
}

func TestQuotaThresholdEditorLayoutAndScroll(t *testing.T) {
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageTraditionalChinese, store.LanguageKorean, store.LanguageJapanese, store.LanguageRussian, store.LanguageFrench} {
		for _, count := range []int{3, 101} {
			t.Run(fmt.Sprintf("%s/%d", lang, count), func(t *testing.T) {
				e := newEnv(t, nil)
				u, tt := openSettingsView(t, e, tabNotifications)
				tt.SetPreferences(ui.Preferences{ReduceMotion: true})
				e.s.SetLanguage(lang)
				e.s.SetQuotaAlerts(true)
				if count == 101 {
					values := make([]int, count)
					for i := range values {
						values[i] = i
					}
					if err := e.s.SetQuotaAlertThresholds(values); err != nil {
						t.Fatal(err)
					}
				}
				e.settle()
				for _, size := range []struct {
					name string
					w, h int
				}{{"min", settingsMinWidth, settingsMinHeight}, {"normal", settingsWidth, settingsHeight}} {
					tt.SetSize(size.w, size.h)
					if count == 101 {
						tt.Scroll(650, 450, 0, 10000)
					}
					for _, text := range tt.Texts() {
						r, ok := tt.Find(text)
						if !ok || r.X < -0.5 || r.X+r.W > float32(size.w)+0.5 {
							t.Fatalf("text outside window %q: %+v", text, r)
						}
					}
					hint, hok := tt.Find(tx(u.L(), xQuotaThresholdRules))
					action, aok := tt.Find(tx(u.L(), xQuotaThresholdDefaults))
					if !hok || !aok || math.Abs(float64(hint.Y+hint.H/2-action.Y-action.H/2)) > 0.5 {
						t.Fatalf("footer not vertically centered: hint%+v action%+v", hint, action)
					}
					if count == 101 {
						r, ok := tt.Find(tx(u.L(), xQuotaThresholdRemove, 100))
						if !ok || r.Y < 0 || r.Y+r.H > float32(size.h) {
							t.Fatalf("cannot scroll to last tag: %+v", r)
						}
					}
					if dir := os.Getenv("RUNWAY_QUOTA_UI_QA"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							exportReviewPNG(t, tt, filepath.Join(dir, fmt.Sprintf("search-tags-%s-%d-%s-dark-%t.png", lang, count, size.name, dark)))
						}
					}
				}
			})
		}
	}
}
