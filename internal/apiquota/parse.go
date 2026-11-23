package apiquota

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// maxUnixSeconds keeps expiry timestamps inside the range time.Time can
// marshal to JSON (year 9999).
const maxUnixSeconds = 253402300799

type object = map[string]any

// decodeJSON parses exactly one JSON document, keeping numbers exact.
func decodeJSON(body string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return v, true
}

func asObject(v any) (object, bool) {
	o, ok := v.(object)
	return o, ok
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

func jsonFloat(v any) (float64, bool) {
	var f float64
	var err error
	switch x := v.(type) {
	case json.Number:
		f, err = x.Float64()
	case string:
		f, err = strconv.ParseFloat(strings.TrimSpace(x), 64)
	default:
		return 0, false
	}
	return f, err == nil && finite(f)
}

func jsonInt(v any) (int64, bool) {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, true
		}
		f, err := x.Float64()
		if err != nil || !finite(f) {
			return 0, false
		}
		return int64(math.Max(math.Min(f, math.MaxInt64), math.MinInt64)), true
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return i, err == nil
	}
	return 0, false
}

func fieldFloat(o object, key string) *float64 {
	if f, ok := jsonFloat(o[key]); ok {
		return &f
	}
	return nil
}

// fieldExpiry reads a positive unix timestamp in seconds; 0 means none.
func fieldExpiry(o object, key string) int64 {
	if i, ok := jsonInt(o[key]); ok && i > 0 && i <= maxUnixSeconds {
		return i
	}
	return 0
}

func fieldBool(o object, key string) (value, ok bool) {
	value, ok = o[key].(bool)
	return
}

func fieldString(o object, key string) (string, bool) {
	s, ok := o[key].(string)
	return s, ok
}

type grants struct {
	remaining, used, total *float64
}

type subscription struct {
	limitUSD  *float64
	expiresAt int64
}

type usage struct {
	totalUsage float64
}

type tokenUsage struct {
	remaining, used, total *float64
	unit                   string
	unlimited              bool
	expiresAt              int64
	hasDisplay             bool
}

// deepSeekBalance is the official DeepSeek balance in one currency.
type deepSeekBalance struct {
	currency string
	total    float64
}

type displayType int

const (
	displayRaw displayType = iota
	displayUSD
	displayCNY
	displayTokens
	displayCustom
)

type quotaStatus struct {
	quotaPerUnit       float64 // 0 when absent
	displayType        displayType
	usdExchangeRate    float64
	customSymbol       string
	customExchangeRate float64
}

func parseCreditGrants(v any) (grants, bool) {
	o, ok := asObject(v)
	if !ok {
		return grants{}, false
	}
	g := grants{
		remaining: fieldFloat(o, "total_available"),
		used:      fieldFloat(o, "total_used"),
		total:     fieldFloat(o, "total_granted"),
	}
	if g.remaining == nil && g.used == nil && g.total == nil {
		return grants{}, false
	}
	if g.remaining == nil && g.total != nil && g.used != nil {
		r := *g.total - *g.used
		g.remaining = &r
	}
	if g.used == nil && g.total != nil && g.remaining != nil {
		u := *g.total - *g.remaining
		g.used = &u
	}
	return g, true
}

func parseSubscription(v any) (subscription, bool) {
	o, ok := asObject(v)
	if !ok {
		return subscription{}, false
	}
	s := subscription{expiresAt: fieldExpiry(o, "access_until")}
	for _, key := range []string{"hard_limit_usd", "system_hard_limit_usd", "soft_limit_usd"} {
		if s.limitUSD = fieldFloat(o, key); s.limitUSD != nil {
			break
		}
	}
	if s.limitUSD == nil && s.expiresAt == 0 {
		return subscription{}, false
	}
	return s, true
}

func parseUsage(v any) (usage, bool) {
	o, ok := asObject(v)
	if !ok {
		return usage{}, false
	}
	f := fieldFloat(o, "total_usage")
	if f == nil {
		return usage{}, false
	}
	return usage{totalUsage: *f}, true
}

