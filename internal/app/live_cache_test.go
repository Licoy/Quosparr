package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

func TestQueryCacheUsesEachProviderLocalStore(t *testing.T) {
	home := t.TempDir()
	quosparr := filepath.Join(home, ".quosparr")
	codexStore := codex.NewStore(filepath.Join(home, "custom-codex"), quosparr)
	grokMod := grok.NewModule(grok.NewStore(filepath.Join(home, "custom-grok"), quosparr))
	claudeMod := claude.NewModule(claude.NewStore(filepath.Join(home, "custom-claude"), quosparr, home, "custom-claude", ""))
	rt := &liveApp{
		codex: codexStore, grokMod: grokMod, grok: newGrokBridge(grokMod, store.DefaultPreferences(), nil),
		claudeMod: claudeMod, claudeB: newClaudeBridge(claudeMod, store.DefaultPreferences(), nil),
	}
	window := usage.Window{Start: time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), End: time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)}
	for _, provider := range store.Providers() {
		report, err := rt.queryCache(context.Background(), provider, CostQuery{Custom: &usage.CostRange{Window: window}})
		if err != nil {
			t.Fatalf("%s: %v", provider, err)
		}
		if !report.Start.Equal(window.Start) || !report.End.Equal(window.End) {
			t.Errorf("%s window = %v - %v", provider, report.Start, report.End)
		}
	}
}

func TestCacheWindowRequiresReportedProviderCycles(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	grokStart, grokEnd := now.Add(-5*24*time.Hour), now.Add(2*24*time.Hour)
	claudeEnd := now.Add(3 * 24 * time.Hour)
	gb := &grokBridge{state: runway.GrokState{Detail: &grok.BillingSnapshot{Period: &grok.BillingPeriod{StartsAt: &grokStart, ResetsAt: &grokEnd}}}}
	cb := &claudeBridge{state: runway.ClaudeState{Detail: &claude.UsageSnapshot{Windows: []claude.UsageWindow{{Kind: claude.WindowSevenDay, ResetsAt: &claudeEnd}}}}}
	rt := &liveApp{grok: gb, claudeB: cb}

	for _, tc := range []struct {
		provider store.Provider
		rangeKey store.CostRange
		start    time.Time
		end      time.Time
	}{
		{store.ProviderGrok, store.CostRangeCurrent, grokStart, now},
		{store.ProviderGrok, store.CostRangePrevious, grokStart.Add(-7 * 24 * time.Hour), grokStart},
		{store.ProviderClaude, store.CostRangeCurrent, claudeEnd.Add(-7 * 24 * time.Hour), now},
		{store.ProviderClaude, store.CostRangePrevious, claudeEnd.Add(-14 * 24 * time.Hour), claudeEnd.Add(-7 * 24 * time.Hour)},
	} {
		got, err := rt.cacheWindow(context.Background(), tc.provider, CostQuery{Range: tc.rangeKey}, now)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.provider, tc.rangeKey, err)
		}
		if !got.Start.Equal(tc.start) || !got.End.Equal(tc.end) {
			t.Errorf("%s/%s = %v - %v, want %v - %v", tc.provider, tc.rangeKey, got.Start, got.End, tc.start, tc.end)
		}
	}

	if _, err := (&liveApp{grok: &grokBridge{}}).cacheWindow(context.Background(), store.ProviderGrok, CostQuery{Range: store.CostRangeCurrent}, now); !errors.Is(err, errGrokCycleUnavailable) {
		t.Errorf("missing Grok cycle error = %v", err)
	}
	staleStart, staleEnd := now.Add(-8*24*time.Hour), now.Add(-24*time.Hour)
	stale := &liveApp{grok: &grokBridge{state: runway.GrokState{Detail: &grok.BillingSnapshot{Period: &grok.BillingPeriod{StartsAt: &staleStart, ResetsAt: &staleEnd}}}}}
	if _, err := stale.cacheWindow(context.Background(), store.ProviderGrok, CostQuery{Range: store.CostRangeCurrent}, now); !errors.Is(err, errGrokCycleUnavailable) {
		t.Errorf("stale Grok cycle error = %v", err)
	}
	if _, err := (&liveApp{claudeB: &claudeBridge{}}).cacheWindow(context.Background(), store.ProviderClaude, CostQuery{Range: store.CostRangeCurrent}, now); !errors.Is(err, errClaudeCycleUnavailable) {
		t.Errorf("missing Claude cycle error = %v", err)
	}
}

func TestQueryCacheRejectsInvalidProviderAndWindow(t *testing.T) {
	window := usage.CostRange{Window: usage.Window{Start: time.Now(), End: time.Now().Add(-time.Hour)}}
	rt := &liveApp{}
	if _, err := rt.queryCache(context.Background(), "other", CostQuery{Custom: &window}); !errors.Is(err, errInvalidCacheProvider) {
		t.Fatalf("invalid provider error = %v", err)
	}
	if _, err := rt.cacheWindow(context.Background(), store.ProviderCodex, CostQuery{Custom: &window}, time.Now()); !errors.Is(err, errInvalidCostRange) {
		t.Fatalf("invalid custom window error = %v", err)
	}
	if _, err := rt.cacheWindow(context.Background(), store.ProviderCodex, CostQuery{Range: "other"}, time.Now()); !errors.Is(err, errInvalidCostRange) {
		t.Fatalf("invalid named range error = %v", err)
	}
}

func TestInstallDetailActionsInstallsCacheQuery(t *testing.T) {
	var actions PanelActions
	installDetailActions(&liveApp{}, &actions)
	if actions.QueryCache == nil {
		t.Fatal("cache query action was not installed")
	}
}
