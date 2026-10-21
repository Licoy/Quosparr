package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
)

// ErrCorrupt marks stored JSON that the Swift app could not decode either. The
// Swift app answers with silent defaults; this package reports an error and
// leaves the stored bytes alone.
var ErrCorrupt = errors.New("stored settings are corrupt")

func corruptf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}

// fields is a decoded top-level JSON object. Keys are matched exactly (case
// sensitive), as Swift's Codable does, which json.Unmarshal into a struct would
// not.
type fields map[string]json.RawMessage

// parseObject decodes data, which must be one JSON object.
func parseObject(data []byte) (fields, error) {
	var m fields
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, corruptf("not valid JSON: %v", err)
	}
	if m == nil {
		return nil, corruptf("not a JSON object")
	}
	return m, nil
}

// reader consumes the keys of an object one by one, with Swift's
// decodeIfPresent rules: a missing key and an explicit null give the default,
// anything else of the wrong type is an error. The first error sticks and every
// later read returns its default. What is left in rest are the unknown keys.
type reader struct {
	rest fields
	err  error
}

func (r *reader) fail(key, format string, args ...any) {
	if r.err == nil {
		r.err = corruptf("%q: %s", key, fmt.Sprintf(format, args...))
	}
}

// take removes key and returns its raw value, or false when it is absent or
// null (or an earlier read failed).
func (r *reader) take(key string) (json.RawMessage, bool) {
	raw, ok := r.rest[key]
	delete(r.rest, key)
	raw = bytes.TrimSpace(raw)
	if !ok || r.err != nil || string(raw) == "null" {
		return nil, false
	}
	return raw, true
}

func (r *reader) bool(key string, def bool) bool {
	raw, ok := r.take(key)
	if !ok {
		return def
	}
	switch string(raw) {
	case "true":
		return true
	case "false":
		return false
	}
	r.fail(key, "expected a boolean")
	return def
}

func (r *reader) int(key string, def int) int {
	raw, ok := r.take(key)
	if !ok {
		return def
	}
	v, err := parseInt(raw)
	if err != nil {
		r.fail(key, "%v", err)
		return def
	}
	return v
}

func (r *reader) float(key string, def float64) float64 {
	raw, ok := r.take(key)
	if !ok {
		return def
	}
	if !looksNumeric(raw) {
		r.fail(key, "expected a number")
		return def
	}
	v, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		r.fail(key, "number is not representable")
		return def
	}
	return v
}

func (r *reader) string(key string) (string, bool) {
	raw, ok := r.take(key)
	if !ok {
		return "", false
	}
	var s string
	if raw[0] != '"' || json.Unmarshal(raw, &s) != nil {
		r.fail(key, "expected a string")
		return "", false
	}
	return s, true
}

// readEnum decodes a string enum strictly. An unknown value is an error, so
// an unknown proxy mode never silently becomes another network route.
func readEnum[T ~string](r *reader, key string, def T, valid func(T) bool) T {
	s, ok := r.string(key)
	if !ok {
		return def
	}
	if v := T(s); valid(v) {
		return v
	}
	r.fail(key, "unknown value %q", truncate(s, 40))
	return def
}

// readEnumOrDefault decodes a settings enum leniently: a name this version
// does not know, such as one a newer or unmerged build wrote, is the field's
// default. A value that is not a string is still an error.
func readEnumOrDefault[T ~string](r *reader, key string, def T, valid func(T) bool) T {
	if s, ok := r.string(key); ok && valid(T(s)) {
		return T(s)
	}
	return def
}

// readWindowMode decodes WindowMode with the Swift tolerance: only
// "rollingWeek" is kept; anything else, of any JSON type, means auto.
func readWindowMode(r *reader, key string) WindowMode {
	raw, ok := r.take(key)
	var s string
	if ok && json.Unmarshal(raw, &s) == nil && WindowMode(s) == WindowModeRollingWeek {
		return WindowModeRollingWeek
	}
	return WindowModeAuto
}

// readThemeColor decodes the Go-only theme color. The Swift app never
// rejects a key it does not know, so a value of any other form is the brand
// color, not an error.
func readThemeColor(r *reader, key string) string {
	raw, ok := r.take(key)
	var s string
	if !ok || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return NormalizeThemeColor(s)
}

