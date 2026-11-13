package migration

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/licoy/quosparr/internal/platform"
)

func copyTree(source, target string, merge bool) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	// Compute families before copying: SQLite database + WAL/SHM/journal are
	// one unit, never mix a new database with an older sidecar (or vice versa).
	families, err := existingDatabases(target)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "codex-runway.lock" || name == "quosparr.lock" || name == marker {
			continue
		}
		from, to := filepath.Join(source, name), filepath.Join(target, name)
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return errors.New("app data contains a symlink or special file")
		}
		if merge && families[databaseFamily(name)] {
			continue
		}
		info, statErr := os.Lstat(to)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		exists := statErr == nil
		if exists && (entry.IsDir() != info.IsDir() || (!info.IsDir() && !info.Mode().IsRegular())) {
			return fmt.Errorf("migration file/directory conflict: %s", name)
		}
		if entry.IsDir() {
			if err := platform.EnsurePrivateDirectory(to); err != nil {
				return err
			}
			if err := copyTree(from, to, merge); err != nil {
				return err
			}
		} else if name == "index.json" || name == "grok-index.json" || name == "claude-index.json" || name == "gemini-index.json" {
			if err := copyIndex(from, to, exists); err != nil {
				return err
			}
		} else if !exists {
			if err := copyPrivateFile(from, to); err != nil {
				return err
			}
		}
	}
	return nil
}

func regularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("migration input is a symlink or special file")
	}
	return nil
}

func copyPrivateFile(source, target string) error {
	if err := regularFile(source); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := platform.ProtectOwnerOnly(target, 0o600); err != nil {
		return errors.Join(err, out.Close())
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Sync(), out.Close())
}

func databaseFamily(name string) string {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		name = strings.TrimSuffix(name, suffix)
	}
	switch filepath.Ext(name) {
	case ".sqlite", ".sqlite3", ".db":
		return name
	}
	return ""
}

func existingDatabases(path string) (map[string]bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	families := map[string]bool{}
	for _, entry := range entries {
		if family := databaseFamily(entry.Name()); family != "" {
			families[family] = true
		}
	}
	return families, nil
}
