package runway

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/codexruntime"
)

var ErrCodexRuntime = errors.New("Codex runtime operation failed before file update")

// CodexRuntimeView contains no server text or account identifiers for display.
// State names are localized by the UI, and survive ordinary quota refreshes.
type CodexRuntimeView struct {
	State         string
	FilesUpdated  bool
	CheckedAt     time.Time
	FailureStage  string
	FailureReason string
	// ProjectShare is the outcome of carrying the desktop app's projects
	// over during the last switch ("projects_*"), empty when not attempted.
	ProjectShare string
	target       string
}

func runtimeTarget(a codex.ManagedAccount) codexruntime.Identity {
	return codexruntime.Identity{AccountID: a.AccountID, UserID: a.IdentityMarkerUserID(), Email: a.Email, API: a.AuthMode == codex.AuthModeAPIKey}
}
func runtimeTargetKey(a codex.ManagedAccount) string {
	return a.ID + "\x00" + a.AccountID + "\x00" + a.Email + "\x00" + a.UserID + "\x00" + a.APIBaseURL + "\x00" + a.APIModel
}
func (s *Service) setRuntimeState(state string, updated bool, target string) {
	s.mu.Lock()
	s.runtimeStatus = CodexRuntimeView{State: state, FilesUpdated: updated, CheckedAt: s.now(), target: target}
	s.publishLocked()
	s.mu.Unlock()
}
func runtimeState(snapshot codexruntime.Snapshot, target codexruntime.Identity, err error) string {
	if err != nil {
		return "inspect_failed"
	}
	if snapshot.Status == codexruntime.NotRunning {
		return "not_running"
	}
	return string(snapshot.Compare(target))
}

// switchCodexRuntime runs under both the switch flag and credential-operation lock.
func (s *Service) switchCodexRuntime(ctx context.Context, id string, restart bool) (codex.SwitchResult, error) {
	commit := func() (codex.SwitchResult, error) { return s.src.SwitchAccount(ctx, id) }
	if restart {
		if s.src.PrepareSwitch == nil {
			s.setRuntimeState("preflight_failed", false, "")
			return codex.SwitchResult{}, ErrCodexRuntime
		}
		prepared, err := s.src.PrepareSwitch(ctx, id)
		if err != nil {
			s.setRuntimeFailure("credential_failed", false, "", err)
			return codex.SwitchResult{}, err
		}
		commit = prepared
	}
	var source codexdesktop.Owner
	share := ""
	enabled := s.src.PrepareDesktopProjects != nil && s.Preferences().ShareCodexDesktopProjects
	if enabled {
		var err error
		source, err = s.desktopIdentity()
		if err != nil {
			share = "projects_unsupported"
			enabled = false
		}
	}
	stopped := stoppedCodex{}
	if restart {
		var err error
		stopped, err = s.stopCodex(ctx)
		if err != nil {
			s.setRuntimeFailure(stopped.failure, false, "", err)
			return codex.SwitchResult{}, ErrCodexRuntime
		}
	}
	if enabled {
		current, err := s.desktopIdentity()
		if err != nil || current != source {
			_, recoveryErr := stopped.restore(s, ctx)
			if recoveryErr != nil {
				s.setRuntimeFailure("recovery_failed", false, "", recoveryErr)
			} else {
				s.setRuntimeState("projects_prepare_failed", false, "")
			}
			return codex.SwitchResult{}, errors.Join(ErrDesktopProjects, ErrDesktopProjectTargetChanged, recoveryErr)
		}
	}
	var apply func() (codexdesktop.Outcome, error)
	if enabled {
		s.mu.Lock()
		account, ok := s.index.Account(id)
		s.mu.Unlock()
		target := codexdesktop.Owner{UserID: account.UserID, AccountID: account.AccountID}
		if !ok || account.AuthMode == codex.AuthModeAPIKey || target.UserID == "" || target.AccountID == "" {
			share = "projects_unsupported"
		} else {
			var err error
			apply, err = s.src.PrepareDesktopProjects(source, target)
			if err == nil && apply == nil {
				err = ErrDesktopProjects
			}
			if errors.Is(err, codexdesktop.ErrDesktopRunning) && !restart {
				share = "projects_running"
				apply = nil
			} else if err != nil {
				_, recoveryErr := stopped.restore(s, ctx)
				if recoveryErr != nil {
					s.setRuntimeFailure("recovery_failed", false, "", recoveryErr)
				} else {
					s.setRuntimeState("projects_prepare_failed", false, "")
				}
				s.setProjectShare(projectShareState("", err))
				return codex.SwitchResult{}, errors.Join(ErrDesktopProjects, recoveryErr)
			}
		}
	}
	if enabled {
		current, err := s.desktopIdentity()
		if err != nil || current != source {
			_, recoveryErr := stopped.restore(s, ctx)
			if recoveryErr != nil {
				s.setRuntimeFailure("recovery_failed", false, "", recoveryErr)
			} else {
				s.setRuntimeState("projects_prepare_failed", false, "")
			}
			return codex.SwitchResult{}, errors.Join(ErrDesktopProjects, ErrDesktopProjectTargetChanged, recoveryErr)
		}
	}
	result, err := commit()
	if err != nil {
		state, recoveryErr := stopped.restore(s, ctx)
		if errors.Is(err, codex.ErrRollbackFailed) {
			state = "rollback_failed"
		}
		s.setRuntimeFailure(state, false, "", recoveryErr)
		return result, err
	}
	var shareErr error
	if apply != nil {
		outcome, err := apply()
		share, shareErr = projectShareState(outcome, err), err
	}
	stopped.start(s, ctx, result.Account)
	s.setProjectShare(share)
	if shareErr != nil {
		return result, ErrDesktopProjects
	}
	return result, nil
}

