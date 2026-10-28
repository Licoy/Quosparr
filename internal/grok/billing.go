package grok

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

// BillingPeriodKind is the length of the quota window.
type BillingPeriodKind string

// Quota window kinds.
const (
	BillingWeekly  BillingPeriodKind = "weekly"
	BillingMonthly BillingPeriodKind = "monthly"
)

// BillingSource says which part of the billing response the numbers came from.
type BillingSource string

// Billing sources. The raw values are part of the stored snapshot JSON.
const (
	BillingSourceCurrent    BillingSource = "current"
	BillingSourceDeprecated BillingSource = "deprecated"
	BillingSourceLegacyFlat BillingSource = "legacyFlat"
)

// BillingPeriod is the included-credits window. Either date may be nil when
// the server did not send it.
type BillingPeriod struct {
	Kind     BillingPeriodKind
	StartsAt *time.Time
	ResetsAt *time.Time
}

// BillingProductUsage is a product's (GrokBuild / GrokImagine / GrokChat)
// contribution to the overall included usage, in percent.
type BillingProductUsage struct {
	Product      string
	UsagePercent float64
}

// BillingSnapshot is the decoded Grok billing state of one account. Optional
// values are pointers: nil means the server did not report them.
type BillingSnapshot struct {
	Plan                 *string
	Subscription         *SubscriptionInfo
	SubscriptionError    string
	IncludedUsagePercent float64
	Period               *BillingPeriod
	// Included allowance in USD cents (config.monthlyLimit / config.used).
	IncludedLimitCents   *int64
	IncludedUsedCents    *int64
	PrepaidBalanceCents  *int64
	OnDemandEnabled      *bool
	OnDemandUsedCents    *int64
	OnDemandLimitCents   *int64
	ProductUsage         []BillingProductUsage
	IsUnifiedBillingUser *bool
	// ResetCredits holds the banked reset cards when that RPC succeeded.
	ResetCredits *ResetCreditSnapshot
	Source       BillingSource
	UpdatedAt    time.Time
}

// Errors of the billing decoders.
var (
	ErrBillingUnknownStructure = errors.New("grok: billing: unknown structure")
	ErrBillingInvalidPercent   = errors.New("grok: billing: invalid percentage")
)

// IncludedRemainingCents is the remaining included allowance when the limit is
// known (a missing "used" counts as 0, negatives clamp to 0).
func (s BillingSnapshot) IncludedRemainingCents() (int64, bool) {
	if s.IncludedLimitCents == nil {
		return 0, false
	}
	used := int64(0)
	if s.IncludedUsedCents != nil && *s.IncludedUsedCents > 0 {
		used = *s.IncludedUsedCents
	}
	remaining := *s.IncludedLimitCents - used
	if remaining < 0 || (used > 0 && remaining > *s.IncludedLimitCents) {
		remaining = 0 // the subtraction overflowed or went negative
	}
	return remaining, true
}

// MergingPlan prefers preferred when the snapshot has no plan (or always with
// overwrite); otherwise the existing plan is normalized to its display name.
func (s BillingSnapshot) MergingPlan(preferred string, overwrite bool) BillingSnapshot {
	display, hasDisplay := TierDisplayName(preferred)
	if overwrite && hasDisplay {
		s.Plan = &display
		return s
	}
	if hasDisplay && (s.Plan == nil || billingTrim(*s.Plan) == "") {
		s.Plan = &display
		return s
	}
	if s.Plan != nil {
		if existing, ok := TierDisplayName(*s.Plan); ok {
			s.Plan = &existing
		}
	}
	return s
}

// MergingMoneyAllowance fills the included limit and used cents when unset.
func (s BillingSnapshot) MergingMoneyAllowance(limit, used *int64) BillingSnapshot {
	if s.IncludedLimitCents == nil {
		s.IncludedLimitCents = limit
	}
	if s.IncludedUsedCents == nil {
		s.IncludedUsedCents = used
	}
	return s
}

