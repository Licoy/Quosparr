package grok

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage"
)

func TestSessionScanCacheUsesExactRangeModelsAndDeduplicates(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"info":{"id":"s"},"generated_title":"Safe title","current_model_id":"fallback"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line1 := `{"timestamp":1792454401,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":100,"cachedReadTokens":50,"modelUsage":{"model-a":{"inputTokens":100,"cachedReadTokens":50}}}}}}`
	line2 := `{"timestamp":1792454402,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":900,"cachedReadTokens":90,"modelUsage":{"model-a":{"inputTokens":400,"cachedReadTokens":40},"model-b":{"inputTokens":500,"cachedReadTokens":50}}}}}}`
	line3 := `{"timestamp":1792454403,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":10,"modelUsage":{"model-b":{"inputTokens":10}}}}}}`
	data := fmt.Sprintf("%s\n%s\n%s\n%s\n", line1, line1, line2, line3)
	if err := os.WriteFile(filepath.Join(dir, "updates.jsonl"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	start, end := time.Unix(1792454401, 0), time.Unix(1792454404, 0)
	r, err := (SessionScanner{GrokHome: home, Cache: &CacheScanCache{}}).ScanCache(context.Background(), usage.Window{Start: start, End: end}, end)
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.InputTokens != 1000 || r.Totals.CachedInputTokens != 140 || r.Totals.Samples != 2 || r.Totals.MissingSamples != 1 {
		t.Fatalf("totals = %+v", r.Totals)
	}
	if len(r.Models) != 2 || len(r.Sessions) != 1 || r.Sessions[0].Name != "Safe title" {
		t.Fatalf("rows = %+v / %+v", r.Models, r.Sessions)
	}
}

func TestSessionScanCacheHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (SessionScanner{GrokHome: t.TempDir()}).ScanCache(ctx, usage.Window{Start: time.Unix(0, 0), End: time.Unix(1, 0)}, time.Time{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestSessionScanCacheDefersIncompleteAppendedTurn(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"info":{"id":"s"},"current_model_id":"m"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	complete := `{"timestamp":1792454401,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":100,"cachedReadTokens":50}}}}` + "\n"
	tail := `{"timestamp":1792454402,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":200`
	path := filepath.Join(dir, "updates.jsonl")
	if err := os.WriteFile(path, []byte(complete+tail), 0o600); err != nil {
		t.Fatal(err)
	}
	s := SessionScanner{GrokHome: home, Cache: &CacheScanCache{}}
	w := usage.Window{Start: time.Unix(1792454400, 0), End: time.Unix(1792454410, 0)}
	first, err := s.ScanCache(context.Background(), w, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if first.Totals.Samples != 1 {
		t.Fatalf("first = %+v", first.Totals)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`,"cachedReadTokens":100}}}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := s.ScanCache(context.Background(), w, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if second.Totals.Samples != 2 || second.Totals.InputTokens != 300 || second.Totals.CachedInputTokens != 150 {
		t.Fatalf("second = %+v", second.Totals)
	}
}

func TestSessionScanCacheCountsCompleteFinalLineWithoutNewline(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	summaryPath := filepath.Join(dir, "summary.json")
	if err := os.WriteFile(summaryPath, []byte(`{"info":{"id":"s"},"current_model_id":"m"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":1792454401,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":100,"cachedReadTokens":50}}}}`
	if err := os.WriteFile(filepath.Join(dir, "updates.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	w := usage.Window{Start: time.Unix(1792454400, 0), End: time.Unix(1792454402, 0)}
	s := SessionScanner{GrokHome: home, Cache: &CacheScanCache{}}
	r, err := s.ScanCache(context.Background(), w, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Totals.Samples != 1 || r.Totals.InputTokens != 100 {
		t.Fatalf("totals = %+v", r.Totals)
	}
	if len(r.Models) != 1 || r.Models[0].ID != "m" {
		t.Fatalf("models = %+v", r.Models)
	}
	if err := os.WriteFile(summaryPath, []byte(`{"info":{"id":"s"},"current_model_id":"new-model"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err = s.ScanCache(context.Background(), w, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Models) != 1 || r.Models[0].ID != "new-model" {
		t.Fatalf("warm models = %+v", r.Models)
	}
}

func TestSessionScanCacheRejectsSemanticErrorWithoutFinalNewline(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"info":{"id":"s"},"current_model_id":"m"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	line := `{"timestamp":1792454401,"params":{"update":{"sessionUpdate":"turn_completed","usage":{"inputTokens":10,"cachedReadTokens":20}}}}`
	if err := os.WriteFile(filepath.Join(dir, "updates.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	w := usage.Window{Start: time.Unix(1792454400, 0), End: time.Unix(1792454402, 0)}
	if _, err := (SessionScanner{GrokHome: home}).ScanCache(context.Background(), w, time.Now()); err == nil {
		t.Fatal("invalid counters accepted")
	}
}
