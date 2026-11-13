//go:build !darwin

package app

// geminiRestartSupported is false: the user restarts Antigravity themselves.
const geminiRestartSupported = false

func prepareGeminiAppRestart() (func() error, error) { return nil, nil }
