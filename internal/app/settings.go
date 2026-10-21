package app

import (
	"errors"
	"reflect"
	"slices"
	"sync"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// PrefsStore is the part of store.Store that the settings use.
type PrefsStore interface {
	LoadPreferences() (store.Preferences, store.Source, error)
	SavePreferences(store.Preferences) error
	LoadNetworkProxy() (store.NetworkProxyConfiguration, error)
	SaveNetworkProxy(store.NetworkProxyConfiguration) error
}

// ErrReadOnly is returned by changes while the stored settings could not
// be read: nothing may be saved over a value that was not understood.
var ErrReadOnly = errors.New("settings are read-only: the stored value could not be read")

// Settings is the view-model of the settings window and the owner of the
// loaded preferences and proxy configuration. It holds no UI. Model changes
// run on the main thread; disk saves and supported login APIs run on workers.
//
// A change is applied to the in-memory preferences at once and saved at
// once on a background goroutine, like the Swift app; the result comes back
// to the main thread. A failed save reverts the change and shows the error.
// When the stored preferences could not be loaded the settings are
// read-only and are never saved.
type Settings struct {
	st  PrefsStore
	sys System

	prefs             store.Preferences
	persisted         store.Preferences
	loaded            bool
	loadErr           error
	source            store.Source
	saveErr           error
	loginErr          error
	loginStatus       LoginStatus
	loginKnown        bool
	loginPending      bool
	loginRefreshAgain bool
	loginTarget       *bool
	loginIdle         sync.WaitGroup
	loginResult       *loginResult
	onLoginChange     []func()
	loc               l10n.Localizer

	proxy      store.NetworkProxyConfiguration
	proxyErr   error // the stored proxy configuration could not be used
	creds      CredentialStore
	proxyHooks ProxyHooks

	mu                  sync.Mutex // guards pending, running
	pending             *store.Preferences
	running             bool
	idle                sync.WaitGroup
	saveResults         []settingsSaveResult
	saveDeliveryPending bool

	onChange []func()
}

type settingsSaveResult struct {
	prefs store.Preferences
	err   error
}

// NewSettings returns settings over st. Call Load before use.
func NewSettings(st PrefsStore, sys System, creds CredentialStore) *Settings {
	s := &Settings{st: st, sys: sys, creds: creds}
	s.prefs = store.DefaultPreferences()
	s.persisted = s.prefs.Clone()
	s.applyLocale()
	return s
}

// OnChange registers fn, called on the main thread after the settings
// changed in a way a window should redraw for.
func (s *Settings) OnChange(fn func()) func() {
	index := len(s.onChange)
	s.onChange = append(s.onChange, fn)
	return func() { s.onChange[index] = nil }
}

func (s *Settings) changed() {
	for _, fn := range s.onChange {
		if fn != nil {
			fn()
		}
	}
}

// Load reads the preferences, importing the legacy domain when the current
// one has none. A failure leaves the defaults on display and the settings
// read-only; it is returned for the startup report.
func (s *Settings) Load() error {
	p, src, err := s.st.LoadPreferences()
	s.loaded = true
	if err != nil {
		s.loadErr = err
		s.prefs = store.DefaultPreferences()
		s.persisted = s.prefs.Clone()
		s.applyLocale()
		return err
	}
	s.loadErr = nil
	s.source = src
	s.prefs = p
	s.persisted = p.Clone()
	s.applyLocale()
	return nil
}

// LoadProxy reads the proxy configuration; a failure blocks network tasks
// until the user saves a valid one.
func (s *Settings) LoadProxy() error {
	c, err := s.st.LoadNetworkProxy()
	if err != nil {
		s.proxyErr = err
		s.proxy = store.DefaultNetworkProxy()
		return err
	}
	s.proxyErr = nil
	s.proxy = c
	return nil
}

// Prefs returns a copy of the current preferences. While the settings are
// read-only they are the defaults, for display only.
func (s *Settings) Prefs() store.Preferences { return s.prefs.Clone() }

// ReadOnly reports whether the stored preferences could not be loaded, so
// no change can be made.
func (s *Settings) ReadOnly() bool { return s.loadErr != nil }

// LoadError is why the preferences could not be loaded, or nil.
func (s *Settings) LoadError() error { return s.loadErr }

// ProxyError is why the stored proxy configuration is unusable, or nil.
func (s *Settings) ProxyError() error { return s.proxyErr }

// Source tells where the loaded preferences came from.
func (s *Settings) Source() store.Source { return s.source }

// SaveError is the last failed save, or nil after a successful one.
func (s *Settings) SaveError() error { return s.saveErr }

// LoginError is the last failure to change the login item, or nil.
func (s *Settings) LoginError() error { return s.loginErr }

// L is the localizer of the language the settings select.
func (s *Settings) L() l10n.Localizer { return s.loc }

// System returns the OS calls in use.
func (s *Settings) System() System { return s.sys }

// BackgroundAllowed reports whether the preferences and the proxy are
// usable, so network and write tasks may run.
func (s *Settings) BackgroundAllowed() bool { return s.loadErr == nil && s.proxyErr == nil }

// Update applies change to the preferences, normalizes them and saves them
// in the background. It returns false, changing nothing, when the settings
// are read-only, and true otherwise (also when nothing differs).
func (s *Settings) Update(change func(*store.Preferences)) bool {
	if s.loadErr != nil {
		return false
	}
	next := s.prefs.Clone()
	change(&next)
	next.Normalize()
	if reflect.DeepEqual(next, s.prefs) {
		return true
	}
	old := s.prefs
	s.prefs = next
	s.saveErr = nil
	if old.Language != next.Language {
		s.applyLocale()
	}
	if old.Appearance != next.Appearance {
		s.applyAppearance()
	}
	if old.AutomaticallyChecksForUpdates != next.AutomaticallyChecksForUpdates && s.sys.SetAutomaticUpdateChecks != nil {
		s.sys.SetAutomaticUpdateChecks(next.AutomaticallyChecksForUpdates)
	}
	s.enqueue(next.Clone())
	s.changed()
	return true
}

// enqueue saves p on the save goroutine, which writes the newest pending
// value and nothing in between.
func (s *Settings) enqueue(p store.Preferences) {
	s.mu.Lock()
	s.pending = &p
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.idle.Add(1)
	s.mu.Unlock()
	go s.drain(mainDispatcher())
}

func (s *Settings) drain(post func(func())) {
	defer s.idle.Done()
	for {
		s.mu.Lock()
		p := s.pending
		s.pending = nil
		if p == nil {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		err := s.st.SavePreferences(*p)
		s.mu.Lock()
		s.saveResults = append(s.saveResults, settingsSaveResult{*p, err})
		notify := !s.saveDeliveryPending
		s.saveDeliveryPending = true
		s.mu.Unlock()
		if notify {
			go post(s.deliverSaveResults)
		}
	}
}

// deliverSaveResults runs on the UI thread, also from Flush while quitting;
// disk writes never wait for this thread to acknowledge their results.
func (s *Settings) deliverSaveResults() {
	s.mu.Lock()
	results := s.saveResults
	s.saveResults = nil
	s.saveDeliveryPending = false
	s.mu.Unlock()
	for _, result := range results {
		s.saved(result.prefs, result.err)
	}
}

// saved runs on the main thread when a save finished.
func (s *Settings) saved(p store.Preferences, err error) {
	if err == nil {
		s.persisted = p
		if s.saveErr != nil && reflect.DeepEqual(s.prefs, p) {
			s.saveErr = nil
		}
		return
	}
	s.saveErr = err
	// A newer change is saved (and may succeed) on its own; only the
	// newest unsaved state goes back to what is stored.
	if reflect.DeepEqual(s.prefs, p) {
		old := s.prefs
		s.prefs = s.persisted.Clone()
		if old.Language != s.prefs.Language {
			s.applyLocale()
		}
		if old.Appearance != s.prefs.Appearance {
			s.applyAppearance()
		}
		if old.AutomaticallyChecksForUpdates != s.prefs.AutomaticallyChecksForUpdates && s.sys.SetAutomaticUpdateChecks != nil {
			s.sys.SetAutomaticUpdateChecks(s.prefs.AutomaticallyChecksForUpdates)
		}
	}
	s.changed()
}

// Flush runs on the UI thread, waits for disk writes, then applies their
// results directly so quitting never waits for a synchronous UI dispatcher.
func (s *Settings) Flush() {
	s.loginIdle.Wait()
	// Quitting consumes the completed result without starting another query.
	s.loginRefreshAgain = false
	s.deliverLoginResult()
	s.idle.Wait()
	s.deliverSaveResults()
}

// Apply puts the language and appearance in effect: at startup, after Load.
func (s *Settings) Apply() {
	s.applyLocale()
	s.applyAppearance()
}

func (s *Settings) applyLocale() {
	var langs []string
	if s.sys.PreferredLanguages != nil {
		langs = s.sys.PreferredLanguages()
	}
	s.loc = l10n.ForPreference(string(s.prefs.Language), langs)
}

func (s *Settings) applyAppearance() {
	if s.sys.SetAppearance != nil {
		s.sys.SetAppearance(s.prefs.Appearance)
	}
}

// ---- Typed changes: one per preference, as the Swift updateXxx methods ----

// SetSelectedProvider selects the provider the panel shows.
func (s *Settings) SetSelectedProvider(v store.Provider) bool {
	return s.Update(func(p *store.Preferences) { p.SelectedProvider = v })
}

// SetPanelProviders chooses the providers the main panel offers; empty shows
// every provider. A hidden selected provider moves to the first shown one.
func (s *Settings) SetPanelProviders(providers []store.Provider) bool {
	return s.Update(func(p *store.Preferences) { p.PanelProviders = slices.Clone(providers) })
}

// SetCursorMonitoringEnabled opts in to reading the current Cursor login.
func (s *Settings) SetCursorMonitoringEnabled(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.CursorMonitoringEnabled = on })
}

