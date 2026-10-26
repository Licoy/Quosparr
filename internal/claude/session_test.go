package claude

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/usage"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestScanSessions(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	proj := filepath.Join(config, "projects", "demo")
	mustMkdir(t, proj)
	mustMkdir(t, filepath.Join(proj, "sess-a", "subagents"))
	mustMkdir(t, filepath.Join(config, "projects", "file-history"))
	mustMkdir(t, filepath.Join(config, "projects", "sessions"))
	writeJSONL(t, filepath.Join(proj, "sess-a.jsonl"),
		`{"type":"last-prompt","lastPrompt":"secret prompt do not keep","sessionId":"sess-a","timestamp":"2026-10-14T20:00:00Z"}`,
		`{"type":"ai-title","aiTitle":"First","sessionId":"sess-a","timestamp":"2026-10-14T20:00:01Z"}`,
		`{"type":"ai-title","aiTitle":"Named","sessionId":"sess-a","timestamp":"2026-10-14T20:00:02Z","cwd":"/work/demo"}`,
		`{"type":"user","sessionId":"sess-a","timestamp":"2026-10-14T20:00:03Z"}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:04Z","message":{"id":"msg_a","model":"claude-haiku-4-5","stop_reason":"end_turn","usage":{"output_tokens":3}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:05Z","message":{"id":"msg_a","model":"claude-haiku-4-5","stop_reason":"end_turn","usage":{"output_tokens":10}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:06Z","requestId":"r1","message":{"id":"msg_b","model":"claude-haiku-4-5","stop_reason":"tool_use","usage":{"output_tokens":4}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:07Z","requestId":"r2","message":{"id":"msg_b","model":"claude-haiku-4-5","stop_reason":"tool_use","usage":{"output_tokens":6}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:08Z","message":{"id":"msg_d","model":"claude-haiku-4-5","content":[{"type":"tool_use","id":"t1"}],"usage":{"output_tokens":5,"cache_read_input_tokens":1}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:09Z","message":{"id":"msg_d","model":"claude-haiku-4-5","usage":{"output_tokens":5,"cache_read_input_tokens":9}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:10Z","message":{"id":"msg_syn","model":"<synthetic>","usage":{"output_tokens":999}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:11Z","message":{"id":"msg_iter","model":"claude-haiku-4-5","usage":{"output_tokens":1,"iterations":[{"output_tokens":100}]}}}`,
		`{`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-a", "subagents", "agent-1.jsonl"),
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:12Z","message":{"id":"msg_a","model":"claude-haiku-4-5","usage":{"output_tokens":1}}}`,
		`{"type":"assistant","sessionId":"sess-a","timestamp":"2026-10-14T20:00:13Z","message":{"id":"msg_c","model":"claude-haiku-4-5","usage":{"output_tokens":2}}}`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-b.jsonl"),
		`{"type":"assistant","sessionId":"sess-b","timestamp":"2026-10-14T20:00:00Z","cwd":"/work/demo","message":{"id":"msg_split","model":"claude-haiku-4-5","stop_reason":"end_turn","usage":{"input_tokens":1,"cache_creation_input_tokens":99,"cache_read_input_tokens":4,"output_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":3}}}}`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-c.jsonl"),
		`{"type":"assistant","sessionId":"sess-c","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_nosplit","model":"claude-haiku-4-5","usage":{"cache_creation_input_tokens":7,"output_tokens":8}}}`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-d.jsonl"),
		`{"type":"assistant","sessionId":"sess-d","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_fast","model":"claude-opus-5-5","usage":{"output_tokens":1000000,"speed":"fast"}}}`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-e.jsonl"),
		`{"type":"assistant","sessionId":"sess-e","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_unknown","model":"claude-not-a-model","usage":{"output_tokens":10}}}`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-f.jsonl"),
		`{"type":"assistant","sessionId":"sess-f","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_bedrock","model":"us.anthropic.claude-haiku-4-5-20251001-v1:0","usage":{"input_tokens":1000000}}}`,
	)
	writeJSONL(t, filepath.Join(proj, "sess-g.jsonl"),
		`{"type":"assistant","sessionId":"sess-g","timestamp":"2026-10-15T06:30:00Z","message":{"id":"msg_day","model":"claude-haiku-4-5","usage":{"output_tokens":1}}}`,
	)
	writeJSONL(t, filepath.Join(config, "projects", "history.jsonl"),
		`{"type":"assistant","sessionId":"hist","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_hist","model":"claude-haiku-4-5","usage":{"output_tokens":7777}}}`,
	)
	writeJSONL(t, filepath.Join(config, "projects", "file-history", "x.jsonl"),
		`{"type":"assistant","sessionId":"fh","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_fh","model":"claude-haiku-4-5","usage":{"output_tokens":8888}}}`,
	)
	writeJSONL(t, filepath.Join(config, "projects", "sessions", "x.jsonl"),
		`{"type":"assistant","sessionId":"skip","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_skip","model":"claude-haiku-4-5","usage":{"output_tokens":6666}}}`,
	)
	compat := filepath.Join(home, ".config", "claude", "projects", "extra")
	mustMkdir(t, compat)
	writeJSONL(t, filepath.Join(compat, "extra.jsonl"),
		`{"type":"assistant","sessionId":"sess-x","timestamp":"2026-10-14T20:00:00Z","message":{"id":"msg_extra","model":"claude-haiku-4-5","usage":{"output_tokens":3}}}`,
	)

	loc := time.FixedZone("UTC-7", -7*3600)
	sc := SessionScanner{ConfigDir: config, Home: home, Location: loc}
	res, err := sc.Scan(context.Background(), SessionScanOptions{RecentLimit: 20, SessionLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.BadLines != 1 {
		t.Fatalf("bad lines %d", res.BadLines)
	}
	a := itemByID(t, res, "sess-a")
	if a.Title != "Named" || a.Title == "secret prompt do not keep" || a.ProjectName != "demo" {
		t.Fatalf("title %q project %q", a.Title, a.ProjectName)
	}
	if a.Totals.Output != 28 || a.Totals.CacheRead != 9 || a.Totals.Turns != 3 {
		t.Fatalf("sess-a tokens out %d cache %d turns %d", a.Totals.Output, a.Totals.CacheRead, a.Totals.Turns)
	}
	b := itemByID(t, res, "sess-b")
	api := b.Totals.APITotals()
	if b.Totals.CacheWrite5m != 2 || b.Totals.CacheWrite1h != 3 || api.UncachedInputTokens != 6 || api.CachedInputTokens != 4 || api.OutputTokens != 5 {
		t.Fatalf("split %+v api %+v", b.Totals, api)
	}
	c := itemByID(t, res, "sess-c")
	if c.Totals.CacheWrite5m != 7 || c.Totals.CacheWrite1h != 0 {
		t.Fatalf("unsplit %+v", c.Totals)
	}
	d := itemByID(t, res, "sess-d")
	if d.Totals.EstimatedUSD == nil || !d.Totals.EstimatedUSD.Equal(decimal.MustParse("40")) {
		t.Fatalf("fast cost %v", d.Totals.EstimatedUSD)
	}
	e := itemByID(t, res, "sess-e")
	if e.Totals.EstimatedUSD != nil || e.Totals.Output != 10 {
		t.Fatalf("unknown %+v", e.Totals)
	}
	f := itemByID(t, res, "sess-f")
	if f.Model == nil || *f.Model != "claude-haiku-4-5" || f.Totals.EstimatedUSD == nil || !f.Totals.EstimatedUSD.Equal(decimal.MustParse("1")) {
		t.Fatalf("bedrock model %v cost %v", f.Model, f.Totals.EstimatedUSD)
	}
	if _, ok := res.DailyTokens["2026-10-15"]; ok {
		t.Fatal("used UTC day")
	}
	if res.DailyTokens["2026-10-14"] == 0 {
		t.Fatal("missing local day")
	}
	if res.Totals.Output == 0 || containsOutput(res, 7777) || containsOutput(res, 8888) || containsOutput(res, 6666) {
		t.Fatal("skipped logs were counted")
	}
	if itemByID(t, res, "sess-x").Totals.Output != 3 {
		t.Fatal("compat projects dir")
	}
	if res.CostSummary.PricingVersion != PricingVersion || res.CostSummary.Confidence != usage.ConfidencePartiallyPriced {
		t.Fatalf("summary %s %s", res.CostSummary.Confidence, res.CostSummary.PricingVersion)
	}
	foundUnknown := false
	for _, w := range res.CostSummary.Warnings {
		if w == "unknown-model:claude-not-a-model" {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatal(res.CostSummary.Warnings)
	}
}

func TestScanWindowsCostButNotHeatmapOrRecent(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	proj := filepath.Join(config, "projects", "demo")
	mustMkdir(t, proj)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	old := filepath.Join(proj, "old.jsonl")
	writeJSONL(t, old,
		`{"type":"custom-title","customTitle":"Renamed","sessionId":"old"}`,
		`{"type":"ai-title","aiTitle":"Generated","sessionId":"old"}`,
		`{"type":"assistant","sessionId":"old","timestamp":"2026-10-01T10:00:00Z","message":{"id":"m_old","model":"claude-haiku-4-5","usage":{"output_tokens":7}}}`,
	)
	if err := os.Chtimes(old, now.AddDate(0, 0, -13), now.AddDate(0, 0, -13)); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(proj, "new.jsonl")
	writeJSONL(t, fresh,
		`{"type":"ai-title","aiTitle":"First","sessionId":"new"}`,
		`{"type":"ai-title","aiTitle":"Second","sessionId":"new"}`,
		`{"type":"assistant","sessionId":"new","timestamp":"2026-10-14T10:00:00Z","message":{"id":"m_new","model":"claude-haiku-4-5","usage":{"output_tokens":5}}}`,
		`{"type":"assistant","sessionId":"new","timestamp":"2026-10-14T10:00:01Z","message":{"id":"m_new2","model":"claude-opus-4-20250514","usage":{"output_tokens":1}}}`,
		`{"type":"assist`,
	)
	trimLastNewline(t, fresh)
	cache := &SessionCache{}
	sc := SessionScanner{ConfigDir: config, Home: home, Location: time.UTC, Cache: cache}
	opts := SessionScanOptions{RecentLimit: 5, SessionLimit: 300, CostWindow: usage.Today(now, time.UTC).Window, Now: now}
	res, err := sc.Scan(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.BadLines != 0 {
		t.Fatalf("an unterminated last line is not malformed: %d", res.BadLines)
	}
	if res.DailyTokens["2026-10-01"] != 7 || res.DailyTokens["2026-10-14"] != 6 {
		t.Fatalf("heatmap must not follow the cost window: %v", res.DailyTokens)
	}
	if len(res.RecentItems) != 2 || res.RecentItems[1].Title != "Renamed" || res.RecentItems[0].Title != "Second" {
		t.Fatalf("recent %+v", res.RecentItems)
	}
	if res.CostSummary.Totals.OutputTokens != 6 || res.SessionCount != 1 || len(res.CostSummary.ModelRows) != 2 {
		t.Fatalf("cost %+v models %+v", res.CostSummary.Totals, res.CostSummary.ModelRows)
	}
	if len(cache.files) != 2 {
		t.Fatalf("cache %d", len(cache.files))
	}
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.Scan(context.Background(), opts); err != nil || len(cache.files) != 1 {
		t.Fatalf("a removed file must leave the cache: %v %d", err, len(cache.files))
	}
}

func TestSpeedSamplesHaveNoTTFT(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	dir := filepath.Join(config, "projects", "demo")
	mustMkdir(t, dir)
	base := time.Now().UTC().Truncate(time.Second)
	userAt := base.Format(time.RFC3339)
	goodAt := base.Add(2 * time.Second).Format(time.RFC3339)
	sameAt := base.Add(3 * time.Second).Format(time.RFC3339)
	writeJSONL(t, filepath.Join(dir, "speed.jsonl"),
		`{"type":"user","sessionId":"spd","timestamp":"`+userAt+`","cwd":"/work/demo"}`,
		`{"type":"assistant","sessionId":"spd","timestamp":"`+goodAt+`","effort":"high","message":{"id":"msg_spd","model":"claude-haiku-4-5","usage":{"output_tokens":100}}}`,
		`{"type":"user","sessionId":"spd","timestamp":"`+sameAt+`"}`,
		`{"type":"assistant","sessionId":"spd","timestamp":"`+sameAt+`","effort":"low","message":{"id":"msg_zero","model":"claude-haiku-4-5","usage":{"output_tokens":50}}}`,
	)
	sc := SessionScanner{ConfigDir: config, Home: home}
	now := base.Add(time.Hour)
	sessions, err := sc.SpeedSessions(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || len(sessions[0].Samples) != 1 {
		t.Fatalf("samples %+v", sessions)
	}
	sample := sessions[0].Samples[0]
	if sample.HasTTFT || sample.TTFT != 0 || sample.OutputTokens != 100 || sample.Decode != 2*time.Second || sample.Effort != "high" {
		t.Fatalf("sample %+v", sample)
	}
	if !sample.At.Equal(base) {
		t.Fatal("at")
	}
	report, err := sc.ScanSpeed(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 1 || report.Groups[0].Calls != 0 || !report.Groups[0].HasTPS() {
		t.Fatalf("report %+v", report.Groups)
	}
}

func TestSpeedSamplesCarryTier(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	dir := filepath.Join(config, "projects", "demo")
	mustMkdir(t, dir)
	base := time.Now().UTC().Truncate(time.Second)
	at := func(sec int) string { return base.Add(time.Duration(sec) * time.Second).Format(time.RFC3339) }
	turn := func(sec int, id, usage string) []string {
		return []string{
			`{"type":"user","sessionId":"tier","timestamp":"` + at(sec) + `"}`,
			`{"type":"assistant","sessionId":"tier","timestamp":"` + at(sec+2) + `","message":{"id":"` + id + `","model":"claude-opus-4-8","usage":` + usage + `}}`,
		}
	}
	var lines []string
	lines = append(lines, turn(0, "m_fast", `{"output_tokens":10,"speed":"fast"}`)...)
	lines = append(lines, turn(10, "m_std", `{"output_tokens":10,"speed":"standard"}`)...)
	lines = append(lines, turn(20, "m_old", `{"output_tokens":10}`)...)
	writeJSONL(t, filepath.Join(dir, "tier.jsonl"), lines...)
	sc := SessionScanner{ConfigDir: config, Home: home}
	sessions, err := sc.SpeedSessions(context.Background(), base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || len(sessions[0].Samples) != 3 {
		t.Fatalf("samples %+v", sessions)
	}
	tiers := map[speed.Tier]int{}
	for _, s := range sessions[0].Samples {
		tiers[s.Tier]++
	}
	if tiers[speed.TierFast] != 1 || tiers[speed.TierStandard] != 1 || tiers[speed.TierUnknown] != 1 {
		t.Fatalf("tiers %v", tiers)
	}
}

func TestSpeedDuplicateKeepsOriginalRequestTime(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	dir := filepath.Join(config, "projects", "demo")
	mustMkdir(t, dir)
	base := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(dir, "speed.jsonl"),
		`{"type":"user","sessionId":"s","timestamp":"2026-10-16T00:00:00Z"}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-16T00:00:10Z","message":{"id":"msg_same","model":"claude-opus-4-8","usage":{"output_tokens":100},"content":[{"type":"tool_use"}]}}`,
		`{"type":"tool_result","sessionId":"s","timestamp":"2026-10-16T00:00:11Z"}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-16T00:00:12Z","message":{"id":"msg_same","model":"claude-opus-4-8","usage":{"output_tokens":100},"content":[{"type":"tool_use"}]}}`,
	)
	sessions, err := (SessionScanner{ConfigDir: config, Home: home}).SpeedSessions(context.Background(), base.Add(time.Hour))
	if err != nil || len(sessions) != 1 || len(sessions[0].Samples) != 1 {
		t.Fatalf("scan err=%v sessions=%+v", err, sessions)
	}
	sample := sessions[0].Samples[0]
	if !sample.At.Equal(base) || sample.Decode != 12*time.Second {
		t.Fatalf("sample %+v", sample)
	}
}

func TestSpeedDuplicateDoesNotBorrowLaterParent(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, "cfg")
	dir := filepath.Join(config, "projects", "demo")
	mustMkdir(t, dir)
	base := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(dir, "speed.jsonl"),
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-16T00:00:10Z","message":{"id":"msg_same","model":"claude-opus-4-8","usage":{"output_tokens":100}}}`,
		`{"type":"tool_result","sessionId":"s","timestamp":"2026-10-16T00:00:11Z"}`,
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-16T00:00:12Z","message":{"id":"msg_same","model":"claude-opus-4-8","usage":{"output_tokens":200}}}`,
	)
	sessions, err := (SessionScanner{ConfigDir: config, Home: home}).SpeedSessions(context.Background(), base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("later tool result fabricated a request start: %+v", sessions)
	}
}

func TestSpeedDedupKeepsEarliestKnownParentAcrossMergeOrder(t *testing.T) {
	base := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	records := []usageRecord{
		{at: base.Add(10 * time.Second), hasTime: true, firstAt: base.Add(10 * time.Second), hasFirst: true, parent: base, hasParent: true, parts: TokenParts{Output: 100}, toolUse: true},
		{at: base.Add(12 * time.Second), hasTime: true, firstAt: base.Add(12 * time.Second), hasFirst: true, parent: base.Add(11 * time.Second), hasParent: true, parts: TokenParts{Output: 200}},
		{at: base.Add(13 * time.Second), hasTime: true, firstAt: base.Add(13 * time.Second), hasFirst: true, parts: TokenParts{Output: 300}},
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 1, 0}} {
		sink := newScanSink("")
		for _, i := range order {
			part := newScanSink("")
			part.keepKey("message", records[i])
			sink.merge(part)
		}
		got := sink.kept["message"]
		if got.parts.Output != 300 || !got.at.Equal(base.Add(13*time.Second)) || !got.toolUse {
			t.Fatalf("order %v changed usage selection: %+v", order, got)
		}
		if !got.hasParent || !got.parent.Equal(base) {
			t.Fatalf("order %v parent %+v", order, got)
		}
	}
	for _, order := range [][]int{{0, 1}, {1, 0}} {
		withoutParent := []usageRecord{
			{firstAt: base.Add(10 * time.Second), hasFirst: true, parts: TokenParts{Output: 100}},
			{firstAt: base.Add(12 * time.Second), hasFirst: true, parent: base.Add(11 * time.Second), hasParent: true, parts: TokenParts{Output: 200}},
		}
		sink := newScanSink("")
		for _, i := range order {
			part := newScanSink("")
			part.keepKey("message", withoutParent[i])
			sink.merge(part)
		}
		if sink.kept["message"].hasParent {
			t.Fatalf("order %v borrowed the later parent", order)
		}
	}

	sink := newScanSink("")
	sink.keepKey("unknown", usageRecord{parts: TokenParts{Output: 1}})
	sink.keepKey("unknown", usageRecord{parts: TokenParts{Output: 2}})
	if sink.kept["unknown"].hasParent {
		t.Fatal("missing parent must stay missing")
	}
}

func TestSpeedScanPropagatesSessionFileReadError(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "cfg", "projects", "demo", "speed.jsonl")
	mustMkdir(t, filepath.Dir(path))
	writeJSONL(t, path, `{"type":"assistant","sessionId":"s","timestamp":"2026-10-16T00:00:01Z","message":{"id":"m","model":"claude-opus-4-8","usage":{"output_tokens":1}}}`)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600)
	if f, err := os.Open(path); err == nil {
		f.Close()
		t.Skip("filesystem does not enforce unreadable file permissions")
	}
	cache := &SessionCache{}
	_, err := (SessionScanner{ConfigDir: filepath.Join(home, "cfg"), Home: home, Cache: cache}).ScanSpeed(context.Background(), time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("unreadable session file must fail the scan")
	}
	if len(cache.files) != 0 {
		t.Fatalf("failed parse cached %d files", len(cache.files))
	}
}

func TestSpeedScanPropagatesProjectsWalkError(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "cfg", "projects", "locked")
	mustMkdir(t, dir)
	writeJSONL(t, filepath.Join(dir, "speed.jsonl"), `{}`)
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("filesystem does not enforce unreadable directory permissions")
	}
	if _, err := (SessionScanner{ConfigDir: filepath.Join(home, "cfg"), Home: home}).ScanSpeed(context.Background(), time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("projects walk error must fail the scan")
	}
}

func itemByID(t *testing.T, res SessionUsageSummary, id string) SessionItem {
	t.Helper()
	for _, item := range res.RecentItems {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("missing %s", id)
	return SessionItem{}
}

func containsOutput(res SessionUsageSummary, n int64) bool {
	for _, item := range res.RecentItems {
		if item.Totals.Output == n {
			return true
		}
	}
	return false
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func trimLastNewline(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data[:len(data)-1], 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	var b []byte
	for _, line := range lines {
		b = append(b, line...)
		b = append(b, '\n')
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}