// MergingResetCredits replaces the reset cards (nil clears them).
func (s BillingSnapshot) MergingResetCredits(r *ResetCreditSnapshot) BillingSnapshot {
	s.ResetCredits = r
	return s
}

// ---- decoding the billing response ----

// billingConfig is the decoded "config" object; every field is best effort.
type billingConfig struct {
	creditUsagePercent *float64
	currentPeriod      *billingUsagePeriod
	onDemandCap        *int64
	onDemandUsed       *int64
	prepaidBalance     *int64
	monthlyLimit       *int64
	used               *int64
	periodStart        *string
	periodEnd          *string
	productUsage       []billingProductEntry
	hasProductUsage    bool
	isUnifiedBilling   *bool
}

type billingProductEntry struct {
	product *string
	percent *float64
}

type billingUsagePeriod struct{ kind, start, end *string }

type billingResponse struct {
	config         *billingConfig
	onDemandEnable *bool
	tier           string
	hasTier        bool
	cycleStart     *string
	cycleEnd       *string
	hasCycle       bool
	monthlyLimit   *int64
	onDemandCap    *int64
	usageIncluded  *int64
	usageOnDemand  *int64
	usageTotal     *int64
	hasUsage       bool
}

func parseBillingResponse(data []byte) (*billingResponse, error) {
	root, err := parseDecoderJSON(data)
	if err != nil || root.kind != jsonObject {
		return nil, ErrBillingUnknownStructure
	}
	r := &billingResponse{}
	if c := root.get("config"); c != nil && c.kind == jsonObject {
		r.config = parseBillingConfig(c)
	}
	r.onDemandEnable = billingBoolField(root, "onDemandEnabled")
	if r.onDemandEnable == nil {
		r.onDemandEnable = billingBoolField(root, "on_demand_enabled")
	}
	for _, key := range []string{"subscriptionTier", "subscription_tier"} {
		if s, ok := root.get(key).str(); ok {
			r.tier, r.hasTier = s, true
			break
		}
	}
	if cycle := root.get("billingCycle"); cycle != nil {
		if start, end, ok := billingCycleFields(cycle); ok {
			r.cycleStart, r.cycleEnd, r.hasCycle = start, end, true
		}
	}
	r.monthlyLimit = billingCentLenient(root.get("monthlyLimit"))
	r.onDemandCap = billingCentLenient(root.get("onDemandCap"))
	if usage := root.get("usage"); usage != nil {
		if inc, od, total, ok := billingUsageFields(usage); ok {
			r.usageIncluded, r.usageOnDemand, r.usageTotal, r.hasUsage = inc, od, total, true
		}
	}
	return r, nil
}

func billingBoolField(obj *jsonNode, key string) *bool {
	if b, ok := obj.get(key).boolean(); ok {
		return &b
	}
	return nil
}

func parseBillingConfig(c *jsonNode) *billingConfig {
	cfg := &billingConfig{
		creditUsagePercent: billingFlexibleDouble(c.get("creditUsagePercent")),
		onDemandCap:        billingCentLenient(c.get("onDemandCap")),
		onDemandUsed:       billingCentLenient(c.get("onDemandUsed")),
		prepaidBalance:     billingCentLenient(c.get("prepaidBalance")),
		monthlyLimit:       billingCentLenient(c.get("monthlyLimit")),
		used:               billingCentLenient(c.get("used")),
		isUnifiedBilling:   billingBoolField(c, "isUnifiedBillingUser"),
	}
	if p := c.get("currentPeriod"); p != nil {
		if kind, start, end, ok := billingPeriodFields(p); ok {
			cfg.currentPeriod = &billingUsagePeriod{kind, start, end}
		}
	}
	cfg.periodStart = billingStringLenient(c.get("billingPeriodStart"))
	cfg.periodEnd = billingStringLenient(c.get("billingPeriodEnd"))
	if list := c.get("productUsage"); list != nil && list.kind == jsonArray {
		entries := make([]billingProductEntry, 0, len(list.arr))
		valid := true
		for _, item := range list.arr {
			if item.kind != jsonObject {
				valid = false // one bad row discards the whole array
				break
			}
			entries = append(entries, billingProductEntry{
				product: billingStringLenient(item.get("product")),
				percent: billingFlexibleDouble(item.get("usagePercent")),
			})
		}
		if valid {
			cfg.productUsage, cfg.hasProductUsage = entries, true
		}
	}
	return cfg
}