// SetLanguage selects the language; it takes effect at once.
func (s *Settings) SetLanguage(v store.Language) bool {
	return s.Update(func(p *store.Preferences) { p.Language = v })
}

// SetAppearance selects light, dark or the system appearance; it takes
// effect at once.
func (s *Settings) SetAppearance(v store.Appearance) bool {
	return s.Update(func(p *store.Preferences) { p.Appearance = v })
}

// SetThemeColor selects the accent of the app's windows, "#RRGGBB" or empty
// for the brand brown; it takes effect at once.
func (s *Settings) SetThemeColor(v string) bool {
	return s.Update(func(p *store.Preferences) { p.ThemeColor = v })
}

// SetFontFamily selects the font of the app's windows, empty for the
// system's; it takes effect at once.
func (s *Settings) SetFontFamily(v string) bool {
	return s.Update(func(p *store.Preferences) { p.FontFamily = v })
}

// SetPanelBackground selects the main panel background.
func (s *Settings) SetPanelBackground(v store.PanelBackground) bool {
	return s.Update(func(p *store.Preferences) { p.MainPanelBackgroundStyle = v })
}

// SetAccountPrivacyMode turns the account privacy mode on or off.
func (s *Settings) SetAccountPrivacyMode(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.AccountPrivacyModeEnabled = on })
}