// parseTokenUsage reads the new-api style token usage document. A complete
// "display" block (remaining, used, total and unit) wins; a partial one is
// ignored so display and raw units are never mixed.
func parseTokenUsage(v any) (tokenUsage, bool) {
	o, ok := asObject(v)
	if !ok {
		return tokenUsage{}, false
	}
	if b, ok := fieldBool(o, "code"); ok && !b {
		return tokenUsage{}, false
	}
	if b, ok := fieldBool(o, "success"); ok && !b {
		return tokenUsage{}, false
	}
	data, ok := asObject(o["data"])
	if !ok {
		return tokenUsage{}, false
	}
	t := tokenUsage{unit: "quota"}
	if d, ok := displayBlock(data); ok {
		t = d
	} else {
		t.remaining = fieldFloat(data, "total_available")
		t.used = fieldFloat(data, "total_used")
		t.total = fieldFloat(data, "total_granted")
		t.unlimited, _ = fieldBool(data, "unlimited_quota")
	}
	if t.remaining == nil && t.used == nil && t.total == nil {
		return tokenUsage{}, false
	}
	t.expiresAt = fieldExpiry(data, "expires_at")
	return t, true
}

func displayBlock(data object) (tokenUsage, bool) {
	d, ok := asObject(data["display"])
	if !ok {
		return tokenUsage{}, false
	}
	remaining, used, total := fieldFloat(d, "remaining"), fieldFloat(d, "used"), fieldFloat(d, "total")
	unit, isString := fieldString(d, "unit")
	if remaining == nil || used == nil || total == nil || !isString {
		return tokenUsage{}, false
	}
	return tokenUsage{
		remaining:  remaining,
		used:       used,
		total:      total,
		unit:       normalizeUnit(unit),
		hasDisplay: true,
	}, true
}

// parseDeepSeekBalance reads the official DeepSeek balance document:
//
//	{"is_available":true,"balance_infos":[{"currency":"CNY",
//	 "total_balance":"110.00","granted_balance":"10.00",
//	 "topped_up_balance":"100.00"}]}
//
// An account can hold several currencies. The first row with money left wins,
// so a zero CNY row does not hide a positive USD one; an account with no money
// left still reports its first row. A row without total_balance but with both
// components is summed, since DeepSeek defines the total as their sum.
func parseDeepSeekBalance(v any) (deepSeekBalance, bool) {
	o, ok := asObject(v)
	if !ok {
		return deepSeekBalance{}, false
	}
	rows, ok := o["balance_infos"].([]any)
	if !ok {
		return deepSeekBalance{}, false
	}
	var picked deepSeekBalance
	for _, row := range rows {
		b, ok := parseDeepSeekBalanceRow(row)
		if !ok {
			continue
		}
		if picked.currency == "" || picked.total <= 0 && b.total > 0 {
			picked = b
		}
	}
	return picked, picked.currency != ""
}

func parseDeepSeekBalanceRow(v any) (deepSeekBalance, bool) {
	o, ok := asObject(v)
	if !ok {
		return deepSeekBalance{}, false
	}
	currency, ok := fieldString(o, "currency")
	if !ok || strings.TrimSpace(currency) == "" {
		return deepSeekBalance{}, false
	}
	total := fieldFloat(o, "total_balance")
	if total == nil {
		granted, toppedUp := fieldFloat(o, "granted_balance"), fieldFloat(o, "topped_up_balance")
		if granted != nil && toppedUp != nil {
			sum := *granted + *toppedUp
			total = &sum
		}
	}
	if total == nil {
		return deepSeekBalance{}, false
	}
	unit := normalizeUnit(currency)
	if unit == "" {
		return deepSeekBalance{}, false
	}
	return deepSeekBalance{currency: unit, total: *total}, true
}

func normalizeUnit(raw string) string {
	u := strings.ToUpper(strings.TrimSpace(raw))
	switch u {
	case "RMB", "CNY", "¥", "元":
		return UnitCNY
	case "USD", "$", "":
		return UnitUSD
	}
	return limitUnit(u)
}

const maxUnitRunes = 16

// limitUnit drops control characters and caps a site supplied unit label.
func limitUnit(u string) string {
	u = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, u)
	if r := []rune(u); len(r) > maxUnitRunes {
		return string(r[:maxUnitRunes])
	}
	return u
}

