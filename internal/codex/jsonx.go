package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

// object is a JSON object whose values are kept as raw bytes, so fields this
// package does not model survive a read-modify-write cycle untouched.
type object map[string]json.RawMessage

// parseObject decodes data, which must be a single JSON object.
func parseObject(data []byte) (object, error) {
	var o object
	if err := json.Unmarshal(data, &o); err != nil {
		return nil, errors.New("not a JSON object")
	}
	if o == nil { // the literal null
		return nil, errors.New("not a JSON object")
	}
	return o, nil
}

func (o object) clone() object {
	c := make(object, len(o))
	for k, v := range o {
		c[k] = v
	}
	return c
}

// encodeValue marshals v the way every file of this package is written: no
// HTML escaping, no trailing newline.
func encodeValue(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func mustValue(v any) json.RawMessage {
	raw, err := encodeValue(v)
	if err != nil {
		panic("codex: encoding a plain value failed: " + err.Error())
	}
	return raw
}

// --- reading -----------------------------------------------------------

// reader pulls typed fields out of a JSON object with the strictness of a
// Swift synthesized Decodable: a wrong type is an error, null counts as
// absent, and a missing required key is an error. The first error sticks.
type reader struct {
	obj  object
	used map[string]struct{}
	err  error
}

func newReader(o object) *reader { return &reader{obj: o, used: map[string]struct{}{}} }

func (r *reader) fail(key, problem string) {
	if r.err == nil {
		r.err = fmt.Errorf("field %q: %s", key, problem)
	}
}

// raw returns the value of key, reporting false for a missing key or null.
func (r *reader) raw(key string) (json.RawMessage, bool) {
	r.used[key] = struct{}{}
	v, ok := r.obj[key]
	if !ok || string(bytes.TrimSpace(v)) == "null" {
		return nil, false
	}
	return v, true
}

func (r *reader) optString(key string) string {
	v, ok := r.raw(key)
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		r.fail(key, "expected a string")
	}
	return s
}

// optStringPtr is optString that tells absent from empty.
func (r *reader) optStringPtr(key string) *string {
	if _, ok := r.raw(key); !ok {
		return nil
	}
	s := r.optString(key)
	return &s
}

func (r *reader) reqString(key string) string {
	if _, ok := r.raw(key); !ok {
		r.fail(key, "missing")
		return ""
	}
	return r.optString(key)
}

func (r *reader) optBool(key string) (value, present bool) {
	v, ok := r.raw(key)
	if !ok {
		return false, false
	}
	if err := json.Unmarshal(v, &value); err != nil {
		r.fail(key, "expected a boolean")
	}
	return value, true
}

func (r *reader) reqBool(key string) bool {
	b, ok := r.optBool(key)
	if !ok {
		r.fail(key, "missing")
	}
	return b
}

func (r *reader) optInt(key string) *int {
	v, ok := r.raw(key)
	if !ok {
		return nil
	}
	n, err := parseIntLiteral(string(v))
	if err != nil {
		r.fail(key, "expected an integer")
		return nil
	}
	return &n
}

func (r *reader) reqInt(key string) int {
	p := r.optInt(key)
	if p == nil {
		r.fail(key, "missing")
		return 0
	}
	return *p
}

func (r *reader) optFloat(key string) *float64 {
	v, ok := r.raw(key)
	if !ok {
		return nil
	}
	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		r.fail(key, "expected a number")
		return nil
	}
	return &f
}

func (r *reader) optTime(key string) *time.Time {
	if _, ok := r.raw(key); !ok {
		return nil
	}
	s := r.optString(key)
	if r.err != nil {
		return nil
	}
	t, ok := ParseTime(s)
	if !ok {
		r.fail(key, "unrecognized date")
		return nil
	}
	return &t
}

func (r *reader) reqTime(key string) time.Time {
	p := r.optTime(key)
	if p == nil {
		r.fail(key, "missing")
		return time.Time{}
	}
	return *p
}

// rest returns the keys no accessor asked for: the fields to carry through.
func (r *reader) rest() object {
	extra := object{}
	for k, v := range r.obj {
		if _, ok := r.used[k]; !ok {
			extra[k] = v
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

// parseIntLiteral accepts what Swift's JSONDecoder accepts for an Int: an
// integer literal, or a float literal with an exact integer value.
func parseIntLiteral(s string) (int, error) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(n), nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, errors.New("not an integer")
	}
	return int(f), nil
}

// --- writing -----------------------------------------------------------

// builder assembles an object, skipping empty optionals the way a Swift
// synthesized encoder skips nil ones.
type builder struct{ o object }

func newBuilder(extra object) *builder { return &builder{o: extra.clone()} }

func (b *builder) set(key string, v any) { b.o[key] = mustValue(v) }
func (b *builder) str(key, s string) {
	if s != "" {
		b.set(key, s)
	}
}
func (b *builder) intPtr(key string, n *int) {
	if n != nil {
		b.set(key, *n)
	}
}
func (b *builder) floatPtr(key string, f *float64) {
	if f != nil {
		b.set(key, *f)
	}
}
func (b *builder) timePtr(key string, t *time.Time) {
	if t != nil {
		b.set(key, FormatTime(*t))
	}
}

// --- pretty printing ---------------------------------------------------

// formatOptions selects the text layout of formatJSON.
type formatOptions struct {
	pretty      bool // two-space indent and `"key" : value`, like Foundation
	escapeSlash bool // write "/" as "\/", Foundation's default
}

// formatJSON re-lays out raw JSON. Object keys are sorted the way Foundation's
// sortedKeys orders them (case-insensitively), numbers keep their literal
// text, and the result has no trailing newline.
func formatJSON(raw []byte, opt formatOptions) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return swiftjson.MarshalWithOptions(v, swiftjson.Options{
		Pretty: opt.pretty, WithoutEscapingSlashes: !opt.escapeSlash,
		CompactEmptyContainers: true, EscapeLineSeparators: true, KeyLess: keyLess,
	})
}

func keyLess(a, b string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return a < b
}
