package codex

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImportAndPreviewKeepFailureCauses(t *testing.T) {
	im, _ := newImporter(t)
	missing := filepath.Join(t.TempDir(), "missing.json")
	batch := im.ImportFiles(ctxBG(), []string{missing}, false)
	if len(batch.FailureErrors) != 1 || !errors.Is(batch.FailureErrors[0], os.ErrNotExist) {
		t.Fatal("import lost missing-file cause")
	}
	valid := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(valid, []byte(pasteAuthJSON(t, "fixture-refresh-token")), 0600); err != nil {
		t.Fatal(err)
	}
	partial := im.ImportFiles(ctxBG(), []string{valid, missing}, false)
	if len(partial.Succeeded) != 1 || len(partial.Failures) != 1 || len(partial.FailureErrors) != 1 || !errors.Is(partial.FailureErrors[0], os.ErrNotExist) {
		t.Fatal("partial import lost success count or missing-file cause")
	}
	preview := im.PreviewFiles([]string{missing})
	if len(preview.FailureErrors) != 1 || !errors.Is(preview.FailureErrors[0], os.ErrNotExist) {
		t.Fatal("preview lost missing-file cause")
	}
	batch = im.ImportPreviewSelection(ctxBG(), []ImportCandidate{{Preview: PreviewItem{ID: "bad"}}}, []string{"bad"}, false)
	if len(batch.Failures) != 1 || len(batch.FailureErrors) != 1 {
		t.Fatal("selection lost credential failure cause")
	}
}

func TestExportKeepsFailureCausesAndPartialCount(t *testing.T) {
	e, accounts := exportFixture(t)
	exporter := Exporter{Store: e.store}
	result, err := exporter.Export([]string{accounts[0].ID, "missing"}, testNow)
	var notFound *AccountNotFoundError
	if err != nil || result.ExportedCount != 1 || len(result.FailureErrors) != 1 || !errors.As(result.FailureErrors[0], &notFound) {
		t.Fatal("partial export lost count or missing-account cause")
	}
	result, err = exporter.Write([]string{"missing"}, filepath.Join(t.TempDir(), "out.json"), testNow)
	if !errors.Is(err, ErrNoAccountsExported) || len(result.FailureErrors) != 1 || !errors.As(result.FailureErrors[0], &notFound) {
		t.Fatal("failed export write lost per-account cause")
	}
}
