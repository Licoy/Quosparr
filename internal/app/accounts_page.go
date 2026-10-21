package app

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// Account form kinds index the provider tabs, in store.Providers() order.
const (
	accountKindCodex = iota
	accountKindClaude
	accountKindGrok
	accountKindCursor
)

// accountsPage is wired from settings_view.go.
func renderAccountsPage(c *ui.Context, u *settingsUI) {
	l := u.L()
	f := accountFormOf(u)
	accountProviderHeader(c, u, f)
	rt := currentRuntime()
	if rt == nil {
		ui.Text(c, l.Text(l10n.KeyNotLoaded)).FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	switch f.kind {
	case accountKindCodex:
		renderCodexAccounts(c, u, f, rt)
	case accountKindGrok:
		renderGrokAccounts(c, u, f, rt)
	case accountKindCursor:
		renderCursorAccount(c, u, rt.svc.Snapshot().Cursor, func() { rt.svc.RefreshCursor(true) })
	default:
		renderClaudeAccounts(c, u, f, rt)
	}
}

func renderClaudeAccounts(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp) {
	l := u.L()
	view := rt.svc.Snapshot().Claude.Accounts
	panelAccountStatus(c, l, view)
	rows := view.Sidebar
	folder := accountFolder{l.Text(l10n.KeyProviderClaude), rt.claudeFolder()}
	accountToolbar(c, u, f, accountCountTitle(l, len(rows)), folder, func(m *ui.Menu) {
		if m.Item(l.Text(l10n.KeyClaudeLoginCLI)).Chosen() {
			u.accountNotice(f, rt.claudeLoginHint(l))
		}
		if m.Item(l.Text(l10n.KeyClaudePasteHint)).Chosen() {
			f.mode = "paste"
		}
		if m.Item(l.Text(l10n.KeyGrokAccountsAddFile)).Chosen() {
			u.previewClaudeFile(f, rt)
		}
		if m.Item(l.Text(l10n.KeyClaudeImportCurrent)).Chosen() {
			u.accountAct(f, "", func() string { return rt.importClaudeOfficial(l) })
		}
		if m.Item(tx(l, xAPIAdd)).Chosen() {
			f.mode = modeClaudeAPIAdd
		}
	}, func() string {
		f.selection = exportAccountSelection(rows)
		return ""
	}, func() string {
		return rt.refreshClaudeAll(context.Background())
	})
	accountEditor(c, u, f)
	if len(rows) == 0 {
		ui.Text(c, l.Text(l10n.KeyClaudeAccountsEmpty)).FontSize(12).TextColor(c.Theme().TextMuted)
	}
	priv := u.s.Prefs().AccountPrivacyModeEnabled
	accountRows(c, len(rows), func(i int) {
		renderClaudeAccountRow(c, u, f, rt, rows[i], view.ActiveID, priv)
	})
}

func (rt *liveApp) claudeFolder() string {
	if rt == nil || rt.claudeMod == nil || rt.claudeMod.Store == nil {
		return ""
	}
	return rt.claudeMod.Store.ConfigDir()
}

func renderClaudeAccountRow(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, current string, priv bool) {
	l := u.L()
	name := maskAccount(priv, acc.ResolvedDisplayName())
	var quota *runway.ClaudeAccountQuota
	if rt != nil {
		if q, ok := rt.svc.Snapshot().Claude.AccountQuotas[acc.ID]; ok {
			quota = &q
		}
	}
	ui.Row(c).Gap(10).Padding(10, 0).AlignItems(ui.Center).Children(func() {
		ProviderIcon(c, store.ProviderClaude, 20)
		accountSettingsIdentity(c, l, accountIdentityView{account: acc, current: acc.ID == current, privacy: priv, provider: store.ProviderClaude, claudeQuota: quota})
		ui.MenuButton(c, l.Text(l10n.KeyAccountsActions), func(m *ui.Menu) {
			if m.Item(l.Text(l10n.KeyAccountsMakeCurrent)).Disabled(acc.ID == current).Chosen() {
				f.mode, f.editID, f.switchName, f.switchRelay = "claude-switch", acc.ID, name, acc.IsRelay()
			}
			if acc.IsRelay() && m.Item(tx(l, xAPIEditMenu)).Chosen() {
				f.openClaudeAPIEditor(rt, acc.ID)
			}
			if m.Item(l.Text(l10n.KeyAccountsMoveUp)).Chosen() {
				u.accountAct(f, "", func() string { return rt.moveClaude(acc.ID, -1) })
			}
			if m.Item(l.Text(l10n.KeyAccountsMoveDown)).Chosen() {
				u.accountAct(f, "", func() string { return rt.moveClaude(acc.ID, 1) })
			}
			if m.Item(l.Text(l10n.KeyAccountsDelete)).Disabled(acc.ID == current).Chosen() {
				u.accountAct(f, "", func() string { return rt.deleteClaude(l, acc.ID) })
			}
		}).Disabled(f.busy || rt == nil)
	})
}

func (rt *liveApp) moveClaude(id string, delta int) string {
	if rt.claudeMod == nil || rt.claudeMod.Store == nil {
		return ""
	}
	idx, err := rt.claudeMod.Store.LoadIndex()
	if err != nil {
		return err.Error()
	}
	rows := idx.OrderedAccounts()
	ids := make([]string, len(rows))
	at := -1
	for i, row := range rows {
		ids[i] = row.ID
		if row.ID == id {
			at = i
		}
	}
	next := at + delta
	if at < 0 || next < 0 || next >= len(ids) {
		return ""
	}
	ids[at], ids[next] = ids[next], ids[at]
	return rt.reorderClaude(ids)
}

func (u *settingsUI) previewClaudeFile(f *accountForm, rt *liveApp) {
	l := u.L()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: l.Text(l10n.KeyGrokAccountsAddFile)})
	if err != nil || len(paths) == 0 {
		return
	}
	u.accountAct(f, "", func() string {
		msg, _ := rt.importClaudeFile(context.Background(), l, paths[0])
		return msg
	})
}

