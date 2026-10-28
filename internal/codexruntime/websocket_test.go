package codexruntime

import (
	"bufio"
	"bytes"
	"net"
	"testing"
)

type frameConn struct {
	net.Conn
	written bytes.Buffer
}

func (c *frameConn) Write(p []byte) (int, error) { return c.written.Write(p) }
func TestFrames(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame []byte
		valid bool
	}{
		{"text", []byte{0x81, 2, 'o', 'k'}, true},
		{"fragment_ping", []byte{0x01, 1, 'o', 0x89, 0, 0x80, 1, 'k'}, true},
		{"masked", []byte{0x81, 0x80}, false},
		{"extension", []byte{0xc1, 0}, false},
		{"continuation", []byte{0x80, 0}, false},
		{"fragmented_control", []byte{0x09, 0}, false},
		{"huge", []byte{0x81, 127, 0, 0, 0, 0, 0, 8, 0, 0}, false},
		{"close", []byte{0x88, 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := &frameConn{}
			rpc := socketRPC{conn: conn, reader: bufio.NewReader(bytes.NewReader(tc.frame))}
			data, err := rpc.message()
			if tc.valid {
				if err != nil || string(data) != "ok" {
					t.Fatalf("frame: %v", err)
				}
			} else if err == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
}
func TestUnexpectedRPC(t *testing.T) {
	for _, raw := range []string{`{"id":2,"result":{}}`, `{"id":1,"error":{"message":"secret"}}`, `invalid`} {
		var frame bytes.Buffer
		writeServerTestFrame(&frame, []byte(raw))
		rpc := socketRPC{conn: &frameConn{}, reader: bufio.NewReader(&frame)}
		if _, err := rpc.result(1); err == nil {
			t.Fatal("unexpected response accepted")
		}
	}
}
func writeServerTestFrame(out *bytes.Buffer, data []byte) {
	out.Write([]byte{0x81, byte(len(data))})
	out.Write(data)
}
