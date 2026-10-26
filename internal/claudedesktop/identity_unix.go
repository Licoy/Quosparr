//go:build !windows

package claudedesktop

import (
	"os"
	"syscall"
)

func directoryID(path string) (directoryIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return directoryIdentity{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return directoryIdentity{}, ErrUnsafePath
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return directoryIdentity{}, ErrUnsafePath
	}
	return directoryIdentity{Volume: uint64(st.Dev), File: uint64(st.Ino)}, nil
}
