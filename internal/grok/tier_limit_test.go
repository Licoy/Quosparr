package grok

import (
	"errors"
	"testing"
)

func TestMergePlanDoesNotInferTierFromMonthlyLimit(t *testing.T) {
	noSettings := billingPart{err: errors.New("offline")}
	limit := func(v int64) BillingSnapshot { return BillingSnapshot{IncludedLimitCents: &v} }
	plan := func(s BillingSnapshot) string {
		if s.Plan == nil {
			return ""
		}
		return *s.Plan
	}
	tests := []struct {
		name  string
		snap  BillingSnapshot
		token string
		want  string
	}{
		{"heavy limit", limit(150_000), "", ""},
		{"supergrok limit", limit(15_000), "", ""},
		{"known claim beats limit", limit(150_000), syntheticTierToken(7), "SuperGrok Plus"},
		{"limit cannot replace unmapped claim", limit(15_000), syntheticTierToken(42), "Tier 42"},
		{"unmapped claim is last", limit(9_999), syntheticTierToken(42), "Tier 42"},
		{"nothing known", limit(9_999), "", ""},
	}
	for _, tc := range tests {
		if got := plan(mergePlan(tc.snap, noSettings, tc.token)); got != tc.want {
			t.Errorf("%s: plan %q, want %q", tc.name, got, tc.want)
		}
	}
	settings := billingPart{data: []byte(`{"subscription_tier_display":"SuperGrok Lite"}`)}
	if got := plan(mergePlan(limit(150_000), settings, "")); got != "SuperGrok Lite" {
		t.Fatalf("settings plan %q must beat the limit", got)
	}
}
