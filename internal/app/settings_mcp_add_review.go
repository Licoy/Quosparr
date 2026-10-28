package app

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/mcp"
	"github.com/licoy/quosparr/internal/store"
	"slices"
)

func (u *settingsUI) mcpAddTargets(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	for _, p := range store.Providers() {
		checked := w.targets[p]
		if ui.Checkbox(c.Key("mcp-target-"+string(p)), &checked, l.Text(providerKey(p))).Disabled(w.task.Running).Changed() {
			w.targets[p] = checked
			acknowledged := w.unverified
			w.invalidate(nil)
			w.unverified = acknowledged
		}
	}
	ui.Text(c, tx(l, xMCPOAuthHint)).FontSize(11).TextColor(c.Theme().TextMuted)
	for _, p := range w.plans {
		if p.err != nil {
			ui.Text(c, l.Text(providerKey(p.provider))+": "+u.mcpAddError(p.err)).FontSize(12).TextColor(c.Theme().Danger)
		}
	}
	any := false
	for _, v := range w.targets {
		any = any || v
	}
	u.mcpAddNavigation(c, func() {
		if w.cancel != nil {
			w.cancel()
			w.cancel = nil
		}
		w.generation++
		w.task = Task{}
		w.step = 1
	}, func() { u.prepareMCPDrafts(actions, false) }, !any || w.task.Running)
}
func (u *settingsUI) mcpAddReview(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	if w.needsVerification() {
		ui.Text(c, tx(l, xMCPUnverifiedStatus)).FontSize(12).TextColor(c.Theme().Warning)
	}
	for _, p := range w.plans {
		Panel(c).Gap(6).Children(func() {
			ui.Text(c, l.Text(providerKey(p.provider))).FontSize(14).FontWeight(600)
			for _, item := range p.plan.Items {
				status := tx(l, xMCPWillAdd)
				if item.Status == "exists" {
					status = tx(l, xMCPAlreadyExists)
				}
				ui.Text(c, item.Name+" · "+status).FontSize(12)
				ui.Text(c, item.Details).FontSize(11).Selectable()
			}
		})
	}
	ui.Text(c, tx(l, xMCPReloadHint)).FontSize(11).TextColor(c.Theme().TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, u.text(l10n.KeyBack)).Clicked() {
			w.step = 2
		}
		if ui.Button(c.Key("mcp-add-submit"), tx(l, xMCPSubmit, mcpPendingTargets(w.plans))).Disabled(w.writing).Clicked() {
			u.submitMCPDrafts(actions)
		}
	})
}
func (u *settingsUI) mcpAddResults(c *ui.Context, actions MCPActions) {
	w, l := u.mcp.add, u.L()
	if w.writing {
		ui.Text(c, tx(l, xMCPWriting)).FontSize(12)
		return
	}
	if w.wasUnverified {
		ui.Text(c, tx(l, xMCPUnverifiedStatus)).FontSize(12).TextColor(c.Theme().Warning)
	}
	failed := false
	for _, p := range w.plans {
		message := tx(l, xMCPAdded, len(p.result.Added), len(p.result.Existing))
		if p.err != nil {
			failed = true
			message = u.mcpAddError(p.err)
			if mcpWriteNeedsReview(p.err) {
				message = tx(l, xMCPNeedsReview) + ": " + message
			}
		}
		ui.Text(c, l.Text(providerKey(p.provider))+": "+message).FontSize(12)
	}
	ui.Text(c, tx(l, xMCPReloadHint)).FontSize(11).TextColor(c.Theme().TextMuted)
	if failed && len(w.entries) > 0 {
		if ui.Button(c.Key("mcp-add-retry"), tx(l, xMCPRetry)).Disabled(w.task.Running).Clicked() {
			u.prepareMCPDrafts(actions, true)
		}
	}
	if ui.Button(c.Key("mcp-add-done"), u.text(l10n.KeyOk)).Clicked() {
		w.completed = false
		u.closeMCPAdd()
		if actions.Query != nil {
			u.startMCPScan(actions)
		}
	}
}
func (u *settingsUI) mcpAddNavigation(c *ui.Context, back, next func(), disabled bool) {
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, u.text(l10n.KeyBack)).Clicked() {
			back()
		}
		if ui.Button(c, tx(u.L(), xMCPNext)).Disabled(disabled).Clicked() {
			next()
		}
	})
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
	w.step = 4
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
