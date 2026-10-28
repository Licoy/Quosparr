package app

import (
	"context"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
)

// mcpAddReview is the last step: a card per provider whose box adds it as
// a target and checks its configuration at once, showing the file it
// writes and, per server, whether it is added or already there.
func (u *settingsUI) mcpAddReview(c *ui.Context, actions MCPActions) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	retrying := slices.ContainsFunc(w.plans, func(p mcpAddTarget) bool { return p.done })
	ui.Column(c).Gap(2).Padding(0, 12).Children(func() {
		ui.Text(c, tx(l, xMCPTargets)).FontSize(13).FontWeight(600)
		ui.Text(c, tx(l, xMCPTargetsHint)).FontSize(11).TextColor(t.TextMuted)
	})
	for _, p := range store.Providers() {
		u.mcpTargetCard(c, p, actions, retrying)
	}
	if w.err != nil && !slices.ContainsFunc(w.plans, func(p mcpAddTarget) bool { return p.err != nil }) {
		listNotice(c, "", u.mcpAddError(w.err), true, "", nil)
	}
	ui.Column(c).Gap(4).Children(func() {
		if w.needsVerification() {
			mcpInfoLine(c, icWarnCircle, tx(l, xMCPUnverifiedStatus), t.Warning)
		}
		mcpInfoLine(c, icInfo, tx(l, xMCPReloadHint), t.TextMuted)
	})
}

func (u *settingsUI) mcpTargetCard(c *ui.Context, p store.Provider, actions MCPActions, retrying bool) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	name := l.Text(providerKey(p))
	checked := w.targets[p]
	plan := w.plan(p)
	Panel(c.Key("mcp-target-"+string(p))).Padding(10, 14).Gap(10).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).MinHeight(26).Children(func() {
			box := ui.Checkbox(c.Key("mcp-target-check-"+string(p)), &checked, "").Label(name).Size(18, 18).Disabled(w.writing || retrying)
			if !w.writing && !retrying {
				box.Cursor(ui.CursorPointer)
			}
			if box.Changed() {
				w.targets[p] = checked
				acknowledged := w.unverified
				w.invalidate(nil)
				w.unverified = acknowledged
				u.prepareMCPDrafts(actions, false)
			}
			ProviderIcon(c, p, 18)
			ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine().Shrink(0)
			if plan != nil && plan.plan.Path() != "" {
				ui.Text(c, plan.plan.Path()).FontSize(11).Font("monospace").TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1)
			}
			ui.Spacer(c).Grow(1)
			switch {
			case plan != nil && plan.done:
				settingsBadge(c, icCheckCircle, tx(l, xMCPTargetDone), t.Success)
			case !checked:
			case w.task.Running:
				ui.Spinner(c).Size(12, 12)
			case plan != nil && plan.err != nil:
				settingsBadge(c, icWarnCircle, tx(l, xMCPTargetBlocked), t.Danger)
			case plan != nil:
				added, existing := mcpPlanCounts(plan.plan)
				ui.Text(c, tx(l, xMCPPlanCounts, added, existing)).FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
		})
		if !checked || plan == nil || plan.done || w.task.Running {
			return
		}
		if plan.err != nil {
			listNotice(c, "", u.mcpAddError(plan.err), true, "", nil)
			return
		}
		for _, item := range plan.plan.Items {
			ui.Column(c.Key(fmt.Sprint("mcp-plan-", p, "-", item.Name))).Gap(6).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, item.Name).FontSize(12).FontWeight(600).Font("monospace").SingleLine().Ellipsis("…").Shrink(1)
					if item.Status == "exists" {
						settingsBadge(c, nil, tx(l, xMCPAlreadyExists), t.TextMuted)
					} else {
						settingsBadge(c, icPlus, tx(l, xMCPWillAdd), t.Success)
					}
				})
				ui.Column(c).Padding(8, 10).Radius(7).Background(pal(t).Inset).Border(1, pal(t).Line).Children(func() {
					ui.Text(c, item.Details).FontSize(10.5).Font("monospace").TextColor(t.TextMuted).Selectable()
				})
			})
		}
	})
}

