package claudedesktop

import (
	"context"
	"errors"

	"github.com/licoy/quosparr/internal/platform"
)

func (m *Manager) preflight(ctx context.Context, req profileRequest) (desktopState, *desktopJournal, error) {
	s, err := m.loadState()
	if err != nil {
		return s, nil, err
	}
	j, err := m.loadJournal()
	if err != nil {
		return s, nil, err
	}
	if j != nil {
		return s, nil, ErrRecoveryRequired
	}
	a, err := m.lookup(ctx, req.id)
	if err != nil {
		return s, nil, err
	}
	if req.kind != "binding" {
		if err = m.checkBound(s, req.id); err != nil {
			return s, nil, err
		}
	}
	app, err := m.resolveProfileApplication(ctx, s, req)
	if err != nil {
		return s, nil, err
	}
	native, err := optionalDirectory(app.DataDir)
	if err != nil {
		return s, nil, err
	}
	// Once tracked, an externally replaced official directory cannot silently
	// become the profile associated with the previous account.
	if s.Application.DataDir != "" && native != s.Native {
		return s, nil, ErrProfileChanged
	}
	target, err := m.targetDirectory(s, req.id, native)
	if err != nil {
		return s, nil, err
	}
	if s.Active != req.id {
		existing, e := optionalDirectory(m.slot(s.Active))
		if e != nil || !existing.empty() {
			return s, nil, errors.Join(ErrProfileChanged, e)
		}
	}
	s.Application = app
	s.Native = native
	p := s.Profiles[req.id]
	p.AccountID = req.id
	p.Identity = a.Identity
	p.ApplicationPath = app.Path
	p.Directory = target
	s.Profiles[req.id] = p
	j = &desktopJournal{Version: 1, Kind: req.kind, Application: app, Previous: s.Active, Target: req.id, PreviousIdentity: native, TargetIdentity: target, Phase: "prepared", Same: s.Active == req.id}
	return s, j, nil
}

func (m *Manager) resolveProfileApplication(ctx context.Context, s desktopState, req profileRequest) (Application, error) {
	selected := req.selected
	if selected == "" {
		selected = s.Profiles[req.id].ApplicationPath
	}
	if selected == "" {
		selected = s.Application.Path
	}
	app, err := m.host.Resolve(ctx, selected)
	if err != nil {
		return app, err
	}
	if err = m.validateApplication(app); err != nil {
		return app, err
	}
	if s.Application.DataDir != "" && s.Application.DataDir != app.DataDir {
		return app, ErrUnsafePath
	}
	return app, nil
}

func (m *Manager) targetDirectory(s desktopState, id string, native directoryIdentity) (directoryIdentity, error) {
	if s.Active == id {
		return native, nil
	}
	target, err := optionalDirectory(m.slot(id))
	if err != nil {
		return target, err
	}
	old := s.Profiles[id]
	if target.empty() {
		if old.IsBound() || !old.Directory.empty() {
			return target, ErrProfileChanged
		}
		if err = platform.EnsurePrivateDirectory(m.slot(id)); err != nil {
			return target, err
		}
		return directoryID(m.slot(id))
	}
	if !old.Directory.empty() && old.Directory != target {
		return target, ErrProfileChanged
	}
	return target, nil
}
