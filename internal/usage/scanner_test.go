package usage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

// scanDir is a codex home for scanner tests; it needs no SQLite.
type scanDir struct {
	t    testing.TB
	root string
}

func newScanDir(t testing.TB) scanDir { return scanDir{t, t.TempDir()} }

func (d scanDir) write(rel, contents string) string {
	d.t.Helper()
	p := filepath.Join(d.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		d.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		d.t.Fatal(err)
	}
	return p
}

func (d scanDir) scanner() Scanner { return Scanner{CodexHome: d.root, Location: time.UTC} }

func win(start, end string) Window { return Window{Start: mustTime(start), End: mustTime(end)} }

var dayWindow = win("2026-06-29T00:00:00Z", "2026-06-30T00:00:00Z")

func (d scanDir) apiEquiv(w Window) Summary {
	d.t.Helper()
	s, err := d.scanner().ScanAPIEquivalent(context.Background(), w, fixedNow)
	if err != nil {
		d.t.Fatal(err)
	}
	return s
}

func TestScannerAggregatesTokenCounts(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-test.jsonl", strings.Join([]string{
		`{"timestamp":"2026-06-28T00:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":999,"cached_input_tokens":0,"output_tokens":999,"reasoning_output_tokens":0}}},"rate_limits":{"plan_type":"pro"}}`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":50,"reasoning_output_tokens":10}}},"turn_context":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-29T00:02:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":500,"cached_input_tokens":0,"output_tokens":25,"reasoning_output_tokens":0}}},"turn_context":{"model":"unknown-model"}}`,
	}, "\n"))
	s, err := d.scanner().Scan(context.Background(), win("2026-06-29T00:00:00Z", "2026-06-29T01:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "input", s.Totals.InputTokens, 1500)
	eq(t, "cached", s.Totals.CachedInputTokens, 200)
	eq(t, "output", s.Totals.OutputTokens, 75)
	if s.EstimatedUSD.Sign() <= 0 {
		t.Fatalf("estimated %s", s.EstimatedUSD)
	}
	eqDeep(t, "unknown", s.UnknownModels, []string{"unknown-model"})
	eq(t, "version", s.PricingVersion, PricingVersion)
	// gpt-5.5: 800 uncached @5, 200 cached @0.5, 50 out @30, per million.
	want := decimal.MustParse("0.004").Add(decimal.MustParse("0.0001")).Add(decimal.MustParse("0.0015"))
	wantDec(t, "known model cost", &s.ModelBreakdown[0].EstimatedUSD, want.String())
	wantDec(t, "total", &s.EstimatedUSD, want.String())
}

func TestScannerUsesTurnContextModel(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-context-model.jsonl", lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":0}}}}`,
	))
	s, err := d.scanner().Scan(context.Background(), win("2026-06-29T00:00:00Z", "2026-06-29T01:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "models", len(s.ModelBreakdown), 1)
	eq(t, "model", s.ModelBreakdown[0].Model, "gpt-5.5")
	eq(t, "unknown", len(s.UnknownModels), 0)
	if s.EstimatedUSD.Sign() <= 0 {
		t.Fatal("expected a cost")
	}
}

func TestScannerDatedPathRelevance(t *testing.T) {
	s := Scanner{CodexHome: "/tmp/.codex", Location: time.UTC}
	w := win("2026-06-29T00:00:00Z", "2026-06-29T05:00:00Z")
	cases := []struct {
		path string
		want bool
	}{
		{"/tmp/.codex/sessions/2026/06/29/rollout-a.jsonl", true},
		{"/tmp/.codex/sessions/2026/06/28/rollout-a.jsonl", true},
		{"/tmp/.codex/sessions/2026/06/30/rollout-a.jsonl", true},
		{"/tmp/.codex/sessions/2026/06/27/rollout-a.jsonl", false},
		{"/tmp/.codex/sessions/2026/03/29/rollout-a.jsonl", false},
		// out-of-range numbers roll over like the system calendar: 2026-13-40 is 2027-02-09
		{"/tmp/.codex/sessions/2026/13/40/rollout-a.jsonl", false},
		{"/tmp/.codex/sessions/2026/06/29/rollout-a.jsonl", true},
		// date in the file name, UTC midnight
		{"/tmp/.codex/sessions/rollout-2026-06-29T10-00.jsonl", true},
		{"/tmp/.codex/sessions/rollout-2026-03-29T10-00.jsonl", false},
		// invalid month in the name is ignored, and a missing file counts as relevant
		{"/tmp/.codex/sessions/rollout-2026-13-29.jsonl", true},
	}
	for _, c := range cases {
		if got := s.IsLikelyRelevant(c.path, w); got != c.want {
			t.Errorf("%s: got %v, want %v", c.path, got, c.want)
		}
	}
}

func TestScannerModificationTimeFallback(t *testing.T) {
	d := newScanDir(t)
	p := d.write("sessions/rollout-undated.jsonl", "x\n")
	w := win("2026-06-29T00:00:00Z", "2026-06-30T00:00:00Z")
	s := d.scanner()
	setMtime(t, p, mustTime("2026-06-28T01:00:00Z"))
	if !s.IsLikelyRelevant(p, w) {
		t.Fatal("within a day before the window")
	}
	setMtime(t, p, mustTime("2026-06-26T01:00:00Z"))
	if s.IsLikelyRelevant(p, w) {
		t.Fatal("too old")
	}
	setMtime(t, p, mustTime("2026-07-01T00:00:00Z"))
	if !s.IsLikelyRelevant(p, w) {
		t.Fatal("end boundary is inclusive")
	}
}

func TestScannerDatedPathUsesLocation(t *testing.T) {
	// 2026-06-29 00:00 in Pacific/Auckland is 2026-06-28T12:00Z.
	s := Scanner{Location: mustLoc("Pacific/Auckland")}
	w := win("2026-06-26T12:00:00Z", "2026-06-27T11:59:59Z")
	if s.IsLikelyRelevant("/x/sessions/2026/06/29/a.jsonl", w) {
		t.Fatal("2026-06-29 Auckland starts after window end + 1 day")
	}
	w = win("2026-06-26T12:00:00Z", "2026-06-27T12:00:00Z")
	if !s.IsLikelyRelevant("/x/sessions/2026/06/29/a.jsonl", w) {
		t.Fatal("window end + 1 calendar day reaches the day start")
	}
}

func TestScannerReportDiagnostics(t *testing.T) {
	d := newScanDir(t)
	first := lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`not-json`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":10,"output_tokens":5,"reasoning_output_tokens":0}}}}`,
	)
	first = strings.TrimSuffix(first, "\n")
	second := `{"timestamp":"2026-06-29T00:02:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"cached_input_tokens":20,"output_tokens":10,"reasoning_output_tokens":0}}},"turn_context":{"model":"gpt-5.5"}}`
	d.write("sessions/2026/06/29/rollout-first.jsonl", first)
	d.write("archived_sessions/2026/06/29/rollout-second.jsonl", second)
	s, diag, err := d.scanner().ScanAPIEquivalentReport(context.Background(), dayWindow, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "turns", s.Totals.Turns, 0)
	eq(t, "tokens", s.Totals.TotalTokens, 315)
	eq(t, "bytes", diag.BytesRead, len(first)+len(second))
	eq(t, "candidates", diag.CandidateLines, 3)
	eq(t, "decoded", diag.DecodedLines, 3)
	eq(t, "buffered", diag.MaxBufferedBytes, max(len(first), len(second)))
	eq(t, "files", diag.CandidateFiles, 2)
}

