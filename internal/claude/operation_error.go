package claude

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/licoy/quosparr/internal/platform"
)

// ErrorStage and ErrorKind are stable diagnostic codes, never external text.
type ErrorStage string
type ErrorKind string

const StageRollbackCredential ErrorStage = "rollback_credential"
const StageOAuthCallback ErrorStage = "oauth_callback"
const KindAddressInUse ErrorKind = "address_in_use"
const KindConnectionRefused ErrorKind = "connection_refused"

const (
	StageRefresh   ErrorStage = "refresh"
	StageReadUsage ErrorStage = "read_usage"
)

const (
	StageReadOfficial      ErrorStage = "read_official"
	StageReadKeychain      ErrorStage = "read_keychain"
	StageWriteKeychain     ErrorStage = "write_keychain"
	StageRollbackKeychain  ErrorStage = "rollback_keychain"
	StageReadIndex         ErrorStage = "read_index"
	StageWriteIndex        ErrorStage = "write_index"
	StageReadCredential    ErrorStage = "read_credential"
	StageWriteCredential   ErrorStage = "write_credential"
	StagePrepareDirectory  ErrorStage = "prepare_directory"
	StageReadIdentity      ErrorStage = "read_identity"
	StageReadSettings      ErrorStage = "read_settings"
	StageOAuthExchange     ErrorStage = "oauth_exchange"
	StageOAuthIdentity     ErrorStage = "oauth_identity"
	KindTimeout            ErrorKind  = "timeout"
	KindCanceled           ErrorKind  = "canceled"
	KindPermission         ErrorKind  = "permission"
	KindReadOnly           ErrorKind  = "read_only"
	KindDiskFull           ErrorKind  = "disk_full"
	KindNotFound           ErrorKind  = "not_found"
	KindInvalidData        ErrorKind  = "invalid_data"
	KindUnsupportedVersion ErrorKind  = "unsupported_version"
	KindNotPrivate         ErrorKind  = "not_private"
	KindCommand            ErrorKind  = "command"
	KindHTTP               ErrorKind  = "http"
	KindNetwork            ErrorKind  = "network"
	KindUnknown            ErrorKind  = "unknown"
	KindRollbackFailed     ErrorKind  = "rollback_failed"
)

// OperationError exposes only safe codes and numeric statuses. The underlying
// error is retained for errors.Is/As, and must never be rendered directly.
type OperationError struct {
	Stage      ErrorStage
	Kind       ErrorKind
	ExitCode   int
	HTTPStatus int
	// Failure and Rollback are one-level safe summaries, only populated when
	// an operation and its recovery both fail. Callers must not render causes.
	Failure  *OperationError
	Rollback *OperationError
	cause    error
}

func (e *OperationError) Error() string {
	return fmt.Sprintf("claude: %s (%s; exit=%d; http=%d)", e.Stage, e.Kind, e.ExitCode, e.HTTPStatus)
}
func (e *OperationError) Unwrap() error { return e.cause }
func operationError(stage ErrorStage, err error) error {
	if err == nil {
		return nil
	}
	var existing *OperationError
	if errors.As(err, &existing) {
		return err
	}
	return &OperationError{Stage: stage, Kind: errorKind(err), cause: err}
}
func classifiedError(stage ErrorStage, kind ErrorKind, err error) error {
	return &OperationError{Stage: stage, Kind: kind, cause: err}
}
func errorKind(err error) ErrorKind {
	switch {
	case errors.Is(err, context.Canceled):
		return KindCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return KindTimeout
	case errors.Is(err, syscall.EADDRINUSE):
		return KindAddressInUse
	case errors.Is(err, os.ErrPermission):
		return KindPermission
	case errors.Is(err, syscall.EROFS):
		return KindReadOnly
	case errors.Is(err, syscall.ENOSPC):
		return KindDiskFull
	case errors.Is(err, os.ErrNotExist), errors.Is(err, ErrNotLoggedIn):
		return KindNotFound
	}
	if kind := platform.NativeErrorKind(err); kind != "" {
		return ErrorKind(kind)
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return KindTimeout
		}
		return KindNetwork
	}
	return KindUnknown
}

func rollbackOperationError(stage ErrorStage, failure, rollback error) error {
	return &OperationError{Stage: stage, Kind: KindRollbackFailed,
		Failure: operationSummary(stage, failure), Rollback: operationSummary(stage, rollback),
		cause: errors.Join(failure, rollback)}
}
func operationSummary(stage ErrorStage, err error) *OperationError {
	var detail *OperationError
	if errors.As(err, &detail) {
		return &OperationError{Stage: detail.Stage, Kind: detail.Kind, ExitCode: detail.ExitCode, HTTPStatus: detail.HTTPStatus}
	}
	return &OperationError{Stage: stage, Kind: errorKind(err)}
}
