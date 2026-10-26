package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestExportPricingDetailReview(t *testing.T) {
	if os.Getenv("QUOSPARR_PRICING_QA") == "" {
		t.Skip("set QUOSPARR_PRICING_QA=1")
	}
	dir := filepath.Join("..", "..", ".build", "pricing-detail-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	in, out, cache, zero := decimal.MustParse("2.5"), decimal.MustParse("15"), decimal.MustParse("0.25"), decimal.Zero
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageRussian, store.LanguageFrench} {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			for _, dark := range []bool{false, true} {
				for _, kind := range []string{"standard", "complex", "missing"} {
					e := newEnv(t, nil)
					e.s.prefs.Language = lang
					e.s.applyLocale()
					l := e.s.L()
					provider := store.ProviderCodex
					row := PricingRow{ProviderID: "openai", ProviderName: "OpenAI", ID: "gpt-example", Name: "GPT Example", Input: &in, Output: &out, CacheRead: &zero, Source: "models.dev", SourceURL: "https://models.dev", Version: "models-dev-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Available: true}
					if kind == "complex" {
						provider = store.ProviderClaude
						row.ProviderID, row.ProviderName, row.ID, row.Name = "anthropic", "Anthropic", "claude-opus-example", "Claude Opus Example"
						row.CacheRead = &cache
						row.SourceURL = "https://platform.claude.com/docs/en/about-claude/pricing"
						row.Details = l.PricingRule("ruleApplied", l.Pricing("builtin"), "models.dev") + "\n\n" + l.PricingRule("ruleTTL", "5m", "$3.125") + "\n" + l.PricingRule("ruleTTL", "1h", "$5") + "\n" + l.PricingRule("ruleTier", ">200000", l.Pricing("input")+" $5 · "+l.Pricing("output")+" $22.5") + "\n" + l.PricingRule("ruleFast", l.Pricing("input")+" $15 · "+l.Pricing("output")+" $75") + "\n" + l.PricingRule("ruleGeoUS", "1.1") + "\n\n" + l.Pricing("rulePreserved")
					}
					if kind == "missing" {
						provider = store.ProviderGrok
						row.ProviderID, row.ProviderName, row.ID, row.Name = "xai", "xAI", "grok-example", "Grok Example"
						row.Input, row.Output, row.CacheRead = nil, nil, nil
						row.Available = false
						row.Details = l.PricingRule("missingPrice", row.ID) + "\n" + l.Pricing("catalogLimitations")
					}
					snap := PricingSnapshot{Rows: []PricingRow{row}, Version: row.Version}
					u := newSettingsUI(e.s, SettingsOptions{Version: "dev", Pricing: func() PricingActions {
						return PricingActions{Snapshot: func() PricingSnapshot { return snap }, Refresh: func(context.Context) error { return nil }}
					}})
					u.tab = tabModelPricing
					u.pricingState().provider = provider
					u.pricing.open = row.ProviderID + "/" + row.ID
					height := settingsHeight
					if width == settingsMinWidth {
						height = settingsMinHeight
					}
					tt := ui.NewTester(u.view, width, height)
					snap.LastAttempt = tt.Now()
					snap.LastSuccess = tt.Now()
					snap.LastChanged = tt.Now()
					snap.NextSync = tt.Now().Add(time.Hour)
					tt.SetDark(dark)
					theme := "light"
					if dark {
						theme = "dark"
					}
					name := fmt.Sprintf("%s-%dx%d-%s-%s", lang, width, height, theme, kind)
					tt.Frame()
					exportReviewPNG(t, tt, filepath.Join(dir, name+".png"))
					if kind == "complex" {
						tt.Scroll(float32(width-100), float32(height-120), 0, 10000)
						exportReviewPNG(t, tt, filepath.Join(dir, name+"-rules.png"))
					}
					if lang == store.LanguageEnglish && width == settingsMinWidth && dark && kind == "standard" {
						for _, surface := range []string{"list", "detail", "modal"} {
							u.pricing.open = ""
							u.pricing.syncSettings = false
							if surface == "detail" {
								u.pricing.open = row.ProviderID + "/" + row.ID
							}
							if surface == "modal" {
								u.pricing.syncSettings = true
							}
							snap.Syncing = true
							tt.Frame()
							exportReviewPNG(t, tt, filepath.Join(dir, "busy-"+surface+"-a.png"))
							tt.Advance(90 * time.Millisecond)
							exportReviewPNG(t, tt, filepath.Join(dir, "busy-"+surface+"-b.png"))
						}
					}
				}
			}
		}
	}
}

// Refresh only the busy frames after indicator styling changes.
func TestExportPricingBusyReview(t *testing.T) {
	if os.Getenv("QUOSPARR_PRICING_QA") == "" {
		t.Skip("set QUOSPARR_PRICING_QA=1")
	}
	dir := filepath.Join("..", "..", ".build", "pricing-detail-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{"list", "detail", "modal"} {
		e := newEnv(t, nil)
		row := PricingRow{ProviderID: "openai", ID: "gpt-example", Name: "GPT Example", Input: pricingTestDecimal("2.5"), Output: pricingTestDecimal("15"), CacheRead: pricingTestDecimal("0"), Source: "models.dev", SourceURL: "https://models.dev", Version: "models-dev-example", Available: true}
		snap := PricingSnapshot{Rows: []PricingRow{row}, Version: row.Version, Syncing: true}
		u := newSettingsUI(e.s, SettingsOptions{Version: "dev", Pricing: func() PricingActions {
			return PricingActions{Snapshot: func() PricingSnapshot { return snap }, Refresh: func(context.Context) error { return nil }}
		}})
		u.tab = tabModelPricing
		if surface == "detail" {
			u.pricingState().open = "openai/gpt-example"
		}
		if surface == "modal" {
			u.pricingState().syncSettings = true
		}
		tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
		snap.LastAttempt = tt.Now()
		snap.LastSuccess = tt.Now()
		tt.SetDark(true)
		exportReviewPNG(t, tt, filepath.Join(dir, "busy-"+surface+"-a.png"))
		tt.Advance(90 * time.Millisecond)
		exportReviewPNG(t, tt, filepath.Join(dir, "busy-"+surface+"-b.png"))
	}
}