// SetStatusBarStyle selects how the status item is drawn.
func (s *Settings) SetStatusBarStyle(v store.StatusBarStyle) bool {
	return s.Update(func(p *store.Preferences) { p.StatusBarDisplayStyle = v })
}

// SetStatusBarMetersDetail selects the detail of the meters style.
func (s *Settings) SetStatusBarMetersDetail(v store.MetersDetail) bool {
	return s.Update(func(p *store.Preferences) { p.StatusBarMetersDetailStyle = v })
}

// SetStatusBarBatteryScope selects the window of the battery style.
func (s *Settings) SetStatusBarBatteryScope(v store.BatteryScope) bool {
	return s.Update(func(p *store.Preferences) { p.StatusBarBatteryScope = v })
}

// SetStatusBarBatteryDetail selects the detail of the battery style.
func (s *Settings) SetStatusBarBatteryDetail(v store.BatteryDetail) bool {
	return s.Update(func(p *store.Preferences) { p.StatusBarBatteryDetailStyle = v })
}

// SetStatusBarProviderScope selects which providers the status item shows.
func (s *Settings) SetStatusBarProviderScope(v store.StatusBarProviderScope) bool {
	return s.Update(func(p *store.Preferences) {
		p.StatusBarProviderScope = v
		draft := *p
		draft.StatusBarProviderScope = v
		if v == store.ProviderScopeCustom && len(draft.StatusBarSelection()) == 0 {
			pick := p.SelectedProvider
			if !pick.Valid() {
				pick = store.ProviderCodex
			}
			p.StatusBarProviders = []store.Provider{pick}
		}
	})
}

