package usage

import (
	"bytes"
	"errors"
)

// LineHeadBytes is how much of an oversized log line is kept to tell what
// record it is.
const LineHeadBytes = 64 * 1024

// LineHead copies the first LineHeadBytes of a line whose start is split
// between buffered and next.
func LineHead(buffered, next []byte) []byte {
	head := make([]byte, 0, LineHeadBytes)
	head = append(head, buffered[:min(len(buffered), LineHeadBytes)]...)
	return append(head, next[:min(len(next), LineHeadBytes-len(head))]...)
}

var errHeadFound = errors.New("usage: head field found")

// HeadString reads the string at path, a chain of object keys, from the start
// of a JSON object that may be cut off. ok is false when the value is not a
// string or lies past the cut. For duplicate keys the first one wins, as in
// the record parser.
func HeadString(head []byte, path ...string) (string, bool) {
	if len(path) == 0 {
		return "", false
	}
	d := &jsonDec{b: head}
	if bytes.HasPrefix(d.b, []byte{0xEF, 0xBB, 0xBF}) {
		d.i = 3
	}
	var value string
	var walk func(depth int) error
	walk = func(depth int) error {
		return d.object(func(key string, first bool) error {
			if !first || key != path[depth] {
				return d.skip()
			}
			if depth < len(path)-1 {
				return walk(depth + 1)
			}
			if d.peek() != '"' {
				return errMalformed
			}
			s, err := d.decodeString()
			if err != nil {
				return err
			}
			value = s
			return errHeadFound
		})
	}
	return value, errors.Is(walk(0), errHeadFound)
}
