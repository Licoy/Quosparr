package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/pricing"
	"testing"
)

func TestPricingOfficialRowsUseThreeProviderTabs(t *testing.T) {
	e := newEnv(t, nil)
	d := pricingTestDecimal("1")
	catalog, err := pricing.NewSnapshot([]pricing.Model{
		{ProviderID: "openai", ID: "official-gpt", Name: "Official GPT", Rate: pricing.Rate{Input: d, Output: d}},
		{ProviderID: "anthropic", ID: "official-claude", Name: "Official Claude", Rate: pricing.Rate{Input: d, Output: d}},
		{ProviderID: "xai", ID: "official-grok", Name: "Official Grok", Rate: pricing.Rate{Input: d, Output: d}},
		{ProviderID: "openrouter", ID: "official-gpt", Name: "Proxy GPT", Rate: pricing.Rate{Input: d, Output: d}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rows := pricingRows(catalog, e.s.L())
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot { return PricingSnapshot{Rows: rows} }}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	tt.Frame()

	for _, tc := range []struct{ label, id string }{{"Codex", "official-gpt"}, {"Claude", "official-claude"}, {"Grok", "official-grok"}} {
		if err := tt.Click(tc.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		for _, id := range []string{"official-gpt", "official-claude", "official-grok"} {
			if hasExact(tt, id) != (id == tc.id) {
				t.Fatalf("%s displayed wrong row %s", tc.label, id)
			}
		}
		if hasExact(tt, "Proxy GPT") || hasExact(tt, "OpenRouter") {
			t.Fatal("third-party provider shown")
		}
	}
	count := 0
	for _, text := range tt.Texts() {
		if text == u.L().Pricing("all") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected only state filter, got %d All texts", count)
	}
}
