package app

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
)

// mcpJSONExample shows the accepted shape in the empty JSON box.
const mcpJSONExample = `{
  "mcpServers": {
    "example": {
      "command": "npx",
      "args": ["-y", "example-mcp-server"]
    }
  }
}`

// mcpAddPage is the add page: the back link, title and steps stay put
// over the step's scrolling body, and its actions stay at the bottom.
func (u *settingsUI) mcpAddPage(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	ui.Column(c).Padding(0, 24).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if detailBackButton(c, l.Text(l10n.KeyBack)).Clicked() {
				u.closeMCPAdd()
			}
			ui.Text(c, tx(l, xMCPAdd)).FontSize(15).FontWeight(650).SingleLine()
		})
		// Leaving may have just dropped the wizard the steps read.
		if w.open {
			u.mcpAddSteps(c, actions)
		}
	})
	if !w.open {
		return
	}
	ui.Scroll(c.Key(fmt.Sprint("mcp-add-step-", w.step))).Grow(1).Basis(0).Padding(0, 24, 20, 24).Gap(16).Children(func() {
		switch w.step {
		case mcpStepConfigure:
			u.mcpAddConfigure(c, actions)
		case mcpStepTools:
			u.mcpAddTools(c, actions)
		case mcpStepReview:
			u.mcpAddReview(c, actions)
		default:
			u.mcpAddResults(c)
		}
	})
	ui.Column(c).Children(func() {
		ui.Box(c).Height(1).Background(pal(c.Theme()).Line)
		ui.Row(c).Padding(10, 24, 12, 24).Gap(8).AlignItems(ui.Center).MinHeight(52).Children(func() {
			u.mcpAddFooter(c, actions)
		})
	})
}

// mcpAddSteps numbers the three steps: done ones carry a tick and lead
// back to themselves, the current one is filled.
func (u *settingsUI) mcpAddSteps(c *ui.Context, actions MCPActions) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	titles := []textKey{xMCPStepConfigure, xMCPStepTools, xMCPStepReview}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
		for i, title := range titles {
			if i > 0 {
				ui.Box(c).Width(20).Height(1).Background(t.Border)
			}
			done, current := i < w.step, i == w.step
			// Only the steps one can go back to are buttons; a disabled button
			// would fade the current step.
			if !done || w.step == mcpStepResults || w.writing {
				ui.Row(c).Gap(6).Padding(3, 6).AlignItems(ui.Center).Children(func() { mcpAddStep(c, i, tx(l, title), done, current) })
				continue
			}
			b := ui.ButtonBase(c.Key(fmt.Sprint("mcp-step-", i))).Label(tx(l, title)).Gap(6).Padding(3, 6).Radius(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(pal(t).Hover)
			}
			b.Children(func() { mcpAddStep(c, i, tx(l, title), done, current) })
			if b.Clicked() {
				u.mcpAddGoTo(i, actions)
			}
		}
	})
}

// mcpAddStep is one step's number, or a tick once done, and its title.
func mcpAddStep(c *ui.Context, i int, title string, done, current bool) {
	t := c.Theme()
	ink, fill := t.TextMuted, ui.Transparent
	if done || current {
		ink, fill = t.AccentText, t.Accent
	}
	dot := ui.Row(c).Size(18, 18).Radius(9).Background(fill).Justify(ui.Center).AlignItems(ui.Center).Shrink(0)
	if !done && !current {
		dot.Border(1, t.Border.Mix(t.Text, 0.2))
	}
	dot.Children(func() {
		if done {
			ui.Icon(c, icTick).Size(11, 11).TextColor(ink)
		} else {
			ui.Text(c, fmt.Sprint(i+1)).FontSize(10.5).FontWeight(650).TextColor(ink)
		}
	})
	label := ui.Text(c, title).FontSize(12).SingleLine()
	if current {
		label.FontWeight(600).TextColor(t.Text)
	} else {
		label.TextColor(t.TextMuted)
	}
}

// mcpAddGoTo returns to an earlier step, leaving a running check behind.
func (u *settingsUI) mcpAddGoTo(step int, actions MCPActions) {
	w := u.mcp.add
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.generation++
	w.task = Task{}
	w.step = step
}

