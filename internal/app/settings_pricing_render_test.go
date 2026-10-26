package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// Preview uses synthetic public prices, frozen UI time and in-memory settings.
func TestExportPricingSettingsReview(t *testing.T) {
	if os.Getenv("QUOSPARR_PRICING_QA") == "" {
		t.Skip("set QUOSPARR_PRICING_QA=1")
	}
	dir := filepath.Join("..", "..", ".build", "pricing-tabs-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	in, out, cache := decimal.MustParse("2.5"), decimal.MustParse("15"), decimal.MustParse("0.25")
	for _, lang := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese, store.LanguageRussian} {
		for _, width := range []int{settingsMinWidth, settingsWidth} {
			for _, dark := range []bool{false, true} {
				e := newEnv(t, nil)
				e.s.prefs.Language = lang
				e.s.applyLocale()
				var rows []PricingRow
				for _, provider := range store.Providers() {
					for i := 0; i < 7; i++ {
						id := fmt.Sprintf("%s-model-%02d", pricingProviderID(provider), i)
						rows = append(rows, PricingRow{ProviderID: pricingProviderID(provider), ID: id, Name: fmt.Sprintf("%s Model %d", e.s.L().Text(providerKey(provider)), i+1), Input: &in, Output: &out, CacheRead: &cache, Source: "models.dev", Version: "models-dev-example", Available: true})
					}
				}
				rows[6].Input = nil
				rows[6].Output = nil
				rows[6].CacheRead = nil
				snap := PricingSnapshot{Rows: rows, Version: "models-dev-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
				u := newSettingsUI(e.s, SettingsOptions{Version: "dev", Pricing: func() PricingActions { return PricingActions{Snapshot: func() PricingSnapshot { return snap }} }})
				u.tab = tabModelPricing
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
				appearance := "light"
				if dark {
					appearance = "dark"
				}
				name := fmt.Sprintf("%s-%dx%d-%s", lang, width, height, appearance)
				for _, provider := range store.Providers() {
					u.pricingState().switchProvider(provider)
					tt.Frame()
					exportReviewPNG(t, tt, filepath.Join(dir, name+"-"+string(provider)+".png"))
				}
				u.pricingState().switchProvider(store.ProviderCodex)
				u.pricing.syncSettings = true
				tt.Frame()
				exportReviewPNG(t, tt, filepath.Join(dir, name+"-sync.png"))
				u.pricing.customOpen = true
				u.pricing.custom = "14"
				u.pricing.validation = u.L().Pricing("invalid")
				e.s.saveErr = errors.New("disk full")
				tt.Frame()
				exportReviewPNG(t, tt, filepath.Join(dir, name+"-sync-error.png"))
			}
		}
	}
}
