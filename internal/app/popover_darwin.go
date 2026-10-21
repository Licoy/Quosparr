//go:build darwin

package app

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"unsafe"
)

// panelPopoverState keeps the NSPopover mygo creates for the panel, so its
// appearance can follow the app's.
type panelPopoverState struct {
	popover     objc.ID
	stopDismiss func()
	fill        popoverFill
}

var popoverWillShowNotification = func() objc.ID {
	lib, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	sym, err := purego.Dlsym(lib, "NSPopoverWillShowNotification")
	if err != nil {
		panic(err)
	}
	// sym is the address of an exported NSString* constant.
	return *(*objc.ID)(*(*unsafe.Pointer)(unsafe.Pointer(&sym)))
}()

// observePanelPopover scopes the public notification to NewFlyout's
// synchronous creation and keeps that popover until its window closes.
func observePanelPopover(a *App) func() {
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(objc.RegisterName("defaultCenter"))
	block := objc.NewBlock(func(_ objc.Block, notification objc.ID) {
		popover := notification.Send(objc.RegisterName("object"))
		popover.Send(objc.RegisterName("retain"))
		if a.popover.popover != 0 {
			a.popover.popover.Send(objc.RegisterName("release"))
		}
		a.popover.popover = popover
		// Dismissal follows the status toggle and outside-click monitors,
		// including desktop clicks that do not deactivate an accessory app.
		popover.Send(objc.RegisterName("setBehavior:"), int64(0)) // applicationDefined
		popover.Send(objc.RegisterName("setAnimates:"), !a.settings.Prefs().DisableMainPanelAnimation)
	})
	token := center.Send(objc.RegisterName("addObserverForName:object:queue:usingBlock:"),
		popoverWillShowNotification, objc.ID(0), objc.ID(0), block)
	return func() {
		center.Send(objc.RegisterName("removeObserver:"), token)
		block.Release()
	}
}

// matchPanelAppearance makes the popover follow the app's appearance, not
// the menu bar's.
func (a *App) matchPanelAppearance() {
	if a.popover.popover == 0 {
		return
	}
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	a.popover.popover.Send(objc.RegisterName("setAppearance:"), app.Send(objc.RegisterName("effectiveAppearance")))
}

func releasePanelPopover(a *App) {
	if a.popover.stopDismiss != nil {
		a.popover.stopDismiss()
	}
	a.popover.fill.release()
	if a.popover.popover != 0 {
		a.popover.popover.Send(objc.RegisterName("release"))
	}
	a.popover = panelPopoverState{}
}
