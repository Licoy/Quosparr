package usage

import (
	"context"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

const maxI64 = int64(math.MaxInt64)

func wantDec(t testing.TB, what string, got *decimal.Decimal, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: got nil, want %s", what, want)
	}
	if !got.Equal(decimal.MustParse(want)) {
		t.Fatalf("%s: got %s, want %s", what, got, want)
	}
}

func hasWarning(s Summary, w string) bool {
	for _, x := range s.Warnings {
		if x == w {
			return true
		}
	}
	return false
}

func dates(s Summary) []string {
	out := []string{}
	for _, r := range s.DailyRows {
		out = append(out, r.Date)
	}
	return out
}

func dayTokens(s Summary) []int64 {
	out := []int64{}
	for _, r := range s.DailyRows {
		out = append(out, r.Totals.TotalTokens)
	}
	return out
}

func eq[T comparable](t testing.TB, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

func eqDeep(t testing.TB, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

// --- aggregation -----------------------------------------------------------

func TestGPT6Pricing(t *testing.T) {
	cases := []struct{ model, usd string }{
		{"gpt-6-astra", "0.132"}, {"gpt-6-astra-2026-09-10", "0.132"}, {" GPT-6-ASTRA ", "0.132"},
		{"gpt-6-sol", "0.0264"}, {"gpt-6-sol-2026-09-26", "0.0264"}, {" GPT-6-SOL ", "0.0264"},
		{"gpt-6-luna", "0.00132"}, {"gpt-6-luna-2026-09-26", "0.00132"}, {" GPT-6-LUNA ", "0.00132"},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
			f := newFixture(t)
			f.write(lines(
				`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"id":"gpt6-session","cwd":"/tmp/gpt6-project"}}`,
				tokenLine("2026-06-29T01:00:00Z", 10000, cachedTokens(2000), outputTokens(1000), modelName(c.model)),
			), "rollout-gpt6.jsonl")
			s := sum(t, f.repo(), fullQuery())
			wantDec(t, "estimated", s.EstimatedUSD, c.usd)
			wantDec(t, "model row", s.ModelRows[0].EstimatedUSD, c.usd)
			eq(t, "confidence", s.Confidence, ConfidencePriced)
			eq(t, "warnings", len(s.Warnings), 0)
			eq(t, "pricing version", s.PricingVersion, PricingVersion)
		})
	}
}

func TestColdBatchScanIncludesEndpoints(t *testing.T) {
	f := newFixture(t)
	contents := lines(
		tokenLine("2026-06-29T00:00:00Z", 100),
		tokenLine("2026-06-29T12:00:00Z", 200),
		tokenLine("2026-06-30T00:00:00Z", 300),
	)
	f.write(contents, "rollout-batch.jsonl")
	r := f.repo()
	first := mkQuery("first", "2026-06-29T00:00:00Z", "2026-06-29T12:00:00Z")
	whole := mkQuery("whole", "2026-06-29T00:00:00Z", "2026-06-30T00:00:00Z")
	got, err := r.Summaries(context.Background(), []Query{first, whole}, fixedNow, RefreshIfChanged, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := r.Diagnostics()
	eq(t, "first turns", got["first"].Totals.Turns, 0)
	eq(t, "first tokens", got["first"].Totals.TotalTokens, 310)
	eq(t, "whole tokens", got["whole"].Totals.TotalTokens, 615)
	if !got["whole"].CalculatedAt.Equal(fixedNow) {
		t.Fatalf("calculatedAt %v", got["whole"].CalculatedAt)
	}
	eq(t, "bytesRead", d.BytesRead, len(contents))
	eq(t, "rebuiltFiles", d.RebuiltFiles, 1)
	eq(t, "indexPasses", d.IndexPasses, 1)
}

func TestLocalCalendarGrouping(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		tokenLine("2026-06-29T18:00:00.125Z", 100),
		tokenLine("2026-06-29T23:30:00-02:00", 200),
	), "rollout-local-days.jsonl")
	q := mkQuery("local", "2026-06-29T17:00:00Z", "2026-06-30T02:00:00Z")
	s := sum(t, f.repo(withLoc(mustLoc("Asia/Singapore"))), q)
	eq(t, "turns", s.Totals.Turns, 0)
	eqDeep(t, "dates", dates(s), []string{"2026-06-30"})
	eqDeep(t, "tokens", dayTokens(s), []int64{310})
}

