//go:build windows

package claudedesktop

import (
	"golang.org/x/sys/windows"
	"os"
)

func directoryID(path string) (directoryIdentity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return directoryIdentity{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return directoryIdentity{}, ErrUnsafePath
	}
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return directoryIdentity{}, err
	}
	h, err := windows.CreateFile(ptr, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return directoryIdentity{}, err
	}
	defer windows.CloseHandle(h)
	var id windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &id); err != nil {
		return directoryIdentity{}, err
	}
	if id.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return directoryIdentity{}, ErrUnsafePath
	}
	return directoryIdentity{Volume: uint64(id.VolumeSerialNumber), File: uint64(id.FileIndexHigh)<<32 | uint64(id.FileIndexLow)}, nil
}
