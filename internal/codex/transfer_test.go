package codex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func exportFixture(t *testing.T) (testEnv, []ManagedAccount) {
	e := newEnv(t)
	a := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "acct-a", email: "a@example.com", refresh: "refresh-export-a-credential"}), true)
	b := upsert(t, e.store, sampleAuth(t, sampleOpt{accountID: "acct-b", email: "b@example.com", refresh: "refresh-export-b-credential"}), false)
	api := must(e.store.Upsert(APIKeyAuth("sk-test-key-1234567890"), UpsertOptions{Now: testNow}))
	alias := "Work / main"
	a.Alias = alias
	noErr(t, e.store.UpdateMetadata(a))
	return e, []ManagedAccount{a, b, api}
}

func TestExportRoundTripsThroughImport(t *testing.T) {
	src, accounts := exportFixture(t)
	exp := Exporter{Store: src.store, AppVersion: "9.9.9"}
	path := filepath.Join(t.TempDir(), "pack.json")
	ids := []string{accounts[0].ID, accounts[1].ID, accounts[2].ID}
	res := must(exp.Write(ids, path, testNow))
	if res.ExportedCount != 3 || len(res.Failures) != 0 {
		t.Fatalf("result: %+v", res)
	}

	dst := newEnv(t)
	im := &Importer{Store: dst.store, Now: func() time.Time { return testNow }}
	preview := im.PreviewFiles([]string{path})
	if !preview.SourceIsPack || len(preview.Items) != 3 || len(preview.Failures) != 0 {
		t.Fatalf("preview: %+v", preview)
	}
	for _, item := range preview.Items {
		if item.Preview.Conflict.WillUpdate {
			t.Errorf("empty library should have no conflicts: %+v", item.Preview)
		}
	}
	var selected []string
	for _, item := range preview.Items {
		selected = append(selected, item.ID())
	}
	batch := im.ImportPreviewSelection(ctxBG(), preview.Items, selected, true)
	if len(batch.Succeeded) != 3 || len(batch.Failures) != 0 {
		t.Fatalf("batch: %+v", batch)
	}
	for i, id := range ids {
		want := must(src.store.LoadCredential(id))
		got := must(dst.store.LoadCredential(batch.Succeeded[i].ID))
		if batch.Succeeded[i].ID != id || !got.Equal(want) {
			t.Errorf("account %d did not survive the round trip", i)
		}
	}
	first, _ := must(dst.store.LoadIndex()).Account(accounts[0].ID)
	if first.Alias != "Work / main" || must(dst.store.LoadIndex()).ActiveAccountID != accounts[0].ID {
		t.Errorf("alias or active account lost: %+v", first)
	}

	// Importing the same pack again is an update, not a duplicate.
	again := im.PreviewFiles([]string{path})
	for _, item := range again.Items {
		if !item.Preview.Conflict.WillUpdate || item.Preview.Conflict.ExistingDisplayName == "" {
			t.Errorf("expected an update conflict: %+v", item.Preview)
		}
	}
}

func TestExportFileIsOwnerOnly(t *testing.T) {
	src, accounts := exportFixture(t)
	path := filepath.Join(t.TempDir(), "pack.json")
	must(Exporter{Store: src.store}.Write([]string{accounts[0].ID}, path, testNow))
	if runtime.GOOS == "windows" {
		return
	}
	if mode := must(os.Stat(path)).Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o", mode)
	}
	// Overwriting an existing, looser file tightens it.
	noErr(t, os.Chmod(path, 0o644))
	must(Exporter{Store: src.store}.Write([]string{accounts[0].ID}, path, testNow))
	if mode := must(os.Stat(path)).Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode after overwrite = %o", mode)
	}
}

func TestExportErrorsAndPartialSelection(t *testing.T) {
	src, accounts := exportFixture(t)
	exp := Exporter{Store: src.store}
	if _, err := exp.Export(nil, testNow); !errors.Is(err, ErrEmptySelection) {
		t.Errorf("empty selection: %v", err)
	}
	if _, err := exp.Export([]string{"ghost"}, testNow); !errors.Is(err, ErrNoAccountsExported) {
		t.Errorf("only unknown ids: %v", err)
	}
	noErr(t, os.Remove(src.store.CredentialPath(accounts[1].ID)))
	res := must(exp.Export([]string{accounts[0].ID, "ghost", accounts[1].ID}, testNow))
	if res.ExportedCount != 1 || len(res.Failures) != 2 {
		t.Fatalf("result: %+v", res)
	}
	if res.Failures[0] != "ghost: account not found" || !strings.HasSuffix(res.Failures[1], ": credential missing") {
		t.Errorf("failures: %q", res.Failures)
	}
}

