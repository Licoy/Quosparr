package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestApplyGeminiLocalMapsScanToState(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	usd := decimal.New(125, -2)
	model := "gemini-3-pro"
	local := &gemini.SessionUsageSummary{
		DailyTokens:  map[string]int64{"2026-10-20": 900},
		CostSummary:  usage.Summary{UnpricedTokens: 7},
		CalculatedAt: now,
		RecentItems: []gemini.SessionItem{{
			ID: "c1", Title: "Fix build", ProjectName: "app", Model: &model, Client: gemini.SessionClientCLI, UpdatedAt: now,
			Totals: gemini.SessionUsageTotals{TotalTokens: 900, InputTokens: 700, CachedReadTokens: 200, OutputTokens: 200, ModelCalls: 3, EstimatedUSD: &usd, UnpricedTokens: 5},
		}},
	}
	var st runway.GeminiState
	applyGeminiLocal(&st, local)
	if st.Cost == nil || st.Cost.UnpricedTokens != 7 || st.Heatmap.Local["2026-10-20"] != 900 || !st.Heatmap.CalculatedAt.Equal(now) {
		t.Fatalf("cost/heatmap = %+v %+v", st.Cost, st.Heatmap)
	}
	if len(st.RecentSessions) != 1 {
		t.Fatalf("recent = %+v", st.RecentSessions)
	}
	row := st.RecentSessions[0]
	want := sessions.Totals{TotalTokens: 900, UncachedInputTokens: 500, CachedInputTokens: 200, OutputTokens: 200, Turns: 3, Threads: 1}
	if row.ID != "c1" || row.Title != "Fix build" || row.Model == nil || *row.Model != model || row.Totals != want || row.UnpricedTokens != 5 || row.State != sessions.StateRecent {
		t.Fatalf("row = %+v", row)
	}
	if row.EstimatedUSD == nil || row.EstimatedUSD.String() != "1.25" {
		t.Fatalf("usd = %v", row.EstimatedUSD)
	}
	empty := runway.GeminiState{}
	applyGeminiLocal(&empty, nil)
	if empty.Cost != nil || empty.RecentSessions != nil {
		t.Fatalf("nil scan changed state: %+v", empty)
	}
}

func TestGeminiLocalErrorAndRangeChange(t *testing.T) {
	gmod := newGeminiModule(t.TempDir(), "test")
	g := newGeminiBridge(gmod, t.TempDir(), store.DefaultPreferences(), nil)
	g.local = &gemini.SessionUsageSummary{CostSummary: usage.Summary{UnpricedTokens: 7}}
	g.localRange = g.prefs.APICostSummaryRange
	g.localErr = errors.New("damaged")
	var st runway.GeminiState
	g.applyLocalLocked(&st)
	if st.Cost == nil || st.LocalError == "" || st.LastError != "" {
		t.Fatalf("state = %+v", st)
	}
	g.localRange = store.CostRangeToday
	if g.prefs.APICostSummaryRange == store.CostRangeToday {
		g.localRange = store.CostRangeThisMonth
	}
	st = runway.GeminiState{}
	g.applyLocalLocked(&st)
	if st.Cost != nil {
		t.Fatal("a cost of another range was shown")
	}
}

func TestGeminiCostWindowFollowsWeeklyReset(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.Local)
	week := 7 * 24 * time.Hour
	reset := now.Add(48 * time.Hour)
	past := now.Add(-time.Hour)
	snap := &gemini.QuotaSnapshot{Buckets: []gemini.QuotaBucket{
		{Family: gemini.FamilyClaude, Window: gemini.WindowWeekly, ResetAt: &past},
		{Family: gemini.FamilyGemini, Window: gemini.WindowWeekly, ResetAt: &reset},
	}}
	cur := geminiCostWindow(store.CostRangeCurrent, snap, now)
	if !cur.Start.Equal(reset.Add(-week)) || !cur.End.Equal(now) {
		t.Fatalf("current = %+v", cur)
	}
	prev := geminiCostWindow(store.CostRangePrevious, snap, now)
	if !prev.End.Equal(cur.Start) || !prev.Start.Equal(cur.Start.Add(-week)) {
		t.Fatalf("previous = %+v", prev)
	}
	stale := &gemini.QuotaSnapshot{Buckets: []gemini.QuotaBucket{{Family: gemini.FamilyGemini, Window: gemini.WindowWeekly, ResetAt: &past}}}
	for _, detail := range []any{nil, stale} {
		if w := geminiCostWindow(store.CostRangeCurrent, detail, now); !w.Start.Equal(now.Add(-week)) || !w.End.Equal(now) {
			t.Fatalf("fallback %v = %+v", detail, w)
		}
	}
	if w := geminiCostWindow(store.CostRangeToday, snap, now); w != usage.Today(now, time.Local).Window {
		t.Fatalf("today = %+v", w)
	}
	if w := geminiCostWindow(store.CostRangeThisMonth, snap, now); w != usage.ThisMonth(now, time.Local).Window {
		t.Fatalf("month = %+v", w)
	}
}