func billingStringLenient(n *jsonNode) *string {
	if s, ok := n.str(); ok {
		return &s
	}
	return nil
}

// billingOptString is a synthesized decodeIfPresent(String): null and a
// missing key are nil, a value of another type fails the whole struct.
func billingOptString(obj *jsonNode, key string) (*string, bool) {
	n := obj.get(key)
	if !n.present() {
		return nil, true
	}
	s, ok := n.str()
	if !ok {
		return nil, false
	}
	return &s, true
}

func billingPeriodFields(n *jsonNode) (kind, start, end *string, ok bool) {
	if n.kind != jsonObject {
		return nil, nil, nil, false
	}
	var ok1, ok2, ok3 bool
	kind, ok1 = billingOptString(n, "type")
	start, ok2 = billingOptString(n, "start")
	end, ok3 = billingOptString(n, "end")
	return kind, start, end, ok1 && ok2 && ok3
}

func billingCycleFields(n *jsonNode) (start, end *string, ok bool) {
	if n.kind != jsonObject {
		return nil, nil, false
	}
	var ok1, ok2 bool
	start, ok1 = billingOptString(n, "billingPeriodStart")
	end, ok2 = billingOptString(n, "billingPeriodEnd")
	return start, end, ok1 && ok2
}

func billingUsageFields(n *jsonNode) (included, onDemand, total *int64, ok bool) {
	if n.kind != jsonObject {
		return nil, nil, nil, false
	}
	var ok1, ok2, ok3 bool
	included, ok1 = billingCentStrict(n.get("includedUsed"))
	onDemand, ok2 = billingCentStrict(n.get("onDemandUsed"))
	total, ok3 = billingCentStrict(n.get("totalUsed"))
	return included, onDemand, total, ok1 && ok2 && ok3
}

// billingFlexibleDouble reads a percentage written as a number or as text.
func billingFlexibleDouble(n *jsonNode) *float64 {
	if n == nil {
		return nil
	}
	switch n.kind {
	case jsonNumber:
		if f := billingNumberDouble(n.s); !math.IsInf(f, 0) {
			return &f
		}
	case jsonString:
		if f, ok := billingSwiftDouble(n.s); ok {
			return &f
		}
	}
	return nil
}

// billingCentLenient is "try? decodeIfPresent(Cent)": nil when absent, null or
// undecodable.
func billingCentLenient(n *jsonNode) *int64 {
	v, _ := billingCentStrict(n)
	return v
}

// billingCentStrict is decodeIfPresent(Cent). The second result is false
// where Swift's decoder throws.
//
// A number is taken as an integer, else rounded; text is trimmed and parsed
// as an integer or a finite number; an object reads its "val" the same way
// and falls back to 0. Anything else, and a magnitude beyond Int64 (which
// traps in Swift), cannot be decoded.
func billingCentStrict(n *jsonNode) (*int64, bool) {
	if !n.present() {
		return nil, true
	}
	switch n.kind {
	case jsonNumber:
		return billingCentNumber(n.s)
	case jsonString:
		if v, ok := billingParseCentText(n.s); ok {
			return &v, true
		}
	case jsonObject:
		val := n.get("val")
		if val == nil {
			zero := int64(0)
			return &zero, true
		}
		switch val.kind {
		case jsonNumber:
			return billingCentNumber(val.s)
		case jsonString:
			if v, ok := billingParseCentText(val.s); ok {
				return &v, true
			}
		}
		zero := int64(0)
		return &zero, true
	}
	return nil, false
}

