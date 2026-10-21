package claude

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage"
)

func TestSessionScanCacheIncludesWritesDeduplicatesAndTracksMissing(t *testing.T) {
	config := t.TempDir()
	dir := filepath.Join(config, "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := "" +
		`{"type":"custom-title","sessionId":"s","customTitle":"Safe title"}` + "\n" +
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-20T00:00:01Z","message":{"id":"m1","model":"claude-a","usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":50,"output_tokens":1}}}` + "\n" +
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-20T00:00:01Z","message":{"id":"m1","model":"claude-a","usage":{"input_tokens":100,"cache_creation_input_tokens":50,"cache_read_input_tokens":50,"output_tokens":1}}}` + "\n" +
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-20T00:00:02Z","message":{"id":"m2","model":"claude-b","usage":{"input_tokens":700,"cache_creation_input_tokens":100,"cache_read_input_tokens":100,"output_tokens":1}}}` + "\n" +
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-20T00:00:03Z","message":{"id":"m3","model":"claude-b","usage":{"input_tokens":10,"cache_creation_input_tokens":0,"output_tokens":1}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	s := SessionScanner{ConfigDir: config, Cache: &SessionCache{}}
	start := time.Date(2026, 10, 20, 0, 0, 1, 0, time.UTC)
	end := start.Add(3 * time.Second)
	r, err := s.ScanCache(context.Background(), usage.Window{Start: start, End: end}, end)
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.InputTokens != 1100 || r.Totals.CachedInputTokens != 150 || r.Totals.Samples != 2 || r.Totals.MissingSamples != 1 {
		t.Fatalf("totals = %+v", r.Totals)
	}
	if len(r.Models) != 2 || len(r.Sessions) != 1 || r.Sessions[0].Name != "Safe title" {
		t.Fatalf("rows = %+v / %+v", r.Models, r.Sessions)
	}
	if got, ok := r.Totals.Percent(); !ok || math.Abs(got-150.0/11) > 1e-12 {
		t.Fatalf("percent = %v, %v", got, ok)
	}
}

func TestSessionScanCacheHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (SessionScanner{ConfigDir: t.TempDir()}).ScanCache(ctx, usage.Window{Start: time.Unix(0, 0), End: time.Unix(1, 0)}, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestSessionScanCacheSkipsOversizedRecordsAndCountsUnreadable(t *testing.T) {
	config := t.TempDir()
	dir := filepath.Join(config, "projects", "p")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pad := strings.Repeat("a", maxJSONLLine)
	lines := "" +
		// A pasted image or tool result: a user record has no usage.
		`{"parentUuid":null,"type":"user","sessionId":"s","message":{"content":"` + pad + `"}}` + "\n" +
		// An assistant message puts its type after the message, past the
		// kept start, so it may hold usage.
		`{"parentUuid":null,"message":{"content":"` + pad + `"},"type":"assistant","sessionId":"s"}` + "\n" +
		`{"type":"assistant","sessionId":"s","message":` + "\n" +
		`{"type":"assistant","sessionId":"s","timestamp":"2026-10-20T00:00:01Z","message":{"id":"m1","model":"claude-a","usage":{"input_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":50,"output_tokens":1}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	old := filepath.Join(dir, "old.jsonl")
	if err := os.WriteFile(old, []byte(`{"type":"assistant",`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, start.Add(-time.Hour), start.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	s := SessionScanner{ConfigDir: config}
	r, err := s.ScanCache(context.Background(), usage.Window{Start: start, End: start.Add(24 * time.Hour)}, start)
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.Samples != 1 || r.Totals.InputTokens != 150 || r.UnreadLines != 2 {
		t.Fatalf("report = %+v", r)
	}
	r, err = s.ScanCache(context.Background(), usage.Window{Start: start.Add(-24 * time.Hour), End: start.Add(24 * time.Hour)}, start)
	if err != nil {
		t.Fatal(err)
	}
	if r.UnreadLines != 3 {
		t.Fatalf("wider range unread = %d", r.UnreadLines)
	}
}
