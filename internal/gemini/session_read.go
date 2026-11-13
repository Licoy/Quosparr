package gemini

import (
	"context"
	"errors"
	"strconv"
	"time"
	"unicode"

	"github.com/egoist/mygo/plugins/sqlite"
)

// Conversation database layout, as written by both Antigravity clients to
// <root>/conversations/<id>.db. Field numbers below are protobuf fields.
//
//	gen_metadata(idx, data): one model call; data.1 is the call metadata:
//	  .4  usage    {2 input, 3 output, 4 cache write, 5 cache read,
//	               9 thinking output, 10 response output} (output = 9 + 10)
//	  .11 time to first token, .12 streaming duration (Duration)
//	  .19 response model id ("gemini-…", "claude-…")
//	  .20 string map {1 key, 2 value}: "last_step_index", "used_claude"
//	steps(idx, step_type, metadata): metadata.1 created at (Timestamp),
//	  metadata.9 the same usage message on the step that holds a response.
//
// The response step of a model call is last_step_index + 1. Usage is taken
// from the steps, which carry one entry per answered call; the model calls
// are only used when no step has usage, and failed calls without token
// counters are never counted.

var sessionErrSchema = errors.New("gemini: not a conversation database")

type sessionUsage struct {
	input, output, cacheRead, cacheWrite, thinking, response int64
}

func (u sessionUsage) total() int64 { return u.input + u.cacheRead + u.cacheWrite + u.output }

// sessionRecord is one counted model call.
type sessionRecord struct {
	at     time.Time // zero when no step time was found
	model  string
	claude bool // the call went to an Anthropic model
	usage  sessionUsage
	// ttft and stream are the call's wait for the first token and streaming
	// time, valid when timed.
	ttft, stream time.Duration
	timed        bool
}

// sessionConversation is the parse of one conversation database.
type sessionConversation struct {
	records   []sessionRecord
	model     string // the latest response model, "" when none
	steps     int64
	malformed int // records that are not valid protobuf
}

type sessionGen struct {
	step       int64 // the response step, -1 when unknown
	model      string
	claude     bool
	usage      sessionUsage
	hasUsage   bool
	ttft       time.Duration
	stream     time.Duration
	hasTimings bool
}

func sessionReadConversation(ctx context.Context, path string) (sessionConversation, sessionFileState, error) {
	var conv sessionConversation
	state, err := sessionWithCopy(ctx, path, func(db *sqlite.DB, _ string) error {
		var err error
		conv, err = sessionParseConversation(ctx, db)
		return err
	})
	return conv, state, err
}

func sessionParseConversation(ctx context.Context, db *sqlite.DB) (sessionConversation, error) {
	var conv sessionConversation
	tables, err := sessionTables(ctx, db)
	if err != nil {
		return conv, err
	}
	if !tables["steps"] && !tables["gen_metadata"] {
		return conv, sessionErrSchema
	}
	var gens []sessionGen
	if tables["gen_metadata"] {
		rows, err := db.Query(ctx, "SELECT data FROM gen_metadata ORDER BY idx")
		if err != nil {
			return conv, err
		}
		for _, row := range rows.Values {
			gen, ok := sessionParseGen(sessionBlob(row[0]))
			if !ok {
				conv.malformed++
				continue
			}
			gens = append(gens, gen)
			if gen.model != "" {
				conv.model = gen.model
			}
		}
	}
	stepAt := map[int64]time.Time{}
	var stepRecords []sessionRecord
	if tables["steps"] {
		rows, err := db.Query(ctx, "SELECT idx, metadata FROM steps ORDER BY idx")
		if err != nil {
			return conv, err
		}
		conv.steps = int64(len(rows.Values))
		byStep := map[int64]sessionGen{}
		for _, gen := range gens {
			if gen.step >= 0 {
				byStep[gen.step] = gen
			}
		}
		for _, row := range rows.Values {
			idx := sessionInt(row[0])
			meta, ok := sessionDecode(sessionBlob(row[1]))
			if !ok {
				conv.malformed++
				continue
			}
			at, _ := meta.timestamp(1)
			stepAt[idx] = at
			usageMsg, ok := meta.message(9)
			if !ok {
				continue
			}
			usage, ok := sessionParseUsage(usageMsg)
			if !ok {
				continue
			}
			gen, matched := byStep[idx]
			rec := sessionRecord{at: at, usage: usage, model: conv.model, claude: sessionLatestClaude(gens)}
			if matched {
				if gen.model != "" {
					rec.model = gen.model
				}
				rec.claude = gen.claude
				rec.ttft, rec.stream, rec.timed = gen.ttft, gen.stream, gen.hasTimings
			}
			stepRecords = append(stepRecords, rec)
		}
	}
	if len(stepRecords) > 0 {
		conv.records = stepRecords
		return conv, nil
	}
	for _, gen := range gens {
		if !gen.hasUsage {
			continue
		}
		model := gen.model
		if model == "" {
			model = conv.model
		}
		conv.records = append(conv.records, sessionRecord{
			at: stepAt[gen.step], model: model, claude: gen.claude, usage: gen.usage,
			ttft: gen.ttft, stream: gen.stream, timed: gen.hasTimings,
		})
	}
	return conv, ctx.Err()
}

func sessionLatestClaude(gens []sessionGen) bool {
	for i := len(gens) - 1; i >= 0; i-- {
		if gens[i].model != "" {
			return gens[i].claude
		}
	}
	return false
}

func sessionParseGen(data []byte) (sessionGen, bool) {
	top, ok := sessionDecode(data)
	if !ok {
		return sessionGen{}, false
	}
	gen := sessionGen{step: -1}
	call, ok := top.message(1)
	if !ok {
		return gen, true
	}
	if usageMsg, ok := call.message(4); ok {
		gen.usage, gen.hasUsage = sessionParseUsage(usageMsg)
	}
	var timedFirst, timedStream bool
	gen.ttft, timedFirst = call.duration(11)
	gen.stream, timedStream = call.duration(12)
	gen.hasTimings = timedFirst || timedStream
	gen.model = sessionModelID(call.text(19))
	for _, entry := range call.messages(20) {
		switch value := entry.text(2); entry.text(1) {
		case "last_step_index":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil && n >= 0 {
				gen.step = n + 1
			}
		case "used_claude":
			gen.claude = value == "true"
		}
	}
	return gen, true
}

func sessionParseUsage(m sessionProto) (sessionUsage, bool) {
	u := sessionUsage{
		input: m.count(2), output: m.count(3), cacheWrite: m.count(4), cacheRead: m.count(5),
		thinking: m.count(9), response: m.count(10),
	}
	if u.output == 0 {
		u.output = u.thinking + u.response
	}
	return u, u.input > 0 || u.output > 0 || u.cacheRead > 0 || u.cacheWrite > 0
}

// sessionModelID keeps a model id only when it looks like one: short
// printable ASCII without spaces.
func sessionModelID(s string) string {
	if s == "" || len(s) > 128 {
		return ""
	}
	for _, r := range s {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return ""
		}
	}
	return s
}
