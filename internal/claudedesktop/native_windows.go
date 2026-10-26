//go:build windows

package claudedesktop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var desktopUser32 = windows.NewLazySystemDLL("user32.dll")
var desktopPostMessage = desktopUser32.NewProc("PostMessageW")

func NewNativeHost(home string) *NativeHost {
	h := newHost(home, "windows")
	h.candidates = func(ctx context.Context) ([]string, error) { return windowsDesktopCandidates(ctx, h.run) }
	h.dataDir = windowsDesktopDataDir
	h.processes = windowsDesktopProcesses
	h.launch = launchWindowsDesktop
	return h
}

func windowsDesktopCandidates(ctx context.Context, run commandRunner) ([]string, error) {
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return nil, err
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	return windowsCandidatePaths(ctx, run, windowsInstallRoots{Local: local, System: system})
}

func windowsDesktopDataDir(ctx context.Context, exe string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	roaming, err := windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, 0)
	if err != nil {
		return "", err
	}
	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", err
	}
	if err := verifyFirstPartyDeployment([]string{filepath.Join(local, "Claude-3p"), filepath.Join(roaming, "Claude-3p")}); err != nil {
		return "", err
	}
	paths := windowsDataDirCandidates(exe, roaming, local)
	return chooseDesktopDataDir(paths, "windows")
}

func windowsDesktopProcesses(ctx context.Context, app Application) (result []hostProcess, err error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	topLevel, err := desktopTopLevelWindows()
	if err != nil {
		return nil, err
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			closeHostProcesses(result)
			result = nil
		}
	}()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if e := ctx.Err(); e != nil {
			return result, e
		}
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if name == "claude.exe" || name == "chrome_crashpad_handler.exe" {
			process, match, e := openWindowsDesktopProcess(entry.ProcessID, app, current.User.Sid)
			if e != nil {
				return result, e
			}
			if match {
				result = append(result, markWindowsWindowHost(process, entry.ProcessID, topLevel))
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		err = nil
		hasMain, hasWindow := false, false
		for _, process := range result {
			hasMain = hasMain || sameExecutable(process.executable, app.Executable, "windows")
			hasWindow = hasWindow || process.terminate != nil
		}
		if hasMain && !hasWindow {
			err = errors.New("Claude Desktop has no normal close window; quit it manually before switching")
		}
	}
	return result, err
}

func openWindowsDesktopProcess(pid uint32, app Application, owner *windows.SID) (hostProcess, bool, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return hostProcess{}, false, nil
	}
	if err != nil {
		return hostProcess{}, false, fmt.Errorf("inspect Claude Desktop process: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			windows.CloseHandle(handle)
		}
	}()
	var token windows.Token
	if err = windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return hostProcess{}, false, err
	}
	user, err := token.GetTokenUser()
	token.Close()
	if err != nil {
		return hostProcess{}, false, err
	}
	if !user.User.Sid.Equals(owner) {
		return hostProcess{}, false, nil
	}
	var buffer [32768]uint16
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return hostProcess{}, false, err
	}
	exe := windows.UTF16ToString(buffer[:size])
	if !belongsToApplication(exe, app, "windows") {
		if err := rejectOtherDesktop(context.Background(), exe, app, "windows", runHostCommand); err != nil {
			return hostProcess{}, false, err
		}
		return hostProcess{}, false, nil
	}
	var birth, exit, kernel, cpu windows.Filetime
	if err = windows.GetProcessTimes(handle, &birth, &exit, &kernel, &cpu); err != nil {
		return hostProcess{}, false, err
	}
	process := hostProcess{executable: exe, close: func() { windows.CloseHandle(handle) }}
	process.exited = func() (bool, error) {
		state, e := windows.WaitForSingleObject(handle, 0)
		return state == windows.WAIT_OBJECT_0, e
	}
	if sameExecutable(exe, app.Executable, "windows") {
		process.terminate = func() error { return closeWindowsDesktopWindows(pid, handle, birth) }
	}
	keep = true
	return process, true, nil
}

func closeWindowsDesktopWindows(pid uint32, handle windows.Handle, birth windows.Filetime) error {
	same := func() (bool, error) {
		var current, exit, kernel, cpu windows.Filetime
		if err := windows.GetProcessTimes(handle, &current, &exit, &kernel, &cpu); err != nil {
			return false, err
		}
		state, err := windows.WaitForSingleObject(handle, 0)
		return current == birth && state != windows.WAIT_OBJECT_0, err
	}
	post := func(hwnd uintptr) error {
		result, _, err := desktopPostMessage.Call(hwnd, 0x0010, 0, 0)
		if result == 0 {
			return err
		}
		return nil
	}
	return requestDesktopWindowsClose(pid, desktopCloseAPI{windows: desktopTopLevelWindows, sameProcess: same, postClose: post})
}

func desktopTopLevelWindows() ([]desktopWindow, error) {
	result := []desktopWindow{}
	var callbackErr error
	callback := syscall.NewCallback(func(hwnd windows.HWND, param uintptr) uintptr {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err != nil {
			callbackErr = err
			return 0
		}
		result = append(result, desktopWindow{handle: uintptr(hwnd), pid: pid})
		return 1
	})
	err := windows.EnumWindows(callback, nil)
	if callbackErr != nil {
		return nil, callbackErr
	}
	if err != nil {
		return nil, err
	}
	return result, nil
}
