package app

import (
	"context"
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/l10n"
)

// codexShareOffered tells whether switching to acc can carry the ChatGPT
// desktop app's local projects; API key accounts have no desktop identity.
func codexShareOffered(acc codex.ManagedAccount) bool {
	return codexDesktopShareSupported && acc.AuthMode != codex.AuthModeAPIKey
}

func codexProjectShareNotice(l l10n.Localizer, message string, api bool) string {
	if codexDesktopShareSupported && api {
		message += "\n\n" + l.CodexRuntime("projects_api_unsupported")
	}
	return message
}

// codexShareOption is the switch dialogs' choice to carry the desktop app's
// local projects to the new account, and the way to undo Quosparr's last
// change to its data.
func codexShareOption(c *ui.Context, l l10n.Localizer, on *bool, busy bool, restore func()) {
	ui.Checkbox(c.Key("share-desktop-projects"), on, tx(l, xCodexDesktopShare)).Disabled(busy)
	dimText(c, tx(l, xCodexDesktopShareHint))
	link := linkButton(c, tx(l, xCodexDesktopRestore))
	link.Tooltip(tx(l, xCodexDesktopRestoreHint))
	if link.Clicked() && !busy {
		restore()
	}
}

func (rt *liveApp) undoDesktopProjects() error {
	target, err := rt.svc.DesktopProjectOwner()
	if err == nil {
		_, err = rt.svc.RestoreDesktopProjects(context.Background(), target, func() (codexdesktop.Outcome, error) {
			if err := rt.desktopSharer.RestoreLatest(); err != nil {
				return "", err
			}
			return codexdesktop.Shared, nil
		})
	}
	return err
}

func (rt *liveApp) restoreDesktopProjects(l l10n.Localizer) string {
	err := rt.undoDesktopProjects()
	switch {
	case err == nil:
		return tx(l, xCodexDesktopRestored)
	case errors.Is(err, codexdesktop.ErrDesktopRunning):
		return tx(l, xCodexDesktopQuitFirst)
	case errors.Is(err, codexdesktop.ErrNoBackup):
		return tx(l, xCodexDesktopNoBackup)
	}
	return desktopRecoveryError(l, err)
}
