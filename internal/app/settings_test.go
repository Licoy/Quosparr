package app

import (
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// mainQueue stands in for the main thread: results of background saves are
// queued by runOnMain and run by drain.
type mainQueue struct {
	mu  sync.Mutex
	fns []func()
}

func (q *mainQueue) hop(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

func (q *mainQueue) drain() {
	for {
		q.mu.Lock()
		fns := q.fns
		q.fns = nil
		q.mu.Unlock()
		if len(fns) == 0 {
			return
		}
		for _, fn := range fns {
			fn()
		}
	}
}

// failDomain is a MemoryDomain whose writes can be made to fail.
type failDomain struct {
	*store.MemoryDomain
	fail atomic.Bool
}

func (d *failDomain) Write(key string, data []byte) error {
	if d.fail.Load() {
		return errors.New("disk full")
	}
	return d.MemoryDomain.Write(key, data)
}

// fakeSys records what the settings ask of the system.
type fakeSys struct {
	appearance  []store.Appearance
	login       bool
	loginSets   []bool
	loginErr    error
	updateCalls []bool
}

func (f *fakeSys) system() System {
	return System{
		PreferredLanguages: func() []string { return []string{"en-US"} },
		SetAppearance:      func(a store.Appearance) { f.appearance = append(f.appearance, a) },
		LoginSupported:     true,
		StatusBarStyles:    true,
		PanelBackground:    true,
		PanelAnimation:     true,
		OpenAtLogin:        func() (bool, error) { return f.login, nil },
		SetOpenAtLogin: func(on bool) error {
			f.loginSets = append(f.loginSets, on)
			if f.loginErr != nil {
				return f.loginErr
			}
			f.login = on
			return nil
		},
		SetAutomaticUpdateChecks: func(on bool) { f.updateCalls = append(f.updateCalls, on) },
	}
}

type env struct {
	t     *testing.T
	s     *Settings
	mem   *failDomain
	st    *store.Store
	sys   *fakeSys
	creds *MemoryCredentials
	q     *mainQueue
}

func newEnv(t *testing.T, seed func(*failDomain)) *env {
	t.Helper()
	e := &env{t: t, mem: &failDomain{MemoryDomain: store.NewMemoryDomain()}, sys: &fakeSys{}, creds: NewMemoryCredentials(), q: &mainQueue{}}
	t.Cleanup(setRunOnMain(e.q.hop))
	if seed != nil {
		seed(e.mem)
	}
	e.st = store.New(e.mem, nil)
	e.s = NewSettings(e.st, e.sys.system(), e.creds)
	return e
}

// settle waits for the saves and delivers their results.
func (e *env) settle() { e.s.Flush(); e.q.drain() }

// stored loads the preferences from a fresh store over the same domain.
func (e *env) stored() store.Preferences {
	e.t.Helper()
	p, _, err := store.New(e.mem, nil).LoadPreferences()
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

// changedFields lists the JSON names of the preference fields that differ.
func changedFields(a, b store.Preferences) []string {
	var out []string
	ta, va, vb := reflect.TypeOf(a), reflect.ValueOf(a), reflect.ValueOf(b)
	for i := 0; i < ta.NumField(); i++ {
		f := ta.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if !reflect.DeepEqual(va.Field(i).Interface(), vb.Field(i).Interface()) {
			out = append(out, strings.Split(f.Tag.Get("json"), ",")[0])
		}
	}
	sort.Strings(out)
	return out
}

func TestSettingsEveryFieldHasItsControlSetter(t *testing.T) {
	grok := store.ProviderGrok
	type setter = func(s *Settings) bool
	flip := func(get func(*store.Preferences) *bool, set func(*Settings, bool) bool) setter {
		return func(s *Settings) bool { p := s.Prefs(); return set(s, !*get(&p)) }
	}
	showFlip := func(m store.Module) setter {
		return func(s *Settings) bool { p := s.Prefs(); return s.SetModuleShown(m, !ModuleShown(p, m)) }
	}
	cases := []struct {
		name   string
		apply  setter
		fields []string
	}{
		{"provider", func(s *Settings) bool { return s.SetSelectedProvider(grok) }, []string{"selectedProvider"}},
		{"language", func(s *Settings) bool { return s.SetLanguage(store.LanguageFrench) }, []string{"language"}},
		{"appearance", func(s *Settings) bool { return s.SetAppearance(store.AppearanceDark) }, []string{"appearance"}},
		{"panel background", func(s *Settings) bool { return s.SetPanelBackground(store.PanelBackgroundOpaque) }, []string{"mainPanelBackgroundStyle"}},
		{"status bar style", func(s *Settings) bool { return s.SetStatusBarStyle(store.StatusBarStyleRings) }, []string{"statusBarDisplayStyle"}},
		{"meters detail", func(s *Settings) bool { return s.SetStatusBarMetersDetail(store.MetersDetailBoth) }, []string{"statusBarMetersDetailStyle"}},
		{"battery detail", func(s *Settings) bool { return s.SetStatusBarBatteryDetail(store.BatteryDetailRemainingPercent) }, []string{"statusBarBatteryDetailStyle"}},
		{"provider scope", func(s *Settings) bool { return s.SetStatusBarProviderScope(store.ProviderScopeBoth) }, []string{"statusBarProviderScope"}},
		{"status bar groups", func(s *Settings) bool {
			return s.SetStatusBarGroups([][]store.Provider{{store.ProviderClaude, grok}, {store.ProviderCodex}})
		}, []string{"statusBarLayout", "statusBarProviderScope", "statusBarProviders"}},
		{"status bar layout", func(s *Settings) bool {
			return s.SetStatusBarLayout(func(l *store.StatusBarLayout) { l.Battery = store.BatteryVertical })
		}, []string{"statusBarLayout"}},
		{"refresh interval", func(s *Settings) bool { return s.SetRefreshInterval(600) }, []string{"refreshIntervalSeconds"}},
		{"widget refresh", func(s *Settings) bool { return s.SetWidgetRefreshInterval(900) }, []string{"widgetRefreshIntervalSeconds"}},
		{"cost range", func(s *Settings) bool { return s.SetAPICostRange(store.CostRangeThisMonth) }, []string{"apiCostSummaryRange"}},
		{"panel height", func(s *Settings) bool { return s.SetMainPanelHeight(700) }, []string{"mainPanelHeight"}},
		{"disable panel animation", func(s *Settings) bool { return s.SetDisableMainPanelAnimation(true) }, []string{"disableMainPanelAnimation"}},
		{"module order", func(s *Settings) bool { return s.MoveMainPanelModule(store.ModuleAPICost, -1) }, []string{"mainPanelModuleOrder"}},
		{"show quota", showFlip(store.ModuleQuota), []string{"showsQuotaSummary"}},
		{"show reset credits", showFlip(store.ModuleResetCredits), []string{"showsResetCreditsSummary"}},
		{"show quota estimate", showFlip(store.ModuleQuotaEstimate), []string{"showsQuotaEstimateSummary"}},
		{"estimate window", func(s *Settings) bool { return s.SetQuotaEstimateWindowMode(store.WindowModeRollingWeek) }, []string{"quotaEstimateWindowMode"}},
		{"show cost", showFlip(store.ModuleAPICost), []string{"showsCostSummary"}},
		{"show recent sessions", showFlip(store.ModuleRecentSessions), []string{"showsRecentSessions"}},
		{"show session repair", showFlip(store.ModuleSessionRepair), []string{"showsSessionRepairSummary"}},
		{"show reset today", showFlip(store.ModuleRateLimitResetToday), []string{"showsRateLimitResetToday"}},
		{"model specific usage", flip(func(p *store.Preferences) *bool { return &p.ShowsModelSpecificQuotaUsage }, (*Settings).SetShowsModelSpecificQuotaUsage), []string{"showsModelSpecificQuotaUsage"}},
		{"show credits balance", flip(func(p *store.Preferences) *bool { return &p.ShowsCreditsBalance }, (*Settings).SetShowsCreditsBalance), []string{"showsCreditsBalance"}},
		{"show token usage", showFlip(store.ModuleTokenUsage), []string{"showsTokenUsageHeatmap"}},
		{"show speed analysis", showFlip(store.ModuleSpeedAnalysis), []string{"showsSpeedAnalysis"}},
		{"chart style", func(s *Settings) bool { return s.SetTokenUsageChartStyle(store.ChartStyleBar) }, []string{"tokenUsageChartStyle"}},
		{"reset today refresh", func(s *Settings) bool {
			return s.SetRateLimitResetTodayRefreshInterval(store.RateLimitResetTodayRefreshIntervalOptions()[1])
		}, []string{"rateLimitResetTodayRefreshIntervalSeconds"}},
		{"automatic updates", flip(func(p *store.Preferences) *bool { return &p.AutomaticallyChecksForUpdates }, (*Settings).SetAutomaticUpdateChecks), []string{"automaticallyChecksForUpdates"}},
		{"launch at login", func(s *Settings) bool { return s.SetLaunchAtLogin(false) }, []string{"launchAtLoginEnabled", "launchAtLoginInitialized"}},
		{"quota alerts", flip(func(p *store.Preferences) *bool { return &p.QuotaAlertsEnabled }, (*Settings).SetQuotaAlerts), []string{"quotaAlertsEnabled"}},
		{"reset credit alerts", flip(func(p *store.Preferences) *bool { return &p.ResetCreditAlertsEnabled }, (*Settings).SetResetCreditAlerts), []string{"resetCreditAlertsEnabled"}},
		{"reset today alerts", flip(func(p *store.Preferences) *bool { return &p.RateLimitResetTodayAlertsEnabled }, (*Settings).SetRateLimitResetTodayAlerts), []string{"rateLimitResetTodayAlertsEnabled"}},
		{"export status json", flip(func(p *store.Preferences) *bool { return &p.ExportsStatusJSON }, (*Settings).SetExportsStatusJSON), []string{"exportsStatusJSON"}},
		{"account privacy", flip(func(p *store.Preferences) *bool { return &p.AccountPrivacyModeEnabled }, (*Settings).SetAccountPrivacyMode), []string{"accountPrivacyModeEnabled"}},
	}
	// The 38 stored fields are each reached by a case above, the login pair
	// by one; statusBarBatteryScope is only kept for older versions, the
	// status bar window took its place.
	seen := map[string]bool{}
	for _, c := range cases {
		for _, f := range c.fields {
			seen[f] = true
		}
	}
	if len(seen) != 38 {
		t.Errorf("cases cover %d preference fields, want 38", len(seen))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, nil)
			if err := e.s.Load(); err != nil {
				t.Fatal(err)
			}
			def := e.s.Prefs()
			if !c.apply(e.s) {
				t.Fatal("the change was refused")
			}
			e.settle()
			if got := changedFields(def, e.s.Prefs()); !slices.Equal(got, c.fields) {
				t.Errorf("in memory, changed %v, want %v", got, c.fields)
			}
			// Saved at once, and what is saved is what is shown.
			if got := changedFields(def, e.stored()); !slices.Equal(got, c.fields) {
				t.Errorf("stored, changed %v, want %v", got, c.fields)
			}
			if e.s.SaveError() != nil {
				t.Errorf("save error: %v", e.s.SaveError())
			}
		})
	}
}