func accountProviderHeader(c *ui.Context, u *settingsUI, f *accountForm) {
	l := u.L()
	ui.Row(c).Children(func() {
		providers := store.Providers()
		if providerTabsOf(c.Key("accounts-provider"), providers, &f.kind, f.busy, providerLabels(l, providers)...) {
			f.mode, f.paste = "", ""
			f.selection, f.api = nil, apiDraft{}
		}
	})
}

func accountCountTitle(l l10n.Localizer, count int) string {
	key := xAccountCount
	if count == 1 {
		key = xAccountCountOne
	}
	return tx(l, key, count)
}

type accountForm struct {
	kind               int
	mode, editID       string
	paste, alias       string
	note, status       string
	editorError        string
	busy               bool
	cancel             context.CancelFunc
	selection          *accountSelection
	restartAfterSwitch bool
	switchName         string
	switchRelay        bool
	api                apiDraft
}

func accountFormOf(u *settingsUI) *accountForm {
	if u.accounts == nil {
		u.accounts = &accountForm{}
	}
	return u.accounts
}

func (u *settingsUI) accountAct(f *accountForm, waiting string, fn func() string) {
	if f.busy {
		return
	}
	f.busy = true
	f.status = waiting
	go func() {
		msg := fn()
		u.post(func() {
			f.busy = false
			f.cancel = nil
			f.status = ""
			u.accountNotice(f, msg)
		})
	}()
}

// accountNotice shows the result of an account action in a dialog titled
// with the provider of the tab, so it never lingers under another tab. A
// cancelled login needs no answer.
func (u *settingsUI) accountNotice(f *accountForm, msg string) {
	l := u.L()
	if msg == "" || msg == l.Text(l10n.KeyAccountsOAuthCancelled) {
		return
	}
	providers := store.Providers()
	title := l.Text(providerKey(providers[min(max(f.kind, 0), len(providers)-1)]))
	u.showNotice(title, msg)
}