func TestUTCBoundaryRebucketsLocalIndex(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		tokenLine("2026-06-29T18:00:00Z", 100),
		tokenLine("2026-06-30T01:30:00Z", 200),
	), "rollout-utc-days.jsonl")
	w := Window{Start: mustTime("2026-06-29T17:00:00Z"), End: mustTime("2026-06-30T02:00:00Z")}
	utcQ := Query{ID: "utc", Window: w, DayBoundary: DayUTC}
	localQ := Query{ID: "local", Window: w}
	r := f.repo(withLoc(mustLoc("Asia/Singapore")))
	got, err := r.Summaries(context.Background(), []Query{utcQ, localQ}, fixedNow, RefreshIfChanged, nil)
	if err != nil {
		t.Fatal(err)
	}
	eqDeep(t, "utc dates", dates(got["utc"]), []string{"2026-06-29", "2026-06-30"})
	eqDeep(t, "utc tokens", dayTokens(got["utc"]), []int64{105, 205})
	eqDeep(t, "local dates", dates(got["local"]), []string{"2026-06-30"})
	eqDeep(t, "local tokens", dayTokens(got["local"]), []int64{310})
	eq(t, "rebuilds", r.Diagnostics().DatabaseRebuilds, 0)
}

func TestTimeZoneChangeRebuildsIndex(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T18:00:00Z", 100)+"\n", "rollout-time-zone-change.jsonl")
	q := mkQuery("time-zone", "2026-06-29T17:00:00Z", "2026-06-29T19:00:00Z")
	utc := f.repo(withLoc(time.UTC))
	eqDeep(t, "utc", dates(sum(t, utc, q)), []string{"2026-06-29"})
	if err := utc.Close(); err != nil {
		t.Fatal(err)
	}

	local := f.repo(withLoc(mustLoc("Asia/Singapore")))
	s := sum(t, local, q)
	eqDeep(t, "singapore", dates(s), []string{"2026-06-30"})
	eq(t, "rebuilds", local.Diagnostics().DatabaseRebuilds, 1)
	if !hasWarning(s, "usage-index-rebuilt:time-zone") {
		t.Fatalf("warnings %v", s.Warnings)
	}
}

func TestInvalidTokenCountsReported(t *testing.T) {
	f := newFixture(t)
	overflow := `{"timestamp":"2026-06-29T01:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":9223372036854775807,"reasoning_output_tokens":1}}}}`
	f.write(lines(
		tokenLine("2026-06-29T01:00:00Z", math.MinInt64),
		overflow,
		tokenLine("2026-06-29T02:00:00Z", 100),
	), "rollout-invalid-token-counts.jsonl")
	r := f.repo()
	s := sum(t, r, fullQuery())
	eq(t, "turns", s.Totals.Turns, 0)
	eq(t, "tokens", s.Totals.TotalTokens, 105)
	if !hasWarning(s, "malformed-jsonl-lines:2") {
		t.Fatalf("warnings %v", s.Warnings)
	}
	eq(t, "malformed", r.Diagnostics().MalformedCandidateLines, 2)
}

func TestCrossRecordOverflowIsAnError(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		tokenLine("2026-06-29T01:00:00Z", maxI64, outputTokens(0)),
		tokenLine("2026-06-29T02:00:00Z", 1, outputTokens(0), modelName("unknown-model")),
	), "rollout-aggregate-overflow.jsonl")
	_, err := f.repo().Summaries(context.Background(), []Query{fullQuery()}, fixedNow, RefreshIfChanged, nil)
	var ae *ArithmeticError
	if !errors.As(err, &ae) {
		t.Fatalf("want ArithmeticError, got %v", err)
	}
}

func TestDuplicateQueryID(t *testing.T) {
	f := newFixture(t)
	q := fullQuery()
	_, err := f.repo().Summaries(context.Background(), []Query{q, q}, fixedNow, RefreshIfChanged, nil)
	var de *DuplicateQueryError
	if !errors.As(err, &de) || de.ID != q.ID {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidQueryWindow(t *testing.T) {
	f := newFixture(t)
	r := f.repo()
	bad := Query{ID: "invalid-window", Window: Window{Start: mustTime("2026-06-30T12:00:00Z"), End: mustTime("2026-06-29T12:00:00Z")}}
	_, err := r.Summaries(context.Background(), []Query{bad}, fixedNow, RefreshIfChanged, nil)
	var we *InvalidQueryWindowError
	if !errors.As(err, &we) || we.ID != bad.ID {
		t.Fatalf("got %v", err)
	}
}

func TestPricingVersionChangeReadsNoSources(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 1_000_000, outputTokens(1_000_000))+"\n", "rollout-pricing.jsonl")
	q := fullQuery()
	cheap := sum(t, f.repo(withBook(priceBookOf("cheap", "1", "1", "1"))), q)
	r2 := f.repo(withBook(priceBookOf("expensive", "10", "10", "10")))
	expensive := sum(t, r2, q)
	eq(t, "cheap version", cheap.PricingVersion, "cheap")
	eq(t, "expensive version", expensive.PricingVersion, "expensive")
	wantDec(t, "cheap", cheap.EstimatedUSD, "2")
	wantDec(t, "expensive", expensive.EstimatedUSD, "20")
	d := r2.Diagnostics()
	eq(t, "bytesRead", d.BytesRead, 0)
	eq(t, "rebuilt", d.RebuiltFiles, 0)
}

