package store

import "slices"

// The enum types below mirror the Swift RunwayPreferences enums. Their string
// values are stored on disk and read by the Swift app, so they must not change.

// Provider is the platform selected in the main panel (Swift RunwayProvider).
type Provider string

// Provider values.
const (
	ProviderCodex  Provider = "codex"
	ProviderClaude Provider = "claude"
	ProviderGrok   Provider = "grok"
	ProviderCursor Provider = "cursor"
)

// Providers returns every valid Provider, in display order.
func Providers() []Provider {
	return []Provider{ProviderCodex, ProviderClaude, ProviderGrok, ProviderCursor}
}

// CLIProviders support account switching, sessions, Skills and MCP management.
func CLIProviders() []Provider { return []Provider{ProviderCodex, ProviderClaude, ProviderGrok} }

// Valid reports whether v is a known value.
func (v Provider) Valid() bool { return slices.Contains(Providers(), v) }

// Language is the user's language preference.
type Language string

// Language values.
const (
	LanguageSystem             Language = "system"
	LanguageEnglish            Language = "english"
	LanguageSimplifiedChinese  Language = "simplifiedChinese"
	LanguageTraditionalChinese Language = "traditionalChinese"
	LanguageKorean             Language = "korean"
	LanguageJapanese           Language = "japanese"
	LanguageRussian            Language = "russian"
	LanguageFrench             Language = "french"
)

// Languages returns every valid Language.
func Languages() []Language {
	return []Language{
		LanguageSystem, LanguageEnglish, LanguageSimplifiedChinese, LanguageTraditionalChinese,
		LanguageKorean, LanguageJapanese, LanguageRussian, LanguageFrench,
	}
}

// Valid reports whether v is a known value.
func (v Language) Valid() bool { return slices.Contains(Languages(), v) }

// Appearance is the light/dark preference.
type Appearance string

// Appearance values.
const (
	AppearanceSystem Appearance = "system"
	AppearanceLight  Appearance = "light"
	AppearanceDark   Appearance = "dark"
)

// Appearances returns every valid Appearance.
func Appearances() []Appearance {
	return []Appearance{AppearanceSystem, AppearanceLight, AppearanceDark}
}

// Valid reports whether v is a known value.
func (v Appearance) Valid() bool { return slices.Contains(Appearances(), v) }

// PanelBackground is the main panel background style.
type PanelBackground string

// PanelBackground values.
const (
	PanelBackgroundTranslucent PanelBackground = "translucent"
	PanelBackgroundOpaque      PanelBackground = "opaque"
)

// PanelBackgrounds returns every valid PanelBackground.
func PanelBackgrounds() []PanelBackground {
	return []PanelBackground{PanelBackgroundTranslucent, PanelBackgroundOpaque}
}

// Valid reports whether v is a known value.
func (v PanelBackground) Valid() bool { return slices.Contains(PanelBackgrounds(), v) }

// StatusBarStyle is how the status item is drawn.
type StatusBarStyle string

// StatusBarStyle values.
const (
	StatusBarStyleMeters    StatusBarStyle = "meters"
	StatusBarStyleCountdown StatusBarStyle = "countdown"
	StatusBarStyleBattery   StatusBarStyle = "battery"
	StatusBarStyleRings     StatusBarStyle = "rings"
	StatusBarStyleText      StatusBarStyle = "text"
)

// StatusBarStyles returns every valid StatusBarStyle.
func StatusBarStyles() []StatusBarStyle {
	return []StatusBarStyle{
		StatusBarStyleMeters, StatusBarStyleCountdown, StatusBarStyleBattery,
		StatusBarStyleRings, StatusBarStyleText,
	}
}

// Valid reports whether v is a known value.
func (v StatusBarStyle) Valid() bool { return slices.Contains(StatusBarStyles(), v) }

// MetersDetail is the detail line of the "meters" status bar style.
type MetersDetail string

// MetersDetail values.
const (
	MetersDetailRemainingPercent MetersDetail = "remainingPercent"
	MetersDetailResetTime        MetersDetail = "resetTime"
	MetersDetailBoth             MetersDetail = "both"
)

// MetersDetails returns every valid MetersDetail.
func MetersDetails() []MetersDetail {
	return []MetersDetail{MetersDetailRemainingPercent, MetersDetailResetTime, MetersDetailBoth}
}

// Valid reports whether v is a known value.
func (v MetersDetail) Valid() bool { return slices.Contains(MetersDetails(), v) }

// BatteryScope is the quota window shown by the "battery" status bar style.
type BatteryScope string

// BatteryScope values.
const (
	BatteryScopeFiveHour BatteryScope = "fiveHour"
	BatteryScopeWeekly   BatteryScope = "weekly"
	BatteryScopeBoth     BatteryScope = "both"
)

// BatteryScopes returns every valid BatteryScope.
func BatteryScopes() []BatteryScope {
	return []BatteryScope{BatteryScopeFiveHour, BatteryScopeWeekly, BatteryScopeBoth}
}

// Valid reports whether v is a known value.
func (v BatteryScope) Valid() bool { return slices.Contains(BatteryScopes(), v) }

// BatteryDetail is the detail text of the "battery" status bar style.
type BatteryDetail string

// BatteryDetail values.
const (
	BatteryDetailCountdown        BatteryDetail = "countdown"
	BatteryDetailRemainingPercent BatteryDetail = "remainingPercent"
)

// BatteryDetails returns every valid BatteryDetail.
func BatteryDetails() []BatteryDetail {
	return []BatteryDetail{BatteryDetailCountdown, BatteryDetailRemainingPercent}
}

