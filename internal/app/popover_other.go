//go:build !darwin

package app

import "github.com/egoist/mygo/ui"

type panelPopoverState struct{}

func observePanelPopover(*App) func()   { return func() {} }
func (a *App) matchPanelAppearance()    {}
func (a *App) syncPanelArrow(ui.Color)  {}
func releasePanelPopover(*App)          {}
func (a *App) watchPanelDismiss() error { return nil }