func TestScannerWeeklyWindowPricesKnownModelsOnly(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/25/rollout-weekly.jsonl", lines(
		`{"timestamp":"2026-06-25T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":200,"output_tokens":50,"reasoning_output_tokens":10}}},"turn_context":{"model":"gpt-5.3-codex"}}`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":500,"cached_input_tokens":0,"output_tokens":25,"reasoning_output_tokens":0}}},"turn_context":{"model":"unknown-model"}}`,
	))
	calc := mustTime("2026-06-30T10:00:00Z")
	s, err := d.scanner().ScanAPIEquivalent(context.Background(), win("2026-06-24T00:00:00Z", "2026-07-01T00:00:00Z"), calc)
	if err != nil {
		t.Fatal(err)
	}
	if !s.CalculatedAt.Equal(calc) {
		t.Fatal("calculatedAt")
	}
	eq(t, "source", s.Source, SourceLocalSessions)
	eq(t, "confidence", s.Confidence, ConfidencePartiallyPriced)
	eq(t, "unpriced tokens", s.UnpricedTokens, 525)
	eq(t, "uncached", s.Totals.UncachedInputTokens, 1300)
	eq(t, "cached", s.Totals.CachedInputTokens, 200)
	eq(t, "output", s.Totals.OutputTokens, 75)
	eqDeep(t, "dates", dates(s), []string{"2026-06-25", "2026-06-29"})
	eq(t, "models", len(s.ModelRows), 2)
	eq(t, "m0", s.ModelRows[0].Name, "gpt-5.3-codex")
	eq(t, "m1", s.ModelRows[1].Name, "unknown-model")
	if s.ModelRows[0].EstimatedUSD == nil || s.ModelRows[1].EstimatedUSD != nil {
		t.Fatal("only the known model is priced")
	}
	wantDec(t, "total", s.EstimatedUSD, s.ModelRows[0].EstimatedUSD.String())
	wantDec(t, "day 0", s.DailyRows[0].EstimatedUSD, s.EstimatedUSD.String())
	if s.DailyRows[1].EstimatedUSD != nil {
		t.Fatal("second day has only an unknown model")
	}
	eqDeep(t, "warnings", s.Warnings, []string{"unknown-model:unknown-model"})
}