func billingCentNumber(tok string) (*int64, bool) {
	if v, ok := billingNumberInt64(tok); ok {
		return &v, true
	}
	if v, ok := billingRoundCents(billingNumberDouble(tok)); ok {
		return &v, true
	}
	return nil, false
}

// billingBestEffortPeriod is UsagePeriod.bestEffortQuotaPeriod.
func (p *billingUsagePeriod) bestEffort() *BillingPeriod {
	if p == nil || p.kind == nil {
		return nil
	}
	switch *p.kind {
	case "USAGE_PERIOD_TYPE_WEEKLY":
		return billingMakePeriod(BillingWeekly, p.start, p.end)
	case "USAGE_PERIOD_TYPE_MONTHLY":
		return billingMakePeriod(BillingMonthly, p.start, p.end)
	}
	return nil
}

// billingMakePeriod keeps usage visible when dates are missing, and drops the
// window when a given date is unreadable or the window is empty or reversed.
func billingMakePeriod(kind BillingPeriodKind, start, end *string) *BillingPeriod {
	if start == nil && end == nil {
		return &BillingPeriod{Kind: kind}
	}
	var startsAt, resetsAt *time.Time
	if start != nil {
		t, ok := BillingParseDate(*start)
		if !ok {
			return nil
		}
		startsAt = &t
	}
	if end != nil {
		t, ok := BillingParseDate(*end)
		if !ok {
			return nil
		}
		resetsAt = &t
	}
	if startsAt != nil && resetsAt != nil && !resetsAt.After(*startsAt) {
		return nil
	}
	return &BillingPeriod{Kind: kind, StartsAt: startsAt, ResetsAt: resetsAt}
}

// decodedProductUsage skips unnamed and non-finite rows and orders the rest
// by share, then name.
func (c *billingConfig) decodedProductUsage() []BillingProductUsage {
	out := []BillingProductUsage{}
	if !c.hasProductUsage {
		return out
	}
	for _, e := range c.productUsage {
		name := ""
		if e.product != nil {
			name = billingTrim(*e.product)
		}
		if name == "" {
			continue
		}
		percent := 0.0
		if e.percent != nil {
			percent = *e.percent
		}
		if math.IsNaN(percent) || math.IsInf(percent, 0) {
			continue
		}
		out = append(out, BillingProductUsage{Product: name, UsagePercent: percent})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UsagePercent != out[j].UsagePercent {
			return out[i].UsagePercent > out[j].UsagePercent
		}
		return billingLocalizedCompare(out[i].Product, out[j].Product) < 0
	})
	return out
}

func billingPlan(r *billingResponse) *string {
	if !r.hasTier {
		return nil
	}
	if name, ok := TierDisplayName(r.tier); ok {
		return &name
	}
	return nil
}

// DecodeBillingResponse decodes the chat-proxy billing JSON (credits or cents
// format, or the legacy flat shape) into a snapshot stamped with now.
func DecodeBillingResponse(data []byte, now time.Time) (BillingSnapshot, error) {
	resp, err := parseBillingResponse(data)
	if err != nil {
		return BillingSnapshot{}, err
	}
	if resp.config != nil {
		snap, err := billingFromConfig(resp.config, resp, now)
		if !errors.Is(err, ErrBillingUnknownStructure) {
			return snap, err
		}
		// An empty wrapper beside the legacy flat fields: use those.
	}
	return billingLegacy(resp, now)
}

