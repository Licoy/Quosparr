//go:build linux

package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/store"
)

// xlib binds GDK's X11 display: GDK traps the errors of a window going away
// while it is read, which would otherwise end the process.
var xlib struct {
	once sync.Once
	ok   bool

	gdkDisplay     func() uintptr
	x11DisplayType func() uintptr
	isA            func(instance, typ uintptr) bool
	xdisplay       func(display uintptr) uintptr
	trapPush       func(display uintptr)
	trapPop        func(display uintptr)
	widgetWindow   func(widget uintptr) uintptr
	xid            func(window uintptr) uintptr
	defaultRoot    func(dpy uintptr) uintptr
	internAtom     func(dpy uintptr, name string, onlyIfExists int32) uintptr
	getProperty    func(dpy, w, prop uintptr, offset, length int64, del int32, reqType uintptr, actualType *uintptr, actualFormat *int32, nitems, after *uint64, data *unsafe.Pointer) int32
	free           func(p unsafe.Pointer) int32
	sendEvent      func(dpy, w uintptr, propagate int32, mask int64, event unsafe.Pointer) int32
	flush          func(dpy uintptr) int32
}

// loadXlib reports whether GDK runs on an X server. It runs on the main
// thread.
func loadXlib() bool {
	xlib.once.Do(func() {
		gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		gdk, err := purego.Dlopen("libgdk-3.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		gobject, err := purego.Dlopen("libgobject-2.0.so.0", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		x11, err := purego.Dlopen("libX11.so.6", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		for lib, fns := range map[uintptr]map[string]any{
			gtk: {"gtk_widget_get_window": &xlib.widgetWindow},
			gdk: {
				"gdk_display_get_default":                &xlib.gdkDisplay,
				"gdk_x11_display_get_type":               &xlib.x11DisplayType,
				"gdk_x11_display_get_xdisplay":           &xlib.xdisplay,
				"gdk_x11_display_error_trap_push":        &xlib.trapPush,
				"gdk_x11_display_error_trap_pop_ignored": &xlib.trapPop,
				"gdk_x11_window_get_xid":                 &xlib.xid,
			},
			gobject: {"g_type_check_instance_is_a": &xlib.isA},
			x11: {
				"XDefaultRootWindow": &xlib.defaultRoot,
				"XInternAtom":        &xlib.internAtom,
				"XGetWindowProperty": &xlib.getProperty,
				"XFree":              &xlib.free,
				"XSendEvent":         &xlib.sendEvent,
				"XFlush":             &xlib.flush,
			},
		} {
			for name, fn := range fns {
				sym, err := purego.Dlsym(lib, name)
				if err != nil {
					return
				}
				purego.RegisterFunc(fn, sym)
			}
		}
		d := xlib.gdkDisplay()
		xlib.ok = d != 0 && xlib.isA(d, xlib.x11DisplayType())
	})
	return xlib.ok
}

// foregroundSupported: X11 tells the active window; Wayland tells no app
// which app is in front. It runs on the main thread.
func foregroundSupported() bool { return loadXlib() }

func newForegroundProbe() (func() foreground, func()) {
	return func() (fg foreground) {
		waitOnMain(func() {
			if loadXlib() {
				fg = x11Foreground()
			}
		})
		return fg
	}, func() {}
}

// x11Foreground is the app of the active window (EWMH). No active window,
// or the desktop's, is the desktop; docks and panels pass.
func x11Foreground() foreground {
	display := xlib.gdkDisplay()
	dpy := xlib.xdisplay(display)
	xlib.trapPush(display)
	defer xlib.trapPop(display)
	atom := func(name string) uintptr { return xlib.internAtom(dpy, name, 0) }
	active := x11Longs(dpy, xlib.defaultRoot(dpy), atom("_NET_ACTIVE_WINDOW"))
	if len(active) == 0 || active[0] == 0 {
		return foreground{kind: fgDesktop}
	}
	w := uintptr(active[0])
	for _, t := range x11Longs(dpy, w, atom("_NET_WM_WINDOW_TYPE")) {
		switch uintptr(t) {
		case atom("_NET_WM_WINDOW_TYPE_DESKTOP"):
			return foreground{kind: fgDesktop}
		case atom("_NET_WM_WINDOW_TYPE_DOCK"), atom("_NET_WM_WINDOW_TYPE_NOTIFICATION"):
			return foreground{}
		}
	}
	var exe string
	if pid := x11Longs(dpy, w, atom("_NET_WM_PID")); len(pid) > 0 {
		if int(pid[0]) == os.Getpid() {
			return foreground{}
		}
		exe, _ = os.Readlink("/proc/" + strconv.FormatUint(pid[0], 10) + "/exe")
	}
	keys := []string{exe, filepath.Base(exe), string(x11Bytes(dpy, w, atom("_GTK_APPLICATION_ID")))}
	if exe == "" {
		keys[1] = ""
	}
	// WM_CLASS is the instance and the class, each ending in a NUL.
	keys = append(keys, strings.Split(string(x11Bytes(dpy, w, atom("WM_CLASS"))), "\x00")...)
	return foreground{kind: fgApp, keys: lowerKeys(keys...)}
}

