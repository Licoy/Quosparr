package usage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/usage/decimal"
)

func sampleSummary() Summary {
	tot := Totals{TotalTokens: 1000, UncachedInputTokens: 700, CachedInputTokens: 200, OutputTokens: 100, Turns: 3, Threads: 1}
	usd := decimal.MustParse("1.25")
	yes, no := true, false
	return Summary{
		Source: SourceLocalSessions, Confidence: ConfidencePriced,
		Window:       Window{Start: mustTime("2026-06-24T00:00:00Z"), End: mustTime("2026-07-01T00:00:00Z")},
		EstimatedUSD: &usd, Totals: tot,
		DailyRows:   []DailyRow{{Date: "2026-06-29", Totals: tot, EstimatedUSD: &usd, CreditsReported: &yes, TotalsReported: &no}},
		ModelRows:   []BreakdownRow{{Name: "gpt-5.5", Totals: tot, EstimatedUSD: &usd}},
		ProjectRows: []BreakdownRow{{Name: "quosparr/x", Totals: tot, EstimatedUSD: &usd, RawCredits: 2.5}},
		ClientRows:  []BreakdownRow{}, RawCredits: 0.125, Warnings: []string{},
		PricingVersion: PricingVersion, CalculatedAt: mustTime("2026-06-30T10:00:00Z"),
	}
}

func TestSummaryRoundTrip(t *testing.T) {
	want := sampleSummary()
	data, err := EncodeSummary(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeSummary(data)
	if err != nil {
		t.Fatal(err)
	}
	if !summariesEqual(got, want) {
		t.Fatalf("round trip differs:\n got %+v\nwant %+v", got, want)
	}
	if !strings.Contains(string(data), `"window":{"start":"2026-06-24T00:00:00Z","duration":604800}`) {
		t.Fatalf("window encoding: %s", data)
	}
}

// summariesEqual compares Decimals by value and times by instant.
func summariesEqual(a, b Summary) bool {
	norm := func(s Summary) Summary {
		s = s.Clone()
		s.Window.Start, s.Window.End, s.CalculatedAt = s.Window.Start.UTC(), s.Window.End.UTC(), s.CalculatedAt.UTC()
		return s
	}
	return reflect.DeepEqual(norm(a), norm(b))
}

func TestSummaryDecodesSwiftCacheFile(t *testing.T) {
	data, err := os.ReadFile("testdata/summary_cache.json")
	if err != nil {
		t.Fatal(err)
	}
	s, err := DecodeSummary(data)
	if err != nil {
		t.Fatal(err)
	}
	if s.Source != SourceLocalSessions || s.Confidence != ConfidencePriced || s.PricingVersion != "openai-builtin-2026-09-27" ||
		!s.CalculatedAt.Equal(mustTime("2026-06-30T12:00:00Z")) ||
		!s.Window.Start.Equal(mustTime("2026-06-29T00:00:00Z")) || !s.Window.End.Equal(mustTime("2026-06-30T00:00:00Z")) {
		t.Fatalf("header: %+v", s)
	}
	if s.EstimatedUSD != nil {
		// The Swift file omits the overall estimate? Then it must be nil.
		t.Logf("estimate %s", s.EstimatedUSD)
	}
	if len(s.ModelRows) != 2 || s.ModelRows[0].Name != "gpt-5.6-sol" || s.ModelRows[0].EstimatedUSD.String() != "4.00004" ||
		s.ModelRows[1].EstimatedUSD != nil || s.ModelRows[0].Totals.TotalTokens != 1000105 {
		t.Fatalf("models: %+v", s.ModelRows)
	}
	if len(s.ProjectRows) != 1 || s.ProjectRows[0].Name != "quosparr" ||
		len(s.Warnings) != 2 || s.Warnings[1] != "malformed-jsonl-lines:1" || len(s.DailyRows) != 1 {
		t.Fatalf("rest: %+v", s)
	}
	if s.DailyRows[0].CreditsReported != nil || s.DailyRows[0].TotalsReported != nil {
		t.Fatal("legacy reported flags must stay nil")
	}
	// Re-encoding and decoding again preserves meaning.
	again, _ := EncodeSummary(s)
	back, err := DecodeSummary(again)
	if err != nil || !summariesEqual(back, s) {
		t.Fatalf("re-round-trip: %v", err)
	}
}

func TestDecodeSummaryRejects(t *testing.T) {
	good, _ := EncodeSummary(sampleSummary())
	for name, mut := range map[string]func(string) string{
		"missing source":    func(s string) string { return strings.Replace(s, `"source":"localSessions",`, ``, 1) },
		"bad source":        func(s string) string { return strings.Replace(s, `"localSessions"`, `"other"`, 1) },
		"bad confidence":    func(s string) string { return strings.Replace(s, `"priced"`, `"exact"`, 1) },
		"fractional tokens": func(s string) string { return strings.Replace(s, `"totalTokens":1000`, `"totalTokens":1000.5`, 1) },
		"string tokens":     func(s string) string { return strings.Replace(s, `"turns":3`, `"turns":"3"`, 1) },
		"fractional date": func(s string) string {
			return strings.Replace(s, `"calculatedAt":"2026-06-30T10:00:00Z"`, `"calculatedAt":"2026-06-30T10:00:00.5Z"`, 1)
		},
		"negative duration": func(s string) string { return strings.Replace(s, `"duration":604800`, `"duration":-1`, 1) },
		"trailing":          func(s string) string { return s + "x" },
	} {
		if _, err := DecodeSummary([]byte(mut(string(good)))); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	// projectRows and estimatedUSD are optional.
	s := strings.Replace(string(good), `"projectRows":[{"name":"quosparr\/x","totals":{"totalTokens":1000,"uncachedInputTokens":700,"cachedInputTokens":200,"outputTokens":100,"turns":3,"threads":1},"estimatedUSD":1.25,"rawCredits":2.5}],`, ``, 1)
	if s == string(good) {
		t.Fatal("test mutation did not apply")
	}
	got, err := DecodeSummary([]byte(s))
	if err != nil || got.ProjectRows == nil || len(got.ProjectRows) != 0 {
		t.Fatalf("optional projectRows: %v %+v", err, got.ProjectRows)
	}
}

func TestEncodeSummaryRejectsNonFiniteCredits(t *testing.T) {
	s := sampleSummary()
	s.RawCredits = inf()
	if _, err := EncodeSummary(s); err == nil {
		t.Fatal("want error for infinite credits")
	}
}

func inf() float64 { var z float64; return 1 / z }

func TestCacheStoreStoresAndLoads(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), "nested", SummaryCacheFileName)}
	if got := store.Load(); got.State != CacheMissing {
		t.Fatalf("state = %v", got.State)
	}
	want := sampleSummary()
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got := store.Load()
	if got.State != CacheLoaded || !summariesEqual(got.Summary, want) {
		t.Fatalf("load = %+v", got)
	}
	st, _ := os.Stat(store.Path)
	if runtimeUnix() && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", st.Mode().Perm())
	}
}