func renderCodexAccounts(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp) {
	l := u.L()
	view := rt.svc.Snapshot().Accounts
	panelAccountStatus(c, l, view)
	rows := view.Sidebar
	folder := accountFolder{l.Text(l10n.KeyCodexFolder), rt.codexFolder()}
	accountToolbar(c, u, f, accountCountTitle(l, len(rows)), folder, func(m *ui.Menu) {
		if m.Item(l.Text(l10n.KeyAccountsAddOAuth)).Chosen() {
			u.startCodexLogin(f, rt)
		}
		if m.Item(l.Text(l10n.KeyAccountsAddLocal)).Chosen() {
			u.accountAct(f, "", func() string { return rt.importCodexOfficial(l) })
		}
		if m.Item(l.Text(l10n.KeyAccountsAddPaste)).Chosen() {
			f.mode = "paste"
		}
		if m.Item(l.Text(l10n.KeyAccountsAddFile)).Chosen() {
			u.previewAccountFiles(f, rt)
		}
		if m.Item(tx(l, xAPIAdd)).Chosen() {
			f.mode = modeCodexAPIAdd
		}
	}, func() string {
		f.selection = exportAccountSelection(rows)
		return ""
	}, func() string {
		<-rt.svc.RefreshAllAccountQuotas()
		return ""
	})
	accountEditor(c, u, f)
	accountSelectionEditor(c, u, f, rt)
	if len(rows) == 0 {
		ui.Text(c, l.Text(l10n.KeyAccountsEmpty)).FontSize(12).TextColor(c.Theme().TextMuted)
	}
	priv := u.s.Prefs().AccountPrivacyModeEnabled
	accountRows(c, len(rows), func(i int) {
		acc := rows[i]
		renderCodexSettingsRow(c, u, f, rt, acc, view.ActiveID, priv)
	})
}

func renderGrokAccounts(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp) {
	l := u.L()
	view := rt.svc.Snapshot().Grok.Accounts
	panelAccountStatus(c, l, view)
	rows := view.Sidebar
	folder := accountFolder{l.Text(l10n.KeyGrokFolder), rt.grokFolder()}
	accountToolbar(c, u, f, accountCountTitle(l, len(rows)), folder, func(m *ui.Menu) {
		// grok.Module has no API-key add; a key alone is not a managed login.
		if m.Item(l.Text(l10n.KeyGrokAccountsAddOAuth)).Chosen() {
			u.startGrokLogin(f, rt)
		}
		if m.Item(l.Text(l10n.KeyGrokAccountsAddPaste)).Chosen() {
			f.mode = "paste"
		}
		if m.Item(l.Text(l10n.KeyGrokAccountsAddFile)).Chosen() {
			u.previewAccountFiles(f, rt)
		}
		if m.Item(l.Text(l10n.KeyGrokAccountsImportOfficial)).Chosen() {
			u.accountAct(f, "", func() string { return rt.importGrokOfficial(l) })
		}
	}, func() string {
		f.selection = exportAccountSelection(rows)
		return ""
	}, func() string {
		report := rt.grokMod.Refresh(context.Background(), grok.RefreshTarget{Kind: grok.RefreshAll})
		rt.grok.reload(true)
		for _, outcome := range report.Outcomes {
			if outcome.Err != nil {
				return accountError(rt)
			}
		}
		return ""
	})
	accountEditor(c, u, f)
	accountSelectionEditor(c, u, f, rt)
	if len(rows) == 0 {
		ui.Text(c, l.Text(l10n.KeyGrokAccountsEmpty)).FontSize(12).TextColor(c.Theme().TextMuted)
	}
	priv := u.s.Prefs().AccountPrivacyModeEnabled
	accountRows(c, len(rows), func(i int) {
		acc := rows[i]
		renderGrokAccountRow(c, u, f, rt, acc, view.ActiveID, priv)
	})
}

