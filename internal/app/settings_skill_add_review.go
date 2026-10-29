package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// skillAddReview is the last step: a card per skills folder whose box adds
// it as a target and checks it at once, showing, per skill, whether it is
// added or left alone.
func (u *settingsUI) skillAddReview(c *ui.Context, acts SkillActions) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	retrying := slices.ContainsFunc(w.plans, func(p skillAddTarget) bool { return p.done })
	ui.Column(c).Gap(2).Padding(0, 12).Children(func() {
		ui.Text(c, tx(l, xSkillAddTargets)).FontSize(13).FontWeight(600)
		ui.Text(c, tx(l, xSkillAddTargetsHint)).FontSize(11).TextColor(t.TextMuted)
	})
	for _, target := range skills.InstallTargets() {
		u.skillTargetCard(c, target, acts, retrying)
	}
	ui.Column(c).Gap(4).Children(func() {
		if w.targets[skills.TargetShared] && (w.targets[skills.TargetCodex] || w.targets[skills.TargetGrok]) {
			mcpInfoLine(c, icWarnCircle, tx(l, xSkillAddSharedOverlap), t.Warning)
		}
		mcpInfoLine(c, icInfo, tx(l, xSkillAddReloadHint), t.TextMuted)
	})
}

func (u *settingsUI) skillTargetCard(c *ui.Context, target skills.InstallTarget, acts SkillActions, retrying bool) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	name := skillTargetName(l, target)
	checked := w.targets[target]
	plan := w.plan(target)
	Panel(c.Key("skill-target-"+string(target))).Padding(10, 14).Gap(10).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).MinHeight(26).Children(func() {
			box := ui.Checkbox(c.Key("skill-target-check-"+string(target)), &checked, "").Label(name).Size(18, 18).Disabled(w.writing || retrying)
			if !w.writing && !retrying {
				box.Cursor(ui.CursorPointer)
			}
			if box.Changed() {
				w.targets[target] = checked
				u.prepareSkillInstall(acts, false)
			}
			skillTargetIcon(c, target)
			ui.Column(c).Gap(1).Shrink(1).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(600).SingleLine()
				if plan != nil && plan.plan.Root != "" {
					ui.Text(c, plan.plan.Root).FontSize(11).Font("monospace").TextColor(t.TextMuted).SingleLine().Ellipsis("…")
				}
			})
			ui.Spacer(c).Grow(1)
			switch {
			case plan != nil && plan.done:
				settingsBadge(c, icCheckCircle, tx(l, xSkillAddTargetDone), t.Success)
			case !checked:
				if target == skills.TargetShared {
					ui.Text(c, tx(l, xSkillAddReaders, skillProviders(l, []skills.Provider{skills.ProviderCodex, skills.ProviderGrok}))).
						FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
			case w.task.Running:
				ui.Spinner(c).Size(12, 12)
			case plan != nil && plan.err != nil:
				settingsBadge(c, icWarnCircle, tx(l, xSkillAddTargetBlocked), t.Danger)
			case plan != nil:
				added, skipped := skillPlanCounts(plan.plan)
				ui.Text(c, tx(l, xSkillAddCounts, added, skipped)).FontSize(11).TextColor(t.TextMuted).SingleLine().Shrink(0)
			}
		})
		if !checked || plan == nil || plan.done || w.task.Running {
			return
		}
		if plan.err != nil {
			listNotice(c, "", skillAddErrorText(l, plan.err), true, "", nil)
			return
		}
		if target == skills.TargetShared && len(plan.plan.Readers) > 0 {
			mcpInfoLine(c, icInfo, tx(l, xSkillAddReaders, skillProviders(l, plan.plan.Readers)), t.TextMuted)
		}
		for _, item := range plan.plan.Items {
			skillPlanItem(c, l, target, item)
		}
	})
}

func skillTargetIcon(c *ui.Context, target skills.InstallTarget) {
	if target == skills.TargetShared {
		ui.Icon(c, icFolder).Size(18, 18).TextColor(c.Theme().TextMuted)
		return
	}
	ProviderIcon(c, store.Provider(target), 18)
}

func skillPlanItem(c *ui.Context, l l10n.Localizer, target skills.InstallTarget, item skills.InstallItem) {
	t := c.Theme()
	ui.Column(c.Key(fmt.Sprint("skill-plan-", target, "-", item.Folder))).Gap(4).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, item.Name).FontSize(12).FontWeight(600).SingleLine().Ellipsis("…").Shrink(1)
			switch item.Status {
			case skills.InstallAdd:
				settingsBadge(c, icPlus, tx(l, xSkillAddWillAdd), t.Success)
			case skills.InstallExists:
				settingsBadge(c, nil, tx(l, xSkillAddExists), t.TextMuted)
			case skills.InstallInstalled:
				settingsBadge(c, nil, tx(l, xSkillAddInstalled), t.TextMuted)
			default:
				settingsBadge(c, icWarnCircle, tx(l, xSkillAddInvalid), t.Danger)
			}
		})
		ui.Text(c, item.Path).FontSize(11).Font("monospace").TextColor(t.TextMuted).SingleLine().Ellipsis("…").Selectable()
		for _, clash := range item.Clashes {
			mcpInfoLine(c, icWarnCircle, tx(l, xSkillAddClash, l.Text(providerKey(store.Provider(clash.Provider))), clash.File), t.Warning)
		}
	})
}

