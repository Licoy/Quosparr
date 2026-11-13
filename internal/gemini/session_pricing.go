package gemini

import (
	"strings"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// SessionPricingVersion names the price source of the session costs: the
// shared price snapshot only, without a bundled table.
const SessionPricingVersion = "gemini-snapshot"

var sessionPriceProviders = []string{"google", "anthropic"}

var sessionModelSuffixes = []string{"-high", "-medium", "-low", "-minimal", "-thinking"}

// sessionPriceProvider is the snapshot provider of a model: Anthropic for
// Claude models, Google for everything else.
func sessionPriceProvider(rec sessionRecord) string {
	model := strings.ToLower(rec.model)
	if strings.HasPrefix(model, "claude") || rec.claude && !strings.HasPrefix(model, "gemini") {
		return "anthropic"
	}
	return "google"
}

func sessionLookup(snapshot pricing.Snapshot, provider, model string) (pricing.Model, bool) {
	candidates := []string{model, strings.ToLower(model)}
	lower := strings.ToLower(model)
	for _, suffix := range sessionModelSuffixes {
		if trimmed, ok := strings.CutSuffix(lower, suffix); ok && trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	}
	for _, candidate := range candidates {
		if m, ok := snapshot.Lookup(provider, candidate); ok {
			return m, true
		}
	}
	return pricing.Model{}, false
}

// sessionCost prices one call with one complete rate group. Uncached input,
// cache reads, cache writes and output are priced separately; a call is
// unpriced when a used part has no rate, when its context tier is ambiguous
// (a legacy premium without a boundary, a non-context tier, or a prompt
// exactly at the boundary), and for Anthropic cache writes, whose duration
// the record does not say.
func sessionCost(snapshot pricing.Snapshot, rec sessionRecord) (decimal.Decimal, bool, string) {
	provider := sessionPriceProvider(rec)
	model, found := sessionLookup(snapshot, provider, rec.model)
	if !found {
		return decimal.Decimal{}, false, "unknown-model:" + rec.model
	}
	u := rec.usage
	prompt := u.input + u.cacheRead + u.cacheWrite
	special := model.LegacyLong != nil && len(model.Tiers) == 0 || provider == "anthropic" && u.cacheWrite > 0
	rate, chosen := model.Rate, int64(-1)
	for _, tier := range model.Tiers {
		switch {
		case tier.Type != "context" || prompt == tier.Size:
			special = true
		case prompt > tier.Size && tier.Size > chosen:
			rate, chosen = tier.Rate, tier.Size
		}
	}
	parts := []struct {
		tokens int64
		rate   *decimal.Decimal
	}{{u.input, rate.Input}, {u.cacheRead, rate.CacheRead}, {u.cacheWrite, rate.CacheWrite}, {u.output, rate.Output}}
	sum := decimal.Decimal{}
	for _, part := range parts {
		if part.tokens <= 0 {
			continue
		}
		if part.rate == nil {
			special = true
			break
		}
		sum = sum.Add(decimal.FromInt64(part.tokens).Mul(*part.rate))
	}
	if special {
		return decimal.Decimal{}, false, "missing-price-rule:" + rec.model
	}
	return sum.Div(decimal.FromInt(1_000_000)), true, ""
}

func sessionPricingVersion(snapshot pricing.Snapshot) string {
	version := SessionPricingVersion
	for _, provider := range sessionPriceProviders {
		if v := snapshot.ProviderVersion(provider); v != "" {
			version += "+" + provider + ":" + v
		}
	}
	return version
}
