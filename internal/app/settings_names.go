package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

// Titles of the option values, as the Swift settings show them.

func providerKey(v store.Provider) l10n.Key {
	switch v {
	case store.ProviderCursor:
		return l10n.KeyProviderCursor
	case store.ProviderGrok:
		return l10n.KeyProviderGrok
	case store.ProviderClaude:
		return l10n.KeyProviderClaude
	default:
		return l10n.KeyProviderCodex
	}
}

func providerTabLabels(l l10n.Localizer) []string { return providerLabels(l, store.CLIProviders()) }

func providerLabels(l l10n.Localizer, providers []store.Provider) []string {
	labels := make([]string, 0, len(providers))
	for _, p := range providers {
		labels = append(labels, l.Text(providerKey(p)))
	}
	return labels
}

func appearanceKey(v store.Appearance) l10n.Key {
	switch v {
	case store.AppearanceLight:
		return l10n.KeyAppearanceLight
	case store.AppearanceDark:
		return l10n.KeyAppearanceDark
	}
	return l10n.KeyAppearanceSystem
}

func panelBackgroundKey(v store.PanelBackground) l10n.Key {
	if v == store.PanelBackgroundOpaque {
		return l10n.KeyMainPanelBackgroundOpaque
	}
	return l10n.KeyMainPanelBackgroundTranslucent
}

func statusBarStyleKey(v store.StatusBarStyle) l10n.Key {
	switch v {
	case store.StatusBarStyleCountdown:
		return l10n.KeyStatusBarCountdown
	case store.StatusBarStyleBattery:
		return l10n.KeyStatusBarBattery
	case store.StatusBarStyleRings:
		return l10n.KeyStatusBarRings
	case store.StatusBarStyleText:
		return l10n.KeyStatusBarText
	}
	return l10n.KeyStatusBarMeters
}

func providerScopeKey(v store.StatusBarProviderScope) l10n.Key {
	switch v {
	case store.ProviderScopeBoth:
		return l10n.KeyStatusBarProviderScopeBoth
	case store.ProviderScopeCustom:
		return l10n.KeyStatusBarProviderScopeCustom
	default:
		return l10n.KeyStatusBarProviderScopeSelected
	}
}

func metersDetailKey(v store.MetersDetail) l10n.Key {
	switch v {
	case store.MetersDetailResetTime:
		return l10n.KeyStatusBarMetersDetailResetTime
	case store.MetersDetailBoth:
		return l10n.KeyStatusBarMetersDetailBoth
	}
	return l10n.KeyStatusBarMetersDetailRemainingPercent
}

func batteryScopeKey(v store.BatteryScope) l10n.Key {
	switch v {
	case store.BatteryScopeWeekly:
		return l10n.KeyStatusBarBatteryScopeWeekly
	case store.BatteryScopeBoth:
		return l10n.KeyStatusBarBatteryScopeBoth
	}
	return l10n.KeyStatusBarBatteryScopeFiveHour
}

func batteryDetailKey(v store.BatteryDetail) l10n.Key {
	if v == store.BatteryDetailRemainingPercent {
		return l10n.KeyStatusBarBatteryDetailRemainingPercent
	}
	return l10n.KeyStatusBarBatteryDetailCountdown
}

func costRangeKey(v store.CostRange) l10n.Key {
	switch v {
	case store.CostRangeCurrent:
		return l10n.KeyCurrentCycle
	case store.CostRangePrevious:
		return l10n.KeyPreviousCycle
	case store.CostRangeThisMonth:
		return l10n.KeyThisMonth
	}
	return l10n.KeyToday
}

func chartStyleKey(v store.ChartStyle) l10n.Key {
	switch v {
	case store.ChartStyleLine:
		return l10n.KeyTokenUsageChartLine
	case store.ChartStyleBar:
		return l10n.KeyTokenUsageChartBar
	}
	return l10n.KeyTokenUsageChartHeatmap
}

func windowModeKey(v store.WindowMode) l10n.Key {
	if v == store.WindowModeRollingWeek {
		return l10n.KeyQuotaEstimateWindowRolling
	}
	return l10n.KeyAuto
}

// moduleInfo is how the settings show a main panel module.
type moduleInfo struct {
	title, subtitle l10n.Key
	// both is true for modules that apply to Codex, Claude and Grok.
	both bool
	icon *ui.SVG
	// config: the module has settings of its own; configWhenHidden keeps
	// them visible while the module is hidden.
	config, configWhenHidden bool
}

// moduleTexts is the title and description a module has in the settings.
// The speed analysis and skills texts are Go-only, so their moduleInfo keys
// are empty.
func moduleTexts(l l10n.Localizer, m store.Module) (title, subtitle string) {
	switch m {
	case store.ModuleSpeedAnalysis:
		return tx(l, xSpeedTitle), tx(l, xSpeedModuleDescription)
	case store.ModuleSkills:
		return tx(l, xSkillsTitle), tx(l, xSkillsModuleDescription)
	case store.ModuleCacheAnalysis:
		return tx(l, xCacheTitle), tx(l, xCacheModuleDescription)
	}
	info := moduleInfoOf(m)
	return l.Text(info.title), l.Text(info.subtitle)
}

func moduleInfoOf(m store.Module) moduleInfo {
	switch m {
	case store.ModuleQuota:
		return moduleInfo{l10n.KeyQuota, l10n.KeyModuleQuotaDescription, true, icGauge, true, true}
	case store.ModuleTokenUsage:
		return moduleInfo{l10n.KeyTokenUsageHeatmap, l10n.KeyTokenUsageHeatmapDescription, true, icChart, true, false}
	case store.ModuleSpeedAnalysis:
		return moduleInfo{"", "", true, icTimerFlash, false, false}
	case store.ModuleSkills:
		return moduleInfo{"", "", true, icSkill, false, false}
	case store.ModuleRateLimitResetToday:
		return moduleInfo{l10n.KeyRateLimitResetToday, l10n.KeyRateLimitResetTodayDescription, false, icReset, true, false}
	case store.ModuleQuotaEstimate:
		return moduleInfo{l10n.KeyQuotaEstimate, l10n.KeyModuleQuotaEstimateDescription, false, icFunction, true, false}
	case store.ModuleResetCredits:
		return moduleInfo{l10n.KeyResetCredits, l10n.KeyModuleResetCreditsDescription, true, icRefresh, false, false}
	case store.ModuleCacheAnalysis:
		return moduleInfo{"", "", true, icChart, false, false}
	case store.ModuleAPICost:
		return moduleInfo{l10n.KeyApiCost, l10n.KeyModuleAPICostDescription, true, icCoins, true, false}
	case store.ModuleSessionRepair:
		return moduleInfo{l10n.KeySessionRepair, l10n.KeyModuleSessionRepairDescription, false, icRepair, false, false}
	}
	return moduleInfo{l10n.KeyRecentSessions, l10n.KeyRecentSessionsDescription, true, icClock, false, false}
}
