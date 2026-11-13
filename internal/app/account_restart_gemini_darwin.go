//go:build darwin

package app

// geminiRestartSupported: the desktop app can be quit and reopened here.
const geminiRestartSupported = true

// antigravityRunning tells whether the Antigravity desktop app is open.
func antigravityRunning() bool {
	apps := runningApplications(isAntigravityApp)
	for _, app := range apps {
		app.release()
	}
	return len(apps) > 0
}

// prepareGeminiAppRestart quits a running Antigravity app and returns the
// call that reopens it; both are nil when the app is not running.
func prepareGeminiAppRestart() (func() error, error) {
	apps := runningApplications(isAntigravityApp)
	if len(apps) == 0 {
		return nil, nil
	}
	return prepareAppRestart(geminiAppRestart, apps, "", antigravityRunning, openCodexApplication)
}
