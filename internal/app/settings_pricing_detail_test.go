package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
)

func pricingIndicatorPixels(t *testing.T, tt *ui.Tester, label string, iconOnly bool) []byte {
	t.Helper()
	box, ok := tt.Find(label)
	if !ok {
		t.Fatalf("sync button %q absent", label)
	}
	x := box.X + 10
	if iconOnly {
		x = box.X + (box.W-14)/2
	}
	y := box.Y + (box.H-14)/2
	rect := image.Rect(int(x), int(y), int(x)+14, int(y)+14)
	crop := image.NewRGBA(image.Rect(0, 0, 14, 14))
	draw.Draw(crop, crop.Bounds(), tt.Image(), rect.Min, draw.Src)
	return bytes.Clone(crop.Pix)
}
func TestPricingSyncIndicatorsAnimateAcrossListDetailAndModal(t *testing.T) {
	for _, surface := range []string{"list", "detail", "modal"} {
		for _, manual := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/manual=%v", surface, manual), func(t *testing.T) {
				e := newEnv(t, nil)
				snap := PricingSnapshot{Rows: []PricingRow{{ProviderID: "openai", ID: "model", Name: "Example Model", Details: "Complete rules"}}}
				var calls atomic.Int32
				a := PricingActions{Snapshot: func() PricingSnapshot { return snap }, Refresh: func(_ context.Context) error { calls.Add(1); return nil }}
				u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions { return a }})
				u.tab = tabModelPricing
				s := u.pricingState()
				if surface == "detail" {
					s.open = "openai/model"
				}
				if surface == "modal" {
					s.syncSettings = true
				}
				if manual {
					s.task.Running = true
				} else {
					snap.Syncing = true
				}
				tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
				iconOnly := surface == "list"
				label := u.L().Pricing("refresh")
				if iconOnly {
					label = u.L().Text(l10n.KeyRefresh)
				}
				before := pricingIndicatorPixels(t, tt, label, iconOnly)
				animated := false
				for range 4 {
					tt.Advance(90 * time.Millisecond)
					if !bytes.Equal(before, pricingIndicatorPixels(t, tt, label, iconOnly)) {
						animated = true
						break
					}
				}
				if !animated {
					t.Fatal("native spinner did not animate inside sync button")
				}
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 0 {
					t.Fatal("busy sync button started another request")
				}
				snap.Syncing = false
				s.task = Task{}
				snap.LastSuccess = tt.Now()
				snap.Error = "visible failure"
				tt.Frame()
				if !tt.HasText("visible failure") {
					t.Fatal("sync error hidden")
				}
				tt.Advance(time.Second)
				stopped := pricingIndicatorPixels(t, tt, label, iconOnly)
				tt.Advance(180 * time.Millisecond)
				if !bytes.Equal(stopped, pricingIndicatorPixels(t, tt, label, iconOnly)) {
					t.Fatal("finished sync indicator kept animating")
				}
			})
		}
	}
}
func TestPricingDetailPreservesListStateAndDropsRemovedModel(t *testing.T) {
	e := newEnv(t, nil)
	rows := make([]PricingRow, 201)
	for i := range rows {
		rows[i] = PricingRow{ProviderID: "openai", ID: fmt.Sprint(i), Name: fmt.Sprintf("Model %03d", i), Details: "All billing rules stay intact\nLast regional rule"}
	}
	snap := PricingSnapshot{Rows: rows, DataVersion: "one"}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions { return PricingActions{Snapshot: func() PricingSnapshot { return snap }} }})
	u.tab = tabModelPricing
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	s := u.pricingState()
	s.search = "Model"
	s.filter = "reference"
	s.sort = "name"
	s.cachedList(snap)
	s.page = 1
	tt.Frame()
	if err := tt.Click("Model 100"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, key := range []string{"back", "tokenPrices", "pricingOrigin", "pricingRules"} {
		if !tt.HasText(u.L().Pricing(key)) {
			t.Fatalf("detail section missing: %s", key)
		}
	}
	if hasExact(tt, "Claude") || hasExact(tt, "Grok") {
		t.Fatal("list provider tabs leaked into independent detail")
	}
	if !hasExact(tt, rows[100].Details) {
		t.Fatal("rules were dropped or reparsed")
	}
	if err := tt.Click(u.L().Pricing("back")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if s.page != 1 || s.search != "Model" || s.filter != "reference" || s.sort != "name" || !hasExact(tt, "Model 100") {
		t.Fatal("return lost list state")
	}
	s.open = "openai/100"
	tt.Frame()
	snap.Rows = rows[:1]
	snap.DataVersion = "two"
	tt.Frame()
	if s.open != "" || hasExact(tt, "Model 100") || hasExact(tt, u.L().Pricing("pricingRules")) {
		t.Fatal("removed model left stale detail")
	}
}

func TestPricingDetailModalSyncTargetsOverlayAction(t *testing.T) {
	e := newEnv(t, nil)
	queue := detailMainQueue(t)
	var calls atomic.Int32
	snap := PricingSnapshot{Rows: []PricingRow{{ProviderID: "openai", ID: "model", Name: "Model"}}}
	actions := PricingActions{Snapshot: func() PricingSnapshot { return snap }, Refresh: func(context.Context) error { calls.Add(1); return nil }}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions { return actions }})
	u.tab = tabModelPricing
	u.pricingState().open = "openai/model"
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	if err := tt.Click(u.L().Pricing("syncSettings")); err != nil {
		t.Fatal(err)
	}
	scope, ok := tt.Find(u.L().Pricing("globalSync"))
	if !ok {
		t.Fatal("sync modal absent")
	}
	done, ok := tt.Find(u.L().Pricing("done"))
	if !ok {
		t.Fatal("modal footer absent")
	}
	// Target the modal footer itself; the underlying detail has the same action.
	tt.ClickAt(scope.X+20, done.Y+done.H/2)
	finishDetail(t, queue)
	tt.Frame()
	if calls.Load() != 1 || !u.pricing.syncSettings || u.pricing.open != "openai/model" {
		t.Fatal("overlay sync missed or changed underlying detail")
	}
}

func TestPricingDetailSourceActionUsesSafeURL(t *testing.T) {
	e := newEnv(t, nil)
	row := PricingRow{ProviderID: "openai", ID: "source", Name: "Source Model", SourceURL: "https://models.dev"}
	u := newSettingsUI(e.s, SettingsOptions{Pricing: func() PricingActions {
		return PricingActions{Snapshot: func() PricingSnapshot { return PricingSnapshot{Rows: []PricingRow{row}} }}
	}})
	u.tab = tabModelPricing
	u.pricingState().open = "openai/source"
	tt := ui.NewTester(u.view, settingsMinWidth, settingsMinHeight)
	if err := tt.Click(u.L().Pricing("openSource")); err != nil {
		t.Fatal(err)
	}
	urls := tt.OpenedURLs()
	if len(urls) != 1 || urls[0] != row.SourceURL {
		t.Fatalf("source link not opened: %v", urls)
	}
	row.SourceURL = "javascript:alert(1)"
	tt.Frame()
	if hasExact(tt, u.L().Pricing("openSource")) {
		t.Fatal("unsafe source link displayed")
	}
}
