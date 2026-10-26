package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/sessions"
)

func TestOptionalPartialPricingFieldsKeepOldSchema(t *testing.T) {
	for _, partial := range []bool{false, true} {
		p := NewProviderSnapshot(ProviderSnapshot{Provider: ProviderCodex, APIEquivalentCostPartial: partial})
		data, err := swiftjson.Marshal(p, false)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "apiEquivalentCostPartial") != partial {
			t.Fatalf("partial=%v %s", partial, data)
		}
		var decoded ProviderSnapshot
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.APIEquivalentCostPartial != partial {
			t.Fatal(decoded)
		}
	}
	var old ProviderSnapshot
	if err := json.Unmarshal([]byte(`{"provider":"codex","apiEquivalentCostUSD":0}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.APIEquivalentCostPartial || old.APIEquivalentCostUSD == nil {
		t.Fatal(old)
	}
	for _, count := range []int64{0, 5} {
		value := struct {
			Cost    CostSummary   `json:"cost"`
			Session sessions.Item `json:"session"`
		}{Cost: CostSummary{Confidence: "partiallyPriced", UnpricedTokens: count, DailyRows: []CostDailyRow{{UnpricedTokens: count}}, ModelRows: []CostBreakdownRow{{UnpricedTokens: count}}}, Session: sessions.Item{UnpricedTokens: count}}
		data, err := swiftjson.Marshal(value, false)
		if err != nil {
			t.Fatal(err)
		}
		want := 0
		if count > 0 {
			want = 4
		}
		if strings.Count(string(data), "unpricedTokens") != want {
			t.Fatalf("count=%d %s", count, data)
		}
	}
}
