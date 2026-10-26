package claudedesktop

import (
	"context"
	"errors"
	"time"
)

// restoreDirectories reconciles only the three recorded directory identities.
// A journal is persisted before the first rename, so interrupted moves can be
// identified without traversing contents or guessing from directory names.
func (m *Manager) restoreDirectories(j *desktopJournal) error {
	native, err := optionalDirectory(j.Application.DataDir)
	if err != nil {
		return err
	}
	if j.Same {
		if native != j.TargetIdentity {
			return ErrProfileChanged
		}
		return nil
	}
	target, err := optionalDirectory(m.slot(j.Target))
	if err != nil {
		return err
	}
	previous, err := optionalDirectory(m.slot(j.Previous))
	if err != nil {
		return err
	}
	switch {
	case native == j.TargetIdentity:
		if !target.empty() {
			return ErrProfileChanged
		}
		if err = m.move(j.Application.DataDir, m.slot(j.Target), j.TargetIdentity); err != nil {
			return err
		}
		native = directoryIdentity{}
	case !native.empty() && native != j.PreviousIdentity:
		return ErrProfileChanged
	case target != j.TargetIdentity:
		return ErrProfileChanged
	}
	if native == j.PreviousIdentity && !native.empty() {
		if !previous.empty() {
			return ErrProfileChanged
		}
		return nil
	}
	if !native.empty() {
		return ErrProfileChanged
	}
	if j.PreviousIdentity.empty() {
		if !previous.empty() {
			return ErrProfileChanged
		}
		return nil
	}
	if previous != j.PreviousIdentity {
		return ErrProfileChanged
	}
	return m.move(m.slot(j.Previous), j.Application.DataDir, j.PreviousIdentity)
}

func (m *Manager) restore(ctx context.Context, s desktopState, j *desktopJournal) (SwitchResult, error) {
	result := SwitchResult{ManualCLIRecoveryRequired: j.CLIWasApplied, ManualReopenRequired: j.WasRunning}
	if err := m.checkJournal(ctx, j); err != nil {
		return result, errors.Join(ErrRollback, err)
	}
	if _, err := m.host.Stop(ctx, j.Application); err != nil {
		return result, errors.Join(ErrRollback, ErrStop, err)
	}
	if err := m.restoreDirectories(j); err != nil {
		return result, errors.Join(ErrRollback, err)
	}
	j.Phase = "restored"
	if err := m.save("journal.json", j); err != nil {
		return result, errors.Join(ErrRollback, err)
	}
	s.Active = j.Previous
	s.Native = j.PreviousIdentity
	s.Application = j.Application
	if p, ok := s.Profiles[j.Target]; ok {
		p.Directory = j.TargetIdentity
		s.Profiles[j.Target] = p
	}
	if j.Confirm {
		p := s.Profiles[j.Target]
		now := time.Now()
		p.ConfirmedAt = &now
		p.Confirmation = "manual"
		s.Profiles[j.Target] = p
	}
	if j.WasRunning {
		if err := m.host.Start(ctx, LaunchRequest{Application: j.Application}); err != nil {
			return result, errors.Join(ErrRollback, ErrLaunch, err)
		}
		result.PreviousDesktopRestored = true
		result.ManualReopenRequired = false
	}
	if err := m.save("profiles.json", s); err != nil {
		return result, errors.Join(ErrRollback, err)
	}
	if err := m.clearJournal(); err != nil {
		return result, errors.Join(ErrRollback, err)
	}
	result.ManualCLIRecoveryRequired = j.CLIWasApplied
	return result, nil
}

func (m *Manager) rollback(ctx context.Context, s desktopState, j *desktopJournal, cause error) (SwitchResult, error) {
	clean, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	r, err := m.restore(clean, s, j)
	// The live CLI transaction owns its own snapshot rollback; only recovery
	// after a process restart requires manual CLI reconciliation.
	r.ManualCLIRecoveryRequired = false
	return r, errors.Join(cause, err)
}

// Recover explicitly restores the recorded previous Desktop directory. After
// an interrupted CLI switch, credentials are not reconstructed from metadata.
func (m *Manager) Recover(ctx context.Context) (r SwitchResult, err error) {
	err = m.run(ctx, func(ctx context.Context) error {
		s, e := m.loadState()
		if e != nil {
			return e
		}
		j, e := m.loadJournal()
		if e != nil {
			return e
		}
		if j == nil {
			return nil
		}
		r, e = m.restore(ctx, s, j)
		return e
	})
	return
}
