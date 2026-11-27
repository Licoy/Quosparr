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
	geminiQuotaToggle(c, u, f)
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

// geminiQuotaToggle switches the Gemini quota reads. They use Antigravity's
// internal API, so turning them on asks first.
func geminiQuotaToggle(c *ui.Context, u *settingsUI, f *accountForm) {
	s, l := u.s, u.L()
	settingsGroup(c, "", "", nil, func(row rowFunc) {
		row(l.Text(l10n.KeyGeminiQuotaReads), l.Text(l10n.KeyGeminiQuotaReadsHint), func() {
			on, changed := toggle(c, "gemini-quota-reads", l.Text(l10n.KeyGeminiQuotaReads), s.Prefs().GeminiQuotaEnabled, s.ReadOnly() || f.busy)
			switch {
			case !changed:
			case !on:
				s.SetGeminiQuotaEnabled(false)
			default:
				u.accountAct(f, "", func() string {
					confirmed, err := confirmDialog(l, l.Text(l10n.KeyGeminiQuotaReadsConfirmTitle), l.Text(l10n.KeyGeminiQuotaReadsConfirmMessage), "", l.Text(l10n.KeyGeminiQuotaReadsConfirm))
					if err != nil {
						return accountFailure(l, l.Text(l10n.KeyStatusError), err)
					}
					if confirmed {
						u.post(func() { s.SetGeminiQuotaEnabled(true) })
					}
					return ""
				})
			}
		})
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
	// With reads off the cached reading stays but nothing is read.
	noRead := rt == nil || !u.s.Prefs().GeminiQuotaEnabled
	accountSettingsRow(c, l, accountIdentityView{account: acc, current: acc.ID == current, privacy: priv, provider: store.ProviderGemini, geminiQuota: quota, quota: quotaOptionsFor(u.s.Prefs(), false)}, func() {
		if accountRefreshButton(c, l, f, acc.ID, noRead).Clicked() {
			id := acc.ID
			u.accountRefresh(f, id, func() string {
				return rt.geminiRefresh(l, gemini.RefreshTarget{Kind: gemini.RefreshAccount, ID: id})
			})
		}
		accountRowActions(c, f.busy || rt == nil, geminiRowActions(u, f, rt, acc, place, name, acc.ID == current))
	})
}

func geminiRowActions(u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, place rowPlace, name string, current bool) []rowAction {
	l := u.L()
	out := []rowAction{makeCurrentAction(l, l10n.KeyGeminiMakeCurrent, l10n.KeyAccountsCurrent, current, func() {
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
