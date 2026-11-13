package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

var errRestartCodex = errors.New("Codex restart failed")

// errCodexQuit lets the runtime status name the cause instead of [unknown].
var errCodexQuit = fmt.Errorf("%w: %w", errRestartCodex, runway.ErrCodexDesktopQuit)

// codexQuitLimit leaves room for the desktop app to save state before quitting.
const codexQuitLimit = 10 * time.Second

// runningApp is a running application that can be asked to quit.
type runningApp struct {
	bundleID, name, path string
	terminate            func() bool
	stopped              func() bool
	release              func()
}

// stopApp asks app to quit and waits up to limit for it to finish.
func stopApp(app runningApp, limit time.Duration) bool {
	if !app.terminate() {
		return false
	}
	deadline := time.Now().Add(limit)
	for !app.stopped() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
	return true
}

func isCodexDesktop(bundleID, name string) bool {
	return strings.ToLower(bundleID) == "com.openai.codex"
}

// runCodexRestart is also used by tests with fake processes and a fake opener.
// A termination request must actually finish before the login is relaunched.
func runCodexRestart(apps []runningApp, fallback string, running func() bool, open func(string) error) error {
	start, err := prepareCodexRestart(apps, fallback, running, open)
	if err != nil {
		return err
	}
	return start()
}

// After a failed quit the returned recovery reopens the app only when it has
// actually exited; running is checked again because the quit may finish late.
func prepareCodexRestart(apps []runningApp, fallback string, running func() bool, open func(string) error) (func() error, error) {
	return prepareAppRestart(appRestart{match: isCodexDesktop, failed: errRestartCodex, quit: errCodexQuit}, apps, fallback, running, open)
}

// appRestart names the desktop app a restart quits and reopens.
type appRestart struct {
	match func(bundleID, name string) bool
	// failed is returned when the app cannot be reopened, quit when it does
	// not quit in time.
	failed, quit error
}

func prepareAppRestart(target appRestart, apps []runningApp, fallback string, running func() bool, open func(string) error) (func() error, error) {
	path := fallback
	start := func() error {
		if path == "" {
			return target.failed
		}
		if err := open(path); err != nil {
			return target.failed
		}
		return nil
	}
	for _, app := range apps {
		if app.release != nil {
			defer app.release()
		}
		if !target.match(app.bundleID, app.name) {
			continue
		}
		if app.path != "" {
			path = app.path
		}
		if !stopApp(app, codexQuitLimit) {
			return func() error {
				if running != nil && running() {
					return nil
				}
				return start()
			}, target.quit
		}
	}
	if path == "" {
		return nil, target.failed
	}
	return start, nil
}

func rtSwitchCodex(l l10n.Localizer, id string, restartAfter bool) string {
	message, _ := rtSwitchCodexResult(l, id, restartAfter)
	return message
}

func rtSwitchCodexResult(l l10n.Localizer, id string, restartAfter bool) (string, bool) {
	rt := currentRuntime()
	if rt == nil {
		return l.Text(l10n.KeyAccountsSwitchFailed), false
	}
	return rt.switchCodexResult(l, id, restartAfter)
}

func (rt *liveApp) switchCodexResult(l l10n.Localizer, id string, restartAfter bool) (string, bool) {
	err := rt.svc.SwitchAccountWithRestart(context.Background(), id, restartAfter)
	status := rt.svc.Snapshot().Accounts.Runtime
	if err != nil {
		if errors.Is(err, runway.ErrDesktopProjects) {
			return codexRuntimeMessage(l, status), status.FilesUpdated
		}
		if errors.Is(err, runway.ErrCodexRuntime) {
			return codexRuntimeMessage(l, status), false
		}
		if key := runway.CredentialModeErrorKey(err); key != "" {
			return l.Text(key), false
		}
		msg := accountFailure(l, l.Text(l10n.KeyAccountsSwitchFailed), err)
		if status.State != "" {
			msg += "\n" + codexRuntimeMessage(l, status)
		}
		return msg, false
	}
	return codexRuntimeMessage(l, status), true
}
