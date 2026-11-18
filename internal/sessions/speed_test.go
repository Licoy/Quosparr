package sessions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/speed"
)

// speedLog writes a synthetic rollout; offsets are seconds after base.
type speedLog struct {
	base time.Time
	sb   strings.Builder
}

func (l *speedLog) at(sec float64) time.Time {
	return l.base.Add(time.Duration(sec * float64(time.Second)))
}

func (l *speedLog) line(sec float64, typ, payload string) {
	fmt.Fprintf(&l.sb, `{"timestamp":%q,"type":%q,"payload":%s}`+"\n",
		l.at(sec).UTC().Format("2006-01-02T15:04:05.000Z"), typ, payload)
}

func (l *speedLog) meta(id, cwd string) {
	l.line(0, "session_meta", fmt.Sprintf(`{"id":%q,"cwd":%q}`, id, cwd))
}

func (l *speedLog) taskStarted(sec float64) { l.line(sec, "event_msg", `{"type":"task_started"}`) }

func (l *speedLog) turnContext(sec float64, model, effort string) {
	l.line(sec, "turn_context", fmt.Sprintf(`{"model":%q,"effort":%q,"cwd":"/x"}`, model, effort))
}

// settings is the thread settings line Codex writes before a turn; the model
// inside it must not be taken for the turn's model.
func (l *speedLog) settings(sec float64, tier string) {
	l.line(sec, "event_msg", fmt.Sprintf(`{"type":"thread_settings_applied","thread_settings":{"model":"other","service_tier":%q}}`, tier))
}

func (l *speedLog) item(sec float64, kind string, startedSec float64) {
	l.line(sec, "event_msg", fmt.Sprintf(`{"type":"item_completed","item":{"type":%q},"started_at_ms":%d,"completed_at_ms":%d}`,
		kind, l.at(startedSec).UnixMilli(), l.at(sec).UnixMilli()))
}

func (l *speedLog) toolOutput(sec float64, kind string) {
	l.line(sec, "response_item", fmt.Sprintf(`{"type":%q,"output":"ok"}`, kind))
}

func (l *speedLog) record(sec float64, output int) {
	l.line(sec, "token_usage_record", fmt.Sprintf(`{"response_id":"resp_1","usage":{"output_tokens":%d,"reasoning_output_tokens":1}}`, output))
}

