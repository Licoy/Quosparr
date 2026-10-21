// Package store is the compatibility layer for Quosparr's user preferences
// and network proxy settings.
//
// The Swift app keeps both as JSON Data inside its UserDefaults domain
// (com.quosparr.app). The Go app reads and writes the same bytes through
// the system API on macOS, and through JSON files in ~/.quosparr on
// Windows and Linux. The JSON schema, key names, enum values, defaults and
// normalization rules are a contract with the Swift app and must not drift.
//
// Neither value holds a secret: the proxy password lives in the system
// credential store and is only referenced by an opaque id.
package store

import (
	"encoding/json"
	"errors"
	"maps"
	"math"
	"net/url"
	"slices"
	"strings"
	"unicode"
)

// Defaults and limits shared with the Swift app.
const (
	DefaultRefreshIntervalSeconds                    = 300
	DefaultWidgetRefreshIntervalSeconds              = 60
	MinWidgetRefreshIntervalSeconds                  = 60
	MaxWidgetRefreshIntervalSeconds                  = 1_800
	DefaultRateLimitResetTodayRefreshIntervalSeconds = 300
	MinRateLimitResetTodayRefreshIntervalSeconds     = 300
	MaxRateLimitResetTodayRefreshIntervalSeconds     = 21_600
	// Zero keeps the default height relative to the current screen.
	DefaultMainPanelHeight = 0.0
	MinMainPanelHeight     = 360.0
	DefaultProxyTestURL    = "https://github.com"
)

// WidgetRefreshIntervalOptions are the intervals the settings UI offers for
// the widget. Stored values outside the list are kept as long as they are in
// range, exactly as the Swift app does.
func WidgetRefreshIntervalOptions() []int { return []int{60, 300, 600, 900, 1_800} }

// RateLimitResetTodayRefreshIntervalOptions are the intervals the settings UI
// offers for the reset-today module.
func RateLimitResetTodayRefreshIntervalOptions() []int {
	return []int{300, 600, 900, 1_800, 3_600, 7_200, 21_600}
}

