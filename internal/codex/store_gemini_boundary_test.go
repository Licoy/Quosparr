package codex

import (
	"os"
	"path/filepath"
	"testing"
)

// The Gemini library shares the accounts directory. A gemini- directory and
// gemini-index.json beside it are not Codex accounts.
func TestLoadIndexDoesNotScanGeminiDirectories(t *testing.T) {
	home := t.TempDir()
	s := NewStore(filepath.Join(home, "codex"), home)
	root := s.Root()
	if err := os.MkdirAll(filepath.Join(root, "gemini-abc"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gemini-index.json"), []byte(`{"version":1,"currentAccountID":"gemini-abc","accounts":[{"id":"gemini-abc","sortIndex":0,"identity":{"subject":"1"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gemini-abc", "credential.json"), []byte(`{"access_token":"test-access-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := s.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Accounts) != 0 || idx.ActiveAccountID != "" {
		t.Fatalf("gemini files were treated as codex accounts: %+v", idx.Accounts)
	}
}