func TestProjectCostIsModelAware(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"cwd":"/Users/me/dev/quosparr"}}`,
		tokenLine("2026-06-29T01:00:00Z", 1_000_000, outputTokens(0), modelName("gpt-5.6-sol")),
		tokenLine("2026-06-29T02:00:00Z", 1_000_000, outputTokens(0), modelName("gpt-5.6-luna")),
	), "rollout-project-models.jsonl")
	book := NewPriceBook("model-aware", map[string]Price{
		"gpt-5.6-sol":  simplePrice("5", "0.5", "30"),
		"gpt-5.6-luna": simplePrice("0.2", "0.02", "1.2"),
	}, simplePrice("999", "999", "999"))
	s := sum(t, f.repo(withBook(book)), fullQuery())
	eq(t, "projects", len(s.ProjectRows), 1)
	eq(t, "project", s.ProjectRows[0].Name, "quosparr")
	wantDec(t, "project cost", s.ProjectRows[0].EstimatedUSD, "5.2")
	wantDec(t, "total", s.EstimatedUSD, "5.2")
}

func modelRow(s Summary, name string) *BreakdownRow {
	for i := range s.ModelRows {
		if s.ModelRows[i].Name == name {
			return &s.ModelRows[i]
		}
	}
	return nil
}

func TestSparkWindowStaysPriced(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"cwd":"/Users/me/dev/quosparr"}}`,
		tokenLine("2026-06-29T01:00:00Z", 1_000_000, outputTokens(0), modelName("gpt-5.6-sol")),
		tokenLine("2026-06-29T02:00:00Z", 1_000_000, outputTokens(0), modelName("gpt-5.3-codex-spark")),
	), "rollout-spark-models.jsonl")
	s := sum(t, f.repo(), fullQuery())
	eq(t, "confidence", s.Confidence, ConfidencePriced)
	wantDec(t, "total", s.EstimatedUSD, "5.75")
	wantDec(t, "spark", modelRow(s, "gpt-5.3-codex-spark").EstimatedUSD, "1.75")
}

func TestMixedUnknownModelsKeepPricedTotal(t *testing.T) {
	f := newFixture(t)
	f.write(lines(
		`{"timestamp":"2026-06-29T00:00:00Z","type":"session_meta","payload":{"cwd":"/Users/me/dev/quosparr"}}`,
		tokenLine("2026-06-29T01:00:00Z", 1_000_000, outputTokens(0), modelName("gpt-5.6-sol")),
		tokenLine("2026-06-29T02:00:00Z", 500_000, outputTokens(0), modelName("unknown-model")),
		tokenLine("2026-06-29T03:00:00Z", 250_000, outputTokens(0), modelName("codex-auto-review")),
	), "rollout-mixed-unknown.jsonl")
	s := sum(t, f.repo(), fullQuery())
	eq(t, "confidence", s.Confidence, ConfidencePartiallyPriced)
	wantDec(t, "total", s.EstimatedUSD, "4")
	eq(t, "unpriced", s.UnpricedTokens, 750_000)
	eq(t, "day unpriced", s.DailyRows[0].UnpricedTokens, 750_000)
	eq(t, "project unpriced", s.ProjectRows[0].UnpricedTokens, 750_000)
	eq(t, "unknown model unpriced", modelRow(s, "unknown-model").UnpricedTokens, 500_000)
	eq(t, "known model unpriced", modelRow(s, "gpt-5.6-sol").UnpricedTokens, 0)
	wantDec(t, "day amount", s.DailyRows[0].EstimatedUSD, "4")
	wantDec(t, "project amount", s.ProjectRows[0].EstimatedUSD, "4")
	wantDec(t, "sol", modelRow(s, "gpt-5.6-sol").EstimatedUSD, "4")
	if modelRow(s, "unknown-model").EstimatedUSD != nil || modelRow(s, "codex-auto-review").EstimatedUSD != nil {
		t.Fatal("unknown models must be unpriced")
	}
	if !hasWarning(s, "unknown-model:unknown-model") || !hasWarning(s, "unknown-model:codex-auto-review") {
		t.Fatalf("warnings %v", s.Warnings)
	}
}

// --- incremental -----------------------------------------------------------

