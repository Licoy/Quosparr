//go:build darwin

package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/ebitengine/purego/objc"
)

const codexRestartSupported = true
const codexRestartDefault = true

func accountOnMain(fn func()) {
	done := make(chan struct{})
	runOnMain(func() { defer close(done); fn() })
	<-done
}

func prepareCodexDesktopRestart() (func() error, error) {
	apps := runningApplications(isCodexDesktop)
	fallback := findCodexApplication()
	if len(apps) == 0 && fallback == "" {
		return nil, nil
	}
	return prepareCodexRestart(apps, fallback, openCodexApplication)
}

func restartCodexDesktop() error {
	return runCodexRestart(runningApplications(isCodexDesktop), findCodexApplication(), openCodexApplication)
}

// runningApplications returns the running applications that match; the
// caller must call release on each.
func runningApplications(match func(bundleID, name string) bool) []runningApp {
	var apps []runningApp
	accountOnMain(func() {
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
		defer pool.Send(objc.RegisterName("drain"))
		workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
		running := workspace.Send(objc.RegisterName("runningApplications"))
		for i := uint64(0); i < objc.Send[uint64](running, objc.RegisterName("count")); i++ {
			app := running.Send(objc.RegisterName("objectAtIndex:"), i)
			bundle, name := accountNSString(app.Send(objc.RegisterName("bundleIdentifier"))), accountNSString(app.Send(objc.RegisterName("localizedName")))
			if !match(bundle, name) {
				continue
			}
			app.Send(objc.RegisterName("retain"))
			url := app.Send(objc.RegisterName("bundleURL"))
			item := runningApp{bundleID: bundle, name: name, path: accountNSString(url.Send(objc.RegisterName("path")))}
			item.terminate = func() bool {
				var ok bool
				accountOnMain(func() {
					ok = objc.Send[bool](app, objc.RegisterName("terminate"))
				})
				return ok
			}
			item.stopped = func() bool {
				var stopped bool
				accountOnMain(func() { stopped = objc.Send[bool](app, objc.RegisterName("isTerminated")) })
				return stopped
			}
			item.release = func() { accountOnMain(func() { app.Send(objc.RegisterName("release")) }) }
			apps = append(apps, item)
		}
	})
	return apps
}

func openCodexApplication(path string) error {
	result := make(chan bool, 1)
	accountOnMain(func() {
		pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
		defer pool.Send(objc.RegisterName("drain"))
		url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), nsStr(path))
		workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
		configuration := objc.ID(objc.GetClass("NSWorkspaceOpenConfiguration")).Send(objc.RegisterName("configuration"))
		configuration.Send(objc.RegisterName("setActivates:"), true)
		block := objc.NewBlock(func(_ objc.Block, application, err objc.ID) { result <- err == 0 && application != 0 })
		workspace.Send(objc.RegisterName("openApplicationAtURL:configuration:completionHandler:"), url, configuration, block)
		block.Release()
	})
	select {
	case ok := <-result:
		if !ok {
			return errRestartCodex
		}
	case <-time.After(10 * time.Second):
		return errRestartCodex
	}
	return nil
}

func accountNSString(value objc.ID) string {
	if value == 0 {
		return ""
	}
	return objc.Send[string](value, objc.RegisterName("UTF8String"))
}

func findCodexApplication() string {
	paths := []string{"/Applications/Codex.app"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, "Applications", "Codex.app"))
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			valid := false
			accountOnMain(func() {
				pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
				defer pool.Send(objc.RegisterName("drain"))
				bundle := objc.ID(objc.GetClass("NSBundle")).Send(objc.RegisterName("bundleWithPath:"), nsStr(path))
				valid = isCodexDesktop(accountNSString(bundle.Send(objc.RegisterName("bundleIdentifier"))), "")
			})
			if valid {
				return path
			}
		}
	}
	return ""
}
