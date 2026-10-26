//go:build windows

package claudedesktop

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var desktopOle32 = windows.NewLazySystemDLL("ole32.dll")
var desktopCoCreate = desktopOle32.NewProc("CoCreateInstance")
var desktopActivationCLSID = windows.GUID{Data1: 0x45ba127d, Data2: 0x10a8, Data3: 0x46ea, Data4: [8]byte{0x8a, 0xb7, 0x56, 0xea, 0x90, 0x78, 0x94, 0x3c}}
var desktopActivationIID = windows.GUID{Data1: 0x2e941141, Data2: 0x7f97, Data3: 0x4756, Data4: [8]byte{0xba, 0x1d, 0x9d, 0xec, 0xde, 0x89, 0x4a, 0x3d}}

func launchWindowsDesktop(ctx context.Context, exe string, env []string) error {
	identity, err := readDesktopMSIX(exe)
	if err != nil {
		return err
	}
	if identity.Root == "" {
		return launchHostProcess(ctx, exe, env)
	}
	// Package activation inherits the Windows user session, not Quosparr's
	// environment; it preserves package identity without routing/debug overrides.
	start := func(string, []string) (desktopChild, error) {
		return startDesktopMSIX(identity, exe, desktopActivationAPI{activate: requestDesktopMSIXActivation, child: openDesktopMSIXChild})
	}
	return observeDesktopLaunch(ctx, exe, nil, start, time.Second)
}

type windowsDesktopChild struct{ handle windows.Handle }

func (child windowsDesktopChild) Wait() error {
	defer windows.CloseHandle(child.handle)
	_, err := windows.WaitForSingleObject(child.handle, windows.INFINITE)
	if err != nil {
		return err
	}
	var code uint32
	if err = windows.GetExitCodeProcess(child.handle, &code); err != nil {
		return err
	}
	if code != 0 {
		return errors.New("Claude Desktop exited unsuccessfully")
	}
	return nil
}

func requestDesktopMSIXActivation(applicationID string) (uint32, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	init, _, _ := desktopOle32.NewProc("CoInitializeEx").Call(0, windows.COINIT_APARTMENTTHREADED)
	if int32(init) < 0 {
		return 0, errors.New("cannot initialize Claude Desktop package activation")
	}
	defer windows.CoUninitialize()
	var manager unsafe.Pointer
	hr, _, _ := desktopCoCreate.Call(uintptr(unsafe.Pointer(&desktopActivationCLSID)), 0, 1, uintptr(unsafe.Pointer(&desktopActivationIID)), uintptr(unsafe.Pointer(&manager)))
	if int32(hr) < 0 || manager == nil {
		return 0, errors.New("cannot create Claude Desktop package activation manager")
	}
	defer desktopCOMCall(manager, 2)
	aumid, err := windows.UTF16PtrFromString(applicationID)
	if err != nil {
		return 0, err
	}
	var pid uint32
	hr = desktopCOMCall(manager, 3, uintptr(unsafe.Pointer(aumid)), 0, 0, uintptr(unsafe.Pointer(&pid)))
	if int32(hr) < 0 || pid == 0 {
		return 0, errors.New("Claude Desktop package activation failed")
	}
	return pid, nil
}

func openDesktopMSIXChild(pid uint32, exe string) (desktopChild, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil, errors.New("cannot verify Claude Desktop package startup")
	}
	if err = verifyActivatedDesktopIdentity(handle, exe); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return windowsDesktopChild{handle}, nil
}

func verifyActivatedDesktopIdentity(handle windows.Handle, exe string) error {
	var buffer [32768]uint16
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return errors.New("cannot verify Claude Desktop package executable")
	}
	if !sameExecutable(windows.UTF16ToString(buffer[:size]), exe, "windows") {
		return errors.New("Claude Desktop package activated a different executable")
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	var token windows.Token
	if err = windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	if !user.User.Sid.Equals(current.User.Sid) {
		return errors.New("Claude Desktop package activated under a different user")
	}
	return nil
}

func desktopCOMCall(object unsafe.Pointer, index uintptr, args ...uintptr) uintptr {
	table := *(*unsafe.Pointer)(object)
	method := *(*uintptr)(unsafe.Add(table, index*unsafe.Sizeof(uintptr(0))))
	result, _, _ := syscall.SyscallN(method, append([]uintptr{uintptr(object)}, args...)...)
	return result
}