func TestWarmAndAppendAreIncremental(t *testing.T) {
	f := newFixture(t)
	initial := tokenLine("2026-06-29T01:00:00Z", 100) + "\n"
	file := f.write(initial, "rollout-append.jsonl")
	r := f.repo()
	q := fullQuery()
	ctx := context.Background()

	cold := sum(t, r, q)
	afterCold := r.Diagnostics()
	warm := sum(t, r, q)
	afterWarm := r.Diagnostics()
	eq(t, "cold tokens", cold.Totals.TotalTokens, 105)
	eq(t, "cold turns", cold.Totals.Turns, 0)
	eqDeep(t, "warm == cold", warm, cold)
	eq(t, "warm bytes", afterWarm.BytesRead, afterCold.BytesRead)
	eq(t, "warm validation", afterWarm.ValidationBytesRead, afterCold.ValidationBytesRead)
	if afterWarm.CacheHits <= afterCold.CacheHits {
		t.Fatal("expected cache hits to grow")
	}

	if _, err := r.Summaries(ctx, []Query{q}, fixedNow, RefreshForce, nil); err != nil {
		t.Fatal(err)
	}
	afterForce := r.Diagnostics()
	eq(t, "force bytes", afterForce.BytesRead, afterWarm.BytesRead)
	eq(t, "force validation", afterForce.ValidationBytesRead, afterWarm.ValidationBytesRead)

	suffix := tokenLine("2026-06-29T02:00:00Z", 200) + "\n"
	appendFile(t, file, suffix)
	appended := sum(t, r, q)
	afterAppend := r.Diagnostics()
	eq(t, "append turns", appended.Totals.Turns, 0)
	eq(t, "append tokens", appended.Totals.TotalTokens, 310)
	eq(t, "append bytes", afterAppend.BytesRead-afterForce.BytesRead, len(suffix))
	eq(t, "appendedFiles", afterAppend.AppendedFiles, afterForce.AppendedFiles+1)

	large := strings.Repeat("x", 1024*1024-1) + "\n"
	appendFile(t, file, large)
	sum(t, r, q)
	afterLarge := r.Diagnostics()
	delta := afterLarge.BytesRead + afterLarge.ValidationBytesRead - afterAppend.BytesRead - afterAppend.ValidationBytesRead
	if delta > len(large)+4*hashWindowBytes || delta >= 1024*1024*11/10 {
		t.Fatalf("physical read delta %d", delta)
	}
}

func TestShortSourceExpandsFirstHashWindow(t *testing.T) {
	f := newFixture(t)
	file := f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-growing-prefix.jsonl")
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)

	appendFile(t, file, strings.Repeat("x", 2*hashWindowBytes)+"\n")
	sum(t, r, q)
	rows := mustSourceRows(t, f)
	eq(t, "sources", len(rows), 1)
	eq(t, "firstHashLength", rows[0].FirstHashLength, int64(hashWindowBytes))
	before := r.Diagnostics()

	fh, err := os.OpenFile(file, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fh.WriteAt([]byte("y"), 4096)
	st, _ := fh.Stat()
	fh.WriteAt([]byte("z\n"), st.Size())
	fh.Close()
	sum(t, r, q)
	after := r.Diagnostics()
	eq(t, "rebuilt", after.RebuiltFiles, before.RebuiltFiles+1)
	eq(t, "appended", after.AppendedFiles, before.AppendedFiles)
}

