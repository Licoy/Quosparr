//go:build windows

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/store"
	"golang.org/x/sys/windows"
)

var (
	procGetApplicationUserModelID   = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetApplicationUserModelId")
	procSHGetPropertyStoreForWindow = windows.NewLazySystemDLL("shell32.dll").NewProc("SHGetPropertyStoreForWindow")
	procSHCreateItemFromParsingName = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
	procPropVariantClear            = windows.NewLazySystemDLL("ole32.dll").NewProc("PropVariantClear")
	procSetWindowPos                = windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowPos")
	procGetWindowLong               = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowLongW")

	iidIPropertyStore = windows.GUID{Data1: 0x886d8eeb, Data2: 0x8cf2, Data3: 0x4446, Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	iidIShellItem2    = windows.GUID{Data1: 0x7e9fb0d3, Data2: 0x919f, Data3: 0x4307, Data4: [8]byte{0xab, 0x2e, 0x9b, 0x18, 0x60, 0x31, 0x0c, 0x93}}
	// PKEY_AppUserModel_ID and PKEY_Link_TargetParsingPath.
	pkeyAppUserModelID    = propertyKey{windows.GUID{Data1: 0x9f4c2855, Data2: 0x9f79, Data3: 0x4b39, Data4: [8]byte{0xa8, 0xd0, 0xe1, 0xd4, 0x2d, 0xe1, 0xd5, 0xf3}}, 5}
	pkeyLinkTargetParsing = propertyKey{windows.GUID{Data1: 0xb9b4b3fc, Data2: 0x2b51, Data3: 0x4a42, Data4: [8]byte{0xb5, 0xd8, 0x32, 0x41, 0x46, 0xaf, 0xcf, 0x25}}, 2}
)

type propertyKey struct {
	fmtid windows.GUID
	pid   uint32
}

// propVariant is a PROPVARIANT holding a string (VT_LPWSTR).
type propVariant struct {
	vt  uint16
	_   [3]uint16
	val *uint16
	_   uintptr
}

const vtLPWStr = 31

// winDesktopClasses are the desktop's windows; winPassingClasses the
// taskbar's and the app switchers'.
var (
	winDesktopClasses = map[string]bool{"Progman": true, "WorkerW": true}
	winPassingClasses = map[string]bool{
		"Shell_TrayWnd": true, "Shell_SecondaryTrayWnd": true, "NotifyIconOverflowWindow": true,
		"TopLevelWindowForOverflowXamlIsland": true, "XamlExplorerHostIslandWindow": true,
		"MultitaskingViewFrame": true, "TaskSwitcherWnd": true, "ForegroundStaging": true,
	}
	// winShellHosts run the Start menu, search and the lock screen.
	winShellHosts = map[string]bool{
		"startmenuexperiencehost.exe": true, "searchhost.exe": true, "searchapp.exe": true,
		"shellexperiencehost.exe": true, "lockapp.exe": true,
	}
)

// foregroundSupported: the foreground window tells the app in front.
func foregroundSupported() bool { return true }

// newForegroundProbe runs on the watching goroutine, in a COM apartment of
// its own for the window property stores.
func newForegroundProbe() (func() foreground, func()) {
	runtime.LockOSThread()
	hr, _, _ := windows.NewLazySystemDLL("ole32.dll").NewProc("CoInitializeEx").Call(0, windows.COINIT_APARTMENTTHREADED)
	return winForeground, func() {
		if int32(hr) >= 0 {
			windows.CoUninitialize()
		}
		runtime.UnlockOSThread()
	}
}

func winForeground() foreground {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return foreground{}
	}
	class := windowClass(hwnd)
	switch {
	case winDesktopClasses[class]:
		return foreground{kind: fgDesktop}
	case winPassingClasses[class]:
		return foreground{}
	}
	var pid uint32
	windows.GetWindowThreadProcessId(hwnd, &pid)
	if pid == 0 || int(pid) == os.Getpid() {
		return foreground{}
	}
	if class == "ApplicationFrameWindow" {
		// A packaged app's frame belongs to ApplicationFrameHost; its
		// content window, to the app.
		if hosted := hostedProcess(hwnd, pid); hosted != 0 {
			pid = hosted
		}
	}
	exe := processImage(pid)
	if winShellHosts[strings.ToLower(filepath.Base(exe))] {
		return foreground{}
	}
	return foreground{kind: fgApp, keys: lowerKeys(exe, processAUMID(pid), windowAUMID(hwnd))}
}

