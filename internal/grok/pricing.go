package grok

import (
	"regexp"
	"strings"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

// PricingVersion is the version date of the built-in Grok price table, the
// official xAI Text API token prices verified against
// https://docs.x.ai/developers/pricing on 2026-09-27. Unknown model IDs are
// never given an exact cost.
const PricingVersion = "xai-builtin-2026-09-27"

// PricingLongContextThreshold is the prompt size, in tokens, from which the
// long-context rates apply to the whole request.
const PricingLongContextThreshold = 200_000

// PricingPrice is the USD price per million tokens of one model. The long
// context rates are nil when the model has none.
type PricingPrice struct {
	InputPerMillion                  decimal.Decimal
	CachedInputPerMillion            decimal.Decimal
	OutputPerMillion                 decimal.Decimal
	LongContextInputPerMillion       *decimal.Decimal
	LongContextCachedInputPerMillion *decimal.Decimal
	LongContextOutputPerMillion      *decimal.Decimal
	LongContextThresholdTokens       int
}

func pricingPrice(in, cached, out, lin, lcached, lout string) PricingPrice {
	d := func(s string) *decimal.Decimal { v := decimal.MustParse(s); return &v }
	return PricingPrice{
		InputPerMillion:                  decimal.MustParse(in),
		CachedInputPerMillion:            decimal.MustParse(cached),
		OutputPerMillion:                 decimal.MustParse(out),
		LongContextInputPerMillion:       d(lin),
		LongContextCachedInputPerMillion: d(lcached),
		LongContextOutputPerMillion:      d(lout),
		LongContextThresholdTokens:       PricingLongContextThreshold,
	}
}

var (
	// Grok 4.7 Fast is not 2x the long-context flagship rate: its long
	// context rates are $6 / $1.50 / $18.
	pricingFast47 = pricingPrice("4", "1", "12", "6", "1.50", "18")
	pricingMid    = pricingPrice("1.25", "0.20", "2.50", "2.50", "0.40", "5")
	pricingFlag   = pricingPrice("2", "0.50", "6", "4", "1", "12")
)

// pricingBuiltIn is the bundled table, keyed by lower-case model ID.
var pricingBuiltIn = map[string]PricingPrice{
	"grok-4.7":                     pricingFlag,
	"grok-4.7-fast":                pricingFast47,
	"grok-4.7-build-fast":          pricingFast47,
	"grok-4.6":                     pricingFlag,
	"grok-4.5":                     pricingPrice("2", "0.30", "6", "4", "0.60", "12"),
	"grok-4.3":                     pricingMid,
	"grok-4.20-0309-reasoning":     pricingMid,
	"grok-4.20-0309-non-reasoning": pricingMid,
	"grok-4.20-multi-agent-0309":   pricingMid,
	"grok-build-0.1":               pricingPrice("1", "0.20", "2", "2", "0.40", "4"),
}

// PricingFor returns the price of a model ID. Besides the exact ID it accepts a
// "-latest" suffix, the CLI product suffixes "-build" and "-build-free" (only
// as the final suffix, and never reducing the name to "grok") and a dated
// "-YYYY-MM-DD" snapshot suffix. It reports false for unknown models.
func PricingFor(model string) (PricingPrice, bool) {
	return pricingLookup(model, pricingBuiltIn)
}

var pricingSnapshotSuffix = regexp.MustCompile(`-\p{Nd}{4}-\p{Nd}{2}-\p{Nd}{2}$`)

func pricingLookup[T any](model string, prices map[string]T) (T, bool) {
	return pricingLookupWith(model, func(key string) (T, bool) { p, ok := prices[key]; return p, ok })
}

func pricingLookupWith[T any](model string, lookup func(string) (T, bool)) (T, bool) {
	var zero T
	key := strings.ToLower(billingTrim(model))
	if key == "" {
		return zero, false
	}
	if p, ok := lookup(key); ok {
		return p, true
	}
	if strings.HasSuffix(key, "-latest") {
		key = strings.TrimSuffix(key, "-latest")
		if p, ok := lookup(key); ok {
			return p, true
		}
	}
	if base, ok := pricingStripProduct(key); ok {
		if p, ok := lookup(base); ok {
			return p, true
		}
	}
	if loc := pricingSnapshotSuffix.FindStringIndex(key); loc != nil {
		snapshot := key[:loc[0]]
		if p, ok := lookup(snapshot); ok {
			return p, true
		}
		if base, ok := pricingStripProduct(snapshot); ok {
			if p, ok := lookup(base); ok {
				return p, true
			}
		}
	}
	return zero, false
}

// pricingStripProduct removes "-build-free" or "-build": CLI product SKUs
// such as grok-4.5-build share the grok-4.5 API price, while the infix in
// grok-build-0.1 and grok-4.7-build-fast stays.
func pricingStripProduct(model string) (string, bool) {
	for _, suffix := range []string{"-build-free", "-build"} {
		if strings.HasSuffix(model, suffix) {
			if base := strings.TrimSuffix(model, suffix); base != "" && base != "grok" {
				return base, true
			}
		}
	}
	return "", false
}

// PricingCost is the API-equivalent cost of one request, or false for a model
// without a known price. inputTokens is the prompt size (it decides the
// long-context threshold) and includes cachedInputTokens; reasoning tokens
// count as part of outputTokens.
func PricingCost(model string, inputTokens, cachedInputTokens, outputTokens int) (decimal.Decimal, bool) {
	price, ok := PricingFor(model)
	if !ok {
		return decimal.Decimal{}, false
	}
	return PricingCostFor(price, inputTokens, cachedInputTokens, outputTokens), true
}

// PricingCostFor prices a request with an explicit price. Negative cached and
// output counts clamp to zero; the long-context rates apply to the whole
// request when the prompt reaches the threshold.
func PricingCostFor(price PricingPrice, inputTokens, cachedInputTokens, outputTokens int) decimal.Decimal {
	cached := max(0, cachedInputTokens)
	uncached := max(0, inputTokens-cached)
	output := max(0, outputTokens)
	useLong := inputTokens >= price.LongContextThresholdTokens && price.LongContextInputPerMillion != nil
	inputRate, cachedRate, outputRate := price.InputPerMillion, price.CachedInputPerMillion, price.OutputPerMillion
	if useLong {
		inputRate = *price.LongContextInputPerMillion
		if price.LongContextCachedInputPerMillion != nil {
			cachedRate = *price.LongContextCachedInputPerMillion
		}
		if price.LongContextOutputPerMillion != nil {
			outputRate = *price.LongContextOutputPerMillion
		}
	}
	million := decimal.FromInt(1_000_000)
	return decimal.FromInt(uncached).Div(million).Mul(inputRate).
		Add(decimal.FromInt(cached).Div(million).Mul(cachedRate)).
		Add(decimal.FromInt(output).Div(million).Mul(outputRate))
}
