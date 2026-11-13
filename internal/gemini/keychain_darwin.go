//go:build darwin

package gemini

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
)

// SystemKeychain is the official item in the login keychain.
func SystemKeychain() Keychain {
	return SecurityKeychain{Service: KeychainService, Account: KeychainAccount}
}

func newSecurityRunner() CommandRunner { return execRunner{} }

// execRunner starts /usr/bin/security. The secret travels only through
// stdin and stdout; stderr is discarded. A cancelled context kills it.
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
