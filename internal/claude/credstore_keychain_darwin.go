//go:build darwin

package claude

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
)

func newSecurityRunner() CommandRunner { return execRunner{} }

// execRunner starts /usr/bin/security. The password travels only through
// stdin. stderr is discarded so a failure log can carry the exit code without
// the secret. A cancelled context kills the process.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err == nil {
		return out, 0, nil
	}
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out, exit.ExitCode(), nil
	}
	return nil, 0, err
}
