//go:build !windows

package claudedesktop

import (
	"errors"
	"os"
	"path/filepath"
)

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
func syncRenameParents(from, to string) error {
	return errors.Join(syncDirectory(filepath.Dir(from)), syncDirectory(filepath.Dir(to)))
}
