package claude

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func claudeSnapshot(t *testing.T, id, value string) pricing.Snapshot {
	t.Helper()
	d := decimal.MustParse(value)
	s, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "anthropic", ID: id, Rate: pricing.Rate{Input: &d, CacheRead: &d, Output: &d}}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSnapshotRetainsClaudeSpecialRules(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-4-5", "claude-haiku-5-5"} {
		snapshot := claudeSnapshot(t, model, "999")
		for _, parts := range []TokenParts{{Input: 1, CacheWrite5m: 2, CacheWrite1h: 3, CacheRead: 4, Output: 5}, {Input: 200001, Output: 3}, {Input: 100001, Output: 3}, {Input: 200000, Fast: true, Output: 3}} {
			for _, geo := range []string{"", "us"} {
				// Ordinary base requests intentionally use the remote group
				p := pricingTable[model]
				special := parts.Fast || parts.CacheWrite5m > 0 || geo == "us" || (p.above != nil && parts.PromptTokens() > p.threshold)
				if !special {
					continue
				}
				want, ok := PricingCost(model, parts, geo)
				got, priced, _ := PricingCostWithSnapshot(snapshot, model, parts, geo)
				if priced != ok || (ok && !got.Equal(want)) {
					t.Fatalf("%s %+v %s = %s %v; want %s %v", model, parts, geo, got, priced, want, ok)
				}
			}
		}
	}
	for _, n := range []int64{100000, 100001, 200000, 200001} {
		model := "claude-haiku-5-5"
		if n >= 200000 {
			model = "claude-sonnet-4-5"
		}
		// Cache writes select the complete builtin group on both boundary sides
		parts := TokenParts{Input: n - 1, CacheWrite1h: 1, Output: 1}
		want, _ := PricingCost(model, parts, "")
		got, ok, _ := PricingCostWithSnapshot(claudeSnapshot(t, model, "999"), model, parts, "")
		if !ok || !got.Equal(want) {
			t.Fatalf("boundary %d: %s != %s", n, got, want)
		}
	}
}