func billingFromConfig(cfg *billingConfig, resp *billingResponse, now time.Time) (BillingSnapshot, error) {
	var percent float64
	var period *BillingPeriod
	var source BillingSource
	switch {
	case cfg.creditUsagePercent != nil:
		percent = *cfg.creditUsagePercent
		period = cfg.currentPeriod.bestEffort()
		source = BillingSourceCurrent
	case cfg.currentPeriod.bestEffort() != nil:
		// After a credits reset the API often returns the new currentPeriod
		// without a percentage: that is 0% used, not the monthly USD window.
		percent = 0
		period = cfg.currentPeriod.bestEffort()
		source = BillingSourceCurrent
	case cfg.monthlyLimit != nil && *cfg.monthlyLimit > 0 && cfg.used != nil:
		percent = float64(*cfg.used) / float64(*cfg.monthlyLimit) * 100
		period = billingMakePeriod(BillingMonthly, cfg.periodStart, cfg.periodEnd)
		source = BillingSourceDeprecated
	default:
		return BillingSnapshot{}, ErrBillingUnknownStructure
	}
	if math.IsNaN(percent) || math.IsInf(percent, 0) {
		return BillingSnapshot{}, ErrBillingInvalidPercent
	}
	return BillingSnapshot{
		Plan:                 billingPlan(resp),
		IncludedUsagePercent: percent,
		Period:               period,
		IncludedLimitCents:   cfg.monthlyLimit,
		IncludedUsedCents:    cfg.used,
		PrepaidBalanceCents:  cfg.prepaidBalance,
		OnDemandEnabled:      resp.onDemandEnable,
		OnDemandUsedCents:    cfg.onDemandUsed,
		OnDemandLimitCents:   cfg.onDemandCap,
		ProductUsage:         cfg.decodedProductUsage(),
		IsUnifiedBillingUser: cfg.isUnifiedBilling,
		Source:               source,
		UpdatedAt:            now,
	}, nil
}

func billingLegacyUsed(resp *billingResponse) *int64 {
	if resp.usageTotal != nil {
		return resp.usageTotal
	}
	return resp.usageIncluded
}

func billingLegacy(resp *billingResponse, now time.Time) (BillingSnapshot, error) {
	used := billingLegacyUsed(resp)
	if resp.monthlyLimit == nil || *resp.monthlyLimit <= 0 || used == nil {
		return BillingSnapshot{}, ErrBillingUnknownStructure
	}
	limit := *resp.monthlyLimit
	percent := float64(*used) / float64(limit) * 100
	if math.IsNaN(percent) || math.IsInf(percent, 0) {
		return BillingSnapshot{}, ErrBillingInvalidPercent
	}
	return BillingSnapshot{
		Plan:                 billingPlan(resp),
		IncludedUsagePercent: percent,
		Period:               billingMakePeriod(BillingMonthly, resp.cycleStart, resp.cycleEnd),
		IncludedLimitCents:   &limit,
		IncludedUsedCents:    used,
		OnDemandEnabled:      resp.onDemandEnable,
		OnDemandUsedCents:    resp.usageOnDemand,
		OnDemandLimitCents:   resp.onDemandCap,
		ProductUsage:         []BillingProductUsage{},
		Source:               BillingSourceLegacyFlat,
		UpdatedAt:            now,
	}, nil
}

// DecodeMoneyAllowance extracts the USD allowance (limit, used) of a default
// or format=cents billing payload; the credits format usually omits them.
func DecodeMoneyAllowance(data []byte) (limit, used *int64, err error) {
	resp, err := parseBillingResponse(data)
	if err != nil {
		return nil, nil, err
	}
	if resp.config != nil && (resp.config.monthlyLimit != nil || resp.config.used != nil) {
		return resp.config.monthlyLimit, resp.config.used, nil
	}
	used = billingLegacyUsed(resp)
	if resp.monthlyLimit != nil || used != nil {
		return resp.monthlyLimit, used, nil
	}
	return nil, nil, ErrBillingUnknownStructure
}