func TestGeminiPriceChangesInvalidateLocalScan(t *testing.T) {
	for _, provider := range []string{"google", "anthropic"} {
		gmod := newGeminiModule(t.TempDir(), "test")
		g := newGeminiBridge(gmod, t.TempDir(), store.DefaultPreferences(), nil)
		g.local = &gemini.SessionUsageSummary{}
		g.state.Cost = &usage.Summary{}
		g.state.RecentSessions = []sessions.Item{{ID: "c1"}}
		rt := &liveApp{geminiB: g}
		rt.pricesChanged(provider)
		if g.pricingGeneration != 1 || g.local != nil || g.state.Cost != nil || g.state.RecentSessions != nil {
			t.Fatalf("%s: generation %d local %v state %+v", provider, g.pricingGeneration, g.local, g.state)
		}
	}
	rt := &liveApp{geminiB: newGeminiBridge(newGeminiModule(t.TempDir(), "test"), t.TempDir(), store.DefaultPreferences(), nil)}
	rt.pricesChanged("xai")
	if rt.geminiB.pricingGeneration != 0 {
		t.Fatal("an xAI price change rescanned Gemini")
	}
	if !slices.Contains(pricingWatched, "google") || pricingProviderID(store.ProviderGemini) != "google" {
		t.Fatalf("watched = %v", pricingWatched)
	}
}

// geminiDeleteFixture writes one App conversation and lists it the way the
// sessions page does; nothing lists it in a summaries database.
func geminiDeleteFixture(t *testing.T, rt *liveApp) sessions.ManagedSession {
	t.Helper()
	scanner, ok := rt.geminiB.sessionScanner()
	if !ok {
		t.Fatal("no session scanner")
	}
	root := filepath.Join(scanner.GeminiHome, "antigravity")
	path := filepath.Join(root, "conversations", "c1.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	listed, err := rt.queryManagedSessions(context.Background(), store.ProviderGemini)
	if err != nil || len(listed) != 1 || listed[0].Path != path {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	return listed[0]
}

func TestGeminiDeletionRefusedWhileClientRunning(t *testing.T) {
	oldTrash, oldRunning := sessionTrash, geminiRunningClients
	t.Cleanup(func() { sessionTrash, geminiRunningClients = oldTrash, oldRunning })
	var trashed []string
	sessionTrash = func(path string) error {
		trashed = append(trashed, path)
		return os.Remove(path)
	}
	cases := []struct {
		name    string
		running map[string]bool
		err     error
		refused bool
	}{
		{"app running", map[string]bool{gemini.SessionClientApp: true}, nil, true},
		{"check failed", nil, errors.New("ps failed"), true},
		{"only cli running", map[string]bool{gemini.SessionClientCLI: true}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trashed = nil
			calls := 0
			geminiRunningClients = func(context.Context) (map[string]bool, error) {
				calls++
				return tc.running, tc.err
			}
			rt, _ := sessionTestRuntime(t)
			item := geminiDeleteFixture(t, rt)
			results, err := rt.deleteManagedSessions(context.Background(), []sessions.ManagedSession{item})
			_, statErr := os.Stat(item.Path)
			if tc.refused {
				if !errors.Is(err, gemini.ErrSessionClientRunning) || len(trashed) != 0 || statErr != nil || len(results) != 1 || results[0].Removed != 0 {
					t.Fatalf("err %v trashed %v stat %v results %+v", err, trashed, statErr, results)
				}
				l := l10n.ForPreference("en", nil)
				if got, failed := sessionDeleteResultText(l, results, err); !failed || got != tx(l, xSessionsGeminiRunning) {
					t.Fatalf("refusal text = %q", got)
				}
			} else if err != nil || len(trashed) != 1 || !os.IsNotExist(statErr) {
				t.Fatalf("err %v trashed %v stat %v", err, trashed, statErr)
			}
			if calls != 1 {
				t.Fatalf("process check ran %d times", calls)
			}
		})
	}
}

func TestGeminiMCPAndSkillsThroughRuntime(t *testing.T) {
	rt, _ := sessionTestRuntime(t)
	ctx := context.Background()
	geminiHome := rt.geminiMod.Store.GeminiHome()
	config := filepath.Join(geminiHome, "config", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"mcpServers":{"alpha":{"command":"tool"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := rt.queryMCP(ctx, store.ProviderGemini)
	if err != nil || len(snap.Servers) != 1 || !snap.Servers[0].Enabled {
		t.Fatalf("mcp = %+v, %v", snap, err)
	}
	if err := rt.setMCPEnabled(ctx, store.ProviderGemini, snap.Servers[0], false); err != nil {
		t.Fatal(err)
	}
	if snap, err = rt.queryMCP(ctx, store.ProviderGemini); err != nil || snap.Servers[0].Enabled {
		t.Fatalf("after toggle = %+v, %v", snap, err)
	}

	skill := filepath.Join(geminiHome, "config", "skills", "review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("---\nname: review\ndescription: Review code\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	all, err := rt.querySkills(ctx)
	if err != nil {
		t.Fatal(err)
	}
	gm := all[store.ProviderGemini]
	if len(all) != len(store.Providers()) || len(gm.Skills) != 1 || gm.Skills[0].Toggleable || gm.Skills[0].ToggleBlock != skills.ReasonNoToggle {
		t.Fatalf("gemini skills = %+v", gm)
	}
	if err := rt.setSkillEnabled(gm.Skills[0], false); !errors.Is(err, errSkillToggleUnsupported) {
		t.Fatalf("toggle = %v", err)
	}
	if !slices.Contains(skills.InstallTargets(), skills.TargetGemini) || skillsTrashTab != len(store.Providers()) {
		t.Fatalf("targets %v trash tab %d", skills.InstallTargets(), skillsTrashTab)
	}
}