// accountFolder is the official CLI directory of a provider, which the
// toolbar opens in the file manager.
type accountFolder struct {
	label, path string
}

// codexFolder is CODEX_HOME (~/.codex by default); "" without a store.
func (rt *liveApp) codexFolder() string {
	if rt.codex == nil {
		return ""
	}
	return filepath.Dir(rt.codex.OfficialAuthPath())
}

// grokFolder is GROK_HOME (~/.grok by default); "" without a store.
func (rt *liveApp) grokFolder() string {
	if rt.grokMod == nil || rt.grokMod.Store == nil {
		return ""
	}
	return rt.grokMod.Store.OfficialHome()
}

func accountToolbar(c *ui.Context, u *settingsUI, f *accountForm, title string, folder accountFolder, menu func(*ui.Menu), export, refresh func() string) {
	l := u.L()
	ui.Row(c).Gap(8).Padding(0, 0, 0, 12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(600).Grow(1).Basis(0).SingleLine()
		if f.busy {
			ui.Spinner(c).Size(14, 14)
			if f.cancel != nil && PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
				f.cancel()
			}
		}
		ui.MenuButton(c, l.Text(l10n.KeyAccountsAdd), menu).Disabled(f.busy)
		if IconButton(c, icRefresh, l.Text(l10n.KeyAccountsRefreshAll), f.busy).Clicked() {
			u.accountAct(f, "", refresh)
		}
		if IconButton(c, icExport, l.Text(l10n.KeyAccountsExport), f.busy).Clicked() {
			u.accountNotice(f, export())
		}
		open := u.opts.Actions.OpenFolder
		if IconButton(c, icFolder, folder.label, open == nil || folder.path == "").Clicked() {
			open(folder.path)
		}
	})
	// Only progress stays inline; results open a dialog.
	if f.busy && f.status != "" {
		ui.Text(c, f.status).FontSize(12).TextColor(c.Theme().TextMuted)
	}
}

// accountRows is the box holding the account rows, with hairlines between.
func accountRows(c *ui.Context, n int, row func(i int)) {
	if n == 0 {
		return
	}
	Panel(c).Children(func() {
		for i := range n {
			if i > 0 {
				ui.Box(c).Height(1).Background(pal(c.Theme()).Line)
			}
			row(i)
		}
	})
}

func renderCodexRow(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, priv bool) {
	activeID := ""
	if rt != nil {
		activeID = rt.svc.Snapshot().Accounts.ActiveID
	}
	renderCodexSettingsRow(c, u, f, rt, acc, activeID, priv)
}

func renderCodexSettingsRow(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, activeID string, priv bool) {
	l := u.L()
	name := maskAccount(priv, acc.ResolvedDisplayName())
	current := acc.ID == activeID
	ui.Row(c).Gap(10).Padding(10, 0).AlignItems(ui.Center).Children(func() {
		ProviderIcon(c, store.ProviderCodex, 20)
		accountSettingsIdentity(c, l, accountIdentityView{account: acc, current: current, privacy: priv, provider: store.ProviderCodex})
		if IconButton(c, icRefresh, l.Text(l10n.KeyRefresh), f.busy || rt == nil).Clicked() {
			id := acc.ID
			u.accountAct(f, "", func() string { <-rt.svc.RefreshAccountQuota(id); return "" })
		}
		ui.MenuButton(c, l.Text(l10n.KeyAccountsActions), func(m *ui.Menu) {
			if m.Item(l.Text(l10n.KeyAccountsMakeCurrent)).Disabled(current).Chosen() {
				f.mode, f.editID, f.switchName, f.switchRelay = "switch", acc.ID, name, acc.IsRelay()
				f.restartAfterSwitch = codexRestartSupported
			}
			if acc.AuthMode == codex.AuthModeAPIKey && m.Item(tx(l, xAPIEditMenu)).Chosen() {
				f.openCodexAPIEditor(rt, acc)
			}
			if m.Item(l.Text(l10n.KeyAlias)).Chosen() {
				f.mode, f.editID, f.alias = "alias", acc.ID, acc.Alias
			}
			if m.Item(l.Text(l10n.KeyAccountsNote)).Chosen() {
				f.mode, f.editID, f.note = "note", acc.ID, acc.Note
			}
			if m.Item(l.Text(l10n.KeyAccountsMoveUp)).Chosen() {
				u.accountAct(f, "", func() string { return rt.moveCodex(acc.ID, -1) })
			}
			if m.Item(l.Text(l10n.KeyAccountsMoveDown)).Chosen() {
				u.accountAct(f, "", func() string { return rt.moveCodex(acc.ID, 1) })
			}
			if m.Item(l.Text(l10n.KeyAccountsDelete)).Chosen() {
				u.accountAct(f, "", func() string { return rt.deleteCodex(l, acc.ID) })
			}
		}).Disabled(f.busy || rt == nil)
	})
}

