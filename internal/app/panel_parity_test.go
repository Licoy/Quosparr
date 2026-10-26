package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestExportPanelParity(t *testing.T) {
	if os.Getenv("RUNWAY_PANEL_PARITY_QA") == "" {
		t.Skip("set RUNWAY_PANEL_PARITY_QA to export fixed synthetic panels")
	}
	dir := filepath.Join("..", "..", ".build", "v1-parity-comparison")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	snap := uiReviewDetails(uiReviewSnapshot())
	expiry := time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)
	snap.Accounts.Display = codex.AccountDisplay{Authenticated: true, DisplayName: "personal@example.com", Tier: codex.TierPro200, SubscriptionExpiresAt: &expiry}
	snap.Codex.QuotaText = "Pro 200"
	estimated, usd := 2000.0, 80.0
	snap.Codex.Estimate = &estimate.Snapshot{CycleStartDate: "2026-10-05", UsedCredits: 620, UsedPercent: 31, EstimatedCredits: &estimated, EstimatedUSD: &usd, CanExtrapolate: true, CalculatedAt: snap.Codex.Heatmap.CalculatedAt, CurrentRows: []estimate.Row{
		{Date: "2026-10-10", Credits: 130, CreditsReported: true, Tokens: 65000, TotalsReported: true},
		{Date: "2026-10-11", Credits: 200, CreditsReported: true, Tokens: 88000, TotalsReported: true},
		{Date: "2026-10-12", Credits: 290, CreditsReported: true, Tokens: 122000, TotalsReported: true},
	}}
	snap.Codex.DisplayedCost.Window = usage.Window{Start: expiry.AddDate(0, -1, 0), End: expiry}
	snap.Codex.DisplayedCost.Totals.Turns = 32
	snap.Codex.DisplayedCost.DailyRows = []usage.DailyRow{{Date: "2026-10-11", Totals: usage.Totals{TotalTokens: 900000}}, {Date: "2026-10-12", Totals: usage.Totals{TotalTokens: 1775321}}}
	snap.Codex.DisplayedCost.ClientRows = []usage.BreakdownRow{{Name: "Codex CLI", Totals: snap.Codex.DisplayedCost.Totals, EstimatedUSD: snap.Codex.DisplayedCost.EstimatedUSD}}
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		a, tt := newPanel(t, func(p *store.Preferences) {
			p.Language = store.LanguageSimplifiedChinese
			p.ShowsRecentSessions = true
		}, snap, PanelActions{})
		tt.SetSize(panelWidth, 584)
		tt.SetDark(dark)
		for _, page := range []struct {
			name   string
			module store.Module
		}{{"main", ""}, {"reset-credits", store.ModuleResetCredits}, {"api-cost", store.ModuleAPICost}, {"quota-estimate", store.ModuleQuotaEstimate}} {
			a.cancelPanelDetails()
			a.panelState.detail = page.module
			tt.Frame()
			exportReviewPNG(t, tt, filepath.Join(dir, "go-"+page.name+"-"+theme+".png"))
		}
		a.panelState.detail = ""
		tt.SetSize(panelWidth, 1400)
		exportReviewPNG(t, tt, filepath.Join(dir, "go-main-full-"+theme+".png"))
	}
}

func TestPanelParityGrokCreditsRequireOfficialSummary(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[available], func(t *testing.T) {
			snap := panelFixture()
			snap.Grok.Availability = runway.GrokReady
			if available {
				snap.Grok.ResetCreditSummary = &runway.ResetCreditSummary{AvailableCount: 0, TotalCount: 0}
			}
			_, tt := newPanel(t, func(p *store.Preferences) {
				p.SelectedProvider = store.ProviderGrok
				p.MainPanelModuleOrder = []store.Module{store.ModuleResetCredits, store.ModuleQuota}
			}, snap, PanelActions{})
			if shown := hasExact(tt, l10n.New(l10n.English).Text(l10n.KeyResetCredits)); shown != available {
				t.Fatalf("reset credits visible = %v, want %v; texts %q", shown, available, tt.Texts())
			}
		})
	}
}

