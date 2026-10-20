package store

import (
	"encoding/json"
	"fmt"
	"slices"
)

// DefaultQuotaAlertThresholds returns independent used-quota percentages.
func DefaultQuotaAlertThresholds() []int { return []int{80, 95, 100} }

// NormalizeQuotaAlertThresholds validates, sorts and deduplicates a copy.
// Empty lists are invalid; disable quota alerts with QuotaAlertsEnabled.
func NormalizeQuotaAlertThresholds(thresholds []int) ([]int, error) {
	if len(thresholds) == 0 {
		return nil, fmt.Errorf("at least one quota alert threshold is required")
	}
	for _, threshold := range thresholds {
		if threshold < 0 || threshold > 100 {
			return nil, fmt.Errorf("quota alert thresholds must be between 0 and 100")
		}
	}
	out := slices.Clone(thresholds)
	slices.Sort(out)
	return slices.Compact(out), nil
}

func readQuotaAlertThresholds(r *reader) []int {
	const key = "quotaAlertThresholds"
	raw, ok := r.take(key)
	if !ok {
		return DefaultQuotaAlertThresholds()
	}
	var values []*int
	if json.Unmarshal(raw, &values) != nil {
		r.fail(key, "expected an array of integers")
		return nil
	}
	thresholds := make([]int, 0, len(values))
	for _, value := range values {
		if value == nil {
			r.fail(key, "expected an array of integers")
			return nil
		}
		thresholds = append(thresholds, *value)
	}
	out, err := NormalizeQuotaAlertThresholds(thresholds)
	if err != nil {
		r.fail(key, "%v", err)
	}
	return out
}
