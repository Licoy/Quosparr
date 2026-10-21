package usage

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanCacheWeightedMissingModelsRangeAndDuplicateFile(t *testing.T) {
	home := t.TempDir()
	active := filepath.Join(home, "sessions", "2026", "10", "20")
	archived := filepath.Join(home, "archived_sessions", "2026", "10", "20")
	if err := os.MkdirAll(active, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(
		`{"timestamp":"2026-10-20T00:00:00Z","type":"session_meta","payload":{"id":"s1"}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:00Z","type":"turn_context","payload":{"model":"model-a"}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":5},"total_token_usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":5}}}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:01.500Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":5},"total_token_usage":{"input_tokens":100,"cached_input_tokens":50,"output_tokens":5}}}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:02Z","type":"event_msg","turn_context":{"model":"model-b"},"payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":900,"cached_input_tokens":90},"total_token_usage":{"input_tokens":1000,"cached_input_tokens":140,"output_tokens":5}}}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:03Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10}}}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":0,"cached_input_tokens":0}}}}` + "\n" +
			`{"timestamp":"2026-10-20T00:00:05Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":777,"cached_input_tokens":7}}}}` + "\n")
	for _, dir := range []string{active, archived} {
		if err := os.WriteFile(filepath.Join(dir, "rollout-same.jsonl"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "session_index.jsonl"), []byte(`{"id":"s1","thread_name":"Safe title"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 20, 0, 0, 1, 0, time.UTC)
	end := time.Date(2026, 10, 20, 0, 0, 5, 0, time.UTC)
	s := Scanner{CodexHome: home, Location: time.UTC, Cache: &CacheScanCache{}}
	r, err := s.ScanCache(context.Background(), Window{Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.InputTokens != 1000 || r.Totals.CachedInputTokens != 140 || r.Totals.Samples != 3 || r.Totals.MissingSamples != 1 {
		t.Fatalf("totals = %+v", r.Totals)
	}
	if len(r.Models) != 2 || len(r.Sessions) != 1 {
		t.Fatalf("models/sessions = %+v / %+v", r.Models, r.Sessions)
	}
	if r.Sessions[0].ID != "s1" || r.Sessions[0].Name != "Safe title" {
		t.Fatalf("session row = %+v", r.Sessions[0])
	}
	if got, ok := r.Totals.Percent(); !ok || math.Abs(got-14) > 1e-12 {
		t.Fatalf("percent = %v, %v", got, ok)
	}
	// A different range reuses the same parsed file without retaining the old range.
	r, err = s.ScanCache(context.Background(), Window{Start: end.Add(-time.Second), End: end})
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.Samples != 1 || r.Totals.InputTokens != 0 {
		t.Fatalf("boundary totals = %+v", r.Totals)
	}
}

func TestScanCacheHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Scanner{CodexHome: t.TempDir()}).ScanCache(ctx, Window{Start: time.Unix(0, 0), End: time.Unix(1, 0)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestScanCacheCountsCompleteFinalLineAndDefersTruncatedTail(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tail.jsonl")
	complete := `{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":50}}}}`
	if err := os.WriteFile(path, []byte(complete), 0o600); err != nil {
		t.Fatal(err)
	}
	s := Scanner{CodexHome: home, Cache: &CacheScanCache{}}
	w := Window{Start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 20, 0, 0, 3, 0, time.UTC)}
	first, err := s.ScanCache(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if first.Totals.Samples != 1 {
		t.Fatalf("complete final line = %+v", first.Totals)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n" + `{"timestamp":"2026-10-20T00:00:02Z","type":"event_msg"`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := s.ScanCache(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if second.Totals.Samples != 1 {
		t.Fatalf("truncated tail = %+v", second.Totals)
	}
}

func TestCacheOnlyFieldsDoNotChangeCostParserContract(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"timestamp":"2026-10-20T00:00:00Z","type":"turn_context","payload":{"model":"m"}}` + "\n" +
		`{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":50},"total_token_usage":{"input_tokens":"bad"}}}}` + "\n"
	w := Window{Start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 20, 0, 0, 2, 0, time.UTC)}
	path := filepath.Join(dir, "r.jsonl")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	// Without a date in its name the cost scan picks the log by mtime.
	if err := os.Chtimes(path, w.Start, w.Start); err != nil {
		t.Fatal(err)
	}
	s := Scanner{CodexHome: home}
	cost, err := s.Scan(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	if cost.Totals.InputTokens != 100 {
		t.Fatalf("cost totals = %+v", cost.Totals)
	}
	if _, err := s.ScanCache(context.Background(), w); err == nil {
		t.Fatal("cache scan accepted invalid cumulative counters")
	}
}

func TestScanCacheRejectsSemanticErrorWithoutFinalNewline(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1,"cached_input_tokens":2}}}}`
	if err := os.WriteFile(filepath.Join(dir, "bad.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	w := Window{Start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 20, 0, 0, 2, 0, time.UTC)}
	if _, err := (Scanner{CodexHome: home}).ScanCache(context.Background(), w); err == nil {
		t.Fatal("invalid counters accepted")
	}
}

func TestScanCacheDefersCounterValueTruncatedAtSnapshot(t *testing.T) {
	for name, tail := range map[string]string{
		"input":  `{"timestamp":"2026-10-20T00:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":`,
		"cached": `{"timestamp":"2026-10-20T00:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1,"cached_input_tokens":`,
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, "sessions")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			complete := `{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":50}}}}` + "\n"
			if err := os.WriteFile(filepath.Join(dir, "tail.jsonl"), []byte(complete+tail), 0o600); err != nil {
				t.Fatal(err)
			}
			w := Window{Start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 20, 0, 0, 3, 0, time.UTC)}
			r, err := (Scanner{CodexHome: home}).ScanCache(context.Background(), w)
			if err != nil {
				t.Fatal(err)
			}
			if r.Totals.Samples != 1 {
				t.Fatalf("totals = %+v", r.Totals)
			}
		})
	}
}