func TestPanelParityEstimateKeepsUnreportedTotalsUnknownAndNewestFirst(t *testing.T) {
	l := l10n.New(l10n.English)
	snap := &estimate.Snapshot{CurrentRows: []estimate.Row{
		{Date: "2026-10-10", Credits: 1, CreditsReported: true, Tokens: 700},
		{Date: "2026-10-11", Credits: 2, CreditsReported: true, Tokens: 800, TotalsReported: true},
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			estimateDetail(c, l, runway.CodexView{Estimate: snap})
		})
	}, 368, 500)
	texts := strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, "700") {
		t.Fatalf("unreported token total presented as known; texts %q", tt.Texts())
	}
	if strings.Contains(texts, l.Text(l10n.KeyQuotaEstimateThisWeekUsed)+" 0.0") {
		t.Fatalf("incomplete Credits presented as known zero; texts %q", tt.Texts())
	}
	if strings.Index(texts, "2026-10-11") > strings.Index(texts, "2026-10-10") {
		t.Fatalf("current rows are oldest first; texts %q", tt.Texts())
	}
}

func TestPanelParityEstimatePreservesReferenceValueAndUsagePercent(t *testing.T) {
	l := l10n.New(l10n.English)
	credits, usd := 2000.0, 80.0
	s := &estimate.Snapshot{EstimatedCredits: &credits, EstimatedUSD: &usd, UsedPercent: 31, CanExtrapolate: true}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			estimateDetail(c, l, runway.CodexView{Estimate: s})
		})
	}, 368, 500)
	if !tt.HasText("$80.00") || !hasExact(tt, "31%") || !tt.HasText(l.Text(l10n.KeyQuotaEstimateWeeklyAllowance)) {
		t.Fatalf("estimate omits reference USD or official used percent; texts %q", tt.Texts())
	}
}

func TestPanelParityCurrentSubscriptionVisibleWithOrWithoutSidebar(t *testing.T) {
	for _, managed := range []bool{false, true} {
		for _, language := range []store.Language{store.LanguageEnglish, store.LanguageSimplifiedChinese} {
			t.Run(string(language)+map[bool]string{false: "/external", true: "/managed"}[managed], func(t *testing.T) {
				snap := panelFixture()
				expiry := time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)
				snap.Accounts.Display = codex.AccountDisplay{Authenticated: true, DisplayName: "ada@example.com", Tier: codex.TierPro200, SubscriptionExpiresAt: &expiry}
				if managed {
					snap.Accounts.Sidebar[0].PlanType = "pro"
					snap.Accounts.Sidebar[0].SubscriptionExpiresAt = &expiry
				} else {
					snap.Accounts.Sidebar = nil
				}
				a, tt := newPanel(t, func(p *store.Preferences) {
					p.Language = language
					p.AccountPrivacyModeEnabled = true
				}, snap, PanelActions{})
				if !hasExact(tt, a.L().Text(l10n.KeyPlanPro200)) || !hasExact(tt, map[store.Language]string{store.LanguageEnglish: "Nov 7, 2026", store.LanguageSimplifiedChinese: "2026/11/7"}[language]) {
					t.Fatalf("current subscription missing; texts %q", tt.Texts())
				}
				if tt.HasText("ada@example.com") {
					t.Fatal("subscription row revealed email in privacy mode")
				}
			})
		}
	}
}

func TestPanelParityCreditsBalanceClosesQuotaSection(t *testing.T) {
	snap := panelFixture()
	expiry := time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)
	balance := 42.5
	snap.Accounts.Display = codex.AccountDisplay{
		Authenticated: true, DisplayName: "ada@example.com", Tier: codex.TierPlus,
		SubscriptionExpiresAt: &expiry,
	}
	snap.Codex.Quota = &codexapi.QuotaSnapshot{CreditsBalance: &balance}
	a, tt := newPanel(t, func(p *store.Preferences) {
		p.Language = store.LanguageSimplifiedChinese
	}, snap, PanelActions{})
	l := a.L()
	texts := tt.Texts()
	label := slices.Index(texts, l.Text(l10n.KeyCreditsBalance))
	amount := slices.Index(texts, "42.50")
	expiryAt := slices.Index(texts, "2026/11/7")
	quota := slices.Index(texts, l.Text(l10n.KeyQuota))
	meter := slices.Index(texts, l10n.Format("%d%% %s", 70, l.Text(l10n.KeyLeft)))
	if label < 0 || amount < 0 || expiryAt < 0 || quota < 0 || meter < 0 {
		t.Fatalf("credits, subscription or quota text missing; texts %q", texts)
	}
	if !(expiryAt < quota && meter < label && label < amount) {
		t.Fatalf("credits balance should follow the last quota meter, not the subscription chip; texts %q", texts)
	}
}

