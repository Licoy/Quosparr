package gemini

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage"
)

func TestSessionScanCacheWeightsWholePromptAndHonorsWindow(t *testing.T) {
	h := sessionNewTestHome(t)
	sessionWriteFixture(t, h)
	sessionMust(t, os.WriteFile(filepath.Join(h.app, "conversations", "broken.db"), []byte("not sqlite"), 0o600))
	now := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	scanner := h.scanner(t)
	report, err := scanner.ScanCache(context.Background(), usage.Window{Start: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), End: now}, now)
	sessionMust(t, err)
	if report.Totals.InputTokens != 253_700 || report.Totals.CachedInputTokens != 200 || report.Totals.Samples != 4 || report.UnreadLines != 1 {
		t.Fatalf("totals = %+v, unread = %d", report.Totals, report.UnreadLines)
	}
	if len(report.Models) != 4 || report.Models[0].ID != "gemini-3.8-pro-high" || len(report.Sessions) != 2 || report.Sessions[1].Name != "Alpha" {
		t.Fatalf("rows = %+v / %+v", report.Models, report.Sessions)
	}
	day, err := scanner.ScanCache(context.Background(), usage.Window{Start: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}, now)
	sessionMust(t, err)
	if day.Totals.InputTokens != 2000 || day.Totals.CachedInputTokens != 0 || day.Totals.Samples != 1 || len(day.Models) != 1 || day.Models[0].ID != "claude-sonnet-5" {
		t.Fatalf("day = %+v", day)
	}
	if _, err := scanner.ScanCache(context.Background(), usage.Window{Start: now, End: now}, now); err == nil {
		t.Fatal("empty window accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanner.ScanCache(ctx, usage.Window{Start: now.Add(-time.Hour), End: now}, now); err == nil {
		t.Fatal("cancelled scan succeeded")
	}
}

func TestSessionClientOfNamesBothRoots(t *testing.T) {
	h := sessionNewTestHome(t)
	s := SessionScanner{GeminiHome: h.home}
	if s.SessionClientOf(h.app) != SessionClientApp || s.SessionClientOf(h.cli+string(filepath.Separator)) != SessionClientCLI || s.SessionClientOf(h.home) != "" {
		t.Fatal("session roots are not named by their client")
	}
}
