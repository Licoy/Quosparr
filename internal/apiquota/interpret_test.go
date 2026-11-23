package apiquota

import (
	"testing"
	"time"
)

func tokenHit(remaining, used, total float64, unit string, unlimited bool) hit {
	return hit{kind: hitToken, token: tokenUsage{
		remaining: &remaining, used: &used, total: &total, unit: unit, unlimited: unlimited, hasDisplay: true,
	}}
}

func TestInconsistentFiniteTokenDataReturnsInvalidDataWithoutFallback(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"total_available":0,"total_granted":24035,
		"total_used":108886337,"unlimited_quota":false},"message":"ok"}`)
	r := round{
		urls:     billingURLs{creditGrants: "g", subscription: "s", usage: "u"},
		tokenURL: "t",
		grants:   notFound(), sub: notFound(), usage: notFound(),
		token: hit{kind: hitToken, token: tok},
	}
	r.token = r.normalizeToken()
	if r.token.kind != hitInvalidData || r.token.msg != "inconsistent quota data" {
		t.Fatalf("%+v", r.token)
	}
	q := r.interpret(testNow)
	if q.Status != StatusInvalidData || q.Error != "inconsistent quota data" || q.Endpoint != "t" {
		t.Fatalf("%+v", q)
	}
	// Invalid data is still a failure, so the origin round may improve on it.
	if !needsFallback(q) {
		t.Fatal("invalid data must allow a fallback round")
	}
}

func TestValidCreditGrantsWinWhenTokenDataIsInvalid(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"total_available":0,"total_granted":10,"total_used":11,"unlimited_quota":false}}`)
	r := round{
		urls:   billingURLs{creditGrants: "g"},
		grants: grantsHit(8, 2, 10), sub: notFound(), usage: notFound(),
		token: hit{kind: hitToken, token: tok},
	}
	r.token = r.normalizeToken()
	q := r.interpret(testNow)
	if q.Source != SourceCreditGrants || q.Status != StatusAvailable {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "remaining", q.Remaining, 8)
}

func TestInvalidDataStatusSerializesCamelCase(t *testing.T) {
	if got := marshal(t, StatusInvalidData); got != `"invalidData"` {
		t.Fatal(got)
	}
}

func TestTokenUsageWinsOverDummyUnlimitedSubscription(t *testing.T) {
	q := interpret(notFound(), subHit(fp(100_000_000), 0), usageHit(8.9686), tokenHit(999.69, 0.31, 1000, "CNY", false))
	if q.Source != SourceTokenUsage || q.Unit != "CNY" || q.Unlimited {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "remaining", q.Remaining, 999.69)
	wantFloat(t, "total", q.Total, 1000)
	if q.Endpoint != "t" {
		t.Fatal(q.Endpoint)
	}
}

func TestTokenExpiryFallsBackToSubscription(t *testing.T) {
	q := interpret(notFound(), subHit(nil, 1767225600), notFound(), tokenHit(1, 1, 2, "USD", false))
	if q.ExpiresAt == nil || !q.ExpiresAt.Equal(time.Unix(1767225600, 0)) {
		t.Fatalf("%v", q.ExpiresAt)
	}
	if q.ExpiresAt.Location() != time.UTC {
		t.Fatal("expiry not in UTC")
	}
	tok := tokenHit(1, 1, 2, "USD", false)
	tok.token.expiresAt = 1800000000
	q = interpret(notFound(), subHit(nil, 1767225600), notFound(), tok)
	if q.ExpiresAt == nil || q.ExpiresAt.Unix() != 1800000000 {
		t.Fatalf("%v", q.ExpiresAt)
	}
}

func TestGrantsWinOverSubscription(t *testing.T) {
	q := interpret(grantsHit(87.5, 12.5, 100), subHit(fp(999), 1767225600), usageHit(1), unsupported())
	if q.Source != SourceCreditGrants || q.Endpoint != "g" || q.Unit != UnitUSD {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "remaining", q.Remaining, 87.5)
	if q.ExpiresAt == nil || q.ExpiresAt.Unix() != 1767225600 {
		t.Fatalf("%v", q.ExpiresAt)
	}
}

func TestSubscriptionWithUsage(t *testing.T) {
	q := interpret(notFound(), subHit(fp(100), 0), usageHit(2500), unsupported())
	if q.Source != SourceSubscriptionUsage || q.Endpoint != "s" {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "used", q.Used, 25)
	wantFloat(t, "remaining", q.Remaining, 75)
	wantFloat(t, "total", q.Total, 100)
	if f, ok := q.UsedFraction(); !ok || f != 0.25 {
		t.Fatal(f, ok)
	}
}

