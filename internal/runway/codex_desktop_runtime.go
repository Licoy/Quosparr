package runway

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/codexruntime"
)

// stoppedCodex retains only a snapshot obtained after the desktop app quit.
type stoppedCodex struct {
	daemon   *codexruntime.Snapshot
	original codexruntime.Identity
	desktop  func() error
	failure  string
}

func (s *Service) stopCodex(ctx context.Context) (stoppedCodex, error) {
	stopped := stoppedCodex{failure: "preflight_failed"}
	if s.src.RuntimeInspect == nil {
		return stopped, ErrCodexRuntime
	}
	if s.src.LegacyStop != nil {
		var err error
		stopped.desktop, err = s.src.LegacyStop()
		if err != nil {
			stopped.failure = "stop_failed"
			_, recoveryErr := stopped.restore(s, ctx)
			if recoveryErr != nil {
				stopped.failure = "recovery_failed"
			}
			return stopped, errors.Join(err, recoveryErr)
		}
	}
	before, err := s.src.RuntimeInspect(ctx)
	if err != nil || (before.Status != codexruntime.Running && before.Status != codexruntime.NotRunning && !(before.Status == codexruntime.Unsupported && stopped.desktop != nil)) {
		_, recoveryErr := stopped.restore(s, ctx)
		if recoveryErr != nil {
			stopped.failure = "recovery_failed"
		}
		return stopped, errors.Join(ErrCodexRuntime, err, recoveryErr)
	}
	if before.Status != codexruntime.Running {
		return stopped, nil
	}
	if s.src.RuntimeStop == nil || s.src.RuntimeStart == nil {
		_, recoveryErr := stopped.restore(s, ctx)
		if recoveryErr != nil {
			stopped.failure = "recovery_failed"
		}
		return stopped, errors.Join(ErrCodexRuntime, err, recoveryErr)
	}
	after, err := s.src.RuntimeStop(ctx, before)
	if err != nil || after.Status != codexruntime.NotRunning {
		stopped.failure = "stop_failed"
		_, recoveryErr := stopped.restore(s, ctx)
		if recoveryErr != nil {
			stopped.failure = "recovery_failed"
		}
		return stopped, errors.Join(ErrCodexRuntime, err, recoveryErr)
	}
	stopped.daemon, stopped.original = &after, before.Identity
	return stopped, nil
}

func (stopped stoppedCodex) restore(s *Service, ctx context.Context) (string, error) {
	state := "credential_failed"
	recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	var failure error
	if stopped.daemon != nil {
		snapshot, err := s.src.RuntimeStart(recovery, *stopped.daemon)
		switch {
		case err != nil || snapshot.Status != codexruntime.Running:
			state, failure = "recovery_failed", ErrCodexRuntime
		case snapshot.Compare(stopped.original) != codexruntime.Matched:
			state = "write_failed_restarted_unverified"
		default:
			state = "write_failed_recovered"
		}
	}
	if stopped.desktop != nil {
		if err := stopped.desktop(); err != nil {
			state, failure = "recovery_failed", ErrCodexRuntime
		} else if failure == nil && stopped.daemon == nil {
			state = "write_failed_restarted_unverified"
		}
	}
	if stopped.daemon != nil && stopped.desktop != nil && failure == nil {
		snapshot, err := s.src.RuntimeInspect(recovery)
		if err != nil || snapshot.Status != codexruntime.Running {
			state, failure = "recovery_failed", errors.Join(ErrCodexRuntime, err)
		} else if snapshot.Compare(stopped.original) != codexruntime.Matched {
			state = "write_failed_restarted_unverified"
		}
	}
	return state, failure
}

func (stopped stoppedCodex) start(s *Service, ctx context.Context, account codex.ManagedAccount) {
	target := runtimeTargetKey(account)
	var snapshot codexruntime.Snapshot
	var err error
	if stopped.daemon != nil {
		snapshot, err = s.src.RuntimeStart(ctx, *stopped.daemon)
		if err == nil && snapshot.Status != codexruntime.Running {
			err = ErrCodexRuntime
		}
	}
	if stopped.desktop != nil {
		if desktopErr := stopped.desktop(); desktopErr != nil {
			err = desktopErr
		}
	}
	if err != nil {
		s.setRuntimeFailure("start_failed", true, target, err)
		return
	}
	if stopped.desktop != nil && stopped.daemon == nil {
		s.setRuntimeState("legacy_unverified", true, target)
		return
	}
	if stopped.daemon == nil || stopped.desktop != nil {
		if s.src.RuntimeInspect == nil {
			s.setRuntimeState("unverified", true, target)
			return
		}
		snapshot, err = s.src.RuntimeInspect(ctx)
	}
	s.recordRuntimeSnapshot(snapshot, account, err)
}

