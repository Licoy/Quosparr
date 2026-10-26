package app

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

type accountChoice struct {
	id, name, detail string
	selected, update bool
}

type accountSelection struct {
	export bool
	rows   []accountChoice
	codex  []codex.ImportCandidate
	grok   []grok.ImportCandidate
	failed int
	causes []error
}

func (s *accountSelection) selectedIDs() []string {
	var ids []string
	for _, row := range s.rows {
		if row.selected {
			ids = append(ids, row.id)
		}
	}
	return ids
}

func exportAccountSelection(accounts []codex.ManagedAccount) *accountSelection {
	s := &accountSelection{export: true}
	for _, account := range accounts {
		s.rows = append(s.rows, accountChoice{id: account.ID, name: account.ResolvedDisplayName(), detail: account.Email, selected: true})
	}
	return s
}

func codexImportSelection(store *codex.Store, paths []string) *accountSelection {
	preview := (&codex.Importer{Store: store}).PreviewFiles(paths)
	s := &accountSelection{codex: preview.Items, failed: len(preview.Failures), causes: preview.FailureErrors}
	for _, candidate := range preview.Items {
		row := candidate.Preview
		s.rows = append(s.rows, accountChoice{id: row.ID, name: row.DisplayName, detail: row.Detail, selected: true, update: row.Conflict.WillUpdate})
	}
	return s
}

func (u *settingsUI) previewAccountFiles(f *accountForm, rt *liveApp) {
	if f.busy {
		return
	}
	f.busy = true
	kind, l := f.kind, u.L()
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: l.Text(l10n.KeyAccountsAddFile), Multiple: true})
		var selection *accountSelection
		if err == nil && len(paths) > 0 {
			if kind == accountKindCodex {
				selection = codexImportSelection(rt.codex, paths)
			} else {
				preview, previewErr := rt.grokMod.PreviewImportFiles(context.Background(), paths)
				err = previewErr
				selection = grokImportSelection(preview)
			}
		}
		u.post(func() { u.completeAccountPreview(f, selection, err) })
	}()
}

func grokImportSelection(preview grok.ImportPreview) *accountSelection {
	s := &accountSelection{grok: preview.Items, failed: len(preview.Failures), causes: preview.FailureErrors}
	for _, candidate := range preview.Items {
		row := candidate.Preview
		s.rows = append(s.rows, accountChoice{id: candidate.ID(), name: row.DisplayName, detail: row.Detail, selected: true, update: row.Conflict.WillUpdate})
	}
	return s
}

func (u *settingsUI) completeAccountPreview(f *accountForm, selection *accountSelection, err error) {
	f.busy = false
	l := u.L()
	if err != nil {
		u.accountNotice(f, accountFailure(l, l.Text(l10n.KeyAccountsImportFailed), err))
		return
	}
	if selection != nil && len(selection.rows) == 0 {
		message := selection.previewFailure(l)
		if message == "" {
			message = l.Text(l10n.KeyAccountsImportNoCredentials)
		}
		u.accountNotice(f, message)
		return
	}
	f.selection = selection
}

func (s *accountSelection) previewFailure(l l10n.Localizer) string {
	if s.failed == 0 {
		return ""
	}
	return tx(l, xAccountPreviewFailures, s.failed) + ": " + accountFailureReasons(l, s.causes)
}

func accountSelectionEditor(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp) {
	s := f.selection
	if s == nil {
		return
	}
	open := true
	ui.Modal(c.Key("account-selection"), &open, func() {
		accountSelectionBody(c, u, f, rt, s)
	}).Width(520)
	if !open && !f.busy {
		f.closeEditor()
	}
}

func accountSelectionBody(c *ui.Context, u *settingsUI, f *accountForm, rt *liveApp, s *accountSelection) {
	l := u.L()
	title, hint, action := l10n.KeyAccountsImportPreviewTitle, l10n.KeyAccountsImportPreviewHint, l10n.KeyAccountsImportSelected
	if s.export {
		title, hint, action = l10n.KeyAccountsExportTitle, l10n.KeyAccountsExportHint, l10n.KeyAccountsExport
	}
	ui.Text(c, l.Text(title)).FontSize(17).FontWeight(600)
	ui.Text(c, l.Text(hint)).FontSize(11).TextColor(c.Theme().TextMuted)
	if f.editorError != "" {
		ui.Text(c, f.editorError).FontSize(12).TextColor(c.Theme().Danger)
	}
	if message := s.previewFailure(l); message != "" {
		ui.Text(c, message).FontSize(11).TextColor(c.Theme().Danger)
	}
	privacy := u.s.Prefs().AccountPrivacyModeEnabled
	ui.Scroll(c.Key("account-selection-rows")).MaxHeight(320).Gap(8).Children(func() {
		for i := range s.rows {
			row := &s.rows[i]
			ui.Checkbox(c.Key("selection-"+row.id), &row.selected, maskAccount(privacy, row.name)).Disabled(f.busy)
			if row.detail != "" {
				ui.Text(c, maskAccount(privacy, row.detail)).FontSize(11).TextColor(c.Theme().TextMuted)
			}
			if !s.export {
				conflict := l10n.KeyAccountsImportWillAdd
				if row.update {
					conflict = l10n.KeyAccountsImportWillUpdate
				}
				ui.Text(c, l.Text(conflict)).FontSize(11).TextColor(c.Theme().TextMuted)
			}
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		if f.busy {
			ui.Spinner(c).Size(14, 14)
		}
		ui.Spacer(c).Grow(1)
		if PillButton(c, nil, l.Text(l10n.KeyCancel), f.busy).Clicked() {
			f.closeEditor()
		}
		if PillButton(c, nil, l.Text(action), f.busy || len(s.selectedIDs()) == 0).Clicked() {
			ids, kind := s.selectedIDs(), f.kind
			u.accountEditAct(f, func() (string, bool) {
				if rt == nil {
					return tx(l, xDetailUnavailable), false
				}
				return rt.commitAccountSelection(l, s, ids, kind)
			})
		}
	})
}

func (rt *liveApp) commitAccountSelection(l l10n.Localizer, s *accountSelection, ids []string, kind int) (string, bool) {
	if s.export {
		if kind == accountKindGrok {
			return rt.exportGrok(l, ids)
		}
		return rt.exportCodex(l, ids)
	}
	count, failed := 0, s.failed
	causes := append([]error(nil), s.causes...)
	if kind == accountKindCodex {
		batch := (&codex.Importer{Store: rt.codex}).ImportPreviewSelection(context.Background(), s.codex, ids, false)
		count, failed = len(batch.Succeeded), failed+len(batch.Failures)
		causes = append(causes, batch.FailureErrors...)
		rt.svc.ReloadAccounts()
	} else {
		batch, _, err := rt.grokMod.ImportPreviewSelection(context.Background(), s.grok, ids)
		count, failed = batch.SuccessCount(), failed+batch.FailureCount()
		causes = append(causes, batch.FailureErrors...)
		if err != nil {
			failed++
			causes = append(causes, err)
		}
		rt.grok.reload(true)
	}
	if message := accountBatchFailure(l, count, failed, causes); message != "" {
		return message, false
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, count), count > 0
}
