package claudedesktop

import (
	"context"
	"errors"
)

type profileRequest struct{ id, selected, kind string }

func (m *Manager) prepare(ctx context.Context, req profileRequest) (desktopState, *desktopJournal, error) {
	s, j, err := m.preflight(ctx, req)
	if err != nil {
		return s, nil, err
	}
	running, err := m.host.Stop(ctx, j.Application)
	if err != nil {
		return s, nil, errors.Join(ErrStop, err)
	}
	stopped, err := optionalDirectory(j.Application.DataDir)
	if err != nil || stopped != j.PreviousIdentity {
		return s, nil, m.restartUnmoved(ctx, j.Application, running, errors.Join(ErrProfileChanged, err))
	}
	j.WasRunning = running
	if err = m.save("profiles.json", s); err != nil {
		return s, nil, m.restartUnmoved(ctx, j.Application, running, err)
	}
	if err = m.save("journal.json", j); err != nil {
		return s, nil, m.restartUnmoved(ctx, j.Application, running, err)
	}
	return s, j, nil
}

func (m *Manager) restartUnmoved(ctx context.Context, app Application, running bool, cause error) error {
	if !running {
		return cause
	}
	if err := m.host.Start(ctx, LaunchRequest{Application: app}); err != nil {
		return errors.Join(cause, ErrLaunch, err)
	}
	return cause
}

func (m *Manager) activate(j *desktopJournal) error {
	if j.Same {
		return nil
	}
	if !j.PreviousIdentity.empty() {
		if err := m.move(j.Application.DataDir, m.slot(j.Previous), j.PreviousIdentity); err != nil {
			return err
		}
	}
	if err := m.move(m.slot(j.Target), j.Application.DataDir, j.TargetIdentity); err != nil {
		return err
	}
	j.Phase = "activated"
	return m.save("journal.json", j)
}

// checkJournal re-resolves the installation before trusting any stored native path.
func (m *Manager) checkJournal(ctx context.Context, j *desktopJournal) error {
	app, err := m.host.Resolve(ctx, j.Application.Path)
	if err != nil {
		return err
	}
	if app.DataDir != j.Application.DataDir || app.Executable != j.Application.Executable {
		return ErrProfileChanged
	}
	if err = m.validateApplication(app); err != nil {
		return err
	}
	for _, id := range []string{j.Previous, j.Target} {
		if err = checkAncestors(m.slot(id)); err != nil {
			return err
		}
	}
	return nil
}
