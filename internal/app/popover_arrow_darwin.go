//go:build darwin

package app

import (
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
)

// popoverFill paints the popover's arrow in the panel's own background. An
// opaque panel draws that background only inside its content view; the
// arrow outside it is the system's glass, which stays translucent.
type popoverFill struct {
	view  objc.ID
	color ui.Color // the color shown; Transparent while hidden
}

// syncPanelArrow keeps the popover's arrow in bg, the panel's background,
// which is Transparent while the panel shows the material.
func (a *App) syncPanelArrow(bg ui.Color) {
	if a.popover.popover == 0 {
		return
	}
	content := a.popover.popover.Send(objc.RegisterName("contentViewController")).Send(objc.RegisterName("view"))
	a.popover.fill.sync(content, bg)
}

// sync puts the fill under content, across the popover frame that holds it.
// A glass frame (macOS 26+) clips its subviews to the popover's shape, the
// arrow included; any other frame keeps the system's arrow.
func (f *popoverFill) sync(content objc.ID, bg ui.Color) {
	frame := content.Send(objc.RegisterName("superview"))
	attached := f.view != 0 && f.view.Send(objc.RegisterName("superview")) == frame
	if bg.A == 0 || frame == 0 || (!attached && !hasGlassSubview(frame)) {
		f.hide()
		return
	}
	if f.view == 0 {
		if f.view = newPopoverFillView(); f.view == 0 {
			return
		}
	}
	if !attached {
		f.view.Send(objc.RegisterName("removeFromSuperview"))
		f.view.Send(objc.RegisterName("setFrame:"), objc.Send[trayRect](frame, objc.RegisterName("bounds")))
		const below = int64(-1) // NSWindowBelow
		frame.Send(objc.RegisterName("addSubview:positioned:relativeTo:"), f.view, below, content)
	}
	if f.color == bg {
		return
	}
	// The content draws its colors as sRGB.
	color := objc.ID(objc.GetClass("NSColor")).Send(objc.RegisterName("colorWithSRGBRed:green:blue:alpha:"),
		float64(bg.R)/255, float64(bg.G)/255, float64(bg.B)/255, float64(bg.A)/255)
	f.view.Send(objc.RegisterName("layer")).Send(objc.RegisterName("setBackgroundColor:"), color.Send(objc.RegisterName("CGColor")))
	f.view.Send(objc.RegisterName("setHidden:"), false)
	f.color = bg
}

func (f *popoverFill) hide() {
	if f.view != 0 && f.color != ui.Transparent {
		f.view.Send(objc.RegisterName("setHidden:"), true)
	}
	f.color = ui.Transparent
}

func (f *popoverFill) release() {
	if f.view != 0 {
		f.view.Send(objc.RegisterName("removeFromSuperview"))
		f.view.Send(objc.RegisterName("release"))
	}
	*f = popoverFill{}
}

func hasGlassSubview(frame objc.ID) bool {
	glass := objc.GetClass("NSGlassEffectView")
	if glass == 0 {
		return false
	}
	subviews := frame.Send(objc.RegisterName("subviews"))
	for i := uint64(0); i < objc.Send[uint64](subviews, objc.RegisterName("count")); i++ {
		if objc.Send[bool](subviews.Send(objc.RegisterName("objectAtIndex:"), i), objc.RegisterName("isKindOfClass:"), glass) {
			return true
		}
	}
	return false
}

// newPopoverFillView returns a hidden fill, or 0 where AppKit refuses its
// class.
func newPopoverFillView() objc.ID {
	const name = "RunwayPopoverFill"
	class := objc.GetClass(name)
	if class == 0 {
		var err error
		// Clicks reach the frame under it, as they did before.
		class, err = objc.RegisterClass(name, objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
			{Cmd: objc.RegisterName("hitTest:"), Fn: func(objc.ID, objc.SEL, trayPoint) objc.ID { return 0 }},
		})
		if err != nil {
			return 0
		}
	}
	view := objc.ID(class).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithFrame:"), trayRect{})
	view.Send(objc.RegisterName("setWantsLayer:"), true)
	view.Send(objc.RegisterName("setAutoresizingMask:"), uint64(2|16)) // width, height sizable
	view.Send(objc.RegisterName("setHidden:"), true)
	return view
}
