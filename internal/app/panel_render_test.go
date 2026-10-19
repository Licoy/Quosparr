package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// Fixtures stay in tests; exporting never opens a native window or reads credentials.
func TestExportUIReview(t *testing.T) {
	phase := os.Getenv("RUNWAY_UI_QA")
	if phase == "" {
		t.Skip("set RUNWAY_UI_QA=before or after to export review PNGs")
	}
	dir := filepath.Join("..", "..", ".build", "v1-ui-qa")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		snap := uiReviewSnapshot()
		a, panel := newPanel(t, nil, snap, PanelActions{})
		if err := a.settings.LoadProxy(); err != nil {
			t.Fatal(err)
		}
		width, height := a.panelSizeWithin(1080, 1040)
		panel.SetSize(width, height)
		panel.SetDark(dark)
		exportReviewPNG(t, panel, filepath.Join(dir, phase+"-panel-"+theme+".png"))
		u := newSettingsUI(a.settings, SettingsOptions{Version: "1.0.0"})
		for _, tab := range []settingsTab{tabGeneral, tabDisplay} {
			u.tab = tab
			view := ui.NewTester(u.view, settingsWidth, settingsHeight)
			view.SetDark(dark)
			name := "general"
			if tab == tabDisplay {
				name = "display"
			}
			exportReviewPNG(t, view, filepath.Join(dir, phase+"-"+name+"-"+theme+".png"))
		}
		if phase == "after" {
			a.settings.sys.StatusBarStyles = true
			a.settings.sys.PanelBackground = true
			u.tab = tabDisplay
			mac := ui.NewTester(u.view, settingsWidth, settingsHeight)
			mac.SetDark(dark)
			exportReviewPNG(t, mac, filepath.Join(dir, phase+"-display-mac-enabled-"+theme+".png"))
			a.settings.sys.StatusBarStyles = false
			a.settings.sys.PanelBackground = false
			for _, style := range []store.ChartStyle{store.ChartStyleLine, store.ChartStyleBar} {
				chartState := &panelState{}
				chart := ui.NewTester(func(c *ui.Context) {
					applyTheme(c)
					ui.Column(c).Fill().Padding(16).Gap(8).Children(func() { panelChart(c, a.L(), style, store.ProviderCodex, snap.Codex.Heatmap, false, chartState) })
				}, 400, 240)
				chart.SetDark(dark)
				exportReviewPNG(t, chart, filepath.Join(dir, phase+"-chart-"+string(style)+"-"+theme+".png"))
			}
			detailed := uiReviewDetails(snap)
			a.snap = detailed
			for _, module := range []store.Module{store.ModuleResetCredits, store.ModuleAPICost} {
				a.panelState.detail = module
				panel.Frame()
				exportReviewPNG(t, panel, filepath.Join(dir, phase+"-details-"+string(module)+"-"+theme+".png"))
			}
			a.panelState.detail = ""
			a.snap = snap
		}
		u.tab = tabAccounts
		f := &accountForm{}
		accounts := ui.NewTester(func(c *ui.Context) {
			applyTheme(c)
			u.shell(c, func(c *ui.Context) {
				ui.Scroll(c).Grow(1).Padding(16).Gap(12).Children(func() {
					l := u.L()
					accountToolbar(c, u, f, l.Text(l10n.KeyAccounts), accountFolder{l.Text(l10n.KeyCodexFolder), "/codex"}, func(*ui.Menu) {}, func() string { return "" }, func() string { return "" })
					for _, row := range snap.Accounts.Sidebar {
						renderCodexRow(c, u, f, nil, row, false)
					}
				})
			})
		}, settingsWidth, settingsHeight)
		accounts.SetDark(dark)
		exportReviewPNG(t, accounts, filepath.Join(dir, phase+"-accounts-"+theme+".png"))
	}
}

func exportReviewPNG(t *testing.T, tt *ui.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, tt.Image()); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}

func uiReviewSnapshot() runway.Snapshot {
	snap := panelFixture()
	snap.Accounts.Sidebar = []codex.ManagedAccount{
		{ID: "ada", DisplayName: "Personal", Email: "personal@example.com"},
		{ID: "work", DisplayName: "Workspace", Email: "workspace@example.com"},
	}
	snap.Codex.QuotaText = "Plus"
	snap.Codex.Meters = []runway.Meter{
		{Title: "5-hour", RemainingPercent: 73, UsedPercent: 27, ResetText: "Resets in 2h 18m", Health: runway.HealthGreen},
		{Title: "Weekly", RemainingPercent: 46, UsedPercent: 54, ResetText: "Resets Oct 15, 10:30", Health: runway.HealthYellow},
	}
	snap.Codex.Heatmap.AllDevices = map[string]int64{}
	snap.Codex.Heatmap.Local = map[string]int64{}
	at := snap.Codex.Heatmap.CalculatedAt
	for i := 0; i < 180; i++ {
		day := at.AddDate(0, 0, -i).Format(time.DateOnly)
		if i%7 != 0 {
			snap.Codex.Heatmap.AllDevices[day] = int64(24000 + (i*8737)%148000)
			snap.Codex.Heatmap.Local[day] = int64(9000 + (i*697)%51000)
		}
	}
	snap.Codex.CostText = "$18.42"
	snap.Codex.CostSubtitle = "API equivalent · local sessions"
	snap.Codex.ResetCreditsText = "3 available / 5 total"
	snap.ResetToday.Text = "No reset today"
	return snap
}

func uiReviewDetails(snap runway.Snapshot) runway.Snapshot {
	now := snap.Codex.Heatmap.CalculatedAt
	snap.Codex.ResetCredits = &codexapi.ResetCreditsSnapshot{AvailableCount: 3, UpdatedAt: now, Credits: []codexapi.ResetCredit{
		{ID: "fixture-one", Status: "available", ExpiresAt: now.Add(48 * time.Hour), RemainingSeconds: 48 * 3600},
		{ID: "fixture-two", Status: "available", ExpiresAt: now.Add(18 * 24 * time.Hour), RemainingSeconds: 18 * 24 * 3600},
		{ID: "fixture-three", Status: "available", ExpiresAt: now.Add(27 * 24 * time.Hour), RemainingSeconds: 27 * 24 * 3600},
		{ID: "fixture-used", Status: "used", ExpiresAt: now.Add(10 * 24 * time.Hour)},
	}}
	snap.Codex.ResetCreditsText = ""
	summary := runway.SummarizeCredits(*snap.Codex.ResetCredits)
	snap.Codex.ResetCreditSummary = &summary
	price, _ := decimal.Parse("18.42")
	snap.Codex.DisplayedCost = &usage.Summary{Source: usage.SourceLocalSessions, Confidence: usage.ConfidencePriced, EstimatedUSD: &price, Totals: usage.Totals{TotalTokens: 2675321, UncachedInputTokens: 658741, CachedInputTokens: 1903418, OutputTokens: 113162}, ModelRows: []usage.BreakdownRow{{Name: "gpt-5.2-codex", Totals: usage.Totals{TotalTokens: 2675321}, EstimatedUSD: &price}}, ProjectRows: []usage.BreakdownRow{{Name: "Quosparr", Totals: usage.Totals{TotalTokens: 2675321}, EstimatedUSD: &price}}}
	return snap
}
