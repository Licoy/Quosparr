package gemini

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Persisted error codes (ManagedAccount.LastError).
const (
	ErrorCodeNotInstalled          = "not_installed"
	ErrorCodeNotSignedIn           = "not_signed_in"
	ErrorCodeAuthenticationNeeded  = "authentication_required"
	ErrorCodeValidationRequired    = "validation_required"
	ErrorCodeSubscriptionRequired  = "subscription_required"
	ErrorCodeTimeout               = "timeout"
	ErrorCodeRateLimited           = "rate_limited"
	ErrorCodeQuotaParseFailed      = "quota_parse_failed"
	ErrorCodeRefreshFailed         = "refresh_failed"
	ErrorCodeCancelled             = "cancelled"
	failureImportFailed            = "import failed"
	failureInvalidCredential       = "invalid Gemini credential"
	failureCredentialIdentityError = "credential identity mismatch"
	failureIdentityUnknown         = "account identity could not be verified"
)

// Account errors. None of them carries credential text.
var (
	// ErrNotInstalled: no Antigravity client is installed.
	ErrNotInstalled = errors.New("gemini: Antigravity is not installed")
	// ErrOfficialCredentialMissing: no official login in the keychain or files.
	ErrOfficialCredentialMissing = errors.New("gemini: official credential is missing")
	// ErrOfficialCredentialMalformed: the official login cannot be used.
	ErrOfficialCredentialMalformed = errors.New("gemini: official credential is malformed")
	// ErrInvalidCredential: the data is not a usable Antigravity credential.
	ErrInvalidCredential = errors.New("gemini: invalid credential")
	// ErrAuthenticationRequired: the refresh token was rejected or is missing;
	// the user has to sign in again.
	ErrAuthenticationRequired = errors.New("gemini: authentication is required")
	// ErrUnauthorized: Cloud Code or userinfo answered 401.
	ErrUnauthorized = errors.New("gemini: request unauthorized")
	// ErrIdentityUnknown: neither the id_token nor userinfo named the account.
	ErrIdentityUnknown = errors.New("gemini: account identity is unknown")
	// ErrOfficialLocked: another Quosparr operation holds the official lock.
	ErrOfficialLocked = errors.New("gemini: the official credential is locked")
	// ErrKeychainUnavailable: the platform keychain cannot be used.
	ErrKeychainUnavailable = errors.New("gemini: keychain is unavailable")
	// ErrKeychainPayloadTooLarge: the security -i line would not fit its
	// 4096-byte buffer; the secret is never passed on argv instead.
	ErrKeychainPayloadTooLarge = errors.New("gemini: keychain payload is too large")
	// ErrKeychainVerify: a keychain read-back did not match what was written.
	ErrKeychainVerify = errors.New("gemini: keychain read-back mismatch")
	// ErrQuotaParse: a Cloud Code answer could not be understood.
	ErrQuotaParse = errors.New("gemini: quota response could not be parsed")
	// ErrQuotaSnapshotJSON: a cached snapshot is not valid snapshot JSON.
	ErrQuotaSnapshotJSON = errors.New("gemini: quota snapshot is invalid")
)

// Login errors.
var (
	ErrLoginPortInUse = errors.New("gemini: login callback port unavailable")
	ErrLoginTimedOut  = errors.New("gemini: login timed out")
	ErrLoginCallback  = errors.New("gemini: invalid login callback")
	ErrLoginDenied    = errors.New("gemini: login authorization denied")
	ErrLoginExchange  = errors.New("gemini: login exchange failed")
	ErrLoginIdentity  = errors.New("gemini: login identity could not be verified")
)

// InvalidIndexError: gemini-index.json cannot be decoded.
type InvalidIndexError struct{ Cause error }

func (e *InvalidIndexError) Error() string { return "gemini: account index is invalid or corrupted" }
func (e *InvalidIndexError) Unwrap() error { return e.Cause }

// UnsupportedIndexVersionError: gemini-index.json is of another version.
type UnsupportedIndexVersionError struct{ Version int }

func (e *UnsupportedIndexVersionError) Error() string {
	return fmt.Sprintf("gemini: unsupported account index version %d", e.Version)
}

// AccountNotFoundError reports an id that is not in the index.
type AccountNotFoundError struct{ ID string }

func (e *AccountNotFoundError) Error() string {
	return fmt.Sprintf("gemini: account %q not found", e.ID)
}

// CredentialMissingError reports an indexed account without a credential file.
type CredentialMissingError struct{ ID string }

