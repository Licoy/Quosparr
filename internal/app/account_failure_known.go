package app

import (
	"errors"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

func accountKnownReason(l l10n.Localizer, err error) string {
	for _, entry := range []struct {
		err error
		key l10n.Key
	}{
		{grok.ErrCLINotInstalled, l10n.KeyGrokCLIUnavailable},
		{grok.ErrAPIKeyOnlyNotManageable, l10n.KeyGrokNoManagedOAuth},
		{grok.ErrNoManagedCredential, l10n.KeyGrokNoManagedOAuth},
		{grok.ErrAuthNoManagedCredential, l10n.KeyGrokNoManagedOAuth},
		{codex.ErrCredentialStoreUnsupported, l10n.KeyCodexCredentialStoreUnsupported},
		{codex.ErrCredentialConfigInvalid, l10n.KeyCodexCredentialConfigInvalid},
		{codex.ErrEmptySelection, l10n.KeyAccountsExportEmpty},
		{codex.ErrNoAccountsExported, l10n.KeyAccountsExportFailed},
	} {
		if errors.Is(err, entry.err) {
			return l.Text(entry.key)
		}
	}
	if errors.Is(err, grok.ErrAuthFileLocked) {
		return accountReasonText(l, "locked")
	}
	var oauth *codexapi.OAuthError
	if errors.As(err, &oauth) {
		switch oauth.Description {
		case "access_denied":
			return tx(l, xClaudeOAuthDenied)
		case "OAuth state mismatch", "OAuth code missing":
			return tx(l, xOAuthCallback)
		}
	}
	return accountKnownTypedReason(l, err)
}

func accountKnownTypedReason(l l10n.Localizer, err error) string {
	var old *grok.CLITooOldError
	var grokCurrent *grok.CurrentAccountCannotBeRemovedError
	var claudeCurrent *claude.CurrentAccountCannotBeRemovedError
	var mismatch *grok.CredentialIdentityMismatchError
	var running *grok.ProcessesRunningError
	var claudeRunning *claude.ProcessesRunningError
	var provider *codex.ProviderMismatchError
	switch {
	case errors.As(err, &old):
		return l.Text(l10n.KeyGrokCLITooOld)
	case errors.As(err, &grokCurrent):
		return l.Text(l10n.KeyGrokCurrentAccountCannotDelete)
	case errors.As(err, &claudeCurrent):
		return l.Text(l10n.KeyClaudeCurrentAccountCannotDelete)
	case errors.As(err, &mismatch):
		return accountReasonText(l, "identity_mismatch")
	case errors.As(err, &running), errors.As(err, &claudeRunning):
		return accountReasonText(l, "processes_running")
	case errors.As(err, &provider):
		return accountReasonText(l, "provider_mismatch")
	}
	return ""
}
