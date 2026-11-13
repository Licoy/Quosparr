package app

import (
	"errors"
	"fmt"
	"strings"
)

var errRestartGemini = errors.New("Antigravity restart failed")

var errGeminiQuit = fmt.Errorf("%w: quit timed out", errRestartGemini)

// geminiAppRestart quits and reopens the Antigravity desktop app.
var geminiAppRestart = appRestart{match: isAntigravityApp, failed: errRestartGemini, quit: errGeminiQuit}

func isAntigravityApp(bundleID, _ string) bool {
	return strings.EqualFold(bundleID, "com.google.antigravity")
}
