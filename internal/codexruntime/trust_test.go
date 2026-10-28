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
	c, _ := fixture(t)
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
	c, _ := fixture(t)
	os.Remove(filepath.Join(c.CodexHome, "app-server-control", "app-server-control.sock"))
	os.Chmod(filepath.Join(c.CodexHome, "app-server-control"), 0777)
	if _, err := c.Inspect(context.Background()); err == nil {
		t.Fatal("absent socket bypassed unsafe directory")
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
