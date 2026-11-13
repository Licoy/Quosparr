package gemini

import (
	"encoding/binary"
	"time"
)

// sessionProto is one decoded protobuf message: its fields in wire order.
// Only the wire types the conversation records use are accepted; a group or
// a truncated field makes the whole message invalid.
type sessionProto []sessionProtoField

type sessionProtoField struct {
	num   uint64
	value uint64 // varint, fixed32 or fixed64 payload
	data  []byte // length-delimited payload
	bytes bool
}

func sessionDecode(b []byte) (sessionProto, bool) {
	var out sessionProto
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 || key>>3 == 0 {
			return nil, false
		}
		b = b[n:]
		field := sessionProtoField{num: key >> 3}
		switch key & 7 {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return nil, false
			}
			field.value, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return nil, false
			}
			field.value, b = binary.LittleEndian.Uint64(b), b[8:]
		case 2:
			size, n := binary.Uvarint(b)
			if n <= 0 || size > uint64(len(b)-n) {
				return nil, false
			}
			field.data, field.bytes, b = b[n:n+int(size)], true, b[n+int(size):]
		case 5:
			if len(b) < 4 {
				return nil, false
			}
			field.value, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		default:
			return nil, false
		}
		out = append(out, field)
	}
	return out, true
}

// uint returns the last varint of field num (protobuf's last-wins rule).
func (p sessionProto) uint(num uint64) (uint64, bool) {
	var v uint64
	found := false
	for _, f := range p {
		if f.num == num && !f.bytes {
			v, found = f.value, true
		}
	}
	return v, found
}

// count is a non-negative counter; anything beyond int64 is clamped.
func (p sessionProto) count(num uint64) int64 {
	v, _ := p.uint(num)
	return int64(min(v, 1<<63-1))
}

func (p sessionProto) raw(num uint64) ([]byte, bool) {
	var data []byte
	found := false
	for _, f := range p {
		if f.num == num && f.bytes {
			data, found = f.data, true
		}
	}
	return data, found
}

func (p sessionProto) message(num uint64) (sessionProto, bool) {
	data, ok := p.raw(num)
	if !ok {
		return nil, false
	}
	return sessionDecode(data)
}

func (p sessionProto) messages(num uint64) []sessionProto {
	var out []sessionProto
	for _, f := range p {
		if f.num != num || !f.bytes {
			continue
		}
		if m, ok := sessionDecode(f.data); ok {
			out = append(out, m)
		}
	}
	return out
}

func (p sessionProto) text(num uint64) string {
	data, _ := p.raw(num)
	return string(data)
}

// timestamp reads a google.protobuf.Timestamp field.
func (p sessionProto) timestamp(num uint64) (time.Time, bool) {
	m, ok := p.message(num)
	if !ok {
		return time.Time{}, false
	}
	seconds, okSeconds := m.uint(1)
	nanos, _ := m.uint(2)
	if !okSeconds || seconds == 0 || seconds > 1<<40 || nanos >= 1e9 {
		return time.Time{}, false
	}
	return time.Unix(int64(seconds), int64(nanos)).UTC(), true
}

// duration reads a google.protobuf.Duration field.
func (p sessionProto) duration(num uint64) (time.Duration, bool) {
	m, ok := p.message(num)
	if !ok {
		return 0, false
	}
	seconds, _ := m.uint(1)
	nanos, _ := m.uint(2)
	if seconds > 1<<32 || nanos >= 1e9 || seconds == 0 && nanos == 0 {
		return 0, false
	}
	return time.Duration(seconds)*time.Second + time.Duration(nanos), true
}
