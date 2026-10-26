//go:build linux

package claudedesktop

import "golang.org/x/sys/unix"

func renameDirectory(from, to string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return syncRenameParents(from, to)
}
