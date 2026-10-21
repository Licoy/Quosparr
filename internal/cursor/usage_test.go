package cursor

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Encoders only construct synthetic protocol fixtures, never real account data.
func intField(k uint64, n uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, k<<3), n)
}
func msgField(k uint64, data []byte) []byte {
	b := binary.AppendUvarint(nil, k<<3|2)
	b = binary.AppendUvarint(b, uint64(len(data)))
	return append(b, data...)
}
func pctField(k uint64, n float64) []byte {
	b := binary.AppendUvarint(nil, k<<3|1)
	return binary.LittleEndian.AppendUint64(b, math.Float64bits(n))
}
func join(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

var sampleNow = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

func planFixture(weekly bool) []byte {
	period := uint64(1)
	if weekly {
		period = 2
	}
	return join(msgField(1, join(msgField(1, []byte("test plan")), intField(6, period), intField(7, uint64(sampleNow.Add(48*time.Hour).UnixMilli())))), intField(3, period-1))
}

func usageFixture(pool []byte) []byte {
	return join(intField(6, 1), msgField(3, pool), msgField(4, intField(1, 1234)))
}

func TestQuotaPoolsKeepOptionalZeroAndSeparateSpend(t *testing.T) {
	p := join(pctField(12, 0), pctField(13, 75.5), intField(9, 1510), intField(11, 2000))
	q, err := ParseUsage(planFixture(false), usageFixture(p), sampleNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Pools) != 2 || q.Pools[0].UsedPercent == nil || *q.Pools[0].UsedPercent != 0 || q.Pools[0].UsedCents != nil {
		t.Fatal("optional zero or absent amount lost")
	}
	if *q.Pools[1].UsedPercent != 75.5 || *q.OnDemandCents != 1234 || !q.Pools[0].ResetsAt.Equal(sampleNow.Add(48*time.Hour)) {
		t.Fatal("quota or billing fields conflated")
	}
	if q.Pools[0].WindowMinutes != 30*24*60 {
		t.Fatal("monthly period missing")
	}
}

func TestWeeklyPoolUsesUsageResetAndDoesNotCombineGrants(t *testing.T) {
	body := join(usageFixture(join(pctField(12, 20), pctField(13, 40), pctField(14, 65))), intField(2, uint64(sampleNow.Add(30*24*time.Hour).UnixMilli())))
	q, err := ParseUsage(planFixture(true), body, sampleNow)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Shared || len(q.Pools) != 1 || q.Pools[0].Kind != "shared" || *q.Pools[0].UsedPercent != 65 || q.Pools[0].WindowMinutes != 10080 || !q.Pools[0].ResetsAt.Equal(sampleNow.Add(48*time.Hour)) {
		t.Fatal("shared weekly pool not preserved")
	}
	grants := join(msgField(1, join(intField(1, 100), msgField(4, []byte("model credit")))), msgField(1, join(intField(1, 200), msgField(4, []byte("promotion")))))
	credits, err := ParseCredits(grants)
	if err != nil || len(credits) != 2 || credits[0].RemainingCents != 100 || credits[1].RemainingCents != 200 {
		t.Fatal("restricted credits must stay separate")
	}
}

func TestUnknownZeroLimitAndOverage(t *testing.T) {
	for _, tc := range []struct {
		pool    []byte
		percent *float64
	}{
		{join(intField(8, 500), intField(10, 0)), nil},
		{join(intField(8, 3000), intField(10, 2000)), ptr(150)},
		{pctField(12, 0), ptr(0)},
	} {
		q, err := ParseUsage(planFixture(false), usageFixture(tc.pool), sampleNow)
		if err != nil || len(q.Pools) != 1 {
			t.Fatal("valid usage shape rejected")
		}
		p := q.Pools[0].UsedPercent
		if (p == nil) != (tc.percent == nil) || p != nil && *p != *tc.percent {
			t.Fatal("unknown limit or overage changed")
		}
	}
}

func ptr(n float64) *float64 { return &n }

func TestMalformedAndChangedProtocolDoNotBecomeFreeQuota(t *testing.T) {
	for _, body := range [][]byte{
		{}, {0}, {0xff}, {0x1a, 0xff, 0xff, 0xff},
		usageFixture(pctField(12, math.NaN())), usageFixture(pctField(12, -1)),
		usageFixture(intField(12, 1)), usageFixture(join(pctField(12, 10), pctField(12, 20))),
		usageFixture(intField(10, math.MaxUint64)),
	} {
		if _, err := ParseUsage(planFixture(false), body, sampleNow); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
	_, err := ParseUsage(planFixture(false), join(intField(6, 1), msgField(3, nil)), sampleNow)
	if !errors.Is(err, ErrResponse) {
		t.Fatal("empty usage manufactured a reading")
	}
	valid := join(usageFixture(pctField(12, 10)), msgField(100, []byte("unknown future field")))
	if _, err := ParseUsage(planFixture(false), valid, sampleNow); err != nil {
		t.Fatal(err)
	}
}

func TestClientUsesUnaryProtocolAndRejectsRedirect(t *testing.T) {
	seen := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/proto" || r.Header.Get("Connect-Protocol-Version") != "1" || r.Header.Get("Authorization") != "Bearer synthetic-test-token" || r.ContentLength > 0 {
			t.Error("incorrect unary request")
		}
		w.Header().Set("Content-Type", "application/proto")
		switch r.URL.Path {
		case "/GetPlanInfo":
			w.Write(planFixture(false))
		case "/GetCurrentPeriodUsage":
			w.Write(usageFixture(pctField(12, 25)))
		case "/GetAggregatedUsageEvents":
			w.Write(eventRow("composer-2", 2, 5000))
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	c := Client{HTTP: server.Client(), endpoint: server.URL + "/"}
	if _, err := c.Fetch(context.Background(), "synthetic-test-token"); err != nil || seen != 3 {
		t.Fatal("unary fetch failed")
	}
	leaked := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c.endpoint = redirect.URL + "/"
	if _, err := c.Fetch(context.Background(), "synthetic-test-token"); !errors.Is(err, ErrUnavailable) || leaked {
		t.Fatal("redirect followed")
	}
}

func TestClientErrorsAreBoundedAndDoNotEchoSecrets(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 200} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "120")
			w.Header().Set("Content-Type", "application/proto")
			w.WriteHeader(status)
			w.Write([]byte("synthetic-private-response"))
		}))
		c := Client{HTTP: server.Client(), endpoint: server.URL + "/"}
		_, err := c.Fetch(context.Background(), "synthetic-test-token")
		server.Close()
		if err == nil || strings.Contains(err.Error(), "synthetic") {
			t.Fatal("response or credential exposed")
		}
		if status == 429 {
			var e *RateLimitedError
			if !errors.As(err, &e) || e.RetryAfter != 120*time.Second {
				t.Fatal("Retry-After lost")
			}
		}
	}
}