// Valid reports whether v is a known value.
func (v BatteryDetail) Valid() bool { return slices.Contains(BatteryDetails(), v) }

// StatusBarProviderScope says which platforms contribute to the status item.
type StatusBarProviderScope string

// StatusBarProviderScope values.
const (
	// ProviderScopeSelected follows the main panel's selected provider.
	ProviderScopeSelected StatusBarProviderScope = "selected"
	// ProviderScopeBoth is the legacy value for Codex and Grok together.
	ProviderScopeBoth StatusBarProviderScope = "both"
	// ProviderScopeCustom uses StatusBarProviders, one or two providers.
	ProviderScopeCustom StatusBarProviderScope = "custom"
)

// StatusBarProviderScopes returns every valid StatusBarProviderScope.
func StatusBarProviderScopes() []StatusBarProviderScope {
	return []StatusBarProviderScope{ProviderScopeSelected, ProviderScopeBoth, ProviderScopeCustom}
}

// Valid reports whether v is a known value.
func (v StatusBarProviderScope) Valid() bool { return slices.Contains(StatusBarProviderScopes(), v) }

// CostRange is the range summarized by the API cost module.
type CostRange string

// CostRange values.
const (
	CostRangeToday     CostRange = "today"
	CostRangeCurrent   CostRange = "current"
	CostRangePrevious  CostRange = "previous"
	CostRangeThisMonth CostRange = "thisMonth"
)

// CostRanges returns every valid CostRange.
func CostRanges() []CostRange {
	return []CostRange{CostRangeToday, CostRangeCurrent, CostRangePrevious, CostRangeThisMonth}
}

// Valid reports whether v is a known value.
func (v CostRange) Valid() bool { return slices.Contains(CostRanges(), v) }

// ChartStyle is the token usage chart style.
type ChartStyle string

// ChartStyle values.
const (
	ChartStyleHeatmap ChartStyle = "heatmap"
	ChartStyleLine    ChartStyle = "line"
	ChartStyleBar     ChartStyle = "bar"
)

// ChartStyles returns every valid ChartStyle.
func ChartStyles() []ChartStyle {
	return []ChartStyle{ChartStyleHeatmap, ChartStyleLine, ChartStyleBar}
}

// Valid reports whether v is a known value.
func (v ChartStyle) Valid() bool { return slices.Contains(ChartStyles(), v) }

// ResetTimeFormat is how the quota rows show the next reset. Only the Go app
// knows it, so the relative countdown is the empty value and is not written.
type ResetTimeFormat string

// ResetTimeFormat values.
const (
	ResetTimeRelative ResetTimeFormat = ""
	ResetTimeExact    ResetTimeFormat = "exact"
)

// ResetTimeFormats returns every valid ResetTimeFormat.
func ResetTimeFormats() []ResetTimeFormat {
	return []ResetTimeFormat{ResetTimeRelative, ResetTimeExact}
}

// Valid reports whether v is a known value.
func (v ResetTimeFormat) Valid() bool { return slices.Contains(ResetTimeFormats(), v) }

// Toggled is the other format.
func (v ResetTimeFormat) Toggled() ResetTimeFormat {
	if v == ResetTimeExact {
		return ResetTimeRelative
	}
	return ResetTimeExact
}

// Module is a section of the main panel.
type Module string

// Module values, in the default panel order.
const (
	ModuleQuota               Module = "quota"
	ModuleTokenUsage          Module = "tokenUsage"
	ModuleSpeedAnalysis       Module = "speedAnalysis"
	ModuleRateLimitResetToday Module = "rateLimitResetToday"
	ModuleQuotaEstimate       Module = "quotaEstimate"
	ModuleResetCredits        Module = "resetCredits"
	ModuleCacheAnalysis       Module = "cacheAnalysis"
	ModuleAPICost             Module = "apiCost"
	ModuleSessionRepair       Module = "sessionRepair"
	ModuleRecentSessions      Module = "recentSessions"
	ModuleSkills              Module = "skills"
)

// Modules returns every Module in the default panel order.
func Modules() []Module {
	return []Module{
		ModuleQuota, ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate,
		ModuleResetCredits, ModuleCacheAnalysis, ModuleAPICost, ModuleSessionRepair, ModuleRecentSessions, ModuleSkills,
	}
}

// Valid reports whether v is a known value.
func (v Module) Valid() bool { return slices.Contains(Modules(), v) }

// WindowMode is how the quota estimate picks its window. Unlike the other
// enums it never fails to decode: any other stored value, including the legacy
// "officialWeek", reads as WindowModeAuto.
type WindowMode string

// WindowMode values.
const (
	WindowModeAuto        WindowMode = "auto"
	WindowModeRollingWeek WindowMode = "rollingWeek"
)

// WindowModes returns every valid WindowMode.
func WindowModes() []WindowMode { return []WindowMode{WindowModeAuto, WindowModeRollingWeek} }

// Valid reports whether v is a known value.
func (v WindowMode) Valid() bool { return slices.Contains(WindowModes(), v) }

// ProxyMode is the network proxy mode.
type ProxyMode string

// ProxyMode values.
const (
	ProxyModeSystem ProxyMode = "system"
	ProxyModeHTTP   ProxyMode = "http"
	ProxyModeSOCKS5 ProxyMode = "socks5"
)

// ProxyModes returns every valid ProxyMode.
func ProxyModes() []ProxyMode { return []ProxyMode{ProxyModeSystem, ProxyModeHTTP, ProxyModeSOCKS5} }

// Valid reports whether v is a known value.
func (v ProxyMode) Valid() bool { return slices.Contains(ProxyModes(), v) }
