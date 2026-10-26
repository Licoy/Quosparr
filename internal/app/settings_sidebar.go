package app

import (
	"fmt"
	"runtime"
	"strconv"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/runway"
)

// settingsVibrancy is the material the sidebar shows, as System Settings'
// does: macOS only, since a Windows window with one loses its menu bar.
func settingsVibrancy() mygo.Vibrancy {
	if runtime.GOOS == "darwin" {
		return mygo.VibrancySidebar
	}
	return mygo.VibrancyNone
}

func (u *settingsUI) shell(c *ui.Context, content func(*ui.Context)) {
	t := c.Theme()
	material := c.Vibrancy()
	if material {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		side := ui.Column(c).Width(settingsSidebarWidth).BorderWidth(0, 1, 0, 0).BorderColor(pal(t).Line)
		if !material {
			side.Background(pal(t).Sidebar)
		}
		side.Children(func() { u.sidebar(c) })
		ui.Column(c).Grow(1).Basis(0).Background(t.Background).Children(func() {
			u.pageHeader(c)
			content(c)
		})
	})
}

// sidebarGroupStarts are the tabs after which the sidebar leaves a gap.
var sidebarGroupStarts = map[settingsTab]bool{tabSessions: true, tabModelPricing: true}

func (u *settingsUI) sidebar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	top := float32(14)
	if bar.Left > 0 && bar.Height > 0 {
		top = max(bar.Height, 52)
	}
	ui.Row(c).Height(top).DragWindow()
	ui.Row(c).Padding(0, 16, 14, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Image(c, appIconBitmap()).Size(36, 36)
		ui.Column(c).Gap(1).Children(func() {
			ui.Text(c, "Quosparr").FontSize(13).FontWeight(600).SingleLine()
			ui.Text(c, u.opts.Version).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	})
	ui.Scroll(c.Key("settings-sidebar")).Grow(1).Padding(0, 10, 10, 10).Gap(2).Children(func() {
		for _, tab := range settingsTabs {
			if sidebarGroupStarts[tab] {
				ui.Box(c).Height(10)
			}
			if u.sidebarItem(c, tab) && u.tab != tab {
				u.tab = tab
				u.setProxyMsg("", false)
			}
		}
	})
}

func (u *settingsUI) sidebarItem(c *ui.Context, tab settingsTab) bool {
	t := c.Theme()
	title, selected := tab.title(u.L()), u.tab == tab
	b := ui.ButtonBase(c.Key(fmt.Sprintf("tab-%d", tab))).Label(title).Padding(5, 6).Gap(8).Radius(6).AlignItems(ui.Center)
	b.Role(ui.RoleTab).Selected(selected)
	col := t.Text
	if selected {
		b.Background(t.Accent)
		col = t.AccentText
	} else if b.Hovered() {
		b.Background(pal(t).Hover)
	}
	b.Children(func() {
		if selected {
			// A tile in the tint could vanish into an accent of the same hue.
			ui.Row(c).Size(20, 20).Radius(5.6).Background(ui.RGBA(255, 255, 255, 0.24)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, tab.icon()).Size(11.6, 11.6).TextColor(t.AccentText)
			})
		} else {
			Tile(c, tab.icon(), tab.tint(), 20, true)
		}
		ui.Text(c, title).FontSize(13).TextColor(col).SingleLine().Grow(1)
		if badge := u.tabBadge(tab); badge != "" {
			badgeCol := t.TextMuted
			if selected {
				badgeCol = t.AccentText
			}
			ui.Text(c, badge).FontSize(13).TextColor(badgeCol).SingleLine()
		}
	})
	return b.Clicked()
}

// tabBadge is the trailing count of a sidebar tab: the managed accounts of
// both providers on the accounts tab, nothing elsewhere.
func (u *settingsUI) tabBadge(tab settingsTab) string {
	if tab != tabAccounts {
		return ""
	}
	rt := currentRuntime()
	if rt == nil {
		return ""
	}
	return strconv.Itoa(managedAccountCount(rt.svc.Snapshot()))
}

// managedAccountCount is how many accounts both providers manage.
func managedAccountCount(snap runway.Snapshot) int {
	return len(snap.Accounts.Sidebar) + len(snap.Grok.Accounts.Sidebar)
}

func (u *settingsUI) pageHeader(c *ui.Context) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, 52)).Padding(0, bar.Right+16, 0, 24).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, u.tab.title(u.L())).FontSize(15).FontWeight(700).SingleLine().Grow(1).Basis(0)
	})
}
