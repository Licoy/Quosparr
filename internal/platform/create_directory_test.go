package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreatePrivateDirectoryExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new")
	if err := CreatePrivateDirectory(path); err != nil {
		t.Fatal(err)
	}
	if err := VerifyOwnerOnly(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := CreatePrivateDirectory(path); err == nil {
		t.Fatal("existing directory accepted")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CreatePrivateDirectory(file); err == nil {
		t.Fatal("existing file accepted")
	}
}

func TestEditPrivateFileWithBackupCheckedRestrictsPublicMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	policy := BackupPolicy{Dir: filepath.Join(dir, "backups"), Prefix: "private-", Ext: ".json", Keep: 10}
	err := EditPrivateFileWithBackupChecked(path, policy, time.Now(), func([]byte) ([]byte, error) { return []byte("secret"), nil }, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyOwnerOnly(path, 0600); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(policy.Dir)
	if err != nil || len(files) != 1 {
		t.Fatal("missing backup", err)
	}
	if err := VerifyOwnerOnly(filepath.Join(policy.Dir, files[0].Name()), 0600); err != nil {
		t.Fatal(err)
	}
}