// mcpAddFooter holds a step's actions, the leading one filled.
func (u *settingsUI) mcpAddFooter(c *ui.Context, actions MCPActions) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	back := func(step int) {
		if PillButton(c, nil, tx(l, xMCPPrevious), w.writing).Clicked() {
			u.mcpAddGoTo(step, actions)
		}
	}
	switch w.step {
	case mcpStepConfigure:
		ui.Icon(c, icShield).Size(12, 12).TextColor(t.TextMuted)
		ui.Text(c, tx(l, xMCPMemoryOnly)).FontSize(11).TextColor(t.TextMuted).Grow(1).Basis(0)
		if AccentPillButton(c, nil, tx(l, xMCPNext), len(w.drafts()) == 0).Clicked() {
			w.continueToTools()
		}
	case mcpStepTools:
		ui.Spacer(c).Grow(1)
		back(mcpStepConfigure)
		if AccentPillButton(c, nil, tx(l, xMCPNext), w.needsVerification() && !w.unverified).Clicked() {
			w.step = mcpStepReview
			u.prepareMCPDrafts(actions, false)
		}
	case mcpStepReview:
		if w.task.Running {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, tx(l, xMCPChecking)).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Spacer(c).Grow(1)
		back(mcpStepTools)
		pending := mcpPendingTargets(w.plans)
		blocked := w.task.Running || w.writing || w.err != nil || pending == 0
		if AccentPillButton(c.Key("mcp-add-submit"), nil, tx(l, xMCPSubmit, pending), blocked).Clicked() {
			u.submitMCPDrafts(actions)
		}
	default:
		ui.Spacer(c).Grow(1)
		if w.writing {
			return
		}
		if mcpAddFailed(w.plans) && len(w.entries) > 0 {
			if PillButton(c.Key("mcp-add-retry"), nil, tx(l, xMCPRetry), w.task.Running).Clicked() {
				w.step = mcpStepReview
				u.prepareMCPDrafts(actions, true)
			}
		}
		if AccentPillButton(c.Key("mcp-add-done"), nil, tx(l, xMCPDone), false).Clicked() {
			w.completed = false
			u.closeMCPAdd()
			if actions.Query != nil {
				u.startMCPScan(actions)
			}
		}
	}
}

// mcpAddConfigure is the first step: the servers, either typed into the
// form or parsed from pasted JSON into it.
func (u *settingsUI) mcpAddConfigure(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	mode := 0
	if w.jsonMode {
		mode = 1
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if PillTabs(c.Key("mcp-input-mode"), &mode, false, tx(l, xMCPManual), tx(l, xMCPJSON)) {
			w.jsonMode, w.jsonErr, w.imported = mode == 1, nil, 0
		}
		ui.Spacer(c).Grow(1)
		if !w.jsonMode && SmallPillButton(c, icPlus, tx(l, xMCPNewDraft), false).Clicked() {
			w.invalidate(nil)
			w.addEntry(mcp.Draft{Transport: "stdio"})
		}
	})
	if w.jsonMode {
		u.mcpAddJSON(c, actions)
		return
	}
	if w.imported > 0 {
		listNotice(c, "mcp-imported-close", tx(l, xMCPImported, w.imported), false, u.text(l10n.KeyOk), func() { w.imported = 0 })
	}
	if len(w.entries) > 1 {
		u.mcpDraftList(c)
	}
	if w.edit < len(w.entries) {
		u.mcpDraftForm(c, w.entries[w.edit])
	}
}

// mcpAddJSON is the JSON box, parsed into the form on request.
func (u *settingsUI) mcpAddJSON(c *ui.Context, actions MCPActions) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	message := ""
	if w.jsonErr != nil {
		message = u.mcpAddError(w.jsonErr)
	}
	Panel(c).Padding(12, 12).Gap(10).Children(func() {
		ui.TextArea(c.Key("mcp-json"), &w.paste).Lines(10, 16).Font("monospace").FontSize(12).
			Placeholder(mcpJSONExample).Label(tx(l, xMCPJSON)).Error(message)
		if message != "" {
			ui.Text(c, message).FontSize(11).TextColor(t.Danger)
		}
		ui.Text(c, tx(l, xMCPJSONHint)).FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if !w.allBlank() {
				ui.Checkbox(c.Key("mcp-json-replace"), &w.replace, tx(l, xMCPReplace, len(w.entries))).FontSize(12)
			}
			ui.Spacer(c).Grow(1)
			if AccentPillButton(c, nil, tx(l, xMCPParse), actions.Parse == nil || strings.TrimSpace(w.paste) == "").Clicked() {
				w.importDrafts(actions)
			}
		})
	})
}

