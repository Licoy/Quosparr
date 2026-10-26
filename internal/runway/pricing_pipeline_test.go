package runway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/sqliteruntime"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestRecentSessionsUseIndexedRequestsAndOnePriceBook(t *testing.T) {
	if err := sqliteruntime.Load(); err != nil {
		t.Fatalf("SQLite unavailable: %v", err)
	}
	root := t.TempDir()
	home := filepath.Join(root, "codex")
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "rollout-aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.jsonl")
	data := `{"timestamp":"2026-10-14T00:00:00Z","type":"session_meta","payload":{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}}
{"timestamp":"2026-10-14T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":60000,"cached_input_tokens":0,"output_tokens":0}}},"turn_context":{"model":"online-model"}}
{"timestamp":"2026-10-14T00:02:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":60000,"cached_input_tokens":0,"output_tokens":0}}},"turn_context":{"model":"online-model"}}
{"timestamp":"2026-10-14T00:03:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":150000,"cached_input_tokens":0,"output_tokens":0}}},"turn_context":{"model":"online-model"}}
{"timestamp":"2026-10-14T00:04:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":5,"output_tokens":0}}},"turn_context":{"model":"unknown"}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	other := strings.ReplaceAll(data, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	other = strings.ReplaceAll(other, "2026-10-14", "2026-10-13")
	if err := os.WriteFile(filepath.Join(dir, "rollout-bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.jsonl"), []byte(other), 0600); err != nil {
		t.Fatal(err)
	}
	base, high := decimal.FromInt(2), decimal.FromInt(7)
	makeSnapshot := func() pricing.Snapshot {
		s, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "openai", ID: "online-model", Rate: pricing.Rate{Input: &base}, Tiers: []pricing.Tier{{Type: "context", Size: 100000, Rate: pricing.Rate{Input: &high}}}}})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	snapshot := makeSnapshot()
	calls := 0
	prices := usage.SnapshotPrices(func() pricing.Snapshot { calls++; return snapshot })
	repo, err := usage.NewRepository(usage.Config{CodexHome: home, DatabasePath: filepath.Join(root, "usage.sqlite3"), Location: time.UTC, Prices: prices})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	sources := NewLiveSources(LiveConfig{Store: &codex.Store{}, CodexHome: home, Repo: repo, Prices: prices, Now: func() time.Time { return now }})
	result, err := sources.ScanRecentSessions(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatal(result)
	}
	item := result.Items[0]
	if item.ID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || item.EstimatedUSD == nil || item.EstimatedUSD.String() != "1.29" || item.UnpricedTokens != 5 || calls != 1 {
		t.Fatalf("indexed requests %+v books=%d", item, calls)
	}
	// The two 60k requests must retain the base tier despite sharing a model
	// A second source must not enter the selected source's amount
	base, high = decimal.Zero, decimal.Zero
	snapshot = makeSnapshot()
	free, err := sources.ScanRecentSessions(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if free.Items[0].EstimatedUSD == nil || free.Items[0].EstimatedUSD.Sign() != 0 || free.Items[0].UnpricedTokens != 5 || calls != 2 {
		t.Fatalf("reprice %+v books=%d", free, calls)
	}
	snapshot = pricing.Snapshot{}
	unknown, err := sources.ScanRecentSessions(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Items[0].EstimatedUSD != nil || unknown.Items[0].UnpricedTokens != 270005 || calls != 3 {
		t.Fatalf("unknown %+v books=%d", unknown, calls)
	}
}

func TestDirectCostScanUsesInjectedPrices(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "2026", "10", "14")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-10-14T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"output_tokens":0}}},"turn_context":{"model":"online-only"}}`
	if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	rate := decimal.FromInt(2)
	snapshot, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "openai", ID: "online-only", Rate: pricing.Rate{Input: &rate}}})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	l := live{LiveConfig: LiveConfig{CodexHome: home, Prices: usage.SnapshotPrices(func() pricing.Snapshot { calls++; return snapshot })}}
	w := usage.Window{Start: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}
	out, err := l.scanCost(context.Background(), []usage.Query{{ID: "a", Window: w}, {ID: "b", Window: w}}, w.End, usage.RefreshIfChanged, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || out["a"].EstimatedUSD == nil || out["a"].EstimatedUSD.String() != "0.00002" || out["b"].PricingVersion != out["a"].PricingVersion {
		t.Fatalf("direct %+v calls=%d", out, calls)
	}
}

