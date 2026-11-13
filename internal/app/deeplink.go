package app

import (
	"slices"

	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// OpenWidgetURL handles only the existing widget contract. Both keeps the
// current provider; resetToday always opens Codex, matching the Swift host.
// Call on the UI thread after App.Ready.
func (a *App) OpenWidgetURL(raw string) bool {
	link, ok := export.ParseDeepLink(raw)
	if !ok || a.settings == nil {
		return false
	}
	a.cancelPanelDetails()
	prefs := a.settings.Prefs()
	provider := widgetProvider(link.Provider, prefs.SelectedProvider)
	if link.Section == export.SectionResetToday {
		provider = store.ProviderCodex
	}
	detail := widgetSectionModule(link.Section)
	if !slices.Contains(prefs.PanelSelection(), provider) {
		// A widget may show a provider the panel hides: the panel stays on
		// its own provider and opens the overview.
		provider, detail = prefs.SelectedProvider, ""
		link.Section = export.SectionOverview
	}
	a.settings.SetSelectedProvider(provider)
	a.panelState.provider = provider
	a.panelState.detail = detail
	acts := a.PanelActions()
	if link.Section == export.SectionOverview {
		if acts.Refresh != nil {
			acts.Refresh()
		}
	} else if acts.RefreshSection != nil {
		acts.RefreshSection(widgetSectionRefresh(link.Section))
	}
	a.OpenPanel()
	a.redrawPanel()
	RefreshWidgetConfigurations()
	return true
}

// widgetProvider opens a single-provider scope on that provider. A combination
// keeps the current provider when it is one of the pair.
func widgetProvider(scope export.ProviderScope, current store.Provider) store.Provider {
	switch scope {
	case export.ScopeCodex:
		return store.ProviderCodex
	case export.ScopeGrok:
		return store.ProviderGrok
	case export.ScopeClaude:
		return store.ProviderClaude
	case export.ScopeGemini:
		return store.ProviderGemini
	}
	for _, p := range scope.Providers() {
		if store.Provider(p) == current {
			return current
		}
	}
	if ps := scope.Providers(); len(ps) > 0 {
		return store.Provider(ps[0])
	}
	return current
}

func widgetSectionModule(s export.Section) store.Module {
	switch s {
	case export.SectionQuota:
		return store.ModuleQuota
	case export.SectionTokens:
		return store.ModuleTokenUsage
	case export.SectionCost:
		return store.ModuleAPICost
	case export.SectionResetToday:
		return store.ModuleRateLimitResetToday
	default:
		return ""
	}
}

func widgetSectionRefresh(s export.Section) runway.Section {
	switch s {
	case export.SectionQuota:
		return runway.SectionQuota
	case export.SectionTokens:
		return runway.SectionTokenHeatmap
	case export.SectionCost:
		return runway.SectionAPICost
	case export.SectionResetToday:
		return runway.SectionResetToday
	default:
		return ""
	}
}