func parseDisplayType(data object) displayType {
	if raw, ok := fieldString(data, "quota_display_type"); ok {
		switch strings.ToUpper(strings.TrimSpace(raw)) {
		case "USD":
			return displayUSD
		case "CNY":
			return displayCNY
		case "TOKENS":
			return displayTokens
		case "CUSTOM":
			return displayCustom
		}
		return displayRaw
	}
	if enabled, ok := fieldBool(data, "display_in_currency"); ok {
		if enabled {
			return displayUSD
		}
		return displayTokens
	}
	return displayRaw
}

func positive(f *float64) float64 {
	if f != nil && *f > 0 {
		return *f
	}
	return 0
}

func parseQuotaStatus(v any) (quotaStatus, bool) {
	o, ok := asObject(v)
	if !ok {
		return quotaStatus{}, false
	}
	if b, ok := fieldBool(o, "success"); ok && !b {
		return quotaStatus{}, false
	}
	data, ok := asObject(o["data"])
	if !ok {
		return quotaStatus{}, false
	}
	symbol, _ := fieldString(data, "custom_currency_symbol")
	return quotaStatus{
		quotaPerUnit:       positive(fieldFloat(data, "quota_per_unit")),
		displayType:        parseDisplayType(data),
		usdExchangeRate:    positive(fieldFloat(data, "usd_exchange_rate")),
		customSymbol:       strings.TrimSpace(symbol),
		customExchangeRate: positive(fieldFloat(data, "custom_currency_exchange_rate")),
	}, true
}

// rawScale converts raw site quota units: value * mul / div.
type rawScale struct {
	mul, div float64
	unit     string
}

func (s quotaStatus) rawScale() (rawScale, bool) {
	switch s.displayType {
	case displayTokens:
		return rawScale{1, 1, UnitTokens}, true
	case displayUSD:
		if s.quotaPerUnit > 0 {
			return rawScale{1, s.quotaPerUnit, UnitUSD}, true
		}
	case displayCNY:
		if s.quotaPerUnit > 0 && s.usdExchangeRate > 0 {
			return rawScale{s.usdExchangeRate, s.quotaPerUnit, UnitCNY}, true
		}
	case displayCustom:
		if s.quotaPerUnit > 0 && s.customExchangeRate > 0 {
			unit := limitUnit(s.customSymbol)
			if unit == "" {
				unit = "CUSTOM"
			}
			return rawScale{s.customExchangeRate, s.quotaPerUnit, unit}, true
		}
	}
	return rawScale{}, false
}

func (s rawScale) apply(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := *v * s.mul / s.div
	return &r
}

var errInconsistent = errors.New("inconsistent quota data")

// normalizeTokenUsage converts raw quota values with the site status
// metadata (when the token document has no complete display block) and
// validates the result.
func normalizeTokenUsage(t tokenUsage, status *quotaStatus) (tokenUsage, error) {
	if !t.hasDisplay {
		scale := rawScale{1, 1, UnitRawQuota}
		if status != nil {
			if s, ok := status.rawScale(); ok {
				scale = s
			}
		}
		t.remaining, t.used, t.total = scale.apply(t.remaining), scale.apply(t.used), scale.apply(t.total)
		t.unit = scale.unit
	}
	if !valuesConsistent(t.remaining, t.used, t.total, t.unlimited) {
		return tokenUsage{}, errInconsistent
	}
	return t, nil
}

// valuesConsistent rejects non-finite or negative values and amounts that do
// not add up (total = remaining + used within a small tolerance). Unlimited
// quotas only need a non-negative used amount.
func valuesConsistent(remaining, used, total *float64, unlimited bool) bool {
	for _, v := range []*float64{remaining, used, total} {
		if v != nil && !finite(*v) {
			return false
		}
	}
	if unlimited {
		return used == nil || *used >= 0
	}
	for _, v := range []*float64{remaining, used, total} {
		if v != nil && *v < 0 {
			return false
		}
	}
	if total != nil {
		tol := math.Abs(*total)*1e-6 + 1e-6
		if used != nil && *used-*total > tol || remaining != nil && *remaining-*total > tol {
			return false
		}
	}
	if remaining == nil || used == nil || total == nil {
		return true
	}
	tol := math.Max(math.Max(math.Abs(*total), math.Abs(*remaining)), math.Abs(*used))*1e-6 + 1e-6
	return math.Abs(*total-*remaining-*used) <= tol
}

// usageToUSD converts dashboard total_usage (hundredths of a dollar) to USD.
func usageToUSD(totalUsage float64) float64 { return totalUsage / 100 }
