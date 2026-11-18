package sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sort"
	"time"

	"github.com/licoy/quosparr/internal/speed"
)

// ScanSpeed reads the model speed samples of the newest speed.SessionLimit
// sessions active within speed.Window before now, newest first. Only the end
// of each file is read; a file that cannot be read fails the scan.
func (s Scanner) ScanSpeed(ctx context.Context, now time.Time) ([]speed.Session, error) {
	index, err := readIndex(s.CodexHome)
	if err != nil {
		return nil, err
	}
	titles := map[string]string{}
	for _, e := range index {
		titles[e.ID] = e.ThreadName
	}
	var result []speed.Session
	for _, c := range speedCandidates(s.CodexHome, now) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		session, err := s.readSpeedSession(ctx, c.path, titles, now)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if session != nil {
			result = append(result, *session)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result[:min(len(result), speed.SessionLimit)], nil
}

// speedCandidates are the newest rollout files by modification time inside the
// window, one per session id, at most speed.SessionLimit.
func speedCandidates(codexHome string, now time.Time) []fileCandidate {
	cutoff := now.Add(-speed.Window)
	byID := map[string]fileCandidate{}
	for _, f := range jsonlFiles(codexHome) {
		if f.id == "" || f.activityDate.Before(cutoff) {
			continue
		}
		if existing, ok := byID[f.id]; ok && !f.newerThan(existing) {
			continue
		}
		byID[f.id] = f
	}
	files := make([]fileCandidate, 0, len(byID))
	for _, f := range byID {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].newerThan(files[j]) })
	return files[:min(len(files), speed.SessionLimit)]
}

// readSpeedSession identifies a session like the recent list does and reads
// its samples; nil means the session has none inside the window.
func (s Scanner) readSpeedSession(ctx context.Context, path string, titles map[string]string, now time.Time) (*speed.Session, error) {
	item, ok, err := s.parseSession(path)
	if err != nil || !ok || item.UpdatedAt.Before(now.Add(-speed.Window)) {
		return nil, err
	}
	samples, err := scanSpeedFile(ctx, path)
	if err != nil {
		return nil, err
	}
	title := item.Title
	if t := cleanTitle(titles[item.ID]); t != "" {
		title = t
	}
	return &speed.Session{
		ID: item.ID, Title: title, ProjectName: item.ProjectName,
		UpdatedAt: item.UpdatedAt, Samples: samples,
	}, nil
}

// scanSpeedFile reads the speed samples out of the end of one rollout file.
func scanSpeedFile(ctx context.Context, path string) ([]speed.Sample, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data, start, _, err := speed.ReadTailAt(f, info.Size(), speed.TailBytes)
	if err != nil {
		return nil, err
	}
	var c speedCursor
	if start > 0 {
		if err := restoreSpeedContext(ctx, f, start, &c); err != nil {
			return nil, err
		}
	}
	err = speed.EachLine(ctx, data, c.line)
	return c.samples, err
}

// Markers that pick the few line kinds worth decoding; the rest, including the
// huge command output lines, are never parsed. Quotes inside string values are
// escaped, so a marker only matches real JSON keys and values.
var (
	markTurnContext   = []byte(`"type":"turn_context"`)
	markTaskStarted   = []byte(`"type":"task_started"`)
	markFuncOutput    = []byte(`"type":"function_call_output"`)
	markCustomOutput  = []byte(`"type":"custom_tool_call_output"`)
	markItemCompleted = []byte(`"type":"item_completed"`)
	markReasoning     = []byte(`"type":"Reasoning"`)
	markAgentMessage  = []byte(`"type":"AgentMessage"`)
	markUsageRecord   = []byte(`"type":"token_usage_record"`)
	markSettings      = []byte(`"type":"thread_settings_applied"`)
)

// speedLine is the part of a rollout line the speed scan reads.
type speedLine struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type        string `json:"type"`
		Model       string `json:"model"`
		Effort      string `json:"effort"`
		StartedAtMs int64  `json:"started_at_ms"`
		Item        struct {
			Type string `json:"type"`
		} `json:"item"`
		Usage struct {
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
		ThreadSettings struct {
			ServiceTier string `json:"service_tier"`
		} `json:"thread_settings"`
	} `json:"payload"`
}