func TestSettingsDamagedConfigIsReadOnlyAndNeverSaved(t *testing.T) {
	damaged := []byte(`{"language": 12, not json`)
	e := newEnv(t, func(d *failDomain) { d.Put(store.KeyPreferences, damaged) })
	err := e.s.Load()
	if err == nil || !e.s.ReadOnly() || e.s.LoadError() == nil {
		t.Fatalf("Load = %v, ReadOnly = %v", err, e.s.ReadOnly())
	}
	if e.s.BackgroundAllowed() {
		t.Error("background tasks allowed on a damaged config")
	}
	for name, ok := range map[string]bool{
		"language":   e.s.SetLanguage(store.LanguageFrench),
		"appearance": e.s.SetAppearance(store.AppearanceDark),
		"module":     e.s.MoveMainPanelModule(store.ModuleAPICost, -1),
		"reset":      e.s.ResetMainPanelModuleOrder(),
		"login":      e.s.SetLaunchAtLogin(false),
		"update":     e.s.Update(func(p *store.Preferences) { p.QuotaAlertsEnabled = !p.QuotaAlertsEnabled }),
	} {
		if ok {
			t.Errorf("%s change was accepted while read-only", name)
		}
	}
	e.settle()
	if n := e.mem.Writes(); n != 0 {
		t.Errorf("%d writes over a damaged config", n)
	}
	got, _, _ := e.mem.Read(store.KeyPreferences)
	if string(got) != string(damaged) {
		t.Error("the damaged value was changed")
	}
	if len(e.sys.loginSets) != 0 {
		t.Error("the login item was changed while read-only")
	}
	// The defaults are on display.
	if !reflect.DeepEqual(e.s.Prefs(), store.DefaultPreferences()) {
		t.Error("read-only settings do not show the defaults")
	}
}

