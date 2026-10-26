package app

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/licoy/quosparr/internal/store"
)

func TestPricingTabsResetSearchFilterPageAndDetails(t *testing.T) {
	e := newEnv(t, nil)
	rows := []PricingRow{{ProviderID: "openai", ID: "same", Name: "Same model"}, {ProviderID: "anthropic", ID: "same", Name: "Same model"}, {ProviderID: "xai", ID: "same", Name: "Same model"}}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot { return PricingSnapshot{Rows: rows} }}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	s := u.pricingState()
	if s.provider != store.ProviderCodex || len(s.cached) != 1 {
		t.Fatal("default Codex must include unpriced model")
	}
	for _, tc := range []struct {
		label    string
		provider store.Provider
	}{{"Claude", store.ProviderClaude}, {"Grok", store.ProviderGrok}, {"Codex", store.ProviderCodex}} {
		s.search = "no match"
		s.filter = "available"
		s.sort = "input"
		s.page = 5
		s.open = pricingProviderID(s.provider) + "/same"
		tt.Frame()
		if err := tt.Click(u.L().Pricing("back")); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if err := tt.Click(tc.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if s.provider != tc.provider || s.search != "" || s.filter != "" || s.sort != "" || s.open != "" || s.page != 0 {
			t.Fatalf("tab state not reset %+v", s)
		}
		if !hasExact(tt, "Same model") || len(s.cached) != 1 || s.cached[0].ProviderID != pricingProviderID(tc.provider) {
			t.Fatal("tab did not show its full model list")
		}
	}
	if hasExact(tt, u.L().Pricing("previous")) || hasExact(tt, u.L().Pricing("next")) {
		t.Fatal("small list shows disabled paging buttons")
	}
}
func TestPricingModalEditsPersistAndClosesOnEscapeOrNavigation(t *testing.T) {
	e := newEnv(t, nil)
	u := newSettingsUI(e.s, SettingsOptions{})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	l := u.L()
	if hasExact(tt, l.Pricing("auto")) || hasExact(tt, l.Pricing("lastAttempt")) {
		t.Fatal("sync controls leaked onto list")
	}
	if err := tt.Click(l.Pricing("syncSettings")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"auto", "interval", "lastAttempt", "lastSuccess", "lastChanged", "nextSync", "source", "version", "globalSync"} {
		if !hasExact(tt, l.Pricing(key)) && key != "interval" {
			t.Fatalf("modal field %s absent", key)
		}
	}
	if err := tt.Click(l.Pricing("auto")); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if e.stored().ModelPricing.AutoSync {
		t.Fatal("auto toggle not saved")
	}
	if err := tt.Click(l.Pricing("interval")); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(pricingIntervalLabel(u, 1800)); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if e.stored().ModelPricing.SyncIntervalSeconds != 1800 {
		t.Fatal("preset not saved")
	}
	if err := tt.Click(l.Pricing("interval")); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(l.Pricing("custom")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(l.Pricing("custom")); err != nil {
		t.Fatal(err)
	}
	tt.Command("selectAll")
	tt.Type("90")
	e.settle()
	tt.Frame()
	if e.stored().ModelPricing.SyncIntervalSeconds != 5400 {
		t.Fatalf("custom not saved: %s %+v", u.pricing.custom, e.stored().ModelPricing)
	}
	tt.Key(0, ui.KeyEscape)
	if u.pricing.syncSettings || hasExact(tt, l.Pricing("auto")) {
		t.Fatal("Escape did not close modal")
	}
	if err := tt.Click(l.Pricing("syncSettings")); err != nil {
		t.Fatal(err)
	}
	u.tab = tabAdvanced
	tt.Frame()
	if u.pricing.syncSettings || hasExact(tt, l.Pricing("globalSync")) {
		t.Fatal("modal survived tab navigation")
	}
}
func TestPricingModalShowsSaveFailureAndReadOnly(t *testing.T) {
	e := newEnv(t, nil)
	u := newSettingsUI(e.s, SettingsOptions{})
	u.tab = tabModelPricing
	u.pricingState().syncSettings = true
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	l := u.L()
	e.mem.fail.Store(true)
	if err := tt.Click(l.Pricing("auto")); err != nil {
		t.Fatal(err)
	}
	e.settle()
	tt.Frame()
	if !e.s.Prefs().ModelPricing.AutoSync || e.s.SaveError() == nil || !tt.HasText("disk full") {
		t.Fatalf("save rollback/error missing: %q", tt.Texts())
	}
	e.s.loadErr = errors.New("unreadable preferences")
	tt.Frame()
	if !tt.HasText("unreadable preferences") {
		t.Fatal("modal read-only explanation absent")
	}
	before := e.s.Prefs()
	if err := tt.Click(l.Pricing("auto")); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if e.s.Prefs().ModelPricing.AutoSync != before.ModelPricing.AutoSync {
		t.Fatal("read-only control changed preferences")
	}
	if err := tt.Click(l.Pricing("done")); err != nil {
		t.Fatal(err)
	}
	if u.pricing.syncSettings {
		t.Fatal("done button did not close")
	}
}

func TestClosingPricingModalKeepsManualSyncRunning(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	actions := PricingActions{Refresh: func(ctx context.Context) error {
		started <- ctx
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	}}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions { return actions }})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsWidth, settingsHeight)
	if err := tt.Click(u.L().Pricing("syncSettings")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(u.L().Pricing("refresh")); err != nil {
		t.Fatal(err)
	}
	ctx := <-started
	tt.Key(0, ui.KeyEscape)
	if u.pricing.syncSettings || ctx.Err() != nil || !u.pricing.task.Running {
		t.Fatal("modal close canceled the sync")
	}
	close(release)
	finishDetail(t, queue)
	tt.Frame()
	if u.pricing.task.Running || u.pricing.task.Err != nil {
		t.Fatal("sync did not complete after modal closed")
	}
}

func TestPricingListAmountsAlignWithHeader(t *testing.T) {
	e := newEnv(t, nil)
	d := pricingTestDecimal("2.5")
	row := PricingRow{ProviderID: "openai", ID: "model", Name: "Model", Input: d, Output: d, CacheRead: d}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot { return PricingSnapshot{Rows: []PricingRow{row}} }}
	}})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	header, ok := tt.Find(u.L().Pricing("input"))
	if !ok {
		t.Fatal("input header absent")
	}
	price, ok := tt.Find("$2.5")
	if !ok {
		t.Fatal("input price absent")
	}
	if math.Abs(float64(header.X+header.W-price.X-price.W)) > 1 {
		t.Fatalf("input price and header not aligned: %v %v", header, price)
	}
}
