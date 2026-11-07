//go:build !darwin

package app

import "runtime"

const codexRestartSupported = runtime.GOOS != "windows"
const codexRestartDefault = false
const codexDesktopShareSupported = false

// codexDesktopRunning is nil: the desktop app cannot be detected here, so
// its state is never changed.
var codexDesktopRunning func() bool

func prepareCodexDesktopRestart() (func() error, error) { return nil, nil }

func restartCodexDesktop() error { return errRestartCodex }
