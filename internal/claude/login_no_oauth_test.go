package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserImportWithoutOfficialOAuth(t *testing.T) {
	for name, raw := range map[string]string{"missing": `{"mcpOAuth":{"preserve":true}}`, "null": `{"claudeAiOauth":null,"mcpOAuth":{}}`} {
		for _, keychain := range []bool{false, true} {
			t.Run(name+map[bool]string{false: "/file", true: "/keychain"}[keychain], func(t *testing.T) {
				r := newSyncRig(t)
				r.login(t, "old", "same")
				if _, err := r.mod.Sync(context.Background()); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(CredentialsFilePath(r.config), []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
				mem := &memSecurity{has: true, password: raw}
				if keychain {
					r.store.Official = KeychainStore{Runner: mem}
				}
				global, err := os.ReadFile(filepath.Join(r.home, ".claude.json"))
				if err != nil {
					t.Fatal(err)
				}
				settings := []byte(`{"env":{},"unrelated":true}`)
				if err := os.WriteFile(r.store.SettingsPath(), settings, 0600); err != nil {
					t.Fatal(err)
				}
				acc, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("same", "new"))
				if err != nil {
					t.Fatalf("browser import failed: %v", err)
				}
				managed, err := os.ReadFile(r.store.CredentialPath(acc.ID))
				if err != nil || bytes.Contains(managed, []byte("mcpOAuth")) || readCred(t, r.store, acc.ID).OAuth().AccessToken != "new" {
					t.Fatal("managed credential incorrect")
				}
				if err := r.mod.Refresh(context.Background(), acc.ID, true); err != nil {
					t.Fatalf("refresh failed: %v", err)
				}
				idx, err := r.store.LoadIndex()
				if err != nil || idx.CurrentAccountID != "" {
					t.Fatal("refresh did not clear stale current")
				}
				if id, err := r.mod.Sync(context.Background()); err != nil || id != "" {
					t.Fatalf("sync did not clear stale current: %v", err)
				}
				if readCred(t, r.store, acc.ID).OAuth().AccessToken != "new" {
					t.Fatal("sync changed managed copy")
				}
				if mem.writes != 0 || mem.password != raw {
					t.Fatal("keychain changed")
				}
				for path, want := range map[string][]byte{CredentialsFilePath(r.config): []byte(raw), filepath.Join(r.home, ".claude.json"): global, r.store.SettingsPath(): settings} {
					got, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(got, want) {
						t.Fatal("official files changed")
					}
				}
			})
		}
	}
}

func TestNoOfficialOAuthKeepsAPICurrentAndSwitchPreservesSibling(t *testing.T) {
	r := newSyncRig(t)
	r.login(t, "old", "same")
	const raw = `{"mcpOAuth":{"preserve":true},"theme":"dark"}`
	if err := os.WriteFile(CredentialsFilePath(r.config), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	acc, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("same", "new"))
	if err != nil {
		t.Fatal(err)
	}
	api, err := r.mod.addAPI("relay", APIProfile{BaseURL: "https://relay.example.com"}, "test-api-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.store.SettingsPath(), []byte(`{"env":{"ANTHROPIC_BASE_URL":"https://relay.example.com","ANTHROPIC_AUTH_TOKEN":"test-api-key"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if id, err := r.mod.Sync(context.Background()); err != nil || id != api.ID {
		t.Fatalf("API selection lost: %v", err)
	}
	if _, err := r.mod.Apply(context.Background(), Command{Kind: CommandSwitch, ID: acc.ID, AllowWhileRunning: true}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(CredentialsFilePath(r.config))
	if err != nil {
		t.Fatal(err)
	}
	root, err := rawObject(got)
	if err != nil || !bytes.Equal(root["mcpOAuth"], []byte(`{"preserve":true}`)) || !bytes.Equal(root["theme"], []byte(`"dark"`)) {
		t.Fatal("switch changed credential siblings")
	}
	if token, err := AccessTokenOf(got); err != nil || token != "new" {
		t.Fatal("switch did not install managed OAuth")
	}
}

func TestBrowserWithoutOAuthProofRejectsContainerChanges(t *testing.T) {
	for name, replacement := range map[string]string{"deleted": "", "logged-in": `{"claudeAiOauth":{"accessToken":"other"}}`, "sibling": `{"mcpOAuth":{"changed":true}}`} {
		t.Run(name, func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "old", "same")
			path := CredentialsFilePath(r.config)
			if err := os.WriteFile(path, []byte(`{"mcpOAuth":{}}`), 0600); err != nil {
				t.Fatal(err)
			}
			proof, err := r.mod.browserOfficialProof(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if replacement == "" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte(replacement), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := proof.recheck(context.Background(), r.store); err == nil {
				t.Fatal("changed official container accepted")
			}
		})
	}
}

func TestOfficialOAuthInvalidContainerRejected(t *testing.T) {
	for name, raw := range map[string]string{"empty": "", "whitespace": " ", "null": "null", "array": "[]", "malformed": `{"broken":`, "oauth-string": `{"claudeAiOauth":"secret"}`, "oauth-array": `{"claudeAiOauth":[]}`} {
		t.Run(name, func(t *testing.T) {
			r := newSyncRig(t)
			r.login(t, "old", "same")
			if err := os.WriteFile(CredentialsFilePath(r.config), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := r.mod.ImportBrowserLogin(context.Background(), browserResult("same", "new"))
			requireOperation(t, err, StageReadOfficial, KindInvalidData)
			_, err = r.mod.Sync(context.Background())
			requireOperation(t, err, StageReadOfficial, KindInvalidData)
			if _, err := os.Stat(r.store.IndexPath()); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid official container created account")
			}
		})
	}
}
