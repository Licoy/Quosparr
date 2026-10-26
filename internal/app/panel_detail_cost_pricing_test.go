package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func partialCostFixture(now time.Time) usage.Summary {
	amount := decimal.MustParse("1.25")
	zero := decimal.Zero
	sum := usage.Summary{Confidence: usage.ConfidencePartiallyPriced, Source: usage.SourceLocalSessions, EstimatedUSD: &amount, UnpricedTokens: 8000, Totals: usage.Totals{TotalTokens: 10000, UncachedInputTokens: 5000, CachedInputTokens: 3000, OutputTokens: 2000, Turns: 3}, PricingVersion: "openai-builtin-2026-09-27+models-dev-example", Window: usage.Window{Start: now.Add(-time.Hour), End: now}, CalculatedAt: now, Warnings: []string{"unknown-model:new-model", "missing-rate:known-model", "fast-unpriced:fast-model", "ambiguous-tier:tier-model", "price-source:models.dev:priced-model", "malformed-jsonl-lines:2"}}
	sum.DailyRows = []usage.DailyRow{{Date: "2026-10-26", Totals: sum.Totals, EstimatedUSD: &amount, UnpricedTokens: 8000}}
	for i := 0; i < 12; i++ {
		sum.ModelRows = append(sum.ModelRows, usage.BreakdownRow{Name: fmt.Sprintf("model-%02d", i), Totals: usage.Totals{TotalTokens: 1000}, EstimatedUSD: &amount, UnpricedTokens: 50})
	}
	sum.ProjectRows = []usage.BreakdownRow{{Name: "Partial project", Totals: sum.Totals, EstimatedUSD: &amount, UnpricedTokens: 8000}, {Name: "Free project", EstimatedUSD: &zero}}
	return sum
}
func TestCostPartialLabelsAndCompleteMissingList(t *testing.T) {
	l := l10n.New(l10n.English)
	sum := partialCostFixture(time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	issues, sources, scan := costIssueLists(l, sum)
	if len(issues) != 16 || len(sources) != 1 || len(scan) != 1 {
		t.Fatalf("issues=%v sources=%v scan=%v", issues, sources, scan)
	}
	if !strings.Contains(strings.Join(issues, "\n"), "model-11") {
		t.Fatal("missing list truncated at breakdown limit")
	}
	if !strings.Contains(costFigure(l, sum), l.Pricing("partial")) {
		t.Fatal("partial total not labeled")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Children(func() { costResult(c, l, &panelCostState{}, sum) })
	}, 600, 2000)
	tt.Frame()
	if !hasExact(tt, l.PricingRule("modelUnpriced", "model-11", "50")) {
		t.Fatal("last unpriced model absent")
	}
	if !hasExact(tt, "$0.0000") {
		t.Fatalf("free price absent: %q", tt.Texts())
	}
	for _, partial := range []bool{false, true} {
		for _, amount := range []*decimal.Decimal{nil, &decimal.Zero} {
			text := costAmount(l, amount, partial)
			if amount == nil && text != "—" {
				t.Fatal("missing treated as zero")
			}
			if amount != nil && !strings.Contains(text, "$0.00") {
				t.Fatal("free treated as missing")
			}
		}
	}
}
func TestCostSourceMetadataDoesNotMarkPartial(t *testing.T) {
	sum := usage.Summary{Confidence: usage.ConfidencePriced, Warnings: []string{"price-source:models.dev:gpt-example", "malformed-jsonl-lines:1"}}
	if claudePartialCost(sum) {
		t.Fatal("source or scan warning treated as unpriced")
	}
	sum.Confidence = usage.ConfidencePartiallyPriced
	if !claudePartialCost(sum) {
		t.Fatal("partial confidence ignored")
	}
}
func TestRecentCostPartialIsLocalized(t *testing.T) {
	value := swiftjson.MustDecimal("1.25")
	it := sessions.Item{EstimatedUSD: &value, UnpricedTokens: 10}
	l := l10n.New(l10n.SimplifiedChinese)
	if !strings.Contains(recentCostLocalized(l, it), l.Pricing("partial")) {
		t.Fatal("recent partial missing")
	}
	it.EstimatedUSD = nil
	if recentCostLocalized(l, it) != "—" {
		t.Fatal("unknown recent cost not missing")
	}
}
func TestExportPartialCostReview(t *testing.T) {
	if os.Getenv("QUOSPARR_PRICING_QA") == "" {
		t.Skip("set QUOSPARR_PRICING_QA=1")
	}
	dir := filepath.Join("..", "..", ".build", "pricing-cost-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []l10n.Language{l10n.English, l10n.SimplifiedChinese, l10n.Russian} {
		for _, dark := range []bool{false, true} {
			l := l10n.New(lang)
			tt := ui.NewTester(func(c *ui.Context) {
				applyTheme(c)
				ui.Scroll(c).Fill().Padding(18).Gap(14).Children(func() { costResult(c, l, &panelCostState{}, partialCostFixture(c.Now())) })
			}, panelWidth, 2200)
			tt.SetDark(dark)
			exportReviewPNG(t, tt, filepath.Join(dir, fmt.Sprintf("%s-%t.png", lang, dark)))
		}
	}
}
func TestCostPricingNotesStayInVersionTooltip(t *testing.T) {
	l := l10n.New(l10n.English)
	sum := partialCostFixture(time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	for i := range costHelpSources + 3 {
		sum.Warnings = append(sum.Warnings, fmt.Sprintf("price-source:models.dev:extra-%02d", i))
	}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Children(func() { costResult(c, l, &panelCostState{}, sum) })
	}, 600, 2000)
	tt.Frame()
	source := l.PricingRule("pricingSourceModel", "priced-model", "models.dev")
	if hasExact(tt, l.Pricing("currentEstimate")) || hasExact(tt, source) || hasExact(tt, l.Pricing("source")) {
		t.Fatalf("pricing notes shown on the page: %q", tt.Texts())
	}
	if !hasExact(tt, l.PricingRule("unknownModel", "new-model")) || !hasExact(tt, l.PricingRule("scanMalformed", "2")) {
		t.Fatal("unpriced or scan notice hidden")
	}
	help := costPricingHelp(l, sum)
	for _, want := range []string{sum.PricingVersion, l.Pricing("currentEstimate"), source, "… +4"} {
		if !strings.Contains(help, want) {
			t.Fatalf("tooltip lacks %q: %q", want, help)
		}
	}
	if strings.Contains(help, "extra-12") {
		t.Fatal("tooltip lists every source")
	}
}
