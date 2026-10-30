package app

import (
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
)

// codexReloadError preserves the read failure without implying the write failed.
type codexReloadError struct{ cause error }

func (e *codexReloadError) Error() string { return "codex: changes saved but account reload failed" }
func (e *codexReloadError) Unwrap() error { return e.cause }

func (rt *liveApp) reloadCodexAfterSave(l l10n.Localizer) string {
	if err := rt.svc.ReloadAccounts(); err != nil {
		return accountFailure(l, tx(l, xCodexSavedReloadFailed), err)
	}
	return ""
}

func (rt *liveApp) codexImportMessage(l l10n.Localizer, batch codex.ImportBatchResult, previewFailed int) (string, bool) {
	message := accountBatchFailure(l, len(batch.Succeeded), len(batch.Failures)+previewFailed, batch.FailureErrors)
	if err := rt.svc.ReloadAccounts(); err != nil {
		action := l.Text(l10n.KeyAccountsReloadFailed)
		if len(batch.Succeeded) > 0 {
			action = tx(l, xCodexSavedReloadFailed)
		}
		reload := accountFailure(l, action, err)
		if message != "" {
			reload = message + "\n" + reload
		}
		return reload, false
	}
	if message != "" {
		return message, false
	}
	if len(batch.Succeeded) == 0 {
		return l.Text(l10n.KeyAccountsImportNoCredentials), false
	}
	return tx(l, xCodexImportResult, batch.CreatedCount, batch.UpdatedCount), true
}