// DecodeSettingsPlan reads the plan label of the chat-proxy /v1/settings
// answer: the first of subscription_tier_display, subscriptionTierDisplay,
// subscription_tier and subscriptionTier that is a string naming a tier.
func DecodeSettingsPlan(data []byte) (string, bool) {
	root, err := parseLooseJSON(data)
	if err != nil || root.kind != jsonObject {
		return "", false
	}
	for _, key := range []string{"subscription_tier_display", "subscriptionTierDisplay", "subscription_tier", "subscriptionTier"} {
		if s, ok := root.get(key).str(); ok {
			if display, ok := TierDisplayName(s); ok {
				return display, true
			}
		}
	}
	return "", false
}

// ---- stored snapshot JSON (Codable shape) ----

type billingMember struct{ key, raw string }

func billingObject(members []billingMember) string {
	sort.Slice(members, func(i, j int) bool { return members[i].key < members[j].key })
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range members {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(billingQuote(m.key))
		b.WriteByte(':')
		b.WriteString(m.raw)
	}
	b.WriteByte('}')
	return b.String()
}

// billingQuote writes a JSON string like JSONEncoder with
// .withoutEscapingSlashes.
func billingQuote(s string) string {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		case r == utf8.RuneError:
			b.WriteString("�")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func billingDateJSON(t time.Time) string { return `"` + acctFormatMillis(t) + `"` }

func billingOptDate(m *[]billingMember, key string, t *time.Time) {
	if t != nil {
		*m = append(*m, billingMember{key, billingDateJSON(*t)})
	}
}

func billingOptInt(m *[]billingMember, key string, v *int64) {
	if v != nil {
		*m = append(*m, billingMember{key, strconv.FormatInt(*v, 10)})
	}
}

func billingOptBool(m *[]billingMember, key string, v *bool) {
	if v != nil {
		*m = append(*m, billingMember{key, strconv.FormatBool(*v)})
	}
}

// MarshalJSON writes the snapshot the way the Swift Codable encoder does:
// sorted keys, absent optionals left out, productUsage always present, dates
// as ISO 8601 UTC with milliseconds. A non-finite percentage cannot be
// encoded.
func (s BillingSnapshot) MarshalJSON() ([]byte, error) {
	values := []float64{s.IncludedUsagePercent}
	for _, p := range s.ProductUsage {
		values = append(values, p.UsagePercent)
	}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("grok: billing: cannot encode %v", v)
		}
	}
	m := []billingMember{
		{"includedUsagePercent", swiftjson.FormatDouble(s.IncludedUsagePercent)},
		{"source", billingQuote(string(s.Source))},
		{"updatedAt", billingDateJSON(s.UpdatedAt)},
	}
	if s.Plan != nil {
		m = append(m, billingMember{"plan", billingQuote(*s.Plan)})
	}
	if s.Subscription != nil {
		m = append(m, billingMember{"subscription", billingObject([]billingMember{
			{"tier", billingQuote(s.Subscription.Tier)}, {"billingSystem", billingQuote(s.Subscription.BillingSystem)},
		})})
	}
	if s.SubscriptionError != "" {
		m = append(m, billingMember{"subscriptionError", billingQuote(s.SubscriptionError)})
	}
	if s.Period != nil {
		p := []billingMember{{"kind", billingQuote(string(s.Period.Kind))}}
		billingOptDate(&p, "startsAt", s.Period.StartsAt)
		billingOptDate(&p, "resetsAt", s.Period.ResetsAt)
		m = append(m, billingMember{"period", billingObject(p)})
	}
	billingOptInt(&m, "includedLimitCents", s.IncludedLimitCents)
	billingOptInt(&m, "includedUsedCents", s.IncludedUsedCents)
	billingOptInt(&m, "prepaidBalanceCents", s.PrepaidBalanceCents)
	billingOptBool(&m, "onDemandEnabled", s.OnDemandEnabled)
	billingOptInt(&m, "onDemandUsedCents", s.OnDemandUsedCents)
	billingOptInt(&m, "onDemandLimitCents", s.OnDemandLimitCents)
	billingOptBool(&m, "isUnifiedBillingUser", s.IsUnifiedBillingUser)
	rows := "["
	for i, p := range s.ProductUsage {
		if i > 0 {
			rows += ","
		}
		rows += billingObject([]billingMember{
			{"product", billingQuote(p.Product)},
			{"usagePercent", swiftjson.FormatDouble(p.UsagePercent)},
		})
	}
	m = append(m, billingMember{"productUsage", rows + "]"})
	if s.ResetCredits != nil {
		m = append(m, billingMember{"resetCredits", billingResetJSON(*s.ResetCredits)})
	}
	return []byte(billingObject(m)), nil
}

