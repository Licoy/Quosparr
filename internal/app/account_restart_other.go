//go:build !darwin

package app

import "runtime"

const codexRestartSupported = runtime.GOOS != "windows"
const codexRestartDefault = false

func prepareCodexDesktopRestart() (func() error, error) { return nil, nil }

func restartCodexDesktop() error { return errRestartCodex }