func renderGrokAccountRow(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp, acc codex.ManagedAccount, current string, priv bool) {
	l := u.L()
	name := maskAccount(priv, acc.ResolvedDisplayName())
	ui.Row(c).Gap(10).Padding(10, 0).AlignItems(ui.Center).Children(func() {
		ProviderIcon(c, store.ProviderGrok, 20)
		var quota *runway.GrokAccountQuota
		if rt != nil {
			if q, ok := rt.svc.Snapshot().Grok.AccountQuotas[acc.ID]; ok {
				quota = &q
			}
		}
		accountSettingsIdentity(c, l, accountIdentityView{account: acc, current: acc.ID == current, privacy: priv, provider: store.ProviderGrok, grokQuota: quota})
		if IconButton(c, icRefresh, l.Text(l10n.KeyRefresh), f.busy || rt == nil).Clicked() {
			u.accountAct(f, "", func() string { return rt.refreshGrok(acc.ID) })
		}
		ui.MenuButton(c, l.Text(l10n.KeyAccountsActions), func(m *ui.Menu) {
			if m.Item(l.Text(l10n.KeyGrokAccountsMakeCurrent)).Disabled(acc.ID == current).Chosen() {
				u.accountAct(f, "", func() string { return rt.switchGrok(l, acc.ID, name) })
			}
			if m.Item(l.Text(l10n.KeyAlias)).Chosen() {
				f.mode, f.editID, f.alias = "alias", acc.ID, acc.Alias
			}
			if m.Item(l.Text(l10n.KeyAccountsMoveUp)).Chosen() {
				u.accountAct(f, "", func() string { return rt.moveGrok(acc.ID, -1) })
			}
			if m.Item(l.Text(l10n.KeyAccountsMoveDown)).Chosen() {
				u.accountAct(f, "", func() string { return rt.moveGrok(acc.ID, 1) })
			}
			if m.Item(l.Text(l10n.KeyGrokAccountsRemove)).Disabled(acc.ID == current).Chosen() {
				u.accountAct(f, "", func() string { return rt.deleteGrok(l, acc.ID) })
			}
		}).Disabled(f.busy || rt == nil)
	})
}

func maskAccount(on bool, value string) string {
	if !on || value == "" {
		return value
	}
	if strings.Contains(value, "@") && strings.ContainsAny(value, " \t") {
		return codex.MaskEmailsIn(value)
	}
	return codex.MaskAccountText(value)
}

func sortedCodex(idx codex.AccountIndex) []codex.ManagedAccount {
	out := append([]codex.ManagedAccount(nil), idx.Accounts...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortIndex != out[j].SortIndex {
			return out[i].SortIndex < out[j].SortIndex
		}
		return strings.ToLower(out[i].ResolvedDisplayName()) < strings.ToLower(out[j].ResolvedDisplayName())
	})
	return out
}