func TestSubscriptionExpiryOnlyWithUsage(t *testing.T) {
	q := interpret(notFound(), subHit(nil, 1767225600), usageHit(2500), unsupported())
	if q.Source != SourceSubscriptionUsage || q.Total != nil || q.Remaining != nil {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "used", q.Used, 25)
}

func TestUsageOnly(t *testing.T) {
	q := interpret(notFound(), notFound(), usageHit(1234), unsupported())
	if q.Source != SourceUsageOnly || q.Endpoint != "u" || q.Total != nil {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "used", q.Used, 12.34)
}

func TestInconsistentSubscriptionUsageIsInvalidAndNeedsFallback(t *testing.T) {
	q := interpret(notFound(), subHit(fp(20), 0), usageHit(2500), unsupported())
	if q.Status != StatusInvalidData || q.Error != "inconsistent quota data" || q.Endpoint != "s" {
		t.Fatalf("%+v", q)
	}
	if !needsFallback(q) {
		t.Fatal("must try the fallback round")
	}
}

func TestLegacyHardLimitIsNotGuessedAsUnlimited(t *testing.T) {
	q := interpret(notFound(), subHit(fp(100_000_000), 0), unsupported(), unsupported())
	if q.Status != StatusAvailable || q.Unlimited || q.Source != SourceSubscriptionOnly {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "total", q.Total, 100_000_000)
	wantNil(t, "remaining", q.Remaining)
	if _, ok := q.UsedFraction(); ok {
		t.Fatal("fraction without used")
	}
}

func TestSubscriptionExpiryWithoutAmountIsNotAvailableQuota(t *testing.T) {
	q := interpret(notFound(), subHit(nil, 1_767_225_600), notFound(), unsupported())
	if q.Status != StatusUnsupported {
		t.Fatalf("%+v", q)
	}
}

func TestRemainingNeverNegative(t *testing.T) {
	// Overspent beyond the tolerance is inconsistent data, not a negative balance.
	q := interpret(notFound(), subHit(fp(10), 0), usageHit(1100), unsupported())
	if q.Status != StatusInvalidData {
		t.Fatalf("%+v", q)
	}
	// The clamp is a last guard for values that pass the consistency check.
	wantFloat(t, "clamped", clampRemaining(fp(-0.5)), 0)
	wantFloat(t, "kept", clampRemaining(fp(2.5)), 2.5)
	wantNil(t, "nil", clampRemaining(nil))
}

func TestCombinedRoundsKeepStrongestDiagnosticAndSourcePriority(t *testing.T) {
	unauthorized := interpret(hit{kind: hitUnauthorized}, notFound(), notFound(), unsupported())
	empty := interpret(notFound(), notFound(), notFound(), unsupported())
	if got := combine(unauthorized, empty); got.Status != StatusUnauthorized {
		t.Fatalf("%+v", got)
	}
	if got := combine(empty, unauthorized); got.Status != StatusUnauthorized {
		t.Fatalf("%+v", got)
	}

	subscription := interpret(notFound(), subHit(fp(100), 0), notFound(), unsupported())
	grants := interpret(grantsHit(80, 20, 100), notFound(), notFound(), unsupported())
	if got := combine(subscription, grants); got.Source != SourceCreditGrants {
		t.Fatalf("%+v", got)
	}
	if got := combine(grants, subscription); got.Source != SourceCreditGrants {
		t.Fatalf("%+v", got)
	}
	// An available quota always beats any failure.
	invalid := interpret(notFound(), subHit(fp(20), 0), usageHit(2500), unsupported())
	if got := combine(invalid, subscription); got.Status != StatusAvailable {
		t.Fatalf("%+v", got)
	}
	// Equal rank keeps the first.
	other := interpret(grantsHit(1, 1, 2), notFound(), notFound(), unsupported())
	if got := combine(grants, other); got.Total == nil || *got.Total != 100 {
		t.Fatalf("%+v", got)
	}
	// Failure ordering: invalid data > unauthorized > error > unsupported.
	failing := interpret(hit{kind: hitError, msg: "boom"}, notFound(), notFound(), unsupported())
	if got := combine(failing, unauthorized); got.Status != StatusUnauthorized {
		t.Fatalf("%+v", got)
	}
	if got := combine(empty, failing); got.Status != StatusError || got.Error != "boom" {
		t.Fatalf("%+v", got)
	}
	if got := combine(unauthorized, invalid); got.Status != StatusUnauthorized && got.Status != StatusInvalidData {
		t.Fatalf("%+v", got)
	}
}

func TestAll404RoundIsUnsupportedAndNeedsFallback(t *testing.T) {
	q := interpret(notFound(), notFound(), notFound(), notFound())
	if q.Status != StatusUnsupported || !needsFallback(q) {
		t.Fatalf("%+v", q)
	}
}

