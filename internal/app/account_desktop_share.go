package app

import (
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/l10n"
)

// codexDesktopShareControls offers sharing the ChatGPT desktop app's local
// projects across Codex accounts, and undoing Quosparr's last change to it.
func codexDesktopShareControls(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp) {
	s, l := u.s, u.L()
	hint := tx(l, xCodexDesktopShareHint)
	if !codexDesktopShareSupported {
		hint = tx(l, xMacOnly)
	}
	ui.Column(c.Key("codex-desktop-share")).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, tx(l, xCodexDesktopShare)).FontSize(12).Grow(1).MinWidth(0)
			if v, ok := toggle(c, "codex-desktop-share", tx(l, xCodexDesktopShare), s.Prefs().ShareCodexDesktopProjects, s.ReadOnly() || f.busy || !codexDesktopShareSupported); ok {
				if v {
					// Accounts may have drifted apart while sharing was off.
					_ = rt.desktopSharer.ResetPool()
				}
				s.SetShareCodexDesktopProjects(v)
			}
		})
		ui.Text(c, hint).FontSize(11).TextColor(c.Theme().TextMuted)
		button := ghostButton(c, icUndo, tx(l, xCodexDesktopRestore), false, f.busy || !codexDesktopShareSupported)
		if button.Clicked() {
			f.desktopRestore = true
		}
		confirmPopover(c, l, button, &f.desktopRestore, tx(l, xCodexDesktopRestore), tx(l, xCodexDesktopRestoreHint), tx(l, xCodexDesktopRestore), nil, func() {
			u.accountAct(f, "", func() string { return rt.restoreDesktopProjects(l) })
		})
	})
}

func (rt *liveApp) restoreDesktopProjects(l l10n.Localizer) string {
	err := rt.desktopSharer.RestoreLatest()
	switch {
	case err == nil:
		return tx(l, xCodexDesktopRestored)
	case errors.Is(err, codexdesktop.ErrDesktopRunning):
		return tx(l, xCodexDesktopQuitFirst)
	case errors.Is(err, codexdesktop.ErrNoBackup):
		return tx(l, xCodexDesktopNoBackup)
	}
	return tx(l, xCodexDesktopRestoreFailed) + ": " + err.Error()
}