func TestSettingsStartupSeamsOnDamage(t *testing.T) {
	e := newEnv(t, func(d *failDomain) {
		d.Put(store.KeyPreferences, []byte("{"))
		d.Put(store.KeyNetworkProxy, []byte("{"))
	})
	r := Seams{LoadConfig: e.s.Load, VerifyProxy: e.s.LoadProxy}.RunSteps()
	if r.BackgroundAllowed || len(r.Problems) != 2 {
		t.Errorf("report = %+v", r)
	}
	if e.s.ProxyError() == nil {
		t.Error("no proxy error")
	}
}

func TestSettingsDamagedProxyBlocksNetworkOnlyUntilSavedValid(t *testing.T) {
	e := newEnv(t, func(d *failDomain) { d.Put(store.KeyNetworkProxy, []byte(`{"mode":"http","host":""}`)) })
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	if e.s.LoadProxy() == nil || e.s.BackgroundAllowed() {
		t.Fatal("a damaged proxy config must block network tasks")
	}
	// Preferences still save.
	if !e.s.SetQuotaAlerts(true) {
		t.Error("preferences should stay editable with a damaged proxy")
	}
	e.settle()
	// An explicit valid save repairs it.
	res, err := e.s.SaveProxy(ProxyDraft{Mode: store.ProxyModeHTTP, Host: "proxy.local", Port: "8080"}, e.s.Proxy())
	if err != nil {
		t.Fatal(err)
	}
	e.s.ProxySaved(res)
	if e.s.ProxyError() != nil || !e.s.BackgroundAllowed() {
		t.Error("a valid save should clear the proxy error")
	}
}

