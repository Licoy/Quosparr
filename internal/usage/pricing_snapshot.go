package usage

import (
	"context"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage/decimal"
	"sort"
)

type snapshotPrices struct{ snapshot func() pricing.Snapshot }

// SnapshotPrices adapts a shared cached catalog; it never refreshes or performs I/O.
func SnapshotPrices(snapshot func() pricing.Snapshot) PriceSource { return snapshotPrices{snapshot} }
func (s snapshotPrices) PriceBook(context.Context) PriceBook {
	if s.snapshot == nil {
		return CurrentPriceBook()
	}
	snap := s.snapshot()
	book := CurrentPriceBook()
	if snap.Version() == "" {
		return book
	}
	book.snapshot = &snap
	if version := snap.ProviderVersion("openai"); version != "" {
		book.version = PricingVersion + "+" + version
	}
	return book
}

// remoteRate only selects scenarios whose missing v1 boundary semantics are verified.
func remoteRate(m pricing.Model, input int64) (pricing.Rate, bool) {
	if len(m.Tiers) == 0 {
		return m.Rate, m.LegacyLong == nil
	}
	tiers := append([]pricing.Tier(nil), m.Tiers...)
	sort.Slice(tiers, func(i, j int) bool { return tiers[i].Size < tiers[j].Size })
	rate := m.Rate
	for _, tier := range tiers {
		if tier.Type != "context" {
			return pricing.Rate{}, false
		}
		if input == tier.Size {
			// Verified provider rule: long context is strictly above 272k.
			if m.ProviderID == "openai" && m.ID == "gpt-6.1-sol" && tier.Size == 272000 {
				continue
			}
			return pricing.Rate{}, false
		}
		if input > tier.Size {
			rate = tier.Rate
		}
	}
	return rate, true
}
func completeRate(r pricing.Rate, t Totals) bool {
	return (t.UncachedInputTokens == 0 || r.Input != nil) && (t.CachedInputTokens == 0 || r.CacheRead != nil) && (t.OutputTokens == 0 || r.Output != nil)
}
func rateCost(r pricing.Rate, t Totals) decimal.Decimal {
	result := decimal.Zero
	million := decimal.FromInt(1000000)
	for _, part := range []struct {
		n int64
		p *decimal.Decimal
	}{{t.UncachedInputTokens, r.Input}, {t.CachedInputTokens, r.CacheRead}, {t.OutputTokens, r.Output}} {
		if part.n > 0 && part.p != nil {
			result = result.Add(decimal.FromInt64(part.n).Div(million).Mul(*part.p))
		}
	}
	return result
}
func (b PriceBook) remoteModel(model string) (pricing.Model, bool) {
	if b.snapshot == nil {
		return pricing.Model{}, false
	}
	id := normalizedModelID(model)
	if m, ok := b.snapshot.Lookup("openai", id); ok {
		return m, true
	}
	// The existing dated-snapshot alias is limited to OpenAI's own namespace.
	if base, ok := snapshotBase(id); ok {
		return b.snapshot.Lookup("openai", base)
	}
	return pricing.Model{}, false
}
