//go:build !darwin && !windows && !linux

package app

import (
	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/store"
)

func foregroundSupported() bool { return false }

func newForegroundProbe() (func() foreground, func()) {
	return func() foreground { return foreground{} }, func() {}
}

func floatingAppKeys(a store.FloatingApp) []string { return lowerKeys(a.ID) }

func floatingAppFromFile(string) (store.FloatingApp, bool) { return store.FloatingApp{}, false }

func lowerFloating(win *mygo.Window) { win.SetAlwaysOnTop(false) }
