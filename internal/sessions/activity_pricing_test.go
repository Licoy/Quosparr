package sessions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

func TestRecentPricesRequestSamplesInsteadOfModelTotal(t *testing.T) {
	f := newFixture(t)
	id := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	lines := []string{fmt.Sprintf(`{"timestamp":"2026-10-14T00:00:00Z","type":"session_meta","payload":{"id":%q}}`, id)}
	for i, model := range []string{"known", "known", "unknown", "zero"} {
		tokens := 60000
		if model == "unknown" {
			tokens = 5
		}
		if model == "zero" {
			tokens = 0
		}
		lines = append(lines, fmt.Sprintf(`{"timestamp":"2026-10-14T00:0%d:00Z","type":"event_msg","payload":{"type":"token_count","model":%q,"info":{"last_token_usage":{"input_tokens":%d}}}}`, i+1, model, tokens))
	}
	path := f.write("sessions/2026/10/14/rollout-"+id+".jsonl", strings.Join(lines, "\n"))
	calls := 0
	pricer := func(model string, tokens Totals) (swiftjson.Decimal, bool) {
		calls++
		if model != "known" {
			return swiftjson.Decimal{}, false
		}
		if tokens.UncachedInputTokens >= 100000 {
			t.Fatal("cumulative usage sent as one request")
		}
		return swiftjson.MustDecimal("0"), true
	}
	result, err := (Scanner{CodexHome: f.home, Pricer: pricer}).Scan(1)
	if err != nil {
		t.Fatal(err)
	}
	item := result.Items[0]
	if item.EstimatedUSD == nil || item.EstimatedUSD.Sign() != 0 || item.UnpricedTokens != 5 || calls != 3 || item.SourcePath != path {
		t.Fatalf("item %+v calls=%d", item, calls)
	}
	without, err := (Scanner{CodexHome: f.home}).Scan(1)
	if err != nil {
		t.Fatal(err)
	}
	if without.Items[0].EstimatedUSD != nil || without.Items[0].UnpricedTokens != 120005 {
		t.Fatal(without)
	}
}

func TestCumulativeOnlyTokensRemainUnpriced(t *testing.T) {
	f := newFixture(t)
	id := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	f.write("sessions/2026/10/14/rollout-"+id+".jsonl", fmt.Sprintf(`{"timestamp":"2026-10-14T00:00:00Z","type":"session_meta","payload":{"id":%q}}
{"timestamp":"2026-10-14T00:01:00Z","type":"event_msg","payload":{"model":"known","info":{"total_token_usage":{"input_tokens":100001}}}}`, id))
	result, err := (Scanner{CodexHome: f.home, Pricer: func(string, Totals) (swiftjson.Decimal, bool) {
		t.Fatal("cumulative-only usage is not a request")
		return swiftjson.Decimal{}, true
	}}).Scan(1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].Totals.TotalTokens != 100001 || result.Items[0].EstimatedUSD != nil || result.Items[0].UnpricedTokens != 100001 {
		t.Fatal(result)
	}
}