// speedCursor follows the requests of a session through its lines. A request
// is sent at the turn start or when the last tool output arrived; the first
// reasoning or message item that started after it is the first streamed token,
// and the response ends at its token_usage_record. The service tier comes
// from the thread settings Codex writes before a turn whenever it applies them;
// it stays unknown until the first such line.
type speedCursor struct {
	model, effort string
	tier          speed.Tier

	requestAt time.Time
	hasReq    bool
	streamAt  time.Time
	hasStream bool

	samples []speed.Sample
}

func (c *speedCursor) line(line []byte) {
	if !wantedSpeedLine(line) {
		return
	}
	var l speedLine
	if json.Unmarshal(line, &l) != nil {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return
	}
	switch l.Type {
	case "turn_context":
		c.turnContext(at, l)
	case "event_msg":
		c.eventMsg(at, l)
	case "response_item":
		if t := l.Payload.Type; t == "function_call_output" || t == "custom_tool_call_output" {
			c.toolOutput(at)
		}
	case "token_usage_record":
		c.responseDone(at, l.Payload.Usage.OutputTokens)
	}
}

func wantedSpeedLine(line []byte) bool {
	switch {
	case bytes.Contains(line, markUsageRecord), bytes.Contains(line, markTurnContext),
		bytes.Contains(line, markTaskStarted), bytes.Contains(line, markFuncOutput),
		bytes.Contains(line, markCustomOutput), bytes.Contains(line, markSettings):
		return true
	case bytes.Contains(line, markItemCompleted):
		return bytes.Contains(line, markReasoning) || bytes.Contains(line, markAgentMessage)
	}
	return false
}

func (c *speedCursor) turnContext(at time.Time, l speedLine) {
	c.model, c.effort = l.Payload.Model, l.Payload.Effort
	// The turn starts a moment before its first request when the session was
	// still setting up; the context is written right before the request.
	if c.hasReq && !c.hasStream && at.After(c.requestAt) {
		c.requestAt = at
	}
}

func (c *speedCursor) eventMsg(at time.Time, l speedLine) {
	switch l.Payload.Type {
	case "task_started":
		c.requestAt, c.hasReq, c.hasStream = at, true, false
	case "item_completed":
		c.itemCompleted(l)
	case "thread_settings_applied":
		c.tier = codexTier(l.Payload.ThreadSettings.ServiceTier)
	}
}

// toolOutput is the next request going out. An output that arrives while a
// response still streams belongs to a tool the model already called; the next
// request then goes out when that response ends.
func (c *speedCursor) toolOutput(at time.Time) {
	if !c.hasStream {
		c.requestAt, c.hasReq = at, true
	}
}

func (c *speedCursor) itemCompleted(l speedLine) {
	if c.hasStream || !c.hasReq {
		return
	}
	if t := l.Payload.Item.Type; t != "Reasoning" && t != "AgentMessage" {
		return
	}
	started := time.UnixMilli(l.Payload.StartedAtMs)
	if l.Payload.StartedAtMs > 0 && !started.Before(c.requestAt) {
		c.streamAt, c.hasStream = started, true
	}
}

// responseDone closes the response that ended at `end` and starts waiting for
// the next request, which cannot go out before now.
func (c *speedCursor) responseDone(end time.Time, outputTokens int64) {
	if c.hasReq {
		if s, ok := c.sample(end, outputTokens); ok {
			c.samples = append(c.samples, s)
		}
	}
	c.requestAt, c.hasReq, c.hasStream = end, true, false
}

// sample is the measurement of the response that just ended; a response with
// neither a first token nor a rate to report is no sample.
func (c *speedCursor) sample(end time.Time, outputTokens int64) (speed.Sample, bool) {
	s := speed.Sample{At: c.requestAt, Model: c.model, Effort: c.effort, Tier: c.tier}
	if c.hasStream {
		if wait := c.streamAt.Sub(c.requestAt); wait <= speed.MaxTTFT {
			s.TTFT, s.HasTTFT = wait, true
		}
		if decode := end.Sub(c.streamAt); decode > 0 && outputTokens > 0 {
			s.OutputTokens, s.Decode = outputTokens, decode
		}
	}
	return s, s.HasTTFT || s.Decode > 0
}

// codexTier maps the service tiers used by the speed report.
func codexTier(v string) speed.Tier {
	switch v {
	case "priority", "fast":
		return speed.TierFast
	case "ultrafast":
		return speed.TierUltrafast
	case "default":
		return speed.TierStandard
	}
	return speed.TierUnknown
}
