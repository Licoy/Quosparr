package usage

import (
	"bytes"
	"errors"
	"math"
	"strconv"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// logRecord is one decoded session-log line.
type logRecord struct {
	Timestamp float64 // Unix seconds; distantPast when unparseable
	DayKey    string
	Model     *string
	// ContextModel is the model that applies to the lines after this one.
	ContextModel *string
	SessionCWD   *string
	SessionID    *string
	Usage        *TokenUsage
	CacheKnown   bool
	TotalUsage   *usageFields
	// CountsAsTurn is true for a Codex task_complete event, the user-visible
	// turn.
	CountsAsTurn bool
}

var errMalformed = errors.New("usage: malformed log line")
var errInvalidCacheCounters = errors.New("usage: invalid cache-hit token counters")

// maxJSONDepth is the deepest nesting the system decoder accepts, top-level
// object included.
const maxJSONDepth = 512

// recordParser decodes log lines. It is not safe for concurrent use.
type recordParser struct {
	loc         *time.Location
	cacheFields bool
}

func newRecordParser(loc *time.Location) *recordParser {
	if loc == nil {
		loc = time.UTC
	}
	return &recordParser{loc: loc}
}

// parse decodes one line. It follows the leniencies and strictness of the
// Swift JSONDecoder it replaces:
//
//   - strings are validated only where they are decoded (keys of the objects
//     we descend into, and the fields we read); skipped values are scanned
//     structurally;
//   - numbers in skipped values are scanned loosely, but integer fields use the
//     strict JSON grammar and must be integral;
//   - trailing commas and a single leading BOM are accepted; comments, NaN,
//     single quotes and trailing garbage are not;
//   - for duplicate keys the first one wins;
//   - a value of the wrong type for a field we read fails the whole line.
func (p *recordParser) parse(line []byte) (logRecord, error) {
	d := &jsonDec{b: line}
	if bytes.HasPrefix(d.b, []byte{0xEF, 0xBB, 0xBF}) {
		d.i = 3
	}
	var (
		timestamp   *string
		typ         *string
		payload     = payloadFields{cacheFields: p.cacheFields}
		turnContext turnContextFields
	)
	err := d.object(func(key string, first bool) error {
		if !first {
			return d.skip()
		}
		switch key {
		case "timestamp":
			return d.optString(&timestamp)
		case "type":
			return d.optString(&typ)
		case "payload":
			return d.optObject(func(k string, first bool) error { return payload.field(d, k, first) })
		case "turn_context":
			return d.optObject(func(k string, first bool) error { return turnContext.field(d, k, first) })
		}
		return d.skip()
	})
	if err != nil {
		return logRecord{}, err
	}
	d.skipSpace()
	if d.i != len(d.b) {
		return logRecord{}, errMalformed
	}

	var usage *TokenUsage
	if payload.usage != nil {
		u, err := payload.usage.decoded()
		if err != nil {
			if p.cacheFields {
				return logRecord{}, errInvalidCacheCounters
			}
			return logRecord{}, err
		}
		usage = &u
	}
	rec := logRecord{Timestamp: distantPast, Usage: usage, TotalUsage: payload.totalUsage}
	if payload.usage != nil {
		rec.CacheKnown = payload.usage.input != nil && payload.usage.cached != nil
	}
	if timestamp != nil {
		if ts, ok := parseTimestamp(*timestamp); ok {
			rec.Timestamp = ts
		}
	}
	rec.DayKey = dayKey(rec.Timestamp, p.loc)
	rec.Model = firstString(turnContext.model, payload.model)
	if typ != nil && *typ == "turn_context" {
		rec.ContextModel = payload.model
	} else {
		rec.ContextModel = turnContext.model
	}
	rec.SessionCWD = firstString(payload.cwd, turnContext.cwd)
	if typ != nil && *typ == "session_meta" {
		rec.SessionID = firstString(payload.id, payload.sessionID)
	}
	rec.CountsAsTurn = payload.typ != nil && *payload.typ == "task_complete"
	return rec, nil
}

func firstString(a, b *string) *string {
	if a != nil {
		return a
	}
	return b
}

type turnContextFields struct{ model, cwd *string }

func (t *turnContextFields) field(d *jsonDec, key string, first bool) error {
	if !first {
		return d.skip()
	}
	switch key {
	case "model":
		return d.optString(&t.model)
	case "cwd":
		return d.optString(&t.cwd)
	}
	return d.skip()
}

type payloadFields struct {
	typ, model, cwd, id, sessionID *string
	usage                          *usageFields
	totalUsage                     *usageFields
	cacheFields                    bool
}

func (p *payloadFields) field(d *jsonDec, key string, first bool) error {
	if !first {
		return d.skip()
	}
	switch key {
	case "type":
		return d.optString(&p.typ)
	case "model":
		return d.optString(&p.model)
	case "cwd":
		return d.optString(&p.cwd)
	case "id":
		if !p.cacheFields {
			return d.skip()
		}
		return d.optString(&p.id)
	case "session_id":
		if !p.cacheFields {
			return d.skip()
		}
		return d.optString(&p.sessionID)
	case "info":
		return d.optObject(func(k string, first bool) error {
			if !first {
				return d.skip()
			}
			var target **usageFields
			switch k {
			case "last_token_usage":
				target = &p.usage
			case "total_token_usage":
				if !p.cacheFields {
					return d.skip()
				}
				target = &p.totalUsage
			default:
				return d.skip()
			}
			return d.optObject(func(k string, first bool) error {
				if *target == nil {
					*target = &usageFields{cacheFields: p.cacheFields}
				}
				return (*target).field(d, k, first)
			})
		})
	}
	return d.skip()
}

type usageFields struct {
	input, cached, output, reasoning *int64
	cacheFields                      bool
}

func (u *usageFields) field(d *jsonDec, key string, first bool) error {
	if !first {
		return d.skip()
	}
	var err error
	switch key {
	case "input_tokens":
		err = d.optInt(&u.input)
	case "cached_input_tokens":
		err = d.optInt(&u.cached)
	case "output_tokens":
		err = d.optInt(&u.output)
	case "reasoning_output_tokens":
		err = d.optInt(&u.reasoning)
	default:
		return d.skip()
	}
	if err != nil && u.cacheFields {
		return errInvalidCacheCounters
	}
	return err
}

func orZero(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func (u *usageFields) decoded() (TokenUsage, error) {
	in, cached, out, reasoning := orZero(u.input), orZero(u.cached), orZero(u.output), orZero(u.reasoning)
	if in < 0 || cached < 0 || out < 0 || reasoning < 0 || cached > in {
		return TokenUsage{}, errMalformed
	}
	if _, err := checkedAdd(in, out, "tokens"); err != nil {
		return TokenUsage{}, errMalformed
	}
	return TokenUsage{InputTokens: in, CachedInputTokens: cached, OutputTokens: out}, nil
}

// ---- minimal JSON reader -------------------------------------------------

type jsonDec struct {
	b     []byte
	i     int
	depth int
}

func (d *jsonDec) skipSpace() {
	for d.i < len(d.b) {
		switch d.b[d.i] {
		case ' ', '\t', '\n', '\r':
			d.i++
		default:
			return
		}
	}
}

func (d *jsonDec) peek() int {
	if d.i < len(d.b) {
		return int(d.b[d.i])
	}
	return -1
}

func (d *jsonDec) enter() error {
	d.depth++
	if d.depth > maxJSONDepth {
		return errMalformed
	}
	return nil
}

// object parses an object whose members are handled by fn. fn must consume the
// member's value. The opening brace is at the current position.
func (d *jsonDec) object(fn func(key string, first bool) error) error {
	d.skipSpace()
	if d.peek() != '{' {
		return errMalformed
	}
	return d.members(fn)
}

func (d *jsonDec) members(fn func(key string, first bool) error) error {
	if err := d.enter(); err != nil {
		return err
	}
	defer func() { d.depth-- }()
	d.i++ // {
	seen := map[string]struct{}{}
	d.skipSpace()
	if d.peek() == '}' {
		d.i++
		return nil
	}
	for {
		d.skipSpace()
		if d.peek() != '"' {
			return errMalformed
		}
		key, err := d.decodeString()
		if err != nil {
			return err
		}
		d.skipSpace()
		if d.peek() != ':' {
			return errMalformed
		}
		d.i++
		d.skipSpace()
		_, dup := seen[key]
		seen[key] = struct{}{}
		if err := fn(key, !dup); err != nil {
			return err
		}
		d.skipSpace()
		switch d.peek() {
		case ',':
			d.i++
			d.skipSpace()
			if d.peek() == '}' {
				d.i++
				return nil
			}
		case '}':
			d.i++
			return nil
		default:
			return errMalformed
		}
	}
}

func (d *jsonDec) literal(word string) bool {
	n := len(word)
	if d.i+n > len(d.b) || string(d.b[d.i:d.i+n]) != word {
		return false
	}
	if d.i+n < len(d.b) && !isDelimiter(d.b[d.i+n]) {
		return false
	}
	d.i += n
	return true
}

func isDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', ',', '}', ']':
		return true
	}
	return false
}

