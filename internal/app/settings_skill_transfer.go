package app

import (
	"errors"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/skills"
	"github.com/licoy/quosparr/internal/store"
)

// skillTransferMenu copies a skill to another CLI, or asks before moving it.
func (u *settingsUI) skillTransferMenu(c *ui.Context, sum skills.Summary, sk skills.Skill, acts SkillActions) {
	s := u.skillsState()
	l := u.L()
	b := IconButton(c, icCopy, tx(l, xSkillsTransfer), acts.Transfer == nil || s.work.Running)
	b.Menu(func(m *ui.Menu) {
		m.Submenu(tx(l, xSkillsCopyTo), func(m *ui.Menu) {
			u.skillTransferTargets(m, l, sk, false, func(target skills.Provider) {
				u.runSkillTransfer(acts, sum, sk, target, false)
			})
		})
		canMove := skills.CanMoveSkill(sk)
		m.Submenu(tx(l, xSkillsMoveTo), func(m *ui.Menu) {
			u.skillTransferTargets(m, l, sk, !canMove, func(target skills.Provider) {
				s.transferID, s.transferTarget = sk.ID, target
			})
		}).Disabled(!canMove)
		if !canMove {
			m.Item(tx(l, xSkillsTransferNotMovable)).Disabled(true)
		}
	})

	open := s.transferID == sk.ID && s.transferTarget != ""
	if open {
		target := s.transferTarget
		name := l.Text(providerKey(store.Provider(target)))
		confirmPopover(c, l, b, &open, tx(l, xSkillsMoveTitle, skillTitle(sk), name), tx(l, xSkillsMoveMessage, name, skillPath(sum, sk.Dir)), tx(l, xSkillsMove), func() {
			if len(sk.SharedWith) > 0 {
				ui.Text(c, tx(l, xSkillsDeleteShared, skillProviders(l, sk.SharedWith))).FontSize(12).TextColor(c.Theme().Warning)
			}
		}, func() {
			s.transferID, s.transferTarget = "", ""
			u.runSkillTransfer(acts, sum, sk, target, true)
		})
	}
	if !open && s.transferID == sk.ID {
		s.transferID, s.transferTarget = "", ""
	}
}

func (u *settingsUI) skillTransferTargets(m *ui.Menu, l l10n.Localizer, sk skills.Skill, disabled bool, choose func(skills.Provider)) {
	for _, p := range store.Providers() {
		target := skills.Provider(p)
		if target == sk.Provider {
			continue
		}
		if m.Item(l.Text(providerKey(p))).Disabled(disabled).Chosen() {
			choose(target)
		}
	}
}

func (u *settingsUI) runSkillTransfer(acts SkillActions, sum skills.Summary, sk skills.Skill, target skills.Provider, move bool) {
	if acts.Transfer == nil {
		return
	}
	l := u.L()
	u.skillWork(acts, func() (string, error) {
		got, err := acts.Transfer(skills.TransferRequest{Skill: sk, Roots: sum.Roots, Target: target, Move: move})
		if err != nil {
			if errors.Is(err, skills.ErrTransferPartial) {
				return "", &skillTransferPartialError{path: got.Path, err: err}
			}
			return "", err
		}
		if !move {
			return tx(l, xSkillsCopiedTo, got.Path), nil
		}
		return tx(l, xSkillsMovedTo, got.Path, skillMovedText(l, got.SourceInSystemTrash)), nil
	}, xSkillsTransferFailed, func(err error) {
		s := u.skillsState()
		if err == nil && move && s.open == sk.ID {
			notice := s.notice
			s.closeItem()
			s.notice = notice
		}
	})
}

type skillTransferPartialError struct {
	path string
	err  error
}

func (e *skillTransferPartialError) Error() string { return e.err.Error() }
func (e *skillTransferPartialError) Unwrap() error { return e.err }
