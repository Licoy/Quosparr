package app

import (
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/pricing"
	"testing"
)

func TestPricingCatalogOnlyOfficialProviderRows(t *testing.T) {
	d := pricingTestDecimal("2")
	models := []pricing.Model{}
	official := map[string]string{"openai": "OpenAI", "anthropic": "Anthropic", "xai": "xAI"}
	for provider, id := range map[string]string{"openai": "gpt-shared", "anthropic": "claude-shared", "xai": "grok-shared"} {
		models = append(models, pricing.Model{ProviderID: provider, ProviderName: "Wrong name", ID: id, Name: id, Rate: pricing.Rate{Input: d, Output: d}})
		for _, other := range []string{"openrouter", "azure", "nvidia", "OpenAI", "openai-compatible"} {
			models = append(models, pricing.Model{ProviderID: other, ID: id, Name: id, Rate: pricing.Rate{Input: d, Output: d}})
		}
	}
	catalog, err := pricing.NewSnapshot(models)
	if err != nil {
		t.Fatal(err)
	}
	offline := pricingRows(pricing.Snapshot{}, l10n.New(l10n.English))
	rows := pricingRows(catalog, l10n.New(l10n.English))
	if len(rows) != len(offline)+3 {
		t.Fatalf("rows=%d offline=%d, want exactly three official additions", len(rows), len(offline))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		want, ok := official[row.ProviderID]
		if !ok || row.ProviderName != want {
			t.Fatalf("nonofficial row or name: %+v", row)
		}
		seen[row.ProviderID+"/"+row.ID] = true
	}
	for _, key := range []string{"openai/gpt-shared", "anthropic/claude-shared", "xai/grok-shared", "openai/gpt-5", "anthropic/claude-haiku-4-5", "xai/grok-4.5"} {
		if !seen[key] {
			t.Fatalf("official or offline builtin missing: %s", key)
		}
	}
}