// mcpDraftList lists the servers when there are several: each one's box
// includes it, a click shows it in the form below, the bin removes it.
func (u *settingsUI) mcpDraftList(c *ui.Context) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	remove := -1
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Padding(0, 12).Children(func() {
			ui.Text(c, tx(l, xMCPServers, len(w.drafts()), len(w.entries))).FontSize(13).FontWeight(600)
		})
		listPanel(c).Children(func() {
			for i, e := range w.entries {
				name := strings.TrimSpace(e.name)
				shown := name
				if shown == "" {
					shown = tx(l, xMCPUnnamed)
				}
				row := ui.Row(c.Key(fmt.Sprint("mcp-draft-", e.id))).Gap(10).Padding(6, listRowPadX).Radius(8).AlignItems(ui.Center)
				switch {
				case i == w.edit:
					row.Background(pal(t).Inset).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.10))
				case row.Hovered():
					row.Background(pal(t).Hover)
				}
				row.Children(func() {
					if ui.Checkbox(c.Key(fmt.Sprint("mcp-draft-check-", e.id)), &e.selected, "").Label(tx(l, xMCPSelectDraft, shown)).
						Size(18, 18).Cursor(ui.CursorPointer).Changed() {
						w.invalidate(nil)
					}
					b := ui.ButtonBase(c.Key(fmt.Sprint("mcp-draft-edit-", e.id))).Label(shown).Grow(1).Basis(0).Gap(8).
						AlignItems(ui.Center).Cursor(ui.CursorPointer)
					b.Children(func() {
						ui.Icon(c, mcpTransportIcon(e.transport)).Size(14, 14).TextColor(t.TextMuted)
						ui.Column(c).Gap(1).Grow(1).Basis(0).Children(func() {
							title := ui.Text(c, shown).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…")
							if name == "" {
								title.TextColor(t.TextMuted)
							}
							ui.Text(c, mcpDraftSummary(l, e)).FontSize(11).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
						})
					})
					if b.Clicked() {
						w.edit = i
					}
					if w.attempted && e.selected && w.entryError(e) != nil {
						settingsBadge(c, icWarnCircle, tx(l, xMCPNeedsFix), t.Danger)
					}
					if IconButton(c, icTrash, tx(l, xMCPRemoveDraft, shown), false).Clicked() {
						remove = i
					}
				})
			}
		})
	})
	if remove >= 0 {
		w.removeEntry(remove)
	}
}

