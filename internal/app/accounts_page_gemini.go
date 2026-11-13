package app

import (
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// renderGeminiAccounts is the Gemini tab of the settings accounts page.
func renderGeminiAccounts(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp) {
	l := u.L()
	view := rt.svc.Snapshot().Gemini.Accounts
	panelAccountStatus(c, l, view)
	rows := view.Sidebar
	folder := accountFolder{l.Text(l10n.KeyGeminiFolder), rt.geminiFolder()}
	accountToolbar(c, u, f, accountCountTitle(l, len(rows)), folder, func(m *ui.Menu) {
		if m.Item(l.Text(l10n.KeyAccountsAddOAuth)).Chosen() {
			u.startGeminiLogin(f, rt)
		}
		if m.Item(l.Text(l10n.KeyAccountsAddPaste)).Chosen() {
			f.mode = "paste"
		}
		if m.Item(l.Text(l10n.KeyAccountsAddFile)).Chosen() {
			u.previewAccountFiles(f, rt)
		}
		if m.Item(l.Text(l10n.KeyGeminiImportCurrent)).Chosen() {
			u.accountAct(f, "", func() string { return rt.importGeminiOfficial(l) })
		}
	}, func() string {
		f.selection = exportAccountSelection(rows)
		return ""
	}, func() string {
		return rt.geminiRefresh(l, gemini.RefreshTarget{Kind: gemini.RefreshAll})
	})
	accountEditor(c, u, f)
	accountSelectionEditor(c, u, f, rt)
	renderGeminiLogin(c, u, f)
	if len(rows) == 0 {
		ui.Text(c, l.Text(l10n.KeyGeminiAccountsEmpty)).FontSize(12).TextColor(c.Theme().TextMuted)
	}
	priv := u.s.Prefs().AccountPrivacyModeEnabled
	accountRows(c, len(rows), func(i int) {
		renderGeminiAccountRow(c, u, f, rt, rows[i], placeAt(i, len(rows)), view.ActiveID, priv)
	})
}

// geminiFolder holds the official Antigravity login files; "" without a store.
func (rt *liveApp) geminiFolder() string {
	if !rt.geminiReady() || rt.geminiMod.Store.Official == nil || len(rt.geminiMod.Store.Official.Files) == 0 {
		return ""
	}
	return filepath.Dir(rt.geminiMod.Store.Official.Files[0])
}

func renderGeminiAccountRow(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, place rowPlace, current string, priv bool) {
	l := u.L()
	name := maskAccount(priv, acc.ResolvedDisplayName())
	var quota *runway.GeminiAccountQuota
	if rt != nil {
		if q, ok := rt.svc.Snapshot().Gemini.AccountQuotas[acc.ID]; ok {
			quota = &q
		}
	}
	accountSettingsRow(c, l, accountIdentityView{account: acc, current: acc.ID == current, privacy: priv, provider: store.ProviderGemini, geminiQuota: quota, quota: quotaOptionsFor(u.s.Prefs(), false)}, func() {
		if IconButton(c, icRefresh, l.Text(l10n.KeyRefresh), f.busy || rt == nil).Clicked() {
			id := acc.ID
			u.accountAct(f, "", func() string {
				return rt.geminiRefresh(l, gemini.RefreshTarget{Kind: gemini.RefreshAccount, ID: id})
			})
		}
		accountRowActions(c, f.busy || rt == nil, geminiRowActions(u, f, rt, acc, place, name, acc.ID == current))
	})
}

func geminiRowActions(u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, place rowPlace, name string, current bool) []rowAction {
	l := u.L()
	out := []rowAction{makeCurrentAction(l, l10n.KeyAccountsMakeCurrent, l10n.KeyAccountsCurrent, current, func() {
		f.mode, f.editID, f.switchName = "gemini-switch", acc.ID, name
		f.restartAfterSwitch = codexRestartDefault
	})}
	out = append(out, rowAction{icon: icPencil, label: l.Text(l10n.KeyAlias), run: func() { f.mode, f.editID, f.alias = "alias", acc.ID, acc.Alias }})
	out = append(out, moveActions(l, place, func(dir int) { u.accountAct(f, "", func() string { return rt.moveGemini(acc.ID, dir) }) })...)
	title := l.Text(l10n.KeyAccountsDelete)
	if current {
		title = l.Text(l10n.KeyGeminiCurrentAccountCannotDelete)
	}
	return append(out, rowAction{icon: icTrash, label: title, disabled: current, run: func() {
		u.accountAct(f, "", func() string { return rt.deleteGemini(l, acc.ID) })
	}})
}

func (rt *liveApp) deleteGemini(l l10n.Localizer, id string) string {
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyAccountsDeleteConfirmTitle), l.Text(l10n.KeyGeminiDeleteConfirmMessage), "", l.Text(l10n.KeyAccountsDelete))
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyStatusError), err)
	}
	if !confirmed {
		return ""
	}
	return rt.removeGemini(l, id)
}