// Preferences is the general settings document (UserDefaults key
// "runway.preferences"). The json tags are the on-disk key names. Use
// DecodePreferences and EncodePreferences to read and write the stored form:
// they keep unknown keys and reject what the Swift app would reject.
type Preferences struct {
	SelectedProvider            Provider               `json:"selectedProvider"`
	Language                    Language               `json:"language"`
	Appearance                  Appearance             `json:"appearance"`
	MainPanelBackgroundStyle    PanelBackground        `json:"mainPanelBackgroundStyle"`
	StatusBarDisplayStyle       StatusBarStyle         `json:"statusBarDisplayStyle"`
	StatusBarMetersDetailStyle  MetersDetail           `json:"statusBarMetersDetailStyle"`
	StatusBarBatteryScope       BatteryScope           `json:"statusBarBatteryScope"`
	StatusBarBatteryDetailStyle BatteryDetail          `json:"statusBarBatteryDetailStyle"`
	StatusBarProviderScope      StatusBarProviderScope `json:"statusBarProviderScope"`
	// StatusBarProviders is the custom status-bar selection, one or two
	// providers in Providers() order. It is omitted unless the scope is custom.
	StatusBarProviders                        []Provider `json:"statusBarProviders,omitempty"`
	RefreshIntervalSeconds                    int        `json:"refreshIntervalSeconds"`
	WidgetRefreshIntervalSeconds              int        `json:"widgetRefreshIntervalSeconds"`
	APICostSummaryRange                       CostRange  `json:"apiCostSummaryRange"`
	MainPanelHeight                           float64    `json:"mainPanelHeight"`
	MainPanelModuleOrder                      []Module   `json:"mainPanelModuleOrder"`
	ShowsQuotaSummary                         bool       `json:"showsQuotaSummary"`
	ShowsResetCreditsSummary                  bool       `json:"showsResetCreditsSummary"`
	ShowsQuotaEstimateSummary                 bool       `json:"showsQuotaEstimateSummary"`
	QuotaEstimateWindowMode                   WindowMode `json:"quotaEstimateWindowMode"`
	ShowsCostSummary                          bool       `json:"showsCostSummary"`
	ShowsRecentSessions                       bool       `json:"showsRecentSessions"`
	ShowsSessionRepairSummary                 bool       `json:"showsSessionRepairSummary"`
	ShowsRateLimitResetToday                  bool       `json:"showsRateLimitResetToday"`
	ShowsModelSpecificQuotaUsage              bool       `json:"showsModelSpecificQuotaUsage"`
	ShowsTokenUsageHeatmap                    bool       `json:"showsTokenUsageHeatmap"`
	TokenUsageChartStyle                      ChartStyle `json:"tokenUsageChartStyle"`
	RateLimitResetTodayRefreshIntervalSeconds int        `json:"rateLimitResetTodayRefreshIntervalSeconds"`
	AutomaticallyChecksForUpdates             bool       `json:"automaticallyChecksForUpdates"`
	LaunchAtLoginEnabled                      bool       `json:"launchAtLoginEnabled"`
	LaunchAtLoginInitialized                  bool       `json:"launchAtLoginInitialized"`
	QuotaAlertsEnabled                        bool       `json:"quotaAlertsEnabled"`
	// QuotaAlertThresholds contains used-quota percentages; default values
	// are omitted from the stored document for Swift compatibility.
	QuotaAlertThresholds             []int `json:"quotaAlertThresholds"`
	ResetCreditAlertsEnabled         bool  `json:"resetCreditAlertsEnabled"`
	RateLimitResetTodayAlertsEnabled bool  `json:"rateLimitResetTodayAlertsEnabled"`
	ExportsStatusJSON                bool  `json:"exportsStatusJSON"`
	AccountPrivacyModeEnabled        bool  `json:"accountPrivacyModeEnabled"`
	// ThemeColor is the accent of the app's windows as "#RRGGBB", or empty
	// for the brand brown. Only the Go app knows the key, so it is omitted
	// while empty and an unreadable value falls back to the brand.
	ThemeColor string `json:"themeColor,omitempty"`
	// FontFamily is Go-only too: the family the app's windows use, omitted
	// for the system's, and an unusable name falls back to it.
	FontFamily string `json:"fontFamily,omitempty"`
	// QuotaResetTimeFormat is Go-only too: omitted for the relative countdown,
	// and an unreadable value falls back to it.
	QuotaResetTimeFormat ResetTimeFormat `json:"quotaResetTimeFormat,omitempty"`
	// ProxyTestURL is Go-only as well: the site "Test Connection" fetches,
	// omitted for DefaultProxyTestURL, and an unreadable value falls back to it.
	ProxyTestURL string `json:"proxyTestURL,omitempty"`
	// ShowsSpeedAnalysis is Go-only: the speed analysis module, on by default
	// and written only when turned off.
	ShowsSpeedAnalysis bool `json:"showsSpeedAnalysis"`
	// ShowsCacheAnalysis is Go-only: the cache analysis module, on by default
	// and written only when turned off.
	ShowsCacheAnalysis bool `json:"showsCacheAnalysis"`
	// ShowsCreditsBalance is Go-only: display ChatGPT credits balance as the
	// last row of the Codex quota section when available. On by default and written only when
	// turned off.
	ShowsCreditsBalance bool `json:"showsCreditsBalance"`
	// ShowsSkills is Go-only: the skills module, on by default and written
	// only when turned off.
	ShowsSkills bool `json:"showsSkills"`
	// CursorMonitoringEnabled opts in to read-only access to Cursor's current login.
	CursorMonitoringEnabled bool `json:"cursorMonitoringEnabled,omitempty"`
	// FloatingWindow is Go-only: the desktop floating window, omitted while
	// it is at its default.
	FloatingWindow FloatingWindow `json:"floatingWindow,omitzero"`
	// PanelProviders is Go-only: the providers the main panel offers, in
	// Providers() order. Empty shows every provider and is not written.
	PanelProviders []Provider `json:"panelProviders,omitempty"`
	// DisableMainPanelAnimation is Go-only: skip the native macOS popover
	// animation when it opens. It is omitted while off.
	DisableMainPanelAnimation bool `json:"disableMainPanelAnimation,omitempty"`

	// extra holds keys this version does not know, written back unchanged so
	// that older and newer apps sharing the domain do not erase each other's
	// settings.
	extra map[string]json.RawMessage
}