func accountIDs[T any](rows []T, id func(T) string) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = id(row)
	}
	return out
}

func moveIDs(ids []string, id string, dir int) []string {
	i := slices.Index(ids, id)
	j := i + dir
	if i < 0 || j < 0 || j >= len(ids) {
		return nil
	}
	out := slices.Clone(ids)
	out[i], out[j] = out[j], out[i]
	return out
}

func confirmDialog(l l10n.Localizer, title, message, detail, action string) (bool, error) {
	res, err := mygo.Dialog.Message(mygo.MessageOptions{
		Type: mygo.MessageQuestion, Title: title, Message: message, Detail: detail,
		Buttons: []string{l.Text(l10n.KeyCancel), action}, DefaultButton: 1, CancelButton: 0,
	})
	return err == nil && res.Button == 1, err
}

func (rt *liveApp) importCodexOfficial(l l10n.Localizer) string {
	if _, err := (&codex.Importer{Store: rt.codex}).ImportOfficial(false); err != nil {
		if key := runway.CredentialModeErrorKey(err); key != "" {
			return l.Text(key)
		}
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	rt.svc.ReloadAccounts()
	return l.Textf(l10n.KeyAccountsImportSucceeded, 1)
}

func (rt *liveApp) pasteCodex(l l10n.Localizer, text string) (string, bool) {
	batch := (&codex.Importer{Store: rt.codex}).ImportPastedText(context.Background(), text, false)
	rt.svc.ReloadAccounts()
	if len(batch.Succeeded) == 0 {
		return l.Text(l10n.KeyAccountsImportNoCredentials), false
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, len(batch.Succeeded)), true
}

func (rt *liveApp) importCodexFiles(l l10n.Localizer) string {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: l.Text(l10n.KeyAccountsAddFile), Multiple: true})
	if err != nil || len(paths) == 0 {
		return ""
	}
	batch := (&codex.Importer{Store: rt.codex}).ImportFiles(context.Background(), paths, false)
	rt.svc.ReloadAccounts()
	if len(batch.Succeeded) == 0 {
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, len(batch.Succeeded))
}

func (rt *liveApp) exportCodex(l l10n.Localizer, ids []string) (string, bool) {
	if len(ids) == 0 {
		return l.Text(l10n.KeyAccountsExportEmpty), false
	}
	path, pickErr := pickExport(l)
	if pickErr != nil {
		return l.Text(l10n.KeyAccountsExportFailed), false
	}
	if path == "" {
		return "", false
	}
	res, err := (codex.Exporter{Store: rt.codex, AppVersion: rt.version}).Write(ids, path, time.Now())
	if err != nil {
		return l.Text(l10n.KeyAccountsExportFailed), false
	}
	return l.Textf(l10n.KeyAccountsExportSucceeded, res.ExportedCount), true
}

func (rt *liveApp) setCodexMeta(l l10n.Localizer, id string, alias, note *string) string {
	idx, err := rt.codex.LoadIndex()
	acc, ok := idx.Account(id)
	if err != nil || !ok {
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	if alias != nil {
		acc.Alias = *alias
	}
	if note != nil {
		acc.Note = *note
	}
	if err := rt.codex.UpdateMetadata(acc); err != nil {
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	rt.svc.ReloadAccounts()
	return ""
}

func (rt *liveApp) moveCodex(id string, dir int) string {
	idx, err := rt.codex.LoadIndex()
	if err != nil {
		return accountError(rt)
	}
	next := moveIDs(accountIDs(sortedCodex(idx), func(a codex.ManagedAccount) string { return a.ID }), id, dir)
	if next == nil {
		return ""
	}
	if err := rt.codex.Reorder(next); err != nil {
		return accountError(rt)
	}
	rt.svc.ReloadAccounts()
	return ""
}

func (rt *liveApp) switchCodex(l l10n.Localizer, id, name string) string {
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyAccountsSwitchConfirmTitle), l.Textf(l10n.KeyAccountsSwitchConfirmMessage, name), "", l.Text(l10n.KeyAccountsMakeCurrent))
	if err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	if !confirmed {
		return ""
	}
	if err := rt.svc.SwitchAccount(context.Background(), id); err != nil {
		return l.Text(l10n.KeyAccountsSwitchFailed)
	}
	return ""
}