func billingResetJSON(r ResetCreditSnapshot) string {
	tokens := "["
	for i, t := range r.Tokens {
		if i > 0 {
			tokens += ","
		}
		fields := []billingMember{{"tokenID", billingQuote(t.TokenID)}}
		billingOptDate(&fields, "validityStart", t.ValidityStart)
		billingOptDate(&fields, "validityEnd", t.ValidityEnd)
		tokens += billingObject(fields)
	}
	return billingObject([]billingMember{
		{"availableCount", strconv.Itoa(r.AvailableCount)},
		{"tokens", tokens + "]"},
		{"updatedAt", billingDateJSON(r.UpdatedAt)},
	})
}

// ErrBillingSnapshotJSON is returned for stored snapshot JSON that the
// Codable decoder would reject.
var ErrBillingSnapshotJSON = errors.New("grok: billing: invalid snapshot JSON")

// ParseBillingSnapshot reads the stored snapshot JSON (the shape of
// MarshalJSON) with the strictness of the synthesized Codable decoder:
// required keys must exist, present values must have the right type, and
// dates must be ISO 8601 strings.
func ParseBillingSnapshot(data []byte) (BillingSnapshot, error) {
	root, err := parseDecoderJSON(data)
	if err != nil || root.kind != jsonObject {
		return BillingSnapshot{}, ErrBillingSnapshotJSON
	}
	d := snapshotDecoder{}
	var s BillingSnapshot
	s.Plan = d.optString(root, "plan")
	if n := root.get("subscription"); n.present() {
		if n.kind != jsonObject {
			d.fail()
		} else {
			s.Subscription = &SubscriptionInfo{Tier: d.requiredString(n, "tier"), BillingSystem: d.requiredString(n, "billingSystem")}
		}
	}
	if value := d.optString(root, "subscriptionError"); value != nil {
		s.SubscriptionError = *value
	}
	s.IncludedUsagePercent = d.requiredDouble(root, "includedUsagePercent")
	if n := root.get("period"); n.present() {
		s.Period = d.period(n)
	}
	s.IncludedLimitCents = d.optInt(root, "includedLimitCents")
	s.IncludedUsedCents = d.optInt(root, "includedUsedCents")
	s.PrepaidBalanceCents = d.optInt(root, "prepaidBalanceCents")
	s.OnDemandEnabled = d.optBool(root, "onDemandEnabled")
	s.OnDemandUsedCents = d.optInt(root, "onDemandUsedCents")
	s.OnDemandLimitCents = d.optInt(root, "onDemandLimitCents")
	s.ProductUsage = []BillingProductUsage{}
	if n := root.get("productUsage"); n.present() {
		if n.kind != jsonArray {
			d.fail()
		} else {
			for _, row := range n.arr {
				if row.kind != jsonObject {
					d.fail()
					continue
				}
				s.ProductUsage = append(s.ProductUsage, BillingProductUsage{
					Product:      d.requiredString(row, "product"),
					UsagePercent: d.requiredDouble(row, "usagePercent"),
				})
			}
		}
	}
	s.IsUnifiedBillingUser = d.optBool(root, "isUnifiedBillingUser")
	if n := root.get("resetCredits"); n.present() {
		s.ResetCredits = d.resetCredits(n)
	}
	switch src := BillingSource(d.requiredString(root, "source")); src {
	case BillingSourceCurrent, BillingSourceDeprecated, BillingSourceLegacyFlat:
		s.Source = src
	default:
		d.fail()
	}
	s.UpdatedAt = d.requiredDate(root, "updatedAt")
	if d.failed {
		return BillingSnapshot{}, ErrBillingSnapshotJSON
	}
	return s, nil
}

