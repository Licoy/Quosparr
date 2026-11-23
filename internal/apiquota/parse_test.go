package apiquota

import (
	"math"
	"testing"
)

func TestParseCreditGrantsFields(t *testing.T) {
	g, ok := parseCreditGrants(mustJSON(t, `{"total_granted":100.0,"total_used":12.5,"total_available":87.5}`))
	if !ok {
		t.Fatal("not parsed")
	}
	wantFloat(t, "total", g.total, 100)
	wantFloat(t, "used", g.used, 12.5)
	wantFloat(t, "remaining", g.remaining, 87.5)
}

func TestParseCreditGrantsFillsMissing(t *testing.T) {
	g, ok := parseCreditGrants(mustJSON(t, `{"total_granted":"100","total_used":"40"}`))
	if !ok {
		t.Fatal("not parsed")
	}
	wantFloat(t, "remaining", g.remaining, 60)

	g, ok = parseCreditGrants(mustJSON(t, `{"total_granted":100,"total_available":70}`))
	if !ok {
		t.Fatal("not parsed")
	}
	wantFloat(t, "used", g.used, 30)
}

func TestParseCreditGrantsRejectsEmptyObject(t *testing.T) {
	for _, body := range []string{`{"object":"credit_summary"}`, `[]`, `"x"`, `{"total_used":"NaN"}`} {
		if _, ok := parseCreditGrants(mustJSON(t, body)); ok {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestParseSubscriptionPrefersHardLimit(t *testing.T) {
	s, ok := parseSubscription(mustJSON(t, `{"hard_limit_usd":100.0,"soft_limit_usd":80.0,"access_until":1767225600}`))
	if !ok {
		t.Fatal("not parsed")
	}
	wantFloat(t, "limit", s.limitUSD, 100)
	if s.expiresAt != 1767225600 {
		t.Fatal(s.expiresAt)
	}

	s, ok = parseSubscription(mustJSON(t, `{"system_hard_limit_usd":"55","soft_limit_usd":80}`))
	if !ok {
		t.Fatal("not parsed")
	}
	wantFloat(t, "system limit", s.limitUSD, 55)

	s, _ = parseSubscription(mustJSON(t, `{"soft_limit_usd":80,"access_until":0}`))
	wantFloat(t, "soft limit", s.limitUSD, 80)
	if s.expiresAt != 0 {
		t.Fatal("zero expiry kept")
	}

	if _, ok := parseSubscription(mustJSON(t, `{"access_until":-5}`)); ok {
		t.Fatal("non-positive expiry accepted")
	}
	// Out-of-range timestamps (e.g. milliseconds) are dropped, not marshalled.
	if s, ok := parseSubscription(mustJSON(t, `{"hard_limit_usd":1,"access_until":1767225600000}`)); !ok || s.expiresAt != 0 {
		t.Fatalf("%+v %v", s, ok)
	}
}

func TestParseUsageReadsTotalUsage(t *testing.T) {
	u, ok := parseUsage(mustJSON(t, `{"object":"list","total_usage":2500.0}`))
	if !ok || u.totalUsage != 2500 {
		t.Fatalf("%+v %v", u, ok)
	}
	if _, ok := parseUsage(mustJSON(t, `{"object":"list"}`)); ok {
		t.Fatal("accepted missing total_usage")
	}
}

func TestUsageToUSDDividesBy100(t *testing.T) {
	if got := usageToUSD(2500); got != 25 {
		t.Fatal(got)
	}
	if got := usageToUSD(8); got != 0.08 {
		t.Fatal(got)
	}
}

func TestParseTokenUsagePrefersDisplayCNY(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{
		"display":{"remaining":999.693074,"total":1000,"unit":"CNY","used":0.306926},
		"expires_at":0,"total_available":499846537,"total_granted":500000000,"total_used":153463,
		"unlimited_quota":true},"message":"ok"}`)
	wantFloat(t, "remaining", tok.remaining, 999.693074)
	wantFloat(t, "used", tok.used, 0.306926)
	wantFloat(t, "total", tok.total, 1000)
	if tok.unit != "CNY" || tok.unlimited || !tok.hasDisplay {
		t.Fatalf("%+v", tok)
	}
}

func TestNormalizeUnit(t *testing.T) {
	tests := map[string]string{
		"rmb": "CNY", " CNY ": "CNY", "¥": "CNY", "元": "CNY", "$": "USD", "usd": "USD", "": "USD",
		"tokens": "TOKENS", "cr": "CR",
	}
	for in, want := range tests {
		if got := normalizeUnit(in); got != want {
			t.Fatalf("normalizeUnit(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeUnit("averyveryverylongunitlabel\x00"); len([]rune(got)) != maxUnitRunes {
		t.Fatalf("unit not capped: %q", got)
	}
}

func TestRawTokenUsageRespectsUnlimitedFlagWithNegativeBalance(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"total_available":-108862302,"total_granted":24035,
		"total_used":108886337,"unlimited_quota":true},"message":"ok"}`)
	wantFloat(t, "remaining", tok.remaining, -108862302)
	wantFloat(t, "total", tok.total, 24035)
	if !tok.unlimited {
		t.Fatal("unlimited flag lost")
	}
	norm, err := normalizeTokenUsage(tok, nil)
	if err != nil {
		t.Fatal(err)
	}
	if norm.unit != UnitRawQuota {
		t.Fatal(norm.unit)
	}
	q := interpret(notFound(), notFound(), notFound(), hit{kind: hitToken, token: norm})
	if q.Status != StatusAvailable || !q.Unlimited || q.Remaining != nil || q.Total != nil || q.Unit != UnitRawQuota {
		t.Fatalf("%+v", q)
	}
	wantFloat(t, "used", q.Used, 108886337)
}

func TestOfficialRawQuotaUsesStatusMetadataForUSD(t *testing.T) {
	token := classifyStatus(200, `{"code":true,"data":{"total_available":1000000,"total_granted":1500000,
		"total_used":500000,"unlimited_quota":false},"message":"ok"}`, expectToken)
	status := classifyStatus(200, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD",
		"display_in_currency":true,"usd_exchange_rate":7.2,"custom_currency_symbol":"¤","custom_currency_exchange_rate":2.5}}`, expectStatus)
	if token.kind != hitToken || status.kind != hitStatus {
		t.Fatalf("%v %v", token.kind, status.kind)
	}
	r := round{token: token, status: status}
	norm := r.normalizeToken()
	if norm.kind != hitToken {
		t.Fatalf("kind %v", norm.kind)
	}
	wantFloat(t, "remaining", norm.token.remaining, 2)
	wantFloat(t, "used", norm.token.used, 1)
	wantFloat(t, "total", norm.token.total, 3)
	if norm.token.unit != "USD" {
		t.Fatal(norm.token.unit)
	}
}

func TestRawQuotaSupportsCNYTokensAndCustomDisplayTypes(t *testing.T) {
	raw := parseToken(t, `{"code":true,"data":{"total_available":500000,"total_granted":750000,
		"total_used":250000,"unlimited_quota":false}}`)
	cny := parseStatus(t, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"CNY","usd_exchange_rate":7}}`)
	tokens := parseStatus(t, `{"success":true,"data":{"quota_display_type":"TOKENS"}}`)
	custom := parseStatus(t, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"CUSTOM",
		"custom_currency_symbol":"CR","custom_currency_exchange_rate":2.5}}`)

	got, err := normalizeTokenUsage(raw, &cny)
	if err != nil {
		t.Fatal(err)
	}
	wantFloat(t, "cny remaining", got.remaining, 7)
	wantFloat(t, "cny used", got.used, 3.5)
	wantFloat(t, "cny total", got.total, 10.5)
	if got.unit != "CNY" {
		t.Fatal(got.unit)
	}

	got, err = normalizeTokenUsage(raw, &tokens)
	if err != nil {
		t.Fatal(err)
	}
	wantFloat(t, "tokens remaining", got.remaining, 500000)
	if got.unit != "TOKENS" {
		t.Fatal(got.unit)
	}

	got, err = normalizeTokenUsage(raw, &custom)
	if err != nil {
		t.Fatal(err)
	}
	wantFloat(t, "custom remaining", got.remaining, 2.5)
	wantFloat(t, "custom used", got.used, 1.25)
	wantFloat(t, "custom total", got.total, 3.75)
	if got.unit != "CR" {
		t.Fatal(got.unit)
	}

	// A custom type without symbol falls back to CUSTOM; without a usable
	// exchange rate it stays raw.
	noSymbol := parseStatus(t, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_exchange_rate":2}}`)
	if got, _ = normalizeTokenUsage(raw, &noSymbol); got.unit != "CUSTOM" {
		t.Fatal(got.unit)
	}
	noRate := parseStatus(t, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"CUSTOM","custom_currency_exchange_rate":0}}`)
	if got, _ = normalizeTokenUsage(raw, &noRate); got.unit != UnitRawQuota {
		t.Fatal(got.unit)
	}
}

func TestLegacyCurrencyFlagAndMissingStatusHaveExplicitUnits(t *testing.T) {
	raw := parseToken(t, `{"code":true,"data":{"total_available":2,"total_granted":3,"total_used":1,"unlimited_quota":false}}`)
	legacyTokens := parseStatus(t, `{"success":true,"data":{"display_in_currency":false}}`)
	noMetadata := parseStatus(t, `{"success":true,"data":{}}`)
	unknownType := parseStatus(t, `{"success":true,"data":{"quota_display_type":"GOLD","display_in_currency":true}}`)

	if got, _ := normalizeTokenUsage(raw, &legacyTokens); got.unit != "TOKENS" {
		t.Fatal(got.unit)
	}
	if noMetadata.displayType != displayRaw {
		t.Fatal("empty status is not raw")
	}
	if got, _ := normalizeTokenUsage(raw, &noMetadata); got.unit != UnitRawQuota {
		t.Fatal(got.unit)
	}
	if got, _ := normalizeTokenUsage(raw, &unknownType); got.unit != UnitRawQuota {
		t.Fatal(got.unit)
	}
	if got, _ := normalizeTokenUsage(raw, nil); got.unit != UnitRawQuota {
		t.Fatal(got.unit)
	}
}

func TestLegacyCurrencyFlagTrueUsesUSD(t *testing.T) {
	raw := parseToken(t, `{"code":true,"data":{"total_available":500000,"total_granted":500000,"total_used":0}}`)
	st := parseStatus(t, `{"success":true,"data":{"display_in_currency":true,"quota_per_unit":500000}}`)
	got, err := normalizeTokenUsage(raw, &st)
	if err != nil || got.unit != "USD" {
		t.Fatalf("%+v %v", got, err)
	}
	wantFloat(t, "remaining", got.remaining, 1)
}

func TestTokenDisplayValuesTakePriorityOverStatusConversion(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"display":{"remaining":7,"used":3,"total":10,"unit":"CNY"},"unlimited_quota":false}}`)
	st := parseStatus(t, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`)
	got, err := normalizeTokenUsage(tok, &st)
	if err != nil {
		t.Fatal(err)
	}
	wantFloat(t, "remaining", got.remaining, 7)
	wantFloat(t, "used", got.used, 3)
	wantFloat(t, "total", got.total, 10)
	if got.unit != "CNY" {
		t.Fatal(got.unit)
	}
}

func TestPartialTokenDisplayDoesNotMixDisplayAndRawUnits(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"display":{"remaining":7,"unit":"CNY"},
		"total_available":500000,"total_granted":750000,"total_used":250000,"unlimited_quota":false}}`)
	st := parseStatus(t, `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`)
	got, err := normalizeTokenUsage(tok, &st)
	if err != nil {
		t.Fatal(err)
	}
	wantFloat(t, "remaining", got.remaining, 1)
	wantFloat(t, "used", got.used, 0.5)
	wantFloat(t, "total", got.total, 1.5)
	if got.unit != "USD" {
		t.Fatal(got.unit)
	}
}

