package gemini

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/usage"
)

var sessionT0 = time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC)

// sessionAppConversation has two answered calls: a Gemini one on Nov 10 and
// a Claude one on Nov 11, plus a failed call without token counters.
func sessionAppConversation() sessionTestConversation {
	return sessionTestConversation{
		steps: []sessionTestStep{
			{kind: 14, at: sessionT0},
			{kind: 15, at: sessionT0.Add(5 * time.Second), usage: &sessionTestUsage{input: 1000, cacheRead: 200, thinking: 100, response: 50}},
			{kind: 14, at: sessionT0.Add(23 * time.Hour)},
			{kind: 15, at: sessionT0.Add(23 * time.Hour).Add(4 * time.Second), usage: &sessionTestUsage{input: 2000, response: 300}},
		},
		gens: []sessionTestGen{
			{model: "gemini-3.8-flash", lastStep: 0, usage: &sessionTestUsage{input: 1000, cacheRead: 200, thinking: 100, response: 50}, ttft: 2 * time.Second, stream: 500 * time.Millisecond},
			{model: "claude-sonnet-5", lastStep: 2, claude: true, usage: &sessionTestUsage{input: 2000, response: 300}, ttft: time.Second, stream: time.Second},
			{lastStep: 7},
		},
	}
}

// sessionCLIConversation has no step usage, so its model calls count: an
// unpriced model and a long-context call priced at its tier.
func sessionCLIConversation() sessionTestConversation {
	return sessionTestConversation{
		steps: []sessionTestStep{{kind: 14, at: sessionT0.Add(46 * time.Hour)}, {kind: 15, at: sessionT0.Add(46 * time.Hour)}},
		gens: []sessionTestGen{
			{model: "gemini-9-unknown", lastStep: 0, usage: &sessionTestUsage{input: 500, response: 20}},
			{model: "gemini-3.8-pro-high", lastStep: 5, usage: &sessionTestUsage{input: 250_000, response: 1000}},
		},
		raw: [][]byte{{0xff, 0xff}},
	}
}

func sessionWriteFixture(t *testing.T, h sessionTestHome) {
	t.Helper()
	sessionWriteConversation(t, h.app, "conv-a", sessionAppConversation(), false)
	sessionWriteConversation(t, h.cli, "conv-c", sessionCLIConversation(), false)
	sessionWriteSummaries(t, h.app, sessionTestSummary{id: "conv-a", title: "Alpha", workspace: "/work/alpha", steps: 4, modified: "2026-11-11 09:00:10.5+00:00"})
	sessionWriteSummaries(t, h.cli,
		sessionTestSummary{id: "conv-c", preview: "First line\nsecond", steps: 2, modified: "2026-11-12 08:00:00+00:00"},
		sessionTestSummary{id: "conv-a", title: "Alpha seen by the CLI", modified: "2026-11-11 09:00:10.5+00:00"},
	)
	sessionMust(t, os.WriteFile(filepath.Join(h.cli, "conversations", "old.pb"), []byte{1}, 0o600))
}

