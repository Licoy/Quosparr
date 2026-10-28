package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/claudedesktop"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/platform"
)

// accountFailure accepts only an already localized action and safe, classified details.
func accountFailure(l l10n.Localizer, action string, err error) string {
	if err == nil {
		return action
	}
	detail := accountErrorReason(l, err)
	if detail == action {
		return action
	}
	return action + "\n" + detail
}

func accountReasonText(l l10n.Localizer, code string) string {
	if code == "connection_refused" {
		code = "connect"
	}
	key := textKey("accountFailure_" + code)
	if _, ok := extraTexts[key]; !ok {
		key = "accountFailure_unknown"
	}
	return tx(l, key)
}

// accountErrorReason never renders Error(), paths, response bodies or account identity.
func accountErrorReason(l l10n.Localizer, err error) string {
	if err == nil {
		return ""
	}
	detail := accountUnderlyingReason(l, err)
	var operation *claude.OperationError
	if errors.As(err, &operation) {
		// The outer operation may deliberately retag identity or rollback failures.
		switch operation.Kind {
		case claude.KindNetwork, claude.KindUnknown:
			if detail == accountReasonText(l, "unknown") {
				detail = accountReasonText(l, string(operation.Kind))
			}
		default:
			detail = accountReasonText(l, string(operation.Kind))
		}
		if accountRollbackFailed(err) {
			detail = accountReasonText(l, "rollback_failed")
		}
		if operation.HTTPStatus >= 100 && operation.HTTPStatus <= 599 {
			detail += fmt.Sprintf(" (HTTP %d)", operation.HTTPStatus)
		}
		if operation.ExitCode != 0 {
			detail += fmt.Sprintf(" (exit %d)", operation.ExitCode)
		}
		text := accountReasonText(l, string(operation.Stage)) + ": " + detail
		if operation.Failure != nil {
			text += "\n" + accountReasonText(l, "original_failure") + ": " + accountOperationSummary(l, operation.Failure)
		}
		if operation.Rollback != nil {
			text += "\n" + accountReasonText(l, "recovery_failure") + ": " + accountOperationSummary(l, operation.Rollback)
		}
		return text
	}
	return detail
}

func accountUnderlyingReason(l l10n.Localizer, err error) string {
	if errors.Is(err, codex.ErrSwitchFilesChanged) {
		return accountReasonText(l, "changed")
	}
	if accountRollbackFailed(err) {
		return accountReasonText(l, "rollback_failed")
	}
	var codexIndex *codex.InvalidIndexError
	var grokIndex *grok.InvalidIndexError
	if errors.As(err, &codexIndex) || errors.As(err, &grokIndex) {
		return accountReasonText(l, "invalid_index")
	}
	if kind := platform.NativeErrorKind(err); kind != "" {
		return accountReasonText(l, kind)
	}
	// Known low-level causes take precedence over generic operation sentinels.
	for _, entry := range []struct {
		err  error
		code string
	}{
		{syscall.EADDRINUSE, "address_in_use"}, {syscall.EROFS, "read_only"}, {syscall.ENOSPC, "disk_full"}, {os.ErrPermission, "permission"},
		{os.ErrNotExist, "not_found"}, {os.ErrExist, "exists"}, {context.DeadlineExceeded, "timeout"}, {context.Canceled, "canceled"},
		{syscall.ECONNREFUSED, "connect"}, {syscall.ECONNRESET, "connect"}, {syscall.ENETUNREACH, "connect"},
	} {
		if errors.Is(err, entry.err) {
			return accountReasonText(l, entry.code)
		}
	}
	var syntax *json.SyntaxError
	var mismatch *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &mismatch) {
		return accountReasonText(l, "invalid_data")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return accountReasonText(l, "dns")
	}
	var cert *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var record tls.RecordHeaderError
	if errors.As(err, &cert) || errors.As(err, &authority) || errors.As(err, &hostname) || errors.As(err, &invalid) || errors.As(err, &record) {
		return accountReasonText(l, "TLS")
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return accountReasonText(l, "timeout")
	}
	if key := netx.L10nKey(err); key != "" {
		return l.Text(key)
	}
	if network != nil {
		return accountReasonText(l, "network")
	}
	var httpError *codexapi.HTTPError
	if errors.As(err, &httpError) && httpError.StatusCode >= 100 && httpError.StatusCode <= 599 {
		return fmt.Sprintf("%s (HTTP %d)", accountReasonText(l, "http"), httpError.StatusCode)
	}
	var apiHTTP *apiquota.HTTPStatusError
	if errors.As(err, &apiHTTP) && apiHTTP.Code >= 100 && apiHTTP.Code <= 599 {
		return fmt.Sprintf("%s (HTTP %d)", accountReasonText(l, "http"), apiHTTP.Code)
	}
	var token *grok.TokenRequestError
	if errors.As(err, &token) && token.StatusCode >= 100 && token.StatusCode <= 599 {
		return fmt.Sprintf("%s (HTTP %d)", accountReasonText(l, "http"), token.StatusCode)
	}
	return accountProviderReason(l, err)
}

