package claude

import (
	"strings"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// PricingCostWithSnapshot chooses one complete rate group for a request
// Missing TTL, Fast, region and tier-boundary semantics never borrow rates
// from a different group
func PricingCostWithSnapshot(snapshot pricing.Snapshot, model string, parts TokenParts, geo string) (decimal.Decimal, bool, string) {
	allowBuiltin := true
	remote, found := snapshot.Lookup("anthropic", model)
	if !found {
		remote, found = snapshot.Lookup("anthropic", NormalizeModel(model))
	}
	special := parts.Fast || parts.CacheWrite5m > 0 || parts.CacheWrite1h > 0 || strings.EqualFold(strings.TrimSpace(geo), "us")
	builtin, known := pricingTable[NormalizeModel(model)]
	if known && builtin.above != nil && parts.PromptTokens() > builtin.threshold {
		special = true
	}
	if found {
		// A legacy premium without a context boundary cannot establish
		// whether any request belongs to the base or premium rate group
		if remote.LegacyLong != nil && len(remote.Tiers) == 0 {
			special = true
		}
		r := remote.Rate
		chosenSize := int64(-1)
		tierBoundary := int64(-1)
		for _, tier := range remote.Tiers {
			if tier.Type != "context" {
				special = true
				continue
			}
			if parts.PromptTokens() >= tier.Size && tier.Size > tierBoundary {
				tierBoundary = tier.Size
			}
			if parts.PromptTokens() == tier.Size {
				special = true
			}
			if parts.PromptTokens() > tier.Size && tier.Size > chosenSize {
				r, chosenSize = tier.Rate, tier.Size
			}
		}
		if tierBoundary >= 0 && !(known && builtin.above != nil && (parts.PromptTokens() > builtin.threshold || tierBoundary == builtin.threshold)) {
			allowBuiltin = false
		}

		if !special && (parts.Input <= 0 || r.Input != nil) && (parts.CacheRead <= 0 || r.CacheRead != nil) && (parts.Output <= 0 || r.Output != nil) {
			chosen := rate{}
			if r.Input != nil {
				chosen.in = *r.Input
			}
			if r.CacheRead != nil {
				chosen.cr = *r.CacheRead
			}
			if r.Output != nil {
				chosen.out = *r.Output
			}
			return costFor(chosen, parts), true, ""
		}
	}
	if cost, ok := PricingCost(model, parts, geo); ok && allowBuiltin {
		return cost, true, ""
	}
	reason := "unknown-model:"
	if found {
		reason = "missing-price-rule:"
	}
	if parts.Fast {
		reason = "fast-unpriced:"
	}
	return decimal.Decimal{}, false, reason + model
}

func snapshotPricingVersion(snapshot pricing.Snapshot) string {
	if snapshot.ProviderVersion("anthropic") == "" {
		return PricingVersion
	}
	return PricingVersion + "+" + snapshot.ProviderVersion("anthropic")
}

// BuiltinPricingModels returns detached base rates for offline price browsing
func BuiltinPricingModels() []pricing.Model {
	out := make([]pricing.Model, 0, len(pricingTable))
	for id, price := range pricingTable {
		r := price.std
		out = append(out, pricing.Model{ProviderID: "anthropic", ID: id, Name: id, Rate: pricing.Rate{Input: &r.in, Output: &r.out, CacheRead: &r.cr}})
	}
	return out
}

func (s SessionScanner) pricingSnapshot() pricing.Snapshot {
	if s.PricingSnapshot != nil {
		return s.PricingSnapshot()
	}
	return pricing.Snapshot{}
}

// BuiltinPricingDetails exposes all verified special rates without display text
func BuiltinPricingDetails() map[string][]pricing.Rule {
	out := make(map[string][]pricing.Rule, len(pricingTable))
	for id, p := range pricingTable {
		out[id] = builtinPricingRules(p)
	}
	return out
}

// BuiltinPricingDetailsFor resolves only the aliases accepted by PricingCost
// Every returned rate is detached from the builtin table
func BuiltinPricingDetailsFor(model string) []pricing.Rule {
	p, ok := pricingTable[model]
	if !ok {
		p, ok = pricingTable[NormalizeModel(model)]
	}
	if !ok {
		return nil
	}
	return builtinPricingRules(p)
}

func builtinPricingRules(p modelPrice) []pricing.Rule {
	describe := func(kind string, r rate) pricing.Rule {
		return pricing.Rule{Kind: kind, Rate: pricing.Rate{Input: &r.in, Output: &r.out, CacheRead: &r.cr}, CacheWrite5m: &r.c5, CacheWrite1h: &r.c1}
	}
	rules := []pricing.Rule{describe("standard", p.std)}
	switch {
	case p.fast != nil:
		rules = append(rules, describe("fast", *p.fast))
	case p.fastAsStd:
		rules = append(rules, describe("fast", p.std))
	default:
		rules = append(rules, pricing.Rule{Kind: "fast-unpriced"})
	}
	if p.above != nil {
		r := describe("above", *p.above)
		r.Threshold = p.threshold
		rules = append(rules, r)
	}
	if p.usSurcharge {
		m := usSurcharge
		rules = append(rules, pricing.Rule{Kind: "geo-us", Multiplier: &m})
	}
	return rules
}
