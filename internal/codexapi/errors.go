package codexapi

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrAuthenticationRequired means the credential was refused (HTTP 401, an
// OAuth refresh that is no longer valid, a missing refresh token, a failed
// login). The account needs to sign in again.
var ErrAuthenticationRequired = errors.New("authentication required")

// ErrBadServerResponse matches any HTTP status outside 2xx that is not an
// authentication refusal; see HTTPError for the status.
var ErrBadServerResponse = errors.New("bad server response")

// CallbackListenError preserves the local bind cause without exposing addresses
// or external error text; ErrPortInUse remains the compatibility sentinel.
type CallbackListenError struct{ Cause error }

func (e *CallbackListenError) Error() string        { return "OAuth callback listener could not start" }
func (e *CallbackListenError) Unwrap() error        { return e.Cause }
func (e *CallbackListenError) Is(target error) bool { return target == ErrPortInUse }

// HTTPError is an unsuccessful HTTP status. Its message never includes the
// response body, which could echo a token.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s (HTTP %d)", http.StatusText(e.StatusCode), e.StatusCode)
}

// Is makes errors.Is(err, ErrBadServerResponse) true for every HTTPError.
func (e *HTTPError) Is(target error) bool { return target == ErrBadServerResponse }

// NeedsReauth reports whether err means the account must sign in again.
func NeedsReauth(err error) bool { return errors.Is(err, ErrAuthenticationRequired) }
