package app

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexdesktop"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

type codexRecoveryDialog struct {
	sources                    []codexdesktop.Source
	target                     codexdesktop.Owner
	selected                   int
	plan                       *codexdesktop.Plan
	busy, loaded, done, closed bool
	undo                       bool
	failure                    error
	load                       func() (codexdesktop.Owner, []codexdesktop.Source, error)
	prepare                    func(codexdesktop.Request) (*codexdesktop.Plan, error)
	apply                      func(codexdesktop.Owner, *codexdesktop.Plan) (codexdesktop.Outcome, error)
}

func newCodexRecovery(rt *liveApp) *codexRecoveryDialog {
	d := &codexRecoveryDialog{selected: -1, prepare: rt.desktopSharer.Prepare}
	d.load = func() (codexdesktop.Owner, []codexdesktop.Source, error) {
		target, err := rt.svc.DesktopProjectOwner()
		if err != nil {
			return target, nil, err
		}
		sources, err := rt.desktopSharer.Sources()
		return target, sources, err
	}
	d.apply = func(target codexdesktop.Owner, plan *codexdesktop.Plan) (codexdesktop.Outcome, error) {
		return rt.svc.RestoreDesktopProjects(context.Background(), target, func() (codexdesktop.Outcome, error) { return rt.desktopSharer.Apply(plan) })
	}
	return d
}

func codexRecoveryAvailable(rt *liveApp, view runway.AccountsView) bool {
	if !codexDesktopShareSupported || rt == nil || rt.codex == nil {
		return false
	}
	for _, a := range view.Sidebar {
		if a.ID == view.ActiveID {
			return codexShareOffered(a)
		}
	}
	return false
}

func codexRecoveryEntry(c *ui.Context, l l10n.Localizer, slot **codexRecoveryDialog, rt *liveApp, view runway.AccountsView, privacy, busy bool, post func(func())) {
	if !codexDesktopShareSupported {
		return
	}
	if PillButton(c, nil, tx(l, xProjectRecover), busy || !codexRecoveryAvailable(rt, view)).Clicked() {
		*slot = newCodexRecovery(rt)
	}
	if PillButton(c, nil, tx(l, xCodexDesktopRestore), busy || !codexRecoveryAvailable(rt, view)).Clicked() {
		d := &codexRecoveryDialog{undo: true, loaded: true, selected: -1, plan: &codexdesktop.Plan{}}
		d.apply = func(codexdesktop.Owner, *codexdesktop.Plan) (codexdesktop.Outcome, error) {
			return codexdesktop.Shared, rt.undoDesktopProjects()
		}
		*slot = d
	}
	renderCodexRecovery(c, l, slot, view.Sidebar, privacy, post)
}

// run keeps all I/O off the UI thread and delivers only to this dialog attempt.
func (d *codexRecoveryDialog) run(action string, post func(func())) {
	if d.busy || d.closed || action == "apply" && d.emptyPreview() {
		return
	}
	d.busy, d.failure = true, nil
	target, selected, plan := d.target, d.selected, d.plan
	var source string
	if selected >= 0 && selected < len(d.sources) {
		source = d.sources[selected].Key
	}
	if action == "load" {
		d.plan, d.loaded = nil, false
	}
	go func() {
		var err error
		var sources []codexdesktop.Source
		var outcome codexdesktop.Outcome
		switch action {
		case "load":
			target, sources, err = d.load()
		case "preview":
			plan, err = d.prepare(codexdesktop.Request{SourceKey: source, Target: target})
		case "apply":
			outcome, err = d.apply(target, plan)
		}
		post(func() {
			d.busy = false
			if d.closed {
				return
			}
			d.failure = err
			if err != nil {
				return
			}
			switch action {
			case "load":
				sources = slices.DeleteFunc(sources, func(s codexdesktop.Source) bool { return s.Key == target.Key() })
				d.target, d.sources, d.loaded = target, sources, true
				d.plan, d.selected = nil, -1
				for i, s := range sources {
					if s.Key == source {
						d.selected = i
						break
					}
				}
			case "preview":
				d.plan = plan
			case "apply":
				if outcome == codexdesktop.Shared || outcome == codexdesktop.SharedWithSkips || outcome == codexdesktop.Unchanged {
					d.done = true
				} else {
					d.failure = codexdesktop.ErrUnavailable
				}
			}
		})
	}()
}

