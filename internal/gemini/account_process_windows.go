//go:build windows

package gemini

import (
	"context"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SystemProcessIDs lists running Antigravity.exe and agy.exe processes,
// excluding this process.
func SystemProcessIDs(ctx context.Context) (pids []int, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(snapshot)) }()
	entry := windows.ProcessEntry32{}
	entry.Size = uint32(unsafe.Sizeof(entry))
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if pid, ok := windowsProcessID(entry.ProcessID, windows.UTF16ToString(entry.ExeFile[:]), os.Getpid()); ok {
			pids = append(pids, pid)
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return pids, nil
}