// skillPlanCounts are the skills a target adds and those it leaves alone.
func skillPlanCounts(plan skills.InstallPlan) (added, skipped int) {
	for _, item := range plan.Items {
		if item.Status == skills.InstallAdd {
			added++
		} else {
			skipped++
		}
	}
	return added, skipped
}

// skillAddResults reports each folder's write: the skills it added, and
// why any failed.
func (u *settingsUI) skillAddResults(c *ui.Context) {
	w, l, t := u.skills.add, u.L(), c.Theme()
	if w.writing {
		ui.Column(c).Gap(10).Padding(40, 0).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(18, 18)
			ui.Text(c, tx(l, xSkillAddWriting)).FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	failed := skillAddFailed(w.plans)
	icon, ink, title := icCheckCircle, t.Success, tx(l, xSkillAddResultsDone)
	if failed {
		icon, ink, title = icWarnCircle, t.Danger, tx(l, xSkillAddResultsFailed)
	}
	ui.Row(c).Gap(8).Padding(0, 12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).Size(16, 16).TextColor(ink)
		ui.Text(c, title).FontSize(14).FontWeight(600)
	})
	Panel(c).Children(func() {
		for i, p := range w.plans {
			settingsRule(c, i)
			ui.Row(c).Gap(10).Padding(9, 0).AlignItems(ui.Start).Children(func() {
				skillTargetIcon(c, p.target)
				ui.Column(c).Gap(3).Grow(1).Basis(0).Children(func() { skillTargetOutcome(c, l, p) })
			})
		}
	})
	mcpInfoLine(c, icInfo, tx(l, xSkillAddReloadHint), t.TextMuted)
}

func skillTargetOutcome(c *ui.Context, l l10n.Localizer, p skillAddTarget) {
	t := c.Theme()
	ui.Text(c, skillTargetName(l, p.target)).FontSize(13).FontWeight(600)
	if p.err != nil {
		ui.Text(c, skillAddErrorText(l, p.err)).FontSize(11).TextColor(t.Danger)
		return
	}
	for _, o := range p.result.Items {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if o.Err != nil {
				ui.Icon(c, icWarnCircle).Size(12, 12).TextColor(t.Danger)
				ui.Text(c, o.Name+" · "+skillAddErrorText(l, o.Err)).FontSize(11).TextColor(t.Danger).Grow(1).Basis(0)
				return
			}
			ui.Icon(c, icCheckCircle).Size(12, 12).TextColor(t.Success)
			ui.Text(c, o.Name).FontSize(11).FontWeight(600).Shrink(0)
			ui.Text(c, o.Path).FontSize(11).Font("monospace").TextColor(t.TextMuted).SingleLine().Ellipsis("…").Shrink(1).Selectable()
		})
	}
	if len(p.result.Items) == 0 {
		ui.Text(c, tx(l, xSkillAddNothing)).FontSize(11).TextColor(t.TextMuted)
	}
	if p.result.RecordErr != nil {
		mcpInfoLine(c, icWarnCircle, tx(l, xSkillAddRecordFailed), t.Warning)
	}
}

// skillAddErrorText names an import or install error by its code, with
// at most a path in the source, a ref or an HTTP status after it.
func skillAddErrorText(l l10n.Localizer, err error) string {
	var ie *skills.ImportError
	switch {
	case errors.As(err, &ie):
		message := tx(l, skillImportErrorKey(ie.Code))
		if ie.Detail != "" {
			message += " · " + ie.Detail
		}
		return message
	case errors.Is(err, errSkillAddSource):
		return tx(l, xSkillAddErrSource)
	case errors.Is(err, context.Canceled):
		return tx(l, xSkillAddErrCancelled)
	}
	return tx(l, xSkillAddFailed)
}

func skillImportErrorKey(code string) textKey {
	keys := map[string]textKey{
		skills.ImportInvalidSource: xSkillAddErrInvalidSource, skills.ImportNotFound: xSkillAddErrNotFound,
		skills.ImportRateLimited: xSkillAddErrRateLimited, skills.ImportNetwork: xSkillAddErrNetwork, skills.ImportHTTP: xSkillAddErrHTTP,
		skills.ImportTooLarge: xSkillAddErrTooLarge, skills.ImportTooManyFiles: xSkillAddErrTooManyFiles,
		skills.ImportInvalidPath: xSkillAddErrInvalidPath, skills.ImportInvalidArchive: xSkillAddErrArchive,
		skills.ImportCaseConflict: xSkillAddErrCase, skills.ImportNoSkills: xSkillAddErrNoSkills, skills.ImportExists: xSkillAddErrExists,
		skills.ImportUnsafeRoot: xSkillAddErrUnsafeRoot, skills.ImportChanged: xSkillAddErrChanged, skills.ImportWrite: xSkillAddErrWrite,
		skills.ImportNotUpdatable: xSkillAddErrNotUpdatable, skills.ImportModified: xSkillAddErrModified, skills.ImportMissing: xSkillAddErrMissing,
	}
	if key, ok := keys[code]; ok {
		return key
	}
	return xSkillAddFailed
}

// importCode is the code of an import error, or "".
func importCode(err error) string {
	var ie *skills.ImportError
	if errors.As(err, &ie) {
		return ie.Code
	}
	return ""
}
