package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// The Pro quota response was captured from the official desktop client's
// RetrieveUserQuotaSummary RPC. It contains no credential or account identity.
// The server below reproduces Cloud Code's observed subclient filtering.
func TestQuotaFetchUsesHubIdentityForArgon(t *testing.T) {
	raw, err := os.ReadFile("testdata/quota_summary_pro.json")
	if err != nil {
		t.Fatal(err)
	}
	var full summaryResponse
	if err = json.Unmarshal(raw, &full); err != nil {
		t.Fatal(err)
	}
	if len(full.Groups) != 3 {
		t.Fatal("fixture must contain the independent Argon group")
	}
	shared := full
	shared.Groups = full.Groups[:2]
	sharedRaw, err := json.Marshal(shared)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, tier         string
		summaryUnavailable bool
	}{
		{name: "pro", tier: "g1-pro-tier"},
		{name: "ultra shared pool", tier: "g1-ultra-tier"},
		{name: "pro model fallback", tier: "g1-pro-tier", summaryUnavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newCloudServer(t, map[string]func(http.ResponseWriter, cloudCall){
				"/v1internal:loadCodeAssist": reply(`{"cloudaicompanionProject":"project","paidTier":{"id":"` + tc.tier + `"}}`),
				"/v1internal:retrieveUserQuotaSummary": func(w http.ResponseWriter, call cloudCall) {
					if tc.summaryUnavailable {
						http.Error(w, "unavailable", http.StatusNotFound)
						return
					}
					if tc.tier == "g1-pro-tier" && strings.HasPrefix(call.ua, "antigravity/hub/") {
						_, _ = w.Write(raw)
					} else {
						_, _ = w.Write(sharedRaw)
					}
				},
				"/v1internal:fetchAvailableModels": func(w http.ResponseWriter, call cloudCall) {
					if strings.HasPrefix(call.ua, "antigravity/hub/") {
						_, _ = w.Write([]byte(`{"models":{"gemini-4-argon-high":{"displayName":"Gemini 4 Argon (High)","quotaInfo":{"remainingFraction":0.47933266,"resetTime":"2026-11-23T07:59:09Z"}}}}`))
					} else {
						_, _ = w.Write([]byte(`{"models":{}}`))
					}
				},
			})
			snap, err := testQuotaClient(server.URL).Fetch(context.Background(), "fixture-token")
			if err != nil {
				t.Fatal(err)
			}
			if len(snap.Models) != 1 || snap.Models[0].ID != "gemini-4-argon-high" || snap.Models[0].RemainingFraction == nil || *snap.Models[0].RemainingFraction != 0.47933266 {
				t.Fatalf("hub model quota missing: %+v", snap.Models)
			}
			if tc.summaryUnavailable {
				if snap.Source != SourceModels || len(snap.Buckets) != 0 {
					t.Fatalf("model fallback: %+v", snap)
				}
				return
			}
			wantBuckets := 4
			if tc.tier == "g1-pro-tier" {
				wantBuckets = 5
			}
			if snap.Source != SourceSummary || len(snap.Buckets) != wantBuckets {
				t.Fatalf("quota pools = %+v", snap.Buckets)
			}
			var argon *QuotaBucket
			for i := range snap.Buckets {
				if snap.Buckets[i].Family == FamilyArgon {
					argon = &snap.Buckets[i]
				}
			}
			if tc.tier != "g1-pro-tier" {
				if argon != nil {
					t.Fatal("shared pool must not synthesize an Argon bucket")
				}
				return
			}
			reset := time.Date(2026, 11, 23, 7, 59, 9, 0, time.UTC)
			if argon == nil || argon.ID != "gemini-argon-wtu-weekly" || argon.Window != WindowWeekly || argon.RemainingFraction != 0.47933266 || argon.ResetAt == nil || !argon.ResetAt.Equal(reset) {
				t.Fatalf("independent Argon quota: %+v", argon)
			}
			if snap.Buckets[0].Family != FamilyGemini || snap.Buckets[0].RemainingFraction != 0.87981594 {
				t.Fatalf("general Gemini pool: %+v", snap.Buckets[0])
			}
		})
	}
}