func TestPartialCostFormattingAndWidgetForEveryLanguage(t *testing.T) {
	for _, lang := range l10n.Languages() {
		for _, provider := range []string{"openai", "anthropic", "xai"} {
			t.Run(string(lang)+"/"+provider, func(t *testing.T) {
				lc := l10n.New(lang)
				zero := decimal.Zero
				now := time.Unix(1800000000, 0)
				summary := usage.Summary{Source: usage.SourceLocalSessions, Confidence: usage.ConfidencePartiallyPriced, EstimatedUSD: &zero, UnpricedTokens: 5, Totals: usage.Totals{TotalTokens: 15}, PricingVersion: provider, CalculatedAt: now, Warnings: []string{"price-source:builtin:known", "unknown-model:future", "missing-price-rule:tier", "fast-unpriced:fast"}}
				display := DisplayCost(lc, summary, store.CostRangeCurrent, "", now)
				if !strings.Contains(display.Text, lc.Pricing("partial")) || !strings.Contains(display.Subtitle, lc.Pricing("partial")) {
					t.Fatalf("partial %+v", display)
				}
				var lines string
				for _, line := range display.Lines {
					lines += line.Title + line.Value
				}
				if strings.Contains(lines, "price-source:") || !strings.Contains(lines, lc.PricingRule("unknownModel", "future")) || !strings.Contains(lines, lc.PricingRule("missingPrice", "tier")) || !strings.Contains(lines, lc.PricingRule("fastUnpriced", "fast")) {
					t.Fatal(lines)
				}
				p := codexWidgetProvider(CodexView{Cost: &summary})
				if !p.APIEquivalentCostPartial || p.APIEquivalentCostUSD == nil {
					t.Fatal(p)
				}
				summary.Confidence = usage.ConfidencePriced
				summary.UnpricedTokens = 0
				display = DisplayCost(lc, summary, store.CostRangeCurrent, "", now)
				if strings.Contains(display.Text, lc.Pricing("partial")) || codexWidgetProvider(CodexView{Cost: &summary}).APIEquivalentCostPartial {
					t.Fatal("priced zero mislabeled partial")
				}
				summary.EstimatedUSD = nil
				summary.Confidence = usage.ConfidenceTokensOnly
				summary.UnpricedTokens = 15
				display = DisplayCost(lc, summary, store.CostRangeCurrent, "", now)
				if !strings.Contains(display.Text, lc.Text(l10n.KeyTokensOnly)) || codexWidgetProvider(CodexView{Cost: &summary}).APIEquivalentCostPartial {
					t.Fatal("unknown must be tokens only")
				}
			})
		}
	}
}

func TestExportCostRetainsPartialCounts(t *testing.T) {
	d := decimal.Zero
	row := usage.BreakdownRow{Name: "m", UnpricedTokens: 5, EstimatedUSD: &d}
	c := exportCost(usage.Summary{Confidence: usage.ConfidencePartiallyPriced, EstimatedUSD: &d, UnpricedTokens: 5, DailyRows: []usage.DailyRow{{Date: "2026-10-14", UnpricedTokens: 5}}, ModelRows: []usage.BreakdownRow{row}, ProjectRows: []usage.BreakdownRow{row}, ClientRows: []usage.BreakdownRow{row}})
	if c.Confidence != "partiallyPriced" || c.UnpricedTokens != 5 || c.DailyRows[0].UnpricedTokens != 5 || c.ModelRows[0].UnpricedTokens != 5 || c.ProjectRows[0].UnpricedTokens != 5 || c.ClientRows[0].UnpricedTokens != 5 {
		t.Fatalf("export %+v", c)
	}
}
