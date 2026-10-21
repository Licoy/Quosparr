package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

// mcpPage lists the provider's user MCP servers under fixed tabs, search
// and selection bar; an open server is a page of its own, without the tabs.
func (u *settingsUI) mcpPage(c *ui.Context) {
	s, actions := u.mcpState(), u.mcpActions()
	if !s.loaded && !s.task.Running && s.task.Err == nil {
		u.startMCPScan(actions)
	}
	server, open := s.find(s.open)
	if s.open != "" && !open {
		s.open = ""
	}
	key := "mcp-list-" + string(s.provider)
	if open {
		key = "mcp-detail-" + s.open
	}
	splitPage(c, key, func() {
		if open {
			u.mcpDetailHeader(c, server, actions)
			u.mcpNotice(c)
			return
		}
		u.mcpProviderTabs(c, actions)
		u.mcpToolbar(c)
		u.mcpNotice(c)
		u.mcpListHeader(c, actions)
	}, func() {
		if open {
			u.mcpDetail(c, server)
			return
		}
		u.mcpList(c, actions)
	})
}

func (u *settingsUI) mcpProviderTabs(c *ui.Context, actions MCPActions) {
	s, l, t := u.mcpState(), u.L(), c.Theme()
	providers := store.Providers()
	selected := slicesIndexProvider(providers, s.provider)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		seg := ui.SegmentedBase(c.Key("mcp-providers"), &selected, len(providers))
		seg.Track.Padding(3).Gap(2).Radius(7).Background(t.Text.Alpha(0.06)).Children(func() {
			for i, provider := range providers {
				part := seg.Segment(i).Padding(5, 10).Gap(6).Radius(5).Label(l.Text(providerKey(provider))).Disabled(s.work.Running)
				color := t.TextMuted
				if i == selected {
					color = t.Text
					part.Background(pal(t).Inset).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12))
				}
				part.Children(func() {
					ProviderIcon(c, provider, 15)
					ui.Text(c, l.Text(providerKey(provider))).FontSize(13).FontWeight(500).TextColor(color).SingleLine()
				})
			}
		})
		if seg.Track.Changed() {
			s.switchProvider(providers[selected])
			u.startMCPScan(actions)
		}
		ui.Spacer(c).Grow(1)
		if s.task.Running || s.work.Running {
			ui.Spinner(c).Size(14, 14)
		}
		if IconButton(c, icRefresh, l.Text(l10n.KeyRefresh), s.task.Running || s.work.Running || actions.Query == nil).Clicked() {
			u.startMCPScan(actions)
		}
	})
}

func slicesIndexProvider(providers []store.Provider, current store.Provider) int {
	for i, provider := range providers {
		if provider == current {
			return i
		}
	}
	return 0
}

func (u *settingsUI) mcpList(c *ui.Context, actions MCPActions) {
	s, l, t := u.mcpState(), u.L(), c.Theme()
	if !s.loaded {
		if s.task.Err != nil {
			ui.Text(c, tx(l, xMCPLoadFailed, s.task.Err.Error())).FontSize(12).TextColor(t.Danger)
		} else {
			skillsLoading(c)
		}
		return
	}
	list := s.filtered(l)
	if len(list) == 0 {
		message := tx(l, xMCPEmpty)
		if len(s.snapshot.Servers) > 0 {
			message = tx(l, xMCPNoMatch)
		}
		skillsEmptyBox(c, message)
		return
	}
	listPanel(c).Children(func() {
		for _, server := range list {
			u.mcpRow(c, server, actions)
		}
	})
}

// mcpToolbar is the note on what is listed and the search.
func (u *settingsUI) mcpToolbar(c *ui.Context) {
	s, l, t := u.mcpState(), u.L(), c.Theme()
	Panel(c).Padding(8, 10).Gap(6).Children(func() {
		ui.TextInput(c.Key("mcp-search"), &s.search).Placeholder(tx(l, xMCPSearchHint)).Label(tx(l, xMCPSearch)).
			FontSize(12).Height(listControlHeight)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icInfo).Size(12, 12).TextColor(t.TextMuted)
			ui.Text(c, tx(l, xMCPScopeInfo)).FontSize(11).TextColor(t.TextMuted).Grow(1).Basis(0)
		})
	})
}