func TestSessionScanSummaryDailyAndPricing(t *testing.T) {
	h := sessionNewTestHome(t)
	sessionWriteFixture(t, h)
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	got, err := h.scanner(t).Scan(context.Background(), SessionScanOptions{RecentLimit: 5, SessionLimit: 10, Now: now,
		CostWindow: usage.Window{Start: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), End: now}})
	sessionMust(t, err)
	if got.SessionCount != 2 || got.Unreadable != 1 {
		t.Fatalf("count = %d, unreadable = %d", got.SessionCount, got.Unreadable)
	}
	if got.Totals.TotalTokens != 255_170 || got.Totals.ModelCalls != 4 || got.Totals.ReasoningTokens != 100 || got.Totals.CachedReadTokens != 200 {
		t.Fatalf("totals = %+v", got.Totals)
	}
	if got.Totals.EstimatedUSD == nil || got.Totals.EstimatedUSD.String() != "1.02946" || got.Totals.UnpricedTokens != 520 {
		t.Fatalf("usd = %v, unpriced = %d", got.Totals.EstimatedUSD, got.Totals.UnpricedTokens)
	}
	cost := got.CostSummary
	if cost.Confidence != usage.ConfidencePartiallyPriced || cost.Source != usage.SourceLocalSessions || !slices.Contains(cost.Warnings, "unknown-model:gemini-9-unknown") {
		t.Fatalf("cost = %+v", cost)
	}
	if cost.PricingVersion == "" || cost.Totals.UncachedInputTokens != 253_500 || cost.Totals.CachedInputTokens != 200 || cost.Totals.OutputTokens != 1470 {
		t.Fatalf("cost totals = %+v (%s)", cost.Totals, cost.PricingVersion)
	}
	wantDaily := map[string]int64{"2026-11-10": 1350, "2026-11-11": 2300, "2026-11-12": 251_520}
	if len(got.DailyTokens) != len(wantDaily) {
		t.Fatalf("daily = %v", got.DailyTokens)
	}
	for day, tokens := range wantDaily {
		if got.DailyTokens[day] != tokens {
			t.Fatalf("daily = %v", got.DailyTokens)
		}
	}
	if len(cost.ClientRows) != 2 || cost.ClientRows[0].Name != SessionClientCLI || cost.ClientRows[1].Name != SessionClientApp || cost.ClientRows[1].Totals.TotalTokens != 3650 {
		t.Fatalf("clients = %+v", cost.ClientRows)
	}
	models := map[string]string{}
	for _, row := range cost.ModelRows {
		usd := "<nil>"
		if row.EstimatedUSD != nil {
			usd = row.EstimatedUSD.String()
		}
		models[row.Name] = usd
	}
	if models["gemini-3.8-flash"] != "0.00096" || models["claude-sonnet-5"] != "0.0105" || models["gemini-3.8-pro-high"] != "1.018" || models["gemini-9-unknown"] != "<nil>" {
		t.Fatalf("models = %v", models)
	}
	if len(got.RecentItems) != 2 {
		t.Fatalf("items = %+v", got.RecentItems)
	}
	cli, app := got.RecentItems[0], got.RecentItems[1]
	if cli.ID != "conv-c" || cli.Title != "First line" || cli.ProjectName != sessions.UnknownProject || cli.CWD != nil || cli.Client != SessionClientCLI ||
		cli.Model == nil || *cli.Model != "gemini-3.8-pro-high" || cli.MessageCount != 2 || !cli.UpdatedAt.Equal(time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("cli item = %+v", cli)
	}
	if app.ID != "conv-a" || app.Title != "Alpha" || app.ProjectName != "alpha" || app.CWD == nil || *app.CWD != filepath.FromSlash("/work/alpha") || app.Client != SessionClientApp ||
		app.Model == nil || *app.Model != "claude-sonnet-5" || app.Totals.TotalTokens != 3650 || app.Totals.EstimatedUSD.String() != "0.01146" {
		t.Fatalf("app item = %+v", app)
	}

	window := usage.Window{Start: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}
	day, err := h.scanner(t).ScanCost(context.Background(), window, now)
	sessionMust(t, err)
	if day.Totals.TotalTokens != 2300 || day.EstimatedUSD.String() != "0.0105" || day.Confidence != usage.ConfidencePriced || len(day.DailyRows) != 1 {
		t.Fatalf("window cost = %+v", day)
	}
}

func TestSessionScanMissingHomeIsEmpty(t *testing.T) {
	for _, home := range []string{"", filepath.Join(t.TempDir(), "absent")} {
		got, err := SessionScanner{GeminiHome: home}.Scan(context.Background(), DefaultSessionScanOptions(time.Now()))
		if err != nil || got.SessionCount != 0 || got.CostSummary.Confidence != usage.ConfidenceUnavailable {
			t.Fatalf("scan = %+v, %v", got, err)
		}
	}
}

func TestSessionCostRules(t *testing.T) {
	snapshot := sessionTestSnapshot(t)
	legacy, err := pricing.NewSnapshot([]pricing.Model{{ProviderID: "google", ID: "gemini-old", Rate: pricing.Rate{Input: sessionDec("1"), Output: sessionDec("2")}, LegacyLong: &pricing.Rate{Input: sessionDec("2"), Output: sessionDec("4")}}})
	sessionMust(t, err)
	cases := []struct {
		name     string
		snapshot pricing.Snapshot
		rec      sessionRecord
		want     string // "" when unpriced
	}{
		{"suffix and case", snapshot, sessionRecord{model: "Gemini-3.8-Flash-High", usage: sessionUsage{input: 1_000_000}}, "0.5"},
		{"below tier", snapshot, sessionRecord{model: "gemini-3.8-pro", usage: sessionUsage{input: 100_000, output: 1000}}, "0.212"},
		{"tier boundary", snapshot, sessionRecord{model: "gemini-3.8-pro", usage: sessionUsage{input: 199_000, cacheRead: 1000}}, ""},
		{"claude flag", snapshot, sessionRecord{model: "sonnet-alias", claude: true, usage: sessionUsage{input: 10}}, ""},
		{"claude model", snapshot, sessionRecord{model: "claude-sonnet-5", usage: sessionUsage{input: 1_000_000, cacheRead: 1_000_000}}, "3.3"},
		{"claude cache write", snapshot, sessionRecord{model: "claude-sonnet-5", usage: sessionUsage{input: 10, cacheWrite: 10}}, ""},
		{"google cache write without rate", snapshot, sessionRecord{model: "gemini-3.8-flash", usage: sessionUsage{input: 10, cacheWrite: 10}}, ""},
		{"legacy premium without boundary", legacy, sessionRecord{model: "gemini-old", usage: sessionUsage{input: 10}}, ""},
		{"empty snapshot", pricing.Snapshot{}, sessionRecord{model: "gemini-3.8-flash", usage: sessionUsage{input: 10}}, ""},
	}
	for _, c := range cases {
		cost, ok, reason := sessionCost(c.snapshot, c.rec)
		if c.want == "" {
			if ok || reason == "" {
				t.Fatalf("%s: priced %v", c.name, cost)
			}
			continue
		}
		if !ok || cost.String() != c.want {
			t.Fatalf("%s: cost = %v, %v, %s", c.name, cost, ok, reason)
		}
	}
	if sessionPriceProvider(sessionRecord{model: "gemini-3.8-flash", claude: true}) != "google" {
		t.Fatal("a Gemini model is priced as Google even when the call reports Claude")
	}
}

