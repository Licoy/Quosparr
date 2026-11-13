package app

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/platform"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want Command
	}{
		{nil, CommandRun},
		{[]string{"--help"}, CommandHelp},
		{[]string{"-h"}, CommandHelp},
		{[]string{"--self-check"}, CommandSelfCheck},
		{[]string{"--self-check", "--help"}, CommandHelp},
		{[]string{"--settings"}, CommandSettings},
		{[]string{"--settings", "--unknown"}, CommandSettings},
		{[]string{"--settings", "--self-check"}, CommandSelfCheck},
		{[]string{"--self-check", "--settings", "--help"}, CommandHelp},
		{[]string{"--settings", "-h"}, CommandHelp},
		{[]string{"--unknown"}, CommandRun},
		{[]string{"-psn_0_12345"}, CommandRun},
	}
	for _, c := range cases {
		if got := ParseArgs(c.args); got != c.want {
			t.Errorf("ParseArgs(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestLaunchActionsInitialReady(t *testing.T) {
	for _, command := range []Command{CommandRun, CommandSettings} {
		var calls []string
		actions := launchRecorder(&calls)
		actions.InitialReady(command)
		want := []string{"ready"}
		if command == CommandSettings {
			want = append(want, "settings")
		}
		if !slices.Equal(calls, want) {
			t.Errorf("InitialReady(%v) called %v, want %v", command, calls, want)
		}
	}
}

func TestLaunchActionsFirstLaunchShowsSettingsOnce(t *testing.T) {
	for _, command := range []Command{CommandRun, CommandSettings} {
		var calls []string
		actions := launchRecorder(&calls)
		actions.FirstLaunch = true
		actions.InitialReady(command)
		if want := []string{"ready", "settings"}; !slices.Equal(calls, want) {
			t.Errorf("first launch %v called %v, want %v", command, calls, want)
		}
	}
}

func TestLaunchActionsSecondInstance(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, "panel"},
		{[]string{"--unknown"}, "panel"},
		{[]string{"-psn_0_12345"}, "panel"},
		{[]string{"--settings"}, "settings"},
		{[]string{"--settings", "--unknown"}, "settings"},
		{[]string{"--settings", "--self-check"}, "panel"},
		{[]string{"--settings", "--help"}, "panel"},
	}
	for _, c := range cases {
		var calls []string
		actions := launchRecorder(&calls)
		actions.SecondInstance(c.args, "/unused")
		if !slices.Equal(calls, []string{c.want}) {
			t.Errorf("SecondInstance(%v) called %v, want %v", c.args, calls, c.want)
		}
	}
}

func TestHelpTextIncludesSettings(t *testing.T) {
	if !strings.Contains(HelpText, "--settings") {
		t.Fatal("help must advertise --settings")
	}
}

func launchRecorder(calls *[]string) LaunchActions {
	return LaunchActions{
		Ready:        func() { *calls = append(*calls, "ready") },
		OpenPanel:    func() { *calls = append(*calls, "panel") },
		OpenSettings: func() { *calls = append(*calls, "settings") },
	}
}

func TestSelfCheckReadsOnlyAndDoesNotCreate(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	SelfCheck(&out, home, "1.2.3")
	got := out.String()
	for _, want := range []string{
		"quosparr: 1.2.3",
		"home: ok",
		"quosparr dir: missing (" + filepath.Join("~", ".quosparr"),
		"lock: not created yet",
		"codex dir: missing",
		"not ported yet:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, home) {
		t.Errorf("output leaks the home path:\n%s", got)
	}
	if _, err := os.Stat(platform.RunwayDir(home)); err == nil {
		t.Error("self-check must not create ~/.quosparr")
	}
}

func TestSelfCheckReportsLockAndCodexFiles(t *testing.T) {
	home := t.TempDir()
	lock, err := platform.AcquireLock(platform.LockPath(home))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	codex := platform.CodexDir(home)
	if err := os.MkdirAll(codex, 0o700); err != nil {
		t.Fatal(err)
	}
	// Contents must never reach the output.
	if err := os.WriteFile(filepath.Join(codex, "auth.json"), []byte(`{"access_token":"SECRET"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	SelfCheck(&out, home, "")
	got := out.String()
	for _, want := range []string{"lock: held by another instance", "codex auth: present", "session index: missing", "quosparr dir: ok"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "SECRET") {
		t.Error("secret leaked")
	}
}

func TestSelfCheckReportsAntigravityLoginFileOnly(t *testing.T) {
	home := t.TempDir()
	var out bytes.Buffer
	SelfCheck(&out, home, "")
	if !strings.Contains(out.String(), "antigravity login file: missing") {
		t.Fatalf("output lacks the missing login file:\n%s", out.String())
	}
	path := gemini.OfficialFiles(home)[0]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"refresh_token":"SECRET"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	SelfCheck(&out, home, "")
	got := out.String()
	if !strings.Contains(got, "antigravity login file: present") || strings.Contains(got, "SECRET") || strings.Contains(got, home) {
		t.Fatalf("output:\n%s", got)
	}
}