func windowClass(hwnd windows.HWND) string {
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(hwnd, &buf[0], int32(len(buf)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

var (
	hostedOnce     sync.Once
	hostedCallback uintptr
	// hostedFound is the search of the enumeration running.
	hostedMu    sync.Mutex
	hostedFound struct{ host, pid uint32 }
)

// hostedProcess is the process of a child window of hwnd that host does
// not own, or 0.
func hostedProcess(hwnd windows.HWND, host uint32) uint32 {
	hostedOnce.Do(func() {
		hostedCallback = syscall.NewCallback(func(child windows.HWND, _ uintptr) uintptr {
			var pid uint32
			windows.GetWindowThreadProcessId(child, &pid)
			if pid != 0 && pid != hostedFound.host {
				hostedFound.pid = pid
				return 0
			}
			return 1
		})
	})
	hostedMu.Lock()
	defer hostedMu.Unlock()
	hostedFound.host, hostedFound.pid = host, 0
	windows.EnumChildWindows(hwnd, hostedCallback, nil)
	return hostedFound.pid
}

func processImage(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// processAUMID is the AppUserModelID of a packaged app's process.
func processAUMID(pid uint32) string {
	if procGetApplicationUserModelID.Find() != nil {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 131) // APPLICATION_USER_MODEL_ID_MAX_LENGTH
	n := uint32(len(buf))
	if r, _, _ := procGetApplicationUserModelID.Call(uintptr(h), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0]))); r != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// windowAUMID is the AppUserModelID a window was given, if any.
func windowAUMID(hwnd windows.HWND) string {
	if procSHGetPropertyStoreForWindow.Find() != nil {
		return ""
	}
	var ps unsafe.Pointer
	if r, _, _ := procSHGetPropertyStoreForWindow.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&iidIPropertyStore)), uintptr(unsafe.Pointer(&ps))); int32(r) < 0 || ps == nil {
		return ""
	}
	defer comRelease(ps)
	var v propVariant
	if comMethod(ps, 5, uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&v))) < 0 { // GetValue
		return ""
	}
	defer procPropVariantClear.Call(uintptr(unsafe.Pointer(&v)))
	if v.vt != vtLPWStr || v.val == nil {
		return ""
	}
	return windows.UTF16PtrToString(v.val)
}

// comMethod calls the method at index i of a COM object's vtable.
func comMethod(obj unsafe.Pointer, i int, args ...uintptr) int32 {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return int32(r)
}

func comRelease(obj unsafe.Pointer) { comMethod(obj, 2) }

// floatingAppKeys: the ID, the full path of a known-folder ID, and the
// program a Start menu shortcut starts, which identifies a desktop app in
// front whose windows carry no AppUserModelID.
func floatingAppKeys(a store.FloatingApp) []string {
	keys := []string{a.ID, knownFolderPath(a.ID)}
	if strings.HasPrefix(strings.ToLower(a.Path), `shell:appsfolder\`) {
		keys = append(keys, shellLinkTarget(a.Path))
	}
	return lowerKeys(keys...)
}

// knownFolderPath resolves an ID such as {6D809377-…}\App\App.exe.
func knownFolderPath(id string) string {
	end := strings.Index(id, `}\`)
	if !strings.HasPrefix(id, "{") || end < 0 {
		return ""
	}
	guid, err := windows.GUIDFromString(id[:end+1])
	if err != nil {
		return ""
	}
	dir, err := windows.KnownFolderPath((*windows.KNOWNFOLDERID)(&guid), 0)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, id[end+2:])
}

// shellLinkTarget is the program of an item of the Applications folder.
func shellLinkTarget(path string) string {
	if procSHCreateItemFromParsingName.Find() != nil {
		return ""
	}
	if hr, _, _ := windows.NewLazySystemDLL("ole32.dll").NewProc("CoInitializeEx").Call(0, windows.COINIT_APARTMENTTHREADED); int32(hr) >= 0 {
		defer windows.CoUninitialize()
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var item unsafe.Pointer
	if r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&iidIShellItem2)), uintptr(unsafe.Pointer(&item))); int32(r) < 0 || item == nil {
		return ""
	}
	defer comRelease(item)
	var s *uint16
	if comMethod(item, 17, uintptr(unsafe.Pointer(&pkeyLinkTargetParsing)), uintptr(unsafe.Pointer(&s))) < 0 || s == nil { // IShellItem2::GetString
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(s))
	return windows.UTF16PtrToString(s)
}

// SetWindowPos flags and the extended style of topmost windows.
const (
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010
	gwlExStyle    = -20
	wsExTopmost   = 0x0008
)

// lowerFloating takes the window off the top. Off it, the window stays above
// every other window, the app that came in front's too, so it goes just
// below the foreground window; below a topmost one it already is.
func lowerFloating(win *mygo.Window) {
	win.SetAlwaysOnTop(false)
	hwnd, front := win.NativeHandle(), windows.GetForegroundWindow()
	if hwnd == 0 || front == 0 || uintptr(front) == hwnd {
		return
	}
	index := int32(gwlExStyle)
	style, _, _ := procGetWindowLong.Call(uintptr(front), uintptr(index))
	if style&wsExTopmost != 0 {
		return
	}
	procSetWindowPos.Call(hwnd, uintptr(front), 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

// floatingAppFromFile takes a program the user picked.
func floatingAppFromFile(path string) (store.FloatingApp, bool) {
	if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return store.FloatingApp{}, false
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return store.FloatingApp{ID: path, Name: name, Path: path}, true
}