func (e *CredentialMissingError) Error() string {
	return fmt.Sprintf("gemini: credential for account %q is missing", e.ID)
}

// CredentialIdentityMismatchError: a credential belongs to another account.
type CredentialIdentityMismatchError struct{ ID string }

func (e *CredentialIdentityMismatchError) Error() string {
	return fmt.Sprintf("gemini: credential does not match account %q", e.ID)
}

// CurrentAccountCannotBeRemovedError: the current account cannot be removed.
type CurrentAccountCannotBeRemovedError struct{ ID string }

func (e *CurrentAccountCannotBeRemovedError) Error() string {
	return fmt.Sprintf("gemini: account %q is current and cannot be removed", e.ID)
}

// ProcessesRunningError: Antigravity processes are running, so a switch is refused.
type ProcessesRunningError struct{ PIDs []int }

func (e *ProcessesRunningError) Error() string {
	ids := make([]string, len(e.PIDs))
	for i, pid := range e.PIDs {
		ids[i] = fmt.Sprint(pid)
	}
	return "gemini: Antigravity is running (pid " + strings.Join(ids, ", ") + ")"
}

// AccountIOError is a storage failure with a fixed, secret-free message.
type AccountIOError struct {
	Message string
	Cause   error
}

func (e *AccountIOError) Unwrap() error { return e.Cause }
func (e *AccountIOError) Error() string { return "gemini: " + e.Message }

// PartialWriteError: an operation failed and could not be fully undone.
type PartialWriteError struct {
	Message string
	Cause   error
}

func (e *PartialWriteError) Unwrap() error { return e.Cause }
func (e *PartialWriteError) Error() string { return "gemini: " + e.Message }

// TimeoutError: a request ran into its timeout.
type TimeoutError struct{ Operation string }

func (e *TimeoutError) Error() string { return "gemini: " + e.Operation + " timed out" }

// TokenRequestError is a failed token request that is not invalid_grant.
type TokenRequestError struct {
	StatusCode int
	Cause      error
}

func (e *TokenRequestError) Unwrap() error { return e.Cause }
func (e *TokenRequestError) Error() string {
	return fmt.Sprintf("gemini: token request failed (HTTP %d)", e.StatusCode)
}

// ValidationRequiredError is a 403 asking the user to verify the account in
// a browser (or to subscribe). URL is the validation link, never a token.
type ValidationRequiredError struct {
	Reason string
	URL    string
}

func (e *ValidationRequiredError) Error() string {
	return "gemini: account validation is required (" + e.Reason + ")"
}

// SubscriptionRequired reports whether the server asked for a subscription
// rather than a verification.
func (e *ValidationRequiredError) SubscriptionRequired() bool {
	return e.Reason == reasonSubscriptionRequired
}

// RateLimitedError is HTTP 429, or a backoff still in effect.
type RateLimitedError struct {
	RetryAfter    time.Duration
	HasRetryAfter bool
}

func (e *RateLimitedError) Error() string { return "gemini: rate limited" }

// StatusError is any other non-success HTTP status of a Cloud Code request.
type StatusError struct {
	Method     string
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("gemini: %s failed (HTTP %d)", e.Method, e.StatusCode)
}

// ErrorCode maps err to the code persisted in ManagedAccount.LastError.
func ErrorCode(err error) string {
	var (
		validation *ValidationRequiredError
		limited    *RateLimitedError
		timeout    *TimeoutError
	)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotInstalled):
		return ErrorCodeNotInstalled
	case errors.Is(err, ErrOfficialCredentialMissing):
		return ErrorCodeNotSignedIn
	case errors.Is(err, ErrAuthenticationRequired), errors.Is(err, ErrUnauthorized):
		return ErrorCodeAuthenticationNeeded
	case errors.As(err, &validation):
		if validation.SubscriptionRequired() {
			return ErrorCodeSubscriptionRequired
		}
		return ErrorCodeValidationRequired
	case errors.As(err, &limited):
		return ErrorCodeRateLimited
	case errors.Is(err, context.Canceled):
		return ErrorCodeCancelled
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout):
		return ErrorCodeTimeout
	case errors.Is(err, ErrQuotaParse):
		return ErrorCodeQuotaParseFailed
	}
	return ErrorCodeRefreshFailed
}

// requiresReauth reports whether err means the user has to sign in again.
func requiresReauth(err error) bool {
	return errors.Is(err, ErrAuthenticationRequired) || errors.Is(err, ErrUnauthorized)
}
