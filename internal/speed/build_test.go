package speed

import (
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func ttft(at time.Time, model, effort string, d time.Duration) Sample {
	return Sample{At: at, Model: model, Effort: effort, TTFT: d, HasTTFT: true}
}

func output(at time.Time, model, effort string, tokens int64, decode time.Duration) Sample {
	return Sample{At: at, Model: model, Effort: effort, OutputTokens: tokens, Decode: decode}
}

func TestBuildEmpty(t *testing.T) {
	r := Build(nil, testNow)
	if !r.Empty() || len(r.Sessions) != 0 || !r.CalculatedAt.Equal(testNow) {
		t.Fatalf("empty report = %+v", r)
	}
	if !r.From.IsZero() || !r.To.IsZero() {
		t.Fatalf("empty report has a range: %v - %v", r.From, r.To)
	}
}

func TestBuildDropsUnusableSamples(t *testing.T) {
	at := testNow.Add(-time.Hour)
	r := Build([]Session{{ID: "s", Samples: []Sample{
		ttft(at, "", "high", time.Second),                            // no model
		ttft(testNow.Add(-Window-time.Second), "m", "", time.Second), // too old
		ttft(testNow.Add(time.Second), "m", "", time.Second),         // in the future
		{At: at, Model: "m", Effort: "high"},                         // nothing measured
		{At: at, Model: "m", HasTTFT: true, TTFT: -time.Second},      // negative wait
		{At: at, Model: "m", OutputTokens: 100},                      // tokens without time
		{At: at, Model: "m", Decode: time.Second},                    // time without tokens
		ttft(testNow.Add(-Window), "edge", "", time.Second),          // exactly at the window start
		ttft(testNow, "edge2", "", time.Second),                      // exactly now
	}}}, testNow)
	if len(r.Groups) != 2 || r.Groups[0].Model != "edge" || r.Groups[1].Model != "edge2" {
		t.Fatalf("groups = %+v", r.Groups)
	}
}

func TestBuildGroupsByModelAndEffort(t *testing.T) {
	at := testNow.Add(-time.Hour)
	var samples []Sample
	for _, d := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		samples = append(samples, ttft(at, "gpt", "xhigh", time.Duration(d)*time.Second))
	}
	samples = append(samples,
		ttft(at, "gpt", "low", time.Second),
		output(at, "gpt", "xhigh", 100, 2*time.Second),
		output(at, "gpt", "xhigh", 200, 2*time.Second),
		output(at, "gpt", "xhigh", 0, 2*time.Second), // not counted
	)
	r := Build([]Session{{ID: "s", Samples: samples}}, testNow)
	if len(r.Groups) != 2 {
		t.Fatalf("groups = %+v", r.Groups)
	}
	g := r.Groups[0]
	if g.Model != "gpt" || g.Effort != "xhigh" || g.Calls != 10 {
		t.Fatalf("group = %+v", g)
	}
	if g.TTFTP50 != 5*time.Second || g.TTFTP90 != 9*time.Second {
		t.Fatalf("p50 = %v, p90 = %v", g.TTFTP50, g.TTFTP90)
	}
	if g.OutputTokens != 300 || g.Decode != 4*time.Second || !g.HasTPS() || g.TPS() != 75 {
		t.Fatalf("output = %d tokens in %v", g.OutputTokens, g.Decode)
	}
	if low := r.Groups[1]; low.Effort != "low" || low.Calls != 1 || low.HasTPS() {
		t.Fatalf("low = %+v", low)
	}
}

func TestPercentileNearestRank(t *testing.T) {
	ms := func(vals ...int) []time.Duration {
		out := make([]time.Duration, len(vals))
		for i, v := range vals {
			out[i] = time.Duration(v) * time.Millisecond
		}
		return out
	}
	for _, c := range []struct {
		in      []time.Duration
		p       int
		want    time.Duration
		comment string
	}{
		{nil, 50, 0, "none"},
		{ms(7), 50, 7 * time.Millisecond, "single"},
		{ms(7), 90, 7 * time.Millisecond, "single p90"},
		{ms(1, 2), 50, 1 * time.Millisecond, "rank 1 of 2"},
		{ms(1, 2, 3, 4), 50, 2 * time.Millisecond, "rank 2 of 4"},
		{ms(1, 2, 3, 4, 5), 50, 3 * time.Millisecond, "rank 3 of 5"},
		{ms(1, 2, 3, 4, 5), 90, 5 * time.Millisecond, "rank 5 of 5"},
	} {
		if got := percentile(c.in, c.p); got != c.want {
			t.Errorf("%s: percentile(%v, %d) = %v, want %v", c.comment, c.in, c.p, got, c.want)
		}
	}
}

func TestBuildGroupOrderIsStable(t *testing.T) {
	at := testNow.Add(-time.Hour)
	samples := []Sample{
		ttft(at, "b", "", time.Second),
		ttft(at, "a", "z", time.Second),
		ttft(at, "a", "a", time.Second),
		ttft(at, "big", "", time.Second), ttft(at, "big", "", time.Second),
		output(at, "out", "", 10, time.Second), output(at, "out", "", 10, time.Second), output(at, "out", "", 10, time.Second),
	}
	for i := 0; i < 20; i++ {
		r := Build([]Session{{ID: "s", Samples: samples}}, testNow)
		var got []string
		for _, g := range r.Groups {
			got = append(got, g.Model+"/"+g.Effort)
		}
		want := []string{"out/", "big/", "a/a", "a/z", "b/"}
		if len(got) != len(want) {
			t.Fatalf("groups = %v", got)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("groups = %v, want %v", got, want)
			}
		}
	}
}

