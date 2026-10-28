// Package codexruntime inspects an existing managed Codex daemon without refreshing credentials.
package codexruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type Status string

const (
	Unknown     Status = "unknown"
	Unsupported Status = "unsupported"
	NotRunning  Status = "not_running"
	Running     Status = "running"
)

type Match string

const (
	Unverified Match = "unverified"
	Matched    Match = "matched"
	Mismatched Match = "mismatched"
)

// Identity omits credentials; AccountID alone can identify a shared workspace, not a person.
type Identity struct {
	AccountID, UserID, Email string
	API                      bool
}
type Snapshot struct {
	Status                 Status
	Identity               Identity
	Version                string
	home, binary           string
	commandDigest          [32]byte
	binaryInfo, socketInfo os.FileInfo
}

// Compare requires the workspace account and logged-in email; available user IDs add another check.
func (s Snapshot) Compare(target Identity) Match {
	if s.Status != Running || s.Identity.API || target.API || s.Identity.AccountID == "" || strings.TrimSpace(s.Identity.Email) == "" || target.AccountID == "" || strings.TrimSpace(target.Email) == "" {
		return Unverified
	}
	if s.Identity.AccountID == target.AccountID && strings.EqualFold(strings.TrimSpace(s.Identity.Email), strings.TrimSpace(target.Email)) && (s.Identity.UserID == "" || target.UserID == "" || s.Identity.UserID == target.UserID) {
		return Matched
	}
	return Mismatched
}

// Error contains only fixed, safe stage and reason codes, never subprocess or server text.
type Error struct{ Stage, Reason string }

func (e *Error) Error() string           { return "codex runtime: " + e.Stage + ": " + e.Reason }
func failure(stage, reason string) error { return &Error{Stage: stage, Reason: reason} }

type Client struct{ CodexHome string }