func renderCodexRecovery(c *ui.Context, l l10n.Localizer, slot **codexRecoveryDialog, accounts []codex.ManagedAccount, privacy bool, post func(func())) {
	d := *slot
	if d == nil {
		return
	}
	if !d.loaded && !d.busy && d.failure == nil {
		d.run("load", post)
	}
	open := true
	ui.Modal(c.Key("desktop-project-restore"), &open, func() {
		title, hint := xProjectRecover, xProjectRecoveryHint
		if d.undo {
			title, hint = xCodexDesktopRestore, xCodexDesktopRestoreHint
		}
		ui.Text(c, tx(l, title)).FontSize(17).FontWeight(600)
		dimText(c, tx(l, hint))
		if d.loaded && !d.undo {
			name := tx(l, xProjectCurrentAccount)
			for _, a := range accounts {
				if (codexdesktop.Owner{UserID: a.UserID, AccountID: a.AccountID}) == d.target {
					name = maskAccount(privacy, a.ResolvedDisplayName())
					break
				}
			}
			dimText(c, tx(l, xProjectTarget, name))
		}
		if d.done {
			key := xProjectRecovered
			if d.undo {
				key = xCodexDesktopRestored
			}
			ui.Text(c, tx(l, key))
		} else if !d.undo {
			renderCodexRecoverySources(c, l, d, accounts, privacy)
			if d.loaded && len(d.sources) == 0 {
				dimText(c, tx(l, xProjectNoSource))
			}
		}
		if d.plan != nil && !d.undo {
			s := d.plan.Summary
			if d.emptyPreview() {
				dimText(c, tx(l, xProjectNothingRecoverable))
			}
			ui.Text(c, tx(l, xProjectSummary, s.Projects, s.Threads, s.Skipped)).FontSize(12)
			for _, reason := range s.Reasons {
				dimText(c, desktopSkipReason(l, reason))
			}
		}
		if d.failure != nil {
			ui.Text(c, desktopRecoveryError(l, d.failure)).FontSize(12).TextColor(c.Theme().Danger)
		}
		ui.Row(c).Gap(8).Children(func() {
			if d.busy {
				ui.Spinner(c).Size(14, 14)
			}
			if d.failure != nil && !d.undo && PillButton(c, nil, tx(l, xProjectRecheck), d.busy).Clicked() {
				d.run("load", post)
			}
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyCancel), d.busy).Clicked() {
				open = false
			}
			if !d.done && d.plan == nil {
				if PillButton(c, nil, tx(l, xProjectPreview), d.busy || !d.loaded || d.selected < 0).Clicked() {
					d.run("preview", post)
				}
			} else if !d.done {
				confirm := xProjectConfirm
				if d.undo {
					confirm = xCodexDesktopRestore
				}
				changed := errors.Is(d.failure, codexdesktop.ErrStateChanged) || errors.Is(d.failure, runway.ErrDesktopProjectTargetChanged)
				if PillButton(c, nil, tx(l, confirm), d.busy || changed || d.emptyPreview()).Clicked() {
					d.run("apply", post)
				}
			}
		})
	}).Width(560)
	if !open && !d.busy {
		d.closed = true
		*slot = nil
	}
}

func desktopSourceLabel(l l10n.Localizer, source codexdesktop.Source, i int, accounts []codex.ManagedAccount, privacy bool) string {
	name := tx(l, xProjectSnapshot, i+1)
	for _, a := range accounts {
		if (codexdesktop.Owner{UserID: a.UserID, AccountID: a.AccountID}).Key() == source.Key {
			name = maskAccount(privacy, a.ResolvedDisplayName())
			break
		}
	}
	if !privacy && len(source.Names) > 0 {
		name += " · " + strings.Join(source.Names[:min(2, len(source.Names))], ", ")
	}
	runes := []rune(name)
	if len(runes) > 32 {
		name = string(runes[:32]) + "…"
	}
	return name + " · " + tx(l, xProjectCount, source.Projects)
}

func desktopRecoveryError(l l10n.Localizer, err error) string {
	restart := errors.Is(err, runway.ErrDesktopProjectRestart)
	dataFailure := errors.Is(err, runway.ErrDesktopProjects) || errors.Is(err, codexdesktop.ErrPartialWrite) || errors.Is(err, codexdesktop.ErrConflict)
	if restart && !dataFailure {
		return tx(l, xProjectRestartFailed)
	}
	message := desktopRecoveryPrimaryError(l, err)
	if restart || dataFailure && errors.Is(err, runway.ErrCodexRuntime) {
		message += "\n" + tx(l, xProjectReopen)
	}
	return message
}

func desktopRecoveryPrimaryError(l l10n.Localizer, err error) string {
	switch {
	case errors.Is(err, codexdesktop.ErrPartialWrite):
		return tx(l, xProjectPartial)
	case errors.Is(err, codexdesktop.ErrConflict):
		return tx(l, xProjectConflict)
	case errors.Is(err, codexdesktop.ErrStateChanged), errors.Is(err, runway.ErrDesktopProjectTargetChanged):
		return tx(l, xProjectChanged)
	case errors.Is(err, codexdesktop.ErrDatabase):
		return tx(l, xProjectDatabase)
	case errors.Is(err, codexdesktop.ErrIO):
		return tx(l, xProjectIO)
	case errors.Is(err, codexdesktop.ErrValidation):
		return tx(l, xProjectSkipUnknown)
	case errors.Is(err, codexdesktop.ErrNoSource):
		return tx(l, xProjectNoSource)
	case errors.Is(err, codexdesktop.ErrDesktopRunning):
		return tx(l, xCodexDesktopQuitFirst)
	case errors.Is(err, codexdesktop.ErrNoBackup):
		return tx(l, xCodexDesktopNoBackup)
	case errors.Is(err, codexdesktop.ErrUnknownFormat):
		return tx(l, xProjectFormat)
	case errors.Is(err, runway.ErrSwitchBusy):
		return tx(l, xProjectBusy)
	case errors.Is(err, runway.ErrCodexRuntime) && !errors.Is(err, runway.ErrDesktopProjects):
		return tx(l, xProjectStopFailed)
	default:
		return tx(l, xCodexDesktopRestoreFailed)
	}
}

func renderCodexRecoverySources(c *ui.Context, l l10n.Localizer, d *codexRecoveryDialog, accounts []codex.ManagedAccount, privacy bool) {
	body := func() {
		for i, source := range d.sources {
			label := desktopSourceLabel(l, source, i, accounts, privacy)
			if PillButton(c.Key(source.Key), nil, label, d.busy).Clicked() {
				d.selected, d.plan, d.failure = i, nil, nil
			}
			if i == d.selected {
				dimText(c, tx(l, xProjectSelected))
			}
		}
	}
	if len(d.sources) > 4 {
		ui.Scroll(c.Key("recovery-sources")).Height(180).Children(func() { ui.Column(c).Gap(8).Children(body) })
	} else {
		body()
	}
}

func (d *codexRecoveryDialog) emptyPreview() bool {
	return !d.undo && (d.plan == nil || d.plan.Summary.Projects == 0 && d.plan.Summary.Threads == 0)
}
