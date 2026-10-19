package app

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// TestMain only starts a desktop when explicitly opted in, and deliberately
// bypasses main.run: no official instance lock, credentials or live services.
func TestMain(m *testing.M) {
	if os.Getenv("RUNWAY_NATIVE_FLOATING_CHILD") == "1" {
		if err := runNativeFloatingHost(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if os.Getenv("RUNWAY_NATIVE_PANEL_HOST") != "1" {
		os.Exit(m.Run())
	}
	if err := runNativePanelHost(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runNativePanelHost() error {
	home, err := os.MkdirTemp("", "quosparr-native-panel-host-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	mygo.App.SetName("Runway Panel Test Host")
	mygo.App.SetVersion("synthetic")
	mygo.App.SetPath(mygo.PathAppData, home)
	if err := DirectMygoPaths(mygo.App, home); err != nil {
		return err
	}
	s := NewSettings(store.New(store.NewMemoryDomain(), nil), System{}, UnavailableCredentials{})
	if err := s.Load(); err != nil {
		return err
	}
	s.prefs.Language = store.LanguageEnglish
	s.prefs.AccountPrivacyModeEnabled = false
	s.prefs.TokenUsageChartStyle = store.ChartStyleLine
	s.persisted = s.prefs.Clone()
	s.applyLocale()
	a := New(Options{Version: "synthetic", Settings: s})
	host := &nativePanelHost{app: a, snap: nativePanelHostSnapshot(), remaining: 2, count: 5}
	a.snap = host.snap
	a.SetPanelActions(host.actions())
	mygo.App.SetMenu(host.menu())
	mygo.App.OnWindowAllClosed(func() {}) // Cmd+O reopens after close/settings.
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.shutdown() })
	mygo.App.WhenReady(host.open)
	fmt.Println("Synthetic native panel host: memory preferences, fixture actions, no live services; Cmd+O opens panel, Cmd+1/2/3 selects heatmap/line/bar")
	return mygo.App.Run()
}

type nativePanelHost struct {
	app                         *App
	snap                        runway.Snapshot
	refreshes, count, remaining int
}

func (h *nativePanelHost) open() {
	if h.app.panel != nil {
		h.app.panel.Focus()
		return
	}
	w, height := h.app.panelSize()
	h.app.panel = mygo.NewWindow(mygo.WindowOptions{Title: "Runway Panel Test Host — synthetic", Width: w, Height: height, UseContentSize: true, DisableResize: true, Content: ui.View(h.app.panelView)})
	h.app.panel.OnClosed(h.app.panelClosed)
	h.app.panel.Focus()
	fmt.Printf("fixture host opened: visible=%t focused=%t bounds=%+v\n", h.app.panel.IsVisible(), h.app.panel.IsFocused(), h.app.panel.Bounds())
}

func (h *nativePanelHost) menu() *mygo.Menu {
	items := []*mygo.MenuItem{{Label: "Open synthetic panel", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { h.open() }}, {Role: mygo.RoleClose}}
	items = append(items, &mygo.MenuItem{Label: "Report synthetic geometry", Accelerator: "CmdOrCtrl+I", Click: func(*mygo.MenuItem, *mygo.Window) {
		if panel := h.app.panel; panel != nil {
			fmt.Printf("fixture geometry: bounds=%+v content=%+v storedHeight=%v previewHeight=%v dragging=%t\n", panel.Bounds(), panel.ContentBounds(), h.app.settings.Prefs().MainPanelHeight, h.app.panelResize.height, h.app.panelResize.dragging)
		}
	}})
	for i, style := range []store.ChartStyle{store.ChartStyleHeatmap, store.ChartStyleLine, store.ChartStyleBar} {
		items = append(items, &mygo.MenuItem{Label: "Chart: " + string(style), Accelerator: fmt.Sprintf("CmdOrCtrl+%d", i+1), Click: func(*mygo.MenuItem, *mygo.Window) { h.app.settings.SetTokenUsageChartStyle(style) }})
	}
	return mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Label: "Fixture", Submenu: items}, {Role: mygo.RoleEditMenu}})
}

