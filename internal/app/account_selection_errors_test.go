package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func assertSelectionReason(t *testing.T, message, expected string, private ...string) {
	t.Helper()
	if !strings.Contains(message, expected) {
		t.Fatal("selection lost the safe failure reason")
	}
	for _, value := range private {
		if strings.Contains(message, value) {
			t.Fatal("selection exposed private error data")
		}
	}
}

func TestAccountSelectionPreviewReportsReadFailureAndKeepsCancellation(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "quosparr"))
	missing := filepath.Join(home, "private-missing.json")
	selection := codexImportSelection(st, []string{missing})
	f := &accountForm{busy: true}
	u.completeAccountPreview(f, selection, nil)
	if f.busy || f.selection != nil || !u.noticeOpen {
		t.Fatal("failed preview changed editor state incorrectly")
	}
	assertSelectionReason(t, u.notice, accountErrorReason(u.L(), os.ErrNotExist), home, "private-missing")
	if strings.Contains(u.notice, tx(u.L(), xAccountBatchResult, 0, 1)) {
		t.Fatal("preview incorrectly reported imported-account counts")
	}
	invalid := filepath.Join(home, "private-invalid.json")
	if err := os.WriteFile(invalid, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	u.completeAccountPreview(f, codexImportSelection(st, []string{invalid}), nil)
	assertSelectionReason(t, u.notice, accountErrorReason(u.L(), codex.ErrInvalidCredential), home, "private-invalid")
	u.noticeOpen = false
	u.completeAccountPreview(f, nil, nil)
	if u.noticeOpen || f.selection != nil {
		t.Fatal("cancelled file selection created a notice or editor")
	}
	u.completeAccountPreview(f, nil, &os.PathError{Path: missing, Err: os.ErrPermission})
	assertSelectionReason(t, u.notice, accountErrorReason(u.L(), os.ErrPermission), home)
}

func TestAccountSelectionPreviewAndCommitPreservePartialFailure(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	home := t.TempDir()
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "quosparr"))
	valid, missing := filepath.Join(home, "valid.json"), filepath.Join(home, "private-missing.json")
	if err := os.WriteFile(valid, []byte(`{"OPENAI_API_KEY":"temporary-selection-fixture"}`), 0600); err != nil {
		t.Fatal(err)
	}
	selection := codexImportSelection(st, []string{valid, missing})
	f := &accountForm{kind: accountKindCodex, busy: true}
	u.completeAccountPreview(f, selection, nil)
	if f.selection != selection || selection.failed != 1 || len(selection.rows) != 1 {
		t.Fatal("partial preview lost valid candidates or failure count")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { accountSelectionBody(c, u, f, nil, selection) })
	}, 630, 460)
	text := strings.Join(tt.Texts(), "\n")
	assertSelectionReason(t, text, tx(u.L(), xAccountPreviewFailures, 1), home, "private-missing")
	if strings.Contains(text, u.L().Textf(l10n.KeyAccountsImportSucceeded, 1)) {
		t.Fatal("preview claimed a successful import before submission")
	}
	rt := &liveApp{codex: st, svc: runway.New(runway.Options{})}
	message, ok := rt.commitAccountSelection(u.L(), selection, selection.selectedIDs(), accountKindCodex)
	if ok {
		t.Fatal("partial import must retain its failure notice")
	}
	assertSelectionReason(t, message, tx(u.L(), xAccountBatchResult, 1, 1), home, "private-missing")
	assertSelectionReason(t, message, accountErrorReason(u.L(), os.ErrNotExist))
	idx, err := st.LoadIndex()
	if err != nil || len(idx.Accounts) != 1 {
		t.Fatal("selection did not actually import its valid account")
	}
	if _, err := os.Stat(filepath.Join(home, "codex", "auth.json")); !os.IsNotExist(err) {
		t.Fatal("selection import changed the official login")
	}
}

func TestAccountSelectionCommitShowsCredentialAndOverallFailures(t *testing.T) {
	home := t.TempDir()
	l := l10n.New(l10n.English)
	st := codex.NewStore(filepath.Join(home, "codex"), filepath.Join(home, "quosparr"))
	rt := &liveApp{codex: st, svc: runway.New(runway.Options{})}
	selection := &accountSelection{codex: []codex.ImportCandidate{
		{Preview: codex.PreviewItem{ID: "valid"}, Auth: selectionAuthFixture()},
		{Preview: codex.PreviewItem{ID: "private-invalid"}},
	}}
	message, ok := rt.commitAccountSelection(l, selection, []string{"valid", "private-invalid"}, accountKindCodex)
	if ok {
		t.Fatal("partial commit was reported as complete")
	}
	assertSelectionReason(t, message, tx(l, xAccountBatchResult, 1, 1), home, "private-invalid")
	assertSelectionReason(t, message, accountErrorReason(l, codex.ErrInvalidCredential))
	message, ok = rt.commitAccountSelection(l, selection, []string{"private-invalid"}, accountKindCodex)
	if ok {
		t.Fatal("all-failed commit was reported as complete")
	}
	assertSelectionReason(t, message, tx(l, xAccountBatchResult, 0, 1), home, "private-invalid")

}

func TestGrokAccountSelectionCommitShowsCredentialAndOverallFailures(t *testing.T) {
	home := t.TempDir()
	l := l10n.New(l10n.English)
	grokStore := grok.NewStore(filepath.Join(home, "grok"), filepath.Join(home, "grok-library"))
	mod := grok.NewModule(grokStore)
	preview, err := mod.PreviewImportFiles(context.Background(), []string{filepath.Join(home, "missing.json")})
	if err != nil {
		t.Fatal(err)
	}
	grokSelection := grokImportSelection(preview)
	assertSelectionReason(t, grokSelection.previewFailure(l), accountErrorReason(l, os.ErrNotExist), home)
	grokSelection.grok = []grok.ImportCandidate{{Preview: codex.PreviewItem{ID: "private-grok"}, CredentialData: []byte(`{}`)}}
	rt := &liveApp{grokMod: mod, grok: newGrokBridge(mod, store.DefaultPreferences(), nil)}
	message, ok := rt.commitAccountSelection(l, grokSelection, []string{"private-grok"}, accountKindGrok)
	if ok {
		t.Fatal("invalid Grok selection was reported as complete")
	}
	assertSelectionReason(t, message, tx(l, xAccountBatchResult, 0, 2), home, "private-grok")
	assertSelectionReason(t, message, accountErrorReason(l, grok.ErrAuthNoManagedCredential))
	if err := os.MkdirAll(filepath.Dir(grokStore.IndexPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(grokStore.IndexPath(), []byte(`{"version":999,"accounts":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	message, ok = rt.commitAccountSelection(l, grokSelection, []string{"private-grok"}, accountKindGrok)
	if ok {
		t.Fatal("overall index failure was reported as complete")
	}
	assertSelectionReason(t, message, tx(l, xAccountBatchResult, 0, 2), home, "private-grok")
	assertSelectionReason(t, message, accountReasonText(l, "unsupported_version"))
}

func selectionAuthFixture() *codex.Auth {
	auth := codex.APIKeyAuth("temporary-selection-commit-fixture")
	return &auth
}