func (rt *liveApp) deleteCodex(l l10n.Localizer, id string) string {
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyAccountsDeleteConfirmTitle), l.Text(l10n.KeyAccountsDeleteConfirmMessage), "", l.Text(l10n.KeyAccountsDelete))
	if err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	if !confirmed {
		return ""
	}
	if err := rt.codex.DeleteAccount(id); err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	rt.svc.ReloadAccounts()
	return ""
}

func (rt *liveApp) loginGrok(l l10n.Localizer) string {
	return rt.loginGrokContext(l, context.Background())
}

func (rt *liveApp) loginGrokContext(l l10n.Localizer, ctx context.Context) string {
	_, err := rt.grokMod.Apply(ctx, grok.Command{Kind: grok.CommandLoginOAuth})
	rt.grok.reload(true)
	if errors.Is(err, grok.ErrLoginCancelled) || errors.Is(err, context.Canceled) {
		return l.Text(l10n.KeyAccountsOAuthCancelled)
	}
	if err != nil {
		return l.Text(l10n.KeyAccountsOAuthFailed)
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, 1)
}

func (rt *liveApp) importGrokFiles(l l10n.Localizer) string {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: l.Text(l10n.KeyGrokAccountsAddFile), Multiple: true})
	if err != nil || len(paths) == 0 {
		return ""
	}
	preview, err := rt.grokMod.PreviewImportFiles(context.Background(), paths)
	if err != nil || len(preview.Items) == 0 {
		return l.Text(l10n.KeyGrokAccountsImportNoCredentials)
	}
	ids := accountIDs(preview.Items, func(c grok.ImportCandidate) string { return c.ID() })
	batch, _, err := rt.grokMod.ImportPreviewSelection(context.Background(), preview.Items, ids)
	rt.grok.reload(true)
	if err != nil || batch.SuccessCount() == 0 {
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, batch.SuccessCount())
}

func (rt *liveApp) importGrokOfficial(l l10n.Localizer) string {
	_, err := rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandImportOfficial})
	rt.grok.reload(true)
	if err != nil {
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, 1)
}

func (rt *liveApp) exportGrok(l l10n.Localizer, ids []string) (string, bool) {
	if len(ids) == 0 {
		return l.Text(l10n.KeyAccountsExportEmpty), false
	}
	path, pickErr := pickExport(l)
	if pickErr != nil {
		return l.Text(l10n.KeyAccountsExportFailed), false
	}
	if path == "" {
		return "", false
	}
	res, err := rt.grokMod.ExportAccounts(ids)
	if err != nil {
		return l.Text(l10n.KeyAccountsExportFailed), false
	}
	if err := codex.WriteTransferPack(path, res.Pack); err != nil {
		return l.Text(l10n.KeyAccountsExportFailed), false
	}
	return l.Textf(l10n.KeyAccountsExportSucceeded, res.ExportedCount), true
}

func (rt *liveApp) setGrokAlias(l l10n.Localizer, id, alias string) string {
	_, err := rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandSetAlias, ID: id, Alias: alias})
	rt.grok.reload(true)
	if err != nil {
		return l.Text(l10n.KeyAccountsImportFailed)
	}
	return ""
}

func (rt *liveApp) moveGrok(id string, dir int) string {
	idx, err := rt.grokMod.Store.LoadIndex()
	if err != nil {
		return accountError(rt)
	}
	next := moveIDs(accountIDs(idx.OrderedAccounts(), func(a grok.ManagedAccount) string { return a.ID }), id, dir)
	if next == nil {
		return ""
	}
	if _, err := rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandReorder, IDs: next}); err != nil {
		return accountError(rt)
	}
	rt.grok.reload(true)
	return ""
}

