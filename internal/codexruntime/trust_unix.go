//go:build !windows

package codexruntime

import (
	"os"
	"syscall"
)

func trusted(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid()) && info.Mode().Perm()&0022 == 0
}

func owned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

func trustedDirectory(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (stat.Uid != 0 && stat.Uid != uint32(os.Getuid())) {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	return info.IsDir() && (info.Mode().Perm()&0022 == 0 || stat.Uid == 0 && info.Mode()&os.ModeSticky != 0)
}
