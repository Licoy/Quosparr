package usage

import (
	"context"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/usage/decimal"
	"path/filepath"
	"testing"
)

func publicBook(t *testing.T, models ...pricing.Model) PriceBook {
	t.Helper()
	s, err := pricing.NewSnapshot(models)
	if err != nil {
		t.Fatal(err)
	}
	return SnapshotPrices(func() pricing.Snapshot { return s }).PriceBook(context.Background())
}
func TestRemotePricesAndPartialConditions(t *testing.T) {
	book := publicBook(t, pricing.Model{ProviderID: "openai", ID: "online-only", Rate: pricing.Rate{Input: dp("2"), Output: dp("0")}}, pricing.Model{ProviderID: "other", ID: "cross-only", Rate: pricing.Rate{Input: dp("10"), Output: dp("10")}})
	events := []indexedEvent{
		{Model: "online-only", Project: "p", DayKey: "2026-06-29", Uncached: 1_000_000},
		{Model: "online-only", Project: "p", DayKey: "2026-06-29", Cached: 50},
		{Model: "gpt-5", Project: "p", DayKey: "2026-06-29", Uncached: 1_000_000},
		{Model: "cross-only", Project: "p", DayKey: "2026-06-29", Uncached: 100},
		{Model: "unknown-zero", Project: "p", DayKey: "2026-06-29", Turns: 1},
	}
	s, err := buildSummary(events, fullQuery().Window, fixedNow, book, nil)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "confidence", s.Confidence, ConfidencePartiallyPriced)
	eq(t, "unpriced", s.UnpricedTokens, 150)
	wantDec(t, "priced total", s.EstimatedUSD, "3.25")
	row := modelRow(s, "online-only")
	eq(t, "same model partial", row.UnpricedTokens, 50)
	wantDec(t, "model partial amount", row.EstimatedUSD, "2")
	eq(t, "daily partial", s.DailyRows[0].UnpricedTokens, 150)
	eq(t, "project partial", s.ProjectRows[0].UnpricedTokens, 150)
	raw, err := EncodeSummary(s)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSummary(raw)
	if err != nil {
		t.Fatal(err)
	}
	eqDeep(t, "codec", decoded, s)
	cloned := s.Clone()
	cloned.ModelRows[0].UnpricedTokens = 999
	eq(t, "clone", s.ModelRows[0].UnpricedTokens, 100)
}
func TestRemoteKnownFreeUnknownAndZero(t *testing.T) {
	book := publicBook(t, pricing.Model{ProviderID: "openai", ID: "free", Rate: pricing.Rate{Input: dp("0"), CacheRead: dp("0"), Output: dp("0")}})
	for _, c := range []struct {
		model      string
		tokens     int64
		confidence Confidence
		unpriced   int64
		amount     *decimal.Decimal
	}{
		{"free", 100, ConfidencePriced, 0, dp("0")}, {"unknown", 100, ConfidenceTokensOnly, 100, nil}, {"unknown", 0, ConfidenceUnavailable, 0, nil},
	} {
		s, err := buildSummary([]indexedEvent{{Model: c.model, Uncached: c.tokens}}, fullQuery().Window, fixedNow, book, nil)
		if err != nil {
			t.Fatal(err)
		}
		eq(t, c.model, s.Confidence, c.confidence)
		eq(t, "unpriced", s.UnpricedTokens, c.unpriced)
		if !optEqual(s.EstimatedUSD, c.amount) {
			t.Fatalf("amount %v want %v", s.EstimatedUSD, c.amount)
		}
	}
}
func TestTierBoundaryAndPerEventSelection(t *testing.T) {
	model := pricing.Model{ProviderID: "openai", ID: "gpt-6.1-sol", Rate: pricing.Rate{Input: dp("2"), CacheRead: dp("0.1"), Output: dp("10")}, Tiers: []pricing.Tier{{Type: "context", Size: 272000, Rate: pricing.Rate{Input: dp("4"), CacheRead: dp("0.2"), Output: dp("15")}}}}
	book := publicBook(t, model)
	for _, c := range []struct {
		input int64
		cost  string
	}{{271999, "0.543998"}, {272000, "0.544"}, {272001, "1.088004"}} {
		usd, ok := book.Cost(model.ID, Totals{UncachedInputTokens: c.input})
		if !ok || usd.String() != c.cost {
			t.Fatalf("%d: %s %v", c.input, usd, ok)
		}
	}
	f := newFixture(t)
	f.write(lines(tokenLine("2026-06-29T01:00:00Z", 200000, outputTokens(0), modelName(model.ID)), tokenLine("2026-06-29T02:00:00Z", 200000, outputTokens(0), modelName(model.ID))), "rollout-tiers.jsonl")
	r := f.repo(withBook(book))
	s := sum(t, r, fullQuery())
	wantDec(t, "separate requests below threshold", s.EstimatedUSD, "0.8")
	before := r.Diagnostics()
	model.Rate.Input = dp("3")
	changed := publicBook(t, model)
	r.prices = FixedPrices(changed)
	s = sum(t, r, fullQuery())
	wantDec(t, "new rate", s.EstimatedUSD, "1.2")
	after := r.Diagnostics()
	eq(t, "price update read no log bytes", after.BytesRead, before.BytesRead)
	eq(t, "price update validates no logs", after.ValidationBytesRead, before.ValidationBytesRead)
}
func TestUnknownTierBoundaryNotAssumed(t *testing.T) {
	book := publicBook(t, pricing.Model{ProviderID: "openai", ID: "unknown-tier", Rate: pricing.Rate{Input: dp("1")}, Tiers: []pricing.Tier{{Type: "context", Size: 200000, Rate: pricing.Rate{Input: dp("2")}}}})
	if _, ok := book.Cost("unknown-tier", Totals{UncachedInputTokens: 200000}); ok {
		t.Fatal("assumed gt/gte boundary")
	}
	if c, ok := book.Cost("unknown-tier", Totals{UncachedInputTokens: 200001}); !ok || c.String() != "0.400002" {
		t.Fatal("unambiguous higher tier not selected", c, ok)
	}
	if _, ok := book.Cost("unknown-tier", Totals{UncachedInputTokens: 199999}); !ok {
		t.Fatal("unambiguous standard rate missing")
	}
}