func TestPanelParityCreditsBalanceHiddenWhenDisabled(t *testing.T) {
	snap := panelFixture()
	expiry := time.Date(2026, 11, 7, 12, 0, 0, 0, time.Local)
	balance := 42.5
	snap.Accounts.Display = codex.AccountDisplay{
		Authenticated: true, DisplayName: "ada@example.com", Tier: codex.TierPlus,
		SubscriptionExpiresAt: &expiry,
	}
	snap.Codex.Quota = &codexapi.QuotaSnapshot{CreditsBalance: &balance}
	a, tt := newPanel(t, func(p *store.Preferences) {
		p.Language = store.LanguageSimplifiedChinese
		p.ShowsCreditsBalance = false
	}, snap, PanelActions{})
	if hasExact(tt, a.L().Text(l10n.KeyCreditsBalance)) || hasExact(tt, "42.50") {
		t.Fatalf("credits balance chip should be hidden; texts %q", tt.Texts())
	}
	if !hasExact(tt, "2026/11/7") {
		t.Fatalf("subscription chip should still be visible; texts %q", tt.Texts())
	}
}

func TestPanelParityRecentCostShowsCurrency(t *testing.T) {
	price, err := swiftjson.ParseDecimal("18.42")
	if err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			recentRow(c, l10n.New(l10n.English), sessions.Item{ID: "fixture", Title: "Synthetic session", EstimatedUSD: &price}, false, true)
		})
	}, 368, 100)
	if !hasExact(tt, "$18.4200") {
		t.Fatalf("recent session cost has no USD currency; texts %q", tt.Texts())
	}
}

func TestPanelParityPrivacyMasksAccountErrorEmail(t *testing.T) {
	snap := panelFixture()
	snap.Accounts.Sidebar[0].LastError = "fixture rejected for ada@example.com"
	a, tt := newPanel(t, func(p *store.Preferences) { p.AccountPrivacyModeEnabled = true }, snap, PanelActions{})
	if err := tt.Click(a.L().Text(l10n.KeyAccounts)); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("ada@example.com") {
		t.Fatal("account error revealed an email while privacy mode is enabled")
	}
	if !tt.HasText("fixture rejected for a***@***.com") {
		t.Fatal("masking removed the visible account failure")
	}
}

func TestPanelParityCostDetailIncludesDailyAndClientRows(t *testing.T) {
	l := l10n.New(l10n.English)
	s := &usage.Summary{
		Source: usage.SourceOnlineAnalytics, RawCredits: 12.345, Totals: usage.Totals{Turns: 23},
		DailyRows:  []usage.DailyRow{{Date: "2026-10-10", Totals: usage.Totals{TotalTokens: 100}}, {Date: "2026-10-11", Totals: usage.Totals{TotalTokens: 200}}},
		ClientRows: []usage.BreakdownRow{{Name: "fixture-client", Totals: usage.Totals{TotalTokens: 300}}},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { costResult(c, l, &panelCostState{}, *s) })
	}, 368, 500)
	if !tt.HasText("2026-10-11") || !tt.HasText("fixture-client") || !hasExact(tt, "23") || !tt.HasText("12.345") {
		t.Fatalf("daily/client cost breakdown missing; texts %q", tt.Texts())
	}
	texts := strings.Join(tt.Texts(), "\n")
	if strings.Index(texts, "2026-10-11") > strings.Index(texts, "2026-10-10") {
		t.Fatal("cost daily rows are oldest first")
	}
}

func TestPanelParityCostDailyRowsFollowSwiftFormat(t *testing.T) {
	usd := decimal.MustParse("1.50")
	s := usage.Summary{DailyRows: []usage.DailyRow{
		{Date: "unpriced", Totals: usage.Totals{TotalTokens: 800, Turns: 3}},
		{Date: "priced", Totals: usage.Totals{TotalTokens: 1_250_000, Turns: 42}, EstimatedUSD: &usd},
	}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { costDailyTable(c, l10n.New(l10n.English), s.DailyRows, "Range") })
	}, 368, 500)
	for _, want := range []string{"800", "1.25M", "$1.5000", "—", "42"} {
		if !hasExact(tt, want) {
			t.Fatalf("daily row cell %q missing; texts %q", want, tt.Texts())
		}
	}
}

func TestPanelParityCreditDetailPreservesRiskAndRemainingDuration(t *testing.T) {
	l := l10n.New(l10n.English)
	v := runway.CodexView{ResetCreditSummary: &runway.ResetCreditSummary{AvailableCount: 3, StableAvailableCount: 2, ExpiringCount: 1, UnavailableCount: 1, TotalCount: 4}, CreditDetails: []runway.CreditDetail{{Title: "fixture credit", State: runway.CreditExpiring, ExpiresAt: panelFixture().Codex.Heatmap.CalculatedAt, RemainingDuration: 48 * time.Hour}}}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { creditsDetail(c, l, v) })
	}, 368, 500)
	if !hasExact(tt, l.Text(l10n.KeyExpiryRisk)) || !hasExact(tt, l.Text(l10n.KeyAvailable)+" 2") || !hasExact(tt, l.Text(l10n.KeyExpiringSoon)+" 1") || !hasExact(tt, l.Text(l10n.KeyUnavailableCredits)+" 1") || !hasExact(tt, "2 days") {
		t.Fatalf("credit risk counts or remaining duration omitted; texts %q", tt.Texts())
	}
}

