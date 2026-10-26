package app

import (
	"sort"
	"strconv"
	"strings"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// prepareRows runs on the sync/preferences worker, never from a UI snapshot read.
func (p *livePricing) prepareRows() {
	catalog := p.manager.Snapshot()
	p.mu.Lock()
	prefs := p.displayPrefs.Clone()
	langs := p.langs
	p.mu.Unlock()
	l := l10n.ForPreference(string(prefs.Language), langs)
	version := catalog.Revision() + "|" + string(l.Language()) + "|" + usage.PricingVersion + "|" + grok.PricingVersion + "|" + claude.PricingVersion
	p.mu.Lock()
	if p.dataVersion == version {
		p.mu.Unlock()
		return
	}
	p.catalogTarget = version
	p.mu.Unlock()
	rows := pricingRows(catalog, l)
	p.mu.Lock()
	if p.catalogTarget == version && !p.closed && p.displayPrefs.Language == prefs.Language && p.manager.Snapshot().Revision() == catalog.Revision() {
		p.rows = rows
		p.dataVersion = version
	}
	p.mu.Unlock()
}
func (p *livePricing) snapshot(_ store.Preferences) PricingSnapshot {
	status := p.manager.Status()
	p.mu.Lock()
	defer p.mu.Unlock()
	next, _ := pricingDue(p.prefs, status, p.now())
	return PricingSnapshot{Rows: p.rows, DataVersion: p.dataVersion, Version: status.Version, Error: status.Error, Syncing: status.Running || p.running > 0, LastAttempt: status.LastAttempt, LastSuccess: status.LastSuccess, LastChanged: status.LastChanged, NextSync: next}
}
func pricingRows(catalog pricing.Snapshot, l l10n.Localizer) []PricingRow {
	models := map[string]pricing.Model{}
	builtin := map[string]bool{}
	rules := map[string][]pricing.Rule{}
	for _, group := range []struct {
		models  []pricing.Model
		details map[string][]pricing.Rule
	}{{usage.BuiltinPricingModels(), usage.BuiltinPricingDetails()}, {grok.BuiltinPricingModels(), grok.BuiltinPricingDetails()}, {claude.BuiltinPricingModels(), claude.BuiltinPricingDetails()}} {
		for _, m := range group.models {
			k := m.ProviderID + "/" + m.ID
			models[k] = m
			builtin[k] = true
			rules[k] = group.details[m.ID]
		}
	}
	for _, m := range catalog.Models() {
		if m.ProviderID != "openai" && m.ProviderID != "anthropic" && m.ProviderID != "xai" {
			continue
		}
		k := m.ProviderID + "/" + m.ID
		models[k] = m
		builtin[k] = false
	}
	rows := make([]PricingRow, 0, len(models))
	for key, m := range models {
		if len(rules[key]) == 0 {
			rules[key] = builtinDetailsFor(m.ProviderID, m.ID)
		}
		name := m.Name
		if name == "" {
			name = m.ID
		}
		providerName := "OpenAI"
		switch m.ProviderID {
		case "anthropic":
			providerName = "Anthropic"
		case "xai":
			providerName = "xAI"
		}
		sourceURL := "https://models.dev"
		source := "models.dev"
		version := catalog.Version()
		if builtin[key] {
			source = "builtin"
			sourceURL = builtinSourceURL(m.ProviderID)
			version = builtinVersion(m.ProviderID)
		}
		supported := m.ProviderID == "openai" || m.ProviderID == "anthropic" || m.ProviderID == "xai"
		available := supported && m.Rate.Input != nil && m.Rate.Output != nil
		if m.LegacyLong != nil && len(m.Tiers) == 0 && !builtin[key] {
			available = false
		}
		for _, tier := range m.Tiers {
			if tier.Type != "context" || tier.Size <= 0 {
				available = false
			}
		}
		details := []string{l.Pricing("currentEstimate")}
		if !builtin[key] {
			details = append(details, l.Pricing("catalogLimitations"), l.Pricing("catalog")+" · models.dev · "+version+"\n"+l.Pricing("ruleStandard")+": "+pricingRateText(l, m.Rate))
		}
		for _, tier := range m.Tiers {
			details = append(details, l.PricingRule("ruleTier", remoteTierLabel(m, tier), pricingRateText(l, tier.Rate)))
		}
		if m.LegacyLong != nil {
			details = append(details, l.Pricing("ruleLegacyLong")+": "+pricingRateText(l, *m.LegacyLong))
		}
		if len(rules[key]) > 0 {
			details = append(details, l.Pricing("rulePreserved"), l.Pricing("builtin")+" · "+builtinVersion(m.ProviderID))
			for _, r := range rules[key] {
				details = append(details, pricingRuleText(l, r))
			}
		}
		rows = append(rows, PricingRow{ProviderID: m.ProviderID, ProviderName: providerName, ID: m.ID, Name: name, Input: m.Rate.Input, Output: m.Rate.Output, CacheRead: m.Rate.CacheRead, CacheWrite: m.Rate.CacheWrite, Source: source, SourceURL: sourceURL, Version: version, Details: strings.Join(details, "\n"), Available: available, Builtin: builtin[key]})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ProviderID+"/"+rows[i].ID < rows[j].ProviderID+"/"+rows[j].ID })
	return rows
}
func builtinVersion(provider string) string {
	switch provider {
	case "openai":
		return usage.PricingVersion
	case "xai":
		return grok.PricingVersion
	case "anthropic":
		return claude.PricingVersion
	}
	return ""
}
func pricingRateText(l l10n.Localizer, r pricing.Rate) string {
	return l.Pricing("input") + " " + pricingValue(r.Input) + " · " + l.Pricing("cacheRead") + " " + pricingValue(r.CacheRead) + " · " + l.Pricing("output") + " " + pricingValue(r.Output) + " · " + l.Pricing("cacheWrite") + " " + pricingValue(r.CacheWrite)
}
func pricingRuleText(l l10n.Localizer, r pricing.Rule) string {
	text := pricingRateText(l, r.Rate)
	switch r.Kind {
	case "standard":
		text = l.Pricing("ruleStandard") + ": " + text
	case "fast":
		text = l.PricingRule("ruleFast", text)
	case "fast-unpriced":
		return l.Pricing("ruleFastUnpriced")
	case "geo-us":
		if r.Multiplier != nil {
			return l.PricingRule("ruleGeoUS", r.Multiplier.String())
		}
	case "above":
		op := "> "
		if r.Inclusive {
			op = "≥ "
		}
		text = l.PricingRule("ruleTier", op+strconv.FormatInt(r.Threshold, 10), text)
	case "long-unresolved":
		text = l.Pricing("ruleLegacyLong") + ": " + text
	}
	if r.CacheWrite5m != nil {
		text += "\n" + l.PricingRule("ruleTTL", "5m", pricingValue(r.CacheWrite5m))
	}
	if r.CacheWrite1h != nil {
		text += "\n" + l.PricingRule("ruleTTL", "1h", pricingValue(r.CacheWrite1h))
	}
	return text
}

func remoteTierLabel(m pricing.Model, t pricing.Tier) string {
	if m.ProviderID == "openai" && m.ID == "gpt-6.1-sol" && t.Size == 272000 {
		return "> 272000"
	}
	return strconv.FormatInt(t.Size, 10) + " (?)"
}
func builtinSourceURL(provider string) string {
	switch provider {
	case "openai":
		return "https://developers.openai.com/api/docs/pricing"
	case "anthropic":
		return "https://platform.claude.com/docs/en/about-claude/pricing"
	case "xai":
		return "https://docs.x.ai/developers/pricing"
	}
	return ""
}

func builtinDetailsFor(provider, model string) []pricing.Rule {
	switch provider {
	case "openai":
		return usage.BuiltinPricingDetailsFor(model)
	case "anthropic":
		return claude.BuiltinPricingDetailsFor(model)
	case "xai":
		return grok.BuiltinPricingDetailsFor(model)
	}
	return nil
}
