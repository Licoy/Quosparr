package app

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
)

func (u *settingsUI) mcpAddPage(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	splitPage(c, "mcp-add-page", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if detailBackButton(c, l.Text(l10n.KeyBack)).Clicked() {
				u.closeMCPAdd()
			}
			ui.Text(c, tx(l, xMCPAdd)).FontSize(15).FontWeight(650)
			ui.Spacer(c).Grow(1)
			if w.writing || w.task.Running {
				ui.Spinner(c).Size(14, 14)
			}
		})
		titles := []textKey{xMCPFill, xMCPTools, xMCPTargets, xMCPReview, xMCPResults}
		ui.Text(c, fmt.Sprintf("%d / 5 · %s", w.step+1, tx(l, titles[w.step]))).FontSize(12).TextColor(c.Theme().TextMuted)
	}, func() {
		if !w.open {
			return
		}
		if w.err != nil {
			ui.Text(c, u.mcpAddError(w.err)).FontSize(12).TextColor(c.Theme().Danger)
		}
		switch w.step {
		case 0:
			u.mcpAddFill(c, actions)
		case 1:
			u.mcpAddTools(c, actions)
		case 2:
			u.mcpAddTargets(c, actions)
		case 3:
			u.mcpAddReview(c, actions)
		case 4:
			u.mcpAddResults(c, actions)
		}
	})
}
func (u *settingsUI) mcpAddFill(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	ui.Text(c, tx(l, xMCPMemoryOnly)).FontSize(11).TextColor(c.Theme().TextMuted)
	ui.Checkbox(c.Key("mcp-json-mode"), &w.importJSON, tx(l, xMCPJSON))
	if w.importJSON {
		ui.TextArea(c.Key("mcp-json"), &w.paste).Lines(5, 8).Label(tx(l, xMCPJSON))
		ui.Checkbox(c.Key("mcp-json-replace"), &w.replace, tx(l, xMCPReplace))
		if ui.Button(c.Key("mcp-json-import"), tx(l, xMCPParse)).Disabled(actions.Parse == nil || strings.TrimSpace(w.paste) == "").Clicked() {
			w.importDrafts(actions)
		}
	}
	Panel(c).Gap(7).Children(func() {
		for i, e := range w.entries {
			ui.Row(c.Key(fmt.Sprint("mcp-draft-", i))).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.Checkbox(c.Key(fmt.Sprint("mcp-draft-check-", i)), &e.selected, "").Label(tx(l, xMCPSelectDraft)).Changed() {
					w.invalidate(nil)
				}
				name := e.draft.Name
				if name == "" {
					name = tx(l, xMCPUnnamed)
				}
				if ui.Button(c.Key(fmt.Sprint("mcp-draft-edit-", i)), name).Grow(1).Basis(0).Clicked() {
					w.edit = i
				}
			})
		}
		if ui.Button(c.Key("mcp-draft-new"), tx(l, xMCPNewDraft)).Clicked() {
			w.invalidate(nil)
			w.entries = append(w.entries, newMCPDraft(mcp.Draft{Transport: "http"}))
			w.edit = len(w.entries) - 1
		}
	})
	if w.edit < len(w.entries) {
		u.mcpDraftForm(c, w.entries[w.edit])
	}
	if ui.Button(c.Key("mcp-fill-next"), tx(l, xMCPNext)).Disabled(len(w.drafts()) == 0).Clicked() {
		w.err = nil
		for _, d := range w.drafts() {
			if err := mcp.ValidateDraft(d); err != nil {
				w.err = err
				break
			}
		}
		if w.err == nil {
			w.step = 1
		}
	}
}
func (u *settingsUI) mcpDraftForm(c *ui.Context, e *mcpDraftEntry) {
	w, l := u.mcp.add, u.L()
	Panel(c).Gap(9).Children(func() {
		field := func(key, label string, value *string) {
			ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted)
			if ui.TextInput(c.Key(key), value).Label(label).Changed() {
				w.invalidate(e)
			}
		}
		field("mcp-name", tx(l, xMCPName), &e.draft.Name)
		transport := e.draft.Transport
		if ui.Select(c.Key("mcp-transport"), &transport, []string{"http", "sse", "stdio"}).Label(tx(l, xMCPTransport)).Changed() {
			if (transport == "stdio") != (e.draft.Transport == "stdio") {
				e.pendingTransport = transport
				e.confirmTransport = true
			} else {
				e.draft.Transport = transport
				w.invalidate(e)
			}
		}
		if ui.AlertDialog(c.Key("mcp-transport-confirm"), &e.confirmTransport, tx(l, xMCPTransport), tx(l, xMCPTransportChange), l.Text(l10n.KeyCancel), l.Text(l10n.KeyOk)) == 1 {
			e.draft.Transport = e.pendingTransport
			if e.draft.Transport == "stdio" {
				e.draft.URL = ""
				e.draft.Headers = nil
				e.draft.EnvHeaders = nil
				e.draft.BearerTokenEnv = ""
			} else {
				e.draft.Command = ""
				e.draft.Args = nil
				e.draft.Env = nil
				e.draft.Cwd = ""
			}
			e.rows = nil
			w.invalidate(e)
		}
		ui.Checkbox(c.Key("mcp-show-secrets"), &w.showSecrets, tx(l, xMCPShowSecrets))
		if e.draft.Transport == "stdio" {
			field("mcp-command", tx(l, xMCPCommand), &e.draft.Command)
			ui.Text(c, tx(l, xMCPArgs)).FontSize(11)
			removeArg := -1
			for i := range e.draft.Args {
				ui.Row(c).Gap(6).Children(func() {
					if ui.TextInput(c.Key(fmt.Sprint("mcp-arg-", i)), &e.draft.Args[i]).Grow(1).Basis(0).Label(tx(l, xMCPArgs)).Changed() {
						w.invalidate(e)
					}
					if ui.Button(c.Key(fmt.Sprint("mcp-arg-remove-", i)), "−").Clicked() {
						removeArg = i
					}
				})
			}
			if removeArg >= 0 {
				e.draft.Args = slices.Delete(e.draft.Args, removeArg, removeArg+1)
				w.invalidate(e)
			}
			if ui.Button(c.Key("mcp-arg-add"), tx(l, xMCPAddArgument)).Clicked() {
				e.draft.Args = append(e.draft.Args, "")
				w.invalidate(e)
			}
			u.mcpDraftMap(c, e, "env", xMCPEnv, &e.draft.Env)
			field("mcp-cwd", tx(l, xMCPCwd), &e.draft.Cwd)
		} else {
			field("mcp-url", tx(l, xMCPURL), &e.draft.URL)
			u.mcpDraftMap(c, e, "headers", xMCPHeaders, &e.draft.Headers)
			u.mcpDraftMap(c, e, "env-headers", xMCPEnvHeaders, &e.draft.EnvHeaders)
			field("mcp-bearer-env", tx(l, xMCPBearerEnv), &e.draft.BearerTokenEnv)
		}
	})
}
func (u *settingsUI) mcpDraftMap(c *ui.Context, e *mcpDraftEntry, key string, title textKey, values *map[string]string) {
	w, l := u.mcp.add, u.L()
	ui.Text(c, tx(l, title)).FontSize(11)
	if e.rows == nil {
		e.rows = map[string][]*mcpPairRow{}
	}
	if _, ok := e.rows[key]; !ok {
		keys := make([]string, 0, len(*values))
		for k := range *values {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			e.nextRow++
			e.rows[key] = append(e.rows[key], &mcpPairRow{id: e.nextRow, key: k})
		}
	}
	remove := -1
	for i, row := range e.rows[key] {
		old := row.key
		value := (*values)[old]
		ui.Row(c.Key(fmt.Sprint(key, row.id))).Gap(6).Children(func() {
			nk := ui.TextInput(c.Key(fmt.Sprint(key, "-key-", row.id)), &row.key).Label(tx(l, xMCPKey)).Placeholder(tx(l, xMCPKey)).Grow(1).Basis(0)
			nv := ui.TextInput(c.Key(fmt.Sprint(key, "-value-", row.id)), &value).Label(tx(l, xMCPValue)).Placeholder(tx(l, xMCPValue)).Grow(1).Basis(0)
			if !w.showSecrets {
				nv.Password()
			}
			if nk.Changed() || nv.Changed() {
				if row.key != old {
					if _, exists := (*values)[row.key]; exists {
						row.key = old
						w.err = &mcp.DraftError{Code: "duplicate", Field: key}
						return
					}
					delete(*values, old)
				}
				(*values)[row.key] = value
				w.invalidate(e)
			}
			if ui.Button(c.Key(fmt.Sprint(key, "-remove-", row.id)), "−").Clicked() {
				remove = i
			}
		})
	}
	if remove >= 0 {
		delete(*values, e.rows[key][remove].key)
		e.rows[key] = slices.Delete(e.rows[key], remove, remove+1)
		w.invalidate(e)
	}
	_, empty := (*values)[""]
	if ui.Button(c.Key(key+"-add"), tx(l, xMCPAddPair)).Disabled(empty).Clicked() {
		if *values == nil {
			*values = map[string]string{}
		}
		(*values)[""] = ""
		e.nextRow++
		e.rows[key] = append(e.rows[key], &mcpPairRow{id: e.nextRow})
		w.invalidate(e)
	}
}

// Program paths and arguments can contain secrets; preview only a conservative
// executable basename, never the supplied command line or environment.
func mcpDraftProgram(command string) string {
	command = strings.ReplaceAll(command, "\\", "/")
	parts := strings.Split(command, "/")
	name := parts[len(parts)-1]
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._-", r)) {
			return "[program]"
		}
	}
	if len(name) > 80 {
		return "[program]"
	}
	return name
}
