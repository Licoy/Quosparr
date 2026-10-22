package app

import (
	"log"
	"reflect"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// TrayRender is what the status item shows: the coordinator's components, in
// the chosen style, and ApplyTray draws it.
type TrayRender struct {
	Style      store.StatusBarStyle
	Battery    store.BatteryOrientation
	HideIcons  bool
	RingValues bool
	Components []runway.StatusBarComponent
	// ToolTip is the hover text.
	ToolTip string
}

// The application has one status item; keep only its last successful draw
// This state is accessed on MyGo's main thread, never from refresh workers
var lastAppliedTray struct {
	tray   *mygo.Tray
	render TrayRender
	light  bool
}

func sameTrayRender(a, b TrayRender) bool { return reflect.DeepEqual(a, b) }

func rememberTray(tray *mygo.Tray, r TrayRender, light bool) {
	stored := r
	stored.Components = make([]runway.StatusBarComponent, len(r.Components))
	for i, comp := range r.Components {
		comp.Cells = append([]runway.StatusBarCell(nil), comp.Cells...)
		stored.Components[i] = comp
	}
	lastAppliedTray.tray, lastAppliedTray.render, lastAppliedTray.light = tray, stored, light
}

// ApplyTray renders the complete menu bar content, or the desktop application
// icon with tooltip; identical content reuses the last successful draw
// It may be called from any goroutine
func ApplyTray(tray *mygo.Tray, r TrayRender) {
	if tray == nil {
		return
	}
	mygo.RunOnMain(func() {
		if r.Style == "" && trayIsTemplate() {
			// No status yet: the app icon until the first snapshot.
			icon, template := trayIcon()
			if err := tray.SetIcon(icon, template); err != nil {
				log.Printf("tray: set icon failed: %v", err)
			}
			tray.SetTitle("")
			tray.SetToolTip(trayTooltip(r.ToolTip))
			return
		}
		// macOS tints every foreground itself, independently of app appearance
		light := trayIsTemplate() || trayLight()
		if lastAppliedTray.tray == tray && lastAppliedTray.light == light && sameTrayRender(lastAppliedTray.render, r) {
			return
		}
		if err := setTrayContent(tray, r, light); err != nil {
			log.Printf("tray: layout failed: %v", err)
			return
		}
		tray.SetToolTip(trayTooltip(r.ToolTip))
		rememberTray(tray, r, light)
	})
}

func trayTooltip(tip string) string {
	if tip == "" {
		return "Quosparr"
	}
	return tip
}