func (h *nativePanelHost) actions() PanelActions {
	return PanelActions{
		Refresh: func() { h.refresh("") }, RefreshSection: h.refresh,
		SelectProvider: func(p store.Provider) { h.app.settings.SetSelectedProvider(p) },
		SwitchAccount:  h.switchAccount, QueryCost: h.queryCost,
		QueryReaction: func(ctx context.Context) (resettoday.ReactionSnapshot, error) {
			if err := ctx.Err(); err != nil {
				return resettoday.ReactionSnapshot{}, err
			}
			var snap resettoday.ReactionSnapshot
			runOnMain(func() {
				if ctx.Err() == nil {
					snap = h.reactionSnapshot()
				}
			})
			return snap, ctx.Err()
		},
		React: func(ctx context.Context) (resettoday.ReactionPostResult, error) {
			if err := nativeFixtureDelay(ctx); err != nil {
				return resettoday.ReactionPostResult{}, err
			}
			// Capture the explicit fixture post on the UI thread, just as the real
			// result is delivered there; no external request is made.
			var snap resettoday.ReactionSnapshot
			runOnMain(func() {
				if ctx.Err() == nil {
					h.count++
					h.remaining--
					snap = h.reactionSnapshot()
				}
			})
			if err := ctx.Err(); err != nil {
				return resettoday.ReactionPostResult{}, err
			}
			return resettoday.ReactionPostResult{OK: true, Data: &snap}, nil
		},
	}
}

func (h *nativePanelHost) refresh(section runway.Section) {
	h.refreshes++
	h.snap.Codex.QuotaText = fmt.Sprintf("Plus · synthetic refresh %d", h.refreshes)
	h.snap.Grok.Plan = fmt.Sprintf("SuperGrok · synthetic refresh %d", h.refreshes)
	h.snap.ResetToday.Text = fmt.Sprintf("Synthetic reset evidence · refresh %d", h.refreshes)
	h.app.PublishSnapshot(h.snap)
	fmt.Printf("fixture refresh: %s (%d)\n", section, h.refreshes)
}

func (h *nativePanelHost) switchAccount(id string) {
	if h.app.settings.Prefs().SelectedProvider == store.ProviderGrok {
		h.snap.Grok.Accounts.ActiveID = id
	} else {
		h.snap.Accounts.ActiveID = id
		for _, account := range h.snap.Accounts.Sidebar {
			if account.ID == id {
				h.snap.Accounts.Display = codex.AccountDisplay{Authenticated: true, AccountID: id, DisplayName: account.ResolvedDisplayName(), Email: account.Email, Tier: account.Tier(), SubscriptionExpiresAt: account.SubscriptionExpiresAt}
			}
		}
	}
	h.app.PublishSnapshot(h.snap)
	fmt.Println("fixture account:", id)
}

func (h *nativePanelHost) queryCost(ctx context.Context, provider store.Provider, q CostQuery) (usage.Summary, error) {
	snap := h.app.Snapshot()
	sum := snap.Codex.DisplayedCost.Clone()
	if provider == store.ProviderGrok {
		sum = snap.Grok.Cost.Clone()
	}
	if err := nativeFixtureDelay(ctx); err != nil {
		return usage.Summary{}, err
	}
	at := snap.Codex.Heatmap.CalculatedAt
	switch q.Range {
	case store.CostRangeToday:
		sum.Window = usage.Today(at, time.UTC).Window
		sum.Totals.TotalTokens = 1234
	case store.CostRangeThisMonth:
		sum.Window = usage.ThisMonth(at, time.UTC).Window
		sum.Totals.TotalTokens = 23456
	case store.CostRangePrevious:
		sum.Window = usage.PreviousCycle(sum.Window, time.UTC).Window
		sum.Totals.TotalTokens = 345678
	}
	if q.Custom != nil {
		sum.Window = q.Custom.Window
		sum.Totals.TotalTokens = 456789
	}
	if activePanelAccount(snap, provider) == "work" {
		sum.Totals.TotalTokens += 1000000
	}
	return sum, nil
}