func TestUnauthorizedStaysUnauthorizedAndNeedsFallback(t *testing.T) {
	q := interpret(hit{kind: hitUnauthorized}, notFound(), unsupported(), unsupported())
	if q.Status != StatusUnauthorized || q.Error != "" || !needsFallback(q) {
		t.Fatalf("%+v", q)
	}
}

func TestFirstErrorHitIsReported(t *testing.T) {
	q := interpret(notFound(), hit{kind: hitError, msg: "HTTP 502"}, hit{kind: hitError, msg: "HTTP 500"}, unsupported())
	if q.Status != StatusError || q.Error != "HTTP 502" {
		t.Fatalf("%+v", q)
	}
}

func TestNeedsFallbackOnlyForWeakSources(t *testing.T) {
	for source, want := range map[Source]bool{
		SourceTokenUsage: false, SourceDeepSeekBalance: false, SourceCreditGrants: false,
		SourceSubscriptionUsage: true, SourceSubscriptionOnly: true, SourceUsageOnly: true,
	} {
		if got := needsFallback(Quota{Status: StatusAvailable, Source: source}); got != want {
			t.Fatalf("%s: got %v", source, got)
		}
	}
}

func TestUsedFraction(t *testing.T) {
	tests := []struct {
		name string
		q    Quota
		want float64
		ok   bool
	}{
		{"half", Quota{Used: fp(5), Total: fp(10)}, 0.5, true},
		{"over total clamps", Quota{Used: fp(15), Total: fp(10)}, 1, true},
		{"negative clamps", Quota{Used: fp(-1), Total: fp(10)}, 0, true},
		{"zero total", Quota{Used: fp(1), Total: fp(0)}, 0, false},
		{"no used", Quota{Total: fp(10)}, 0, false},
		{"no total", Quota{Used: fp(1)}, 0, false},
		{"unlimited", Quota{Used: fp(1), Total: fp(10), Unlimited: true}, 0, false},
	}
	for _, tt := range tests {
		got, ok := tt.q.UsedFraction()
		if got != tt.want || ok != tt.ok {
			t.Fatalf("%s: got %v, %v", tt.name, got, ok)
		}
	}
}

func TestQuotaJSONContract(t *testing.T) {
	at := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	exp := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	q := Quota{
		Status: StatusAvailable, Remaining: fp(1.5), Used: fp(0.5), Total: fp(2), Unit: "USD",
		ExpiresAt: &exp, Source: SourceTokenUsage, Endpoint: "https://x/api/usage/token", FetchedAt: at,
	}
	want := `{"status":"available","remaining":1.5,"used":0.5,"total":2,"unit":"USD",` +
		`"expiresAt":"2026-12-31T00:00:00Z","source":"tokenUsage","endpoint":"https://x/api/usage/token","fetchedAt":"2026-08-21T12:00:00Z"}`
	if got := marshal(t, q); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if got := marshal(t, Quota{Status: StatusUnsupported, FetchedAt: at}); got != `{"status":"unsupported","fetchedAt":"2026-08-21T12:00:00Z"}` {
		t.Fatal(got)
	}
	if got := marshal(t, Quota{Status: StatusAvailable, Unlimited: true, FetchedAt: at}); got != `{"status":"available","unlimited":true,"fetchedAt":"2026-08-21T12:00:00Z"}` {
		t.Fatal(got)
	}
}

func TestDeepSeekBalanceIsRemainingOnly(t *testing.T) {
	r := round{
		deepSeekURL: "d",
		deepSeek: hit{kind: hitDeepSeekBalance, deepSeek: deepSeekBalance{
			currency: UnitCNY, total: 110,
		}},
	}
	q := r.interpret(testNow)
	if q.Status != StatusAvailable || q.Source != SourceDeepSeekBalance || q.Unit != UnitCNY || q.Endpoint != "d" {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "remaining", q.Remaining, 110)
	wantNil(t, "used", q.Used)
	wantNil(t, "total", q.Total)
	if _, ok := q.UsedFraction(); ok {
		t.Fatal("a prepaid balance has no used share")
	}
	if q.Unlimited {
		t.Fatal("balance marked unlimited")
	}
}

func TestDeepSeekInvalidBalanceReportsItsEndpoint(t *testing.T) {
	r := round{
		deepSeekURL: "d",
		deepSeek:    hit{kind: hitInvalidData, msg: msgInvalidQuota},
	}
	q := r.interpret(testNow)
	if q.Status != StatusInvalidData || q.Error != msgInvalidQuota || q.Endpoint != "d" {
		t.Fatalf("%+v", q)
	}
}

func TestDeepSeekUnauthorizedNeedsNoFallback(t *testing.T) {
	r := round{deepSeekURL: "d", deepSeek: hit{kind: hitUnauthorized}}
	q := r.interpret(testNow)
	if q.Status != StatusUnauthorized || q.Endpoint != "" {
		t.Fatalf("%+v", q)
	}
}
