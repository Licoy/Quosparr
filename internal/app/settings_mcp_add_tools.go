package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
)

// mcpAddTools is the second step: a card per included server that fetches
// its tools on request, then the consent to add what was not verified.
func (u *settingsUI) mcpAddTools(c *ui.Context, actions MCPActions) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	ui.Column(c).Gap(4).Children(func() {
		mcpInfoLine(c, icInfo, tx(l, xMCPPreviewInfo), t.TextMuted)
		mcpInfoLine(c, icKey, tx(l, xMCPOAuthHint), t.TextMuted)
	})
	for _, e := range w.entries {
		if e.selected {
			u.mcpToolCard(c, e, actions)
		}
	}
	if !w.needsVerification() {
		return
	}
	ui.Column(c).Gap(8).Padding(12, 14).Radius(10).Background(t.Warning.Alpha(0.10)).Children(func() {
		mcpInfoLine(c, icWarnCircle, tx(l, xMCPUnverifiedStatus), t.Warning)
		ui.Checkbox(c.Key("mcp-unverified"), &w.unverified, tx(l, xMCPUnverified))
	})
}

// mcpToolCard is one server: what it is and how its fetch went, its fetch
// button, and the tools it reported.
func (u *settingsUI) mcpToolCard(c *ui.Context, e *mcpDraftEntry, actions MCPActions) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	Panel(c.Key(fmt.Sprint("mcp-preview-", e.id))).Padding(12, 14).Gap(10).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			Tile(c, mcpTransportIcon(e.transport), TintTeal, 30, false)
			ui.Column(c).Gap(3).Grow(1).Basis(0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, strings.TrimSpace(e.name)).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…").Shrink(1)
					settingsBadge(c, nil, mcpTransportName(l, e.transport), t.TextMuted)
				})
				text, ink := mcpProbeStatus(l, e), t.TextMuted
				switch {
				case e.task.Err != nil:
					ink = t.Danger
				case e.probed && e.probe.Truncated:
					ink = t.Warning
				case e.probed:
					ink = t.Success
				}
				ui.Text(c, text).FontSize(11).TextColor(ink).SingleLine().Ellipsis("…")
			})
			if e.task.Running {
				ui.Spinner(c).Size(14, 14)
				if PillButton(c.Key(fmt.Sprint("mcp-probe-cancel-", e.id)), nil, l.Text(l10n.KeyCancel), false).Clicked() {
					w.invalidate(e)
				}
				return
			}
			title := tx(l, xMCPFetch)
			switch {
			case e.probed || e.task.Err != nil:
				title = tx(l, xMCPRefetch)
			case e.transport == "stdio":
				title = tx(l, xMCPStartFetch)
			}
			if PillButton(c.Key(fmt.Sprint("mcp-probe-", e.id)), icToolbox, title, actions.Probe == nil).Clicked() {
				u.probeMCPDraft(actions, e)
			}
		})
		if e.transport == "stdio" && !e.probed && !e.task.Running {
			mcpInfoLine(c, icTerminal, tx(l, xMCPExecuteWarning, mcpDraftProgram(e.command)), t.Warning)
		}
		if e.task.Err != nil {
			listNotice(c, "", u.mcpAddError(e.task.Err), true, "", nil)
		}
		if e.probe.CleanupFailed {
			mcpInfoLine(c, icWarnCircle, tx(l, xMCPCleanupWarning), t.Warning)
		}
		if e.probed {
			u.mcpToolList(c, e)
		}
	})
}

// mcpProbeStatus is the line under a server's name: not fetched yet,
// connecting, failed, or what the server said about itself.
func mcpProbeStatus(l l10n.Localizer, e *mcpDraftEntry) string {
	switch {
	case e.task.Running:
		return tx(l, xMCPFetching)
	case e.task.Err != nil:
		return tx(l, xMCPProbeError)
	case !e.probed:
		return tx(l, xMCPNotFetched)
	}
	server := strings.TrimSpace(e.probe.ServerName + " " + e.probe.ServerVersion)
	parts := []string{tx(l, xMCPToolCount, len(e.probe.Tools)), server}
	if e.probe.ProtocolVersion != "" {
		parts = append(parts, tx(l, xMCPProtocol, e.probe.ProtocolVersion))
	}
	return strings.Join(nonEmpty(parts), " · ")
}

