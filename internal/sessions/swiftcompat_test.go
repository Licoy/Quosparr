package sessions

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

// The frozen fixtures in testdata come from the Swift releases' scanner and
// repairer; the Go port must reproduce them.
func swiftCompatHome(t *testing.T) *fixture {
	f := newFixture(t)
	idx := f.write("session_index.jsonl", `{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","thread_name":"Index title \"quoted\"/slash","updated_at":"2026-06-29T00:04:00Z"}
{"id":"gone","thread_name":"Gone","updated_at":"2026-06-01T00:00:00Z"}`)
	f.touch(idx, "2026-06-29T00:04:00Z")
	a := f.write("sessions/2026/06/29/rollout-2026-06-29T00-00-00-aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.jsonl", `{"timestamp":"2026-06-29T00:00:00.500Z","type":"session_meta","payload":{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","cwd":"/Users/me/dev/quosparr"}}
{"timestamp":"2026-06-29T00:01:00Z","type":"event_msg","payload":{"type":"message","role":"user","content":"Fix the status bar"}}
{"timestamp":"2026-06-29T00:02:00Z","type":"event_msg","payload":{"type":"approval_request"}}
{"timestamp":"2026-06-29T00:03:00Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1000000,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":10}}},"turn_context":{"model":"gpt-5.3-codex-spark"}}
{"timestamp":"2026-06-29T00:04:00Z","type":"event_msg","payload":{"type":"task_complete"}}
`)
	f.touch(a, "2026-06-29T00:04:00Z")
	b := f.write("sessions/2026/06/30/rollout-bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.jsonl", `{"timestamp":"2026-06-30T00:00:00Z","type":"session_meta","payload":{"session_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}}
{"timestamp":"2026-06-30T00:01:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"  second\nmessage  "},{"type":"input_text","text":"parts"}]}}
{"timestamp":"2026-06-30T00:02:00Z","type":"event_msg","payload":{"type":"error","status":"failed"}}
{"timestamp":"2026-06-30T00:03:00Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":123456,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}},"turn_context":{"model":"gpt-5.3-codex-spark"}}
{"timestamp":"2026-06-30T00:03:30Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":50,"cached_input_tokens":10,"output_tokens":5,"reasoning_output_tokens":1}}},"turn_context":{"model":"mystery-model"}}
`)
	f.touch(b, "2026-06-30T00:03:30Z")
	return f
}

func TestActivityMatchesSwiftEncoding(t *testing.T) {
	f := swiftCompatHome(t)
	want, err := os.ReadFile("testdata/swift_activity.json")
	if err != nil {
		t.Fatal(err)
	}
	// The frozen Swift fixture priced cumulative-only totals as one request
	// Such rows now retain tokens without inventing a request-size tier
	var migrated map[string]any
	decoder := json.NewDecoder(bytes.NewReader(want))
	decoder.UseNumber()
	if err := decoder.Decode(&migrated); err != nil {
		t.Fatal(err)
	}
	for _, raw := range migrated["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
			delete(item, "estimatedUSD")
			item["unpricedTokens"] = int64(123511)
		}
	}
	want, err = swiftjson.Marshal(migrated, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := swiftjson.Marshal(f.scan(5), false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("activity JSON differs from Swift\n got: %s\nwant: %s", got, want)
	}
}

func TestRepairMatchesSwift(t *testing.T) {
	f := swiftCompatHome(t)
	repairer := Repairer{CodexHome: f.home}
	dry, err := repairer.DryRun()
	if err != nil {
		t.Fatal(err)
	}
	wantDry, err := os.ReadFile("testdata/swift_repair_dry.txt")
	if err != nil {
		t.Fatal(err)
	}
	gotDry := "missing=" + strings.Join(dry.MissingIndexIDs, ",") +
		"\norphan=" + strings.Join(dry.OrphanIndexIDs, ",") +
		"\nduplicate=" + strings.Join(dry.DuplicateIndexIDs, ",") +
		"\nstale=" + strings.Join(dry.StaleTitleIDs, ",") +
		"\nplanned=" + itoa(dry.PlannedEntries) + "\n"
	if gotDry != string(wantDry) {
		t.Fatalf("dry run differs\n got: %q\nwant: %q", gotDry, wantDry)
	}
	if _, err := repairer.Repair(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f.home + "/session_index.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/swift_repaired_index.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	// Swift's dictionary key order is nondeterministic, so compare per line
	// as decoded objects.
	gl, wl := strings.Split(strings.TrimSpace(string(got)), "\n"), strings.Split(strings.TrimSpace(string(want)), "\n")
	if len(gl) != len(wl) {
		t.Fatalf("line count %d != %d\n%s", len(gl), len(wl), got)
	}
	for i := range gl {
		var g, w map[string]any
		if err := json.Unmarshal([]byte(gl[i]), &g); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(wl[i]), &w); err != nil {
			t.Fatal(err)
		}
		if len(g) != len(w) {
			t.Fatalf("line %d: %v != %v", i, g, w)
		}
		for k, v := range w {
			if g[k] != v {
				t.Fatalf("line %d key %s: %v != %v", i, k, g[k], v)
			}
		}
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }
