package codex

import (
	"errors"
	"fmt"
)

// Sentinel errors. None of them ever carries credential text.
var (
	// ErrInvalidCredential: a credential file or pasted value is not a usable
	// Codex auth JSON.
	ErrInvalidCredential = errors.New("codex: invalid credential")
	// ErrExpiredAccessWithoutRefresh: the access token is expired and there is
	// no refresh token to renew it.
	ErrExpiredAccessWithoutRefresh = errors.New("codex: access token expired and no refresh token")
	// ErrNotUsableAsCodexLogin: the tokens are placeholders or truncated and
	// must not be installed as the official login.
	ErrNotUsableAsCodexLogin = errors.New("codex: credential is not usable as a Codex login")
	// ErrReauthRequired: the server refused the credential (revoked or
	// refused refresh). Verification hooks wrap or return it so the account
	// is marked for re-authorization.
	ErrReauthRequired = errors.New("codex: re-authorization required")
	// ErrRefreshUnavailable: a refresh was needed but no refresher is wired.
	ErrRefreshUnavailable = errors.New("codex: token refresh unavailable")
	// ErrRollbackFailed: a failed operation could not be fully undone.
	ErrRollbackFailed = errors.New("codex: restoring the previous state failed")
)

// InvalidIndexError identifies an unreadable account-index format while
// preserving the parser cause without exposing its text.
type InvalidIndexError struct{ Cause error }

func (e *InvalidIndexError) Error() string { return "codex: account index is invalid or corrupted" }
func (e *InvalidIndexError) Unwrap() error { return e.Cause }

// AccountNotFoundError reports an id that is not in the account index.
type AccountNotFoundError struct{ ID string }

func (e *AccountNotFoundError) Error() string {
	return fmt.Sprintf("codex: account %q not found", e.ID)
}

// CredentialMissingError reports an indexed account without a credential file.
type CredentialMissingError struct{ ID string }

func (e *CredentialMissingError) Error() string {
	return fmt.Sprintf("codex: credential for account %q is missing", e.ID)
}