// mcpToolList lists the fetched tools, searchable when there are many; a
// click opens one's description, parameters and input schema.
func (u *settingsUI) mcpToolList(c *ui.Context, e *mcpDraftEntry) {
	l, t := u.L(), c.Theme()
	if e.probe.Truncated {
		mcpInfoLine(c, icWarnCircle, tx(l, xMCPIncomplete), t.Warning)
	}
	if len(e.probe.Tools) == 0 {
		mcpInfoLine(c, icInfo, tx(l, xMCPZeroTools), t.TextMuted)
		return
	}
	if len(e.probe.Tools) > 6 {
		ui.TextInput(c.Key(fmt.Sprint("mcp-tool-search-", e.id)), &e.search).Label(tx(l, xMCPToolSearch)).Placeholder(tx(l, xMCPToolSearch)).
			FontSize(12).Height(listControlHeight)
	}
	query := strings.ToLower(strings.TrimSpace(e.search))
	ui.Column(c).Gap(2).Padding(4).Radius(9).Background(pal(t).Inset).Border(1, pal(t).Line).Children(func() {
		for i, tool := range e.probe.Tools {
			if query != "" && !containsAnyFold(query, tool.Name, tool.Title, tool.Description) {
				continue
			}
			open := e.expanded[i]
			b := ui.ButtonBase(c.Key(fmt.Sprintf("mcp-tool-%d-%d", e.id, i))).Label(tool.Name).Expanded(open).Gap(8).Padding(6, 8).
				Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(pal(t).Hover)
			}
			b.Children(func() {
				chevron := icChevRight
				if open {
					chevron = icChevDown
				}
				ui.Icon(c, chevron).Size(11, 11).TextColor(t.TextMuted)
				ui.Text(c, tool.Name).FontSize(12).FontWeight(600).Font("monospace").SingleLine().Shrink(0)
				summary := tool.Title
				if summary == "" {
					summary = tool.Description
				}
				ui.Text(c, summary).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).Basis(0)
			})
			if b.Clicked() {
				e.expanded[i] = !open
			}
			if open {
				mcpToolDetail(c, l, tool)
			}
		}
	})
}

// mcpToolDetail is an opened tool, indented under its name.
func mcpToolDetail(c *ui.Context, l l10n.Localizer, tool mcp.Tool) {
	t := c.Theme()
	ui.Column(c).Gap(8).Padding(2, 8, 10, 27).Children(func() {
		if tool.Description != "" {
			ui.Text(c, tool.Description).FontSize(12).Selectable()
		}
		if len(tool.Parameters) > 0 {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, tx(l, xMCPToolParams)).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
				for _, p := range tool.Parameters {
					ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
						name := p.Name
						if p.Required {
							name += "*"
						}
						ui.Text(c, name).FontSize(11).Font("monospace").FontWeight(600).Shrink(0)
						if p.Type != "" {
							settingsBadge(c, nil, p.Type, t.TextMuted)
						}
						ui.Text(c, p.Description).FontSize(11).TextColor(t.TextMuted).Grow(1).Basis(0).Selectable()
					})
				}
			})
		}
		if tool.InputSchema != "" {
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, tx(l, xMCPInputSchema)).FontSize(11).FontWeight(600).TextColor(t.TextMuted)
				ui.Column(c).Padding(8, 10).Radius(7).Background(pal(t).Card).Children(func() {
					ui.Text(c, tool.InputSchema).FontSize(10.5).Font("monospace").TextColor(t.TextMuted).Selectable()
				})
			})
		}
	})
}

// mcpInfoLine is a short note led by an icon, in one color.
func mcpInfoLine(c *ui.Context, icon *ui.SVG, text string, ink ui.Color) {
	ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icon).Size(12, 12).TextColor(ink).Margin(2, 0, 0, 0)
		ui.Text(c, text).FontSize(11).TextColor(ink).Grow(1).Basis(0)
	})
}