// DefaultPreferences returns the settings of a first launch. It matches the
// Swift RunwayPreferences() initializer.
func DefaultPreferences() Preferences {
	return Preferences{
		SelectedProvider:                          ProviderCodex,
		Language:                                  LanguageSystem,
		Appearance:                                AppearanceSystem,
		MainPanelBackgroundStyle:                  PanelBackgroundTranslucent,
		StatusBarDisplayStyle:                     StatusBarStyleMeters,
		StatusBarMetersDetailStyle:                MetersDetailRemainingPercent,
		StatusBarBatteryScope:                     BatteryScopeFiveHour,
		StatusBarBatteryDetailStyle:               BatteryDetailCountdown,
		StatusBarProviderScope:                    ProviderScopeSelected,
		RefreshIntervalSeconds:                    DefaultRefreshIntervalSeconds,
		WidgetRefreshIntervalSeconds:              DefaultWidgetRefreshIntervalSeconds,
		APICostSummaryRange:                       CostRangeToday,
		MainPanelHeight:                           DefaultMainPanelHeight,
		MainPanelModuleOrder:                      Modules(),
		ShowsQuotaSummary:                         true,
		ShowsResetCreditsSummary:                  true,
		ShowsQuotaEstimateSummary:                 true,
		QuotaEstimateWindowMode:                   WindowModeAuto,
		ShowsCostSummary:                          true,
		ShowsRecentSessions:                       false,
		ShowsSessionRepairSummary:                 true,
		ShowsRateLimitResetToday:                  true,
		ShowsModelSpecificQuotaUsage:              false,
		ShowsTokenUsageHeatmap:                    true,
		ShowsSpeedAnalysis:                        true,
		ShowsCacheAnalysis:                        true,
		ShowsCreditsBalance:                       true,
		ShowsSkills:                               true,
		TokenUsageChartStyle:                      ChartStyleHeatmap,
		RateLimitResetTodayRefreshIntervalSeconds: DefaultRateLimitResetTodayRefreshIntervalSeconds,
		AutomaticallyChecksForUpdates:             true,
		LaunchAtLoginEnabled:                      true,
		LaunchAtLoginInitialized:                  false,
		QuotaAlertsEnabled:                        false,
		QuotaAlertThresholds:                      DefaultQuotaAlertThresholds(),
		ResetCreditAlertsEnabled:                  false,
		RateLimitResetTodayAlertsEnabled:          true,
		ExportsStatusJSON:                         false,
		AccountPrivacyModeEnabled:                 false,
	}
}

// Clone returns a deep copy.
func (p Preferences) Clone() Preferences {
	p.MainPanelModuleOrder = slices.Clone(p.MainPanelModuleOrder)
	p.StatusBarProviders = slices.Clone(p.StatusBarProviders)
	p.FloatingWindow = p.FloatingWindow.clone()
	p.PanelProviders = slices.Clone(p.PanelProviders)
	p.QuotaAlertThresholds = slices.Clone(p.QuotaAlertThresholds)
	p.extra = maps.Clone(p.extra)
	return p
}

// StatusBarSelection is the status item's providers. Nil means follow the
// main panel. Legacy both is Codex then Grok. Custom is the stored list.
func (p Preferences) StatusBarSelection() []Provider {
	switch p.StatusBarProviderScope {
	case ProviderScopeBoth:
		return []Provider{ProviderCodex, ProviderGrok}
	case ProviderScopeCustom:
		return canonicalStatusBarProviders(p.StatusBarProviders)
	default:
		return nil
	}
}

