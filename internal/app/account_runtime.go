package app

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

func codexRuntimeMessage(l l10n.Localizer, status runway.CodexRuntimeView) string {
	state := status.State
	if state == "" {
		state = "unverified"
	}
	message := l.CodexRuntime(state)
	if status.FilesUpdated {
		message = l.CodexRuntime("updated") + " · " + message
	} else {
		switch state {
		case "preflight_failed", "stop_failed", "credential_failed", "write_failed_recovered", "write_failed_restarted_unverified", "recovery_failed":
			message = l.CodexRuntime("not_updated") + " · " + message
		}
	}
	if status.FailureReason != "" {
		detail := accountReasonText(l, status.FailureReason)
		if status.FailureStage != "" {
			detail = accountReasonText(l, status.FailureStage) + ": " + detail
		}
		message += "\n" + detail
	}
	return message
}

func (rt *liveApp) checkCodexRuntime(l l10n.Localizer) string {
	if err := rt.svc.CheckCodexRuntime(context.Background()); err != nil {
		return l.CodexRuntime("inspect_failed")
	}
	return codexRuntimeMessage(l, rt.svc.Snapshot().Accounts.Runtime)
}

func codexRuntimeControls(c *ui.Context, l l10n.Localizer, view runway.AccountsView, open *bool, busy bool, check, apply func()) {
	ui.Column(c.Key("codex-runtime-status")).Gap(6).Children(func() {
		ui.Text(c, l.CodexRuntime("title")).FontSize(12)
		ui.Text(c, codexRuntimeMessage(l, view.Runtime)).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if ghostButton(c, icRefresh, l.CodexRuntime("check"), false, busy || view.Switching || check == nil).Clicked() {
				check()
			}
			button := ghostButton(c, icCheckCircle, l.CodexRuntime("apply"), false, !codexRestartSupported || busy || view.Switching || view.ActiveID == "" || apply == nil)
			if button.Clicked() {
				*open = true
			}
			confirmPopover(c, l, button, open, l.CodexRuntime("apply"), l.CodexRuntime("restart_hint"), l.CodexRuntime("apply"), nil, func() {
				if apply != nil {
					apply()
				}
			})
		})
	})
}
