package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestProbeHelper runs solely as a child of the tests below; it never contacts an
// external server or reads any installed CLI configuration.
func TestProbeHelper(t *testing.T) {
	mode := os.Getenv("MCP_PROBE_HELPER")
	if mode == "" {
		return
	}
	if mode == "child" {
		conn, e := net.Dial("tcp", os.Getenv("MCP_PROBE_LISTENER"))
		if e != nil {
			os.Exit(21)
		}
		defer conn.Close()
		fmt.Fprintf(conn, "%d\n", os.Getpid())
		io.Copy(io.Discard, conn)
		os.Exit(0)
	}
	if os.Getenv("CODEX_API_KEY") != "" || os.Getenv("QUOSPARR_PROBE_PRIVATE") != "" {
		os.Exit(22)
	}
	if mode == "tree" || mode == "detached" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProbeHelper$")
		cmd.Env = append(os.Environ(), "MCP_PROBE_HELPER=child")
		cmd.Stderr = io.Discard
		if mode == "detached" {
			probeTestDetach(cmd)
			cmd.Stderr = os.Stderr
			cmd.Stdout = os.Stdout
		}
		if cmd.Start() != nil {
			os.Exit(23)
		}
	}
	if mode == "args" {
		if os.Args[len(os.Args)-1] != "$(touch never-execute-shell)" {
			os.Exit(24)
		}
		fmt.Fprint(os.Stderr, strings.Repeat("private-stderr", 200000))
	}
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var req struct {
			ID     any
			Method string
		}
		if json.Unmarshal(scan.Bytes(), &req) != nil {
			os.Exit(25)
		}
		if req.ID == nil {
			continue
		}
		var result any
		switch req.Method {
		case "server/discover":
			if mode == "hang" {
				time.Sleep(time.Minute)
				continue
			}
			if mode == "legacy" {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32602, "message": "private-error"}})
				continue
			}
			result = probeDiscovery(true)
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "test-server", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{probeTestTool("safe")}}
		default:
			os.Exit(26)
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		if mode == "quick" && req.Method == "tools/list" {
			os.Exit(0)
		}
	}
	os.Exit(0)
}
func TestProbeProcessModernLegacyAndNoShell(t *testing.T) {
	t.Setenv("CODEX_API_KEY", "must-not-inherit")
	t.Setenv("QUOSPARR_PROBE_PRIVATE", "must-not-inherit")
	for _, mode := range []string{"modern", "legacy", "args", "quick"} {
		t.Run(mode, func(t *testing.T) {
			draft := Draft{Name: "fake", Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestProbeHelper$"}, Env: map[string]string{"MCP_PROBE_HELPER": mode}}
			if mode == "args" {
				draft.Args = append(draft.Args, "--", "$(touch never-execute-shell)")
			}
			result, err := Probe(context.Background(), draft)
			if err != nil || len(result.Tools) != 1 {
				t.Fatalf("%+v %v", result, err)
			}
			want := probeVersion
			if mode == "legacy" {
				want = "2025-11-25"
			}
			if result.ProtocolVersion != want {
				t.Fatal("wrong protocol")
			}
		})
	}
}
func TestProbeProcessCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := Probe(ctx, Draft{Name: "fake", Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestProbeHelper$"}, Env: map[string]string{"MCP_PROBE_HELPER": "hang"}})
	assertProbeCode(t, err, "timeout")
	if time.Since(started) > 2*time.Second {
		t.Fatal("process cleanup stalled")
	}
}
func TestProbeProcessTreeCleanup(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
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
			// The helper can finish discovery before its descendant connects; hold the
			// probe open until the connection proves the descendant has actually started.
			draft := Draft{Name: "fake", Transport: "stdio", Command: os.Args[0], Args: []string{"-test.run=^TestProbeHelper$"}, Env: map[string]string{"MCP_PROBE_HELPER": "tree", "MCP_PROBE_LISTENER": listener.Addr().String()}}
			wire, err := newProbeProcess(ctx, draft)
			if err != nil {
				t.Fatal(err)
			}
			defer wire.close()
			var conn net.Conn
			select {
			case conn = <-accepted:
			case <-ctx.Done():
				wire.close()
				t.Fatal("descendant did not start")
			}
			defer conn.Close()
			assertChildExit := probeTestChildExit(t, conn)
			if cancelled {
				cancel()
			} else {
				wire.close()
			}
			assertChildExit()
			wire.close()
		})
	}
}

func TestProbeWindowsEnvironmentMerge(t *testing.T) {
	env := probeMergeEnv([]string{"PATH=old", "SystemRoot=system"}, map[string]string{"Path": "explicit"}, true)
	if len(env) != 2 || env[0] != "PATH=explicit" || env[1] != "SYSTEMROOT=system" {
		t.Fatalf("unexpected merged env %v", env)
	}
}
