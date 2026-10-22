package app

import (
	"os"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// nativeFloatingBelowFront reports whether the window is below the front
// window another app has on screen; known is false without one.
func nativeFloatingBelowFront(win *mygo.Window) (below, known bool) {
	own := objc.Send[int](objc.ID(win.NativeHandle()), objc.RegisterName("windowNumber"))
	self := int32(os.Getpid())
	ownAt, foreignAt, i := -1, -1, 0
	macNormalWindowsOnScreen(func(owner, number int32) bool {
		switch {
		case int(number) == own:
			ownAt = i
		case owner != self && foreignAt < 0:
			foreignAt = i
		}
		i++
		return true
	})
	return ownAt > foreignAt, ownAt >= 0 && foreignAt >= 0
}