// readFontFamily decodes the Go-only font family leniently, as
// readThemeColor: anything but a usable name is the system's font.
func readFontFamily(r *reader, key string) string {
	raw, ok := r.take(key)
	var s string
	if !ok || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return NormalizeFontFamily(s)
}

// readProxyTestURL decodes the Go-only test site leniently: anything but a
// valid URL is the default.
func readProxyTestURL(r *reader, key string) string {
	raw, ok := r.take(key)
	var s string
	if !ok || json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return NormalizeProxyTestURL(s)
}

// readPanelProviders decodes the Go-only panel providers leniently: an
// unreadable value shows every provider, and unknown names are dropped.
func readPanelProviders(r *reader, key string) []Provider {
	raw, ok := r.take(key)
	var ids []string
	if !ok || json.Unmarshal(raw, &ids) != nil {
		return nil
	}
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		out = append(out, Provider(id))
	}
	return canonicalProviders(out, len(Providers()))
}

// readResetTimeFormat decodes the Go-only reset time format leniently, as
// readThemeColor: anything but a known value is the relative countdown.
func readResetTimeFormat(r *reader, key string) ResetTimeFormat {
	raw, ok := r.take(key)
	var s string
	if !ok || json.Unmarshal(raw, &s) != nil || !ResetTimeFormat(s).Valid() {
		return ResetTimeRelative
	}
	return ResetTimeFormat(s)
}

// readModules decodes the module order. Unknown module names are dropped (a
// newer app may know more modules); a non-array, or a non-string element, is
// an error.
func readModules(r *reader, key string, def []Module) []Module {
	raw, ok := r.take(key)
	if !ok {
		return def
	}
	var ids []*string
	if raw[0] != '[' || json.Unmarshal(raw, &ids) != nil {
		r.fail(key, "expected an array of strings")
		return def
	}
	out := make([]Module, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			r.fail(key, "expected an array of strings")
			return def
		}
		if m := Module(*id); m.Valid() {
			out = append(out, m)
		}
	}
	return out
}

// readProviders decodes a provider list. Unknown names are dropped. A
// non-array, or a non-string element, is an error. A missing key is nil.
func readProviders(r *reader, key string) []Provider {
	raw, ok := r.take(key)
	if !ok {
		return nil
	}
	var ids []*string
	if raw[0] != '[' || json.Unmarshal(raw, &ids) != nil {
		r.fail(key, "expected an array of strings")
		return nil
	}
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			r.fail(key, "expected an array of strings")
			return nil
		}
		if p := Provider(*id); p.Valid() {
			out = append(out, p)
		}
	}
	return out
}

// looksNumeric rejects JSON strings, booleans, arrays and objects before a
// number parser sees them.
func looksNumeric(raw []byte) bool {
	return len(raw) > 0 && (raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9'))
}

// parseInt reads a JSON number the way Swift decodes Int: plain integers and
// integral floating-point forms such as 300.0 or 3e2 are accepted; fractions
// and out-of-range values are not.
func parseInt(raw []byte) (int, error) {
	if !looksNumeric(raw) {
		return 0, errors.New("expected an integer")
	}
	if v, err := strconv.ParseInt(string(raw), 10, 64); err == nil {
		return fitInt(v)
	}
	f, err := strconv.ParseFloat(string(raw), 64)
	if err != nil || f != math.Trunc(f) || f < -(1<<63) || f >= 1<<63 {
		return 0, errors.New("number is not representable as an integer")
	}
	return fitInt(int64(f))
}

func fitInt(v int64) (int, error) {
	if v < math.MinInt || v > math.MaxInt {
		return 0, errors.New("number is not representable as an integer")
	}
	return int(v), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// encodeFields marshals v (a struct with json tags) and adds the unknown keys
// kept from decoding. Known keys win over extras. The output is compact with
// sorted keys. Each edit may change the known fields before the extras join.
func encodeFields(v any, extra map[string]json.RawMessage, edits ...func(fields)) ([]byte, error) {
	known, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m fields
	if err := json.Unmarshal(known, &m); err != nil {
		return nil, err
	}
	for _, edit := range edits {
		edit(m)
	}
	for k, raw := range extra {
		if _, ok := m[k]; !ok {
			m[k] = raw
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
