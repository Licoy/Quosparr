package usage

import "github.com/licoy/quosparr/internal/pricing"

// BuiltinPricingModels returns detached base rates for offline browsing.
func BuiltinPricingModels() []pricing.Model {
	out := make([]pricing.Model, 0, len(builtInPrices))
	for _, id := range sortedKeys(builtInPrices) {
		p := builtInPrices[id]
		out = append(out, pricing.Model{ProviderID: "openai", ProviderName: "OpenAI", ID: id, Name: id, Rate: pricing.Rate{Input: cloneDecimal(&p.InputPerMillion), Output: cloneDecimal(&p.OutputPerMillion), CacheRead: cloneDecimal(&p.CachedInputPerMillion), CacheWrite: cloneDecimal(p.CacheWritePerMillion)}})
	}
	return out
}

// BuiltinPricingDetails includes legacy premiums without inventing their threshold.
func BuiltinPricingDetails() map[string][]pricing.Rule {
	out := make(map[string][]pricing.Rule, len(builtInPrices))
	for id := range builtInPrices {
		out[id] = BuiltinPricingDetailsFor(id)
	}
	return out
}

// BuiltinPricingDetailsFor uses the same exact and dated-snapshot matching as actual costs.
func BuiltinPricingDetailsFor(model string) []pricing.Rule {
	p, ok := LookupPrice(builtInPrices, model)
	if !ok {
		return nil
	}
	rules := []pricing.Rule{{Kind: "standard", Rate: pricing.Rate{Input: cloneDecimal(&p.InputPerMillion), Output: cloneDecimal(&p.OutputPerMillion), CacheRead: cloneDecimal(&p.CachedInputPerMillion), CacheWrite: cloneDecimal(p.CacheWritePerMillion)}}}
	if p.LongContextInputPerMillion != nil {
		rules = append(rules, pricing.Rule{Kind: "long-unresolved", Rate: pricing.Rate{Input: cloneDecimal(p.LongContextInputPerMillion), Output: cloneDecimal(p.LongContextOutputPerMillion), CacheRead: cloneDecimal(p.LongContextCachedInputPerMillion), CacheWrite: cloneDecimal(p.LongContextCacheWritePerMillion)}})
	}
	return rules
}
