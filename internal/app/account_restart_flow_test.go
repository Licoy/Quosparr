package app

import (
	"errors"
	"testing"
)

func TestPrepareCodexRestartRetainsRecoveryAfterPartialQuit(t *testing.T) {
	opened := 0
	apps := []runningApp{
		{bundleID: "com.openai.codex", path: "/fake/ChatGPT.app", terminate: func() bool { return true }, stopped: func() bool { return true }},
		{bundleID: "com.openai.codex", path: "/fake/ChatGPT.app", terminate: func() bool { return false }},
	}
	recover, err := prepareCodexRestart(apps, "", func(string) error { opened++; return nil })
	if !errors.Is(err, errRestartCodex) || recover == nil || opened != 0 {
		t.Fatalf("recovery=%v err=%v opens=%d", recover != nil, err, opened)
	}
	if err := recover(); err != nil || opened != 1 {
		t.Fatalf("recovery err=%v opens=%d", err, opened)
	}
}
