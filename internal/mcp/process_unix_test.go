//go:build !windows

package mcp

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func probeTestDetach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// POSIX process groups cover descendants that remain in that group, not a
// server deliberately creating a new session. This verifies bounded cleanup
// even then; the test, which owns the fake descendant PID, explicitly reaps it.
func TestProbeDetachedProcessCannotBlockCleanup(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, e := listener.Accept()
		if e == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	wire, err := newProbeProcess(ctx, Draft{Name: "fake", Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestProbeHelper$"}, Env: map[string]string{"MCP_PROBE_HELPER": "detached", "MCP_PROBE_LISTENER": listener.Addr().String()}})
	if err != nil {
		t.Fatal(err)
	}
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-ctx.Done():
		wire.close()
		t.Fatal("fake descendant missing")
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		wire.close()
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		wire.close()
		t.Fatal(err)
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		wire.close()
		t.Fatal(err)
	}
	defer child.Kill()
	start := time.Now()
	wire.close()
	if time.Since(start) > time.Second {
		t.Fatal("cleanup blocked on inherited descriptors")
	}
}
