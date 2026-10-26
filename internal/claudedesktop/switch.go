package claudedesktop

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/claude"
)

// Switch stages a confirmed Desktop directory, then lets the CLI's own
// transaction install credentials and invoke after. The callback must be
// invoked synchronously exactly once after CLI installation.
func (m *Manager) Switch(ctx context.Context, id string, apply func(after func() error) error) (r SwitchResult, err error) {
	if apply == nil {
		return r, errors.New("claude desktop: CLI switch callback required")
	}
	err = m.run(ctx, func(ctx context.Context) error {
		var e error
		r, e = m.switchProfile(ctx, id, apply, true)
		return e
	})
	return
}

// Open changes Desktop only, including when the CLI login is expired or absent.
func (m *Manager) Open(ctx context.Context, id string) (r SwitchResult, err error) {
	err = m.run(ctx, func(ctx context.Context) error {
		var e error
		r, e = m.switchProfile(ctx, id, func(after func() error) error { return after() }, false)
		return e
	})
	return
}

func (m *Manager) switchProfile(ctx context.Context, id string, apply func(func() error) error, cli bool) (r SwitchResult, err error) {
	kind := "open"
	if cli {
		kind = "switch"
	}
	s, j, err := m.prepare(ctx, profileRequest{id: id, kind: kind})
	if err != nil {
		return r, err
	}
	if err = m.activate(j); err != nil {
		return m.rollback(ctx, s, j, err)
	}
	j.CLIWasApplied = cli
	j.Phase = "calling-cli"
	if err = m.save("journal.json", j); err != nil {
		return m.rollback(ctx, s, j, err)
	}
	called, cleanupFailed := false, false
	after := func() (e error) {
		if called {
			return errors.New("claude desktop: CLI callback invoked more than once")
		}
		called = true
		defer func() {
			if e != nil {
				clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
				defer cancel()
				// Stop the target before the CLI transaction rolls back, but keep both
				// directory restoration and restarting previous Desktop after CLI rollback.
				if _, stopErr := m.host.Stop(clean, j.Application); stopErr != nil {
					cleanupFailed = true
					e = errors.Join(e, ErrRollback, ErrStop, stopErr)
				}
			}
		}()
		j.CLIWasApplied = cli
		j.Phase = "starting"
		if e = m.save("journal.json", j); e != nil {
			return e
		}
		if e = m.host.Start(ctx, LaunchRequest{Application: j.Application}); e != nil {
			return errors.Join(ErrLaunch, e)
		}
		native, checkErr := directoryID(j.Application.DataDir)
		if checkErr != nil || native != j.TargetIdentity {
			return errors.Join(ErrProfileChanged, checkErr)
		}
		next := s
		next.Active = id
		next.Native = j.TargetIdentity
		if e = m.save("profiles.json", next); e != nil {
			return e
		}
		if e = m.clearJournal(); e != nil {
			return e
		}
		r = SwitchResult{CLISwitched: cli, DesktopStarted: true, NeedsLoginConfirmation: true}
		return nil
	}
	err = apply(after)
	if err == nil && !called {
		err = errors.New("claude desktop: CLI transaction did not invoke callback")
	}
	if err == nil {
		return r, nil
	}
	if errors.Is(err, claude.ErrSwitchRollback) {
		r = SwitchResult{ManualCLIRecoveryRequired: true, ManualReopenRequired: j.WasRunning}
		// Keep the stopped target and its move journal until the user has checked
		// CLI credentials; restarting previous Desktop with unknown CLI state is unsafe.
		return r, errors.Join(err, ErrRollback, m.save("journal.json", j))
	}
	if !cleanupFailed {
		// A nonconforming caller may return an error after a successful callback;
		// recreate the move intent before reversing any directory movement.
		if e := m.save("journal.json", j); e != nil {
			return r, errors.Join(err, ErrRollback, e)
		}
		r, err = m.rollback(ctx, s, j, err)
	} else {
		r.ManualReopenRequired = j.WasRunning
	}

	r.CLISwitched = false
	r.DesktopStarted = false
	return r, err
}