func TestCompleteDisplayValuesIgnoreRawUnlimitedFlag(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"display":{"remaining":150000,"used":50000,"total":200000,"unit":"TOKENS"},
		"total_available":-1,"total_granted":1,"total_used":2,"unlimited_quota":true}}`)
	if tok.unlimited {
		t.Fatal("display block must not be unlimited")
	}
	wantFloat(t, "total", tok.total, 200000)
}

func TestParseTokenUsageRejects(t *testing.T) {
	for _, body := range []string{
		`{"code":false,"data":{"total_available":1}}`,
		`{"success":false,"data":{"total_available":1}}`,
		`{"code":true}`,
		`{"code":true,"data":{}}`,
		`{"code":true,"data":{"total_available":"NaN"}}`,
		`{"code":true,"data":[1]}`,
		`[]`,
	} {
		if _, ok := parseTokenUsage(mustJSON(t, body)); ok {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestParseTokenUsageExpiry(t *testing.T) {
	tok := parseToken(t, `{"code":true,"data":{"total_available":1,"expires_at":1767225600}}`)
	if tok.expiresAt != 1767225600 {
		t.Fatal(tok.expiresAt)
	}
	tok = parseToken(t, `{"code":true,"data":{"total_available":1,"expires_at":"1767225600"}}`)
	if tok.expiresAt != 1767225600 {
		t.Fatal(tok.expiresAt)
	}
	tok = parseToken(t, `{"code":true,"data":{"total_available":1,"expires_at":-1}}`)
	if tok.expiresAt != 0 {
		t.Fatal("never-expiring -1 kept")
	}
}

func TestParseQuotaStatus(t *testing.T) {
	if _, ok := parseQuotaStatus(mustJSON(t, `{"success":false,"data":{}}`)); ok {
		t.Fatal("failed status accepted")
	}
	if _, ok := parseQuotaStatus(mustJSON(t, `{"success":true}`)); ok {
		t.Fatal("status without data accepted")
	}
	st := parseStatus(t, `{"success":true,"data":{"quota_per_unit":"500000","usd_exchange_rate":-1,"custom_currency_symbol":"  "}}`)
	if st.quotaPerUnit != 500000 || st.usdExchangeRate != 0 || st.customSymbol != "" {
		t.Fatalf("%+v", st)
	}
}

func TestValuesConsistent(t *testing.T) {
	tests := []struct {
		name                   string
		remaining, used, total *float64
		unlimited              bool
		want                   bool
	}{
		{"adds up", fp(1.5), fp(0.5), fp(2), false, true},
		{"within tolerance", fp(1.5), fp(0.5000001), fp(2), false, true},
		{"does not add up", fp(1), fp(0.5), fp(2), false, false},
		{"used above total", fp(0), fp(11), fp(10), false, false},
		{"negative remaining", fp(-1), fp(1), fp(0), false, false},
		{"partial", nil, fp(5), nil, false, true},
		{"total only", nil, nil, fp(5), false, true},
		{"unlimited negative remaining", fp(-9), fp(5), fp(1), true, true},
		{"unlimited negative used", nil, fp(-5), nil, true, false},
		{"infinite", nil, fp(math.Inf(1)), nil, false, false},
		{"infinite unlimited", nil, fp(math.Inf(1)), nil, true, false},
	}
	for _, tt := range tests {
		if got := valuesConsistent(tt.remaining, tt.used, tt.total, tt.unlimited); got != tt.want {
			t.Fatalf("%s: got %v", tt.name, got)
		}
	}
}

func TestRawScaleOverflowIsInvalidData(t *testing.T) {
	raw := parseToken(t, `{"code":true,"data":{"total_available":1e300,"total_granted":1e300,"total_used":0}}`)
	st := parseStatus(t, `{"success":true,"data":{"quota_per_unit":1e-300,"quota_display_type":"USD"}}`)
	if _, err := normalizeTokenUsage(raw, &st); err == nil {
		t.Fatal("overflowed scale accepted")
	}
}

func TestParseDeepSeekBalance(t *testing.T) {
	tests := []struct {
		name, body, currency string
		total                float64
	}{
		{"CNY", `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"110.00","granted_balance":"10.00","topped_up_balance":"100.00"}]}`, UnitCNY, 110},
		{"USD", `{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"4.25"}]}`, UnitUSD, 4.25},
		{"positive currency wins over zero", `{"balance_infos":[{"currency":"CNY","total_balance":"0"},{"currency":"USD","total_balance":"3.50"}]}`, UnitUSD, 3.5},
		{"all zero keeps the first", `{"balance_infos":[{"currency":"CNY","total_balance":"0"},{"currency":"USD","total_balance":"0"}]}`, UnitCNY, 0},
		{"components are summed", `{"balance_infos":[{"currency":"CNY","granted_balance":"2.5","topped_up_balance":"7.5"}]}`, UnitCNY, 10},
		{"unit normalized", `{"balance_infos":[{"currency":"rmb","total_balance":1}]}`, UnitCNY, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, ok := parseDeepSeekBalance(mustJSON(t, tt.body))
			if !ok || b.currency != tt.currency {
				t.Fatalf("%+v %v", b, ok)
			}
			wantFloat(t, "total", &b.total, tt.total)
		})
	}
}

func TestParseDeepSeekBalanceRejects(t *testing.T) {
	for _, body := range []string{
		`{}`, `[]`, `"x"`, `{"balance_infos":[]}`, `{"balance_infos":"x"}`,
		`{"balance_infos":[{}]}`, `{"balance_infos":[{"currency":"","total_balance":"1"}]}`,
		`{"balance_infos":[{"currency":"CNY"}]}`, `{"balance_infos":[{"currency":"CNY","total_balance":"NaN"}]}`,
	} {
		if b, ok := parseDeepSeekBalance(mustJSON(t, body)); ok {
			t.Fatalf("accepted %s: %+v", body, b)
		}
	}
}