// mcpDraftForm is the form of one server: its name and transport with the
// connection, then the environment or headers, then the rarely needed
// fields folded away. Errors show once Continue was pressed.
func (u *settingsUI) mcpDraftForm(c *ui.Context, e *mcpDraftEntry) {
	w, l := u.mcp.add, u.L()
	errs := map[string]string{}
	var general string
	if w.attempted && e.selected {
		var d *mcp.DraftError
		if err := w.entryError(e); errors.As(err, &d) && mcpFormFields[d.Field] {
			errs[d.Field] = tx(l, mcpErrorTextKey(d.Code))
		} else if err != nil {
			general = u.mcpAddError(err)
		}
	}
	key := func(name string) *ui.Context { return c.Key(fmt.Sprint(name, "-", e.id)) }
	if general != "" {
		listNotice(c, "", general, true, "", nil)
	}
	Panel(c).Padding(12, 14).Children(func() {
		ui.Form(c, func() {
			ui.Field(c, tx(l, xMCPName), func() {
				if ui.TextInput(key("mcp-name"), &e.name).Placeholder("my-server").Changed() {
					w.invalidate(e)
				}
			}).Description(tx(l, xMCPNameHint)).Error(errs["name"])
			ui.Field(c, tx(l, xMCPTransport), func() {
				i := max(0, slices.Index(mcpTransports, e.transport))
				if PillTabs(key("mcp-transport"), &i, false, mcpTransportName(l, "stdio"), mcpTransportName(l, "http"), mcpTransportName(l, "sse")) {
					e.transport = mcpTransports[i]
					w.invalidate(e)
				}
			}).Description(mcpTransportHint(l, e.transport))
			if e.transport == "stdio" {
				ui.Field(c, tx(l, xMCPCommand), func() {
					if ui.TextInput(key("mcp-command"), &e.command).Placeholder("npx").Font("monospace").Changed() {
						w.invalidate(e)
					}
				}).Error(errs["command"])
				ui.Field(c, tx(l, xMCPArgs), func() { u.mcpArgs(c, e) }).Description(tx(l, xMCPArgsHint)).Error(errs["args"])
				return
			}
			ui.Field(c, tx(l, xMCPURL), func() {
				if ui.TextInput(key("mcp-url"), &e.url).Placeholder("https://example.com/mcp").Changed() {
					w.invalidate(e)
				}
			}).Error(errs["url"])
		})
	})
	if e.transport == "stdio" {
		u.mcpPairs(c, e, "env", &e.env, mcpPairsText{title: tx(l, xMCPEnv), hint: tx(l, xMCPEnvHint), key: tx(l, xMCPVariable), value: tx(l, xMCPValue), add: tx(l, xMCPAddVariable)}, true, errs["env"])
	} else {
		u.mcpPairs(c, e, "headers", &e.headers, mcpPairsText{title: tx(l, xMCPHeaders), hint: tx(l, xMCPHeadersHint), key: tx(l, xMCPHeaderName), value: tx(l, xMCPValue), add: tx(l, xMCPAddHeader)}, true, errs["headers"])
	}
	ui.Collapsible(key("mcp-advanced"), tx(l, xMCPAdvanced), &e.advanced, func() {
		ui.Column(c).Gap(14).Padding(8, 0, 0, 0).Children(func() {
			if e.transport == "stdio" {
				Panel(c).Padding(12, 14).Children(func() {
					ui.Form(c, func() {
						ui.Field(c, tx(l, xMCPCwd), func() {
							if ui.TextInput(key("mcp-cwd"), &e.cwd).Placeholder(tx(l, xMCPOptional)).Font("monospace").Changed() {
								w.invalidate(e)
							}
						}).Description(tx(l, xMCPCwdHint)).Error(errs["cwd"])
					})
				})
				return
			}
			Panel(c).Padding(12, 14).Children(func() {
				ui.Form(c, func() {
					ui.Field(c, tx(l, xMCPBearerEnv), func() {
						if ui.TextInput(key("mcp-bearer-env"), &e.bearerEnv).Placeholder("MCP_TOKEN").Font("monospace").Changed() {
							w.invalidate(e)
						}
					}).Description(tx(l, xMCPBearerHint)).Error(errs["bearer_token_env"])
				})
			})
			u.mcpPairs(c, e, "env-headers", &e.envHeaders, mcpPairsText{title: tx(l, xMCPEnvHeaders), hint: tx(l, xMCPEnvHeadersHint), key: tx(l, xMCPHeaderName), value: tx(l, xMCPVariable), add: tx(l, xMCPAddHeader)}, false, errs["env_headers"])
		})
	})
}

// mcpFormFields are the fields the form marks itself; other errors show
// over it.
var mcpFormFields = map[string]bool{"name": true, "command": true, "args": true, "url": true, "cwd": true,
	"env": true, "headers": true, "env_headers": true, "bearer_token_env": true}

// mcpArgs are the command's arguments, one input each, in order.
func (u *settingsUI) mcpArgs(c *ui.Context, e *mcpDraftEntry) {
	w, l := u.mcp.add, u.L()
	remove := -1
	ui.Column(c).Gap(6).Children(func() {
		for i := range e.args {
			arg := &e.args[i]
			ui.Row(c.Key(fmt.Sprint("mcp-arg-", e.id, "-", arg.id))).Gap(6).AlignItems(ui.Center).Children(func() {
				if ui.TextInput(c.Key(fmt.Sprint("mcp-arg-value-", e.id, "-", arg.id)), &arg.value).Label(tx(l, xMCPArgument, i+1)).
					Font("monospace").Grow(1).Basis(0).Changed() {
					w.invalidate(e)
				}
				if IconButton(c, icClose, tx(l, xMCPRemove), false).Clicked() {
					remove = i
				}
			})
		}
		ui.Row(c).Children(func() {
			if SmallPillButton(c, icPlus, tx(l, xMCPAddArgument), false).Clicked() {
				e.args = append(e.args, mcpArg{id: e.newID()})
				w.invalidate(e)
			}
		})
	})
	if remove >= 0 {
		e.args = slices.Delete(e.args, remove, remove+1)
		w.invalidate(e)
	}
}