func projectNames(s Summary) []string {
	out := []string{}
	for _, r := range s.ProjectRows {
		out = append(out, r.Name)
	}
	return out
}

func TestScannerGroupsProjects(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-project.jsonl", lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/Users/me/dev/quosparr"}}`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":100,"output_tokens":50,"reasoning_output_tokens":0}}},"turn_context":{"model":"gpt-5.5"}}`,
	))
	d.write("sessions/2026/06/29/rollout-unknown.jsonl", lines(
		`{"timestamp":"2026-06-29T00:02:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":500,"cached_input_tokens":0,"output_tokens":25,"reasoning_output_tokens":0}}},"turn_context":{"model":"gpt-5.5"}}`,
	))
	s := d.apiEquiv(dayWindow)
	eqDeep(t, "projects", projectNames(s), []string{"quosparr", "Unknown project"})
	eq(t, "p0", s.ProjectRows[0].Totals.TotalTokens, 1050)
	eq(t, "p1", s.ProjectRows[1].Totals.TotalTokens, 525)
}

func TestScannerContextBeforeWindow(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-before.jsonl", lines(
		`{"timestamp":"2026-06-28T23:59:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/Users/me/dev/quosparr"}}`,
		`{"timestamp":"2026-06-28T23:59:30Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":0}}}}`,
	))
	s := d.apiEquiv(dayWindow)
	eqDeep(t, "projects", projectNames(s), []string{"quosparr"})
	eq(t, "model", s.ModelRows[0].Name, "gpt-5.5")
	eq(t, "warnings", len(s.Warnings), 0)
}

func TestScannerChunkBoundariesCRLFAndFinalRecord(t *testing.T) {
	d := newScanDir(t)
	padding := strings.Repeat("x", 262_100)
	irrelevant := `{"timestamp":"2026-06-29T00:00:00Z","type":"event_msg","payload":{"type":"message","content":"` + padding + `"}}`
	token := `{"timestamp":"2026-06-29T00:01:00.123Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":5,"reasoning_output_tokens":2}}},"turn_context":{"model":"gpt-5.5"}}`
	contents := irrelevant + "\r\n" + token
	d.write("sessions/2026/06/29/rollout-boundary.jsonl", contents)
	for _, chunk := range []int{0, 1, 7, 4096} {
		sc := d.scanner()
		sc.ChunkSize = chunk
		s, diag, err := sc.ScanAPIEquivalentReport(context.Background(), dayWindow, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, "uncached", s.Totals.UncachedInputTokens, 80)
		eq(t, "cached", s.Totals.CachedInputTokens, 20)
		eq(t, "output", s.Totals.OutputTokens, 5)
		eq(t, "turns", s.Totals.Turns, 0)
		eq(t, "bytes", diag.BytesRead, len(contents))
	}
}