func mustSourceRows(t testing.TB, f *repoFixture) []indexedSource {
	t.Helper()
	s, err := openIndexStore(context.Background(), f.dbPath, CurrentParserVersion, LocationID(time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.sourceRows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestUnterminatedTailIsRereadWithoutDuplication(t *testing.T) {
	f := newFixture(t)
	tail := tokenLine("2026-06-29T01:00:00Z", 100)
	file := f.write(tail, "rollout-tail.jsonl")
	r := f.repo()
	q := fullQuery()

	first := sum(t, r, q)
	d1 := r.Diagnostics()
	second := sum(t, r, q)
	d2 := r.Diagnostics()
	eq(t, "first", first.Totals.TotalTokens, 105)
	eq(t, "second", second.Totals.TotalTokens, 105)
	eq(t, "turns", first.Totals.Turns, 0)
	eq(t, "reread", d2.BytesRead-d1.BytesRead, len(tail))
	if d2.IncompleteTailFiles <= d1.IncompleteTailFiles {
		t.Fatal("incompleteTailFiles must grow")
	}

	appendFile(t, file, "\n")
	completed := sum(t, r, q)
	d3 := r.Diagnostics()
	warm := sum(t, r, q)
	d4 := r.Diagnostics()
	eq(t, "completed", completed.Totals.TotalTokens, 105)
	eq(t, "warm", warm.Totals.TotalTokens, 105)
	eq(t, "completion bytes", d3.BytesRead-d2.BytesRead, len(tail)+1)
	eq(t, "warm bytes", d4.BytesRead, d3.BytesRead)
}

func TestProvisionalMalformedTailDoesNotAccumulate(t *testing.T) {
	f := newFixture(t)
	file := f.write(`{"type":"token_count"`, "rollout-malformed-tail.jsonl")
	r := f.repo()
	q := fullQuery()
	first, second := sum(t, r, q), sum(t, r, q)
	if !hasWarning(first, "malformed-jsonl-lines:1") || !hasWarning(second, "malformed-jsonl-lines:1") || hasWarning(second, "malformed-jsonl-lines:2") {
		t.Fatalf("warnings %v / %v", first.Warnings, second.Warnings)
	}
	appendFile(t, file, "\n")
	completed, warm := sum(t, r, q), sum(t, r, q)
	if !hasWarning(completed, "malformed-jsonl-lines:1") || !hasWarning(warm, "malformed-jsonl-lines:1") || hasWarning(warm, "malformed-jsonl-lines:2") {
		t.Fatalf("warnings %v / %v", completed.Warnings, warm.Warnings)
	}
}

// --- mutation ---------------------------------------------------------------

func TestTruncateAndSameSizeRewriteRebuild(t *testing.T) {
	f := newFixture(t)
	truncated := f.write(lines(tokenLine("2026-06-29T01:00:00Z", 100), tokenLine("2026-06-29T02:00:00Z", 200)), "rollout-truncate.jsonl")
	origRewrite := tokenLine("2026-06-29T03:00:00Z", 300) + "\n"
	rewritten := tokenLine("2026-06-29T03:00:00Z", 900) + "\n"
	eq(t, "same size", len(origRewrite), len(rewritten))
	rewrittenFile := f.write(origRewrite, "rollout-rewrite.jsonl")
	tid, rid := fileID(t, truncated), fileID(t, rewrittenFile)
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()

	replaceKeepingIdentity(t, truncated, tokenLine("2026-06-29T01:00:00Z", 700)+"\n")
	replaceKeepingIdentity(t, rewrittenFile, rewritten)
	eq(t, "truncated id", fileID(t, truncated), tid)
	eq(t, "rewritten id", fileID(t, rewrittenFile), rid)

	s := sum(t, r, q)
	eq(t, "turns", s.Totals.Turns, 0)
	eq(t, "tokens", s.Totals.TotalTokens, 1610)
	eq(t, "rebuilt", r.Diagnostics().RebuiltFiles, before.RebuiltFiles+2)
}

func TestChangedPrefixBeforeAppendRebuilds(t *testing.T) {
	f := newFixture(t)
	file := f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-prefix-change.jsonl")
	id := fileID(t, file)
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()

	replaceKeepingIdentity(t, file, lines(tokenLine("2026-06-29T01:00:00Z", 900), tokenLine("2026-06-29T02:00:00Z", 200)))
	eq(t, "id", fileID(t, file), id)
	s := sum(t, r, q)
	after := r.Diagnostics()
	eq(t, "tokens", s.Totals.TotalTokens, 1110)
	eq(t, "rebuilt", after.RebuiltFiles, before.RebuiltFiles+1)
	eq(t, "appended", after.AppendedFiles, before.AppendedFiles)
}

func TestDeletionCascadesEvents(t *testing.T) {
	f := newFixture(t)
	file := f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-delete.jsonl")
	r := f.repo()
	q := fullQuery()
	eq(t, "initial", sum(t, r, q).Totals.TotalTokens, 105)
	before := r.Diagnostics()
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	s := sum(t, r, q)
	eq(t, "source", s.Source, SourceUnavailable)
	eq(t, "totals", s.Totals, Totals{})
	eq(t, "removed", r.Diagnostics().RemovedFiles, before.RemovedFiles+1)
}

func TestParserVersionMismatchRebuilds(t *testing.T) {
	f := newFixture(t)
	contents := tokenLine("2026-06-29T01:00:00Z", 100) + "\n"
	f.write(contents, "rollout-parser-version.jsonl")
	q := fullQuery()
	old := f.repo(withParser(1))
	sum(t, old, q)
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	r := f.repo(withParser(2))
	s := sum(t, r, q)
	d := r.Diagnostics()
	eq(t, "tokens", s.Totals.TotalTokens, 105)
	eq(t, "bytes", d.BytesRead, len(contents))
	eq(t, "dbRebuilds", d.DatabaseRebuilds, 1)
	eq(t, "rebuilt", d.RebuiltFiles, 1)
}

func TestRebuildReportPolicyLeavesDatabase(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-report.jsonl")
	sum(t, f.repo(withParser(1)), fullQuery())
	before, err := os.ReadFile(f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	r := f.repo(withParser(2), withRebuild(RebuildReport))
	_, err = r.Summaries(context.Background(), []Query{fullQuery()}, fixedNow, RefreshIfChanged, nil)
	var re *RebuildRequiredError
	if !errors.As(err, &re) || re.Reason != ReasonParserVersion {
		t.Fatalf("got %v", err)
	}
	after, _ := os.ReadFile(f.dbPath)
	if len(before) == 0 || string(before) != string(after) {
		t.Fatal("database must stay untouched")
	}
	eq(t, "rebuilds", r.Diagnostics().DatabaseRebuilds, 0)
}

// --- inventory --------------------------------------------------------------

func moveFile(t testing.TB, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func TestAppendedSourceMoveIsAdopted(t *testing.T) {
	f := newFixture(t)
	initial := tokenLine("2026-06-29T01:00:00Z", 100) + "\n"
	original := f.write(initial, "rollout-appended-move.jsonl")
	id := fileID(t, original)
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)

	suffix := tokenLine("2026-06-29T02:00:00Z", 200) + "\n"
	appendFile(t, original, suffix)
	appended := sum(t, r, q)
	beforeMove := r.Diagnostics()
	eq(t, "append turns", appended.Totals.Turns, 0)

	archived := f.archivedPath("rollout-appended-move.jsonl")
	moveFile(t, original, archived)
	eq(t, "id after move", fileID(t, archived), id)
	moved := sum(t, r, q)
	after := r.Diagnostics()
	eq(t, "turns", moved.Totals.Turns, 0)
	eq(t, "tokens", moved.Totals.TotalTokens, 310)
	eq(t, "adopted", after.AdoptedFiles, beforeMove.AdoptedFiles+1)
	eq(t, "rebuilt", after.RebuiltFiles, beforeMove.RebuiltFiles)
	eq(t, "bytes", after.BytesRead, beforeMove.BytesRead)
	eq(t, "validation", after.ValidationBytesRead-beforeMove.ValidationBytesRead, len(initial)+len(suffix))
}

// rewriteThenMove replaces the content keeping inode and mtime, then moves the
// file to the archive; it returns the archived path.
func rewriteThenMove(t testing.TB, f *repoFixture, original, changed string) string {
	t.Helper()
	mt := mtimeOf(t, original)
	replaceKeepingIdentity(t, original, changed)
	setMtime(t, original, mt)
	archived := f.archivedPath(baseName(original))
	moveFile(t, original, archived)
	return archived
}

func baseName(p string) string { return p[strings.LastIndexAny(p, `/\`)+1:] }

func TestUnterminatedTailMoveRebuilds(t *testing.T) {
	f := newFixture(t)
	original := f.write("aaaa\n", "rollout-incomplete-move.jsonl")
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()
	rewriteThenMove(t, f, original, "bbbbb")
	sum(t, r, q)
	after := r.Diagnostics()
	eq(t, "rebuilt", after.RebuiltFiles, before.RebuiltFiles+1)
	eq(t, "adopted", after.AdoptedFiles, before.AdoptedFiles)
}

func TestOversizedCountChangeMoveRebuilds(t *testing.T) {
	f := newFixture(t)
	const maximum = defaultMaxLineBytes
	originalText := strings.Repeat("x", maximum+1) + "\n"
	changed := strings.Repeat("y", maximum/2) + "\n" + strings.Repeat("z", maximum-maximum/2) + "\n"
	eq(t, "sizes", len(originalText), len(changed))
	original := f.write(originalText, "rollout-oversized-move.jsonl")
	r := f.repo()
	q := fullQuery()
	initial := sum(t, r, q)
	before := r.Diagnostics()
	if !hasWarning(initial, "oversized-jsonl-lines:1") {
		t.Fatalf("warnings %v", initial.Warnings)
	}
	rewriteThenMove(t, f, original, changed)
	refreshed := sum(t, r, q)
	after := r.Diagnostics()
	if hasWarning(refreshed, "oversized-jsonl-lines:1") {
		t.Fatalf("stale warning %v", refreshed.Warnings)
	}
	eq(t, "rebuilt", after.RebuiltFiles, before.RebuiltFiles+1)
	eq(t, "adopted", after.AdoptedFiles, before.AdoptedFiles)
}

func TestArchiveMoveAndIdenticalCopyAreDeduplicated(t *testing.T) {
	f := newFixture(t)
	contents := tokenLine("2026-06-29T01:00:00Z", 100) + "\n"
	original := f.write(contents, "rollout-move.jsonl")
	id := fileID(t, original)
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	beforeMove := r.Diagnostics()

	archived := f.archivedPath("rollout-move.jsonl")
	moveFile(t, original, archived)
	eq(t, "id", fileID(t, archived), id)
	eq(t, "moved tokens", sum(t, r, q).Totals.TotalTokens, 105)
	afterMove := r.Diagnostics()
	eq(t, "bytes", afterMove.BytesRead, beforeMove.BytesRead)
	eq(t, "adopted", afterMove.AdoptedFiles, beforeMove.AdoptedFiles+1)

	copied := f.sessionPath("rollout-move.jsonl")
	if err := os.WriteFile(copied, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if fileID(t, copied) == id {
		t.Fatal("copy must have a different identity")
	}
	eq(t, "dedup tokens", sum(t, r, q).Totals.TotalTokens, 105)
	eq(t, "duplicates", r.Diagnostics().DuplicateFiles, afterMove.DuplicateFiles+1)

	restarted := f.repo()
	eq(t, "persisted warm", sum(t, restarted, q).Totals.TotalTokens, 105)
	pd := restarted.Diagnostics()
	eq(t, "persisted bytes", pd.BytesRead, 0)
	eq(t, "persisted validation", pd.ValidationBytesRead, 0)

	if err := os.Remove(archived); err != nil {
		t.Fatal(err)
	}
	eq(t, "adopted copy", sum(t, restarted, q).Totals.TotalTokens, 105)
	ad := restarted.Diagnostics()
	eq(t, "adopted", ad.AdoptedFiles, pd.AdoptedFiles+1)
	eq(t, "rebuilt", ad.RebuiltFiles, pd.RebuiltFiles)
	eq(t, "bytes", ad.BytesRead, pd.BytesRead)
	eq(t, "validation", ad.ValidationBytesRead, pd.ValidationBytesRead)
}

func TestRewrittenMoveIsNotBlindlyAdopted(t *testing.T) {
	f := newFixture(t)
	pad := func(c string) string {
		return `{"type":"event_msg","payload":{"type":"message","content":"` + strings.Repeat(c, 20*1024) + `"}}` + "\n"
	}
	prefix, suffix := pad("p"), pad("s")
	orig := prefix + tokenLine("2026-06-29T01:00:00Z", 100) + "\n" + suffix
	changed := prefix + tokenLine("2026-06-29T01:00:00Z", 900) + "\n" + suffix
	eq(t, "sizes", len(orig), len(changed))
	original := f.write(orig, "rollout-rewritten-move.jsonl")
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()
	rewriteThenMove(t, f, original, changed)
	s := sum(t, r, q)
	after := r.Diagnostics()
	eq(t, "tokens", s.Totals.TotalTokens, 905)
	eq(t, "rebuilt", after.RebuiltFiles, before.RebuiltFiles+1)
	eq(t, "adopted", after.AdoptedFiles, before.AdoptedFiles)
}

func TestChangedDuplicateReportsConflict(t *testing.T) {
	f := newFixture(t)
	orig := tokenLine("2026-06-29T01:00:00Z", 100) + "\n"
	changed := tokenLine("2026-06-29T01:00:00Z", 900) + "\n"
	eq(t, "sizes", len(orig), len(changed))
	f.write(orig, "rollout-cached-conflict.jsonl")
	copyPath := f.archivedPath("rollout-cached-conflict.jsonl")
	if err := os.WriteFile(copyPath, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()
	replaceKeepingIdentity(t, copyPath, changed)
	_, err := r.Summaries(context.Background(), []Query{q}, fixedNow, RefreshIfChanged, nil)
	var ce *DuplicateConflictError
	if !errors.As(err, &ce) || ce.Basename != "rollout-cached-conflict.jsonl" {
		t.Fatalf("got %v", err)
	}
	eq(t, "validation", r.Diagnostics().ValidationBytesRead-before.ValidationBytesRead, len(changed))
}

func TestDivergentDuplicateFailsExplicitly(t *testing.T) {
	f := newFixture(t)
	const base = "rollout-conflict.jsonl"
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", base)
	if err := os.WriteFile(f.archivedPath(base), []byte(tokenLine("2026-06-29T01:00:00Z", 900)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := f.repo().Summaries(context.Background(), []Query{fullQuery()}, fixedNow, RefreshIfChanged, nil)
	var ce *DuplicateConflictError
	if !errors.As(err, &ce) || ce.Basename != base {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), f.root) {
		t.Fatalf("error leaks local path: %v", err)
	}
}

func TestSymbolicLinksAreNotIndexed(t *testing.T) {
	f := newFixture(t)
	outside := f.root + "/outside.jsonl"
	if err := os.WriteFile(outside, []byte(tokenLine("2026-06-29T01:00:00Z", 100)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, f.sessionPath("rollout-link.jsonl")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	r := f.repo()
	s := sum(t, r, fullQuery())
	eq(t, "source", s.Source, SourceUnavailable)
	d := r.Diagnostics()
	eq(t, "bytes", d.BytesRead, 0)
	eq(t, "rebuilt", d.RebuiltFiles, 0)
}

// --- concurrency ------------------------------------------------------------

func TestConcurrentCallersShareOneScan(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-concurrent.jsonl")
	g := newGate()
	r := f.repo(withGate(g.wait))
	q := fullQuery()
	type res struct {
		tokens int64
		err    error
	}
	ch := make(chan res, 6)
	for i := 0; i < 6; i++ {
		go func() {
			m, err := r.Summaries(context.Background(), []Query{q}, fixedNow, RefreshForce, nil)
			ch <- res{m[q.ID].Totals.TotalTokens, err}
		}()
	}
	eventually(t, "five joiners", func() bool { return r.Diagnostics().SharedFlightHits == 5 })
	g.open()
	for i := 0; i < 6; i++ {
		x := <-ch
		if x.err != nil || x.tokens != 105 {
			t.Fatalf("result %+v", x)
		}
	}
	d := r.Diagnostics()
	eq(t, "passes", d.IndexPasses, 1)
	eq(t, "shared", d.SharedFlightHits, 5)
	eq(t, "max scans", d.MaxConcurrentScans, 1)
}

func TestCancelledJoinerDoesNotCancelSharedScan(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-cancelled-joiner.jsonl")
	g := newGate()
	r := f.repo(withGate(g.wait))
	q := fullQuery()
	ownerCh := make(chan Summary, 1)
	go func() {
		m, _ := r.Summaries(context.Background(), []Query{q}, fixedNow, RefreshIfChanged, nil)
		ownerCh <- m[q.ID]
	}()
	eventually(t, "owner parked", func() bool { return g.waitingCount() == 1 })
	ctx, cancel := context.WithCancel(context.Background())
	joinErr := make(chan error, 1)
	go func() {
		_, err := r.Summaries(ctx, []Query{q}, fixedNow, RefreshIfChanged, nil)
		joinErr <- err
	}()
	eventually(t, "joiner", func() bool { return r.Diagnostics().SharedFlightHits == 1 })
	cancel()
	if err := <-joinErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("joiner err %v", err)
	}
	g.open()
	eq(t, "owner tokens", (<-ownerCh).Totals.TotalTokens, 105)
	eq(t, "passes", r.Diagnostics().IndexPasses, 1)
}

func TestCancellingOnlyWaiterCancelsPhysicalScan(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-cancelled-owner.jsonl")
	g := newGate()
	r := f.repo(withGate(g.wait))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := r.Summaries(ctx, []Query{fullQuery()}, fixedNow, RefreshIfChanged, nil)
		errCh <- err
	}()
	eventually(t, "parked", func() bool { return g.waitingCount() == 1 })
	cancel()
	eventually(t, "flight cancelled", func() bool { return r.Diagnostics().CancelledFlights == 1 })
	g.open()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	d := r.Diagnostics()
	eq(t, "cancelled", d.CancelledFlights, 1)
	eq(t, "passes", d.IndexPasses, 0)
}

func TestResultsAreIndependentCopies(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-copy.jsonl")
	r := f.repo()
	a := sum(t, r, fullQuery())
	a.DailyRows[0].Totals.TotalTokens = -1
	a.Warnings = append(a.Warnings, "x")
	b := sum(t, r, fullQuery())
	eq(t, "tokens", b.DailyRows[0].Totals.TotalTokens, 105)
}

// --- corruption -------------------------------------------------------------

func TestCorruptDatabaseIsRebuilt(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-corrupt-db.jsonl")
	if err := os.WriteFile(f.dbPath, []byte("not-a-sqlite-database"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.repo()
	s := sum(t, r, fullQuery())
	d := r.Diagnostics()
	eq(t, "tokens", s.Totals.TotalTokens, 105)
	eq(t, "dbRebuilds", d.DatabaseRebuilds, 1)
	eq(t, "rebuilt", d.RebuiltFiles, 1)
}

func TestRuntimeDatabaseCorruptionRebuildsOnce(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-runtime-corrupt.jsonl")
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()
	execSQL(t, f.dbPath, "DROP TABLE usage_events")
	s := sum(t, r, q)
	after := r.Diagnostics()
	eq(t, "tokens", s.Totals.TotalTokens, 105)
	eq(t, "dbRebuilds", after.DatabaseRebuilds, before.DatabaseRebuilds+1)
	eq(t, "rebuilt", after.RebuiltFiles, before.RebuiltFiles+1)
}

func TestInvalidStorageClassesRebuild(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-storage-class.jsonl")
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	prev := r.Diagnostics().DatabaseRebuilds
	for _, stmt := range []string{
		"UPDATE source_files SET device = 'not-an-integer'",
		"UPDATE usage_events SET uncached_input_tokens = 'not-an-integer'",
		"UPDATE usage_events SET timestamp = 'not-a-number'",
		"UPDATE usage_events SET timestamp = 1e999",
		"UPDATE source_files SET malformed_lines = 9223372036854775807",
		"UPDATE source_files SET full_hash = NULL",
	} {
		execSQL(t, f.dbPath, stmt)
		s := sum(t, r, q)
		eq(t, stmt+" tokens", s.Totals.TotalTokens, 105)
		now := r.Diagnostics().DatabaseRebuilds
		eq(t, stmt+" rebuilds", now, prev+1)
		prev = now
	}
}

func TestAggregateOverflowDoesNotRebuild(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100)+"\n", "rollout-query-error.jsonl")
	r := f.repo()
	q := fullQuery()
	sum(t, r, q)
	before := r.Diagnostics()
	execSQL(t, f.dbPath, "UPDATE usage_events SET uncached_input_tokens = 9223372036854775807")
	execSQL(t, f.dbPath, `INSERT INTO usage_events
		(file_id, byte_offset, timestamp, day_key, model, project,
		 uncached_input_tokens, cached_input_tokens, output_tokens, turns)
		SELECT file_id, byte_offset + 1, timestamp, day_key, model, project,
		       9223372036854775807, 0, 0, 0
		FROM usage_events LIMIT 1`)
	_, err := r.Summaries(context.Background(), []Query{q}, fixedNow, RefreshIfChanged, nil)
	if err == nil {
		t.Fatal("expected aggregate overflow")
	}
	if !strings.Contains(err.Error(), "integer overflow") {
		t.Fatalf("expected explicit aggregate overflow: %v", err)
	}
	eq(t, "rebuilds", r.Diagnostics().DatabaseRebuilds, before.DatabaseRebuilds)
	if _, err := os.Stat(f.dbPath); err != nil {
		t.Fatal(err)
	}
}
