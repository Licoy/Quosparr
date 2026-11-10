package app

import (
	"errors"
	"testing"

	"github.com/licoy/quosparr/internal/runway"
)

func TestPrepareCodexRestartRetainsRecoveryAfterPartialQuit(t *testing.T) {
	opened := 0
	apps := []runningApp{
		{bundleID: "com.openai.codex", path: "/fake/ChatGPT.app", terminate: func() bool { return true }, stopped: func() bool { return true }},
		{bundleID: "com.openai.codex", path: "/fake/ChatGPT.app", terminate: func() bool { return false }},
	}
	recover, err := prepareCodexRestart(apps, "", nil, func(string) error { opened++; return nil })
	if !errors.Is(err, errRestartCodex) || recover == nil || opened != 0 {
		t.Fatalf("recovery=%v err=%v opens=%d", recover != nil, err, opened)
	}
	if err := recover(); err != nil || opened != 1 {
		t.Fatalf("recovery err=%v opens=%d", err, opened)
	}
}

func TestPrepareCodexRestartRecoveryRechecksRunningApp(t *testing.T) {
	for _, stillRunning := range []bool{true, false} {
		opened := 0
		apps := []runningApp{{bundleID: "com.openai.codex", path: "/fake/Codex.app", terminate: func() bool { return false }}}
		recover, err := prepareCodexRestart(apps, "", func() bool { return stillRunning }, func(string) error { opened++; return nil })
		if !errors.Is(err, errRestartCodex) || !errors.Is(err, runway.ErrCodexDesktopQuit) || recover == nil {
			t.Fatalf("recovery=%v err=%v", recover != nil, err)
		}
		if err := recover(); err != nil {
			t.Fatal(err)
		}
		if want := map[bool]int{true: 0, false: 1}[stillRunning]; opened != want {
			t.Fatalf("running=%v opens=%d", stillRunning, opened)
		}
	}
}
