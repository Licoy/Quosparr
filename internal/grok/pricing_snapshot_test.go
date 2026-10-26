package grok

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func grokSnapshot(t *testing.T, id, value string) pricing.Snapshot {
	t.Helper()
	d := decimal.MustParse(value)
	s, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "xai", ID: id, Rate: pricing.Rate{Input: &d, CacheRead: &d, Output: &d}}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGrokSnapshotUsesRemoteBaseAndBuiltinInclusiveLongTier(t *testing.T) {
	snapshot := grokSnapshot(t, "grok-4.5", "9")
	got, ok, _ := PricingCostWithSnapshot(snapshot, "grok-4.5-build", 199999, 0, 1)
	if !ok || !got.Equal(decimal.MustParse("1.8")) {
		t.Fatalf("remote alias %s %v", got, ok)
	}
	for _, n := range []int{200000, 200001} {
		want, _ := PricingCost("grok-4.5-build", n, 100, 1)
		got, ok, _ := PricingCostWithSnapshot(snapshot, "grok-4.5-build", n, 100, 1)
		if !ok || !got.Equal(want) {
			t.Fatalf("long tier %d %s != %s", n, got, want)
		}
	}
	exact := grokSnapshot(t, "grok-4.5-build", "8")
	got, ok, _ = PricingCostWithSnapshot(exact, "grok-4.5-build", 1, 0, 0)
	if !ok || got.String() != "0.000008" {
		t.Fatalf("exact %s %v", got, ok)
	}
}

func TestGrokDynamicSnapshotPartialZeroUnknownAndReprice(t *testing.T) {
	root := t.TempDir()
	sessionWrite(t, root, "mixed", "/work/demo", "grok-future", "2026-08-01T00:00:00Z",
		`{"timestamp":1785541392,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":20,"outputTokens":10,"totalTokens":30,"modelUsage":{"grok-future-build":{"inputTokens":10,"outputTokens":5,"totalTokens":15},"unknown-model":{"inputTokens":10,"outputTokens":5,"totalTokens":15}}}}}}`)
	snapshot := grokSnapshot(t, "grok-future", "0")
	calls := 0
	scanner := SessionScanner{GrokHome: root, Location: time.UTC, PricingSnapshot: func() pricing.Snapshot { calls++; return snapshot }}
	opts := SessionScanOptions{SessionLimit: 5, RecentLimit: 1, Now: time.Unix(1800000000, 0), CostWindow: usage.Window{Start: time.Unix(1700000000, 0), End: time.Unix(1800000000, 0)}}
	result, err := scanner.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	c := result.CostSummary
	if c.EstimatedUSD == nil || c.EstimatedUSD.Sign() != 0 || c.Confidence != usage.ConfidencePartiallyPriced || c.UnpricedTokens != 15 || calls != 1 {
		t.Fatalf("zero partial %+v %d", c, calls)
	}
	if c.DailyRows[0].UnpricedTokens != 15 || c.ProjectRows[0].UnpricedTokens != 15 || c.ClientRows[0].UnpricedTokens != 15 || result.RecentItems[0].Totals.UnpricedTokens != 15 {
		t.Fatal("lost partial propagation")
	}
	var unknown int64
	for _, m := range c.ModelRows {
		unknown += m.UnpricedTokens
	}
	if unknown != 15 {
		t.Fatal(c.ModelRows)
	}
	version := c.PricingVersion
	snapshot = grokSnapshot(t, "grok-future", "2")
	changed, err := scanner.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if changed.CostSummary.PricingVersion == version || changed.CostSummary.EstimatedUSD == nil || changed.CostSummary.EstimatedUSD.String() != "0.00003" || calls != 2 {
		t.Fatalf("stale %+v calls=%d", changed.CostSummary, calls)
	}
	snapshot = pricing.Snapshot{}
	unknownResult, err := scanner.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if unknownResult.CostSummary.EstimatedUSD != nil || unknownResult.CostSummary.Confidence != usage.ConfidenceTokensOnly || unknownResult.CostSummary.UnpricedTokens != 30 {
		t.Fatalf("unknown %+v", unknownResult.CostSummary)
	}
}

func TestGrokRemoteAmbiguousTierAndMissingRateAreUnpriced(t *testing.T) {
	d := decimal.FromInt(2)
	for _, m := range []pricing.Model{
		{ProviderID: "xai", ID: "future", Rate: pricing.Rate{Input: &d}},
		{ProviderID: "xai", ID: "future", Rate: pricing.Rate{Input: &d, Output: &d}, Tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &d, Output: &d}}}},
	} {
		snapshot, err := pricing.NewSnapshot([]pricing.Model{m})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, reason := PricingCostWithSnapshot(snapshot, "future", 100000, 0, 1); ok || reason != "missing-price-rule:future" {
			t.Fatalf("ambiguous %v %s", ok, reason)
		}
	}
}