func (rt *liveApp) switchGrok(l l10n.Localizer, id, name string) string {
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyAccountsSwitchConfirmTitle), l.Text(l10n.KeyGrokAccountsMakeCurrent), name, l.Text(l10n.KeyGrokAccountsMakeCurrent))
	if err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	if !confirmed {
		return ""
	}
	_, err = rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandMakeCurrent, ID: id})
	var running *grok.ProcessesRunningError
	if errors.As(err, &running) {
		confirmed, confirmErr := confirmDialog(l, l.Text(l10n.KeyAccountsForceCurrent), l.Text(l10n.KeyGrokAccountsMakeCurrent), name, l.Text(l10n.KeyAccountsForceCurrent))
		if confirmErr != nil {
			return l.Text(l10n.KeyStatusError)
		}
		if !confirmed {
			return ""
		}
		_, err = rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandMakeCurrent, ID: id, AllowWhileRunning: true})
	}
	rt.grok.reload(true)
	if err != nil {
		return l.Text(l10n.KeyAccountsSwitchFailed)
	}
	return ""
}

func (rt *liveApp) refreshGrok(id string) string {
	report := rt.grokMod.Refresh(context.Background(), grok.RefreshTarget{Kind: grok.RefreshAccount, ID: id})
	rt.grok.reload(true)
	for _, outcome := range report.Outcomes {
		if outcome.Err != nil {
			return accountError(rt)
		}
	}
	return ""
}

func (rt *liveApp) deleteClaude(l l10n.Localizer, id string) string {
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyAccountsDeleteConfirmTitle), l.Text(l10n.KeyClaudeDeleteConfirmMessage), "", l.Text(l10n.KeyAccountsDelete))
	if err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	if !confirmed {
		return ""
	}
	return rt.removeClaude(l, id)
}

func (rt *liveApp) deleteGrok(l l10n.Localizer, id string) string {
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyGrokAccountsDeleteConfirmTitle), l.Text(l10n.KeyGrokAccountsDeleteConfirmMessage), "", l.Text(l10n.KeyGrokAccountsRemove))
	if err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	if !confirmed {
		return ""
	}
	_, err = rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandRemove, ID: id})
	rt.grok.reload(true)
	var current *grok.CurrentAccountCannotBeRemovedError
	if errors.As(err, &current) {
		return l.Text(l10n.KeyGrokAccountsCurrent)
	}
	if err != nil {
		return l.Text(l10n.KeyStatusError)
	}
	return ""
}

func pickExport(l l10n.Localizer) (string, error) {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Title: l.Text(l10n.KeyAccountsExportTitle), Message: l.Text(l10n.KeyAccountsExportWarning),
		DefaultPath: "accounts.json",
	})
	return path, err
}

func pasteAccount(l l10n.Localizer, kind int, text string) (string, bool) {
	rt := currentRuntime()
	if rt == nil {
		return tx(l, xDetailUnavailable), false
	}
	if kind == accountKindGrok {
		return rt.grokPasteText(l, text)
	}
	if kind == accountKindClaude {
		return rt.claudePasteText(context.Background(), l, text)
	}
	return rt.pasteCodex(l, text)
}

func saveAccountMeta(l l10n.Localizer, kind int, mode, id, alias, note string) string {
	rt := currentRuntime()
	if rt == nil {
		return tx(l, xDetailUnavailable)
	}
	if kind == accountKindGrok {
		return rt.setGrokAlias(l, id, alias)
	}
	if mode == "note" {
		return rt.setCodexMeta(l, id, nil, &note)
	}
	return rt.setCodexMeta(l, id, &alias, nil)
}

func accountError(rt *liveApp) string {
	return l10n.New(rt.svc.Snapshot().Language).Text(l10n.KeyStatusError)
}
