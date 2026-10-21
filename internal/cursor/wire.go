package cursor

import (
	"encoding/binary"
	"math"
)

// A bounded reader for the scalar/message fields of the desktop unary protocol.
// It skips unknown fields, preserves optional zero values and rejects malformed
// lengths, duplicate singular fields and known fields with changed wire types.
type wireValue struct {
	kind byte
	n    uint64
	data []byte
}
type message map[uint32][]wireValue

func readMessage(b []byte) (message, error) {
	m := message{}
	for count := 0; len(b) > 0; count++ {
		if count >= 1024 {
			return nil, ErrResponse
		}
		tag, n := binary.Uvarint(b)
		if n <= 0 || tag>>3 == 0 || tag>>3 > (1<<29)-1 {
			return nil, ErrResponse
		}
		b = b[n:]
		v := wireValue{kind: byte(tag & 7)}
		switch v.kind {
		case 0:
			v.n, n = binary.Uvarint(b)
			if n <= 0 {
				return nil, ErrResponse
			}
			b = b[n:]
		case 1:
			if len(b) < 8 {
				return nil, ErrResponse
			}
			v.n = binary.LittleEndian.Uint64(b)
			b = b[8:]
		case 2:
			size, n := binary.Uvarint(b)
			if n <= 0 || size > uint64(len(b)-n) {
				return nil, ErrResponse
			}
			b = b[n:]
			v.data = b[:int(size)]
			b = b[int(size):]
		case 5:
			if len(b) < 4 {
				return nil, ErrResponse
			}
			v.n = uint64(binary.LittleEndian.Uint32(b))
			b = b[4:]
		default:
			return nil, ErrResponse
		}
		key := uint32(tag >> 3)
		m[key] = append(m[key], v)
	}
	return m, nil
}

func (m message) one(key uint32, kind byte) (wireValue, bool, error) {
	v := m[key]
	if len(v) == 0 {
		return wireValue{}, false, nil
	}
	if len(v) != 1 || v[0].kind != kind {
		return wireValue{}, false, ErrResponse
	}
	return v[0], true, nil
}

func (m message) integer(key uint32) (*int64, error) {
	v, ok, err := m.one(key, 0)
	if err != nil || !ok {
		return nil, err
	}
	if v.n > math.MaxInt64 {
		return nil, ErrResponse
	}
	n := int64(v.n)
	return &n, nil
}

func (m message) percent(key uint32) (*float64, error) {
	v, ok, err := m.one(key, 1)
	if err != nil || !ok {
		return nil, err
	}
	n := math.Float64frombits(v.n)
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return nil, ErrResponse
	}
	return &n, nil
}

func (m message) nested(key uint32) (message, error) {
	v, ok, err := m.one(key, 2)
	if err != nil || !ok {
		return nil, err
	}
	return readMessage(v.data)
}

func (m message) text(key uint32) (string, error) {
	v, _, err := m.one(key, 2)
	return cleanLabel(string(v.data)), err
}

func number(n *int64) int64 {
	if n == nil {
		return 0
	}
	return *n
}