func TestSettingsModuleOrderOperations(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	def := e.s.Prefs().MainPanelModuleOrder

	e.s.MoveMainPanelModule(def[2], -1)
	got := e.s.Prefs().MainPanelModuleOrder
	if got[1] != def[2] || got[2] != def[1] {
		t.Errorf("up: %v from %v", got, def)
	}
	e.s.MoveMainPanelModule(def[2], 1)
	if got = e.s.Prefs().MainPanelModuleOrder; got[1] != def[1] || got[2] != def[2] {
		t.Errorf("down: %v", got)
	}
	// The ends do not wrap.
	e.s.MoveMainPanelModule(def[0], -1)
	e.s.MoveMainPanelModule(def[len(def)-1], 1)
	if !slices.Equal(e.s.Prefs().MainPanelModuleOrder, def) {
		t.Error("moving past an end changed the order")
	}
	// Hiding a module keeps its place.
	e.s.MoveMainPanelModule(def[3], -2)
	moved := e.s.Prefs().MainPanelModuleOrder
	e.s.SetModuleShown(def[3], false)
	if !slices.Equal(e.s.Prefs().MainPanelModuleOrder, moved) {
		t.Error("hiding a module changed the order")
	}
	e.s.ResetMainPanelModuleOrder()
	e.settle()
	if !slices.Equal(e.s.Prefs().MainPanelModuleOrder, store.Modules()) {
		t.Error("reset did not restore the default order")
	}
	if !slices.Equal(e.stored().MainPanelModuleOrder, store.Modules()) {
		t.Error("the reset order was not saved")
	}
}

func TestSettingsLanguageAndAppearanceApplyLive(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	e.s.Apply()
	if e.s.L().Language() != l10n.English {
		t.Fatalf("auto with an English system = %v", e.s.L().Language())
	}
	changes := 0
	e.s.OnChange(func() { changes++ })
	e.s.SetLanguage(store.LanguageFrench)
	if e.s.L().Language() != l10n.French {
		t.Errorf("language not applied at once: %v", e.s.L().Language())
	}
	e.s.SetAppearance(store.AppearanceDark)
	if last := e.sys.appearance[len(e.sys.appearance)-1]; last != store.AppearanceDark {
		t.Errorf("appearance applied = %v", e.sys.appearance)
	}
	if changes != 2 {
		t.Errorf("%d change notifications, want 2", changes)
	}
	e.settle()
}

func TestSettingsFailedSaveRevertsAndExplains(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	e.mem.fail.Store(true)
	e.s.SetLanguage(store.LanguageFrench)
	e.s.SetAppearance(store.AppearanceDark)
	e.settle()
	p := e.s.Prefs()
	if p.Language != store.LanguageSystem || p.Appearance != store.AppearanceSystem {
		t.Errorf("not reverted: %v %v", p.Language, p.Appearance)
	}
	if e.s.SaveError() == nil {
		t.Error("no save error shown")
	}
	if e.s.L().Language() != l10n.English {
		t.Error("language not reverted")
	}
	if last := e.sys.appearance[len(e.sys.appearance)-1]; last != store.AppearanceSystem {
		t.Errorf("appearance not restored: %v", e.sys.appearance)
	}
	// The next change saves again.
	e.mem.fail.Store(false)
	e.s.SetQuotaAlerts(true)
	e.settle()
	if e.s.SaveError() != nil || !e.stored().QuotaAlertsEnabled {
		t.Error("the setting did not recover after the disk did")
	}
}

