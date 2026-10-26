package swiftjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	timeType      = reflect.TypeOf(time.Time{})
	numberType    = reflect.TypeOf(json.Number(""))
	marshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

// Marshal encodes v the way Swift's JSONEncoder does with
// dateEncodingStrategy = .iso8601 and outputFormatting = [.sortedKeys], plus
// .prettyPrinted when pretty is set.
//
// Rules, all taken from the Foundation encoder:
//   - object keys are sorted by their UTF-8 bytes;
//   - a nil pointer or nil interface struct field is left out (Swift's
//     synthesized Codable uses encodeIfPresent for optionals); a nil slice
//     or map encodes as [] or {};
//   - time.Time is an ISO 8601 UTC string with whole seconds ("...T08:00:00Z");
//   - float64 prints like Swift's Double description ("12", "0.04", "1e+16");
//   - "/" is escaped as "\/", control characters as \b \f \t \r \n or \u00xx,
//     and everything else (including <, >, & and U+2028) is written raw;
//   - a type with MarshalJSON must return a JSON scalar, which is copied as is
//     (Decimal does this);
//   - pretty printing uses two spaces, `"key" : value`, and writes an empty
//     array or object as the opening bracket, a blank line, then the closing
//     bracket.
//
// Struct fields use the name in their `json` tag; fields without a tag use the
// Go name. Unexported fields and `json:"-"` are skipped. omitempty is ignored.
// The extra option `omitnil` drops a nil slice or map the way Swift drops a nil
// optional array (a nil slice without it encodes as []); `omitzero` opts a
// field into omitting its zero value without changing legacy Swift fields.
func Marshal(v any, pretty bool) ([]byte, error) {
	return MarshalWithOptions(v, Options{Pretty: pretty})
}

// Options preserves the Foundation writer settings of each on-disk format.
// KeyLess defaults to UTF-8 byte order; NormalizeNumbers uses
// JSONSerialization's number description rather than keeping json.Number text.
type Options struct {
	Pretty                 bool
	WithoutEscapingSlashes bool
	CompactEmptyContainers bool
	EscapeLineSeparators   bool
	NormalizeNumbers       bool
	KeyLess                func(a, b string) bool
}

// MarshalWithOptions writes JSONEncoder or JSONSerialization-compatible JSON.
func MarshalWithOptions(v any, options Options) ([]byte, error) {
	e := encoder{options: options}
	if err := e.value(reflect.ValueOf(v), 0); err != nil {
		return nil, err
	}
	return e.buf.Bytes(), nil
}

type encoder struct {
	buf     bytes.Buffer
	options Options
}

func (e *encoder) indent(depth int) {
	e.buf.WriteByte('\n')
	for i := 0; i < depth; i++ {
		e.buf.WriteString("  ")
	}
}

func (e *encoder) value(v reflect.Value, depth int) error {
	if !v.IsValid() {
		e.buf.WriteString("null")
		return nil
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		// Dereference first: *time.Time would otherwise reach its own
		// MarshalJSON, which keeps the fractional seconds Swift drops.
		if v.IsNil() {
			e.buf.WriteString("null")
			return nil
		}
		return e.value(v.Elem(), depth)
	}
	if v.Type() == timeType {
		e.buf.WriteByte('"')
		e.buf.WriteString(FormatDate(v.Interface().(time.Time)))
		e.buf.WriteByte('"')
		return nil
	}
	if v.Type() == numberType {
		text := v.String()
		if !json.Valid([]byte(text)) || text == "" || !strings.ContainsRune("-0123456789", rune(text[0])) {
			return fmt.Errorf("swiftjson: invalid JSON number")
		}
		if e.options.NormalizeNumbers {
			text = normalizeNumber(text)
		}
		e.buf.WriteString(text)
		return nil
	}
	if v.Type().Implements(marshalerType) {
		raw, err := v.Interface().(json.Marshaler).MarshalJSON()
		if err != nil {
			return err
		}
		e.buf.Write(raw)
		return nil
	}
	switch v.Kind() {
	case reflect.Bool:
		e.buf.WriteString(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.buf.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		e.buf.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float64, reflect.Float32:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("swiftjson: cannot encode %v", f)
		}
		e.buf.WriteString(FormatDouble(f))
	case reflect.String:
		e.writeString(v.String())
	case reflect.Slice, reflect.Array:
		return e.array(v, depth)
	case reflect.Map:
		return e.mapValue(v, depth)
	case reflect.Struct:
		return e.object(v, depth)
	default:
		return fmt.Errorf("swiftjson: unsupported type %s", v.Type())
	}
	return nil
}

func (e *encoder) array(v reflect.Value, depth int) error {
	e.buf.WriteByte('[')
	n := v.Len()
	if n == 0 {
		if e.options.Pretty && !e.options.CompactEmptyContainers {
			e.buf.WriteByte('\n')
			e.indent(depth)
		}
		e.buf.WriteByte(']')
		return nil
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			e.buf.WriteByte(',')
		}
		if e.options.Pretty {
			e.indent(depth + 1)
		}
		if err := e.value(v.Index(i), depth+1); err != nil {
			return err
		}
	}
	if e.options.Pretty {
		e.indent(depth)
	}
	e.buf.WriteByte(']')
	return nil
}

type member struct {
	key string
	val reflect.Value
}

func (e *encoder) mapValue(v reflect.Value, depth int) error {
	if v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("swiftjson: map key must be a string, got %s", v.Type().Key())
	}
	members := make([]member, 0, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		members = append(members, member{iter.Key().String(), iter.Value()})
	}
	return e.members(members, depth)
}

func (e *encoder) object(v reflect.Value, depth int) error {
	t := v.Type()
	members := make([]member, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		omitNil, omitZero := false, false
		if tag, ok := f.Tag.Lookup("json"); ok {
			head, opts, _ := strings.Cut(tag, ",")
			omitNil = strings.Contains(","+opts+",", ",omitnil,")
			omitZero = strings.Contains(","+opts+",", ",omitzero,")
			if head == "-" {
				continue
			}
			if head != "" {
				name = head
			}
		}
		fv := v.Field(i)
		if omitZero && fv.IsZero() {
			continue
		}
		if (fv.Kind() == reflect.Pointer || fv.Kind() == reflect.Interface) && fv.IsNil() {
			continue
		}
		if omitNil && (fv.Kind() == reflect.Slice || fv.Kind() == reflect.Map) && fv.IsNil() {
			continue
		}
		members = append(members, member{name, fv})
	}
	return e.members(members, depth)
}

func (e *encoder) members(members []member, depth int) error {
	sort.Slice(members, func(i, j int) bool {
		if e.options.KeyLess != nil {
			return e.options.KeyLess(members[i].key, members[j].key)
		}
		return members[i].key < members[j].key
	})
	e.buf.WriteByte('{')
	if len(members) == 0 {
		if e.options.Pretty && !e.options.CompactEmptyContainers {
			e.buf.WriteByte('\n')
			e.indent(depth)
		}
		e.buf.WriteByte('}')
		return nil
	}
	for i, m := range members {
		if i > 0 {
			e.buf.WriteByte(',')
		}
		if e.options.Pretty {
			e.indent(depth + 1)
		}
		e.writeString(m.key)
		if e.options.Pretty {
			e.buf.WriteString(" : ")
		} else {
			e.buf.WriteByte(':')
		}
		if err := e.value(m.val, depth+1); err != nil {
			return err
		}
	}
	if e.options.Pretty {
		e.indent(depth)
	}
	e.buf.WriteByte('}')
	return nil
}