// Shaped like a Team plan response: the usage page shows Cursor and other
// models separately; total spend includes free bonus usage and is not a cap.
func TestSplitPoolsWithoutTotalAndIncludedSpend(t *testing.T) {
	p := join(intField(1, 24243), intField(2, 2000), intField(3, 22243), intField(5, 2000), pctField(12, 21.3), pctField(13, 9.3), pctField(14, 19.4))
	q, err := ParseUsage(planFixture(false), usageFixture(p), sampleNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Pools) != 2 || q.Pools[0].Kind != "auto" || *q.Pools[0].UsedPercent != 21.3 || q.Pools[1].Kind != "api" || *q.Pools[1].UsedPercent != 9.3 {
		t.Fatal("model pools not separate or a total pool was added")
	}
}

func TestUnspecifiedPeriodDoesNotBorrowBillingEnd(t *testing.T) {
	plan := join(msgField(1, join(msgField(1, []byte("test plan")))))
	body := join(usageFixture(pctField(12, 20)), intField(2, uint64(sampleNow.Add(30*24*time.Hour).UnixMilli())))
	q, err := ParseUsage(plan, body, sampleNow)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Pools[0].ResetsAt.IsZero() || q.Pools[0].WindowMinutes != 0 {
		t.Fatal("unknown period inferred a reset")
	}
}