func nativeFixtureDelay(ctx context.Context) error {
	timer := time.NewTimer(400 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (h *nativePanelHost) reactionSnapshot() resettoday.ReactionSnapshot {
	polarity, count, remaining := resettoday.PolarityYes, h.count, h.remaining
	return resettoday.ReactionSnapshot{Enabled: true, Ready: true, Polarity: &polarity, Count: &count, Remaining: &remaining, DailyLimit: 2}
}

func nativePanelHostSnapshot() runway.Snapshot {
	snap := uiReviewDetails(uiReviewSnapshot())
	at := snap.Codex.Heatmap.CalculatedAt
	expiry := at.AddDate(0, 0, 26)
	snap.Accounts.Display = codex.AccountDisplay{Authenticated: true, AccountID: "ada", DisplayName: "personal@example.com", Tier: codex.TierPro200, SubscriptionExpiresAt: &expiry}
	snap.Accounts.Sidebar[0].PlanType = "pro"
	snap.Accounts.Sidebar[0].SubscriptionExpiresAt = &expiry
	snap.Accounts.Sidebar[1].PlanType = "team"
	snap.Accounts.Sidebar[1].SubscriptionExpiresAt = &expiry
	snap.Codex.DisplayedCost.Window = usage.Window{Start: at.AddDate(0, 0, -6), End: at}
	snap.Codex.DisplayedCost.CalculatedAt = at
	snap.Codex.DisplayedCost.PricingVersion = usage.PricingVersion
	snap.Codex.Estimate = nativeHostEstimate(at)
	plan, limit, used := "SuperGrok", int64(15000), int64(277)
	start := at.AddDate(0, 0, -6)
	billing := &grok.BillingSnapshot{Plan: &plan, Period: &grok.BillingPeriod{Kind: grok.BillingMonthly, StartsAt: &start, ResetsAt: &expiry}, IncludedLimitCents: &limit, IncludedUsedCents: &used, IncludedUsagePercent: float64(used) / float64(limit) * 100, Source: grok.BillingSourceCurrent, UpdatedAt: at}
	billing.ResetCredits = &grok.ResetCreditSnapshot{AvailableCount: 3, UpdatedAt: at}
	for i := range 3 {
		billing.ResetCredits.Tokens = append(billing.ResetCredits.Tokens, grok.ResetCreditToken{TokenID: fmt.Sprintf("grok-fixture-%d", i), ValidityStart: &start, ValidityEnd: &expiry})
	}
	credits := billing.ResetCredits.CodexSnapshot(at)
	summary := runway.SummarizeCredits(credits)
	cost := snap.Codex.DisplayedCost.Clone()
	cost.ModelRows[0].Name = "grok-4"
	snap.Grok = runway.GrokState{Availability: runway.GrokReady, Plan: plan, Detail: billing, Meters: []runway.Meter{{Title: "Included quota", RemainingPercent: 98, UsedPercent: 2, Health: runway.HealthGreen}}, Accounts: runway.AccountsView{ActiveID: "ada", Sidebar: []codex.ManagedAccount{{ID: "ada", DisplayName: "Grok personal", Email: "grok-personal@example.com"}, {ID: "work", DisplayName: "Grok workspace", Email: "grok-work@example.com"}}}, Heatmap: runway.HeatmapData{Local: snap.Codex.Heatmap.Local, CalculatedAt: at}, Cost: &cost, ResetCredits: &credits, ResetCreditSummary: &summary}
	snap.ResetToday.Text = "Synthetic reset evidence"
	return snap
}

func nativeHostEstimate(at time.Time) *estimate.Snapshot {
	credits, usd, change := 2000.0, 80.0, 12.5
	previous := estimate.HistorySample{CycleStartDate: "2026-09-28", EstimatedCredits: 1777.8, UsedPercent: 72.4, UsedCredits: 1287.1, RecordedAt: at.AddDate(0, 0, -7)}
	return &estimate.Snapshot{CycleStartDate: "2026-10-05", UsedCredits: 620, UsedPercent: 31, EstimatedCredits: &credits, EstimatedUSD: &usd, CanExtrapolate: true, CalculatedAt: at, PreviousEstimate: &previous, ChangePercent: &change, History: []estimate.HistorySample{previous}, CurrentRows: []estimate.Row{
		{Date: "2026-10-10", Credits: 130, CreditsReported: true, Tokens: 65000, TotalsReported: true},
		{Date: "2026-10-11", Credits: 200, CreditsReported: true, Tokens: 88000, TotalsReported: true},
		{Date: "2026-10-12", Credits: 290, CreditsReported: true, Tokens: 122000, TotalsReported: true},
	}}
}

func TestNativePanelHostFixturesMatchProviderContracts(t *testing.T) {
	snap := nativePanelHostSnapshot()
	if len(snap.Grok.Heatmap.AllDevices) != 0 || len(snap.Grok.Heatmap.Local) == 0 || snap.Grok.Heatmap.OfficialStatsAsOf != "" || !snap.Grok.Heatmap.OfficialGeneratedAt.IsZero() {
		t.Fatal("Grok fixture invented official usage")
	}
	if snap.Grok.Cost.CalculatedAt.IsZero() || snap.Grok.Cost.PricingVersion == "" {
		t.Fatal("cost fixture missing provenance")
	}
	billing, ok := snap.Grok.Detail.(*grok.BillingSnapshot)
	if !ok || billing.IncludedLimitCents == nil || len(snap.Grok.Meters) == 0 || billing.ResetCredits == nil {
		t.Fatal("ready Grok fixture missing billing data")
	}
	if snap.Accounts.Display.AccountID != snap.Accounts.ActiveID || !snap.Accounts.Display.Authenticated || snap.Accounts.Display.SubscriptionExpiresAt == nil {
		t.Fatal("current account fixture missing display metadata")
	}
	if snap.Codex.Estimate == nil || snap.Codex.Estimate.EstimatedUSD == nil || !snap.Codex.Estimate.CurrentRows[0].TotalsReported {
		t.Fatal("estimate fixture missing displayable values")
	}
}