func TestIncompleteRateFallbackIsWholeApplicableGroup(t *testing.T) {
	model := pricing.Model{ProviderID: "openai", ID: "gpt-6-sol", Rate: pricing.Rate{Input: dp("99"), Output: dp("99")}, Tiers: []pricing.Tier{{Type: "context", Size: 272000, Rate: pricing.Rate{Input: dp("199")}}}}
	book := publicBook(t, model)
	c, ok, source := book.CostWithSource(model.ID, Totals{UncachedInputTokens: 100, CachedInputTokens: 100})
	if !ok || source != "builtin" || c.String() != "0.00022" {
		t.Fatalf("mixed rate groups: %s %v %s", c, ok, source)
	}
	if _, ok, source := book.CostWithSource(model.ID, Totals{UncachedInputTokens: 272001, OutputTokens: 1}); ok || source != "missing-long-rule" {
		t.Fatal("unverified builtin long boundary accepted")
	}
	s, err := buildSummary([]indexedEvent{{Model: model.ID, Uncached: 272001, Output: 1}}, fullQuery().Window, fixedNow, book, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(s, "missing-long-rule:"+model.ID) || s.UnpricedTokens != 272002 {
		t.Fatalf("missing reason %+v", s)
	}
}

func TestSourcePathQueryFiltersEventsAndRequestIdentity(t *testing.T) {
	f := newFixture(t)
	a := f.write(tokenLine("2026-06-29T01:00:00Z", 100, outputTokens(0))+"\n", "rollout-path-a.jsonl")
	b := f.write(tokenLine("2026-06-29T01:00:00Z", 200, outputTokens(0))+"\n", "rollout-path-b.jsonl")
	a, err := filepath.EvalSymlinks(a)
	if err != nil {
		t.Fatal(err)
	}
	b, err = filepath.EvalSymlinks(b)
	if err != nil {
		t.Fatal(err)
	}
	r := f.repo()
	q := fullQuery()
	q.SourcePath = a
	first := sum(t, r, q)
	eq(t, "source a", first.Totals.TotalTokens, 100)
	q.SourcePath = b
	second := sum(t, r, q)
	eq(t, "source b", second.Totals.TotalTokens, 200)
	q.SourcePath = "' OR 1=1 --"
	empty := sum(t, r, q)
	eq(t, "bound source path", empty.Totals.TotalTokens, 0)
	aQuery, bQuery := fullQuery(), fullQuery()
	aQuery.SourcePath = a
	bQuery.SourcePath = b
	if requestKey([]Query{aQuery}, fixedNow, RefreshIfChanged, "v") == requestKey([]Query{bQuery}, fixedNow, RefreshIfChanged, "v") {
		t.Fatal("source absent from identity")
	}
}

func TestMissingRemoteRateRuleCannotReuseBuiltinPriceCache(t *testing.T) {
	f := newFixture(t)
	f.write(tokenLine("2026-06-29T01:00:00Z", 100, outputTokens(0), modelName("gpt-5"))+"\n", "rollout-rule-change.jsonl")
	initial, err := pricing.Parse([]byte(`{"other":{"models":{"priced":{"cost":{"input":1}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := initial
	source := SnapshotPrices(func() pricing.Snapshot { return snapshot })
	r := f.repo(func(o *repoOptions) { o.prices = source })
	first := sum(t, r, fullQuery())
	eq(t, "initial builtin confidence", first.Confidence, ConfidencePriced)
	before := r.Diagnostics()
	snapshot, err = pricing.Parse([]byte(`{"other":{"models":{"priced":{"cost":{"input":1}}}},"openai":{"models":{"gpt-5":{"cost":{"context_over_200k":{}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	second := sum(t, r, fullQuery())
	eq(t, "changed rule confidence", second.Confidence, ConfidenceTokensOnly)
	eq(t, "changed rule unpriced", second.UnpricedTokens, 100)
	if first.PricingVersion == second.PricingVersion || !hasWarning(second, "ambiguous-tier:gpt-5") {
		t.Fatalf("old query reused %+v", second)
	}
	eq(t, "repriced without rereading logs", r.Diagnostics().BytesRead, before.BytesRead)
}