func TestScannerOversizedAndMalformedLines(t *testing.T) {
	d := newScanDir(t)
	oversized := `{"token_count":"` + strings.Repeat("x", 8*1024*1024) + `"}`
	malformed := `{"type":"token_count"`
	token := `{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}},"turn_context":{"model":"gpt-5.5"}}`
	d.write("sessions/2026/06/29/rollout-oversized.jsonl", oversized+"\n"+malformed+"\n"+token+"\n")
	s, diag, err := d.scanner().ScanAPIEquivalentReport(context.Background(), dayWindow, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "turns", s.Totals.Turns, 0)
	eq(t, "oversized", diag.OversizedLines, 1)
	eq(t, "malformed", diag.MalformedCandidateLines, 1)
	if diag.MaxBufferedBytes > 8*1024*1024 {
		t.Fatalf("buffered %d", diag.MaxBufferedBytes)
	}
	if !hasWarning(s, "oversized-jsonl-lines:1") {
		t.Fatalf("warnings %v", s.Warnings)
	}
}

func TestScannerRejectsInvalidTokenCounts(t *testing.T) {
	d := newScanDir(t)
	pre := `{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{`
	suf := `}}},"turn_context":{"model":"gpt-5.5"}}`
	d.write("sessions/2026/06/29/rollout-invalid-token-counts.jsonl", lines(
		pre+`"input_tokens":-9223372036854775808,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0`+suf,
		pre+`"input_tokens":1,"cached_input_tokens":2,"output_tokens":0,"reasoning_output_tokens":0`+suf,
		pre+`"input_tokens":1,"cached_input_tokens":0,"output_tokens":9223372036854775807,"reasoning_output_tokens":1`+suf,
		pre+`"input_tokens":100,"cached_input_tokens":20,"output_tokens":5,"reasoning_output_tokens":2`+suf,
	))
	s, diag, err := d.scanner().ScanAPIEquivalentReport(context.Background(), dayWindow, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "tokens", s.Totals.TotalTokens, 105)
	eq(t, "turns", s.Totals.Turns, 0)
	eq(t, "malformed", diag.MalformedCandidateLines, 3)
	eq(t, "decoded", diag.DecodedLines, 1)
}

func TestScannerHonoursCancellation(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-cancel.jsonl", tokenLine("2026-06-29T00:01:00Z", 100)+"\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := d.scanner().ScanAPIEquivalent(ctx, dayWindow, fixedNow)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestScannerTurnContextCWDGroupsProjects(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-quosparr.jsonl", lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"turn_context","payload":{"cwd":"/Users/me/dev/quosparr","model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000,"cached_input_tokens":0,"output_tokens":20,"reasoning_output_tokens":0}}}}`,
	))
	d.write("sessions/2026/06/29/rollout-aqbot.jsonl", lines(
		`{"timestamp":"2026-06-29T00:02:00Z","type":"turn_context","payload":{"cwd":"/Users/me/dev/aqbot","model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-29T00:03:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":500,"cached_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":0}}}}`,
	))
	s := d.apiEquiv(dayWindow)
	eqDeep(t, "projects", projectNames(s), []string{"quosparr", "aqbot"})
	eq(t, "t0", s.ProjectRows[0].Totals.TotalTokens, 1020)
	eq(t, "t1", s.ProjectRows[1].Totals.TotalTokens, 510)
}

func TestScannerSymlinksIgnored(t *testing.T) {
	d := newScanDir(t)
	out := d.write("outside.jsonl", tokenLine("2026-06-29T01:00:00Z", 100)+"\n")
	if err := os.MkdirAll(filepath.Join(d.root, "sessions/2026/06/29"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(d.root, "sessions/2026/06/29/rollout-link.jsonl")); err != nil {
		t.Skip(err)
	}
	s := d.apiEquiv(dayWindow)
	eq(t, "source", s.Source, SourceUnavailable)
}

func TestScannerMissingRootsAreEmpty(t *testing.T) {
	s := newScanDir(t).apiEquiv(dayWindow)
	eq(t, "source", s.Source, SourceUnavailable)
	eq(t, "confidence", s.Confidence, ConfidenceUnavailable)
}

// --- turn counting ----------------------------------------------------------

func TestScannerTaskCompleteCountsOneTurn(t *testing.T) {
	d := newScanDir(t)
	m := modelName("gpt-5.6-sol")
	d.write("sessions/2026/06/29/rollout-turns.jsonl", lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"cwd":"/Users/me/dev/content-server"}}`,
		`{"timestamp":"2026-06-29T00:00:01Z","type":"turn_context","payload":{"model":"gpt-5.6-sol"}}`,
		tokenLine("2026-06-29T00:01:00Z", 1000, cachedTokens(800), outputTokens(50), m),
		tokenLine("2026-06-29T00:01:10Z", 1200, cachedTokens(900), outputTokens(40), m),
		tokenLine("2026-06-29T00:01:20Z", 1400, cachedTokens(1000), outputTokens(30), m),
		taskCompleteLine("2026-06-29T00:01:30Z", "turn-1"),
	))
	s := d.apiEquiv(dayWindow)
	eq(t, "turns", s.Totals.Turns, 1)
	eq(t, "uncached", s.Totals.UncachedInputTokens, 900)
	eq(t, "cached", s.Totals.CachedInputTokens, 2700)
	eq(t, "output", s.Totals.OutputTokens, 120)
	eq(t, "daily", s.DailyRows[0].Totals.Turns, 1)
	eqDeep(t, "projects", projectNames(s), []string{"content-server"})
	eq(t, "project turns", s.ProjectRows[0].Totals.Turns, 1)
}

