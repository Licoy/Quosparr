package claudedesktop

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeHostArchive(t *testing.T, path string, metadata string) {
	t.Helper()
	entry := map[string]any{"files": map[string]any{"package.json": map[string]any{"size": len(metadata), "offset": "0"}}}
	header, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	padded := (len(header) + 3) &^ 3
	data := make([]byte, 16+padded)
	binary.LittleEndian.PutUint32(data[0:4], 4)
	binary.LittleEndian.PutUint32(data[4:8], uint32(8+padded))
	binary.LittleEndian.PutUint32(data[8:12], uint32(4+padded))
	binary.LittleEndian.PutUint32(data[12:16], uint32(len(header)))
	copy(data[16:], header)
	data = append(data, []byte(metadata)...)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func fixtureHost(t *testing.T, platform string) *NativeHost {
	t.Helper()
	home := t.TempDir()
	home, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	h := newHost(home, platform)
	exe := filepath.Join(home, "install", "claude-desktop")
	archive := filepath.Join(filepath.Dir(exe), "resources", "app.asar")
	if platform == "windows" {
		exe = filepath.Join(home, "install", "Claude.exe")
		archive = filepath.Join(filepath.Dir(exe), "resources", "app.asar")
	}
	if platform == "darwin" {
		exe = filepath.Join(home, "Claude.app", "Contents", "MacOS", "Claude")
		archive = filepath.Join(home, "Claude.app", "Contents", "Resources", "app.asar")
	}
	writeHostArchive(t, archive, `{"name":"@ant/desktop","productName":"Claude","main":".vite/build/index.pre.js"}`)
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("fixture only, never execute")
	if platform == "linux" {
		content = fixtureDesktopELF()
	}
	if err := os.WriteFile(exe, content, 0700); err != nil {
		t.Fatal(err)
	}
	path := exe
	if platform == "darwin" {
		path = filepath.Join(home, "Claude.app")
	}
	h.candidates = func(context.Context) ([]string, error) { return []string{path}, nil }
	h.dataDir = func(context.Context, string) (string, error) { return filepath.Join(home, "official", "Claude"), nil }
	h.processes = func(context.Context, Application) ([]hostProcess, error) { return nil, nil }
	h.launch = func(context.Context, string, []string) error { return errors.New("unexpected fixture launch") }
	h.environ = func() []string { return nil }
	h.run = func(_ context.Context, command string, args ...string) ([]byte, error) {
		switch {
		case command == "/usr/bin/plutil" && len(args) == 6 && args[1] == "CFBundleIdentifier":
			return []byte("com.anthropic.claudefordesktop"), nil
		case command == "/usr/bin/plutil" && len(args) == 6 && args[1] == "CFBundleExecutable":
			return []byte("Claude"), nil
		case command == "/usr/bin/codesign" && reflect.DeepEqual(args, []string{"--verify", "--deep", "--strict", path}):
			return nil, nil
		}
		t.Fatalf("unexpected system command %q %q", command, args)
		return nil, nil
	}
	h.stopTimeout = 15 * time.Millisecond
	return h
}

func TestHostResolveAndNormalLaunchAllPlatforms(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			h := fixtureHost(t, platform)
			app, err := h.Resolve(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			if app.DataDir != filepath.Join(h.home, "official", "Claude") {
				t.Fatal(app)
			}
			want := []string{"PATH=/fixture/bin", "HOME=/fixture/home", "LOCALAPPDATA=/fixture/local"}
			h.environ = func() []string {
				return append(append([]string{}, want...), "CLAUDE_USER_DATA_DIR=/forbidden", "CLAUDE_CDP_AUTH=secret", "ELECTRON_RUN_AS_NODE=1", "NODE_OPTIONS=--inspect", "ANTHROPIC_AUTH_TOKEN=secret", "QUOSPARR_TOKEN=secret")
			}
			launched := false
			h.launch = func(ctx context.Context, exe string, env []string) error {
				launched = true
				if exe != app.Executable || !reflect.DeepEqual(env, want) {
					t.Fatalf("wrong ordinary launch %q %q", exe, env)
				}
				return nil
			}
			if err = h.Start(context.Background(), LaunchRequest{Application: app}); err != nil || !launched {
				t.Fatalf("launch %v %v", launched, err)
			}
		})
	}
}