// xClientMessage is an XClientMessageEvent of 32-bit items, which Xlib
// hands as longs, padded to an XEvent.
type xClientMessage struct {
	typ         int32
	serial      uint64
	sendEvent   int32
	display     uintptr
	window      uintptr
	messageType uintptr
	format      int32
	data        [5]int64
	_           [12]int64
}

const (
	xClientMessageEvent = 33
	// SubstructureNotifyMask | SubstructureRedirectMask: to the window
	// manager.
	xToWindowManager = 1<<19 | 1<<20
	// The source of a _NET_RESTACK_WINDOW from a pager, and Below.
	ewmhSourcePager = 2
	xStackBelow     = 1
)

// lowerFloating takes the window off the top. Off it, window managers keep
// the window above the one that came in front, so it asks them, as a pager
// does (EWMH), for just below the active window.
func lowerFloating(win *mygo.Window) {
	win.SetAlwaysOnTop(false)
	if !loadXlib() {
		return
	}
	gdkWindow := xlib.widgetWindow(win.NativeHandle())
	if gdkWindow == 0 {
		return
	}
	display := xlib.gdkDisplay()
	dpy := xlib.xdisplay(display)
	xlib.trapPush(display)
	defer xlib.trapPop(display)
	root, self := xlib.defaultRoot(dpy), xlib.xid(gdkWindow)
	active := x11Longs(dpy, root, xlib.internAtom(dpy, "_NET_ACTIVE_WINDOW", 0))
	if len(active) == 0 || active[0] == 0 || uintptr(active[0]) == self {
		return
	}
	ev := xClientMessage{
		typ: xClientMessageEvent, sendEvent: 1, display: dpy, window: self,
		messageType: xlib.internAtom(dpy, "_NET_RESTACK_WINDOW", 0), format: 32,
		data: [5]int64{ewmhSourcePager, int64(active[0]), xStackBelow},
	}
	xlib.sendEvent(dpy, root, 0, xToWindowManager, unsafe.Pointer(&ev))
	xlib.flush(dpy)
}

// x11Bytes reads a property of 8-bit items.
func x11Bytes(dpy, w, prop uintptr) []byte {
	data, n, format := x11Property(dpy, w, prop)
	if data == nil {
		return nil
	}
	defer xlib.free(data)
	if format != 8 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice((*byte)(data), n)...)
}

// x11Longs reads a property of 32-bit items, which Xlib hands as longs.
func x11Longs(dpy, w, prop uintptr) []uint64 {
	data, n, format := x11Property(dpy, w, prop)
	if data == nil {
		return nil
	}
	defer xlib.free(data)
	if format != 32 {
		return nil
	}
	return append([]uint64(nil), unsafe.Slice((*uint64)(data), n)...)
}

func x11Property(dpy, w, prop uintptr) (data unsafe.Pointer, n uint64, format int32) {
	var typ uintptr
	var after uint64
	if xlib.getProperty(dpy, w, prop, 0, 1024, 0, 0, &typ, &format, &n, &after, &data) != 0 {
		return nil, 0, 0
	}
	if n == 0 && data != nil {
		xlib.free(data)
		return nil, 0, 0
	}
	return data, n, format
}

// floatingAppKeys: the names a desktop entry's windows go by, or a
// program's path.
func floatingAppKeys(a store.FloatingApp) []string {
	if !strings.HasSuffix(a.ID, ".desktop") {
		return lowerKeys(a.ID)
	}
	content, _ := os.ReadFile(a.Path)
	return desktopEntryKeys(a.ID, string(content))
}

// floatingAppFromFile takes a desktop entry or a program the user picked.
func floatingAppFromFile(path string) (store.FloatingApp, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return store.FloatingApp{}, false
	}
	if strings.HasSuffix(path, ".desktop") {
		content, err := os.ReadFile(path)
		if err != nil {
			return store.FloatingApp{}, false
		}
		id := filepath.Base(path)
		name := desktopEntryValue(string(content), "Name")
		if name == "" {
			name = strings.TrimSuffix(id, ".desktop")
		}
		return store.FloatingApp{ID: id, Name: name, Path: path}, true
	}
	if info.Mode()&0o111 == 0 {
		return store.FloatingApp{}, false
	}
	return store.FloatingApp{ID: path, Name: filepath.Base(path), Path: path}, true
}
