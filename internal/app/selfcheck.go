package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/platform"
)

// SelfCheck prints redacted local diagnostics to w. It never touches the
// network, never prints credentials and only reads ~/.codex: it checks that
// files exist, not what is in them. home is the user's home directory.
func SelfCheck(w io.Writer, home, version string) {
	line := func(format string, args ...any) { fmt.Fprintf(w, format+"\n", args...) }

	line("quosparr: %s (%s/%s)", valueOr(version, "dev"), runtime.GOOS, runtime.GOARCH)
	if home == "" {
		line("home: unavailable")
		line("not ported yet: %s", strings.Join(NotPortedChecks, ", "))
		return
	}
	line("home: ok")

	runwayDir := platform.RunwayDir(home)
	line("quosparr dir: %s", describeRunwayDir(runwayDir, home))
	line("lock: %s", describeLock(platform.LockPath(home), home))

	codexDir := platform.CodexDir(home)
	switch {
	case !exists(codexDir):
		line("codex dir: missing")
	default:
		line("codex dir: present")
		line("codex auth: %s", presence(filepath.Join(codexDir, "auth.json")))
		line("session index: %s", presence(filepath.Join(codexDir, "session_index.jsonl")))
	}
	claudeDir := claude.ResolveConfigDir(os.Getenv("CLAUDE_CONFIG_DIR"), home)
	if exists(claudeDir) {
		line("claude config: present")
	} else {
		line("claude config: missing")
	}
	line("claude credentials file: %s", presence(claude.CredentialsFilePath(claudeDir)))
	if gemini.ClientInstalled(home) {
		line("antigravity client: present")
	} else {
		line("antigravity client: missing")
	}
	line("antigravity login file: %s", anyPresence(gemini.OfficialFiles(home)))
	line("not ported yet: %s", strings.Join(NotPortedChecks, ", "))
}

// NotPortedChecks lists what the Swift --self-check reports and the Go
// host does not yet.
var NotPortedChecks = []string{
	"auth summary (redacted)",
	"token validity",
	"sessions dry-run summary",
	"grok cli version",
	"grok auth status",
	"grok identity",
	"claude cli version",
	"claude auth identity",
}

func describeRunwayDir(dir, home string) string {
	fi, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return "missing (" + tilde(dir, home) + ", created on launch)"
	}
	if err != nil {
		return "unreadable (" + tilde(dir, home) + ")"
	}
	if !fi.IsDir() {
		return "not a directory (" + tilde(dir, home) + ")"
	}
	writable := "writable"
	probe, err := os.CreateTemp(dir, ".selfcheck-*")
	if err != nil {
		writable = "not writable"
	} else {
		probe.Close()
		_ = os.Remove(probe.Name())
	}
	return fmt.Sprintf("ok (%s, mode %04o, %s)", tilde(dir, home), fi.Mode().Perm(), writable)
}

func describeLock(path, home string) string {
	state, err := platform.ProbeLock(path)
	switch {
	case err != nil:
		return "unreadable (" + tilde(path, home) + ")"
	case state == platform.LockHeld:
		return "held by another instance (" + tilde(path, home) + ")"
	case state == platform.LockFree:
		return "free (" + tilde(path, home) + ")"
	}
	return "not created yet (" + tilde(path, home) + ")"
}

func presence(path string) string {
	if exists(path) {
		return "present"
	}
	return "missing"
}

// anyPresence is "present" when one of paths exists.
func anyPresence(paths []string) string {
	for _, path := range paths {
		if exists(path) {
			return "present"
		}
	}
	return "missing"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// tilde shortens paths under home so the user name stays out of reports.
func tilde(path, home string) string {
	if rest, ok := strings.CutPrefix(path, home); ok {
		return "~" + rest
	}
	return path
}

func valueOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
