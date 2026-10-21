package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// Isolate each real page/group callback so scrolling, duplicate labels and
// unrelated animation cannot make a missed event look like a fast event.
func TestSettingsOtherControlEventLatency(t *testing.T) {
	type check struct {
		name   string
		page   func(*settingsUI, *ui.Context)
		label  func(*settingsUI) string
		choice func(*settingsUI) string
	}
	text := func(k l10n.Key) func(*settingsUI) string { return func(u *settingsUI) string { return u.text(k) } }
	module := func(m store.Module) func(*settingsUI, *ui.Context) {
		return func(u *settingsUI, c *ui.Context) { u.moduleCard(c, u.s.Prefs(), m, 0, len(store.Modules())) }
	}
	checks := []check{
		{"appearance", (*settingsUI).appearanceGroup, text(l10n.KeyAppearance), func(u *settingsUI) string { return u.text(appearanceKey(store.AppearanceDark)) }},
		{"provider", (*settingsUI).appearanceGroup, text(l10n.KeyProviderGrok), text(l10n.KeyProviderGrok)},
		{"panel background", (*settingsUI).appearanceGroup, text(l10n.KeyMainPanelBackground), func(u *settingsUI) string { return u.text(panelBackgroundKey(store.PanelBackgroundOpaque)) }},
		{"panel height", (*settingsUI).appearanceGroup, func(u *settingsUI) string { return tx(u.L(), xPanelHeight) }, nil},
		{"statusbar style", (*settingsUI).statusBarPage, nil, func(u *settingsUI) string { return u.text(statusBarStyleKey(store.StatusBarStyleRings)) }},
		{"statusbar provider scope", (*settingsUI).statusBarPage, text(l10n.KeyStatusBarProviderScope), nil},
		{"statusbar meters detail", (*settingsUI).statusBarPage, text(l10n.KeyStatusBarMetersDetailStyle), func(u *settingsUI) string { return u.text(metersDetailKey(store.MetersDetailBoth)) }},
		{"statusbar battery orientation", (*settingsUI).statusBarPage, nil, func(u *settingsUI) string { return tx(u.L(), xBatteryVertical) }},
		{"statusbar battery detail", (*settingsUI).statusBarPage, func(u *settingsUI) string { return tx(u.L(), xBatteryText) }, func(u *settingsUI) string { return u.text(batteryDetailKey(store.BatteryDetailRemainingPercent)) }},
		{"statusbar window", (*settingsUI).statusBarPage, func(u *settingsUI) string { return tx(u.L(), xStatusBarWindow) }, func(u *settingsUI) string { return statusBarWindowName(u.L(), store.StatusBarWindowWeekly) }},
		{"statusbar icons", (*settingsUI).statusBarPage, func(u *settingsUI) string { return tx(u.L(), xStatusBarIcons) }, nil},
		{"restore module order", (*settingsUI).modulesGroup, text(l10n.KeyRestoreDefaultOrder), nil},
		{"privacy", (*settingsUI).privacyGroup, text(l10n.KeyAccountPrivacyMode), nil},
		{"quota alert", (*settingsUI).notificationsGroup, text(l10n.KeyQuotaAlerts), nil},
		{"credits alert", (*settingsUI).notificationsGroup, text(l10n.KeyResetCreditAlerts), nil},
		{"today alert", (*settingsUI).notificationsGroup, text(l10n.KeyRateLimitResetTodayAlerts), nil},
		{"status export", (*settingsUI).advancedPage, text(l10n.KeyExportStatusJSON), nil},
		{"refresh interval", (*settingsUI).generalPage, text(l10n.KeyRefreshInterval), func(u *settingsUI) string { return u.minutes(60) }},
		{"widget interval", (*settingsUI).generalPage, text(l10n.KeyWidgetRefreshInterval), func(u *settingsUI) string { return u.widgetInterval(600) }},
		{"language", (*settingsUI).generalPage, text(l10n.KeyLanguage), func(u *settingsUI) string { return l10n.MenuTitle("french", u.L().Language()) }},
		{"module order", module(store.ModuleQuota), text(l10n.KeyAccountsMoveDown), nil},
		{"module visibility", module(store.ModuleQuota), text(l10n.KeyModuleVisible), nil},
		{"model quota", module(store.ModuleQuota), text(l10n.KeyShowModelSpecificQuotaUsage), nil},
		{"chart style", module(store.ModuleTokenUsage), text(l10n.KeyTokenUsageChartStyle), func(u *settingsUI) string { return u.text(chartStyleKey(store.ChartStyleLine)) }},
		{"today interval", module(store.ModuleRateLimitResetToday), text(l10n.KeyRateLimitResetTodayRefreshInterval), func(u *settingsUI) string { return u.resetTodayInterval(600) }},
		{"cost range", module(store.ModuleAPICost), text(l10n.KeyApiCostSummaryRange), func(u *settingsUI) string { return u.text(costRangeKey(store.CostRangeThisMonth)) }},
		{"estimate window", module(store.ModuleQuotaEstimate), text(l10n.KeyQuotaEstimateWindowMode), func(u *settingsUI) string { return u.text(windowModeKey(store.WindowModeRollingWeek)) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			e := newEnv(t, nil)
			if err := e.s.Load(); err != nil {
				t.Fatal(err)
			}
			e.s.sys.Widgets = true
			if check.name == "statusbar battery orientation" || check.name == "statusbar battery detail" {
				e.s.prefs.StatusBarDisplayStyle = store.StatusBarStyleBattery
			}
			if check.name == "restore module order" {
				e.s.prefs.MainPanelModuleOrder[0], e.s.prefs.MainPanelModuleOrder[1] = e.s.prefs.MainPanelModuleOrder[1], e.s.prefs.MainPanelModuleOrder[0]
			}
			u := newSettingsUI(e.s, SettingsOptions{})
			tt := ui.NewTester(func(c *ui.Context) { check.page(u, c) }, settingsWidth, settingsHeight)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			before := e.s.Prefs()
			var elapsed time.Duration
			if check.label == nil || check.name == "appearance" || check.name == "panel background" || check.name == "provider" {
				start := time.Now()
				if err := tt.Click(check.choice(u)); err != nil {
					t.Fatal(err)
				}
				elapsed = time.Since(start)
			} else {
				focusSettingsAction(t, tt, check.label(u))
				start := time.Now()
				if check.name == "panel height" {
					tt.Key(0, ui.KeyUp)
				} else {
					tt.Key(0, ui.KeySpace)
				}
				if check.name == "statusbar provider scope" {
					// "Custom" is also the theme-color swatch, so pick it by key.
					tt.Key(0, ui.KeyDown)
					tt.Key(0, ui.KeyEnter)
				}
				if check.choice != nil {
					if err := tt.Click(check.choice(u)); err != nil {
						t.Fatal(err)
					}
				}
				elapsed = time.Since(start)
			}
			t.Logf("event=%s", elapsed)
			// A language change reshapes the whole page in the new language.
			if elapsed > latencyBudget(100*time.Millisecond) {
				t.Fatal("control event was slow")
			}
			if reflect.DeepEqual(before, e.s.Prefs()) {
				t.Fatal("control event did not change actual preferences")
			}
			e.settle()
			if !reflect.DeepEqual(e.stored(), e.s.Prefs()) {
				t.Fatal("control choice was not saved")
			}
		})
	}
}

