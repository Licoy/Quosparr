package store

import (
	"encoding/json"
	"fmt"
	"slices"
)

// DecodePreferences reads the stored "runway.preferences" JSON with the Swift
// decoder's rules:
//
//   - a missing or null key gives that field's default (an explicit false is a
//     value, not a missing key);
//   - integer fields accept 300, 300.0 and 3e2, but not 300.5;
//   - the quota estimate window mode accepts anything and falls back to auto,
//     which also maps the legacy "officialWeek";
//   - unknown module names are dropped, and the order is repaired;
//   - numeric fields are clamped to their ranges;
//   - keys this version does not know are kept for EncodePreferences.
//
// Where the Swift decoder throws (and the Swift app then resets everything),
// DecodePreferences returns an error wrapping ErrCorrupt: invalid JSON, a
// non-object document, a wrong JSON type, or an unknown enum value.
func DecodePreferences(data []byte) (Preferences, error) {
	rest, err := parseObject(data)
	if err != nil {
		return Preferences{}, fmt.Errorf("preferences: %w", err)
	}
	r := &reader{rest: rest}
	p := readPreferences(r)
	if r.err != nil {
		return Preferences{}, fmt.Errorf("preferences: %w", r.err)
	}
	p.Normalize()
	if len(r.rest) > 0 {
		p.extra = r.rest
	}
	return p, nil
}

// readPreferences reads every known key; the order follows the Swift struct.
func readPreferences(r *reader) Preferences {
	p := DefaultPreferences()
	p.SelectedProvider = readEnum(r, "selectedProvider", p.SelectedProvider, Provider.Valid)
	p.Language = readEnum(r, "language", p.Language, Language.Valid)
	p.Appearance = readEnum(r, "appearance", p.Appearance, Appearance.Valid)
	p.MainPanelBackgroundStyle = readEnum(r, "mainPanelBackgroundStyle", p.MainPanelBackgroundStyle, PanelBackground.Valid)
	p.StatusBarDisplayStyle = readEnum(r, "statusBarDisplayStyle", p.StatusBarDisplayStyle, StatusBarStyle.Valid)
	p.StatusBarMetersDetailStyle = readEnum(r, "statusBarMetersDetailStyle", p.StatusBarMetersDetailStyle, MetersDetail.Valid)
	p.StatusBarBatteryScope = readEnum(r, "statusBarBatteryScope", p.StatusBarBatteryScope, BatteryScope.Valid)
	p.StatusBarBatteryDetailStyle = readEnum(r, "statusBarBatteryDetailStyle", p.StatusBarBatteryDetailStyle, BatteryDetail.Valid)
	p.StatusBarProviderScope = readEnum(r, "statusBarProviderScope", p.StatusBarProviderScope, StatusBarProviderScope.Valid)
	p.StatusBarProviders = readProviders(r, "statusBarProviders")
	p.RefreshIntervalSeconds = r.int("refreshIntervalSeconds", p.RefreshIntervalSeconds)
	p.WidgetRefreshIntervalSeconds = r.int("widgetRefreshIntervalSeconds", p.WidgetRefreshIntervalSeconds)
	p.APICostSummaryRange = readEnum(r, "apiCostSummaryRange", p.APICostSummaryRange, CostRange.Valid)
	p.MainPanelHeight = r.float("mainPanelHeight", p.MainPanelHeight)
	p.MainPanelModuleOrder = readModules(r, "mainPanelModuleOrder", p.MainPanelModuleOrder)
	p.ShowsQuotaSummary = r.bool("showsQuotaSummary", p.ShowsQuotaSummary)
	p.ShowsResetCreditsSummary = r.bool("showsResetCreditsSummary", p.ShowsResetCreditsSummary)
	p.ShowsQuotaEstimateSummary = r.bool("showsQuotaEstimateSummary", p.ShowsQuotaEstimateSummary)
	p.QuotaEstimateWindowMode = readWindowMode(r, "quotaEstimateWindowMode")
	p.ShowsCostSummary = r.bool("showsCostSummary", p.ShowsCostSummary)
	p.ShowsRecentSessions = r.bool("showsRecentSessions", p.ShowsRecentSessions)
	p.ShowsSessionRepairSummary = r.bool("showsSessionRepairSummary", p.ShowsSessionRepairSummary)
	p.ShowsRateLimitResetToday = r.bool("showsRateLimitResetToday", p.ShowsRateLimitResetToday)
	p.ShowsModelSpecificQuotaUsage = r.bool("showsModelSpecificQuotaUsage", p.ShowsModelSpecificQuotaUsage)
	p.ShowsTokenUsageHeatmap = r.bool("showsTokenUsageHeatmap", p.ShowsTokenUsageHeatmap)
	p.TokenUsageChartStyle = readEnum(r, "tokenUsageChartStyle", p.TokenUsageChartStyle, ChartStyle.Valid)
	p.RateLimitResetTodayRefreshIntervalSeconds = r.int("rateLimitResetTodayRefreshIntervalSeconds", p.RateLimitResetTodayRefreshIntervalSeconds)
	p.AutomaticallyChecksForUpdates = r.bool("automaticallyChecksForUpdates", p.AutomaticallyChecksForUpdates)
	p.LaunchAtLoginEnabled = r.bool("launchAtLoginEnabled", p.LaunchAtLoginEnabled)
	p.LaunchAtLoginInitialized = r.bool("launchAtLoginInitialized", p.LaunchAtLoginInitialized)
	p.QuotaAlertsEnabled = r.bool("quotaAlertsEnabled", p.QuotaAlertsEnabled)
	p.QuotaAlertThresholds = readQuotaAlertThresholds(r)
	p.ResetCreditAlertsEnabled = r.bool("resetCreditAlertsEnabled", p.ResetCreditAlertsEnabled)
	p.RateLimitResetTodayAlertsEnabled = r.bool("rateLimitResetTodayAlertsEnabled", p.RateLimitResetTodayAlertsEnabled)
	p.ExportsStatusJSON = r.bool("exportsStatusJSON", p.ExportsStatusJSON)
	p.AccountPrivacyModeEnabled = r.bool("accountPrivacyModeEnabled", p.AccountPrivacyModeEnabled)
	p.ThemeColor = readThemeColor(r, "themeColor")
	p.FontFamily = readFontFamily(r, "fontFamily")
	p.QuotaResetTimeFormat = readResetTimeFormat(r, "quotaResetTimeFormat")
	p.ProxyTestURL = readProxyTestURL(r, "proxyTestURL")
	p.ShowsSpeedAnalysis = r.bool("showsSpeedAnalysis", p.ShowsSpeedAnalysis)
	p.ShowsCacheAnalysis = r.bool("showsCacheAnalysis", p.ShowsCacheAnalysis)
	p.ShowsCreditsBalance = r.bool("showsCreditsBalance", p.ShowsCreditsBalance)
	p.ShowsSkills = r.bool("showsSkills", p.ShowsSkills)
	p.CursorMonitoringEnabled = r.bool("cursorMonitoringEnabled", false)
	p.FloatingWindow = readFloatingWindow(r, "floatingWindow")
	p.PanelProviders = readPanelProviders(r, "panelProviders")
	p.DisableMainPanelAnimation = r.bool("disableMainPanelAnimation", p.DisableMainPanelAnimation)
	return p
}