type mcpPairsText struct{ title, hint, key, value, add string }

// mcpPairs is a table of names and values under its title: environment
// variables or headers, whose values hide unless shown.
func (u *settingsUI) mcpPairs(c *ui.Context, e *mcpDraftEntry, name string, rows *[]mcpPair, text mcpPairsText, secret bool, message string) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	remove := -1
	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(10).Padding(0, 12).AlignItems(ui.End).Children(func() {
			ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
				ui.Text(c, text.title).FontSize(13).FontWeight(600)
				ui.Text(c, text.hint).FontSize(11).TextColor(t.TextMuted)
			})
			if secret && len(*rows) > 0 {
				icon, label := icEye, tx(l, xMCPShowSecrets)
				if w.showSecrets {
					icon, label = icEyeOff, tx(l, xMCPHideSecrets)
				}
				if SmallPillButton(c, icon, label, false).Clicked() {
					w.showSecrets = !w.showSecrets
				}
			}
		})
		panel := Panel(c).Padding(10, 12).Gap(6)
		if message != "" {
			panel.BorderColor(t.Danger)
		}
		panel.Children(func() {
			for i := range *rows {
				row := &(*rows)[i]
				ui.Row(c.Key(fmt.Sprint(name, "-", e.id, "-", row.id))).Gap(6).AlignItems(ui.Center).Children(func() {
					k := ui.TextInput(c.Key(fmt.Sprint(name, "-key-", e.id, "-", row.id)), &row.key).Label(text.key).Placeholder(text.key).
						Font("monospace").Grow(2).Basis(0)
					v := ui.TextInput(c.Key(fmt.Sprint(name, "-value-", e.id, "-", row.id)), &row.value).Label(text.value).Placeholder(text.value).
						Grow(3).Basis(0)
					if secret && !w.showSecrets {
						v.Password()
					} else if !secret {
						v.Font("monospace")
					}
					if k.Changed() || v.Changed() {
						w.invalidate(e)
					}
					if IconButton(c, icClose, tx(l, xMCPRemove), false).Clicked() {
						remove = i
					}
				})
			}
			ui.Row(c).Children(func() {
				if SmallPillButton(c, icPlus, text.add, false).Clicked() {
					*rows = append(*rows, mcpPair{id: e.newID()})
					w.invalidate(e)
				}
			})
		})
		if message != "" {
			ui.Text(c, message).FontSize(11).TextColor(t.Danger).Padding(0, 12)
		}
	})
	if remove >= 0 {
		*rows = slices.Delete(*rows, remove, remove+1)
		w.invalidate(e)
	}
}

func mcpTransportName(l l10n.Localizer, transport string) string {
	switch transport {
	case "stdio":
		return tx(l, xMCPTransportStdio)
	case "sse":
		return "SSE"
	}
	return "HTTP"
}

func mcpTransportHint(l l10n.Localizer, transport string) string {
	switch transport {
	case "stdio":
		return tx(l, xMCPStdioHint)
	case "sse":
		return tx(l, xMCPSSEHint)
	}
	return tx(l, xMCPHTTPHint)
}

func mcpTransportIcon(transport string) *ui.SVG {
	if transport == "stdio" {
		return icTerminal
	}
	return icGlobe
}

// mcpDraftSummary names a server's transport and where it connects: the
// program's name or the URL's host, never arguments, a path, a query or
// header and environment values.
func mcpDraftSummary(l l10n.Localizer, e *mcpDraftEntry) string {
	target := ""
	if e.transport == "stdio" {
		if command := strings.TrimSpace(e.command); command != "" {
			target = mcpDraftProgram(command)
		}
	} else if u, err := url.Parse(strings.TrimSpace(e.url)); err == nil {
		target = u.Hostname()
	}
	return strings.Join(nonEmpty([]string{mcpTransportName(l, e.transport), target}), " · ")
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
