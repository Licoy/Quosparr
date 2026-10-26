package claudedesktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Application struct{ Path, Executable, DataDir string }
type LaunchRequest struct{ Application Application }
type Host interface {
	Resolve(context.Context, string) (Application, error)
	Stop(context.Context, Application) (bool, error)
	Start(context.Context, LaunchRequest) error
}

type hostProcess struct {
	executable string
	terminate  func() error // nil for helpers, which must leave with their parent
	exited     func() (bool, error)
	close      func()
}
type commandRunner func(context.Context, string, ...string) ([]byte, error)

// NativeHost has no system side effects until a method is called.
// Native boundaries are functions so tests never enumerate or signal real apps.
type NativeHost struct {
	home, platform string
	run            commandRunner
	candidates     func(context.Context) ([]string, error)
	dataDir        func(context.Context, string) (string, error)
	processes      func(context.Context, Application) ([]hostProcess, error)
	launch         func(context.Context, string, []string) error
	environ        func() []string
	stopTimeout    time.Duration
}

func runHostCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, path, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("Claude Desktop system command failed: %w", err)
	}
	return output, nil
}

func (h *NativeHost) Stop(ctx context.Context, app Application) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	verified, err := h.Resolve(ctx, app.Path)
	if err != nil {
		return false, err
	}
	if verified != app {
		return false, errors.New("Claude Desktop installation or data directory changed")
	}
	processes, err := h.processes(ctx, app)
	if err != nil {
		return false, err
	}
	defer closeHostProcesses(processes)
	matched := []hostProcess{}
	for _, p := range processes {
		if belongsToApplication(p.executable, app, h.platform) {
			matched = append(matched, p)
		}
	}
	for _, p := range matched {
		if err := ctx.Err(); err != nil {
			return len(matched) > 0, err
		}
		if p.terminate != nil {
			if err := p.terminate(); err != nil {
				return true, fmt.Errorf("Claude Desktop refused normal exit: %w", err)
			}
		}
	}
	deadline := time.NewTimer(h.stopTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		all := true
		for _, p := range matched {
			exited, err := p.exited()
			if err != nil {
				return true, err
			}
			all = all && exited
		}
		if all {
			// Re-enumeration also catches helpers spawned while normal exit was pending.
			remaining, err := h.processes(ctx, app)
			if err != nil {
				return len(matched) > 0, err
			}
			active := false
			for _, p := range remaining {
				if belongsToApplication(p.executable, app, h.platform) {
					active = true
				}
			}
			closeHostProcesses(remaining)
			if !active {
				return len(matched) > 0, nil
			}
		}
		select {
		case <-ctx.Done():
			return len(matched) > 0, ctx.Err()
		case <-deadline.C:
			return len(matched) > 0, errors.New("Claude Desktop or its helpers did not exit before timeout; quit Claude Desktop manually before switching")
		case <-tick.C:
		}
	}
}

func closeHostProcesses(processes []hostProcess) {
	for _, p := range processes {
		p.close()
	}
}

func sameExecutable(a, b, platform string) bool {
	if a == "" || b == "" {
		return false
	}
	if platform == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func belongsToApplication(exe string, app Application, platform string) bool {
	if sameExecutable(exe, app.Executable, platform) {
		return true
	}
	if platform == "darwin" {
		root := filepath.Join(app.Path, "Contents", "Frameworks") + string(filepath.Separator)
		return strings.HasPrefix(exe, root)
	}
	// Electron uses the main executable for renderer/utility processes and a
	// distinct crash handler; never widen matching to sibling CLI executables.
	name := filepath.Base(exe)
	if platform == "windows" {
		name = strings.ToLower(name)
	}
	return (name == "chrome_crashpad_handler" || name == "chrome_crashpad_handler.exe") && sameExecutable(filepath.Dir(exe), filepath.Dir(app.Executable), platform)
}

func desktopEnvironment(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		// Authentication stays in Desktop's official store; never pass CLI tokens,
		// routing overrides, debug hooks, or Quosparr's private environment to it.
		if strings.HasPrefix(key, "ELECTRON_") || strings.HasPrefix(key, "NODE_") || strings.HasPrefix(key, "QUOSPARR_") || strings.HasPrefix(key, "ANTHROPIC_") || strings.HasPrefix(key, "CLAUDE_") || key == "SSLKEYLOGFILE" {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (h *NativeHost) Start(ctx context.Context, req LaunchRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	app, err := h.Resolve(ctx, req.Application.Path)
	if err != nil {
		return err
	}
	if app != req.Application {
		return errors.New("Claude Desktop installation or data directory changed")
	}
	processes, err := h.processes(ctx, app)
	if err != nil {
		return err
	}
	defer closeHostProcesses(processes)
	for _, p := range processes {
		if belongsToApplication(p.executable, app, h.platform) {
			return errors.New("Claude Desktop or its helpers are still running")
		}
	}
	return h.launch(ctx, app.Executable, desktopEnvironment(h.environ()))
}

func newHost(home, platform string) *NativeHost {
	return &NativeHost{home: home, platform: platform, run: runHostCommand, launch: launchHostProcess, environ: os.Environ, stopTimeout: 15 * time.Second}
}
