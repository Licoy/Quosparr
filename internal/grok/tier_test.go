package grok

import (
	"encoding/base64"
	"strconv"
	"testing"
)

// TestTierMatchesSwift replays JWT payloads, tokens and tier names through
// GrokSubscriptionTier.
func TestTierMatchesSwift(t *testing.T) {
	var inputs []struct{ Kind, V string }
	var golden []map[string]any
	readJSON(t, "testdata/billing_tier_in.json", &inputs)
	readJSON(t, "testdata/billing_tier_out.json", &golden)
	if len(inputs) != len(golden) {
		t.Fatalf("%d inputs, %d goldens", len(inputs), len(golden))
	}
	for i, in := range inputs {
		want := golden[i]
		// The new exact product names supersede these two frozen Swift expectations
		// without changing the original fixtures or the remaining compatibility cases
		if in.Kind == "raw" && (in.V == "Plus" || in.V == "Premium") {
			want = map[string]any{"tier": "X · " + in.V, "name": "X · " + in.V}
		}
		switch in.Kind {
		case "payload", "token":
			token := in.V
			if in.Kind == "payload" {
				token = "h." + base64.RawURLEncoding.EncodeToString([]byte(in.V)) + ".s"
			}
			var claim, name any
			if c, ok := TierJWTClaim(token); ok {
				claim = strconv.FormatUint(c, 10)
			}
			if n, ok := TierDisplayNameFromAccessToken(token); ok {
				name = n
			}
			if claim != want["claim"] || name != want["name"] {
				t.Errorf("%d %s %q: claim %v name %v, Swift %v %v", i, in.Kind, in.V, claim, name, want["claim"], want["name"])
			}
		case "raw":
			var name any
			if n, ok := TierDisplayName(in.V); ok {
				name = n
			}
			if got := TierResolve(in.V).DisplayName(); got != want["tier"] || name != want["name"] {
				t.Errorf("%d raw %q: tier %q name %v, Swift %v %v", i, in.V, got, name, want["tier"], want["name"])
			}
		}
	}
}