func TestPanelParityGrokChartUsesLocalCalendar(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("fixture UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = previous })
	for _, provider := range []store.Provider{store.ProviderCodex, store.ProviderGrok} {
		t.Run(string(provider), func(t *testing.T) {
			snap := panelFixture()
			heat := runway.HeatmapData{Local: map[string]int64{"2027-01-01": 900}, CalculatedAt: time.Date(2027, 1, 1, 0, 30, 0, 0, time.Local)}
			snap.Codex.Heatmap, snap.Grok.Heatmap = heat, heat
			_, tt := newPanel(t, func(p *store.Preferences) {
				p.SelectedProvider = provider
				p.TokenUsageChartStyle = store.ChartStyleLine
			}, snap, PanelActions{})
			if shown := hasExact(tt, "2027-01-01"); shown != (provider == store.ProviderGrok) {
				t.Fatalf("local Jan 1 data visible = %v for %s; texts %q", shown, provider, tt.Texts())
			}
			if provider == store.ProviderGrok {
				r, ok := tt.Find("2027-01-01")
				if !ok {
					t.Fatal("local day missing")
				}
				t.Logf("tooltip hit box: %+v", r)
				tt.Move(r.X+r.W/2, r.Y+r.H/2)
				tt.Advance(650 * time.Millisecond)
				if !tt.HasText("900") {
					t.Fatal("local day tooltip lost its token count")
				}
			}
		})
	}
}

func TestPanelParityGrokTooltipHidesOfficialSource(t *testing.T) {
	snap := panelFixture()
	snap.Grok.Heatmap = runway.HeatmapData{Local: map[string]int64{"2026-10-12": 900}, CalculatedAt: snap.Codex.Heatmap.CalculatedAt}
	_, tt := newPanel(t, func(p *store.Preferences) { p.SelectedProvider = store.ProviderGrok }, snap, PanelActions{})
	r, ok := tt.Find("2026-10-12")
	if !ok {
		t.Fatal("fixture day missing")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(650 * time.Millisecond)
	joined := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(joined, "2026-10-12\n") {
		t.Fatalf("tooltip did not open; texts %q", tt.Texts())
	}
	if strings.Contains(joined, l10n.New(l10n.English).Text(l10n.KeyHeatmapAllDevices)) {
		t.Fatalf("Grok tooltip claims official stats; texts %q", tt.Texts())
	}
}

func TestPanelParityTrendUsesMonthlyLabel(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, style := range []store.ChartStyle{store.ChartStyleHeatmap, store.ChartStyleLine, store.ChartStyleBar} {
		t.Run(string(style), func(t *testing.T) {
			state := &panelState{}
			tt := ui.NewTester(func(c *ui.Context) {
				applyTheme(c)
				ui.Column(c).Fill().Children(func() {
					first := true
					panelToken(c, l, style, panelFixture(), PanelActions{}, &first, state)
				})
			}, 368, 240)
			key := l10n.KeyHeatmapMonthly
			if style == store.ChartStyleHeatmap {
				key = l10n.KeyHeatmapCumulative
			}
			if !hasExact(tt, l.Text(key)) {
				t.Fatalf("%s chart lacks %q; texts %q", style, l.Text(key), tt.Texts())
			}
		})
	}
}

func TestPanelParityCreditsBalanceHiddenWhenZero(t *testing.T) {
	snap := panelFixture()
	balance := 0.0
	snap.Accounts.Display = codex.AccountDisplay{Authenticated: true, DisplayName: "ada@example.com", Tier: codex.TierPlus}
	snap.Codex.Quota = &codexapi.QuotaSnapshot{CreditsBalance: &balance}
	a, tt := newPanel(t, func(p *store.Preferences) {
		p.Language = store.LanguageSimplifiedChinese
	}, snap, PanelActions{})
	if hasExact(tt, a.L().Text(l10n.KeyCreditsBalance)) || hasExact(tt, "0.00") {
		t.Fatalf("zero credits balance should be hidden; texts %q", tt.Texts())
	}
}
