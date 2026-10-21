package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"testing"
)

func TestAPIApplyFailureRestoresOriginalSettingsAndIndex(t *testing.T) {
	for _, existed := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "existing"}[existed], func(t *testing.T) {
			r := newAPIRig(t)
			a := r.addAPI(t, "", keyX, APIProfile{Model: "chosen"})
			original := []byte("{ \"model\": \"hand\", \"env\": {\"ANTHROPIC_MODEL\":\"legacy\", \"KEEP\":\"value\"} }")
			var originalMode os.FileMode
			if existed {
				r.writeSettings(t, string(original))
				if err := os.Chmod(r.store.SettingsPath(), 0o640); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(r.store.SettingsPath())
				if err != nil {
					t.Fatal(err)
				}
				originalMode = info.Mode().Perm()
				if runtime.GOOS != "windows" && originalMode != 0o640 {
					t.Fatal("fixture permissions not applied")
				}
			}
			index := fileBytes(t, r.store.IndexPath())
			// Force the index commit to fail after applying valid settings, through
			// the real Store transaction seam without any production failure hooks.
			invalid := a
			invalid.CachedQuota = json.RawMessage("{")
			_, err := r.store.applyAPIAccount(invalid, keyY, a.API.Env(keyY), nil, r.now, true)
			if err == nil {
				t.Fatal("expected index commit rejection")
			}
			if key, loadErr := r.store.LoadAPIKey(a.ID); loadErr != nil || key != keyX {
				t.Fatal("failed index commit did not restore key")
			}
			if !bytes.Equal(index, fileBytes(t, r.store.IndexPath())) {
				t.Fatal("failed application changed index")
			}
			data, err := os.ReadFile(r.store.SettingsPath())
			if !existed {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal("rollback created originally missing settings")
				}
				return
			}
			if err != nil || !bytes.Equal(data, original) {
				t.Fatal("rollback failed to restore original settings bytes")
			}
			info, err := os.Stat(r.store.SettingsPath())
			if err != nil || info.Mode().Perm() != originalMode {
				t.Fatal("rollback did not preserve original permissions")
			}
		})
	}
}

func TestAPIOAuthFailureRestoresOriginalOverrideBytes(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	a := r.addAPI(t, "", keyX, APIProfile{Model: "main", OpusModel: "opus"})
	if err := r.mod.Switch(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	original := []byte("{ \"model\": \"manual\", \"env\": {\"ANTHROPIC_MODEL\":\"legacy\", \"ANTHROPIC_AUTH_TOKEN\":\"" + keyX + "\",\"ANTHROPIC_BASE_URL\":\"" + r.relay.url() + "\",\"ANTHROPIC_DEFAULT_OPUS_MODEL\":\"manual-opus\"} }")
	r.writeSettings(t, string(original))
	oauth := seedAccount(t, r.store, "acct-b", "org-b", false, r.now)
	if err := os.Remove(r.store.CredentialPath(oauth)); err != nil {
		t.Fatal(err)
	}
	if err := r.mod.Switch(ctx, oauth, true); err == nil {
		t.Fatal("expected OAuth install failure")
	}
	if !bytes.Equal(fileBytes(t, r.store.SettingsPath()), original) {
		t.Fatal("failed OAuth switch did not restore exact manual settings")
	}
}

func TestAPIModelRecordContainsOnlyFiveNonSecretFields(t *testing.T) {
	r := newAPIRig(t)
	a := r.addAPI(t, "", keyX, APIProfile{Model: "main", Context1M: true})
	if err := r.mod.Switch(context.Background(), a.ID, true); err != nil {
		t.Fatal(err)
	}
	data := fileBytes(t, r.store.IndexPath())
	if bytes.Contains(data, []byte(keyX)) || bytes.Contains(data, []byte("authToken")) || bytes.Contains(data, []byte("apiKey")) {
		t.Fatal("index contains credential material")
	}
	stored, _ := r.index(t).Account(a.ID)
	if stored.AppliedAPIModels == nil || stored.AppliedAPIModels.Model != "main[1m]" {
		t.Fatal("expanded model record missing")
	}
}

func TestAPILegacyOverrideChangesOnlyOnExplicitApply(t *testing.T) {
	r := newAPIRig(t)
	a := r.addAPI(t, "", keyX, APIProfile{Model: "selected"})
	if err := r.mod.Switch(context.Background(), a.ID, true); err != nil {
		t.Fatal(err)
	}
	data := r.settings(t)
	liveEnv(data)["ANTHROPIC_MODEL"] = "legacy"
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	r.writeSettings(t, string(body))
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: *a.API}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, fileBytes(t, r.store.SettingsPath())) {
		t.Fatal("save-only changed legacy override")
	}
	if _, err := r.mod.EditAPIAccount(a.ID, APIEdit{Profile: *a.API, Apply: true}); err != nil {
		t.Fatal(err)
	}
	if _, exists := liveEnv(r.settings(t))["ANTHROPIC_MODEL"]; exists {
		t.Fatal("explicit apply kept legacy override")
	}
}

func TestAPIIndexCommitFailureRestoresMissingOrInvalidManagedKey(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid", true: "missing"}[missing], func(t *testing.T) {
			r := newAPIRig(t)
			a := r.addAPI(t, "", keyX, APIProfile{Model: "selected"})
			original := []byte("invalid-managed-copy")
			if missing {
				if err := os.Remove(r.store.CredentialPath(a.ID)); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(r.store.CredentialPath(a.ID), original, 0o600); err != nil {
				t.Fatal(err)
			}
			a.CachedQuota = json.RawMessage("{")
			if _, err := r.store.applyAPIAccount(a, keyY, a.API.Env(keyY), nil, r.now, true); err == nil {
				t.Fatal("expected index commit rejection")
			}
			data, err := os.ReadFile(r.store.CredentialPath(a.ID))
			if missing {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal("failed commit retained newly created key")
				}
			} else if err != nil || !bytes.Equal(data, original) {
				t.Fatal("failed commit did not restore invalid managed copy")
			}
		})
	}
}
