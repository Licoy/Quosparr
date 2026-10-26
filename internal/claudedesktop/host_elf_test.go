package claudedesktop

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func fixtureDesktopELF() []byte {
	data := make([]byte, 64)
	copy(data, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(data[16:18], 2)
	binary.LittleEndian.PutUint16(data[18:20], 62)
	binary.LittleEndian.PutUint32(data[20:24], 1)
	binary.LittleEndian.PutUint16(data[52:54], 64)
	return data
}

func TestLinuxDesktopSymlinkEntryMayResolveToDifferentELFBasename(t *testing.T) {
	h := fixtureHost(t, "linux")
	original, err := h.Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(filepath.Dir(original.Executable), "Claude")
	if err := os.Rename(original.Executable, real); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(h.home, "bin", "claude-desktop")
	if err := os.MkdirAll(filepath.Dir(entry), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, entry); err != nil {
		t.Skipf("fixture symlink unavailable: %v", err)
	}
	app, err := h.Resolve(context.Background(), entry)
	if err != nil || app.Executable != real {
		t.Fatalf("%#v %v", app, err)
	}
	cli := filepath.Join(h.home, "cli", "claude")
	if err := os.MkdirAll(filepath.Dir(cli), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cli, fixtureDesktopELF(), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Resolve(context.Background(), cli); err == nil {
		t.Fatal("CLI ELF without Desktop archive accepted")
	}
	if err := os.WriteFile(real, []byte("#!/bin/sh\nexec claude\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Resolve(context.Background(), entry); err == nil {
		t.Fatal("shell wrapper executed or accepted")
	}
}

func TestAutomaticDiscoveryRejectsMultipleDistinctInstallations(t *testing.T) {
	h := fixtureHost(t, "linux")
	app, err := h.Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(h.home, "second-install", "Claude")
	if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, fixtureDesktopELF(), 0700); err != nil {
		t.Fatal(err)
	}
	writeHostArchive(t, filepath.Join(filepath.Dir(other), "resources", "app.asar"), `{"name":"@ant/desktop","productName":"Claude","main":".vite/build/index.pre.js"}`)
	h.candidates = func(context.Context) ([]string, error) { return []string{app.Path, other}, nil }
	if _, err := h.Resolve(context.Background(), ""); err == nil {
		t.Fatal("ambiguous installation auto-selected")
	}
	if selected, err := h.Resolve(context.Background(), other); err != nil || selected.Executable != other {
		t.Fatalf("%#v %v", selected, err)
	}
}
