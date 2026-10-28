package codexruntime

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"
)

const messageLimit = 256 * 1024

type socketRPC struct {
	conn   net.Conn
	reader *bufio.Reader
}

func readIdentity(ctx context.Context, path string) (Identity, error) {
	var identity Identity
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return identity, failure("connection", "unavailable")
	}
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	rpc := socketRPC{conn: conn, reader: bufio.NewReader(conn)}
	if err = rpc.handshake(); err != nil {
		return identity, err
	}
	init := map[string]any{"clientInfo": map[string]string{"name": "quosparr_account_check", "version": "1"}, "capabilities": map[string]bool{"experimentalApi": true, "requestAttestation": false}}
	if err = rpc.send(map[string]any{"id": 1, "method": "initialize", "params": init}); err != nil {
		return identity, err
	}
	if _, err = rpc.result(1); err != nil {
		return identity, err
	}
	if err = rpc.send(map[string]any{"method": "initialized"}); err != nil {
		return identity, err
	}
	if err = rpc.send(map[string]any{"id": 2, "method": "account/read", "params": map[string]bool{"refreshToken": false}}); err != nil {
		return identity, err
	}
	result, err := rpc.result(2)
	if err != nil {
		return identity, err
	}
	var v struct {
		Account          *struct{ Type, Email, UserID string }
		WorkspaceRouting struct{ ChatgptAccountID string }
	}
	if json.Unmarshal(result, &v) != nil {
		return identity, failure("account", "malformed_response")
	}
	if v.Account == nil {
		return identity, nil
	}
	switch v.Account.Type {
	case "apiKey":
		identity.API = true
	case "chatgpt":
		identity.Email = v.Account.Email
		identity.AccountID = v.WorkspaceRouting.ChatgptAccountID
		identity.UserID = v.Account.UserID
	default:
		return identity, failure("account", "unsupported_identity")
	}
	return identity, nil
}
func (s socketRPC) handshake() error {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return failure("handshake", "random_failed")
	}
	key := base64.StdEncoding.EncodeToString(nonce[:])
	if _, err := fmt.Fprintf(s.conn, "GET / HTTP/1.1\r\nHost: localhost\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: %s\r\n\r\n", key); err != nil {
		return failure("handshake", "write_failed")
	}
	// ReadHeader is bounded independently of the JSON-RPC frame limit.
	var header strings.Builder
	for header.Len() < 16*1024 {
		lineBytes, err := s.reader.ReadSlice('\n')
		line := string(lineBytes)
		if err != nil || len(line)+header.Len() > 16*1024 {
			return failure("handshake", "invalid_response")
		}
		header.WriteString(line)
		if line == "\r\n" {
			break
		}
	}
	if !strings.HasSuffix(header.String(), "\r\n\r\n") {
		return failure("handshake", "invalid_response")
	}
	response, err := http.ReadResponse(bufio.NewReader(strings.NewReader(header.String())), nil)
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if err != nil || response.StatusCode != 101 || !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") || !headerToken(response.Header.Get("Connection"), "upgrade") || response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) || response.Header.Get("Sec-WebSocket-Extensions") != "" || response.Header.Get("Sec-WebSocket-Protocol") != "" {
		return failure("handshake", "invalid_response")
	}
	return nil
}
func headerToken(value, target string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(token), target) {
			return true
		}
	}
	return false
}
func (s socketRPC) send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return failure("protocol", "invalid_request")
	}
	return s.writeFrame(1, data)
}
func (s socketRPC) writeFrame(op byte, data []byte) error {
	if len(data) > messageLimit {
		return failure("protocol", "message_too_large")
	}
	header := []byte{0x80 | op}
	n := len(data)
	switch {
	case n < 126:
		header = append(header, byte(n)|0x80)
	case n <= 65535:
		header = append(header, 126|0x80, byte(n>>8), byte(n))
	default:
		header = append(header, 127|0x80, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return failure("protocol", "random_failed")
	}
	header = append(header, mask[:]...)
	frame := make([]byte, len(header)+n)
	copy(frame, header)
	for i, b := range data {
		frame[len(header)+i] = b ^ mask[i%4]
	}
	for len(frame) > 0 {
		n, err := s.conn.Write(frame)
		if err != nil || n == 0 {
			return failure("protocol", "write_failed")
		}
		frame = frame[n:]
	}
	return nil
}
func (s socketRPC) message() ([]byte, error) {
	var result []byte
	fragmented := false
	for {
		var head [2]byte
		if _, err := io.ReadFull(s.reader, head[:]); err != nil {
			return nil, failure("protocol", "read_failed")
		}
		final, op := head[0]&0x80 != 0, head[0]&15
		if head[0]&0x70 != 0 || head[1]&0x80 != 0 {
			return nil, failure("protocol", "invalid_frame")
		}
		length := uint64(head[1] & 127)
		if length == 126 {
			var n [2]byte
			if _, err := io.ReadFull(s.reader, n[:]); err != nil {
				return nil, failure("protocol", "read_failed")
			}
			length = uint64(binary.BigEndian.Uint16(n[:]))
		} else if length == 127 {
			var n [8]byte
			if _, err := io.ReadFull(s.reader, n[:]); err != nil {
				return nil, failure("protocol", "read_failed")
			}
			length = binary.BigEndian.Uint64(n[:])
		}
		if length > messageLimit || uint64(len(result))+length > messageLimit || op >= 8 && (!final || length > 125) {
			return nil, failure("protocol", "invalid_frame")
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(s.reader, payload); err != nil {
			return nil, failure("protocol", "read_failed")
		}
		switch op {
		case 8:
			return nil, failure("protocol", "closed")
		case 9:
			if err := s.writeFrame(10, payload); err != nil {
				return nil, err
			}
			continue
		case 10:
			continue
		case 1:
			if fragmented {
				return nil, failure("protocol", "invalid_frame")
			}
			fragmented = true
		case 0:
			if !fragmented {
				return nil, failure("protocol", "invalid_frame")
			}
		default:
			return nil, failure("protocol", "invalid_frame")
		}
		result = append(result, payload...)
		if final {
			if !utf8.Valid(result) {
				return nil, failure("protocol", "invalid_text")
			}
			return result, nil
		}
	}
}
func (s socketRPC) result(id int) (json.RawMessage, error) {
	for count := 0; count < 64; count++ {
		data, err := s.message()
		if err != nil {
			return nil, err
		}
		var envelope struct {
			ID     *int
			Method string
			Result json.RawMessage
			Error  json.RawMessage
		}
		if json.Unmarshal(data, &envelope) != nil {
			return nil, failure("protocol", "malformed_response")
		}
		if envelope.ID == nil && envelope.Method != "" {
			continue
		}
		if envelope.ID == nil || *envelope.ID != id || len(envelope.Error) > 0 || len(envelope.Result) == 0 {
			return nil, failure("protocol", "unexpected_response")
		}
		return envelope.Result, nil
	}
	return nil, failure("protocol", "notification_limit")
}
