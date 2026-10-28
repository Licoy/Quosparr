//go:build windows

package codexruntime

import "os"

func trusted(os.FileInfo) bool { return false }

func owned(os.FileInfo) bool { return false }

func trustedDirectory(os.FileInfo) bool { return false }
