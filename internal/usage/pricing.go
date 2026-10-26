package usage

import (
	"github.com/licoy/quosparr/internal/pricing"
	"strings"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

// PricingVersion is the version date of the built-in price table. It was
// verified against the official OpenAI pricing documentation; the dated
// suffix changes whenever a price in builtInPrices changes.
const PricingVersion = "openai-builtin-2026-09-27"

// onlinePricingVersionPrefix starts the version of a price book built from the
// downloaded catalog.
const onlinePricingVersionPrefix = "openai-docs-"

// Price is the USD price per million tokens of one model. The optional fields
// are nil when the catalog does not list them.
type Price struct {
	InputPerMillion                  decimal.Decimal  `json:"inputPerMillion"`
	CachedInputPerMillion            decimal.Decimal  `json:"cachedInputPerMillion"`
	CacheWritePerMillion             *decimal.Decimal `json:"cacheWritePerMillion,omitempty"`
	OutputPerMillion                 decimal.Decimal  `json:"outputPerMillion"`
	LongContextInputPerMillion       *decimal.Decimal `json:"longContextInputPerMillion,omitempty"`
	LongContextCachedInputPerMillion *decimal.Decimal `json:"longContextCachedInputPerMillion,omitempty"`
	LongContextCacheWritePerMillion  *decimal.Decimal `json:"longContextCacheWritePerMillion,omitempty"`
	LongContextOutputPerMillion      *decimal.Decimal `json:"longContextOutputPerMillion,omitempty"`
}

// Equal reports whether two prices are numerically identical.
func (p Price) Equal(o Price) bool {
	return p.InputPerMillion.Equal(o.InputPerMillion) &&
		p.CachedInputPerMillion.Equal(o.CachedInputPerMillion) &&
		p.OutputPerMillion.Equal(o.OutputPerMillion) &&
		optEqual(p.CacheWritePerMillion, o.CacheWritePerMillion) &&
		optEqual(p.LongContextInputPerMillion, o.LongContextInputPerMillion) &&
		optEqual(p.LongContextCachedInputPerMillion, o.LongContextCachedInputPerMillion) &&
		optEqual(p.LongContextCacheWritePerMillion, o.LongContextCacheWritePerMillion) &&
		optEqual(p.LongContextOutputPerMillion, o.LongContextOutputPerMillion)
}

func optEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func dp(s string) *decimal.Decimal {
	d := decimal.MustParse(s)
	return &d
}

func basicPrice(in, cached, out string) Price {
	return Price{
		InputPerMillion:       decimal.MustParse(in),
		CachedInputPerMillion: decimal.MustParse(cached),
		OutputPerMillion:      decimal.MustParse(out),
	}
}

func longPrice(in, cached, write, out, lin, lcached, lwrite, lout string) Price {
	p := basicPrice(in, cached, out)
	p.CacheWritePerMillion = dp(write)
	p.LongContextInputPerMillion = dp(lin)
	p.LongContextCachedInputPerMillion = dp(lcached)
	p.LongContextCacheWritePerMillion = dp(lwrite)
	p.LongContextOutputPerMillion = dp(lout)
	return p
}

// builtInPrices is the bundled price table, keyed by lower-case model ID.
var builtInPrices = map[string]Price{
	"gpt-6-astra":         longPrice("10", "1", "12.5", "50", "20", "2", "25", "75"),
	"gpt-6-sol":           longPrice("2", "0.2", "2.5", "10", "4", "0.4", "5", "15"),
	"gpt-6-luna":          longPrice("0.1", "0.01", "0.125", "0.5", "0.2", "0.02", "0.25", "0.75"),
	"gpt-5.6":             longPrice("4", "0.4", "5", "20", "8", "0.8", "10", "30"),
	"gpt-5.6-sol":         longPrice("4", "0.4", "5", "20", "8", "0.8", "10", "30"),
	"gpt-5.6-terra":       longPrice("2", "0.2", "2.5", "12", "4", "0.4", "5", "18"),
	"gpt-5.6-luna":        longPrice("0.2", "0.02", "0.25", "1.2", "0.4", "0.04", "0.5", "1.8"),
	"gpt-5.5":             basicPrice("5", "0.5", "30"),
	"gpt-5.5-pro":         basicPrice("30", "30", "180"),
	"gpt-5.4":             basicPrice("2.5", "0.25", "15"),
	"gpt-5.4-pro":         basicPrice("30", "30", "180"),
	"gpt-5.4-mini":        basicPrice("0.75", "0.075", "4.5"),
	"gpt-5.4-nano":        basicPrice("0.2", "0.02", "1.25"),
	"gpt-5.3-codex":       basicPrice("1.75", "0.175", "14"),
	"gpt-5.3-codex-spark": basicPrice("1.75", "0.175", "14"),
	"gpt-5.2-codex":       basicPrice("1.75", "0.175", "14"),
	"gpt-5.2":             basicPrice("1.75", "0.175", "14"),
	"gpt-5.1":             basicPrice("1.25", "0.125", "10"),
	"gpt-5":               basicPrice("1.25", "0.125", "10"),
	"gpt-5-mini":          basicPrice("0.25", "0.025", "2"),
	"gpt-5-nano":          basicPrice("0.05", "0.005", "0.4"),
	"gpt-5-pro":           basicPrice("15", "15", "120"),
	"codex-mini-latest":   basicPrice("1.5", "0.375", "6"),
}

// BuiltInPrices returns a copy of the bundled price table.
func BuiltInPrices() map[string]Price {
	out := make(map[string]Price, len(builtInPrices))
	for k, v := range builtInPrices {
		out[k] = v
	}
	return out
}

// EquivalentPrice is the price used for the "API-equivalent" headline figure
// of online analytics, whose rows carry no model.
func EquivalentPrice() Price { return builtInPrices["gpt-5.6-sol"] }

func normalizedModelID(model string) string {
	return strings.ToLower(strings.TrimFunc(model, isSwiftWhitespace))
}

// isSwiftWhitespace matches CharacterSet.whitespacesAndNewlines closely enough
// for model IDs: Unicode spaces, tabs and line breaks.
func isSwiftWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r', 0x85, 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// snapshotBase strips a trailing "-YYYY-MM-DD" snapshot suffix.
func snapshotBase(model string) (string, bool) {
	n := len(model)
	if n < 11 {
		return "", false
	}
	s := model[n-11:]
	if s[0] != '-' || s[5] != '-' || s[8] != '-' {
		return "", false
	}
	for _, i := range []int{1, 2, 3, 4, 6, 7, 9, 10} {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
	}
	return model[:n-11], true
}

// LookupPrice finds the price of model in prices. The match is exact after
// trimming and lower-casing, except that a dated snapshot ("-YYYY-MM-DD")
// falls back to its base model. Unknown models return ok=false: they are never
// estimated.
func LookupPrice(prices map[string]Price, model string) (Price, bool) {
	key := normalizedModelID(model)
	if p, ok := prices[key]; ok {
		return p, true
	}
	if base, ok := snapshotBase(key); ok {
		p, ok := prices[base]
		return p, ok
	}
	return Price{}, false
}

// PriceFor looks model up in the built-in table.
func PriceFor(model string) (Price, bool) { return LookupPrice(builtInPrices, model) }

// IsCompatiblePricingVersion reports whether a cached summary priced with
// version may still be shown: the current built-in version, or any version
// derived from the online catalog.
func IsCompatiblePricingVersion(version string) bool {
	return version == PricingVersion || strings.HasPrefix(version, PricingVersion+"+models-dev-") || strings.HasPrefix(version, onlinePricingVersionPrefix)
}

func costFor(uncached, cached, output int64, p Price) decimal.Decimal {
	million := decimal.FromInt(1_000_000)
	a := decimal.FromInt64(uncached).Div(million).Mul(p.InputPerMillion)
	b := decimal.FromInt64(cached).Div(million).Mul(p.CachedInputPerMillion)
	c := decimal.FromInt64(output).Div(million).Mul(p.OutputPerMillion)
	return a.Add(b).Add(c)
}

// CostForUsage prices a token triple with the built-in table. It returns
// ok=false for an unknown model.
func CostForUsage(model string, u TokenUsage) (decimal.Decimal, bool) {
	p, ok := PriceFor(model)
	if !ok {
		return decimal.Zero, false
	}
	return costFor(u.InputTokens-u.CachedInputTokens, u.CachedInputTokens, u.OutputTokens, p), true
}

// CostForTotals prices totals with the built-in table.
func CostForTotals(model string, t Totals) (decimal.Decimal, bool) {
	p, ok := PriceFor(model)
	if !ok {
		return decimal.Zero, false
	}
	return costFor(t.UncachedInputTokens, t.CachedInputTokens, t.OutputTokens, p), true
}

// EquivalentCost prices totals at EquivalentPrice.
func EquivalentCost(t Totals) decimal.Decimal {
	return costFor(t.UncachedInputTokens, t.CachedInputTokens, t.OutputTokens, EquivalentPrice())
}

// PriceBook is an immutable, versioned price lookup. Use CurrentPriceBook for
// the built-in table or PricingCatalogProvider for the online one.
type PriceBook struct {
	version         string
	prices          map[string]Price
	equivalentPrice Price
	snapshot        *pricing.Snapshot
}

// NewPriceBook builds a price book from an explicit table.
func NewPriceBook(version string, prices map[string]Price, equivalent Price) PriceBook {
	cp := make(map[string]Price, len(prices))
	for k, v := range prices {
		cp[k] = v
	}
	return PriceBook{version: version, prices: cp, equivalentPrice: equivalent}
}

// CurrentPriceBook is the built-in table at PricingVersion.
func CurrentPriceBook() PriceBook {
	return NewPriceBook(PricingVersion, builtInPrices, EquivalentPrice())
}

// Version returns the version label of the book.
func (b PriceBook) Version() string { return b.version }

// Price looks model up; ok is false for unknown models.
func (b PriceBook) Price(model string) (Price, bool) {
	if m, ok := b.remoteModel(model); ok && m.Rate.Input != nil && m.Rate.CacheRead != nil && m.Rate.Output != nil {
		return Price{InputPerMillion: *m.Rate.Input, CachedInputPerMillion: *m.Rate.CacheRead, OutputPerMillion: *m.Rate.Output}, true
	}
	return LookupPrice(b.prices, model)
}

// Cost prices totals for model; ok is false for unknown models.
func (b PriceBook) Cost(model string, t Totals) (decimal.Decimal, bool) {
	c, ok, _ := b.CostWithSource(model, t)
	return c, ok
}

// CostWithSource selects a whole applicable rate group, never mixing missing fields.
func (b PriceBook) CostWithSource(model string, t Totals) (decimal.Decimal, bool, string) {
	m, remote := b.remoteModel(model)
	if remote {
		r, applicable := remoteRate(m, t.UncachedInputTokens+t.CachedInputTokens)
		if applicable && completeRate(r, t) {
			return rateCost(r, t), true, "models.dev"
		}
		if !applicable {
			return decimal.Zero, false, "ambiguous-tier"
		}
	}
	p, ok := LookupPrice(b.prices, model)
	if !ok {
		reason := "unknown-model"
		if remote {
			reason = "missing-rate"
		}
		return decimal.Zero, false, reason
	}
	if remote && len(m.Tiers) > 0 {
		input := t.UncachedInputTokens + t.CachedInputTokens
		for _, tier := range m.Tiers {
			if input > tier.Size {
				// The legacy built-in table stores long rates without their boundaries.
				// It cannot prove that its long group applies to this remote tier.
				return decimal.Zero, false, "missing-long-rule"
			}
		}
	}

	return costFor(t.UncachedInputTokens, t.CachedInputTokens, t.OutputTokens, p), true, "builtin"
}

// EquivalentCost prices totals at the book's equivalent price.
func (b PriceBook) EquivalentCost(t Totals) decimal.Decimal {
	return costFor(t.UncachedInputTokens, t.CachedInputTokens, t.OutputTokens, b.equivalentPrice)
}

func (b PriceBook) valid() bool { return b.prices != nil }
