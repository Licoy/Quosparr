package grok

import (
	"errors"
	"fmt"
	"strings"
)

// Account errors. None of them carries credential text.
var (
	// ErrCLINotInstalled: the Grok CLI is needed but missing.
	ErrCLINotInstalled = errors.New("grok: the Grok CLI is not installed")
	// ErrOfficialCredentialMissing: ~/.grok/auth.json does not exist.
	ErrOfficialCredentialMissing = errors.New("grok: official credential is missing")
	// ErrOfficialCredentialMalformed: the official file cannot be used.
	ErrOfficialCredentialMalformed = errors.New("grok: official credential is malformed")
	// ErrAPIKeyOnlyNotManageable: the official file holds only an API key.
	ErrAPIKeyOnlyNotManageable = errors.New("grok: an API key alone cannot be managed")
	// ErrNoManagedCredential: the credential has no OAuth or legacy scope.
	ErrNoManagedCredential = errors.New("grok: credential has no manageable login")
	// ErrAuthFileLocked: another process holds the official auth file lock.
	ErrAuthFileLocked = errors.New("grok: the official auth file is locked")
	// ErrLoginCancelled: the OAuth login was cancelled.
	ErrLoginCancelled = errors.New("grok: login cancelled")
	// ErrInvalidCredential: the credential is not a usable Grok auth.json.
	ErrInvalidCredential = errors.New("grok: invalid credential")
	// ErrAuthenticationRequired: the server refused the credential; the user
	// has to sign in again.
	ErrAuthenticationRequired = errors.New("grok: authentication is required")
)

// InvalidIndexError identifies an unreadable account-index format while
// preserving the parser cause without exposing its text.
type InvalidIndexError struct{ Cause error }

func (e *InvalidIndexError) Error() string { return "grok: account index is invalid or corrupted" }
func (e *InvalidIndexError) Unwrap() error { return e.Cause }

// AccountNotFoundError reports an id that is not in the index.
type AccountNotFoundError struct{ ID string }

func (e *AccountNotFoundError) Error() string { return fmt.Sprintf("grok: account %q not found", e.ID) }

// CredentialMissingError reports an indexed account without a credential file.
type CredentialMissingError struct{ ID string }

func (e *CredentialMissingError) Error() string {
	return fmt.Sprintf("grok: credential for account %q is missing", e.ID)
}

// CredentialIdentityMismatchError: a credential belongs to another identity
// than the account it was filed under.
type CredentialIdentityMismatchError struct{ ID string }

func (e *CredentialIdentityMismatchError) Error() string {
	return fmt.Sprintf("grok: credential does not match account %q", e.ID)
}

// CurrentAccountCannotBeRemovedError: the current account cannot be removed.
type CurrentAccountCannotBeRemovedError struct{ ID string }

func (e *CurrentAccountCannotBeRemovedError) Error() string {
	return fmt.Sprintf("grok: account %q is current and cannot be removed", e.ID)
}

// ProcessesRunningError: Grok processes are running, so a switch is refused.
type ProcessesRunningError struct{ PIDs []int }

func (e *ProcessesRunningError) Error() string {
	ids := make([]string, len(e.PIDs))
	for i, pid := range e.PIDs {
		ids[i] = fmt.Sprint(pid)
	}
	return "grok: Grok is running (pid " + strings.Join(ids, ", ") + ")"
}

// UnsupportedIndexVersionError: grok-index.json is of a version this build
// does not read.
type UnsupportedIndexVersionError struct{ Version int }

func (e *UnsupportedIndexVersionError) Error() string {
	return fmt.Sprintf("grok: unsupported account index version %d", e.Version)
}

// LoginFailedError carries a message that is safe to show to the user.
type LoginFailedError struct {
	Message string
	Cause   error
}

func (e *LoginFailedError) Unwrap() error { return e.Cause }

func (e *LoginFailedError) Error() string { return "grok: " + e.Message }

// RefreshFailedError: a refresh of one account failed.
type RefreshFailedError struct {
	AccountID, Message string
	Cause              error
}

func (e *RefreshFailedError) Unwrap() error { return e.Cause }

func (e *RefreshFailedError) Error() string {
	return fmt.Sprintf("grok: refreshing account %q failed: %s", e.AccountID, e.Message)
}

// CLITooOldError: the Grok CLI does not support the requested method.
type CLITooOldError struct{ Version string }

func (e *CLITooOldError) Error() string { return "grok: the Grok CLI is too old (" + e.Version + ")" }

// AccountIOError is a file-system failure with a fixed, secret-free message.
type AccountIOError struct {
	Message string
	Cause   error
}

func (e *AccountIOError) Unwrap() error { return e.Cause }

func (e *AccountIOError) Error() string { return "grok: " + e.Message }

// PartialWriteError: an operation failed and could not be fully undone.
type PartialWriteError struct{ Message string }

func (e *PartialWriteError) Error() string { return "grok: " + e.Message }

// TokenRequestError is a failed token request. Message is short and carries no
// credential text.
type TokenRequestError struct {
	Message    string
	Cause      error
	StatusCode int
}

func (e *TokenRequestError) Unwrap() error { return e.Cause }

func (e *TokenRequestError) Error() string { return "grok: token request failed: " + e.Message }

// MalformedResponseError: a server answer that could not be understood.
type MalformedResponseError struct{ Message string }

func (e *MalformedResponseError) Error() string { return "grok: invalid response: " + e.Message }

// ProviderMismatchError is reused from the shared transfer types.