func TestClaudeSnapshotRepricesCachedRecordsAndPartialAmounts(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "projects", "demo")
	mustMkdir(t, dir)
	writeJSONL(t, filepath.Join(dir, "s.jsonl"),
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-14T00:00:00Z","cwd":"/work/demo","message":{"id":"a","model":"claude-future","usage":{"output_tokens":10}}}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-14T00:00:01Z","cwd":"/work/demo","message":{"id":"b","model":"claude-future","usage":{"output_tokens":20,"speed":"fast"}}}`,
	)
	snapshot := claudeSnapshot(t, "claude-future", "0")
	calls := 0
	scanner := SessionScanner{ConfigDir: home, Location: time.UTC, Cache: &SessionCache{}, PricingSnapshot: func() pricing.Snapshot { calls++; return snapshot }}
	opts := SessionScanOptions{RecentLimit: 1, Now: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}
	result, err := scanner.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	c := result.CostSummary
	if c.Confidence != usage.ConfidencePartiallyPriced || c.UnpricedTokens != 20 || c.EstimatedUSD == nil || c.EstimatedUSD.Sign() != 0 || calls != 1 {
		t.Fatalf("partial zero %+v calls=%d", c, calls)
	}
	if c.DailyRows[0].UnpricedTokens != 20 || c.ModelRows[0].UnpricedTokens != 20 || c.ProjectRows[0].UnpricedTokens != 20 || c.ClientRows[0].UnpricedTokens != 20 || result.RecentItems[0].Totals.UnpricedTokens != 20 {
		t.Fatal("lost partial propagation")
	}
	if len(c.Warnings) != 1 || c.Warnings[0] != "fast-unpriced:claude-future" {
		t.Fatal(c.Warnings)
	}
	version := c.PricingVersion
	snapshot = claudeSnapshot(t, "claude-future", "2")
	changed, err := scanner.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if changed.CostSummary.EstimatedUSD == nil || !changed.CostSummary.EstimatedUSD.Equal(decimal.MustParse("0.00002")) || changed.CostSummary.PricingVersion == version || calls != 2 {
		t.Fatalf("stale cached price %+v", changed.CostSummary)
	}
	snapshot = pricing.Snapshot{}
	unknown, err := scanner.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.CostSummary.EstimatedUSD != nil || unknown.CostSummary.UnpricedTokens != 30 || unknown.CostSummary.Confidence != usage.ConfidenceTokensOnly {
		t.Fatalf("unknown %+v", unknown.CostSummary)
	}
	if !strings.Contains(version, "anthropic-builtin-") {
		t.Fatal(version)
	}
}

func TestClaudeSnapshotExactIDPrecedesAlias(t *testing.T) {
	snapshot := claudeSnapshot(t, "claude-haiku-4-5-20251001", "9")
	got, ok, _ := PricingCostWithSnapshot(snapshot, "claude-haiku-4-5-20251001", TokenParts{Output: 1000000}, "")
	if !ok || got.String() != "9" {
		t.Fatalf("exact ID %s %v", got, ok)
	}
	unknown := claudeSnapshot(t, "claude-future", "2")
	if _, ok, reason := PricingCostWithSnapshot(unknown, "claude-future", TokenParts{CacheWrite1h: 1}, ""); ok || reason != "missing-price-rule:claude-future" {
		t.Fatalf("TTL %v %s", ok, reason)
	}
}

func TestClaudeRemoteTierSemanticsAndCatalogIsolation(t *testing.T) {
	base, high := decimal.FromInt(2), decimal.FromInt(7)
	for _, tc := range []struct {
		name   string
		tiers  []pricing.Tier
		legacy *pricing.Rate
		input  int64
		priced bool
		want   string
	}{
		{name: "above", tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &high}}}, input: 100001, priced: true, want: "0.700007"},
		{name: "equal", tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &high}}}, input: 100000},
		{name: "unknown-kind", tiers: []pricing.Tier{{Type: "other", Size: 100000, Rate: pricing.Rate{Input: &high}}}, input: 1},
		{name: "legacy-without-boundary", legacy: &pricing.Rate{Input: &high}, input: 1},
		{name: "real-tier-supersedes-legacy", tiers: []pricing.Tier{{Type: "context", Size: 272000, Rate: pricing.Rate{Input: &high}}}, legacy: &pricing.Rate{Input: &high}, input: 200001, priced: true, want: "0.400002"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "anthropic", ID: "future", Rate: pricing.Rate{Input: &base}, Tiers: tc.tiers, LegacyLong: tc.legacy}})
			if err != nil {
				t.Fatal(err)
			}
			got, ok, _ := PricingCostWithSnapshot(snapshot, "future", TokenParts{Input: tc.input}, "")
			if ok != tc.priced || (ok && got.String() != tc.want) {
				t.Fatalf("%s %v want %s %v", got, ok, tc.want, tc.priced)
			}
		})
	}
	want, _ := PricingCost("claude-opus-5-5", TokenParts{Input: 1, Fast: true}, "us")
	details := BuiltinPricingDetails()
	*details["claude-opus-5-5"][0].Rate.Input = decimal.FromInt(999)
	*details["claude-opus-5-5"][len(details["claude-opus-5-5"])-1].Multiplier = decimal.FromInt(999)
	models := BuiltinPricingModels()
	for _, model := range models {
		*model.Rate.Input = decimal.FromInt(999)
	}
	got, _ := PricingCost("claude-opus-5-5", TokenParts{Input: 1, Fast: true}, "us")
	if !got.Equal(want) {
		t.Fatal("exported catalog mutated pricing rules")
	}
}

func TestClaudeScanExactIDAndFullyPricedZero(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "projects", "p")
	mustMkdir(t, dir)
	writeJSONL(t, filepath.Join(dir, "s.jsonl"), `{"type":"assistant","sessionId":"s","timestamp":"2026-10-14T00:00:00Z","message":{"id":"a","model":"claude-haiku-4-5-20251001","usage":{"output_tokens":1000000}}}`)
	snapshot := claudeSnapshot(t, "claude-haiku-4-5-20251001", "9")
	scanner := SessionScanner{ConfigDir: home, Location: time.UTC, PricingSnapshot: func() pricing.Snapshot { return snapshot }}
	result, err := scanner.Scan(context.Background(), SessionScanOptions{RecentLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.CostSummary.EstimatedUSD == nil || result.CostSummary.EstimatedUSD.String() != "9" {
		t.Fatalf("exact scan %+v", result.CostSummary)
	}
	snapshot = claudeSnapshot(t, "claude-haiku-4-5-20251001", "0")
	result, err = scanner.Scan(context.Background(), SessionScanOptions{RecentLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.CostSummary.Confidence != usage.ConfidencePriced || result.CostSummary.UnpricedTokens != 0 || result.CostSummary.EstimatedUSD == nil || result.CostSummary.EstimatedUSD.Sign() != 0 {
		t.Fatalf("free %+v", result.CostSummary)
	}
}

func TestClaudeRemotePremiumMissingRateCannotFallBackToStandard(t *testing.T) {
	d := decimal.FromInt(3)
	for _, n := range []int64{100000, 100001} {
		snapshot, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "anthropic", ID: "claude-opus-4-5", Rate: pricing.Rate{Input: &d, Output: &d}, Tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &d}}}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok, reason := PricingCostWithSnapshot(snapshot, "claude-opus-4-5", TokenParts{Input: n, Output: 1}, ""); ok || reason != "missing-price-rule:claude-opus-4-5" {
			t.Fatalf("%d %v %s", n, ok, reason)
		}
	}
}

func TestClaudeBuiltinDetailsResolveOnlySupportedAliases(t *testing.T) {
	for _, model := range []string{"claude-haiku-4-5", "claude-haiku-4-5-20251001", "claude-haiku-4-5@2025-10-01", "us.anthropic.claude-haiku-4-5-20251001-v1:0", " claude-haiku-4-5[1m] "} {
		got := BuiltinPricingDetailsFor(model)
		want := BuiltinPricingDetails()["claude-haiku-4-5"]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("alias %q: %+v != %+v", model, got, want)
		}
	}
	for _, model := range []string{"claude-future", "claude-opus-4-7-fast", "claude-haiku-4-5-latest", "other/claude-haiku-4-5"} {
		if got := BuiltinPricingDetailsFor(model); got != nil {
			t.Fatalf("unknown borrowed rules %q: %+v", model, got)
		}
	}
	rules := BuiltinPricingDetailsFor("claude-opus-5-5-20261001")
	for i := range rules {
		if rules[i].Rate.Input != nil {
			*rules[i].Rate.Input = decimal.FromInt(999)
		}
		if rules[i].CacheWrite5m != nil {
			*rules[i].CacheWrite5m = decimal.FromInt(999)
		}
		if rules[i].CacheWrite1h != nil {
			*rules[i].CacheWrite1h = decimal.FromInt(999)
		}
		if rules[i].Multiplier != nil {
			*rules[i].Multiplier = decimal.FromInt(999)
		}
	}
	fresh := BuiltinPricingDetailsFor("claude-opus-5-5")
	if fresh[0].Rate.Input.String() != "4" || fresh[0].CacheWrite5m.String() != "5" || fresh[0].CacheWrite1h.String() != "8" || fresh[len(fresh)-1].Multiplier.String() != "1.1" {
		t.Fatalf("rules were mutated: %+v", fresh)
	}
}