// mcpListHeader reports a failed refresh and holds the selection bar: a
// box selecting every deletable server shown, mixed while only some are,
// the counts, and on the right the batch delete.
func (u *settingsUI) mcpListHeader(c *ui.Context, actions MCPActions) {
	s, l, t := u.mcpState(), u.L(), c.Theme()
	if !s.loaded {
		return
	}
	if s.task.Err != nil {
		ui.Text(c, tx(l, xMCPRefreshFailed, s.task.Err.Error())).FontSize(12).TextColor(t.Danger)
	}
	list := s.filtered(l)
	selected := len(s.selectedServers())
	deletable, all := 0, true
	for _, server := range list {
		if server.Deletable {
			deletable++
			all = all && s.selected[server.ID]
		}
	}
	all = all && deletable > 0
	busy := s.task.Running || s.work.Running || s.stale
	listBar(c).Children(func() {
		label := tx(l, xMCPSelectFiltered, deletable)
		if all {
			label = tx(l, xMCPClearSelection)
		}
		if mixedCheckbox(c, "mcp-select-all", all, selected > 0, label, busy || deletable == 0) {
			s.selected = map[string]bool{}
			if !all {
				for _, server := range list {
					if server.Deletable {
						s.selected[server.ID] = true
					}
				}
			}
		}
		ui.Text(c, tx(l, xMCPSelectionCounts, len(list), selected)).FontSize(12).FontFeatures("tnum").TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
		if SmallPillButton(c, icTrash, tx(l, xMCPDeleteSelected), busy || selected == 0 || actions.Delete == nil).
			Tooltip(tx(l, xMCPDeleteSelected)).Cursor(ui.CursorPointer).Clicked() {
			s.confirmDelete("")
		}
	})
}

// mcpRow is one server: its box, its name and summary, which open it, its
// state, then its actions. The whole row lights up under the pointer.
func (u *settingsUI) mcpRow(c *ui.Context, server mcp.Server, actions MCPActions) {
	s, l, t := u.mcpState(), u.L(), c.Theme()
	checked := s.selected[server.ID]
	listRow(c, "mcp-row-"+server.ID, checked).Children(func() {
		box := ui.Checkbox(c.Key("mcp-check-"+server.ID), &checked, "").Label(tx(l, xMCPSelectOne, server.Name)).Size(18, 18).
			Disabled(s.work.Running || !server.Deletable)
		if server.Deletable {
			box.Cursor(ui.CursorPointer)
		} else {
			box.Tooltip(tx(l, xMCPReadOnly))
		}
		if box.Changed() {
			if checked {
				s.selected[server.ID] = true
			} else {
				delete(s.selected, server.ID)
			}
		}
		button := ui.ButtonBase(c.Key("mcp-open-" + server.ID)).Label(tx(l, xMCPOpenDetails, server.Name)).Grow(1).Basis(0).Cursor(ui.CursorPointer)
		button.Children(func() {
			ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, server.Name).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…").Shrink(1)
					mcpBadges(c, l, server)
				})
				if summary := mcpSummary(server); summary != "" {
					ui.Text(c, summary).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
		})
		if button.Clicked() {
			s.open = server.ID
		}
		u.mcpSwitch(c, server, actions)
		u.mcpActionButtons(c, server, actions)
	})
}

// mcpBadges are the small tags after a server's name.
func mcpBadges(c *ui.Context, l l10n.Localizer, server mcp.Server) {
	t := c.Theme()
	if !server.Enabled {
		settingsBadge(c, nil, tx(l, xMCPDisabled), t.TextMuted)
	}
	if server.Problem != "" {
		settingsBadge(c, icWarnCircle, tx(l, xMCPProblem), t.Warning)
	}
	if !server.Deletable {
		settingsBadge(c, nil, tx(l, xMCPReadOnly), t.TextMuted)
	}
}

// mcpActionButtons show a server's configuration file and delete it, after
// the same confirmation as the selection.
func (u *settingsUI) mcpActionButtons(c *ui.Context, server mcp.Server, actions MCPActions) {
	s, l := u.mcpState(), u.L()
	if actions.Reveal != nil && IconButton(c, icFolder, tx(l, xMCPReveal), false).Cursor(ui.CursorPointer).Clicked() {
		if err := actions.Reveal(server.Path); err != nil {
			s.notice, s.noticeErr = err.Error(), true
		}
	}
	if server.Deletable && actions.Delete != nil &&
		IconButton(c, icTrash, tx(l, xMCPDeleteOne), s.task.Running || s.work.Running || s.stale).Cursor(ui.CursorPointer).Clicked() {
		s.confirmDelete(server.ID)
	}
}

// mcpSwitch turns a Codex or Grok server on or off at once, like a skill
// switch; an entry written in a form that cannot be edited safely keeps a
// disabled switch that says why. Claude has none: Claude Code records MCP
// on/off per project.
func (u *settingsUI) mcpSwitch(c *ui.Context, server mcp.Server, actions MCPActions) {
	if server.Provider == mcp.ProviderClaude {
		return
	}
	s, l := u.mcpState(), u.L()
	on := server.Enabled
	if v, ok := s.pending[server.ID]; ok {
		on = v
	}
	label := tx(l, xMCPEnableServer, server.Name)
	tip := label
	if !server.Toggleable {
		tip = tx(l, xMCPToggleUnsupported)
	}
	usable := server.Toggleable && actions.SetEnabled != nil
	sw := ui.Switch(c.Key("mcp-switch-"+server.ID), &on).Label(label).Tooltip(tip).Size(30, 16).Radius(8).
		Disabled(!usable || s.task.Running || s.work.Running || s.stale)
	if sw.Changed() && usable {
		u.toggleMCP(actions, server, on)
	}
}