func TestSettingsRapidChangesSaveTheNewest(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	for _, n := range []int{120, 180, 240, 600, 900} {
		e.s.SetRefreshInterval(n)
	}
	e.settle()
	if got := e.stored().RefreshIntervalSeconds; got != 900 {
		t.Errorf("stored interval = %d", got)
	}
}

func TestSettingsClampsOutOfRangeValues(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	e.s.SetRefreshInterval(5)
	if got := e.s.Prefs().RefreshIntervalSeconds; got != 60 {
		t.Errorf("interval = %d, want clamp to 60", got)
	}
	e.s.SetMainPanelHeight(100)
	if got := e.s.Prefs().MainPanelHeight; got != store.MinMainPanelHeight {
		t.Errorf("height = %v, want minimum", got)
	}
	e.settle()
}

func TestSettingsPreservesTallPanelHeight(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	e.s.SetMainPanelHeight(1620)
	e.settle()
	if got := e.stored().MainPanelHeight; got != 1620 {
		t.Fatalf("stored height = %v, want 1620", got)
	}
}

func TestSettingsLaunchAtLoginReconcile(t *testing.T) {
	// First launch: the default (on) is registered and recorded.
	e := newEnv(t, nil)
	_ = e.s.Load()
	if err := e.s.ReconcileLogin(); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if !slices.Equal(e.sys.loginSets, []bool{true}) || !e.stored().LaunchAtLoginInitialized {
		t.Errorf("first launch: sets %v, stored %+v", e.sys.loginSets, e.stored())
	}

	// Later: a login item the user turned off in the system stays off.
	e.sys.login = false
	e.sys.loginSets = nil
	if err := e.s.ReconcileLogin(); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if len(e.sys.loginSets) != 0 {
		t.Errorf("the login item was registered again: %v", e.sys.loginSets)
	}
	if e.stored().LaunchAtLoginEnabled {
		t.Error("the stored choice should follow the system")
	}
}

func TestSettingsLaunchAtLoginFailureKeepsStoredValue(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	e.sys.loginErr = errors.New("denied")
	if e.s.SetLaunchAtLogin(false) {
		t.Error("a failed login change reported success")
	}
	if e.s.LoginError() == nil || !e.s.Prefs().LaunchAtLoginEnabled || e.s.Prefs().LaunchAtLoginInitialized {
		t.Errorf("stored choice changed: %+v, err %v", e.s.Prefs(), e.s.LoginError())
	}
}

func TestSettingsUnsupportedPlatformKeepsStoredValues(t *testing.T) {
	e := newEnv(t, func(d *failDomain) {
		p := store.DefaultPreferences()
		p.StatusBarDisplayStyle = store.StatusBarStyleRings
		p.WidgetRefreshIntervalSeconds = 900
		p.LaunchAtLoginEnabled = false
		p.LaunchAtLoginInitialized = true
		data, err := store.EncodePreferences(p)
		if err != nil {
			t.Fatal(err)
		}
		d.Put(store.KeyPreferences, data)
	})
	e.s = NewSettings(e.st, System{}, e.creds) // nothing is supported
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	if e.s.LoginAvailable() || e.s.ReconcileLogin() != nil {
		t.Error("login should be unavailable and reconcile a no-op")
	}
	e.s.SetQuotaAlerts(true) // an unrelated save must not lose them
	e.settle()
	got := e.stored()
	if !got.QuotaAlertsEnabled {
		t.Fatal("the unrelated change was not saved")
	}
	if got.StatusBarDisplayStyle != store.StatusBarStyleRings || got.WidgetRefreshIntervalSeconds != 900 || got.LaunchAtLoginEnabled {
		t.Errorf("platform-specific values were not kept: %+v", got)
	}
}

func TestSettingsAutomaticUpdateChecksReachTheUpdater(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	e.s.SetAutomaticUpdateChecks(false)
	if !slices.Equal(e.sys.updateCalls, []bool{false}) {
		t.Errorf("updater calls = %v", e.sys.updateCalls)
	}
	e.settle()
}

func TestAutomaticUpdateChecksRollbackReappliesSystem(t *testing.T) {
	e := newEnv(t, nil)
	if err := e.s.Load(); err != nil {
		t.Fatal(err)
	}
	e.mem.fail.Store(true)
	e.s.SetAutomaticUpdateChecks(false)
	e.settle()
	if !e.s.Prefs().AutomaticallyChecksForUpdates {
		t.Fatal("preference did not roll back")
	}
	if len(e.sys.updateCalls) != 2 || e.sys.updateCalls[0] || !e.sys.updateCalls[1] {
		t.Fatalf("system not rolled back: %v", e.sys.updateCalls)
	}
}