func TestSessionScanReadsWALWithoutTouchingLiveFiles(t *testing.T) {
	h := sessionNewTestHome(t)
	path := sessionWriteConversation(t, h.app, "live", sessionAppConversation(), true)
	before, err := os.Stat(path + "-wal")
	sessionMust(t, err)
	if before.Size() == 0 {
		t.Fatal("fixture rows are not in the WAL")
	}
	got, err := h.scanner(t).Scan(context.Background(), SessionScanOptions{RecentLimit: 1, Now: sessionT0.Add(72 * time.Hour)})
	sessionMust(t, err)
	if got.SessionCount != 1 || got.Totals.TotalTokens != 3650 || got.RecentItems[0].Title != sessionUntitled {
		t.Fatalf("scan = %+v", got)
	}
	after, err := os.Stat(path + "-wal")
	sessionMust(t, err)
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("scan changed the live WAL")
	}
}

func TestSessionCacheReusesUnchangedDatabase(t *testing.T) {
	h := sessionNewTestHome(t)
	path := sessionWriteConversation(t, h.app, "conv-a", sessionAppConversation(), false)
	scanner := h.scanner(t)
	scanner.Cache = &SessionCache{}
	opts := SessionScanOptions{RecentLimit: 1, Now: sessionT0.Add(72 * time.Hour)}
	first, err := scanner.Scan(context.Background(), opts)
	sessionMust(t, err)
	info, err := os.Stat(path)
	sessionMust(t, err)
	garbage := make([]byte, info.Size())
	sessionMust(t, os.WriteFile(path, garbage, 0o600))
	sessionMust(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
	cached, err := scanner.Scan(context.Background(), opts)
	sessionMust(t, err)
	if cached.Totals.TotalTokens != first.Totals.TotalTokens || cached.Unreadable != 0 {
		t.Fatalf("unchanged database was read again: %+v", cached.Totals)
	}
	sessionMust(t, os.Remove(path))
	conv := sessionAppConversation()
	conv.steps = conv.steps[:2]
	sessionWriteConversation(t, h.app, "conv-a", conv, false)
	changed, err := scanner.Scan(context.Background(), opts)
	sessionMust(t, err)
	if changed.Totals.TotalTokens != 1350 {
		t.Fatalf("changed database not re-read: %+v", changed.Totals)
	}
	sessionMust(t, os.Remove(path))
	if _, err := scanner.Scan(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if len(scanner.Cache.files) != 0 {
		t.Fatal("cache kept a database no scan listed")
	}
}

func TestSessionScanSpeed(t *testing.T) {
	h := sessionNewTestHome(t)
	sessionWriteConversation(t, h.app, "conv-a", sessionAppConversation(), false)
	sessionWriteSummaries(t, h.app, sessionTestSummary{id: "conv-a", title: "Alpha", modified: "2026-11-11 09:00:10+00:00"})
	got, err := h.scanner(t).ScanSpeed(context.Background(), sessionT0.Add(48*time.Hour))
	sessionMust(t, err)
	if len(got) != 1 || len(got[0].Samples) != 2 {
		t.Fatalf("speed = %+v", got)
	}
	first := got[0].Samples[0]
	if first.Model != "gemini-3.8-flash" || !first.HasTTFT || first.TTFT != 2*time.Second || first.OutputTokens != 50 || first.Decode != 500*time.Millisecond || !first.At.Equal(sessionT0.Add(5*time.Second)) {
		t.Fatalf("sample = %+v", first)
	}
	late, err := h.scanner(t).ScanSpeed(context.Background(), sessionT0.Add(30*24*time.Hour))
	sessionMust(t, err)
	if len(late) != 0 {
		t.Fatalf("old conversation in speed window: %+v", late)
	}
}

func TestSessionWorkspaceDrivePath(t *testing.T) {
	want := "/C:/Users/a b"
	if runtime.GOOS == "windows" {
		want = `C:\Users\a b`
	}
	if got := sessionWorkspace(`["file:///C:/Users/a%20b"]`); got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}
