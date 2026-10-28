package app

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"strings"
)

func (u *settingsUI) mcpAddTools(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	ui.Text(c, tx(l, xMCPPreviewInfo)).FontSize(11).TextColor(c.Theme().TextMuted)
	for i, e := range w.entries {
		if e.selected {
			Panel(c.Key(fmt.Sprint("mcp-preview-", i))).Gap(9).Children(func() {
				ui.Text(c, e.draft.Name).FontSize(14).FontWeight(600)
				title := tx(l, xMCPFetch)
				if e.draft.Transport == "stdio" {
					title = tx(l, xMCPStartFetch)
					ui.Text(c, tx(l, xMCPExecuteWarning)).FontSize(12).TextColor(c.Theme().Warning)
					ui.Text(c, mcpDraftProgram(e.draft.Command)).FontSize(12).Font("monospace")
				}
				if ui.Button(c.Key(fmt.Sprint("mcp-probe-", i)), title).Disabled(e.task.Running || actions.Probe == nil).Clicked() {
					u.probeMCPDraft(actions, e)
				}
				if e.task.Running {
					ui.Spinner(c).Size(14, 14)
					if ui.Button(c.Key(fmt.Sprint("mcp-probe-cancel-", i)), l.Text(l10n.KeyCancel)).Clicked() {
						w.invalidate(e)
					}
				} else if e.task.Err != nil {
					ui.Text(c, u.mcpAddError(e.task.Err)).FontSize(12).TextColor(c.Theme().Danger)
				} else if !e.probed {
					ui.Text(c, tx(l, xMCPNotFetched)).FontSize(12).TextColor(c.Theme().TextMuted)
				} else {
					u.mcpToolList(c, e, i)
				}
				if e.probe.CleanupFailed {
					ui.Text(c, tx(l, xMCPCleanupWarning)).FontSize(12).TextColor(c.Theme().Warning)
				}
			})
		}
	}
	if w.needsVerification() {
		ui.Checkbox(c.Key("mcp-unverified"), &w.unverified, tx(l, xMCPUnverified))
	}
	u.mcpAddNavigation(c, func() { w.step = 0 }, func() { w.step = 2 }, w.needsVerification() && !w.unverified)
}
func (u *settingsUI) mcpToolList(c *ui.Context, e *mcpDraftEntry, index int) {
	l := u.L()
	ui.Text(c, fmt.Sprintf("%s · %s · %s · %d", tx(l, xMCPPreviewSuccess), e.probe.ServerName, e.probe.ProtocolVersion, len(e.probe.Tools))).FontSize(12)
	if e.probe.Truncated {
		ui.Text(c, tx(l, xMCPIncomplete)).FontSize(12).TextColor(c.Theme().Warning)
	}
	if len(e.probe.Tools) == 0 {
		ui.Text(c, tx(l, xMCPZeroTools)).FontSize(12)
		return
	}
	ui.TextInput(c.Key(fmt.Sprint("mcp-tool-search-", index)), &e.search).Label(tx(l, xMCPToolSearch)).Placeholder(tx(l, xMCPToolSearch))
	for i, tool := range e.probe.Tools {
		if e.search != "" && !containsAnyFold(strings.ToLower(e.search), tool.Name, tool.Title, tool.Description) {
			continue
		}
		if ui.Button(c.Key(fmt.Sprintf("mcp-tool-%d-%d", index, i)), tool.Name+"  "+tool.Title).Clicked() {
			e.expanded[i] = !e.expanded[i]
		}
		if e.expanded[i] {
			ui.Text(c, tool.Description).FontSize(11).Selectable()
			for _, p := range tool.Parameters {
				required := ""
				if p.Required {
					required = " *"
				}
				ui.Text(c, p.Name+required+" · "+p.Type+" · "+p.Description).FontSize(11).Selectable()
			}
			ui.Text(c, tool.InputSchema).FontSize(10).Font("monospace").Selectable()
		}
	}
}
