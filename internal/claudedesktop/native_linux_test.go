//go:build linux

package claudedesktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxProcessIdentityOnlyReadsInjectedProcFixture(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "123")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Fields 3..22, with an executable command containing spaces and parentheses.
	fields := []string{"S"}
	for range 18 {
		fields = append(fields, "0")
	}
	fields = append(fields, "123456")
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("123 (Claude (fixture) desktop) "+strings.Join(fields, " ")), 0600); err != nil {
		t.Fatal(err)
	}
	if birth, err := linuxProcessBirth(dir); err != nil || birth != "123456" {
		t.Fatalf("%q %v", birth, err)
	}
	if err := os.Symlink("/fixture/claude-code", filepath.Join(dir, "exe")); err != nil {
		t.Fatal(err)
	}
	processes, err := linuxDesktopProcesses(context.Background(), Application{Executable: "/fixture/claude-desktop"}, root, uint32(os.Getuid()))
	if err != nil || len(processes) != 0 {
		t.Fatalf("unrelated CLI was selected: %v %v", processes, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := linuxProcessBirth(dir); err == nil {
		t.Fatal("corrupt process identity accepted")
	}
}