// optObject handles a value that must be an object or null.
func (d *jsonDec) optObject(fn func(key string, first bool) error) error {
	switch d.peek() {
	case 'n':
		if d.literal("null") {
			return nil
		}
	case '{':
		return d.members(fn)
	}
	return errMalformed
}

// optString handles a value that must be a string or null.
func (d *jsonDec) optString(dst **string) error {
	switch d.peek() {
	case 'n':
		if d.literal("null") {
			return nil
		}
	case '"':
		s, err := d.decodeString()
		if err != nil {
			return err
		}
		*dst = &s
		return nil
	}
	return errMalformed
}

// optInt handles a value that must be an integral number or null.
func (d *jsonDec) optInt(dst **int64) error {
	c := d.peek()
	if c == 'n' {
		if d.literal("null") {
			return nil
		}
		return errMalformed
	}
	if c != '-' && (c < '0' || c > '9') {
		return errMalformed
	}
	tok := d.numberToken()
	v, ok := parseIntegralJSON(tok)
	if !ok {
		return errMalformed
	}
	*dst = &v
	return nil
}

// numberToken consumes the loose number run the scanner accepts: it starts with
// '-' or a digit and continues over digits, '.', 'e', 'E', '+' and '-'.
func (d *jsonDec) numberToken() []byte {
	start := d.i
	d.i++
	for d.i < len(d.b) {
		c := d.b[d.i]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			d.i++
			continue
		}
		break
	}
	return d.b[start:d.i]
}