// canonicalStatusBarProviders keeps one or two known providers, drops
// duplicates, and sorts them into Providers() order.
func canonicalStatusBarProviders(in []Provider) []Provider { return canonicalProviders(in, 2) }

// canonicalProviders keeps up to limit known providers, drops duplicates,
// and sorts them into Providers() order.
func canonicalProviders(in []Provider, limit int) []Provider {
	seen := map[Provider]bool{}
	for _, p := range in {
		if p.Valid() {
			seen[p] = true
		}
	}
	out := make([]Provider, 0, limit)
	for _, p := range Providers() {
		if seen[p] {
			out = append(out, p)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

// Normalize applies the clamping and module-order repair the Swift initializer
// and decoder apply. It does not touch enum fields; see Validate. Quota alert
// thresholds are sorted and deduplicated; invalid lists stay for Validate.
func (p *Preferences) Normalize() {
	p.WidgetRefreshIntervalSeconds = ClampWidgetRefreshInterval(p.WidgetRefreshIntervalSeconds)
	p.MainPanelHeight = ClampMainPanelHeight(p.MainPanelHeight)
	p.MainPanelModuleOrder = NormalizeModuleOrder(p.MainPanelModuleOrder)
	p.RateLimitResetTodayRefreshIntervalSeconds =
		ClampRateLimitResetTodayRefreshInterval(p.RateLimitResetTodayRefreshIntervalSeconds)
	p.ThemeColor = NormalizeThemeColor(p.ThemeColor)
	p.FontFamily = NormalizeFontFamily(p.FontFamily)
	if !p.QuotaResetTimeFormat.Valid() {
		p.QuotaResetTimeFormat = ResetTimeRelative
	}
	p.ProxyTestURL = NormalizeProxyTestURL(p.ProxyTestURL)
	p.normalizeStatusBarProviders()
	p.FloatingWindow.normalize()
	p.normalizePanelProviders()
	if p.QuotaAlertThresholds == nil {
		p.QuotaAlertThresholds = DefaultQuotaAlertThresholds()
	} else if thresholds, err := NormalizeQuotaAlertThresholds(p.QuotaAlertThresholds); err == nil {
		p.QuotaAlertThresholds = thresholds
	}
}

// PanelSelection is the providers the main panel offers: the chosen ones,
// or every provider.
func (p Preferences) PanelSelection() []Provider {
	if sel := canonicalProviders(p.PanelProviders, len(Providers())); len(sel) > 0 {
		return sel
	}
	return Providers()
}

// normalizePanelProviders keeps the custom panel list canonical, empty for
// every provider, and moves the selected provider onto a shown one.
func (p *Preferences) normalizePanelProviders() {
	p.PanelProviders = canonicalProviders(p.PanelProviders, len(Providers()))
	if len(p.PanelProviders) == 0 {
		p.PanelProviders = nil
		return
	}
	if !slices.Contains(p.PanelProviders, p.SelectedProvider) {
		p.SelectedProvider = p.PanelProviders[0]
	}
}

// normalizeStatusBarProviders applies the custom-list rules. Legacy both is
// left as both so a frozen settings document still round-trips. An empty or
// invalid custom list falls back to following the main panel.
func (p *Preferences) normalizeStatusBarProviders() {
	switch p.StatusBarProviderScope {
	case ProviderScopeBoth, ProviderScopeSelected:
		p.StatusBarProviders = nil
	case ProviderScopeCustom:
		p.StatusBarProviders = canonicalStatusBarProviders(p.StatusBarProviders)
		if len(p.StatusBarProviders) == 0 {
			p.StatusBarProviderScope = ProviderScopeSelected
			p.StatusBarProviders = nil
		}
	default:
		p.StatusBarProviders = nil
	}
}

// ErrInvalidProxyTestURL rejects a test site that is not an http(s) URL with
// a host.
var ErrInvalidProxyTestURL = errors.New("invalid proxy test URL")

// ParseProxyTestURL checks a test site typed by the user and returns it in
// canonical form. A bare host gets "https://"; user info is refused so no
// password ends up in the preferences.
func ParseProxyTestURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 2_048 || strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", ErrInvalidProxyTestURL
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return "", ErrInvalidProxyTestURL
	}
	return u.String(), nil
}

// NormalizeProxyTestURL returns the stored form of a test site: empty for
// the default or an invalid value.
func NormalizeProxyTestURL(raw string) string {
	s, err := ParseProxyTestURL(raw)
	if err != nil || s == DefaultProxyTestURL || s == DefaultProxyTestURL+"/" {
		return ""
	}
	return s
}

// ProxyTestTarget is the site "Test Connection" fetches.
func (p Preferences) ProxyTestTarget() string {
	if p.ProxyTestURL == "" {
		return DefaultProxyTestURL
	}
	return p.ProxyTestURL
}

// NormalizeThemeColor returns color as an upper-case "#RRGGBB", or empty
// when it is not one.
func NormalizeThemeColor(color string) string {
	if len(color) != 7 || color[0] != '#' {
		return ""
	}
	for _, r := range color[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return ""
		}
	}
	return strings.ToUpper(color)
}