func TestCacheStoreRejectsStalePricingVersion(t *testing.T) {
	for _, v := range []string{"stale-pricing-version", "openai-builtin-2026-08-13", "openai-builtin-2026-09-11", "openai-builtin-2026-09-26"} {
		store := CacheStore{Path: filepath.Join(t.TempDir(), SummaryCacheFileName)}
		s := sampleSummary()
		s.PricingVersion = v
		if err := store.Save(s); err != nil {
			t.Fatal(err)
		}
		if got := store.Load(); got.State != CacheIncompatible || got.Reason != "stale-pricing" {
			t.Errorf("%s: %+v", v, got)
		}
	}
}

func TestCacheStoreAcceptsOfficialPricingVersion(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), SummaryCacheFileName)}
	s := sampleSummary()
	s.PricingVersion = "openai-docs-official-etag"
	s.CalculatedAt = time.Unix(60, 0)
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	if got := store.Load(); got.State != CacheLoaded || !summariesEqual(got.Summary, s) {
		t.Fatalf("%+v", got)
	}
}

func TestCacheStoreIgnoresCorruptFile(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), SummaryCacheFileName)}
	if err := os.WriteFile(store.Path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := store.Load(); got.State != CacheIncompatible || got.Reason != "corrupt" {
		t.Fatalf("%+v", got)
	}
	// A later Save replaces the corrupt file.
	if err := store.Save(sampleSummary()); err != nil || store.Load().State != CacheLoaded {
		t.Fatalf("save over corrupt: %v", err)
	}
}

func runtimeUnix() bool { return os.PathSeparator == '/' }

func TestLegacyMixedCacheRequiresCoverageRebuild(t *testing.T) {
	store := CacheStore{Path: filepath.Join(t.TempDir(), SummaryCacheFileName)}
	s := sampleSummary()
	s.Warnings = []string{"unknown-model:new-model"}
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	if got := store.Load(); got.State != CacheIncompatible || got.Reason != "stale-coverage" {
		t.Fatalf("unsafe old cache %+v", got)
	}
	// Already covered new partial summaries and unrelated warning-only caches remain readable.
	s.Confidence = ConfidencePartiallyPriced
	s.UnpricedTokens = 500
	s.DailyRows[0].UnpricedTokens = 500
	s.ModelRows[0].UnpricedTokens = 500
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	if got := store.Load(); got.State != CacheLoaded || got.Summary.UnpricedTokens != 500 {
		t.Fatalf("new cache %+v", got)
	}
	s = sampleSummary()
	s.Warnings = []string{"oversized-jsonl-lines:1"}
	if err := store.Save(s); err != nil {
		t.Fatal(err)
	}
	if got := store.Load(); got.State != CacheLoaded {
		t.Fatalf("unrelated warning %+v", got)
	}
}
