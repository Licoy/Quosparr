//go:build windows

package claudedesktop

import "golang.org/x/sys/windows"

func renameDirectory(from, to string) error {
	src, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(src, dst, windows.MOVEFILE_WRITE_THROUGH)
}

// Windows metadata replacements and directory moves use MOVEFILE_WRITE_THROUGH.
func syncDirectory(string) error { return nil }
