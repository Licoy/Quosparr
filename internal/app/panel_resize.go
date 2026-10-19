package app

import (
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/store"
)

type panelResizeState struct {
	start, pointerY, delta, height float64
	dragging, moved                bool
	// top puts the handle on the top edge: the flyout hangs above a tray
	// icon, as on a Windows taskbar at the bottom of the screen, so its
	// bottom edge stays by the icon while the top edge moves.
	top                                           bool
	x, y, bottom, chrome, available, screenHeight int
}

func panelResizeHandle(c *ui.Context, a *App) {
	s := &a.panelResize
	grip := ui.Box(c).Absolute().Left(0).Right(0).Height(10).Cursor(ui.CursorResizeNS).Role(ui.RoleNone).Label("panel-resize-handle")
	if s.top {
		grip.Top(0)
	} else {
		grip.Bottom(0)
	}
	_, _, held := grip.Dragged()
	pointerY := a.panelResizePointerY(c)
	if held {
		if !s.dragging {
			_, height := c.Size()
			*s = panelResizeState{start: float64(height), pointerY: pointerY, dragging: true, top: s.top}
			if a.panel != nil {
				bounds := a.panel.Bounds()
				s.x, s.y, s.bottom = bounds.X, bounds.Y, bounds.Y+bounds.Height
				display := mygo.Screen.DisplayMatching(bounds)
				area := display.WorkArea
				s.screenHeight = display.Bounds.Height
				s.chrome = max(0, bounds.Height-int(height))
				s.available = max(1, area.Y+area.Height-bounds.Y-s.chrome)
				if s.top {
					s.available = max(1, s.bottom-area.Y-s.chrome)
				}
			}
		}
		delta := pointerY - s.pointerY
		if delta != s.delta {
			s.delta = delta
			s.moved = true
			a.resizePanelHeight(s.start+s.growth(), false)
		}
	} else if s.dragging {
		s.dragging = false
		// A native frame can coalesce the last move with mouse-up, when
		// Dragged no longer reports a delta; keep that final pointer position.
		s.delta = pointerY - s.pointerY
		if s.moved || s.delta != 0 {
			a.resizePanelHeight(s.start+s.growth(), true)
		}
	}
	color := ui.Transparent
	if grip.Hovered() || held {
		color = c.Theme().TextMuted.Alpha(0.55)
	}
	grip.AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Box(c).Size(36, 3).Radius(2).Background(color) })
}

// growth is the height change of the drag: up grows a top edge, down a bottom one.
func (s *panelResizeState) growth() float64 {
	if s.top {
		return -s.delta
	}
	return s.delta
}

// panelResizePointerY reads the pointer in screen space while the top edge
// moves the window under it; page coordinates would shift with each resize.
func (a *App) panelResizePointerY(c *ui.Context) float64 {
	if a.panelResize.top && a.panel != nil {
		return float64(mygo.Screen.CursorScreenPoint().Y)
	}
	_, y, _ := c.Root().PointerPosition()
	return float64(y)
}

// syncPanelResizeEdge picks the edge away from the tray icon once the
// flyout is placed; Linux reports no icon bounds and keeps the bottom edge.
func (a *App) syncPanelResizeEdge() {
	if a.panel == nil || a.tray == nil {
		return
	}
	icon := a.tray.Bounds()
	if icon.Width == 0 && icon.Height == 0 {
		return
	}
	bounds := a.panel.Bounds()
	a.panelResize.top = bounds.Y+bounds.Height/2 < icon.Y+icon.Height/2
}

func (a *App) resizePanelHeight(proposed float64, persist bool) {
	s := &a.panelResize
	height := clampPanelHeight(proposed, s.screenHeight, s.available)
	s.height = height
	if panel := a.panel; panel != nil {
		x, y := s.x, s.y
		if s.top {
			y = s.bottom - s.chrome - int(height)
		}
		// Defer native geometry until this render pass ends; keep the edge
		// by the tray icon in place while only the other edge moves.
		mygo.App.WhenReady(func() {
			if a.panel != panel || panel.IsDestroyed() || a.panelResize.height != height {
				return
			}
			panel.SetContentSize(panelWidth, int(height))
			panel.SetPosition(x, y)
		})
	}
	if persist {
		a.settings.SetMainPanelHeight(height)
	}
}

func panelHeightLimit(screenHeight, available int) float64 {
	upper := math.Inf(1) // Headless UI tests have no display constraint.
	if screenHeight > 0 {
		upper = math.Floor(float64(screenHeight) * 0.75)
	}
	if available > 0 {
		upper = min(upper, float64(available-16))
	}
	return max(store.MinMainPanelHeight, upper)
}

func clampPanelHeight(proposed float64, screenHeight, available int) float64 {
	height := store.ClampMainPanelHeight(proposed)
	if height == store.DefaultMainPanelHeight {
		// Before native display geometry is available, layout uses the minimum.
		height = max(store.MinMainPanelHeight, math.Floor(float64(screenHeight)*0.65))
	}
	return math.Round(min(panelHeightLimit(screenHeight, available), height))
}