func (c Client) home() (string, error) {
	if c.CodexHome == "" {
		return "", failure("discovery", "invalid_home")
	}
	home, err := filepath.Abs(c.CodexHome)
	if err != nil {
		return "", failure("discovery", "invalid_home")
	}
	return filepath.Clean(home), nil
}
func boundedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}
func command(ctx context.Context, binary, home, action string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, "app-server", "daemon", action)
	cmd.WaitDelay = 250 * time.Millisecond
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "CODEX_HOME=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "CODEX_HOME="+home)
	var out limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, failure(action, "timeout_or_cancelled")
		}
		return nil, failure(action, "command_failed")
	}
	return out.Bytes(), nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 64*1024 {
		return 0, failure("command", "response_too_large")
	}
	return b.Buffer.Write(p)
}
func discover(home string) (string, os.FileInfo, error) {
	candidates := []string{filepath.Join(home, "packages", "app-server-daemon", "current", "bin", "codex")}
	if p, err := exec.LookPath("codex"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		info, err := os.Lstat(resolved)
		if err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 && trusted(info) && trustedParents(p) && trustedParents(resolved) {
			return resolved, info, nil
		}
	}
	return "", nil, failure("discovery", "no_trusted_binary")
}
func (c Client) Inspect(ctx context.Context) (Snapshot, error) {
	s := Snapshot{Status: Unknown}
	home, err := c.home()
	if err != nil {
		return s, err
	}
	s.home = home
	if runtime.GOOS == "windows" {
		s.Status = Unsupported
		return s, nil
	}
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	s.binary, s.binaryInfo, err = discover(home)
	if err != nil {
		s.Status = Unsupported
		return s, err
	}
	socket := filepath.Join(home, "app-server-control", "app-server-control.sock")
	homeInfo, homeErr := os.Stat(home)
	if homeErr != nil || !homeInfo.IsDir() || !trusted(homeInfo) || !trustedDirectoryPath(home) {
		return s, failure("discovery", "untrusted_home")
	}
	controlPath := filepath.Dir(socket)
	if _, linkErr := os.Lstat(controlPath); linkErr == nil {
		if !trustedDirectoryPath(controlPath) {
			return s, failure("connection", "untrusted_socket")
		}
	} else if !os.IsNotExist(linkErr) {
		return s, failure("connection", "socket_unavailable")
	}
	controlInfo, controlErr := os.Stat(filepath.Dir(socket))
	if controlErr != nil && !os.IsNotExist(controlErr) {
		return s, failure("connection", "socket_unavailable")
	}
	if controlErr == nil && (!controlInfo.IsDir() || !trusted(controlInfo) || !trustedDirectoryPath(filepath.Dir(socket))) {
		return s, failure("connection", "untrusted_socket")
	}

	if _, statErr := os.Stat(socket); os.IsNotExist(statErr) {
		// A stopped official daemon has no reachable socket and version exits nonzero.
		s.Status = NotRunning
		return s, nil
	} else if statErr != nil {
		return s, failure("connection", "socket_unavailable")
	}
	resolvedSocket, existingSocket, socketErr := trustedSocket(socket)
	if socketErr != nil {
		return s, socketErr
	}
	probe, probeErr := (&net.Dialer{}).DialContext(ctx, "unix", resolvedSocket)
	if errors.Is(probeErr, syscall.ECONNREFUSED) {
		s.Status = NotRunning
		s.socketInfo = existingSocket
		return s, nil
	}
	if probeErr != nil {
		return s, failure("connection", "unavailable")
	}
	probe.Close()
	data, err := command(ctx, s.binary, home, "version")
	if err != nil {
		return s, err
	}
	var v struct{ Status, Backend, SocketPath, ManagedCodexPath, ManagedCodexVersion, AppServerVersion string }
	if json.Unmarshal(data, &v) != nil {
		return s, failure("version", "malformed_response")
	}
	s.commandDigest = sha256.Sum256(data)
	s.Version = v.AppServerVersion
	switch v.Status {
	case "stopped", "not_running":
		s.Status = NotRunning
		return s, nil
	case "running":
	default:
		return s, failure("version", "unknown_status")
	}
	if v.Backend != "pid" || filepath.Clean(v.SocketPath) != socket {
		s.Status = Unsupported
		return s, failure("version", "unsupported_endpoint")
	}
	resolved, socketInfo, err := trustedSocket(socket)
	s.socketInfo = socketInfo
	if err != nil {
		return s, failure("connection", "untrusted_socket")
	}
	s.Identity, err = readIdentity(ctx, resolved)
	if err != nil {
		return s, err
	}
	resolvedAfter, after, err := trustedSocket(socket)
	if err != nil || resolvedAfter != resolved || !os.SameFile(after, s.socketInfo) {
		return s, failure("connection", "daemon_changed")
	}
	s.Status = Running
	return s, nil
}
func same(a, b Snapshot) bool {
	return a.home == b.home && a.binary == b.binary && a.Status == b.Status && a.commandDigest == b.commandDigest && a.Identity == b.Identity && a.binaryInfo != nil && b.binaryInfo != nil && os.SameFile(a.binaryInfo, b.binaryInfo) && a.binaryInfo.Size() == b.binaryInfo.Size() && a.binaryInfo.ModTime() == b.binaryInfo.ModTime() && ((a.socketInfo == nil && b.socketInfo == nil) || (a.socketInfo != nil && b.socketInfo != nil && os.SameFile(a.socketInfo, b.socketInfo)))
}

// Stop requires the unchanged inspected daemon; it never kills arbitrary processes.
func (c Client) Stop(ctx context.Context, expected Snapshot) (Snapshot, error) {
	return c.control(ctx, expected, "stop", Running, NotRunning)
}

// Start is explicit and requires a stopped snapshot returned by Inspect or Stop.
func (c Client) Start(ctx context.Context, expected Snapshot) (Snapshot, error) {
	return c.control(ctx, expected, "start", NotRunning, Running)
}
func (c Client) control(ctx context.Context, expected Snapshot, action string, before, after Status) (Snapshot, error) {
	current, err := c.Inspect(ctx)
	if err != nil {
		return current, err
	}
	if expected.Status != before || !same(expected, current) {
		return current, failure(action, "state_changed")
	}
	ctx, cancel := boundedContext(ctx)
	defer cancel()
	if _, err = command(ctx, current.binary, current.home, action); err != nil {
		return current, err
	}
	result, err := c.Inspect(ctx)
	if err != nil {
		return result, err
	}
	if result.Status != after {
		return result, failure(action, "state_not_confirmed")
	}
	return result, nil
}
