//go:build !windows

package codexruntime

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type testDaemon struct {
	mu              sync.Mutex
	identity        Identity
	malformed, hang bool
	closed          chan struct{}
}

func fixture(t *testing.T) (Client, *testDaemon) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Keep Unix socket paths below the platform's small sockaddr_un limit.
	dir, err := os.MkdirTemp("", "qr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	os.Chmod(socket, 0600)
	control := filepath.Join(home, "app-server-control")
	os.MkdirAll(control, 0700)
	os.Symlink(socket, filepath.Join(control, "app-server-control.sock"))
	binary := filepath.Join(home, "packages", "app-server-daemon", "current", "bin", "codex")
	os.MkdirAll(filepath.Dir(binary), 0700)
	script := `#!/bin/sh
case "$4" in
version) if [ -e "$CODEX_HOME/stopped" ]; then printf '{"status":"stopped"}'; else printf '{"status":"running","backend":"pid","socketPath":"%s/app-server-control/app-server-control.sock","appServerVersion":"0.165.0"}' "$CODEX_HOME"; fi ;;
stop) printf stop >> "$CODEX_HOME/actions"; touch "$CODEX_HOME/stopped" ;;
start) printf start >> "$CODEX_HOME/actions"; if [ -e "$CODEX_HOME/fail-start" ]; then exit 1; fi; rm "$CODEX_HOME/stopped" ;;
*) exit 2 ;;
esac
`
	// app-server daemon action is argument 3.
	script = strings.Replace(script, `"$4"`, `"$3"`, 1)
	if err = os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	daemon := &testDaemon{identity: Identity{AccountID: "workspace", Email: "a@example.test"}, closed: make(chan struct{}, 32)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go daemon.serve(conn)
		}
	}()
	return Client{CodexHome: home}, daemon
}
func (d *testDaemon) serve(conn net.Conn) {
	defer conn.Close()
	defer func() { d.closed <- struct{}{} }()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	sum := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
	for {
		payload, err := readClientFrame(reader)
		if err != nil {
			return
		}
		var request struct {
			ID     int
			Method string
		}
		if json.Unmarshal(payload, &request) != nil {
			return
		}
		if request.Method == "initialized" {
			continue
		}
		d.mu.Lock()
		identity, malformed, hang := d.identity, d.malformed, d.hang
		d.mu.Unlock()
		if hang {
			io.Copy(io.Discard, reader)
			return
		}
		if malformed {
			writeServerFrame(conn, []byte("not json"))
			return
		}
		result := any(map[string]any{})
		if request.Method == "account/read" {
			var params struct{ Params struct{ RefreshToken bool } }
			json.Unmarshal(payload, &params)
			if params.Params.RefreshToken {
				return
			}
			result = map[string]any{"account": map[string]string{"type": "chatgpt", "email": identity.Email, "userId": identity.UserID}, "workspaceRouting": map[string]string{"chatgptAccountId": identity.AccountID}}
		}
		data, _ := json.Marshal(map[string]any{"id": request.ID, "result": result})
		if writeServerFrame(conn, data) != nil {
			return
		}
	}
}
func readClientFrame(r io.Reader) ([]byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := int(h[1] & 127)
	if n == 126 {
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return nil, err
		}
		n = int(binary.BigEndian.Uint16(b[:]))
	}
	if n > messageLimit {
		return nil, io.ErrShortBuffer
	}
	var mask [4]byte
	if _, err := io.ReadFull(r, mask[:]); err != nil {
		return nil, err
	}
	data := make([]byte, n)
	_, err := io.ReadFull(r, data)
	for i := range data {
		data[i] ^= mask[i%4]
	}
	return data, err
}
func writeServerFrame(w io.Writer, data []byte) error {
	header := []byte{0x81}
	if len(data) < 126 {
		header = append(header, byte(len(data)))
	} else {
		header = append(header, 126, byte(len(data)>>8), byte(len(data)))
	}
	_, err := w.Write(append(header, data...))
	return err
}
func inspect(t *testing.T, c Client) Snapshot {
	t.Helper()
	s, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestInspectAndControlledRestart(t *testing.T) {
	c, d := fixture(t)
	s := inspect(t, c)
	if s.Status != Running || s.Compare(Identity{AccountID: "workspace", Email: " A@EXAMPLE.TEST "}) != Matched {
		t.Fatal("identity not matched")
	}
	if s.Compare(Identity{AccountID: "workspace", Email: "b@example.test"}) != Mismatched {
		t.Fatal("shared workspace mistaken for same person")
	}
	if s.Compare(Identity{API: true}) != Unverified {
		t.Fatal("API key verified")
	}
	stopped, err := c.Stop(context.Background(), s)
	if err != nil || stopped.Status != NotRunning {
		t.Fatalf("stop: %v", err)
	}
	if _, err = c.Stop(context.Background(), s); err == nil {
		t.Fatal("stale stop accepted")
	}
	started, err := c.Start(context.Background(), stopped)
	if err != nil || started.Status != Running {
		t.Fatalf("start: %v", err)
	}
	d.mu.Lock()
	d.identity.Email = "b@example.test"
	d.mu.Unlock()
	if _, err = c.Stop(context.Background(), started); err == nil {
		t.Fatal("concurrent identity change accepted")
	}
	actions, _ := os.ReadFile(filepath.Join(c.CodexHome, "actions"))
	if string(actions) != "stopstart" {
		t.Fatal("unexpected control actions")
	}
}
func TestDifferentHomeAndFailedStart(t *testing.T) {
	c, _ := fixture(t)
	s := inspect(t, c)
	other, _ := fixture(t)
	if _, err := other.Stop(context.Background(), s); err == nil {
		t.Fatal("cross-home snapshot accepted")
	}
	stopped, err := c.Stop(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(c.CodexHome, "fail-start"), nil, 0600)
	if _, err = c.Start(context.Background(), stopped); err == nil || err.Error() != "codex runtime: start: command_failed" {
		t.Fatalf("failed start: %v", err)
	}
	if inspect(t, c).Status != NotRunning {
		t.Fatal("failed start reported running")
	}
}
func TestMalformedTimeoutMissingIdentityAndCleanup(t *testing.T) {
	for _, mode := range []string{"malformed", "timeout", "missing"} {
		t.Run(mode, func(t *testing.T) {
			c, d := fixture(t)
			d.mu.Lock()
			d.malformed = mode == "malformed"
			d.hang = mode == "timeout"
			if mode == "missing" {
				d.identity = Identity{}
			}
			d.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			s, err := c.Inspect(ctx)
			if mode == "missing" {
				if err != nil || s.Compare(Identity{AccountID: "a", Email: "a"}) != Unverified {
					t.Fatal("missing identity verified")
				}
			} else if err == nil || strings.Contains(err.Error(), c.CodexHome) {
				t.Fatalf("unsafe or missing failure: %v", err)
			}
			for connection := 0; connection < 2; connection++ {
				select {
				case <-d.closed:
				case <-time.After(time.Second):
					t.Fatal("connection not closed")
				}
			}
		})
	}
}
func TestNotRunningAndUntrustedSocket(t *testing.T) {
	c, _ := fixture(t)
	os.WriteFile(filepath.Join(c.CodexHome, "stopped"), nil, 0600)
	s := inspect(t, c)
	if s.Status != NotRunning {
		t.Fatal("not stopped")
	}
	if _, err := c.Stop(context.Background(), s); err == nil {
		t.Fatal("stopped daemon stopped")
	}
	os.Remove(filepath.Join(c.CodexHome, "stopped"))
	os.Chmod(filepath.Join(c.CodexHome, "app-server-control"), 0777)
	if _, err := c.Inspect(context.Background()); err == nil {
		t.Fatal("untrusted socket accepted")
	}
}
func TestNoBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s, err := (Client{CodexHome: t.TempDir()}).Inspect(context.Background())
	if s.Status != Unsupported || err == nil {
		t.Fatal("missing binary not reported")
	}
}

func TestStoppedOfficialSocketAbsent(t *testing.T) {
	c, _ := fixture(t)
	if err := os.Remove(filepath.Join(c.CodexHome, "app-server-control", "app-server-control.sock")); err != nil {
		t.Fatal(err)
	}
	s := inspect(t, c)
	if s.Status != NotRunning {
		t.Fatal("missing official control endpoint not recognized")
	}
	// version would be allowed to fail: it must not be executed when endpoint is absent.
	binary := filepath.Join(c.CodexHome, "packages", "app-server-daemon", "current", "bin", "codex")
	os.WriteFile(binary, []byte("#!/bin/sh\nexit 1\n"), 0700)
	if inspect(t, c).Status != NotRunning {
		t.Fatal("stopped version exit treated as unsupported")
	}
}
