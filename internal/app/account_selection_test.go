package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

func TestAccountPreviewSelectionDoesNotWriteUntilConfirmed(t *testing.T) {
	dir := t.TempDir()
	store := codex.NewStore(filepath.Join(dir, "codex"), filepath.Join(dir, "runway"))
	paths := []string{filepath.Join(dir, "first.json"), filepath.Join(dir, "second.json")}
	for i, path := range paths {
		if err := os.WriteFile(path, []byte(`{"OPENAI_API_KEY":"temporary-key-`+string(rune('a'+i))+`"}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	selection := codexImportSelection(store, paths)
	index, err := store.LoadIndex()
	if err != nil || len(index.Accounts) != 0 || len(selection.rows) != 2 {
		t.Fatal("preview mutated account library or lost candidate")
	}
	selection.rows[1].selected = false
	batch := (&codex.Importer{Store: store}).ImportPreviewSelection(context.Background(), selection.codex, selection.selectedIDs(), false)
	if len(batch.Succeeded) != 1 || len(batch.Failures) != 0 {
		t.Fatal("selection not honored")
	}
	index, err = store.LoadIndex()
	if err != nil || len(index.Accounts) != 1 {
		t.Fatal("unselected account imported")
	}
	if _, err := os.Stat(filepath.Join(dir, "codex", "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("import changed official login")
	}
	preview := codexImportSelection(store, paths[:1])
	if len(preview.rows) != 1 || !preview.rows[0].update {
		t.Fatal("update conflict missing")
	}
}

func TestAccountBrowserLoginEntryAndCancelControl(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	rt := &liveApp{svc: runway.New(runway.Options{})}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(20).Gap(8).Children(func() { renderCodexAccounts(c, u, f, rt) })
	}, 630, 460)
	if err := tt.Click(u.L().Text(l10n.KeyAccountsAdd)); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, title := range tt.Menu() {
		if title == u.L().Text(l10n.KeyAccountsAddOAuth) {
			found = true
		}
	}
	if !found {
		t.Fatal("Codex browser login entry missing")
	}
	tt.CloseMenu()
	cancelled := false
	f.busy, f.status, f.cancel = true, u.L().Text(l10n.KeyAccountsOAuthWaiting), func() { cancelled = true }
	tt.Frame()
	if err := tt.Click(u.L().Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if !cancelled {
		t.Fatal("login cancel button not wired")
	}
	if os.Getenv("RUNWAY_UI_QA") != "" {
		exportReviewPNG(t, tt, filepath.Join("..", "..", ".build", "v1-ui-qa", "after-account-oauth-waiting.png"))
	}
}

func TestAccountExportSelectionUsesOnlyCheckedIDs(t *testing.T) {
	dir := t.TempDir()
	store := codex.NewStore(filepath.Join(dir, "codex"), filepath.Join(dir, "runway"))
	var accounts []codex.ManagedAccount
	for _, key := range []string{"temporary-export-a", "temporary-export-b"} {
		account, err := store.Upsert(codex.APIKeyAuth(key), codex.UpsertOptions{})
		if err != nil {
			t.Fatal(err)
		}
		accounts = append(accounts, account)
	}
	selection := exportAccountSelection(accounts)
	selection.rows[0].selected = false
	pack, err := (codex.Exporter{Store: store}).Export(selection.selectedIDs(), time.Now())
	if err != nil || pack.ExportedCount != 1 || pack.Pack.Accounts[0].ID != accounts[1].ID {
		t.Fatal("export ignored account selection")
	}
}

func TestAccountSelectionUIAndCancellation(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := &accountForm{selection: &accountSelection{export: true, rows: []accountChoice{{id: "a", name: "Personal", selected: true}, {id: "b", name: "Work", selected: true}}}}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(20).Gap(8).Children(func() { accountSelectionEditor(c, u, f, nil) })
	}, 630, 460)
	if err := tt.Click("Work"); err != nil {
		t.Fatal(err)
	}
	if ids := f.selection.selectedIDs(); len(ids) != 1 || ids[0] != "a" {
		t.Fatal("checkbox did not affect selection")
	}
	if err := tt.Click(u.L().Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if f.selection != nil {
		t.Fatal("cancel retained credentials")
	}
	if os.Getenv("RUNWAY_UI_QA") != "" {
		dir := filepath.Join("..", "..", ".build", "v1-ui-qa")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		f.selection = &accountSelection{export: true, rows: []accountChoice{{id: "a", name: "Personal", selected: true}, {id: "b", name: "Work", selected: false}}}
		tt.Frame()
		exportReviewPNG(t, tt, filepath.Join(dir, "after-account-export-selection.png"))
		f.selection.export = false
		tt.Frame()
		exportReviewPNG(t, tt, filepath.Join(dir, "after-account-import-selection.png"))
		f.mode, f.switchName = "switch", "Personal"
		confirmation := ui.NewTester(func(c *ui.Context) {
			applyTheme(c)
			ui.Column(c).Fill().Padding(20).Gap(8).Children(func() { accountEditor(c, u, f) })
		}, 630, 300)
		exportReviewPNG(t, confirmation, filepath.Join(dir, "after-account-switch-confirmation.png"))
	}
}

func TestCodexDesktopRestartUsesPreciseIdentity(t *testing.T) {
	for _, identity := range []struct{ bundle, name string }{{"com.openai.codex.helper", "Codex"}, {"com.example.editor", "Codex"}, {"com.openai.something.codex", "Codex"}, {"", "Codex"}} {
		if isCodexDesktop(identity.bundle, identity.name) {
			t.Fatal("helper, prefix or same-name app matched")
		}
	}
	calls := 0
	// The complete stop/write/start ordering is covered through Service in
	// runway/codex_runtime_test.go; this test retains exact desktop matching.
	fake := runningApp{bundleID: "com.openai.codex", path: "/fake/Codex.app", terminate: func() bool { calls++; return true }, stopped: func() bool { return true }}
	unrelated := runningApp{bundleID: "com.example.editor", name: "Editor", terminate: func() bool { t.Fatal("unrelated app terminated"); return false }}
	if err := runCodexRestart([]runningApp{unrelated, fake}, "", func(path string) error {
		if path != "/fake/Codex.app" {
			t.Fatal("wrong launch path")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fake.terminate = func() bool { return false }
	if err := runCodexRestart([]runningApp{fake}, "", func(string) error { t.Fatal("opened app after rejected termination"); return nil }); !errors.Is(err, errRestartCodex) {
		t.Fatal("termination failure hidden")
	}
}
