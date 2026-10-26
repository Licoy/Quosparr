package claudedesktop

import (
	"errors"
	"reflect"
	"testing"
)

func TestWindowsOnlyWindowHostReceivesNormalClose(t *testing.T) {
	windows := []desktopWindow{{handle: 11, pid: 101}, {handle: 99, pid: 999}}
	processes := []hostProcess{{executable: "fixture/Claude.exe", terminate: func() error { return nil }}, {executable: "fixture/Claude.exe", terminate: func() error { t.Fatal("renderer was terminated"); return nil }}, {executable: "fixture/Claude.exe", terminate: func() error { t.Fatal("utility was terminated"); return nil }}}
	for i, pid := range []uint32{101, 102, 103} {
		processes[i] = markWindowsWindowHost(processes[i], pid, windows)
	}
	if processes[0].terminate == nil || processes[1].terminate != nil || processes[2].terminate != nil {
		t.Fatal("window host and helpers were not distinguished")
	}
	sent := []uintptr{}
	api := desktopCloseAPI{windows: func() ([]desktopWindow, error) { return windows, nil }, sameProcess: func() (bool, error) { return true, nil }, postClose: func(hwnd uintptr) error { sent = append(sent, hwnd); return nil }}
	if err := requestDesktopWindowsClose(101, api); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sent, []uintptr{11}) {
		t.Fatal("closed another process's window", sent)
	}
}

func TestWindowsCloseRejectsMissingWindowAndReusedProcess(t *testing.T) {
	for _, mode := range []string{"no-window", "pid-reused", "enumeration-failed", "identity-failed", "close-refused"} {
		t.Run(mode, func(t *testing.T) {
			sent := false
			api := desktopCloseAPI{
				windows: func() ([]desktopWindow, error) {
					if mode == "enumeration-failed" {
						return nil, errors.New("fixture failure")
					}
					pid := uint32(10)
					if mode == "no-window" {
						pid = 11
					}
					return []desktopWindow{{handle: 1, pid: pid}}, nil
				},
				sameProcess: func() (bool, error) {
					if mode == "identity-failed" {
						return false, errors.New("fixture failure")
					}
					return mode != "pid-reused", nil
				},
				postClose: func(uintptr) error { sent = true; return errors.New("fixture refusal") },
			}
			if err := requestDesktopWindowsClose(10, api); err == nil {
				t.Fatal("failure was swallowed")
			}
			if mode != "close-refused" && sent {
				t.Fatal("sent close to an unverified process")
			}
		})
	}
}