func (s *Service) setProjectShare(state string) {
	if state == "" {
		return
	}
	s.mu.Lock()
	s.runtimeStatus.ProjectShare = state
	s.publishLocked()
	s.mu.Unlock()
}

// CheckCodexRuntime only inspects the existing daemon, never starts or stops it.
func (s *Service) CheckCodexRuntime(ctx context.Context) error {
	s.mu.Lock()
	if s.switching {
		s.mu.Unlock()
		return ErrSwitchBusy
	}
	epoch := s.epoch
	account, ok := s.index.Account(s.index.ActiveAccountID)
	s.mu.Unlock()
	if !ok {
		return ErrCodexRuntime
	}
	s.codexAuthMu.Lock()
	defer s.codexAuthMu.Unlock()
	s.mu.Lock()
	if s.switching || epoch != s.epoch {
		s.mu.Unlock()
		return ErrSwitchBusy
	}
	s.mu.Unlock()
	target := runtimeTarget(account)
	if s.src.RuntimeIdentity != nil {
		official, err := s.src.RuntimeIdentity()
		if err != nil || !sameRuntimeIdentity(official, target) {
			s.setRuntimeState("unverified", false, "")
			return nil
		}
		target = official
	}
	snapshot, err := codexruntime.Snapshot{}, error(nil)
	if s.src.RuntimeInspect != nil {
		snapshot, err = s.src.RuntimeInspect(ctx)
	}
	if s.src.RuntimeIdentity != nil {
		after, identityErr := s.src.RuntimeIdentity()
		if identityErr != nil || !sameRuntimeIdentity(after, target) {
			s.setRuntimeState("unverified", false, "")
			return nil
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _ := s.index.Account(s.index.ActiveAccountID)
	if s.switching || epoch != s.epoch || runtimeTargetKey(account) != runtimeTargetKey(current) {
		return ErrSwitchBusy
	}
	stage, reason := runtimeFailureCodes(err)
	s.runtimeStatus = CodexRuntimeView{State: runtimeState(snapshot, target, err), CheckedAt: s.now(), target: runtimeTargetKey(account), FailureStage: stage, FailureReason: reason}
	s.publishLocked()
	return nil
}

// InvalidateCodexRuntime drops an earlier verification after applying edited credentials.
func (s *Service) InvalidateCodexRuntime() {
	s.setRuntimeState("unverified", false, "")
}

func sameRuntimeIdentity(a, b codexruntime.Identity) bool {
	return a.AccountID == b.AccountID && a.Email == b.Email && a.API == b.API && (a.UserID == "" || b.UserID == "" || a.UserID == b.UserID)
}

func (s *Service) setRuntimeFailure(state string, updated bool, target string, err error) {
	stage, reason := runtimeFailureCodes(err)
	s.mu.Lock()
	s.runtimeStatus = CodexRuntimeView{State: state, FilesUpdated: updated, CheckedAt: s.now(), target: target, FailureStage: stage, FailureReason: reason}
	s.publishLocked()
	s.mu.Unlock()
}

// Only fixed categories cross into the UI; unknown server/subprocess text is discarded.
func runtimeFailureCodes(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	var failure *codexruntime.Error
	if !errors.As(err, &failure) {
		return "", "unknown"
	}
	stage := "command"
	switch failure.Stage {
	case "discovery", "version":
		stage = "read_settings"
	case "connection", "handshake", "protocol":
		stage = "connect"
	case "account":
		stage = "read_identity"
	}
	reason := "unknown"
	switch failure.Reason {
	case "timeout_or_cancelled":
		reason = "timeout"
	case "invalid_home", "untrusted_home", "no_trusted_binary", "untrusted_socket":
		reason = "permission"
	case "socket_unavailable", "unavailable", "closed", "read_failed", "write_failed":
		reason = "connect"
	case "command_failed":
		reason = "command"
	case "state_changed", "daemon_changed":
		reason = "changed"
	case "state_not_confirmed":
		reason = "verify"
	case "unsupported_endpoint", "unsupported_identity":
		reason = "unsupported_version"
	case "malformed_response", "unknown_status", "invalid_response", "invalid_frame", "invalid_text", "unexpected_response", "notification_limit":
		reason = "invalid_data"
	}
	return stage, reason
}

func (s *Service) recordRuntimeSnapshot(snapshot codexruntime.Snapshot, account codex.ManagedAccount, err error) {
	if s.src.RuntimeIdentity != nil {
		official, identityErr := s.src.RuntimeIdentity()
		if identityErr != nil || !sameRuntimeIdentity(official, runtimeTarget(account)) {
			s.setRuntimeFailure("unverified", true, runtimeTargetKey(account), identityErr)
			return
		}
	}
	s.setRuntimeFailure(runtimeState(snapshot, runtimeTarget(account), err), true, runtimeTargetKey(account), err)
}