func TestScannerTokenCountsAreNotTurns(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-model-calls.jsonl", lines(
		tokenLine("2026-06-29T00:01:00Z", 100),
		tokenLine("2026-06-29T00:02:00Z", 200),
	))
	s := d.apiEquiv(dayWindow)
	eq(t, "turns", s.Totals.Turns, 0)
	eq(t, "tokens", s.Totals.TotalTokens, 310)
}

func TestScannerEachTaskCompleteIsOneTurn(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-parent.jsonl", lines(
		tokenLine("2026-06-29T00:01:00Z", 100), taskCompleteLine("2026-06-29T00:01:30Z", "parent")))
	d.write("sessions/2026/06/29/rollout-subagent.jsonl", lines(
		tokenLine("2026-06-29T00:02:00Z", 200), taskCompleteLine("2026-06-29T00:02:30Z", "child")))
	s := d.apiEquiv(dayWindow)
	eq(t, "turns", s.Totals.Turns, 2)
	eq(t, "tokens", s.Totals.TotalTokens, 310)
}

// --- scanner as oracle for the repository ----------------------------------

func TestScannerOverflowIsAnError(t *testing.T) {
	d := newScanDir(t)
	d.write("sessions/2026/06/29/rollout-aggregate-overflow.jsonl", lines(
		tokenLine("2026-06-29T01:00:00Z", maxI64, outputTokens(0)),
		tokenLine("2026-06-29T02:00:00Z", 1, outputTokens(0), modelName("unknown-model")),
	))
	_, err := d.scanner().ScanAPIEquivalent(context.Background(), dayWindow, fixedNow)
	var ae *ArithmeticError
	if !errors.As(err, &ae) {
		t.Fatalf("got %v", err)
	}
}

func TestGPT6PricingMatchesScanner(t *testing.T) {
	for _, c := range []struct{ model, usd string }{
		{"gpt-6-astra", "0.132"}, {" GPT-6-SOL ", "0.0264"}, {"gpt-6-luna-2026-09-26", "0.00132"},
	} {
		f := newFixture(t)
		f.write(lines(
			`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"id":"gpt6-session","cwd":"/tmp/gpt6-project"}}`,
			tokenLine("2026-06-29T01:00:00Z", 10000, cachedTokens(2000), outputTokens(1000), modelName(c.model)),
		), "rollout-gpt6.jsonl")
		streamed, err := Scanner{CodexHome: f.codexHome, Location: time.UTC}.ScanAPIEquivalent(context.Background(), fullQuery().Window, fixedNow)
		if err != nil {
			t.Fatal(err)
		}
		wantDec(t, c.model+" streamed", streamed.EstimatedUSD, c.usd)
		wantDec(t, c.model+" streamed row", streamed.ModelRows[0].EstimatedUSD, c.usd)
		eq(t, "warnings", len(streamed.Warnings), 0)
		eqDeep(t, "indexed == streamed", sum(t, f.repo(), fullQuery()), streamed)
	}
}

