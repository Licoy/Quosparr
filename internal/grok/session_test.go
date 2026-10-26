package grok

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func sessionWrite(t *testing.T, home, id, cwd, model, updatedAt, updates string) {
	t.Helper()
	dir := filepath.Join(home, "sessions", "%2Ftmp", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	summary := fmt.Sprintf(`{
  "info": {"id": %q, "cwd": %q},
  "generated_title": %q,
  "updated_at": %q,
  "last_active_at": %q,
  "num_chat_messages": 1,
  "current_model_id": %q
}`, id, cwd, id, updatedAt, updatedAt, model)
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "updates.jsonl"), []byte(updates), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sessionDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func sessionDecEq(got *decimal.Decimal, want decimal.Decimal) bool {
	return got != nil && got.Equal(want)
}

func TestSessionCostTicksToUSD(t *testing.T) {
	got, ok := SessionCostUSDFromTicks(2_486_936_000)
	if !ok || !got.Equal(sessionDecimal(t, "2.486936")) {
		t.Fatalf("ticks = %v %v", got, ok)
	}
	if _, ok := SessionCostUSDFromTicks(0); ok {
		t.Fatal("zero ticks must have no cost")
	}
}

func TestSessionScansSummaryAndUsage(t *testing.T) {
	root := t.TempDir()
	dir1 := filepath.Join(root, "sessions", "%2Ftmp", "session-1")
	dir2 := filepath.Join(root, "sessions", "%2Ftmp", "session-2")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir1, "summary.json"), `{
  "info": {"id": "session-1", "cwd": "/tmp/demo-project"},
  "generated_title": "Demo session",
  "updated_at": "2026-08-01T00:00:00Z",
  "last_active_at": "2026-08-01T01:00:00Z",
  "num_chat_messages": 4,
  "current_model_id": "grok-4.5"
}`)
	write(filepath.Join(dir1, "updates.jsonl"), `{"timestamp":1785541389,"params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hi"}}}}
{"timestamp":1785541392,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":1000,"outputTokens":200,"totalTokens":1200,"cachedReadTokens":400,"reasoningTokens":50,"modelCalls":2,"costUsdTicks":1500000000,"modelUsage":{"grok-4.5-build":{"inputTokens":1000,"outputTokens":200,"totalTokens":1200,"cachedReadTokens":400,"reasoningTokens":50,"modelCalls":2,"costUsdTicks":1500000000}}}}}}
{"timestamp":1785541492,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":500,"outputTokens":100,"totalTokens":600,"cachedReadTokens":100,"reasoningTokens":10,"modelCalls":1,"costUsdTicks":500000000,"modelUsage":{"grok-4.5-build":{"inputTokens":500,"outputTokens":100,"totalTokens":600,"cachedReadTokens":100,"reasoningTokens":10,"modelCalls":1,"costUsdTicks":500000000}}}}}}`)
	write(filepath.Join(dir2, "summary.json"), `{
  "info": {"id": "session-2", "cwd": "/tmp/other"},
  "session_summary": "Older",
  "updated_at": "2026-07-01T00:00:00Z",
  "num_chat_messages": 1,
  "current_model_id": "grok-4.5"
}`)
	for path, sec := range map[string]int64{filepath.Join(dir1, "summary.json"): 1_800_000_100, filepath.Join(dir2, "summary.json"): 1_800_000_000} {
		if err := os.Chtimes(path, time.Unix(sec, 0), time.Unix(sec, 0)); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Unix(1_800_000_000, 0)
	res, err := SessionScanner{GrokHome: root, Location: time.UTC}.Scan(context.Background(), SessionScanOptions{
		RecentLimit: 5, SessionLimit: 10, Now: now,
		CostWindow: usage.Window{Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), End: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := sessionDecimal(t, "0.00395")
	if len(res.RecentItems) != 2 || res.RecentItems[0].ID != "session-1" {
		t.Fatalf("recent = %+v", res.RecentItems)
	}
	first := res.RecentItems[0]
	if first.Title != "Demo session" || first.ProjectName != "demo-project" || first.Totals.TotalTokens != 1800 || first.Totals.Turns != 2 {
		t.Errorf("first item = %+v", first)
	}
	// The CLI's own costUsdTicks ($2) must not leak in as the API-equivalent cost.
	if !sessionDecEq(first.Totals.EstimatedUSD, want) {
		t.Errorf("item cost = %v", first.Totals.EstimatedUSD)
	}
	if res.Totals.TotalTokens != 1800 || len(res.ModelRows) == 0 || res.ModelRows[0].Model != "grok-4.5-build" ||
		res.ModelRows[0].Totals.TotalTokens != 1800 || !sessionDecEq(res.ModelRows[0].Totals.EstimatedUSD, want) {
		t.Errorf("totals = %+v models = %+v", res.Totals, res.ModelRows)
	}
	c := res.CostSummary
	if c.Confidence != usage.ConfidencePriced || !sessionDecEq(c.EstimatedUSD, want) || c.PricingVersion != PricingVersion || len(c.Warnings) != 0 {
		t.Errorf("cost summary = %+v", c)
	}
	var days int64
	for _, v := range res.DailyTokens {
		days += v
	}
	if len(res.DailyTokens) == 0 || days != 1800 {
		t.Errorf("daily tokens = %v", res.DailyTokens)
	}
}

func TestSessionKeepsPricedTotalsWithUnknownModels(t *testing.T) {
	root := t.TempDir()
	sessionWrite(t, root, "mixed", "/tmp/mixed", "grok-4.5", "2026-08-01T00:00:00Z",
		`{"timestamp":1785541392,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":2000,"outputTokens":100,"totalTokens":2100,"cachedReadTokens":0,"modelUsage":{"grok-4.5-build":{"inputTokens":1000,"outputTokens":50,"totalTokens":1050,"cachedReadTokens":0},"mystery-model":{"inputTokens":1000,"outputTokens":50,"totalTokens":1050,"cachedReadTokens":0}}}}}}`)
	now := time.Unix(1_800_000_000, 0)
	res, err := SessionScanner{GrokHome: root, Location: time.UTC}.Scan(context.Background(), SessionScanOptions{
		RecentLimit: 1, SessionLimit: 5, Now: now, CostWindow: usage.Window{Start: time.Unix(1_700_000_000, 0), End: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	priced, ok := PricingCost("grok-4.5-build", 1000, 0, 50)
	if !ok {
		t.Fatal("grok-4.5-build must be priced")
	}
	c := res.CostSummary
	if !sessionDecEq(c.EstimatedUSD, priced) || c.Confidence != usage.ConfidencePartiallyPriced || c.UnpricedTokens != 1050 {
		t.Errorf("cost = %v %s", c.EstimatedUSD, c.Confidence)
	}
	if len(c.ModelRows) != 2 || c.ModelRows[0].Name != "grok-4.5-build" || c.ModelRows[1].Name != "mystery-model" {
		t.Fatalf("model rows = %+v", c.ModelRows)
	}
	if !sessionDecEq(c.ModelRows[0].EstimatedUSD, priced) || c.ModelRows[1].EstimatedUSD != nil {
		t.Errorf("row costs = %v %v", c.ModelRows[0].EstimatedUSD, c.ModelRows[1].EstimatedUSD)
	}
	if len(c.Warnings) != 1 || c.Warnings[0] != "unknown-model:mystery-model" {
		t.Errorf("warnings = %v", c.Warnings)
	}
}

func TestSessionAttributesModelsByDayInPartialWindow(t *testing.T) {
	for _, tz := range []string{"UTC", "Asia/Tokyo", "America/Los_Angeles"} {
		t.Run(tz, func(t *testing.T) {
			loc, err := time.LoadLocation(tz)
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			day1 := time.Date(2026, 8, 1, 0, 0, 0, 0, loc)
			day2 := time.Date(2026, 8, 2, 0, 0, 0, 0, loc)
			day1Afternoon := time.Date(2026, 8, 1, 15, 0, 0, 0, loc)
			day3 := time.Date(2026, 8, 3, 15, 0, 0, 0, loc)
			sessionWrite(t, root, "span", "/tmp/span", "grok-4.5", "2026-08-03T00:00:00Z", fmt.Sprintf(
				`{"timestamp":%d,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":1000,"outputTokens":0,"totalTokens":1000,"cachedReadTokens":0,"modelUsage":{"grok-4.5-build":{"inputTokens":1000,"outputTokens":0,"totalTokens":1000,"cachedReadTokens":0}}}}}}
{"timestamp":%d,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":2000,"outputTokens":0,"totalTokens":2000,"cachedReadTokens":0,"modelUsage":{"grok-4.6-build":{"inputTokens":2000,"outputTokens":0,"totalTokens":2000,"cachedReadTokens":0}}}}}}`,
				day1Afternoon.Unix(), day3.Unix()))
			res, err := SessionScanner{GrokHome: root, Location: loc}.Scan(context.Background(), SessionScanOptions{
				SessionLimit: 5, Now: day2, CostWindow: usage.Window{Start: day1, End: day2},
			})
			if err != nil {
				t.Fatal(err)
			}
			c := res.CostSummary
			if len(c.ModelRows) != 1 || c.ModelRows[0].Name != "grok-4.5-build" {
				t.Errorf("model rows = %+v", c.ModelRows)
			}
			if !sessionDecEq(c.EstimatedUSD, sessionDecimal(t, "0.002")) || res.Totals.TotalTokens != 1000 {
				t.Errorf("cost = %v tokens = %d", c.EstimatedUSD, res.Totals.TotalTokens)
			}
		})
	}
}

func TestSessionPricesLongContextTurns(t *testing.T) {
	root := t.TempDir()
	sessionWrite(t, root, "long", "/tmp/long", "grok-4.5", "2026-08-01T00:00:00Z",
		`{"timestamp":1785541392,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":200000,"outputTokens":0,"totalTokens":200000,"cachedReadTokens":0,"modelUsage":{"grok-4.5":{"inputTokens":200000,"outputTokens":0,"totalTokens":200000,"cachedReadTokens":0}}}}}}`)
	now := time.Unix(1_800_000_000, 0)
	res, err := SessionScanner{GrokHome: root, Location: time.UTC}.ScanCost(context.Background(),
		usage.Window{Start: time.Unix(1_700_000_000, 0), End: now}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !sessionDecEq(res.EstimatedUSD, sessionDecimal(t, "0.8")) {
		t.Errorf("cost = %v", res.EstimatedUSD)
	}
}

func TestSessionMissingHomeAndCancel(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	res, err := SessionScanner{GrokHome: filepath.Join(t.TempDir(), "absent"), Location: time.UTC}.Scan(context.Background(), DefaultSessionScanOptions(now))
	if err != nil || res.SessionCount != 0 || res.SourceLabel != "local-sessions" || res.CostSummary.Source != usage.SourceUnavailable {
		t.Fatalf("missing home = %+v %v", res, err)
	}
	root := t.TempDir()
	sessionWrite(t, root, "s", "/tmp/x", "grok-4.5", "2026-08-01T00:00:00Z", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (SessionScanner{GrokHome: root, Location: time.UTC}).Scan(ctx, DefaultSessionScanOptions(now)); err == nil {
		t.Fatal("a cancelled context must fail the scan")
	}
}

// The project name of a Windows working directory is its last component (the
// macOS app keeps the whole string; the port reads such paths natively).
func TestSessionWindowsWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	sessionWrite(t, root, "w", `C:\\Users\\me\\proj`, "grok-4.5", "2026-08-01T00:00:00Z", "")
	res, err := SessionScanner{GrokHome: root, Location: time.UTC}.Scan(context.Background(), SessionScanOptions{RecentLimit: 1, SessionLimit: 5, Now: time.Unix(1_800_000_000, 0)})
	if err != nil || len(res.RecentItems) != 1 || res.RecentItems[0].ProjectName != "proj" {
		t.Fatalf("items = %+v %v", res.RecentItems, err)
	}
}

func TestSessionCalendarHybrid(t *testing.T) {
	for _, tc := range []struct {
		secs float64
		want string
	}{
		{-62135596800, "0001-01-03"}, {-12219292800, "1582-10-15"}, {-12219292801, "1582-10-04"},
		{253402300800, "10000-01-01"}, {1e15, "5828963-12-20"}, {-1e15, "5838270-09-20"},
	} {
		if got := sessionDayString(tc.secs, time.UTC); got != tc.want {
			t.Errorf("day(%v) = %s, want %s", tc.secs, got, tc.want)
		}
	}
	// Keys read back as dates of the common era in the same calendar.
	d, ok := sessionDayDate("1582-10-04", time.UTC)
	if !ok || d.Unix() != -12219292800-86400 {
		t.Errorf("1582-10-04 = %v %v", d, ok)
	}
}

// Symlinked directories and summary files are not followed (as the macOS app).
func TestSessionIgnoresSymlinks(t *testing.T) {
	root := t.TempDir()
	sessionWrite(t, root, "real", "/x/real", "grok-4.5", "2026-08-01T00:00:00Z", "")
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "linked"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "linked", "summary.json")
	if err := os.WriteFile(target, []byte(`{"info":{"id":"linked"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "linked"), filepath.Join(root, "sessions", "dirlink")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sessions", "fl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "sessions", "fl", "summary.json")); err != nil {
		t.Fatal(err)
	}
	res, err := SessionScanner{GrokHome: root, Location: time.UTC}.Scan(context.Background(), SessionScanOptions{RecentLimit: 10, SessionLimit: 10, Now: time.Unix(1_800_000_000, 0)})
	if err != nil || res.SessionCount != 1 || res.RecentItems[0].ID != "real" {
		t.Fatalf("scan = %+v %v", res.RecentItems, err)
	}
}