// The arrangement switches and check boxes bind a per-frame value, so a
// click must be read through Changed in the same pass or it is lost.
func TestStatusBarArrangementEditsTheComponents(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	e.s.prefs.SetStatusBarGroups([][]store.Provider{{store.ProviderCodex}})
	u := newSettingsUI(e.s, SettingsOptions{})
	tt := ui.NewTester(func(c *ui.Context) { u.statusBarArrangement(c, e.s.Prefs(), false) }, settingsWidth, settingsHeight)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	l := u.L()
	name := func(p store.Provider) string { return l.Text(providerKey(p)) }
	step := func(label string, want [][]store.Provider) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if got := e.s.Prefs().StatusBarGroups(); !reflect.DeepEqual(got, want) {
			t.Fatalf("after %q groups = %v, want %v", label, got, want)
		}
	}
	codex, claude, grok := store.ProviderCodex, store.ProviderClaude, store.ProviderGrok
	step(tx(l, xStatusBarShowProvider, name(grok)), [][]store.Provider{{codex}, {grok}})
	step(tx(l, xStatusBarShowProvider, name(claude)), [][]store.Provider{{codex}, {grok}, {claude}})
	step(tx(l, xStatusBarJoinProvider, name(grok)), [][]store.Provider{{codex, grok}, {claude}})
	// Claude would make a third of the pair.
	step(tx(l, xStatusBarJoinProvider, name(claude)), [][]store.Provider{{codex, grok}, {claude}})
	// The first line moves down; the pair stays in place.
	step(l.Text(l10n.KeyAccountsMoveDown), [][]store.Provider{{grok, codex}, {claude}})
	step(tx(l, xStatusBarShowProvider, name(grok)), [][]store.Provider{{codex}, {claude}})
	if got := e.s.Prefs().StatusBarSelection(); !reflect.DeepEqual(got, []store.Provider{codex, claude}) {
		t.Fatalf("older versions read %v", got)
	}
}

func TestStatusBarSlotsKeepComponentsInPlace(t *testing.T) {
	codex, claude, grok := store.ProviderCodex, store.ProviderClaude, store.ProviderGrok
	slots := statusBarSlots([][]store.Provider{{codex, claude}, {grok}})
	if !reflect.DeepEqual(statusBarGroupsOf(slots), [][]store.Provider{{codex, claude}, {grok}}) {
		t.Fatalf("round trip = %v", statusBarGroupsOf(slots))
	}
	if canJoinStatusBarSlot(slots, 2) || canJoinStatusBarSlot(slots, 0) {
		t.Error("a third provider joined a pair")
	}
	if got := statusBarGroupsOf(withoutStatusBarSlot(slots, 0)); !reflect.DeepEqual(got, [][]store.Provider{{claude}, {grok}}) {
		t.Errorf("without the head = %v", got)
	}
	if got := statusBarGroupsOf(swapStatusBarSlots(slots, 1, 2)); !reflect.DeepEqual(got, [][]store.Provider{{codex, grok}, {claude}}) {
		t.Errorf("swapped = %v", got)
	}
}