func TestScanCacheDeduplicatesOnlyUnchangedCumulativeUsage(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := func(at string, total string) string {
		totalField := ""
		if total != "" {
			totalField = `,"total_token_usage":{"input_tokens":` + total + `,"cached_input_tokens":` + total + `,"output_tokens":5}`
		}
		return `{"timestamp":"` + at + `","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":5}` + totalField + `}}}`
	}
	data := line("2026-10-20T00:00:01Z", "100") + "\n" +
		line("2026-10-20T00:00:02Z", "200") + "\n" +
		line("2026-10-20T00:00:03Z", "200") + "\n" +
		line("2026-10-20T00:00:04Z", "") + "\n" +
		line("2026-10-20T00:00:05Z", "") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "r.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := (Scanner{CodexHome: home}).ScanCache(context.Background(), Window{Start: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 20, 0, 0, 6, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.Samples != 4 || r.Totals.InputTokens != 400 || r.Totals.CachedInputTokens != 320 {
		t.Fatalf("totals = %+v", r.Totals)
	}
}

func TestScanCacheRejectsDivergentDuplicate(t *testing.T) {
	home := t.TempDir()
	for i, root := range []string{"sessions", "archived_sessions"} {
		dir := filepath.Join(home, root)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := []byte(`{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"info":{"last_token_usage":{"input_tokens":` + string(rune('1'+i)) + `,"cached_input_tokens":0}}}}`)
		if err := os.WriteFile(filepath.Join(dir, "same.jsonl"), line, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := (Scanner{CodexHome: home}).ScanCache(context.Background(), Window{Start: time.Unix(0, 0), End: time.Now()})
	if err == nil {
		t.Fatal("expected duplicate conflict")
	}
}

func writeCacheLog(t *testing.T, path, data string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

const cacheTestSample = `{"timestamp":"2026-10-20T00:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":50}}}}` + "\n"

func TestScanCacheSkipsCompactionOverRealLineLimit(t *testing.T) {
	home := t.TempDir()
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	compacted := `{"timestamp":"2026-10-20T00:00:00Z","type":"compacted","payload":{"message":"` + strings.Repeat("a", defaultMaxLineBytes) + `"}}` + "\n"
	writeCacheLog(t, filepath.Join(home, "sessions", "rollout-example.jsonl"), compacted+cacheTestSample, start.Add(time.Hour))
	r, err := (Scanner{CodexHome: home}).ScanCache(context.Background(), Window{Start: start, End: start.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.Samples != 1 || r.Totals.InputTokens != 100 || r.UnreadLines != 0 {
		t.Fatalf("report = %+v", r)
	}
}

func TestScanCacheCountsUnreadableLinesAndKeepsOtherLogs(t *testing.T) {
	home := t.TempDir()
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	pad := strings.Repeat("a", 1024)
	data := `{"timestamp":"2026-10-20T00:00:00Z","type":"response_item","payload":{"type":"function_call_output","output":"` + pad + `"}}` + "\n" +
		`{"timestamp":"2026-10-20T00:00:00Z","type":"event_msg","payload":{"type":"item_completed","item":"` + pad + `"}}` + "\n" +
		// Oversized lines that may hold usage: a token_count event, and one
		// whose type lies past the kept start.
		`{"timestamp":"2026-10-20T00:00:00Z","type":"event_msg","payload":{"type":"token_count","pad":"` + pad + `"}}` + "\n" +
		`{"pad":"` + strings.Repeat("a", LineHeadBytes) + `","type":"compacted"}` + "\n" +
		`{"timestamp":"2026-10-20T00:00:00Z","type":"event_msg","payload":{"type":"token_count"` + "\n" +
		cacheTestSample
	writeCacheLog(t, filepath.Join(home, "sessions", "rollout-a.jsonl"), data, start.Add(time.Hour))
	writeCacheLog(t, filepath.Join(home, "sessions", "rollout-b.jsonl"), cacheTestSample, start.Add(time.Hour))
	// Damage in a log last modified before the range does not mark it.
	writeCacheLog(t, filepath.Join(home, "sessions", "rollout-old.jsonl"), `{"type":"event_msg","payload":{"type":"token_count"`+"\n", start.Add(-time.Hour))
	s := Scanner{CodexHome: home, MaxLineBytes: 512}
	r, err := s.ScanCache(context.Background(), Window{Start: start, End: start.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.Samples != 2 || r.Totals.InputTokens != 200 || r.UnreadLines != 3 {
		t.Fatalf("report = %+v", r)
	}
	r, err = s.ScanCache(context.Background(), Window{Start: start.Add(-24 * time.Hour), End: start.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if r.UnreadLines != 4 {
		t.Fatalf("wider range unread = %d", r.UnreadLines)
	}
}