// parseIntegralJSON validates tok against the JSON number grammar and returns
// its value if it is an integer representable as int64.
func parseIntegralJSON(tok []byte) (int64, bool) {
	i := 0
	if i < len(tok) && tok[i] == '-' {
		i++
	}
	if i >= len(tok) {
		return 0, false
	}
	if tok[i] == '0' {
		i++
	} else if tok[i] >= '1' && tok[i] <= '9' {
		for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
			i++
		}
	} else {
		return 0, false
	}
	plain := i == len(tok)
	if i < len(tok) && tok[i] == '.' {
		i++
		n := 0
		for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
			i++
			n++
		}
		if n == 0 {
			return 0, false
		}
	}
	if i < len(tok) && (tok[i] == 'e' || tok[i] == 'E') {
		i++
		if i < len(tok) && (tok[i] == '+' || tok[i] == '-') {
			i++
		}
		n := 0
		for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
			i++
			n++
		}
		if n == 0 {
			return 0, false
		}
	}
	if i != len(tok) {
		return 0, false
	}
	if plain {
		v, err := strconv.ParseInt(string(tok), 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	f, err := strconv.ParseFloat(string(tok), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || f != math.Trunc(f) {
		return 0, false
	}
	if f < -9223372036854775808.0 || f >= 9223372036854775808.0 {
		return 0, false
	}
	return int64(f), true
}

// skip consumes any value without validating string contents.
func (d *jsonDec) skip() error {
	switch c := d.peek(); {
	case c == '"':
		return d.skipString()
	case c == '{':
		return d.skipContainer('{', '}')
	case c == '[':
		return d.skipContainer('[', ']')
	case c == '-' || (c >= '0' && c <= '9'):
		d.numberToken()
		return d.afterValue()
	case c == 't':
		if d.literal("true") {
			return nil
		}
	case c == 'f':
		if d.literal("false") {
			return nil
		}
	case c == 'n':
		if d.literal("null") {
			return nil
		}
	}
	return errMalformed
}

// afterValue rejects a value glued to the following character.
func (d *jsonDec) afterValue() error {
	if d.i < len(d.b) && !isDelimiter(d.b[d.i]) {
		return errMalformed
	}
	return nil
}

func (d *jsonDec) skipString() error {
	d.i++ // opening quote
	for d.i < len(d.b) {
		switch d.b[d.i] {
		case '"':
			d.i++
			return nil
		case '\\':
			d.i++
			if d.i >= len(d.b) {
				return errMalformed
			}
			if d.b[d.i] == 'u' {
				d.i += 5
			} else {
				d.i++
			}
		default:
			d.i++
		}
	}
	return errMalformed
}

func (d *jsonDec) skipContainer(open, close byte) error {
	if err := d.enter(); err != nil {
		return err
	}
	defer func() { d.depth-- }()
	d.i++
	d.skipSpace()
	if d.peek() == int(close) {
		d.i++
		return nil
	}
	for {
		d.skipSpace()
		if open == '{' {
			if d.peek() != '"' {
				return errMalformed
			}
			if err := d.skipString(); err != nil {
				return err
			}
			d.skipSpace()
			if d.peek() != ':' {
				return errMalformed
			}
			d.i++
			d.skipSpace()
		}
		if err := d.skip(); err != nil {
			return err
		}
		d.skipSpace()
		switch d.peek() {
		case ',':
			d.i++
			d.skipSpace()
			if d.peek() == int(close) {
				d.i++
				return nil
			}
		case int(close):
			d.i++
			return nil
		default:
			return errMalformed
		}
	}
}

// decodeString decodes and validates a string at the current quote: raw
// control characters, invalid UTF-8, unknown escapes and unpaired surrogates
// are errors.
func (d *jsonDec) decodeString() (string, error) {
	d.i++ // opening quote
	start := d.i
	// Fast path: no escapes.
	for d.i < len(d.b) {
		c := d.b[d.i]
		if c == '"' {
			s := d.b[start:d.i]
			if !utf8.Valid(s) {
				return "", errMalformed
			}
			d.i++
			return string(s), nil
		}
		if c == '\\' {
			break
		}
		if c < 0x20 {
			return "", errMalformed
		}
		d.i++
	}
	if d.i >= len(d.b) {
		return "", errMalformed
	}
	out := append([]byte{}, d.b[start:d.i]...)
	if !utf8.Valid(out) {
		return "", errMalformed
	}
	for d.i < len(d.b) {
		c := d.b[d.i]
		switch {
		case c == '"':
			d.i++
			if !utf8.Valid(out) {
				return "", errMalformed
			}
			return string(out), nil
		case c < 0x20:
			return "", errMalformed
		case c != '\\':
			out = append(out, c)
			d.i++
		default:
			d.i++
			if d.i >= len(d.b) {
				return "", errMalformed
			}
			e := d.b[d.i]
			d.i++
			switch e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				r, err := d.unicodeEscape()
				if err != nil {
					return "", err
				}
				out = utf8.AppendRune(out, r)
			default:
				return "", errMalformed
			}
		}
	}
	return "", errMalformed
}

func (d *jsonDec) hex4() (rune, bool) {
	if d.i+4 > len(d.b) {
		return 0, false
	}
	var v rune
	for _, c := range d.b[d.i : d.i+4] {
		switch {
		case c >= '0' && c <= '9':
			v = v<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			v = v<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			v = v<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	d.i += 4
	return v, true
}

func (d *jsonDec) unicodeEscape() (rune, error) {
	r, ok := d.hex4()
	if !ok {
		return 0, errMalformed
	}
	if utf16.IsSurrogate(r) {
		if r >= 0xDC00 || d.i+2 > len(d.b) || d.b[d.i] != '\\' || d.b[d.i+1] != 'u' {
			return 0, errMalformed
		}
		d.i += 2
		lo, ok := d.hex4()
		if !ok || lo < 0xDC00 || lo > 0xDFFF {
			return 0, errMalformed
		}
		return utf16.DecodeRune(r, lo), nil
	}
	return r, nil
}