func TestDecodeRejectsBadPacks(t *testing.T) {
	cred := `{"auth_mode":"apikey","OPENAI_API_KEY":"sk-test-key-1234567890"}`
	valid := func(mut func(map[string]any)) []byte {
		var m map[string]any
		noErr(t, json.Unmarshal([]byte(`{"format":"quosparr-accounts","version":1,"provider":"codex","exportedAt":"2024-01-01T00:00:00Z","accounts":[{"credential":`+cred+`}]}`), &m))
		mut(m)
		return must(json.Marshal(m))
	}
	var unsupportedFormat *UnsupportedFormatError
	var unsupportedVersion *UnsupportedVersionError
	tests := []struct {
		name string
		data []byte
		is   func(error) bool
	}{
		{"wrong format", valid(func(m map[string]any) { m["format"] = "other" }), func(e error) bool { return errors.As(e, &unsupportedFormat) }},
		{"missing format", valid(func(m map[string]any) { delete(m, "format") }), func(e error) bool { return errors.As(e, &unsupportedFormat) && strings.Contains(e.Error(), "missing") }},
		{"version 2", valid(func(m map[string]any) { m["version"] = 2 }), func(e error) bool { return errors.As(e, &unsupportedVersion) && unsupportedVersion.Version == 2 }},
		{"version text", valid(func(m map[string]any) { m["version"] = "1" }), func(e error) bool { return errors.As(e, &unsupportedVersion) }},
		{"no version", valid(func(m map[string]any) { delete(m, "version") }), func(e error) bool { return errors.As(e, &unsupportedVersion) && unsupportedVersion.Version == -1 }},
		{"unknown provider", valid(func(m map[string]any) { m["provider"] = "other" }), func(e error) bool { return errors.As(e, &unsupportedFormat) }},
		{"accounts not array", valid(func(m map[string]any) { m["accounts"] = "x" }), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"accounts missing", valid(func(m map[string]any) { delete(m, "accounts") }), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"account not object", valid(func(m map[string]any) { m["accounts"] = []any{1} }), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"credential missing", valid(func(m map[string]any) { m["accounts"] = []any{map[string]any{"id": "x"}} }), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"credential scalar", valid(func(m map[string]any) { m["accounts"] = []any{map[string]any{"credential": "text"}} }), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"credential null", valid(func(m map[string]any) { m["accounts"] = []any{map[string]any{"credential": nil}} }), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"root not object", []byte(`[1]`), func(e error) bool { return errors.Is(e, ErrInvalidCredential) }},
		{"not json", []byte(`nope`), func(e error) bool { return e != nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeTransferPack(tc.data)
			if err == nil || !tc.is(err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	if _, err := DecodeTransferPack(valid(func(map[string]any) {})); err != nil {
		t.Fatalf("the valid baseline must decode: %v", err)
	}
}

func TestDecodeToleratesOptionalFields(t *testing.T) {
	data := `{"format":"quosparr-accounts","version":1.0,"provider":"grok","exportedAt":"garbage","accounts":[
		{"id":"  spaced  ","alias":"   ","credential":{"b":1,"a":{"z":2,"y":"x/y"}}}]}`
	pack := must(DecodeTransferPack([]byte(data)))
	if pack.Provider != ProviderGrok || pack.AppVersion != "" || time.Since(pack.ExportedAt) > time.Minute {
		t.Fatalf("pack: %+v", pack)
	}
	e := pack.Accounts[0]
	if e.ID != "spaced" || e.Alias != "" || string(e.Credential) != `{"a":{"y":"x/y","z":2},"b":1}` {
		t.Fatalf("entry: %+v (%s)", e, e.Credential)
	}
}

func TestEncodeValidatesHeader(t *testing.T) {
	cred := []byte(`{"auth_mode":"apikey","OPENAI_API_KEY":"sk-x"}`)
	pack := NewTransferPack(ProviderCodex, testNow, "", []TransferEntry{{Credential: cred}})
	var fe *UnsupportedFormatError
	var ve *UnsupportedVersionError
	bad := pack
	bad.Format = "x"
	if _, err := EncodeTransferPack(bad); !errors.As(err, &fe) {
		t.Errorf("format: %v", err)
	}
	bad = pack
	bad.Version = 7
	if _, err := EncodeTransferPack(bad); !errors.As(err, &ve) {
		t.Errorf("version: %v", err)
	}
	bad = pack
	bad.Accounts = []TransferEntry{{Credential: []byte("not json")}}
	if _, err := EncodeTransferPack(bad); !errors.Is(err, ErrInvalidCredential) {
		t.Errorf("credential: %v", err)
	}
}

func TestEncodeLayout(t *testing.T) {
	cred := []byte(`{"OPENAI_API_KEY":"sk-a/b","auth_mode":"apikey"}`)
	pack := NewTransferPack(ProviderCodex, time.Unix(1_700_000_000, 500_000_000), "1.2.3", []TransferEntry{
		{ID: "id-1", Alias: "A / B", Credential: cred},
	})
	out := string(must(EncodeTransferPack(pack)))
	for _, want := range []string{
		`"exportedAt" : "2023-11-14T22:13:20.500Z"`, `"appVersion" : "1.2.3"`, `"alias" : "A / B"`, `"OPENAI_API_KEY" : "sk-a/b"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	if strings.Contains(out, `\/`) {
		t.Error("slashes must not be escaped in packs")
	}
	if strings.Contains(out, "email") || strings.Contains(out, "displayName") {
		t.Error("empty optional fields must be omitted")
	}
}

func TestPreviewPackProviderMismatch(t *testing.T) {
	im := &Importer{Store: newEnv(t).store}
	_, err := im.PreviewPack(TransferPack{Provider: ProviderGrok})
	var pm *ProviderMismatchError
	if !errors.As(err, &pm) || pm.Actual != ProviderGrok || pm.Expected != ProviderCodex {
		t.Fatalf("err = %v", err)
	}
}

func TestPreviewFilesRoutesForeignPack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grok.json")
	noErr(t, os.WriteFile(path, fixture(t, "swift_pack_grok.json"), 0o600))
	preview := (&Importer{Store: newEnv(t).store}).PreviewFiles([]string{path})
	if preview.RoutedProvider != ProviderGrok || !preview.SourceIsPack || len(preview.Items) != 0 || len(preview.Failures) != 1 {
		t.Fatalf("preview: %+v", preview)
	}
}

func TestImportsSwiftPackFixture(t *testing.T) {
	e := newEnv(t)
	path := filepath.Join(t.TempDir(), "swift.json")
	noErr(t, os.WriteFile(path, fixture(t, "swift_pack.json"), 0o600))
	im := &Importer{Store: e.store, Now: func() time.Time { return testNow }}
	preview := im.PreviewFiles([]string{path})
	if len(preview.Items) != 3 {
		t.Fatalf("preview: %+v", preview)
	}
	var ids []string
	for _, i := range preview.Items {
		ids = append(ids, i.ID())
	}
	batch := im.ImportPreviewSelection(ctxBG(), preview.Items, ids, false)
	if len(batch.Succeeded) != 3 {
		t.Fatalf("batch: %+v", batch)
	}
	var want []string
	noErr(t, json.Unmarshal(fixture(t, "swift_account_ids.json"), &want))
	for i, acc := range batch.Succeeded {
		if acc.ID != want[i] {
			t.Errorf("account %d id %s, Swift %s", i, acc.ID, want[i])
		}
	}
}

func TestNothingSelected(t *testing.T) {
	im := &Importer{Store: newEnv(t).store}
	batch := im.ImportPreviewSelection(ctxBG(), nil, []string{"x"}, false)
	if len(batch.Failures) != 1 || batch.Failures[0] != FailureNothingSelected {
		t.Fatalf("batch: %+v", batch)
	}
}

func TestPackWriteFailureLeavesNoFile(t *testing.T) {
	pack := NewTransferPack(ProviderCodex, testNow, "", []TransferEntry{{Credential: []byte("bad")}})
	path := filepath.Join(t.TempDir(), "out.json")
	if err := WriteTransferPack(path, pack); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("a failed export left a file")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 0 {
		t.Fatalf("temp files left: %v", entries)
	}
}

func TestDecodeIfPack(t *testing.T) {
	if _, ok := DecodeTransferPackIfPack(fixture(t, "swift_pack.json")); !ok {
		t.Error("pack not recognized")
	}
	for _, raw := range [][]byte{fixture(t, "swift_cred_oauth.json"), []byte("text"), []byte(`{"format":"quosparr-accounts","version":9,"provider":"codex","accounts":[]}`)} {
		if _, ok := DecodeTransferPackIfPack(raw); ok {
			t.Errorf("%.40s should not be a pack", raw)
		}
	}
}

func TestDecodeAcceptsGeminiPacks(t *testing.T) {
	data := `{"format":"quosparr-accounts","version":1,"provider":"gemini","accounts":[{"id":"a","credential":{"k":1}}]}`
	pack := must(DecodeTransferPack([]byte(data)))
	if pack.Provider != ProviderGemini || len(pack.Accounts) != 1 {
		t.Fatalf("pack: %+v", pack)
	}
	path := filepath.Join(t.TempDir(), "gemini.json")
	noErr(t, os.WriteFile(path, []byte(data), 0o600))
	preview := (&Importer{Store: newEnv(t).store}).PreviewFiles([]string{path})
	if preview.RoutedProvider != ProviderGemini || !preview.SourceIsPack || len(preview.Items) != 0 {
		t.Fatalf("preview: routed=%s pack=%t items=%d", preview.RoutedProvider, preview.SourceIsPack, len(preview.Items))
	}
}
