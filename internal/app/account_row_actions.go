package app

import (
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
)

// rowAction is one icon button of a settings account row; its label is the
// tooltip and the accessible name.
type rowAction struct {
	icon     *ui.SVG
	label    string
	disabled bool
	run      func()
}

// accountRowActions are the icon buttons of a settings account row, set off
// from the refresh button by a rule.
func accountRowActions(c *ui.Context, busy bool, actions []rowAction) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(1, 14).Background(hairlineColor(c.Theme()))
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			for _, a := range actions {
				if IconButton(c, a.icon, a.label, busy || a.disabled).Clicked() && a.run != nil {
					a.run()
				}
			}
		})
	})
}

// makeCurrentAction is the tick of a row: filled and disabled on the current
// account.
func makeCurrentAction(l l10n.Localizer, switchKey, currentKey l10n.Key, current bool, run func()) rowAction {
	if current {
		return rowAction{icon: icCheckCircleFill, label: l.Text(currentKey), disabled: true}
	}
	return rowAction{icon: icCheckCircle, label: l.Text(switchKey), run: run}
}

// rowPlace is where a row sits in its list: the first can't move up, the
// last can't move down.
type rowPlace struct{ first, last bool }

func placeAt(i, n int) rowPlace { return rowPlace{first: i == 0, last: i == n-1} }

func moveActions(l l10n.Localizer, place rowPlace, move func(dir int)) []rowAction {
	return []rowAction{
		{icon: icArrowUp, label: l.Text(l10n.KeyAccountsMoveUp), disabled: place.first, run: func() { move(-1) }},
		{icon: icArrowDown, label: l.Text(l10n.KeyAccountsMoveDown), disabled: place.last, run: func() { move(1) }},
	}
}

func codexRowActions(u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, place rowPlace, name string, current bool) []rowAction {
	l := u.L()
	out := []rowAction{makeCurrentAction(l, l10n.KeyAccountsMakeCurrent, l10n.KeyAccountsIsCurrentLogin, current, func() {
		f.mode, f.editID, f.switchName, f.switchRelay = "switch", acc.ID, name, acc.IsRelay()
		f.restartAfterSwitch = codexRestartSupported
	})}
	if acc.AuthMode == codex.AuthModeAPIKey {
		out = append(out, rowAction{icon: icKey, label: tx(l, xAPIEdit), run: func() { f.openCodexAPIEditor(rt, acc) }})
	}
	out = append(out,
		rowAction{icon: icPencil, label: l.Text(l10n.KeyAlias), run: func() { f.mode, f.editID, f.alias = "alias", acc.ID, acc.Alias }},
		rowAction{icon: icFile, label: l.Text(l10n.KeyAccountsNote), run: func() { f.mode, f.editID, f.note = "note", acc.ID, acc.Note }},
	)
	out = append(out, moveActions(l, place, func(dir int) { u.accountAct(f, "", func() string { return rt.moveCodex(acc.ID, dir) }) })...)
	return append(out, rowAction{icon: icTrash, label: l.Text(l10n.KeyAccountsDelete), run: func() {
		u.accountAct(f, "", func() string { return rt.deleteCodex(l, acc.ID) })
	}})
}

func claudeRowActions(u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, place rowPlace, name string, current, priv bool) []rowAction {
	l := u.L()
	out := []rowAction{makeCurrentAction(l, l10n.KeyAccountsMakeCurrent, l10n.KeyAccountsCurrent, current, func() {
		f.mode, f.editID, f.switchName, f.switchRelay = "claude-switch", acc.ID, name, acc.AuthMode == codex.AuthModeAPIKey
		f.desktopSwitch = false
	})}
	if acc.AuthMode != codex.AuthModeAPIKey {
		out = append(out,
			rowAction{icon: icLink, label: tx(l, xDesktopBind), run: func() { f.desktop = newDesktopDialog(rt, acc.ID, priv) }},
			rowAction{icon: icExternal, label: tx(l, xDesktopOpen), disabled: !desktopBound(rt, acc.ID), run: func() {
				f.desktop = newDesktopDialog(rt, acc.ID, priv)
				f.desktop.openOnly = true
			}},
		)
	}
	if acc.IsRelay() {
		out = append(out, rowAction{icon: icKey, label: tx(l, xAPIEdit), run: func() { f.openClaudeAPIEditor(rt, acc.ID) }})
	}
	out = append(out, moveActions(l, place, func(dir int) { u.accountAct(f, "", func() string { return rt.moveClaude(acc.ID, dir) }) })...)
	return append(out, rowAction{icon: icTrash, label: l.Text(l10n.KeyAccountsDelete), disabled: current, run: func() {
		u.accountAct(f, "", func() string { return rt.deleteClaude(l, acc.ID) })
	}})
}

func grokRowActions(u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, place rowPlace, name string, current bool) []rowAction {
	l := u.L()
	out := []rowAction{makeCurrentAction(l, l10n.KeyGrokAccountsMakeCurrent, l10n.KeyGrokAccountsCurrent, current, func() {
		u.accountAct(f, "", func() string { return rt.switchGrok(l, acc.ID, name) })
	})}
	out = append(out, rowAction{icon: icPencil, label: l.Text(l10n.KeyAlias), run: func() { f.mode, f.editID, f.alias = "alias", acc.ID, acc.Alias }})
	out = append(out, moveActions(l, place, func(dir int) { u.accountAct(f, "", func() string { return rt.moveGrok(acc.ID, dir) }) })...)
	return append(out, rowAction{icon: icTrash, label: l.Text(l10n.KeyGrokAccountsRemove), disabled: current, run: func() {
		u.accountAct(f, "", func() string { return rt.deleteGrok(l, acc.ID) })
	}})
}
