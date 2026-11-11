//go:build !windows

package codexruntime

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUnsafeBinaryParent(t *testing.T) {
	c, _ := fixture(t)
	t.Setenv("PATH", t.TempDir())
	os.Chmod(filepath.Join(c.CodexHome, "packages"), 0777)
	s, err := c.Inspect(context.Background())
	if err == nil || s.Status != Unsupported {
		t.Fatal("writable executable parent accepted")
	}
}

func TestStaleSocketIsNotRunning(t *testing.T) {
	for _, mode := range []string{"with_binary", "without_binary"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := fixture(t)
			if mode == "without_binary" {
				t.Setenv("PATH", t.TempDir())
				if err := os.Remove(filepath.Join(c.CodexHome, "packages", "app-server-daemon", "current", "bin", "codex")); err != nil {
					t.Fatal(err)
				}
			}
			entry := filepath.Join(c.CodexHome, "app-server-control", "app-server-control.sock")
			target, err := os.Readlink(entry)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(target); err != nil {
				t.Fatal(err)
			}
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: target, Net: "unix"})
			if err != nil {
				t.Fatal(err)
			}
			listener.SetUnlinkOnClose(false)
			listener.Close()
			os.Chmod(target, 0600)
			if inspect(t, c).Status != NotRunning {
				t.Fatal("stale endpoint reported running")
			}
		})
	}
}

func TestAbsentSocketWithoutBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, mode := range []string{"missing_control", "private_control"} {
		t.Run(mode, func(t *testing.T) {
			c := Client{CodexHome: t.TempDir()}
			if mode == "private_control" {
				if err := os.Mkdir(filepath.Join(c.CodexHome, "app-server-control"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			s := inspect(t, c)
			if s.Status != NotRunning {
				t.Fatalf("missing endpoint: %s", s.Status)
			}
			if _, err := c.Start(context.Background(), s); err == nil || err.Error() != "codex runtime: start: no_trusted_binary" {
				t.Fatalf("start without binary: %v", err)
			}
		})
	}
}

func TestInheritedCommandOutputIsBounded(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(home, "fake")
	os.WriteFile(binary, []byte("#!/bin/sh\nsleep 2 &\necho $! > \"$CODEX_HOME/child\"\nexit 0\n"), 0700)
	started := time.Now()
	_, err := command(context.Background(), binary, home, "start")
	data, _ := os.ReadFile(filepath.Join(home, "child"))
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid > 0 {
		if child, e := os.FindProcess(pid); e == nil {
			child.Kill()
		}
	}
	if err == nil || time.Since(started) > time.Second {
		t.Fatal("inherited output was not bounded")
	}
}
func TestAbsentSocketDoesNotBypassParentTrust(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, dir := range []string{"home", "control"} {
		t.Run(dir, func(t *testing.T) {
			c := Client{CodexHome: t.TempDir()}
			unsafePath, want := c.CodexHome, "codex runtime: discovery: untrusted_home"
			if dir == "control" {
				unsafePath = filepath.Join(c.CodexHome, "app-server-control")
				want = "codex runtime: connection: untrusted_socket"
				if err := os.Mkdir(unsafePath, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(unsafePath, 0777); err != nil {
				t.Fatal(err)
			}
			s, err := c.Inspect(context.Background())
			if err == nil || err.Error() != want || s.Status == NotRunning {
				t.Fatalf("absent socket bypassed unsafe directory: status=%s err=%v", s.Status, err)
			}
		})
	}
}

func TestStopWithoutSocketPreservesStartBinary(t *testing.T) {
	c, _ := fixture(t)
	entry := filepath.Join(c.CodexHome, "app-server-control", "app-server-control.sock")
	target, err := os.Readlink(entry)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUOSPARR_TEST_SOCKET", target)
	binary := filepath.Join(c.CodexHome, "packages", "app-server-daemon", "current", "bin", "codex")
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Replace(string(data), "stop) ", "stop) rm \"$CODEX_HOME/app-server-control/app-server-control.sock\"; ", 1)
	script = strings.Replace(script, "start) ", "start) ln -s \"$QUOSPARR_TEST_SOCKET\" \"$CODEX_HOME/app-server-control/app-server-control.sock\"; ", 1)
	if err = os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	before := inspect(t, c)
	stopped, err := c.Stop(context.Background(), before)
	if err != nil || stopped.Status != NotRunning {
		t.Fatalf("stop: %v", err)
	}
	if _, err = os.Lstat(entry); !os.IsNotExist(err) {
		t.Fatalf("stop kept endpoint: %v", err)
	}
	if stopped.binary != before.binary || stopped.binaryInfo == nil || !os.SameFile(stopped.binaryInfo, before.binaryInfo) {
		t.Fatal("stopped snapshot lost trusted binary identity")
	}
	started, err := c.Start(context.Background(), stopped)
	if err != nil || started.Status != Running {
		t.Fatalf("start: %v", err)
	}
	actions, err := os.ReadFile(filepath.Join(c.CodexHome, "actions"))
	if err != nil || string(actions) != "stopstart" {
		t.Fatalf("unexpected control actions: %q, %v", actions, err)
	}
}

func TestDanglingControlDirectoryIsRejected(t *testing.T) {
	c, _ := fixture(t)
	control := filepath.Join(c.CodexHome, "app-server-control")
	if err := os.RemoveAll(control); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(c.CodexHome, "missing"), control); err != nil {
		t.Fatal(err)
	}
	s, err := c.Inspect(context.Background())
	if err == nil || s.Status == NotRunning {
		t.Fatal("dangling control directory accepted")
	}
	if _, err = c.Start(context.Background(), s); err == nil {
		t.Fatal("dangling control directory allowed start")
	}
}
func TestDanglingSocketLeafIsNotRunning(t *testing.T) {
	c, _ := fixture(t)
	entry := filepath.Join(c.CodexHome, "app-server-control", "app-server-control.sock")
	target, err := os.Readlink(entry)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if inspect(t, c).Status != NotRunning {
		t.Fatal("dangling socket leaf not recognized")
	}
}

func TestRelativeHomeMapsToSameAbsoluteDaemon(t *testing.T) {
	c, _ := fixture(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, c.CodexHome)
	if err != nil {
		t.Fatal(err)
	}
	relativeClient := Client{CodexHome: filepath.Join(relative, ".")}
	expected := inspect(t, c)
	actual := inspect(t, relativeClient)
	if actual.home != c.CodexHome || !same(expected, actual) {
		t.Fatal("relative home did not resolve to the same daemon")
	}
	other, _ := fixture(t)
	if _, err = other.Stop(context.Background(), actual); err == nil {
		t.Fatal("relative home snapshot accepted by a different home")
	}
	stopped, err := relativeClient.Stop(context.Background(), expected)
	if err != nil || stopped.Status != NotRunning {
		t.Fatalf("relative home stop: %v", err)
	}
	if inspect(t, other).Status != Running {
		t.Fatal("other home was changed")
	}
	if _, err = (Client{}).Inspect(context.Background()); err == nil {
		t.Fatal("empty home was accepted")
	}
}