func (l *speedLog) write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(l.sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func scanOneFile(t *testing.T, l *speedLog) []speed.Sample {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	l.write(t, path)
	samples, err := scanSpeedFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return samples
}

func wantSample(t *testing.T, got speed.Sample, at time.Time, ttft time.Duration, hasTTFT bool, tokens int64, decode time.Duration) {
	t.Helper()
	if !got.At.Equal(at) || got.HasTTFT != hasTTFT || got.TTFT != ttft || got.OutputTokens != tokens || got.Decode != decode {
		t.Fatalf("sample = %+v, want at %v ttft %v (%v) tokens %d decode %v", got, at, ttft, hasTTFT, tokens, decode)
	}
}

func TestScanSpeedFileFollowsRequests(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.meta("id", "/proj")
	l.taskStarted(0)
	l.turnContext(4, "gpt-test", "high") // 4s of setup after the task started
	l.item(4.01, "UserMessage", 4.01)
	l.item(8, "Reasoning", 6)
	l.record(10, 100) // request 4, first item 6, end 10
	l.toolOutput(11, "function_call_output")
	l.item(15, "AgentMessage", 12.5)
	l.item(15.1, "CommandExecution", 12.9) // not a stream marker
	l.record(15.2, 50)
	l.toolOutput(16, "custom_tool_call_output")
	l.record(18, 7) // nothing streamed: no sample

	samples := scanOneFile(t, l)
	if len(samples) != 2 {
		t.Fatalf("samples = %+v", samples)
	}
	for _, s := range samples {
		if s.Model != "gpt-test" || s.Effort != "high" {
			t.Fatalf("model/effort = %q/%q", s.Model, s.Effort)
		}
	}
	wantSample(t, samples[0], l.at(4), 2*time.Second, true, 100, 4*time.Second)
	wantSample(t, samples[1], l.at(11), 1500*time.Millisecond, true, 50, 2700*time.Millisecond)
}

func TestScanSpeedFileTracksServiceTier(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	turn := func(sec float64) {
		l.taskStarted(sec)
		l.turnContext(sec, "m", "high")
		l.item(sec+2, "Reasoning", sec+1)
		l.record(sec+3, 10)
	}
	turn(0) // before any settings line: unknown
	l.settings(9, "priority")
	turn(10)
	l.settings(19, "ultrafast")
	turn(20)
	l.settings(29, "default")
	turn(30)
	l.settings(39, "flex")
	turn(40)

	samples := scanOneFile(t, l)
	want := []speed.Tier{speed.TierUnknown, speed.TierFast, speed.TierUltrafast, speed.TierStandard, speed.TierUnknown}
	if len(samples) != len(want) {
		t.Fatalf("samples = %+v", samples)
	}
	for i, s := range samples {
		if s.Tier != want[i] || s.Model != "m" {
			t.Fatalf("sample %d = %+v, want tier %q", i, s, want[i])
		}
	}
}

func TestScanSpeedFileToolOutputDuringStream(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.taskStarted(0)
	l.turnContext(0, "m", "")
	l.item(2, "Reasoning", 1)
	l.toolOutput(3, "function_call_output") // a tool the model already called; the stream goes on
	l.record(5, 40)
	l.item(7, "AgentMessage", 5.5) // the next request went out when the response ended
	l.record(8, 10)

	samples := scanOneFile(t, l)
	if len(samples) != 2 {
		t.Fatalf("samples = %+v", samples)
	}
	wantSample(t, samples[0], l.at(0), time.Second, true, 40, 4*time.Second)
	wantSample(t, samples[1], l.at(5), 500*time.Millisecond, true, 10, 2500*time.Millisecond)
}

func TestScanSpeedFileIgnoresStaleItemsAndTurns(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.taskStarted(0)
	l.turnContext(0, "m1", "low")
	l.toolOutput(10, "function_call_output")
	l.item(11, "Reasoning", 9) // started before the request: not this response
	l.item(13, "Reasoning", 12)
	l.record(14, 20)
	l.taskStarted(20)
	l.turnContext(20, "m2", "") // the effort is unknown for this turn
	l.item(22, "AgentMessage", 21)
	l.record(23, 5)

	samples := scanOneFile(t, l)
	if len(samples) != 2 {
		t.Fatalf("samples = %+v", samples)
	}
	wantSample(t, samples[0], l.at(10), 2*time.Second, true, 20, 2*time.Second)
	if samples[0].Model != "m1" || samples[0].Effort != "low" {
		t.Fatalf("first = %+v", samples[0])
	}
	if samples[1].Model != "m2" || samples[1].Effort != "" {
		t.Fatalf("second = %+v", samples[1])
	}
}

func TestScanSpeedFileStaleRequestKeepsOnlyDecode(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.taskStarted(0)
	l.turnContext(0, "m", "")
	l.item(3600, "Reasoning", 3599) // an hour after the request
	l.record(3601, 30)

	samples := scanOneFile(t, l)
	if len(samples) != 1 {
		t.Fatalf("samples = %+v", samples)
	}
	wantSample(t, samples[0], l.at(0), 0, false, 30, 2*time.Second)
}

func TestScanSpeedFileIgnoresMarkersInsideText(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.taskStarted(0)
	l.turnContext(0, "m", "")
	l.item(2, "Reasoning", 1)
	// Command output quoting a record: the quotes are escaped inside the string.
	l.line(3, "event_msg", `{"type":"item_completed","item":{"type":"CommandExecution","aggregated_output":"{\"type\":\"token_usage_record\",\"usage\":{\"output_tokens\":999}}"}}`)
	l.sb.WriteString("{not json at all \"type\":\"token_usage_record\"\n")
	l.sb.WriteString(`{"timestamp":"bad","type":"token_usage_record","payload":{"usage":{"output_tokens":5}}}` + "\n")
	l.record(5, 40)

	samples := scanOneFile(t, l)
	if len(samples) != 1 {
		t.Fatalf("samples = %+v", samples)
	}
	wantSample(t, samples[0], l.at(0), time.Second, true, 40, 4*time.Second)
}

func TestScanSpeedFileWithoutTurnStartHasNoSample(t *testing.T) {
	// A file cut mid-turn: the response has no request time to measure from.
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.item(2, "Reasoning", 1)
	l.record(5, 40)
	if samples := scanOneFile(t, l); len(samples) != 0 {
		t.Fatalf("samples = %+v", samples)
	}
}

func TestScanSpeedFileReadsOnlyTheTail(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	// An old turn, then more than the tail limit of filler, then a new turn.
	l.taskStarted(0)
	l.turnContext(0, "old", "")
	l.settings(0, "priority")
	l.item(2, "Reasoning", 1)
	l.record(3, 10)
	filler := fmt.Sprintf(`{"type":"event_msg","payload":{"type":"exec_command_output_delta","chunk":%q}}`+"\n", strings.Repeat("x", 1<<20))
	for i := 0; i < speed.TailBytes/(1<<20)+2; i++ {
		l.sb.WriteString(filler)
	}
	l.taskStarted(100)
	l.turnContext(100, "new", "high")
	l.settings(100, "default")
	l.item(102, "Reasoning", 101)
	l.record(103, 20)

	samples := scanOneFile(t, l)
	if len(samples) != 1 || samples[0].Model != "new" || samples[0].Effort != "high" || samples[0].Tier != speed.TierStandard {
		t.Fatalf("samples = %+v", samples)
	}
}

func TestScanSpeedFileRestoresContextBeforeTail(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)}
	l.taskStarted(0)
	l.turnContext(0, "gpt-old", "low")
	l.settings(0, "default")
	l.turnContext(1, "gpt-test", "high")
	l.settings(1, "priority")
	filler := fmt.Sprintf(`{"type":"event_msg","payload":{"type":"exec_command_output_delta","chunk":%q}}`+"\n", strings.Repeat("x", speed.TailBytes+1))
	l.sb.WriteString(filler)
	l.toolOutput(100, "function_call_output")
	l.item(102, "Reasoning", 101)
	l.record(103, 100)

	samples := scanOneFile(t, l)
	if len(samples) != 1 {
		t.Fatalf("samples = %+v", samples)
	}
	if got := samples[0]; got.Model != "gpt-test" || got.Effort != "high" || got.Tier != speed.TierFast {
		t.Fatalf("restored context = %+v", got)
	}
}