// SetStatusBarProviders stores a custom status-bar selection of one or two providers.
func (s *Settings) SetStatusBarProviders(providers []store.Provider) bool {
	return s.Update(func(p *store.Preferences) {
		if codexAndGrokOnly(providers) {
			p.StatusBarProviderScope = store.ProviderScopeBoth
			p.StatusBarProviders = nil
			return
		}
		p.StatusBarProviderScope = store.ProviderScopeCustom
		p.StatusBarProviders = append([]store.Provider(nil), providers...)
	})
}

func codexAndGrokOnly(providers []store.Provider) bool {
	seen := map[store.Provider]bool{}
	for _, p := range providers {
		if p.Valid() {
			seen[p] = true
		}
	}
	return len(seen) == 2 && seen[store.ProviderCodex] && seen[store.ProviderGrok]
}

// SetRefreshInterval sets the refresh interval, limited to 60...1800 s.
func (s *Settings) SetRefreshInterval(seconds int) bool {
	return s.Update(func(p *store.Preferences) { p.RefreshIntervalSeconds = max(60, min(1_800, seconds)) })
}

// SetWidgetRefreshInterval sets the widget refresh interval (clamped).
func (s *Settings) SetWidgetRefreshInterval(seconds int) bool {
	return s.Update(func(p *store.Preferences) { p.WidgetRefreshIntervalSeconds = seconds })
}

// SetRateLimitResetTodayRefreshInterval sets the interval of the reset
// dynamics refresh (clamped).
func (s *Settings) SetRateLimitResetTodayRefreshInterval(seconds int) bool {
	return s.Update(func(p *store.Preferences) { p.RateLimitResetTodayRefreshIntervalSeconds = seconds })
}

// SetAPICostRange selects the range of the API cost module.
func (s *Settings) SetAPICostRange(v store.CostRange) bool {
	return s.Update(func(p *store.Preferences) { p.APICostSummaryRange = v })
}

// SetMainPanelHeight saves the preferred height; the UI applies screen limits.
func (s *Settings) SetMainPanelHeight(h float64) bool {
	return s.Update(func(p *store.Preferences) { p.MainPanelHeight = h })
}

// SetDisableMainPanelAnimation skips or restores the native panel animation.
func (s *Settings) SetDisableMainPanelAnimation(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.DisableMainPanelAnimation = on })
}

// MoveMainPanelModule moves a module up (-1) or down (+1) in the panel.
func (s *Settings) MoveMainPanelModule(m store.Module, offset int) bool {
	return s.Update(func(p *store.Preferences) { p.MoveMainPanelModule(m, offset) })
}

// ResetMainPanelModuleOrder restores the default module order.
func (s *Settings) ResetMainPanelModuleOrder() bool {
	return s.Update(func(p *store.Preferences) { p.ResetMainPanelModuleOrder() })
}

// SetQuotaEstimateWindowMode selects the window of the quota estimate.
func (s *Settings) SetQuotaEstimateWindowMode(v store.WindowMode) bool {
	return s.Update(func(p *store.Preferences) { p.QuotaEstimateWindowMode = v })
}

// SetProxyTestURL saves the site "Test Connection" fetches; the default is
// stored as empty.
func (s *Settings) SetProxyTestURL(v string) bool {
	return s.Update(func(p *store.Preferences) { p.ProxyTestURL = v })
}

// SetQuotaResetTimeFormat selects how the quota rows show the next reset.
func (s *Settings) SetQuotaResetTimeFormat(v store.ResetTimeFormat) bool {
	return s.Update(func(p *store.Preferences) { p.QuotaResetTimeFormat = v })
}

// SetTokenUsageChartStyle selects the token usage chart style.
func (s *Settings) SetTokenUsageChartStyle(v store.ChartStyle) bool {
	return s.Update(func(p *store.Preferences) { p.TokenUsageChartStyle = v })
}

// SetShowsModelSpecificQuotaUsage shows the per-model quota usage.
func (s *Settings) SetShowsModelSpecificQuotaUsage(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.ShowsModelSpecificQuotaUsage = on })
}

// SetShowsCreditsBalance toggles displaying the ChatGPT credits balance.
func (s *Settings) SetShowsCreditsBalance(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.ShowsCreditsBalance = on })
}

// moduleShown returns the visibility switch of a main panel module.
func moduleShown(p *store.Preferences, m store.Module) *bool {
	switch m {
	case store.ModuleQuota:
		return &p.ShowsQuotaSummary
	case store.ModuleTokenUsage:
		return &p.ShowsTokenUsageHeatmap
	case store.ModuleSpeedAnalysis:
		return &p.ShowsSpeedAnalysis
	case store.ModuleSkills:
		return &p.ShowsSkills
	case store.ModuleRateLimitResetToday:
		return &p.ShowsRateLimitResetToday
	case store.ModuleQuotaEstimate:
		return &p.ShowsQuotaEstimateSummary
	case store.ModuleResetCredits:
		return &p.ShowsResetCreditsSummary
	case store.ModuleCacheAnalysis:
		return &p.ShowsCacheAnalysis
	case store.ModuleAPICost:
		return &p.ShowsCostSummary
	case store.ModuleSessionRepair:
		return &p.ShowsSessionRepairSummary
	case store.ModuleRecentSessions:
		return &p.ShowsRecentSessions
	}
	return nil
}

// ModuleShown reports whether a module is shown in the main panel.
func ModuleShown(p store.Preferences, m store.Module) bool {
	if v := moduleShown(&p, m); v != nil {
		return *v
	}
	return false
}

// SetModuleShown shows or hides a module of the main panel.
func (s *Settings) SetModuleShown(m store.Module, on bool) bool {
	return s.Update(func(p *store.Preferences) {
		if v := moduleShown(p, m); v != nil {
			*v = on
		}
	})
}

// SetQuotaAlerts turns the quota alerts on or off.
func (s *Settings) SetQuotaAlerts(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.QuotaAlertsEnabled = on })
}

// SetResetCreditAlerts turns the reset credit alerts on or off.
func (s *Settings) SetResetCreditAlerts(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.ResetCreditAlertsEnabled = on })
}

// SetRateLimitResetTodayAlerts turns the reset dynamics alerts on or off.
func (s *Settings) SetRateLimitResetTodayAlerts(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.RateLimitResetTodayAlertsEnabled = on })
}

// SetExportsStatusJSON turns the status JSON export on or off.
func (s *Settings) SetExportsStatusJSON(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.ExportsStatusJSON = on })
}

// SetAutomaticUpdateChecks turns the automatic update check on or off and
// tells the updater.
func (s *Settings) SetAutomaticUpdateChecks(on bool) bool {
	return s.Update(func(p *store.Preferences) { p.AutomaticallyChecksForUpdates = on })
}

// ---- Launch at login ----

// LoginSwitch reads the confirmed system snapshot, or the pending target.
// Before the first status observation it displays the stored choice.
func (s *Settings) LoginSwitch() bool {
	if s.loginTarget != nil {
		return *s.loginTarget
	}
	if s.loginKnown {
		return s.loginStatus.Registered()
	}
	return s.prefs.LaunchAtLoginEnabled
}

// LoginAvailable reports whether the login item can be changed here.
func (s *Settings) LoginAvailable() bool {
	return s.sys.LoginSupported && s.sys.SetOpenAtLogin != nil
}

// SetLaunchAtLogin changes the system's login item and, when that worked,
// the stored choice (marking it as made). A failure keeps the stored value
// and is shown with LoginError.
func (s *Settings) SetLaunchAtLogin(on bool) bool {
	if s.loadErr != nil {
		return false
	}
	if !s.LoginAvailable() {
		return false
	}
	st, err := s.applyLogin(on)
	s.loginStatus, s.loginKnown = st, st != LoginUnavailable
	if err != nil {
		s.loginErr = err
		return false
	}
	s.loginErr = nil
	return s.Update(func(p *store.Preferences) {
		p.LaunchAtLoginEnabled = on
		p.LaunchAtLoginInitialized = true
	})
}

// ReconcileLogin brings the login item and the stored choice together at
// startup, as the Swift app does. A choice never made registers the default
// (on); a choice made follows the system, so a login item the user turned
// off in the system settings is not turned on again. Failures are shown
// with LoginError and never block the app.
func (s *Settings) ReconcileLogin() error {
	if s.loadErr != nil || !s.LoginAvailable() {
		return nil
	}
	if !s.prefs.LaunchAtLoginInitialized {
		if !s.SetLaunchAtLogin(s.prefs.LaunchAtLoginEnabled) {
			return s.loginErr
		}
		return nil
	}
	st, err := s.readLogin()
	if err != nil {
		s.loginErr = err
		return err
	}
	s.loginErr = nil
	s.loginStatus, s.loginKnown = st, true
	on := st.Registered()
	if on != s.prefs.LaunchAtLoginEnabled {
		s.Update(func(p *store.Preferences) { p.LaunchAtLoginEnabled = on })
	}
	return nil
}