func mcpPlanCounts(plan mcp.AddPlan) (added, existing int) {
	for _, item := range plan.Items {
		if item.Status == "exists" {
			existing++
		} else {
			added++
		}
	}
	return added, existing
}

// mcpAddResults reports each provider's write: what it added, or why it
// failed and whether its file needs a look.
func (u *settingsUI) mcpAddResults(c *ui.Context) {
	w, l, t := u.mcp.add, u.L(), c.Theme()
	if w.writing {
		ui.Column(c).Gap(10).Padding(40, 0).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(18, 18)
			ui.Text(c, tx(l, xMCPWriting)).FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	failed := mcpAddFailed(w.plans)
	icon, ink, title := icCheckCircle, t.Success, tx(l, xMCPResultsDone)
	if failed {
		icon, ink, title = icWarnCircle, t.Danger, tx(l, xMCPResultsFailed)
	}
	ui.Row(c).Gap(8).Padding(0, 12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(ink)
		ui.Text(c, title).FontSize(14).FontWeight(600)
	})
	if w.wasUnverified {
		mcpInfoLine(c, icWarnCircle, tx(l, xMCPUnverifiedStatus), t.Warning)
	}
	Panel(c).Children(func() {
		for i, p := range w.plans {
			settingsRule(c, i)
			ui.Row(c).Gap(10).Padding(9, 0).AlignItems(ui.Start).Children(func() {
				ProviderIcon(c, p.provider, 18)
				ui.Column(c).Gap(2).Grow(1).Basis(0).Children(func() {
					ui.Text(c, l.Text(providerKey(p.provider))).FontSize(13).FontWeight(600)
					message, ink := tx(l, xMCPAdded, len(p.result.Added), len(p.result.Existing)), t.TextMuted
					if p.err != nil {
						message, ink = u.mcpAddError(p.err), t.Danger
						if mcpWriteNeedsReview(p.err) {
							message, ink = tx(l, xMCPNeedsReview)+" · "+message, t.Warning
						}
					}
					ui.Text(c, message).FontSize(11).TextColor(ink)
				})
				if p.err == nil {
					ui.Icon(c, icCheckCircle).Size(14, 14).TextColor(t.Success)
				} else {
					ui.Icon(c, icWarnCircle).Size(14, 14).TextColor(t.Danger)
				}
			})
		}
	})
	mcpInfoLine(c, icInfo, tx(l, xMCPReloadHint), t.TextMuted)
}

func mcpAddFailed(plans []mcpAddTarget) bool {
	return slices.ContainsFunc(plans, func(p mcpAddTarget) bool { return p.err != nil })
}

func mcpPendingTargets(plans []mcpAddTarget) int {
	count := 0
	for _, p := range plans {
		if !p.done {
			count++
		}
	}
	return count
}

func (u *settingsUI) submitMCPDrafts(actions MCPActions) {
	w := u.mcp.add
	if w.writing || w.task.Running || w.err != nil || len(w.plans) == 0 || actions.Add == nil || (w.needsVerification() && !w.unverified) {
		return
	}
	w.wasUnverified = w.needsVerification()
	w.writing = true
	w.step = mcpStepResults
	plans := slices.Clone(w.plans)
	go func() {
		for i := range plans {
			if !plans[i].done {
				plans[i].result, plans[i].err = actions.Add(context.Background(), plans[i].provider, plans[i].plan)
				plans[i].done = plans[i].err == nil
			}
		}
		u.post(func() {
			w.plans = plans
			w.writing = false
			w.completed = true
			// Plans may contain complete credentials in their private fields; dispose
			// them after each attempt, and prepare failures anew before retrying.
			for i := range w.plans {
				w.plans[i].plan = mcp.AddPlan{}
			}
			owner := u.currentMCPUI()
			if actions.Query != nil {
				owner.startMCPScan(actions)
			}
			if owner.win != nil && !owner.win.IsDestroyed() {
				owner.win.Update(func() {})
			}
		})
	}()
}