// mcpToggleNote says why a server has no working switch.
func mcpToggleNote(l l10n.Localizer, server mcp.Server) string {
	switch {
	case server.Provider == mcp.ProviderClaude:
		return tx(l, xMCPToggleClaude)
	case !server.Toggleable:
		return tx(l, xMCPToggleUnsupported)
	}
	return ""
}

func mcpSummary(server mcp.Server) string {
	return strings.Join(nonEmpty([]string{server.Transport, server.Command, server.URL}), " · ")
}

func (u *settingsUI) mcpNotice(c *ui.Context) {
	s := u.mcpState()
	listNotice(c, "mcp-notice-close", s.notice, s.noticeErr, u.text(l10n.KeyOk), func() { s.notice, s.noticeErr = "", false })
}

// mcpDetailHeader stays above the scrolling server: the quiet back link of
// the panel's pages, the name and the server's actions.
func (u *settingsUI) mcpDetailHeader(c *ui.Context, server mcp.Server, actions MCPActions) {
	s, l := u.mcpState(), u.L()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if detailBackButton(c, l.Text(l10n.KeyBack)).Label(tx(l, xMCPBack)).Tooltip(tx(l, xMCPBack)).Clicked() {
			s.open = ""
			return
		}
		ui.Text(c, server.Name).FontSize(15).FontWeight(650).SingleLine().Ellipsis("…").Shrink(1)
		mcpBadges(c, l, server)
		ui.Spacer(c).Grow(1)
		u.mcpSwitch(c, server, actions)
		u.mcpActionButtons(c, server, actions)
	})
}

// mcpDetail is a server's facts, then its configuration with secrets
// hidden.
func (u *settingsUI) mcpDetail(c *ui.Context, server mcp.Server) {
	l, t := u.L(), c.Theme()
	fields := []struct{ label, value string }{
		{tx(l, xMCPPath), server.Path},
		{tx(l, xMCPTransport), server.Transport},
		{tx(l, xMCPCommand), server.Command},
		{tx(l, xMCPURL), server.URL},
		{tx(l, xMCPStatus), mcpStatusText(l, server)},
		{tx(l, xMCPToggle), mcpToggleNote(l, server)},
	}
	if server.Problem != "" {
		fields = append(fields, struct{ label, value string }{tx(l, xMCPProblem), mcpProblemText(l, server.Problem)})
	}
	Panel(c).Children(func() {
		i := 0
		for _, field := range fields {
			if strings.TrimSpace(field.value) == "" {
				continue
			}
			settingsRule(c, i)
			i++
			ui.Row(c).Gap(12).Padding(7, 0).Children(func() {
				ui.Text(c, field.label).Width(120).FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, field.value).FontSize(11).Grow(1).Basis(0).Selectable()
			})
		}
	})
	if server.Details != "" {
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, tx(l, xMCPRedactedConfig)).FontSize(13).FontWeight(600)
			ui.Column(c).Padding(12).Radius(9).Background(pal(t).Inset).Border(1, pal(t).Line).Children(func() {
				ui.Text(c, server.Details).FontSize(11).Font("monospace").TextColor(t.TextMuted).Selectable()
			})
		})
	}
}

func (s *settingsMCP) find(id string) (mcp.Server, bool) {
	for _, server := range s.snapshot.Servers {
		if server.ID == id {
			return server, true
		}
	}
	return mcp.Server{}, false
}

func mcpStatusText(l l10n.Localizer, server mcp.Server) string {
	status := tx(l, xMCPEnabled)
	if !server.Enabled {
		status = tx(l, xMCPDisabled)
	}
	if !server.Deletable {
		status += " · " + tx(l, xMCPReadOnly)
	}
	return status
}

func mcpProblemText(l l10n.Localizer, problem string) string {
	switch problem {
	case "missingTransport":
		return tx(l, xMCPMissingTransport)
	case "multipleTransports":
		return tx(l, xMCPMultipleTransports)
	case "invalidTransport":
		return tx(l, xMCPInvalidTransport)
	case "missingType":
		return tx(l, xMCPMissingType)
	case "transportMismatch":
		return tx(l, xMCPTransportMismatch)
	default:
		return tx(l, xMCPUnknownProblem)
	}
}