type snapshotDecoder struct{ failed bool }

func (d *snapshotDecoder) fail() { d.failed = true }

func (d *snapshotDecoder) optString(o *jsonNode, key string) *string {
	n := o.get(key)
	if !n.present() {
		return nil
	}
	s, ok := n.str()
	if !ok {
		d.fail()
		return nil
	}
	return &s
}

func (d *snapshotDecoder) requiredString(o *jsonNode, key string) string {
	n := o.get(key)
	s, ok := n.str()
	if !ok {
		d.fail()
	}
	return s
}

func (d *snapshotDecoder) requiredDouble(o *jsonNode, key string) float64 {
	n := o.get(key)
	if n == nil || n.kind != jsonNumber || math.IsInf(billingNumberDouble(n.s), 0) {
		d.fail()
		return 0
	}
	return billingNumberDouble(n.s)
}

func (d *snapshotDecoder) optInt(o *jsonNode, key string) *int64 {
	n := o.get(key)
	if !n.present() {
		return nil
	}
	if n.kind == jsonNumber {
		if v, ok := billingNumberInt64(n.s); ok {
			return &v
		}
	}
	d.fail()
	return nil
}

func (d *snapshotDecoder) optBool(o *jsonNode, key string) *bool {
	n := o.get(key)
	if !n.present() {
		return nil
	}
	b, ok := n.boolean()
	if !ok {
		d.fail()
		return nil
	}
	return &b
}

func (d *snapshotDecoder) date(n *jsonNode) (time.Time, bool) {
	if s, ok := n.str(); ok {
		if t, ok := BillingParseDate(s); ok {
			return t, true
		}
	}
	d.fail()
	return time.Time{}, false
}

func (d *snapshotDecoder) requiredDate(o *jsonNode, key string) time.Time {
	n := o.get(key)
	if n == nil {
		d.fail()
		return time.Time{}
	}
	t, _ := d.date(n)
	return t
}

func (d *snapshotDecoder) optDate(o *jsonNode, key string) *time.Time {
	n := o.get(key)
	if !n.present() {
		return nil
	}
	if t, ok := d.date(n); ok {
		return &t
	}
	return nil
}

func (d *snapshotDecoder) period(n *jsonNode) *BillingPeriod {
	if n.kind != jsonObject {
		d.fail()
		return nil
	}
	kind := BillingPeriodKind(d.requiredString(n, "kind"))
	if kind != BillingWeekly && kind != BillingMonthly {
		d.fail()
	}
	return &BillingPeriod{Kind: kind, StartsAt: d.optDate(n, "startsAt"), ResetsAt: d.optDate(n, "resetsAt")}
}

func (d *snapshotDecoder) resetCredits(n *jsonNode) *ResetCreditSnapshot {
	if n.kind != jsonObject {
		d.fail()
		return nil
	}
	r := &ResetCreditSnapshot{Tokens: []ResetCreditToken{}}
	count := n.get("availableCount")
	if count == nil || count.kind != jsonNumber {
		d.fail()
	} else if v, ok := billingNumberInt64(count.s); ok {
		r.AvailableCount = int(v)
	} else {
		d.fail()
	}
	list := n.get("tokens")
	if list == nil || list.kind != jsonArray {
		d.fail()
	} else {
		for _, row := range list.arr {
			if row.kind != jsonObject {
				d.fail()
				continue
			}
			r.Tokens = append(r.Tokens, ResetCreditToken{
				TokenID:       d.requiredString(row, "tokenID"),
				ValidityStart: d.optDate(row, "validityStart"),
				ValidityEnd:   d.optDate(row, "validityEnd"),
			})
		}
	}
	r.UpdatedAt = d.requiredDate(n, "updatedAt")
	return r
}
