package claudedesktop

import (
	"context"
	"errors"
	"github.com/licoy/quosparr/internal/claude"
)

// BeginBinding stages the target's whole local Desktop directory. It never
// changes CLI credentials and does not assert that Desktop is logged in.
func (m *Manager) BeginBinding(ctx context.Context, id, selected string) (p Profile, err error) {
	err = m.run(ctx, func(ctx context.Context) error {
		s, j, e := m.prepare(ctx, profileRequest{id: id, selected: selected, kind: "binding"})
		if e != nil {
			return e
		}
		if e = m.activate(j); e == nil {
			j.Phase = "awaiting-confirmation"
			e = m.save("journal.json", j)
		}
		if e == nil {
			e = m.host.Start(ctx, LaunchRequest{Application: j.Application})
			if e != nil {
				e = errors.Join(ErrLaunch, e)
			}
		}
		if e != nil {
			_, e = m.rollback(ctx, s, j, e)
			return e
		}
		p = s.Profiles[id]
		p.Pending = true
		p.RecoveryRequired = true
		return nil
	})
	return
}

func (m *Manager) finishBinding(ctx context.Context, id string, confirm bool) (p Profile, err error) {
	err = m.run(ctx, func(ctx context.Context) error {
		var identity claude.CredentialIdentity
		if confirm {
			a, e := m.lookup(ctx, id)
			if e != nil {
				return e
			}
			identity = a.Identity
		}
		s, e := m.loadState()
		if e != nil {
			return e
		}
		j, e := m.loadJournal()
		if e != nil {
			return e
		}
		if j == nil || j.Kind != "binding" || j.Target != id {
			return ErrRecoveryRequired
		}
		// Confirmation is a durable user assertion about exactly this library
		// account and organization, not an automated Desktop identity check.
		j.Confirm = confirm
		if e = m.save("journal.json", j); e != nil {
			return e
		}
		if _, e = m.restore(ctx, s, j); e != nil {
			return e
		}
		s, e = m.loadState()
		if e != nil {
			return e
		}
		p = s.Profiles[id]
		p.Identity = identity
		return nil
	})
	return
}
func (m *Manager) ConfirmBinding(ctx context.Context, id string) (Profile, error) {
	return m.finishBinding(ctx, id, true)
}
func (m *Manager) CancelBinding(ctx context.Context, id string) error {
	_, err := m.finishBinding(ctx, id, false)
	return err
}
