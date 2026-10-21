//go:build !darwin

package app

import (
	"github.com/egoist/mygo"
)

// trayIsTemplate: only macOS tints a black icon to the menu bar.
func trayIsTemplate() bool { return false }

func trayLight() bool { return taskbarLight() }

func setTrayContent(tray *mygo.Tray, r TrayRender, light bool) error {
	icon, err := TrayImage(r, light, true)
	if err != nil {
		return err
	}
	if err := tray.SetIcon(icon, false); err != nil {
		return err
	}
	tray.SetTitle("")
	return nil
}