// NormalizeFontFamily returns a font family name trimmed, or empty for the
// system's font when it is not one name: too long, or with a comma, which
// would make it a list, or a control character.
func NormalizeFontFamily(family string) string {
	s := strings.TrimSpace(family)
	if len(s) > 256 || strings.ContainsFunc(s, func(r rune) bool { return r == ',' || unicode.IsControl(r) }) {
		return ""
	}
	return s
}

// ClampWidgetRefreshInterval limits the widget refresh interval to 60...1800 s.
func ClampWidgetRefreshInterval(seconds int) int {
	return max(MinWidgetRefreshIntervalSeconds, min(MaxWidgetRefreshIntervalSeconds, seconds))
}

// ClampRateLimitResetTodayRefreshInterval limits the interval to 300...21600 s.
func ClampRateLimitResetTodayRefreshInterval(seconds int) int {
	return max(MinRateLimitResetTodayRefreshIntervalSeconds,
		min(MaxRateLimitResetTodayRefreshIntervalSeconds, seconds))
}

// ClampMainPanelHeight preserves the automatic default and enforces the
// minimum for explicit heights; the UI resolves the screen-dependent size.
func ClampMainPanelHeight(height float64) float64 {
	if height == DefaultMainPanelHeight || math.IsNaN(height) || math.IsInf(height, 0) {
		return DefaultMainPanelHeight
	}
	return max(MinMainPanelHeight, height)
}

// NormalizeModuleOrder drops duplicates and unknown modules, keeps the user's order, and puts each
// missing module back next to its default-order neighbour: just before the
// first later default module that is present, or at the end.
func NormalizeModuleOrder(order []Module) []Module {
	result := make([]Module, 0, len(Modules()))
	for _, m := range order {
		if m.Valid() && !slices.Contains(result, m) {
			result = append(result, m)
		}
	}
	defaults := Modules()
	for i, m := range defaults {
		if slices.Contains(result, m) {
			continue
		}
		result = insertBeforeNeighbor(result, m, defaults[i+1:])
	}
	return result
}

// insertBeforeNeighbor places m before the first of later that is in order, or
// appends it when none is.
func insertBeforeNeighbor(order []Module, m Module, later []Module) []Module {
	for _, n := range later {
		if at := slices.Index(order, n); at >= 0 {
			return slices.Insert(order, at, m)
		}
	}
	return append(order, m)
}

// MoveMainPanelModule moves m by offset positions (swapping with the module
// at the destination). It does nothing when m or the destination is missing.
func (p *Preferences) MoveMainPanelModule(m Module, offset int) {
	order := NormalizeModuleOrder(p.MainPanelModuleOrder)
	from := slices.Index(order, m)
	to := from + offset
	if from < 0 || to < 0 || to >= len(order) {
		return
	}
	order[from], order[to] = order[to], order[from]
	p.MainPanelModuleOrder = order
}

// ResetMainPanelModuleOrder restores the default module order.
func (p *Preferences) ResetMainPanelModuleOrder() { p.MainPanelModuleOrder = Modules() }
