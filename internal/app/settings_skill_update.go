package app

import (
	"context"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/skills"
)

// skillUpdateState is the update check of the open skill page. force is
// set once an update was refused for a copy edited since it was added.
type skillUpdateState struct {
	id      string
	task    Task
	check   skills.UpdateCheck
	cancel  context.CancelFunc
	gen     uint64
	confirm bool
	force   bool
}

func (st *skillUpdateState) stop() {
	if st.cancel != nil {
		st.cancel()
	}
	*st = skillUpdateState{gen: st.gen + 1}
}

func (u *settingsUI) checkSkillUpdate(acts SkillActions, sk skills.Skill) {
	st := &u.skillsState().update
	if acts.CheckUpdate == nil || st.task.Running {
		return
	}
	st.stop()
	st.id = sk.ID
	ctx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	st.task = Task{Running: true}
	gen, check := st.gen, acts.CheckUpdate
	go func() {
		got := check(ctx, sk)
		u.post(func() {
			if ctx.Err() != nil || gen != st.gen {
				return
			}
			st.cancel = nil
			st.task = Task{Err: got.Err}
			st.check = got
		})
	}()
}

// updateSkill replaces the skill with its repository's latest version;
// the old one goes to the skills trash.
func (u *settingsUI) updateSkill(acts SkillActions, sum skills.Summary, sk skills.Skill) {
	st := &u.skillsState().update
	l, update, force := u.L(), acts.Update, st.force
	u.skillWork(acts, func() (string, error) {
		res, err := update(context.Background(), skills.UpdateRequest{Skill: sk, Roots: sum.Roots, Force: force})
		switch {
		case err != nil:
			return "", err
		case res.Unchanged:
			return tx(l, xSkillUpdateUnchanged), nil
		case res.OldInSystemTrash:
			return tx(l, xSkillUpdatedSystem), nil
		}
		return tx(l, xSkillUpdated), nil
	}, xSkillUpdateFailed, func(err error) {
		modified := importCode(err) == skills.ImportModified
		st.stop()
		st.id, st.force = sk.ID, modified
	})
}

// skillUpdateControls sit in the origin box of a skill added from GitHub:
// the check, its outcome and, when there is something new, the update.
func (u *settingsUI) skillUpdateControls(c *ui.Context, sum skills.Summary, sk skills.Skill, acts SkillActions) {
	s := u.skillsState()
	st := &s.update
	if st.id != sk.ID {
		st.stop()
		st.id = sk.ID
	}
	l, t := u.L(), c.Theme()
	busy := st.task.Running || s.work.Running
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		switch {
		case st.task.Running:
			ui.Spinner(c).Size(12, 12)
			ui.Text(c, tx(l, xSkillUpdateChecking)).FontSize(11).TextColor(t.TextMuted).SingleLine()
		case st.task.Err != nil:
			settingsBadge(c, icWarnCircle, skillAddErrorText(l, st.task.Err), t.Danger)
		case st.force:
			settingsBadge(c, icWarnCircle, tx(l, xSkillAddErrModified), t.Warning)
		case st.check.State == skills.UpdateCurrent:
			settingsBadge(c, icCheckCircle, tx(l, xSkillUpdateCurrent), t.Success)
		case st.check.State == skills.UpdateAvailable:
			settingsBadge(c, icCloudDown, tx(l, xSkillUpdateAvailable, shortCommit(st.check.Commit)), t.Accent)
		}
		if st.check.State == skills.UpdateAvailable || st.force {
			u.skillUpdateButton(c, sum, sk, acts, busy)
		}
		if SmallPillButton(c.Key("skill-update-check"), icRefresh, tx(l, xSkillUpdateCheck), busy || acts.CheckUpdate == nil).Clicked() {
			u.checkSkillUpdate(acts, sk)
		}
	})
}

func (u *settingsUI) skillUpdateButton(c *ui.Context, sum skills.Summary, sk skills.Skill, acts SkillActions, busy bool) {
	st := &u.skillsState().update
	l := u.L()
	label, message := tx(l, xSkillUpdate), tx(l, xSkillUpdateMessage, skillPath(sum, sk.Dir))
	if st.force {
		label, message = tx(l, xSkillUpdateForce), message+"\n\n"+tx(l, xSkillUpdateForceMessage)
	}
	b := AccentPillButton(c.Key("skill-update"), icCloudDown, label, busy || acts.Update == nil)
	if b.Clicked() && !busy && acts.Update != nil {
		st.confirm = true
	}
	confirmPopover(c, l, b, &st.confirm, tx(l, xSkillUpdateTitle), message, label, nil, func() { u.updateSkill(acts, sum, sk) })
}