func TestGrokRemoteTierSemanticsAndCatalogIsolation(t *testing.T) {
	base, high := decimal.FromInt(2), decimal.FromInt(7)
	for _, tc := range []struct {
		name   string
		tiers  []pricing.Tier
		legacy *pricing.Rate
		input  int
		priced bool
		want   string
	}{
		{name: "above", tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &high}}}, input: 100001, priced: true, want: "0.700007"},
		{name: "unknown-kind", tiers: []pricing.Tier{{Type: "other", Size: 100000, Rate: pricing.Rate{Input: &high}}}, input: 1},
		{name: "legacy-without-boundary", legacy: &pricing.Rate{Input: &high}, input: 1},
		{name: "real-tier-supersedes-legacy", tiers: []pricing.Tier{{Type: "context", Size: 272000, Rate: pricing.Rate{Input: &high}}}, legacy: &pricing.Rate{Input: &high}, input: 200001, priced: true, want: "0.400002"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "xai", ID: "future", Rate: pricing.Rate{Input: &base}, Tiers: tc.tiers, LegacyLong: tc.legacy}})
			if err != nil {
				t.Fatal(err)
			}
			got, ok, _ := PricingCostWithSnapshot(snapshot, "future", tc.input, 0, 0)
			if ok != tc.priced || (ok && got.String() != tc.want) {
				t.Fatalf("%s %v want %s %v", got, ok, tc.want, tc.priced)
			}
		})
	}
	want, _ := PricingCost("grok-4.5", 200000, 0, 0)
	models := BuiltinPricingModels()
	for _, model := range models {
		*model.LegacyLong.Input = decimal.FromInt(999)
	}
	details := BuiltinPricingDetails()
	*details["grok-4.5"][1].Rate.Input = decimal.FromInt(999)
	got, _ := PricingCost("grok-4.5", 200000, 0, 0)
	if !got.Equal(want) {
		t.Fatal("exported catalog mutated builtin rules")
	}
}

func TestGrokRemotePremiumMissingRateCannotFallBackToStandard(t *testing.T) {
	d := decimal.FromInt(3)
	for _, n := range []int{100000, 100001} {
		snapshot, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "xai", ID: "grok-4.5", Rate: pricing.Rate{Input: &d, Output: &d}, Tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &d}}}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, reason := PricingCostWithSnapshot(snapshot, "grok-4.5", n, 0, 1); ok || reason != "missing-price-rule:grok-4.5" {
			t.Fatalf("%d %v %s", n, ok, reason)
		}
	}
}

func TestGrokBuiltinDetailsResolveAliasesAndPreserveExactSKU(t *testing.T) {
	for _, model := range []string{"grok-4.5", "grok-4.5-latest", "grok-4.5-build", "grok-4.5-build-free", "grok-4.5-build-2026-09-27"} {
		got := BuiltinPricingDetailsFor(model)
		want := BuiltinPricingDetails()["grok-4.5"]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("alias %q: %+v != %+v", model, got, want)
		}
	}
	for _, model := range []string{"grok-4.7-build-fast", "grok-4.7-fast", "grok-build-0.1"} {
		got := BuiltinPricingDetailsFor(model)
		if !reflect.DeepEqual(got, BuiltinPricingDetails()[model]) {
			t.Fatalf("exact SKU %q: %+v", model, got)
		}
	}
	fast := BuiltinPricingDetailsFor("grok-4.7-build-fast")
	if fast[0].Rate.Input.String() != "4" || fast[1].Rate.Input.String() != "6" {
		t.Fatal("exact fast SKU reduced to flagship")
	}
	for _, model := range []string{"grok-future-build", "grok-build", "grok-4.5-other", "other/grok-4.5"} {
		if got := BuiltinPricingDetailsFor(model); got != nil {
			t.Fatalf("unknown borrowed rules %q: %+v", model, got)
		}
	}
	*fast[0].Rate.Input = decimal.FromInt(999)
	*fast[1].Rate.Input = decimal.FromInt(999)
	fresh := BuiltinPricingDetailsFor("grok-4.7-build-fast")
	if fresh[0].Rate.Input.String() != "4" || fresh[1].Rate.Input.String() != "6" {
		t.Fatal("returned rules share builtin state")
	}
}