func accountProviderReason(l l10n.Localizer, err error) string {
	if known := accountKnownReason(l, err); known != "" {
		return known
	}
	for _, known := range []struct {
		err error
		key textKey
	}{
		{claude.ErrLoginDenied, xClaudeOAuthDenied}, {claude.ErrLoginPortInUse, xClaudeOAuthPort}, {codexapi.ErrPortInUse, xOAuthPort},
		{claudedesktop.ErrRollback, xDesktopRecovery}, {claudedesktop.ErrRecoveryRequired, xDesktopRecovery}, {claudedesktop.ErrStop, xDesktopStopFailed},
		{claudedesktop.ErrLaunch, xDesktopLaunchFailed}, {claudedesktop.ErrBusy, xDesktopBusy}, {claudedesktop.ErrNotBound, xDesktopUnbound},
		{claudedesktop.ErrCrossVolume, xDesktopCrossVolume}, {claudedesktop.ErrProfileChanged, xDesktopChanged}, {claudedesktop.ErrUnsafePath, xDesktopUnsafe},
		{claudedesktop.ErrPersistence, xDesktopPersistence},
	} {
		if errors.Is(err, known.err) {
			return tx(l, known.key)
		}
	}
	if errors.Is(err, ErrProxyInvalidCredentials) {
		return l.Text(l10n.KeyProxyInvalidCredentials)
	}
	for _, entry := range []struct {
		err  error
		code string
	}{
		{claude.ErrLoginRollback, "rollback_failed"}, {claude.ErrSwitchRollback, "rollback_failed"}, {codex.ErrRollbackFailed, "rollback_failed"},
		{claude.ErrKeychainPayloadTooLarge, "payload_large"}, {claude.ErrKeychainVerify, "verify"},
		{claude.ErrCredentialChanged, "changed"}, {claude.ErrOAuthAccountConflict, "changed"},
		{codex.ErrProviderConfigInvalid, "invalid_data"}, {codex.ErrProviderConfigUnsupported, "unsupported_version"}, {codex.ErrNotUsableAsCodexLogin, "invalid_data"}, {grok.ErrAuthInvalidRoot, "invalid_data"}, {grok.ErrAuthInvalidManagedCredential, "invalid_data"},
		{claude.ErrSettingsInvalid, "invalid_data"}, {claude.ErrAuthInvalidRoot, "invalid_data"}, {claude.ErrAuthNoOAuth, "invalid_data"},
		{claude.ErrPastedCredential, "invalid_data"}, {claude.ErrUsageParse, "invalid_data"}, {codex.ErrInvalidCredential, "invalid_data"},
		{grok.ErrInvalidCredential, "invalid_data"}, {grok.ErrOfficialCredentialMalformed, "invalid_data"}, {codexapi.ErrDecode, "invalid_data"},
		{claude.ErrNotLoggedIn, "not_found"}, {grok.ErrOfficialCredentialMissing, "not_found"},
		{claude.ErrUnauthorized, "authentication"}, {claude.ErrTokenExpired, "authentication"}, {claude.ErrInvalidGrant, "authentication"},
		{codex.ErrReauthRequired, "authentication"}, {codex.ErrExpiredAccessWithoutRefresh, "authentication"},
		{codexapi.ErrAuthenticationRequired, "authentication"}, {grok.ErrAuthenticationRequired, "authentication"},
		{apiquota.ErrTimeout, "timeout"}, {apiquota.ErrModelsNotFound, "not_found"}, {apiquota.ErrInvalidResponse, "invalid_data"},
		{claude.ErrLoginTimedOut, "timeout"}, {codexapi.ErrTimedOut, "timeout"},
		{claude.ErrLoginCallback, "invalid_data"}, {claude.ErrLoginIdentity, "invalid_data"}, {claude.ErrIdentityRequired, "invalid_data"},
		{context.Canceled, "canceled"},
	} {
		if errors.Is(err, entry.err) {
			return accountReasonText(l, entry.code)
		}
	}
	var oauth *grok.OAuthError
	if errors.As(err, &oauth) {
		switch oauth.Kind {
		case grok.OAuthDiscoveryFailed:
			return accountReasonText(l, "oauth_discovery")
		case grok.OAuthInvalidEndpoint:
			return accountReasonText(l, "invalid_data")
		case grok.OAuthDeviceCodeFailed:
			return accountReasonText(l, "oauth_device")
		case grok.OAuthAuthorizationPending:
			return l.Text(l10n.KeyAccountsOAuthWaiting)
		case grok.OAuthSlowDown:
			return accountReasonText(l, "rate_limited")
		case grok.OAuthExpired:
			return accountReasonText(l, "timeout")
		case grok.OAuthDenied:
			return tx(l, xClaudeOAuthDenied)
		case grok.OAuthTokenFailed:
			return tx(l, xOAuthExchange)
		case grok.OAuthMissingIdentity:
			return accountReasonText(l, "oauth_identity") + ": " + accountReasonText(l, "invalid_data")
		case grok.OAuthWriteFailed:
			return accountReasonText(l, "write_credential") + ": " + accountReasonText(l, "unknown")
		case grok.OAuthBrowserOpenFailed:
			return tx(l, xOAuthBrowser)
		}
	}
	var rate *claude.RateLimitedError
	if errors.As(err, &rate) {
		return accountReasonText(l, "rate_limited")
	}
	var claudeMissing *claude.AccountNotFoundError
	var codexMissing *codex.AccountNotFoundError
	var codexCredential *codex.CredentialMissingError
	var grokMissing *grok.AccountNotFoundError
	var grokCredential *grok.CredentialMissingError
	if errors.As(err, &claudeMissing) || errors.As(err, &codexMissing) || errors.As(err, &codexCredential) || errors.As(err, &grokMissing) || errors.As(err, &grokCredential) {
		return accountReasonText(l, "not_found")
	}
	var malformed *grok.MalformedResponseError
	var format *codex.UnsupportedFormatError
	if errors.As(err, &malformed) || errors.As(err, &format) {
		return accountReasonText(l, "invalid_data")
	}
	var version *grok.UnsupportedIndexVersionError
	var transferVersion *codex.UnsupportedVersionError
	if errors.As(err, &version) || errors.As(err, &transferVersion) {
		return accountReasonText(l, "unsupported_version")
	}
	return accountReasonText(l, "unknown")
}

// Explicit proxy configuration messages are already actionable; wrapped platform
// and transport failures also include a classified underlying cause.
func proxyFailureText(l l10n.Localizer, err error) string {
	return accountFailure(l, l.Text(proxyErrorKey(err)), err)
}

func accountOperationSummary(l l10n.Localizer, op *claude.OperationError) string {
	text := accountReasonText(l, string(op.Stage)) + ": " + accountReasonText(l, string(op.Kind))
	if op.HTTPStatus >= 100 && op.HTTPStatus <= 599 {
		text += fmt.Sprintf(" (HTTP %d)", op.HTTPStatus)
	}
	if op.ExitCode != 0 {
		text += fmt.Sprintf(" (exit %d)", op.ExitCode)
	}
	return text
}

// Recovery failures must stay visible even when cancellation caused the original failure.
func accountRollbackFailed(err error) bool {
	if errors.Is(err, claude.ErrLoginRollback) || errors.Is(err, claude.ErrSwitchRollback) || errors.Is(err, codex.ErrRollbackFailed) {
		return true
	}
	var operation *claude.OperationError
	if errors.As(err, &operation) && operation.Kind == claude.KindRollbackFailed {
		return true
	}
	var partial *grok.PartialWriteError
	return errors.As(err, &partial)
}
