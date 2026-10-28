package app

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
)

func (f *accountForm) closeEditor() {
	f.mode, f.editID, f.paste = "", "", ""
	f.alias, f.note, f.switchName, f.editorError = "", "", "", ""
	f.selection, f.api, f.switchRelay = nil, apiDraft{}, false
	f.desktopSwitch = false
}

func accountEditorTitle(l l10n.Localizer, f *accountForm) string {
	if isAPIMode(f.mode) {
		return apiEditorTitle(l, f.mode)
	}
	key := l10n.KeyAccountsAddPaste
	switch f.mode {
	case "switch", "claude-switch":
		key = l10n.KeyAccountsSwitchConfirmTitle
	case "alias":
		key = l10n.KeyAlias
	case "note":
		key = l10n.KeyAccountsNote
	case "paste":
		if f.kind == accountKindGrok {
			key = l10n.KeyGrokAccountsAddPaste
		}
	}
	return l.Text(key)
}

func accountEditor(c *ui.Context, u *settingsUI, f *accountForm) {
	if f.mode == "" {
		return
	}
	l, open := u.L(), true
	ui.Modal(c.Key("account-editor"), &open, func() {
		ui.Text(c, accountEditorTitle(l, f)).FontSize(17).FontWeight(600)
		accountEditorFields(c, u, f)
		if f.editorError != "" {
			ui.Text(c, f.editorError).FontSize(12).TextColor(c.Theme().Danger)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if f.busy {
				ui.Spinner(c).Size(14, 14)
			}
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyCancel), f.busy).Clicked() {
				f.closeEditor()
			}
			empty := f.mode == "paste" && strings.TrimSpace(f.paste) == "" || apiEditorIncomplete(f)
			confirm := l.Text(l10n.KeyOk)
			if f.mode == "switch" || f.mode == "claude-switch" {
				confirm = l.Text(l10n.KeyAccountsMakeCurrent)
			}
			if PillButton(c, nil, confirm, f.busy || empty || f.mode == "").Clicked() {
				u.submitAccountEditor(f)
			}
		})
	}).Width(520)
	if !open && !f.busy {
		f.closeEditor()
	}
	u.apiApplyDialog(c, f)
}

// apiApplyDialog asks, before saving an edit of the API account in use,
// whether its config files change now or at the next switch. A modal rather
// than an alert: three buttons do not fit the alert's width in every language.
func (u *settingsUI) apiApplyDialog(c *ui.Context, f *accountForm) {
	if !f.api.askApply {
		return
	}
	l, product := u.L(), "Codex"
	if isClaudeAPIMode(f.mode) {
		product = "Claude Code"
	}
	ui.Modal(c.Key("api-apply"), &f.api.askApply, func() {
		ui.Text(c, tx(l, xAPIApplyTitle)).FontSize(17).FontWeight(600)
		ui.Text(c, tx(l, xAPIApplyMessage, product, product)).FontSize(12)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
				f.api.askApply = false
			}
			if PillButton(c, nil, tx(l, xAPISaveOnly), false).Clicked() {
				f.api.askApply = false
				u.submitAccountEdit(f, apiApplySaveOnly)
			}
			if PillButton(c, nil, tx(l, xAPISaveAndApply), false).Clicked() {
				f.api.askApply = false
				u.submitAccountEdit(f, apiApplyWrite)
			}
		})
	}).Width(480)
}

func accountEditorFields(c *ui.Context, u *settingsUI, f *accountForm) {
	l := u.L()
	if isAPIMode(f.mode) {
		apiAccountFields(c, u, f)
		return
	}
	switch f.mode {
	case "switch":
		ui.Text(c, l.Textf(l10n.KeyAccountsSwitchConfirmMessage, f.switchName)).FontSize(12)
		if f.switchRelay {
			ui.Text(c, tx(l, xAPIHintCodex)).FontSize(12).TextColor(c.Theme().TextMuted)
		}
		ui.Checkbox(c.Key("restart-after-switch"), &f.restartAfterSwitch, l.CodexRuntime("restart")).Disabled(!codexRestartSupported || f.busy)
		ui.Text(c, l.CodexRuntime("restart_hint")).FontSize(11).TextColor(c.Theme().TextMuted)
	case "claude-switch":
		ui.Text(c, l.Textf(l10n.KeyClaudeSwitchConfirmMessage, f.switchName)).FontSize(12)
		risk := l.Text(l10n.KeyClaudeSwitchRisk)
		if f.switchRelay {
			risk = tx(l, xAPIClaudeSwitchNote)
		}
		ui.Text(c, risk).FontSize(12).TextColor(c.Theme().TextMuted)
		if !f.switchRelay {
			desktopSwitchOption(c, l, currentRuntime(), f.editID, &f.desktopSwitch, f.busy, func() {
				id := f.editID
				f.closeEditor()
				f.desktop = newDesktopDialog(currentRuntime(), id, u.s.Prefs().AccountPrivacyModeEnabled)
			})
		}
	case "paste":
		hint := l.Text(l10n.KeyAccountsPasteHint)
		if f.kind == accountKindGrok {
			hint = l.Text(l10n.KeyGrokAccountsPasteHint)
		}
		if f.kind == accountKindClaude {
			hint = l.Text(l10n.KeyClaudePasteHint)
		}
		ui.Text(c, hint).FontSize(12).TextColor(c.Theme().TextMuted)
		ui.TextArea(c.Key("acct-paste"), &f.paste).Lines(6, 8).Label(hint).Disabled(f.busy)
	case "alias":
		ui.TextInput(c.Key("acct-edit"), &f.alias).Label(l.Text(l10n.KeyAlias)).Disabled(f.busy)
	case "note":
		ui.TextArea(c.Key("acct-note"), &f.note).Lines(3, 6).Label(l.Text(l10n.KeyAccountsNote)).Disabled(f.busy)
	}
}

func (u *settingsUI) submitAccountEditor(f *accountForm) {
	if apiNeedsApplyChoice(f) {
		f.api.askApply = true
		return
	}
	u.submitAccountEdit(f, apiApplyNone)
}

func (u *settingsUI) submitAccountEdit(f *accountForm, apply apiApply) {
	l := u.L()
	mode, id, kind := f.mode, f.editID, f.kind
	paste, alias, note, restart := f.paste, f.alias, f.note, f.restartAfterSwitch
	draft := f.api
	desktop := f.desktopSwitch
	u.accountEditAct(f, func() (string, bool) {
		if isAPIMode(mode) {
			return submitAPIAccount(l, mode, id, alias, draft, apply)
		}
		switch mode {
		case "paste":
			return pasteAccount(l, kind, paste)
		case "switch":
			return rtSwitchCodexResult(l, id, restart)
		case "claude-switch":
			rt := currentRuntime()
			if rt == nil {
				return tx(l, xDetailUnavailable), false
			}
			return rt.switchClaudeDesktopResult(context.Background(), l, id, desktop)
		default:
			msg := saveAccountMeta(l, kind, mode, id, alias, note)
			return msg, msg == ""
		}
	})
}

// Keep the form and draft available after failures; completion is explicit.
func (u *settingsUI) accountEditAct(f *accountForm, action func() (string, bool)) {
	if f.busy {
		return
	}
	f.busy, f.editorError = true, ""
	go func() {
		message, ok := action()
		u.post(func() {
			f.busy = false
			if ok {
				f.closeEditor()
				u.accountNotice(f, message)
			} else {
				f.editorError = message
			}
		})
	}()
}
