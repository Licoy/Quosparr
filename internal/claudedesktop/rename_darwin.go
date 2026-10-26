//go:build darwin

package claudedesktop

import "golang.org/x/sys/unix"

func renameDirectory(from, to string) error {
	if err := unix.RenamexNp(from, to, unix.RENAME_EXCL); err != nil {
		return err
	}
	return syncRenameParents(from, to)
}
