package claudedesktop

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

type desktopChild interface{ Wait() error }
type desktopChildStarter func(string, []string) (desktopChild, error)

func startDesktopChild(executable string, env []string) (desktopChild, error) {
	cmd := exec.Command(executable)
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return nil, errors.New("Claude Desktop could not be started")
	}
	return cmd, nil
}

func launchHostProcess(ctx context.Context, executable string, env []string) error {
	return observeDesktopLaunch(ctx, executable, env, startDesktopChild, time.Second)
}

// This only establishes that the ordinary process survived initial startup,
// never that a particular account is logged in or future execution will succeed.
func observeDesktopLaunch(ctx context.Context, exe string, env []string, start desktopChildStarter, interval time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	child, err := start(exe, env)
	if err != nil {
		return err
	}
	exited := make(chan error, 1)
	// The buffered result also reaps a later ordinary exit after observation ends.
	go func() { exited <- child.Wait() }()
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case err := <-exited:
		if err != nil {
			return errors.New("Claude Desktop failed during startup")
		}
		return errors.New("Claude Desktop exited before startup could be confirmed")
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		select {
		case <-exited:
			return errors.New("Claude Desktop exited before startup could be confirmed")
		default:
			return nil
		}
	}
}
