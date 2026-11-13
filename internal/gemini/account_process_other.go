//go:build !windows

package gemini

import (
	"bytes"
	"context"
	"os"
	"os/exec"
)

// psPaths are the absolute ps locations tried in order.
var psPaths = []string{"/bin/ps", "/usr/bin/ps"}

// SystemProcessIDs lists running Antigravity and agy processes, excluding
// this process.
func SystemProcessIDs(ctx context.Context) ([]int, error) {
	found, err := systemProcesses(ctx)
	return processIDs(found), err
}

func systemProcesses(ctx context.Context) ([]antigravityProcess, error) {
	var lastErr error
	for _, ps := range psPaths {
		if _, err := os.Stat(ps); err != nil {
			lastErr = err
			continue
		}
		cmd := exec.CommandContext(ctx, ps, "-axo", "pid=,comm=")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return nil, err
		}
		return parseProcesses(out.String(), os.Getpid()), nil
	}
	return nil, lastErr
}