// Validate reports invalid enums and quota alert thresholds. EncodePreferences
// refuses these values rather than silently replacing them with defaults.
func (p Preferences) Validate() error {
	checks := []struct {
		key string
		ok  bool
	}{
		{"selectedProvider", p.SelectedProvider.Valid()},
		{"language", p.Language.Valid()},
		{"appearance", p.Appearance.Valid()},
		{"mainPanelBackgroundStyle", p.MainPanelBackgroundStyle.Valid()},
		{"statusBarDisplayStyle", p.StatusBarDisplayStyle.Valid()},
		{"statusBarMetersDetailStyle", p.StatusBarMetersDetailStyle.Valid()},
		{"statusBarBatteryScope", p.StatusBarBatteryScope.Valid()},
		{"statusBarBatteryDetailStyle", p.StatusBarBatteryDetailStyle.Valid()},
		{"statusBarProviderScope", p.StatusBarProviderScope.Valid()},
		{"apiCostSummaryRange", p.APICostSummaryRange.Valid()},
		{"quotaEstimateWindowMode", p.QuotaEstimateWindowMode.Valid()},
		{"tokenUsageChartStyle", p.TokenUsageChartStyle.Valid()},
	}
	for _, c := range checks {
		if !c.ok {
			return fmt.Errorf("preferences: invalid value for %q", c.key)
		}
	}
	if _, err := NormalizeQuotaAlertThresholds(p.QuotaAlertThresholds); err != nil {
		return fmt.Errorf("preferences: quotaAlertThresholds: %w", err)
	}
	return nil
}

// EncodePreferences returns the JSON the Swift RunwayPreferences decoder reads
// back to the same values. The settings are normalized first (clamped, module
// order repaired); an invalid enum value is an error. Unknown keys kept by
// DecodePreferences are written back unchanged. Keys are sorted; the output is
// compact.
func EncodePreferences(p Preferences) ([]byte, error) {
	p = p.Clone()
	p.Normalize()
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return encodeFields(p, p.extra, p.omitGoOnlyDefaults)
}

// omitGoOnlyDefaults keeps what only this app knows out of the document while
// it is at its default, so a settings document nobody changed is byte for byte
// the one the Swift app wrote: showsSpeedAnalysis is written only when off, and
// the speedAnalysis module only where reading would not put it back anyway.
func (p Preferences) omitGoOnlyDefaults(m fields) {
	if slices.Equal(p.QuotaAlertThresholds, DefaultQuotaAlertThresholds()) {
		delete(m, "quotaAlertThresholds")
	}
	if p.ShowsSpeedAnalysis {
		delete(m, "showsSpeedAnalysis")
	}
	if p.ShowsCacheAnalysis {
		delete(m, "showsCacheAnalysis")
	}
	if p.ShowsCreditsBalance {
		delete(m, "showsCreditsBalance")
	}
	if p.ShowsSkills {
		delete(m, "showsSkills")
	}
	order := wireModuleOrder(p.MainPanelModuleOrder)
	if len(order) == len(p.MainPanelModuleOrder) {
		return
	}
	if raw, err := json.Marshal(order); err == nil {
		m["mainPanelModuleOrder"] = raw
	}
}

// goOnlyModules are the modules the Swift app has no case for.
var goOnlyModules = []Module{ModuleSpeedAnalysis, ModuleCacheAnalysis, ModuleSkills}

// wireModuleOrder is the order as written: a Go-only module that
// NormalizeModuleOrder would put at its place again is left out, so the Swift
// app, which drops module names it does not know, still sees the order it
// wrote. A module the user moved elsewhere stays in.
func wireModuleOrder(order []Module) []Module {
	full := order
	for _, m := range goOnlyModules {
		at := slices.Index(order, m)
		if at < 0 {
			continue
		}
		rest := slices.Delete(slices.Clone(order), at, at+1)
		if slices.Equal(NormalizeModuleOrder(rest), full) {
			order = rest
		}
	}
	return order
}
