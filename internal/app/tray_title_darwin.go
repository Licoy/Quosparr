//go:build darwin

package app

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// trayIsTemplate: the menu bar tints a black-and-transparent icon itself.
func trayIsTemplate() bool { return true }

func trayLight() bool {
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	appearance := app.Send(objc.RegisterName("effectiveAppearance"))
	if button := trayButton(); button != 0 {
		appearance = button.Send(objc.RegisterName("effectiveAppearance"))
	}
	names := objc.ID(objc.GetClass("NSArray")).Send(objc.RegisterName("arrayWithObjects:"), nsStr("NSAppearanceNameAqua"), nsStr("NSAppearanceNameDarkAqua"), objc.ID(0))
	match := appearance.Send(objc.RegisterName("bestMatchFromAppearancesWithNames:"), names)
	return !objc.Send[bool](match, objc.RegisterName("isEqualToString:"), nsStr("NSAppearanceNameDarkAqua"))
}

// MyGo's tray images are normally 18 pt high; these complete contents use
// the Swift status item's 22 pt coordinate system, including two text rows
func setTrayContent(tray *mygo.Tray, r TrayRender, _ bool) error {
	foreground, err := renderTrayImage(r, trayImageOptions{light: true, foreground: true})
	if err != nil {
		return err
	}
	colors, err := renderTrayImage(r, trayImageOptions{colorsOnly: true})
	if err != nil {
		return err
	}
	tray.SetTitle("")
	button := trayButton()
	if button == 0 {
		return fmt.Errorf("status item button unavailable")
	}
	image, err := trayNativeImage(foreground)
	if err != nil {
		return err
	}
	defer image.Send(objc.RegisterName("release"))
	s := objc.Send[traySize](image, objc.RegisterName("size"))
	if s.Height > 0 {
		image.Send(objc.RegisterName("setSize:"), traySize{Width: s.Width * 22 / s.Height, Height: 22})
		image.Send(objc.RegisterName("setTemplate:"), true)
		// NSStatusBarButton sizes the highlight/hit area when its image is
		// assigned; changing the 18pt framework image afterwards is too late.
		button.Send(objc.RegisterName("setImage:"), image)
		return setTrayColorOverlay(button, colors)
	}
	return fmt.Errorf("status item image has no height")
}

// trayButton is the button of the app's menu bar item: the content of a
// status bar window, which mygo has send mygoTrayClicked:. 0 when none.
func trayButton() objc.ID {
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	windows := app.Send(objc.RegisterName("windows"))
	action := objc.RegisterName("mygoTrayClicked:")
	n := objc.Send[uint](windows, objc.RegisterName("count"))
	for i := range n {
		w := windows.Send(objc.RegisterName("objectAtIndex:"), i)
		if b := findTrayButton(w.Send(objc.RegisterName("contentView")), action); b != 0 {
			return b
		}
	}
	return 0
}

// findTrayButton looks for the button with action in view and its subviews.
func findTrayButton(view objc.ID, action objc.SEL) objc.ID {
	if view == 0 {
		return 0
	}
	if objc.Send[bool](view, objc.RegisterName("respondsToSelector:"), objc.RegisterName("action")) &&
		objc.Send[objc.SEL](view, objc.RegisterName("action")) == action {
		return view
	}
	subviews := view.Send(objc.RegisterName("subviews"))
	n := objc.Send[uint](subviews, objc.RegisterName("count"))
	for i := range n {
		if b := findTrayButton(subviews.Send(objc.RegisterName("objectAtIndex:"), i), action); b != 0 {
			return b
		}
	}
	return 0
}

func nsStr(s string) objc.ID {
	str := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("alloc")).
		Send(objc.RegisterName("initWithUTF8String:"), s)
	return str.Send(objc.RegisterName("autorelease"))
}
