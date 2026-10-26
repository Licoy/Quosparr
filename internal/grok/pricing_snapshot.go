package grok

import (
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// PricingCostWithSnapshot prices one request with a complete rate group
// Ambiguous remote tier boundaries retain verified builtin long-context rules
func PricingCostWithSnapshot(snapshot pricing.Snapshot, model string, input, cached, output int) (decimal.Decimal, bool, string) {
	allowBuiltin := true
	remote, found := snapshot.Lookup("xai", model)
	if !found {
		remote, found = pricingLookupWith(model, func(key string) (pricing.Model, bool) { return snapshot.Lookup("xai", key) })
	}
	builtin, known := PricingFor(model)
	special := known && input >= builtin.LongContextThresholdTokens && builtin.LongContextInputPerMillion != nil
	if found {
		// A legacy premium without a context boundary cannot establish
		// whether any request belongs to the base or premium rate group
		if remote.LegacyLong != nil && len(remote.Tiers) == 0 {
			special = true
		}
		r := remote.Rate
		chosenSize := int64(-1)
		tierRequested := false
		for _, tier := range remote.Tiers {
			if tier.Type != "context" {
				special = true
				continue
			}
			if int64(input) >= tier.Size {
				tierRequested = true
			}
			if int64(input) == tier.Size {
				special = true
			}
			if int64(input) > tier.Size && tier.Size > chosenSize {
				r, chosenSize = tier.Rate, tier.Size
			}
		}
		if tierRequested && !(known && builtin.LongContextInputPerMillion != nil && input >= builtin.LongContextThresholdTokens) {
			allowBuiltin = false
		}

		if !special && (input-cached <= 0 || r.Input != nil) && (cached <= 0 || r.CacheRead != nil) && (output <= 0 || r.Output != nil) {
			p := PricingPrice{}
			if r.Input != nil {
				p.InputPerMillion = *r.Input
			}
			if r.CacheRead != nil {
				p.CachedInputPerMillion = *r.CacheRead
			}
			if r.Output != nil {
				p.OutputPerMillion = *r.Output
			}
			return PricingCostFor(p, input, cached, output), true, ""
		}
	}
	if known && allowBuiltin {
		return PricingCostFor(builtin, input, cached, output), true, ""
	}
	reason := "unknown-model:"
	if found {
		reason = "missing-price-rule:"
	}
	return decimal.Decimal{}, false, reason + model
}

func snapshotPricingVersion(snapshot pricing.Snapshot) string {
	if snapshot.ProviderVersion("xai") == "" {
		return PricingVersion
	}
	return PricingVersion + "+" + snapshot.ProviderVersion("xai")
}

// BuiltinPricingModels returns detached base and legacy long-context rates
func BuiltinPricingModels() []pricing.Model {
	out := make([]pricing.Model, 0, len(pricingBuiltIn))
	for id, p := range pricingBuiltIn {
		r := pricing.Rate{Input: &p.InputPerMillion, CacheRead: &p.CachedInputPerMillion, Output: &p.OutputPerMillion}
		lin, lcr, lout := *p.LongContextInputPerMillion, *p.LongContextCachedInputPerMillion, *p.LongContextOutputPerMillion
		long := pricing.Rate{Input: &lin, CacheRead: &lcr, Output: &lout}
		out = append(out, pricing.Model{ProviderID: "xai", ID: id, Name: id, Rate: r, LegacyLong: &long})
	}
	return out
}

func (s SessionScanner) pricingSnapshot() pricing.Snapshot {
	if s.PricingSnapshot != nil {
		return s.PricingSnapshot()
	}
	return pricing.Snapshot{}
}

// BuiltinPricingDetails exposes the inclusive long-context boundary
func BuiltinPricingDetails() map[string][]pricing.Rule {
	out := make(map[string][]pricing.Rule, len(pricingBuiltIn))
	for id, p := range pricingBuiltIn {
		out[id] = builtinPricingRules(p)
	}
	return out
}

// BuiltinPricingDetailsFor uses PricingFor's exact and safe alias lookup
// Every returned rate is detached from the builtin table
func BuiltinPricingDetailsFor(model string) []pricing.Rule {
	p, ok := PricingFor(model)
	if !ok {
		return nil
	}
	return builtinPricingRules(p)
}

func builtinPricingRules(p PricingPrice) []pricing.Rule {
	r := pricing.Rate{Input: &p.InputPerMillion, CacheRead: &p.CachedInputPerMillion, Output: &p.OutputPerMillion}
	rules := []pricing.Rule{{Kind: "standard", Rate: r}}
	if p.LongContextInputPerMillion != nil {
		lin, lcr, lout := *p.LongContextInputPerMillion, *p.LongContextCachedInputPerMillion, *p.LongContextOutputPerMillion
		rules = append(rules, pricing.Rule{Kind: "above", Threshold: int64(p.LongContextThresholdTokens), Inclusive: true, Rate: pricing.Rate{Input: &lin, CacheRead: &lcr, Output: &lout}})
	}
	return rules
}