func TestScanSpeedFileHonoursCancel(t *testing.T) {
	l := &speedLog{base: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)}
	l.taskStarted(0)
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	l.write(t, path)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanSpeedFile(ctx, path); err == nil {
		t.Fatal("cancelled scan returned no error")
	}
}

// writeSpeedSession writes one session file with a sample, dated by mtime.
func writeSpeedSession(t *testing.T, home, folder, id string, active time.Time, title string) {
	t.Helper()
	l := &speedLog{base: active.Add(-time.Minute)}
	l.meta(id, "/work/"+id[:4])
	l.line(0.5, "response_item", fmt.Sprintf(`{"type":"message","role":"user","content":%q}`, title))
	l.taskStarted(1)
	l.turnContext(1, "gpt-test", "high")
	l.item(4, "Reasoning", 3)
	l.record(6, 100)
	path := filepath.Join(home, folder, "2026", "10", "14", "rollout-2026-10-14T00-00-00-"+id+".jsonl")
	l.write(t, path)
	if err := os.Chtimes(path, active, active); err != nil {
		t.Fatal(err)
	}
}

func uuid(n int) string { return fmt.Sprintf("%08d-0000-0000-0000-000000000000", n) }

func TestScanSpeedPicksNewestSessionsInsideTheWindow(t *testing.T) {
	home := t.TempDir()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	// 12 recent sessions, one hour apart, session 12 the newest.
	for i := 1; i <= 12; i++ {
		writeSpeedSession(t, home, "sessions", uuid(i), now.Add(-time.Duration(13-i)*time.Hour), fmt.Sprintf("title %d", i))
	}
	writeSpeedSession(t, home, "sessions", uuid(99), now.Add(-speed.Window-time.Hour), "too old")
	// The same session archived: only the newer copy counts.
	writeSpeedSession(t, home, "archived_sessions", uuid(12), now.Add(-30*time.Hour), "archived copy")
	index := fmt.Sprintf(`{"id":%q,"thread_name":"Indexed name","updated_at":"2026-10-14T11:00:00Z"}`+"\n", uuid(12))
	if err := os.WriteFile(filepath.Join(home, IndexFileName), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Scanner{CodexHome: home}.ScanSpeed(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != speed.SessionLimit {
		t.Fatalf("got %d sessions, want %d", len(got), speed.SessionLimit)
	}
	if got[0].ID != uuid(12) || got[0].Title != "Indexed name" || got[0].ProjectName != "0000" {
		t.Fatalf("newest = %+v", got[0])
	}
	if got[1].Title != "title 11" || got[len(got)-1].ID != uuid(3) {
		t.Fatalf("order: second %q, last %s", got[1].Title, got[len(got)-1].ID)
	}
	for _, s := range got {
		if s.ID == uuid(99) {
			t.Fatal("a session outside the window was read")
		}
		if len(s.Samples) != 1 || s.Samples[0].Model != "gpt-test" || s.Samples[0].Effort != "high" {
			t.Fatalf("session %s samples = %+v", s.ID, s.Samples)
		}
	}
}

func TestScanSpeedMissingHomeIsEmpty(t *testing.T) {
	got, err := Scanner{CodexHome: filepath.Join(t.TempDir(), "none")}.ScanSpeed(context.Background(), time.Now())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, err %v", got, err)
	}
}