func TestBuildSessionsNewestFirstAndDropsEmpty(t *testing.T) {
	at := testNow.Add(-time.Hour)
	older := Session{ID: "older", Title: "o", UpdatedAt: testNow.Add(-3 * time.Hour), Samples: []Sample{ttft(at.Add(-time.Hour), "m", "", time.Second)}}
	newer := Session{ID: "newer", Title: "n", ProjectName: "p", UpdatedAt: testNow.Add(-time.Hour), Samples: []Sample{
		ttft(at, "m", "", 3*time.Second), output(at, "m", "", 50, time.Second),
	}}
	empty := Session{ID: "empty", UpdatedAt: testNow, Samples: []Sample{ttft(at, "", "", time.Second)}}
	r := Build([]Session{older, empty, newer}, testNow)
	if len(r.Sessions) != 2 || r.Sessions[0].ID != "newer" || r.Sessions[1].ID != "older" {
		t.Fatalf("sessions = %+v", r.Sessions)
	}
	s := r.Sessions[0]
	if s.Title != "n" || s.ProjectName != "p" || len(s.Groups) != 1 || s.Groups[0].Calls != 1 || s.Groups[0].TPS() != 50 {
		t.Fatalf("newer = %+v", s)
	}
	if r.Groups[0].Calls != 2 {
		t.Fatalf("merged calls = %d, want 2", r.Groups[0].Calls)
	}
	if !r.From.Equal(at.Add(-time.Hour)) || !r.To.Equal(at) {
		t.Fatalf("range = %v - %v", r.From, r.To)
	}
	if !r.CalculatedAt.Equal(testNow) {
		t.Fatalf("calculated at %v", r.CalculatedAt)
	}
}

func TestStatWithoutOutputHasNoTPS(t *testing.T) {
	if (Stat{}).HasTPS() || (Stat{}).TPS() != 0 {
		t.Fatal("zero stat rates output")
	}
	if (Stat{OutputTokens: 10}).HasTPS() || (Stat{Decode: time.Second}).HasTPS() {
		t.Fatal("half an output part rates")
	}
}

func tiered(s Sample, tier Tier) Sample {
	s.Tier = tier
	return s
}

func TestBuildIgnoresTiersWithoutFast(t *testing.T) {
	at := testNow.Add(-time.Hour)
	r := Build([]Session{{ID: "s", Samples: []Sample{
		tiered(ttft(at, "m", "high", time.Second), TierStandard),
		ttft(at, "m", "high", 2*time.Second),
		// A fast sample outside the window does not split the tiers.
		tiered(ttft(testNow.Add(-Window-time.Second), "m", "high", time.Second), TierFast),
	}}}, testNow)
	if r.Tiered || len(r.Groups) != 1 || r.Groups[0].Calls != 2 || r.Groups[0].Tier != TierUnknown {
		t.Fatalf("report = %+v", r)
	}
	if g := r.Sessions[0].Groups; len(g) != 1 || g[0].Tier != TierUnknown {
		t.Fatalf("session groups = %+v", g)
	}
}

func TestBuildSplitsTiersNextToEachOther(t *testing.T) {
	at := testNow.Add(-time.Hour)
	var samples []Sample
	for i := 0; i < 3; i++ {
		samples = append(samples, tiered(ttft(at, "big", "high", time.Second), TierFast))
	}
	samples = append(samples,
		ttft(at, "big", "high", 4*time.Second),
		tiered(ttft(at, "big", "high", 3*time.Second), TierStandard),
		tiered(output(at, "big", "high", 100, time.Second), TierFast),
		tiered(ttft(at, "big", "high", time.Second), TierUltrafast),
	)
	for i := 0; i < 5; i++ {
		samples = append(samples, tiered(ttft(at, "mid", "", time.Second), TierStandard))
	}
	r := Build([]Session{{ID: "s", Samples: samples}}, testNow)
	if !r.Tiered {
		t.Fatal("report with a fast sample is not tiered")
	}
	var got []string
	for _, g := range r.Groups {
		got = append(got, g.Model+"/"+string(g.Tier))
	}
	// big leads by total samples even though each of its tiers has fewer
	// samples than mid.
	want := []string{"big/standard", "big/fast", "big/ultrafast", "big/", "mid/standard"}
	if len(got) != len(want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("groups = %v, want %v", got, want)
		}
	}
	if fast := r.Groups[1]; fast.Calls != 3 || fast.TPS() != 100 {
		t.Fatalf("fast = %+v", fast)
	}
	if g := r.Sessions[0].Groups; len(g) != 5 || g[1].Tier != TierFast || g[2].Tier != TierUltrafast {
		t.Fatalf("session groups = %+v", g)
	}
}

func TestBuildUltrafastEnablesTiersWithoutFast(t *testing.T) {
	at := testNow.Add(-time.Hour)
	r := Build([]Session{{ID: "s", Samples: []Sample{
		tiered(ttft(at, "m", "high", 2*time.Second), TierStandard),
		tiered(output(at, "m", "high", 100, time.Second), TierUltrafast),
	}}}, testNow)
	if !r.Tiered || len(r.Groups) != 2 || r.Groups[0].Tier != TierStandard ||
		r.Groups[1].Tier != TierUltrafast || r.Groups[1].TPS() != 100 {
		t.Fatalf("report = %+v", r)
	}
}
