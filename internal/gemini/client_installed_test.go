package gemini

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestClientInstalledReadsFilesThenPath(t *testing.T) {
	dir := t.TempDir()
	missing := func(string) (string, error) { return "", errors.New("not found") }
	found := func(string) (string, error) { return "/usr/local/bin/agy", nil }
	app := filepath.Join(dir, "Antigravity.app", "Contents", "Info.plist")
	if clientInstalled([]string{app}, missing) {
		t.Fatal("nothing installed reads as installed")
	}
	if !clientInstalled([]string{app}, found) {
		t.Fatal("agy on PATH reads as not installed")
	}
	if err := os.MkdirAll(filepath.Dir(app), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app, []byte("<plist/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !clientInstalled([]string{app}, missing) {
		t.Fatal("the desktop app reads as not installed")
	}
}
