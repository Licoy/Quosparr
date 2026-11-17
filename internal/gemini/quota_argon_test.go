package gemini

import "testing"

// These are compatibility cases, not a captured account response: pools may
// share a bucket id, while the group and explicit window identify Argon.
func TestQuotaSummaryKeepsArgonIndependent(t *testing.T) {
	for _, id := range []string{"gemini-argon-wtu-weekly", "gemini-argon-weekly", "gemini-weekly", "argon-quota"} {
		t.Run(id, func(t *testing.T) {
			raw := []byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.89}]},{"displayName":"Gemini 4 Argon","buckets":[{"bucketId":"` + id + `","window":"weekly","remainingFraction":0.48,"resetTime":"2026-11-23T01:43:47Z"}]}]}`)
			buckets, err := parseQuotaSummary(raw)
			if err != nil {
				t.Fatal(err)
			}
			if len(buckets) != 2 {
				t.Fatalf("buckets = %+v", buckets)
			}
			general, argon := buckets[0], buckets[1]
			if general.Family != FamilyGemini || general.RemainingFraction != 0.89 || argon.Family != FamilyArgon || argon.Window != WindowWeekly || argon.RemainingFraction != 0.48 || argon.ResetAt == nil {
				t.Fatalf("independent pools = %+v", buckets)
			}
		})
	}
}

func TestQuotaSummaryUsesExplicitWindowAndKeepsLegacyIDs(t *testing.T) {
	buckets, err := parseQuotaSummary([]byte(`{"groups":[{"buckets":[
 {"bucketId":"gemini-argon-wtu-weekly","remainingFraction":0.48},
 {"bucketId":"unknown-pool","window":"weekly","remainingFraction":0.7},
 {"bucketId":"gemini-weekly","window":"5h","remainingFraction":0.89}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 3 || buckets[0].Family != FamilyArgon || buckets[0].Window != WindowWeekly || buckets[1].Family != FamilyOther || buckets[1].Window != WindowWeekly || buckets[2].Window != WindowFiveHour {
		t.Fatalf("buckets = %+v", buckets)
	}
}
