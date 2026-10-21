//go:build darwin

package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
)

// AppKit must initialize on the process main thread, before Go test workers
func init() {
	if os.Getenv("RUNWAY_POPOVER_FILL_CHECK") == "1" {
		if err := verifyPopoverFill(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

func TestPopoverFillPaintsGlassFrameUnderContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "RUNWAY_POPOVER_FILL_CHECK=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("popover fill: %v: %s", err, out)
	}
}

func verifyPopoverFill() error {
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	newView := func(class string, w, h float64) objc.ID {
		return objc.ID(objc.GetClass(class)).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithFrame:"), trayRect{Size: traySize{Width: w, Height: h}})
	}
	plain := newView("NSView", 226, 146)
	defer plain.Send(objc.RegisterName("release"))
	content := newView("NSView", 200, 120)
	defer content.Send(objc.RegisterName("release"))
	plain.Send(objc.RegisterName("addSubview:"), content)
	var f popoverFill
	defer f.release()
	f.sync(content, ui.RGB(30, 30, 32))
	if f.view != 0 {
		return fmt.Errorf("a frame without glass got a fill")
	}
	content.Send(objc.RegisterName("removeFromSuperview"))
	if objc.GetClass("NSGlassEffectView") == 0 {
		return nil // before macOS 26, popovers keep the system's arrow
	}

	frame := newView("NSView", 226, 146)
	defer frame.Send(objc.RegisterName("release"))
	glass := newView("NSGlassEffectView", 226, 146)
	defer glass.Send(objc.RegisterName("release"))
	frame.Send(objc.RegisterName("addSubview:"), glass)
	frame.Send(objc.RegisterName("addSubview:"), content)
	bg := ui.RGB(30, 30, 32)
	f.sync(content, bg)
	subviews := frame.Send(objc.RegisterName("subviews"))
	if objc.Send[uint64](subviews, objc.RegisterName("count")) != 3 ||
		subviews.Send(objc.RegisterName("objectAtIndex:"), uint64(0)) != glass ||
		subviews.Send(objc.RegisterName("objectAtIndex:"), uint64(1)) != f.view ||
		subviews.Send(objc.RegisterName("objectAtIndex:"), uint64(2)) != content {
		return fmt.Errorf("fill is not between the glass and the content")
	}
	if r := objc.Send[trayRect](f.view, objc.RegisterName("frame")); r.Size.Width != 226 || r.Size.Height != 146 {
		return fmt.Errorf("fill frame = %+v, want the frame's bounds", r)
	}
	if err := checkFill(f.view, bg); err != nil {
		return err
	}

	light := ui.RGB(246, 246, 248)
	f.sync(content, light)
	if err := checkFill(f.view, light); err != nil {
		return err
	}
	if objc.Send[uint64](frame.Send(objc.RegisterName("subviews")), objc.RegisterName("count")) != 3 {
		return fmt.Errorf("a second sync added another fill")
	}
	f.sync(content, ui.Transparent)
	if !objc.Send[bool](f.view, objc.RegisterName("isHidden")) {
		return fmt.Errorf("the material shows under a visible fill")
	}
	f.sync(content, bg)
	if err := checkFill(f.view, bg); err != nil {
		return err
	}
	f.release()
	if objc.Send[uint64](frame.Send(objc.RegisterName("subviews")), objc.RegisterName("count")) != 2 {
		return fmt.Errorf("release left the fill in the frame")
	}
	return nil
}

func checkFill(view objc.ID, want ui.Color) error {
	if objc.Send[bool](view, objc.RegisterName("isHidden")) {
		return fmt.Errorf("fill is hidden")
	}
	cg := view.Send(objc.RegisterName("layer")).Send(objc.RegisterName("backgroundColor"))
	srgb := objc.ID(objc.GetClass("NSColorSpace")).Send(objc.RegisterName("sRGBColorSpace"))
	color := objc.ID(objc.GetClass("NSColor")).Send(objc.RegisterName("colorWithCGColor:"), cg).Send(objc.RegisterName("colorUsingColorSpace:"), srgb)
	got := [4]float64{
		objc.Send[float64](color, objc.RegisterName("redComponent")),
		objc.Send[float64](color, objc.RegisterName("greenComponent")),
		objc.Send[float64](color, objc.RegisterName("blueComponent")),
		objc.Send[float64](color, objc.RegisterName("alphaComponent")),
	}
	for i, v := range [4]uint8{want.R, want.G, want.B, want.A} {
		if math.Abs(got[i]*255-float64(v)) > 0.5 {
			return fmt.Errorf("fill color = %v, want %v", got, want)
		}
	}
	return nil
}
