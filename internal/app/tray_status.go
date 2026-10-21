package app

import "github.com/licoy/quosparr/internal/runway"

// TrayFromStatus hands the coordinator's status bar to the tray renderer.
func TrayFromStatus(v runway.StatusBarView) TrayRender {
	return TrayRender{Style: v.Style, Battery: v.Battery, HideIcons: v.HideIcons, Components: v.Components, ToolTip: v.Tooltip}
}