func eventRow(model string, tier uint64, cents float64) []byte {
	return msgField(1, join(msgField(1, []byte(model)), pctField(6, cents), intField(8, tier)))
}

// Shaped like a Team cycle: Cursor models at 21.318% and other models at
// 9.295% invert to the $1050 and $200 caps; bot usage is in neither pool.
func TestSpendByModelPoolInvertsCaps(t *testing.T) {
	body := join(
		eventRow("grok-4.7-high-fast", 2, 20781.3848), eventRow("gemini-4-argon-high", 1, 1816.547935),
		eventRow("cursor-grok-4.6-high", 2, 731.4268), eventRow("cursor-grok-4.6-high-fast", 2, 633.7028),
		eventRow("auto-smart", 2, 126.2776), eventRow("composer-2.5-fast", 2, 97.9793),
		eventRow("auto-smart", 1, 24.413653), eventRow("claude-opus-5-5-medium", 1, 17.724125),
		eventRow("cursor-grok-4.6-medium", 2, 13.3558), eventRow("grok-bot", 1, 900), eventRow("composer-2", 0, 0),
		intField(9, 1791439455000))
	spend, err := ParseSpend(body)
	if err != nil {
		t.Fatal(err)
	}
	if spend["auto"] != 22384 || spend["api"] != 1859 {
		t.Fatalf("spend = %v", spend)
	}
	auto, api := 21.31809523809524, 9.295
	pools := []Pool{{Kind: "auto", UsedPercent: &auto}, {Kind: "api", UsedPercent: &api}}
	ApplySpend(pools, spend)
	if *pools[0].LimitCents != 105000 || *pools[1].LimitCents != 20000 || !pools[0].LimitEstimated || *pools[1].UsedCents != 1859 {
		t.Fatal("caps not inverted")
	}
}

func TestSpendCannotInvertFullOrUnusedPool(t *testing.T) {
	full, zero, served := 100.0, 0.0, 50.0
	limit := int64(4000)
	pools := []Pool{{Kind: "auto", UsedPercent: &full}, {Kind: "api", UsedPercent: &zero}, {Kind: "x", UsedPercent: &served, LimitCents: &limit}}
	ApplySpend(pools, map[string]int64{"auto": 5000, "api": 0, "x": 100})
	if pools[0].LimitCents != nil || pools[1].LimitCents != nil || *pools[0].UsedCents != 5000 {
		t.Fatal("estimated a cap without a usable percentage")
	}
	if *pools[2].LimitCents != 4000 || pools[2].UsedCents != nil || pools[2].LimitEstimated {
		t.Fatal("server amounts were overridden")
	}
}

func TestSpendFailureKeepsServerPercentages(t *testing.T) {
	for _, status := range []int{500, 200} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/proto")
			switch r.URL.Path {
			case "/GetPlanInfo":
				w.Write(planFixture(false))
			case "/GetCurrentPeriodUsage":
				w.Write(usageFixture(pctField(12, 25)))
			default:
				w.WriteHeader(status)
				w.Write([]byte{0xff})
			}
		}))
		q, err := Client{HTTP: server.Client(), endpoint: server.URL + "/"}.Fetch(context.Background(), "synthetic-test-token")
		server.Close()
		if err != nil || len(q.Pools) != 1 || *q.Pools[0].UsedPercent != 25 || q.Pools[0].UsedCents != nil {
			t.Fatalf("status %d: spend failure dropped percentages: %v", status, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		switch r.URL.Path {
		case "/GetPlanInfo":
			w.Write(planFixture(false))
		case "/GetCurrentPeriodUsage":
			w.Write(usageFixture(pctField(12, 25)))
		default:
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	defer server.Close()
	_, err := Client{HTTP: server.Client(), endpoint: server.URL + "/"}.Fetch(context.Background(), "synthetic-test-token")
	var limited *RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatal("rate limit on spend was ignored")
	}
}

func TestSpendAcceptsRefundRows(t *testing.T) {
	spend, err := ParseSpend(join(eventRow("gemini-4-argon-high", 1, 500), eventRow("gemini-4-argon-high", 1, -120)))
	if err != nil || spend["api"] != 380 {
		t.Fatalf("spend = %v, %v", spend, err)
	}
}
