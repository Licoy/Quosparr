package app

import (
	"errors"
	"fmt"

	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
)

// geminiKnownReason classifies the Gemini errors; "" leaves err to the
// shared classification. It never renders Error(), which may name an
// account or a validation link.
func geminiKnownReason(l l10n.Localizer, err error) string {
	for _, entry := range []struct {
		err error
		key l10n.Key
	}{
		{gemini.ErrNotInstalled, l10n.KeyGeminiNotInstalled},
		{gemini.ErrIdentityUnknown, l10n.KeyGeminiOAuthIdentity},
		{gemini.ErrLoginIdentity, l10n.KeyGeminiOAuthIdentity},
		{gemini.ErrLoginPortInUse, l10n.KeyGeminiOAuthPort},
	} {
		if errors.Is(err, entry.err) {
			return l.Text(entry.key)
		}
	}
	for _, entry := range []struct {
		err error
		key textKey
	}{
		{gemini.ErrLoginDenied, xClaudeOAuthDenied}, {gemini.ErrLoginCallback, xOAuthCallback}, {gemini.ErrLoginExchange, xOAuthExchange},
	} {
		if errors.Is(err, entry.err) {
			return tx(l, entry.key)
		}
	}
	for _, entry := range []struct {
		err  error
		code string
	}{
		{gemini.ErrOfficialCredentialMissing, "not_found"}, {gemini.ErrOfficialLocked, "locked"},
		{gemini.ErrOfficialCredentialMalformed, "invalid_data"}, {gemini.ErrInvalidCredential, "invalid_data"},
		{gemini.ErrQuotaParse, "invalid_data"}, {gemini.ErrQuotaSnapshotJSON, "invalid_data"},
		{gemini.ErrAuthenticationRequired, "authentication"}, {gemini.ErrUnauthorized, "authentication"},
		{gemini.ErrKeychainUnavailable, "read_keychain"}, {gemini.ErrKeychainPayloadTooLarge, "payload_large"},
		{gemini.ErrKeychainVerify, "verify"}, {gemini.ErrLoginTimedOut, "timeout"},
	} {
		if errors.Is(err, entry.err) {
			return accountReasonText(l, entry.code)
		}
	}
	return geminiTypedReason(l, err)
}

func geminiTypedReason(l l10n.Localizer, err error) string {
	var (
		validation *gemini.ValidationRequiredError
		limited    *gemini.RateLimitedError
		timeout    *gemini.TimeoutError
		current    *gemini.CurrentAccountCannotBeRemovedError
		running    *gemini.ProcessesRunningError
		missing    *gemini.AccountNotFoundError
		credential *gemini.CredentialMissingError
		index      *gemini.InvalidIndexError
		version    *gemini.UnsupportedIndexVersionError
		mismatch   *gemini.CredentialIdentityMismatchError
		partial    *gemini.PartialWriteError
		token      *gemini.TokenRequestError
		status     *gemini.StatusError
	)
	switch {
	case errors.As(err, &validation):
		if validation.SubscriptionRequired() {
			return l.Text(l10n.KeyGeminiSubscriptionRequired)
		}
		return l.Text(l10n.KeyGeminiValidationRequired)
	case errors.As(err, &current):
		return l.Text(l10n.KeyGeminiCurrentAccountCannotDelete)
	case errors.As(err, &limited):
		return accountReasonText(l, "rate_limited")
	case errors.As(err, &timeout):
		return accountReasonText(l, "timeout")
	case errors.As(err, &running):
		return accountReasonText(l, "processes_running")
	case errors.As(err, &missing), errors.As(err, &credential):
		return accountReasonText(l, "not_found")
	case errors.As(err, &index):
		return accountReasonText(l, "invalid_index")
	case errors.As(err, &version):
		return accountReasonText(l, "unsupported_version")
	case errors.As(err, &mismatch):
		return accountReasonText(l, "identity_mismatch")
	case errors.As(err, &partial):
		return accountReasonText(l, "rollback_failed")
	case errors.As(err, &token) && token.StatusCode >= 100 && token.StatusCode <= 599:
		return fmt.Sprintf("%s (HTTP %d)", accountReasonText(l, "http"), token.StatusCode)
	case errors.As(err, &status) && status.StatusCode >= 100 && status.StatusCode <= 599:
		return fmt.Sprintf("%s (HTTP %d)", accountReasonText(l, "http"), status.StatusCode)
	}
	return ""
}