func TestHostResolveUsesFilesystemPermissions(t *testing.T) {
	for _, platform := range []string{"darwin", "windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			h := fixtureHost(t, platform)
			app, err := h.Resolve(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []os.FileMode{0600, 0700} {
				if err := os.Chmod(app.Executable, mode); err != nil {
					t.Fatal(err)
				}
				_, err := h.Resolve(context.Background(), app.Path)
				wantValid := runtime.GOOS == "windows" || mode&0111 != 0
				if wantValid && err != nil {
					t.Fatalf("mode %o on %s: %v", mode, runtime.GOOS, err)
				}
				if !wantValid && (err == nil || !strings.Contains(err.Error(), "executable is invalid")) {
					t.Fatalf("mode %o on %s must reject a non-executable file, got %v", mode, runtime.GOOS, err)
				}
			}
		})
	}
}

func TestHostRejectsCLIAndChangedInstallation(t *testing.T) {
	h := fixtureHost(t, "linux")
	app, err := h.Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Resolve(context.Background(), filepath.Join(h.home, "claude")); err == nil {
		t.Fatal("missing CLI accepted")
	}
	bad := app
	bad.DataDir = filepath.Join(h.home, "wrong")
	if err = h.Start(context.Background(), LaunchRequest{Application: bad}); err == nil {
		t.Fatal("changed directory accepted")
	}
	writeHostArchive(t, filepath.Join(filepath.Dir(app.Executable), "resources", "app.asar"), `{"name":"@anthropic-ai/claude-code","productName":"Claude"}`)
	if _, err = h.Resolve(context.Background(), app.Path); err == nil {
		t.Fatal("CLI masquerading as desktop accepted")
	}
}

func TestHostStopWaitsForHelpersAndDoesNotStopUnrelated(t *testing.T) {
	h := fixtureHost(t, "darwin")
	app, err := h.Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	stopped := false
	closed := 0
	scans := 0
	h.processes = func(context.Context, Application) ([]hostProcess, error) {
		scans++
		if stopped {
			return nil, nil
		}
		return []hostProcess{
			{executable: "/elsewhere/Claude.app/Contents/MacOS/Claude", terminate: func() error { t.Fatal("stopped unrelated app"); return nil }, exited: func() (bool, error) { return false, nil }, close: func() { closed++ }},
			{executable: app.Executable, terminate: func() error { stopped = true; return nil }, exited: func() (bool, error) { return stopped, nil }, close: func() { closed++ }},
			{executable: filepath.Join(app.Path, "Contents", "Frameworks", "Claude Helper.app", "Contents", "MacOS", "Claude Helper"), exited: func() (bool, error) { return stopped, nil }, close: func() { closed++ }},
		}, nil
	}
	running, err := h.Stop(context.Background(), app)
	if err != nil || !running || !stopped || closed != 3 || scans != 2 {
		t.Fatalf("running=%v stopped=%v closed=%d scans=%d err=%v", running, stopped, closed, scans, err)
	}
}

func TestHostRefusedExitTimeoutCancellationAndLaunchFailure(t *testing.T) {
	for _, mode := range []string{"refused", "timeout", "cancelled", "inspect-error", "launch-error", "still-running", "helper-only"} {
		t.Run(mode, func(t *testing.T) {
			h := fixtureHost(t, "linux")
			app, err := h.Resolve(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			p := hostProcess{executable: app.Executable, terminate: func() error {
				if mode == "refused" {
					return errors.New("refused")
				}
				return nil
			}, exited: func() (bool, error) { return false, nil }, close: func() {}}
			if mode == "helper-only" {
				p.executable = filepath.Join(filepath.Dir(app.Executable), "chrome_crashpad_handler")
				p.terminate = nil
			}
			h.processes = func(context.Context, Application) ([]hostProcess, error) {
				if mode == "inspect-error" {
					return nil, errors.New("cannot inspect")
				}
				if mode == "launch-error" {
					return nil, nil
				}
				return []hostProcess{p}, nil
			}
			if mode == "launch-error" || mode == "still-running" {
				if err = h.Start(ctx, LaunchRequest{Application: app}); err == nil {
					t.Fatal("start failure was swallowed")
				}
				return
			}
			if _, err = h.Stop(ctx, app); err == nil {
				t.Fatal("stop failure was swallowed")
			}
		})
	}
}

func TestChooseDesktopDataDirRejectsAmbiguity(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	if err := os.Mkdir(a, 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := chooseDesktopDataDir([]string{a, b}, "windows"); err != nil || got != a {
		t.Fatalf("%q %v", got, err)
	}
	if err := os.Mkdir(b, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := chooseDesktopDataDir([]string{a, b}, "windows"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal(err)
	}
}

func TestASARRejectsCorruptAndUntrustedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.asar")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyDesktopArchive(path); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	writeHostArchive(t, path, `{"name":"@ant/desktop","productName":"Claude-3p","main":".vite/build/index.pre.js"}`)
	if err := verifyDesktopArchive(path); err == nil {
		t.Fatal("third party profile accepted")
	}
}