func TestRepositoryMatchesScannerFieldByField(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		`{"timestamp":"2026-06-28T23:58:00Z","type":"session_meta","payload":{"cwd":"/Users/me/dev/quosparr"}}`,
		`{"timestamp":"2026-06-28T23:59:00Z","type":"turn_context","payload":{"model":"gpt-5.5"}}`,
		`{"timestamp":"2026-06-29T01:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":5,"reasoning_output_tokens":2}}}}`,
		`{"timestamp":"2026-06-29T02:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":200,"cached_input_tokens":0,"output_tokens":10,"reasoning_output_tokens":3}}},"turn_context":{"model":"unknown-model"}}`,
	), "rollout-differential.jsonl")
	q := fullQuery()
	scanned, err := Scanner{CodexHome: f.codexHome, Location: time.UTC}.ScanAPIEquivalent(context.Background(), q.Window, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	indexed := sum(t, f.repo(), q)
	eqDeep(t, "summary", indexed, scanned)
}

func TestRepositoryMatchesScannerTurnRule(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		tokenLine("2026-06-29T01:00:00Z", 100),
		tokenLine("2026-06-29T01:01:00Z", 200),
		taskCompleteLine("2026-06-29T01:02:00Z", "turn-1"),
		tokenLine("2026-06-29T02:00:00Z", 50, modelName("unknown-model")),
	), "rollout-indexed-turns.jsonl")
	q := fullQuery()
	scanned, err := Scanner{CodexHome: f.codexHome, Location: time.UTC}.ScanAPIEquivalent(context.Background(), q.Window, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	indexed := sum(t, f.repo(), q)
	eq(t, "turns", scanned.Totals.Turns, 1)
	eqDeep(t, "totals", indexed.Totals, scanned.Totals)
	eqDeep(t, "daily", indexed.DailyRows, scanned.DailyRows)
	eqDeep(t, "models", indexed.ModelRows, scanned.ModelRows)
}

// --- the log stream on its own ----------------------------------------------

func TestStreamCheckpointsOnlyLFCompleteRecords(t *testing.T) {
	d := newScanDir(t)
	context1 := `{"timestamp":"2026-06-29T00:00:00Z","type":"turn_context","payload":{"model":"gpt-5.5"}}` + "\n"
	token := `{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}}}`
	file := d.write("rollout-partial.jsonl", context1+token)
	checkpoint := uint64(len(context1))
	stream := newLogStream(17, 0, time.UTC)

	var first []parsedLine
	partial, err := stream.read(context.Background(), readRequest{Path: file, FromOffset: checkpoint}, func(l parsedLine) error {
		first = append(first, l)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "first count", len(first), 1)
	eq(t, "first offset", first[0].Offset, checkpoint)
	eq(t, "first LF", first[0].LFComplete, false)
	eq(t, "lastComplete", partial.LastCompleteOffset, checkpoint)
	eq(t, "trailing", partial.TrailingLineStart, checkpoint)
	eq(t, "hasTrailing", partial.HasTrailingLine, true)

	appendFile(t, file, "\n")
	var second []parsedLine
	done, err := stream.read(context.Background(), readRequest{Path: file, FromOffset: checkpoint}, func(l parsedLine) error {
		second = append(second, l)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "second count", len(second), 1)
	eq(t, "second offset", second[0].Offset, checkpoint)
	eq(t, "second LF", second[0].LFComplete, true)
	eq(t, "lastComplete", done.LastCompleteOffset, done.Meta.Size)
	eq(t, "hasTrailing", done.HasTrailingLine, false)
}

func TestStreamReadsOnlyTheOpeningSnapshot(t *testing.T) {
	d := newScanDir(t)
	initial := `{"timestamp":"2026-06-29T00:00:00Z","type":"turn_context","payload":{"model":"gpt-5.5"}}` + "\n"
	appended := `{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":0,"output_tokens":5,"reasoning_output_tokens":0}}}}` + "\n"
	file := d.write("rollout-growing.jsonl", initial)
	stream := newLogStream(19, 0, time.UTC)
	count := 0
	first, err := stream.read(context.Background(), readRequest{Path: file}, func(parsedLine) error {
		count++
		appendFile(t, file, appended)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "count", count, 1)
	eq(t, "snapshot", first.Meta.Size, uint64(len(initial)))
	eq(t, "bytes", first.BytesRead, len(initial))
	suffixCount := 0
	suffix, err := stream.read(context.Background(), readRequest{Path: file, FromOffset: first.LastCompleteOffset}, func(parsedLine) error {
		suffixCount++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "suffix count", suffixCount, 1)
	eq(t, "suffix bytes", suffix.BytesRead, len(appended))
}
