package cursor

import (
	"math"
	"regexp"
	"strings"
)

// Model pools follow the usage page: tier 2 and Cursor's own model names are
// "Cursor models", tier 1 is "other models". Bot (sand) usage is billed
// outside both pools and is left out.
var (
	cursorModelName = regexp.MustCompile(`^(composer|cursor-grok)([-_]|$)`)
	botModelToken   = regexp.MustCompile(`(^|[-_.])(bot|sand)([-_.]|$)`)
)

const maxInvertPercent = 99.5

func spendPool(model string, tier int64) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return ""
	}
	if !strings.Contains(name, "cursor-grok") && (botModelToken.MatchString(name) || strings.Contains(name, "grok-bot") ||
		strings.Contains(name, "grok-api") || strings.Contains(name, "grok-agents")) {
		return ""
	}
	if tier == 2 || cursorModelName.MatchString(name) {
		return "auto"
	}
	if tier == 1 {
		return "api"
	}
	return ""
}

// ParseSpend sums the current cycle's aggregated usage events by model pool,
// in whole cents. Event costs are fractional cents.
func ParseSpend(body []byte) (map[string]int64, error) {
	m, err := readMessage(body)
	if err != nil {
		return nil, err
	}
	sums := map[string]float64{}
	for _, v := range m[1] {
		if v.kind != 2 {
			return nil, ErrResponse
		}
		row, err := readMessage(v.data)
		if err != nil {
			return nil, err
		}
		model, err := row.text(1)
		if err != nil {
			return nil, err
		}
		cents, err := row.amount(6)
		if err != nil {
			return nil, err
		}
		tier, err := row.integer(8)
		if err != nil {
			return nil, err
		}
		if pool := spendPool(model, number(tier)); pool != "" && cents != nil {
			sums[pool] += *cents
		}
	}
	out := make(map[string]int64, len(sums))
	for k, v := range sums {
		out[k] = int64(math.Round(v))
	}
	return out, nil
}

// ApplySpend sets each model pool's spend and, when the server reports no
// amounts, estimates its cap as spend ÷ used percentage rounded to $10.
// A pool at or near 100% or without spend cannot be inverted.
func ApplySpend(pools []Pool, spend map[string]int64) {
	for i := range pools {
		p := &pools[i]
		used, ok := spend[p.Kind]
		if !ok || p.UsedCents != nil || p.LimitCents != nil {
			continue
		}
		p.UsedCents = &used
		if p.UsedPercent == nil || used <= 0 || *p.UsedPercent <= 0 || *p.UsedPercent >= maxInvertPercent {
			continue
		}
		limit := int64(math.Round(float64(used) / *p.UsedPercent * 100 / 1000)) * 1000
		if limit > 0 {
			p.LimitCents, p.LimitEstimated = &limit, true
		}
	}
}

// amount reads a double that, unlike a percentage, may be negative (refunds).
func (m message) amount(key uint32) (*float64, error) {
	v, ok, err := m.one(key, 1)
	if err != nil || !ok {
		return nil, err
	}
	n := math.Float64frombits(v.n)
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, ErrResponse
	}
	return &n, nil
}
