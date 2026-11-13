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
func SystemProcessIDs(ctx context.Context) ([]int, error) {
	found, err := systemProcesses(ctx)
	return processIDs(found), err
}

func systemProcesses(ctx context.Context) (found []antigravityProcess, err error) {
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
		if p, ok := windowsProcess(entry.ProcessID, windows.UTF16ToString(entry.ExeFile[:]), os.Getpid()); ok {
			found = append(found, p)
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return found, nil
}
