package app

import (
	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

var procGetWindow = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindow")

const gwHwndNext = 2

// nativeFloatingBelowFront reports whether the window is below the
// foreground window; known is false without one it could go below.
func nativeFloatingBelowFront(win *mygo.Window) (below, known bool) {
	hwnd, front := win.NativeHandle(), windows.GetForegroundWindow()
	if front == 0 || uintptr(front) == hwnd {
		return false, false
	}
	index := int32(gwlExStyle)
	if style, _, _ := procGetWindowLong.Call(uintptr(front), uintptr(index)); style&wsExTopmost != 0 {
		return false, false
	}
	for w, _, _ := procGetWindow.Call(uintptr(front), gwHwndNext); w != 0; w, _, _ = procGetWindow.Call(w, gwHwndNext) {
		if w == hwnd {
			return true, true
		}
	}
	return false, true
}