// DesktopProjectOwner reads the actual official identity while excluding switches.
func (s *Service) DesktopProjectOwner() (codexdesktop.Owner, error) {
	s.mu.Lock()
	busy := s.switching
	s.mu.Unlock()
	if busy {
		return codexdesktop.Owner{}, ErrSwitchBusy
	}
	s.codexAuthMu.Lock()
	defer s.codexAuthMu.Unlock()
	s.mu.Lock()
	busy = s.switching
	s.mu.Unlock()
	if busy {
		return codexdesktop.Owner{}, ErrSwitchBusy
	}
	return s.desktopIdentity()
}

// RestoreDesktopProjects executes an already previewed write without changing credentials.
// The operation shares the switch lock and rechecks the exact user and workspace.
func (s *Service) RestoreDesktopProjects(ctx context.Context, target codexdesktop.Owner, apply func() (codexdesktop.Outcome, error)) (codexdesktop.Outcome, error) {
	if apply == nil || target.UserID == "" || target.AccountID == "" {
		return "", ErrDesktopProjects
	}
	s.mu.Lock()
	if s.switching {
		s.mu.Unlock()
		return "", ErrSwitchBusy
	}
	s.switching = true
	s.publishLocked()
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.switching = false; s.publishLocked(); s.mu.Unlock() }()
	s.codexAuthMu.Lock()
	defer s.codexAuthMu.Unlock()
	owner, err := s.desktopIdentity()
	if err != nil || owner != target {
		return "", ErrDesktopProjectTargetChanged
	}
	stopped, err := s.stopCodex(ctx)
	if err != nil {
		s.setRuntimeFailure(stopped.failure, false, "", err)
		return "", ErrCodexRuntime
	}
	owner, err = s.desktopIdentity()
	if err != nil || owner != target {
		_, recoveryErr := stopped.restore(s, ctx)
		if recoveryErr != nil {
			s.setRuntimeFailure("recovery_failed", false, "", recoveryErr)
		}
		return "", errors.Join(ErrDesktopProjectTargetChanged, recoveryErr)
	}
	outcome, applyErr := apply()
	_, recoveryErr := stopped.restore(s, ctx)
	if recoveryErr == nil && stopped.daemon != nil {
		verifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		snapshot, inspectErr := s.src.RuntimeInspect(verifyCtx)
		cancel()
		if inspectErr != nil || snapshot.Status != codexruntime.Running || snapshot.Identity.API || snapshot.Identity.UserID != target.UserID || snapshot.Identity.AccountID != target.AccountID {
			recoveryErr = ErrDesktopProjectRestart
		}
	}
	owner, identityErr := s.desktopIdentity()
	if identityErr != nil || owner != target {
		recoveryErr = errors.Join(recoveryErr, ErrDesktopProjectRestart)
	}
	if recoveryErr != nil {
		recoveryErr = errors.Join(ErrDesktopProjectRestart, recoveryErr)
		s.setRuntimeFailure("recovery_failed", false, "", recoveryErr)
	}
	if applyErr != nil {
		return outcome, errors.Join(ErrDesktopProjects, applyErr, recoveryErr)
	}
	if recoveryErr != nil {
		return outcome, ErrDesktopProjectRestart
	}
	return outcome, nil
}

var (
	ErrDesktopProjectTargetChanged = errors.New("runway: desktop project target changed")
	ErrDesktopProjectRestart       = errors.New("runway: desktop projects updated but restart failed")
)

var ErrDesktopProjects = errors.New("runway: desktop project operation failed")

func projectShareState(outcome codexdesktop.Outcome, err error) string {
	if errors.Is(err, codexdesktop.ErrDesktopRunning) {
		return "projects_running"
	}
	if errors.Is(err, codexdesktop.ErrPartialWrite) {
		return "projects_partial"
	}
	if err != nil {
		return "projects_failed"
	}
	if outcome == codexdesktop.Unchanged || outcome == "" {
		return ""
	}
	return "projects_" + string(outcome)
}

// skippedProjectShare names a preflight result that skips sharing while the
// switch goes on: an open app without restart, or desktop data the sharing
// cannot handle at all. Empty means the preflight failed.
func skippedProjectShare(err error, restart bool) string {
	switch {
	case errors.Is(err, codexdesktop.ErrDesktopRunning) && !restart:
		return "projects_running"
	case errors.Is(err, codexdesktop.ErrNotPartitioned):
		return "projects_not_partitioned"
	case errors.Is(err, codexdesktop.ErrUnknownFormat), errors.Is(err, codexdesktop.ErrDatabase):
		return "projects_unsupported"
	}
	return ""
}

func (s *Service) desktopIdentity() (codexdesktop.Owner, error) {
	if s.src.RuntimeIdentity == nil {
		return codexdesktop.Owner{}, ErrDesktopProjects
	}
	identity, err := s.src.RuntimeIdentity()
	if err != nil || identity.API || identity.UserID == "" || identity.AccountID == "" {
		return codexdesktop.Owner{}, ErrDesktopProjects
	}
	return codexdesktop.Owner{UserID: identity.UserID, AccountID: identity.AccountID}, nil
}
