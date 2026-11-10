package app

import (
	"context"
	"errors"
	"slices"
	"strconv"
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

// codexRecoveryEntry sits under the runtime controls, in their layout: a
// caption, a muted line and ghost buttons for recovery and undo.
func codexRecoveryEntry(c *ui.Context, l l10n.Localizer, slot **codexRecoveryDialog, rt *liveApp, view runway.AccountsView, privacy, busy bool, post func(func())) {
	if !codexDesktopShareSupported {
		return
	}
	if codexRecoveryAvailable(rt, view) {
		ui.Column(c.Key("codex-desktop-projects")).Gap(6).Children(func() {
			ui.Text(c, tx(l, xProjectSection)).FontSize(12)
			ui.Text(c, tx(l, xProjectSectionHint)).FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if ghostButton(c, icFolder, tx(l, xProjectRecover), false, busy).Clicked() {
					*slot = newCodexRecovery(rt)
				}
				if ghostButton(c, icUndo, tx(l, xCodexDesktopRestore), false, busy).Clicked() {
					d := &codexRecoveryDialog{undo: true, loaded: true, selected: -1, plan: &codexdesktop.Plan{}}
					d.apply = func(codexdesktop.Owner, *codexdesktop.Plan) (codexdesktop.Outcome, error) {
						return codexdesktop.Shared, rt.undoDesktopProjects()
					}
					*slot = d
				}
			})
		})
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
		ui.Column(c).Gap(14).Children(func() {
			recoveryHeader(c, l, d, accounts, privacy)
			switch {
			case d.done:
				key := xProjectRecovered
				if d.undo {
					key = xCodexDesktopRestored
				}
				recoveryNotice(c, icCheckCircleFill, sys(c.Theme(), sysGreen), tx(l, key))
			case !d.undo:
				renderCodexRecoverySources(c, l, d, accounts, privacy, post)
			}
			if d.plan != nil && !d.undo {
				renderCodexRecoveryPreview(c, l, d.plan.Summary, d.emptyPreview(), d.done)
			}
			if d.failure != nil {
				recoveryNotice(c, icWarnCircle, c.Theme().Danger, desktopRecoveryError(l, d.failure))
			}
			recoveryFooter(c, l, d, &open, post)
		})
	}).Width(520)
	if !open && !d.busy {
		d.closed = true
		*slot = nil
	}
}

func recoveryHeader(c *ui.Context, l l10n.Localizer, d *codexRecoveryDialog, accounts []codex.ManagedAccount, privacy bool) {
	title, hint := xProjectRecover, xProjectRecoveryHint
	if d.undo {
		title, hint = xCodexDesktopRestore, xCodexDesktopRestoreHint
	}
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, tx(l, title)).FontSize(17).FontWeight(600)
		ui.Text(c, tx(l, hint)).FontSize(fsCallout).TextColor(inkSecondary(c.Theme()))
	})
	if !d.loaded || d.undo {
		return
	}
	name := tx(l, xProjectCurrentAccount)
	for _, a := range accounts {
		if (codexdesktop.Owner{UserID: a.UserID, AccountID: a.AccountID}) == d.target {
			name = maskAccount(privacy, a.ResolvedDisplayName())
			break
		}
	}
	cardColumn(c, surfaceSunkenKind, radiusRow+2).Padding(8, 10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icUsers).Size(12, 12).TextColor(inkSecondary(c.Theme()))
			ui.Text(c, tx(l, xProjectTarget, name)).FontSize(fsCallout).SingleLine()
		})
	})
}

// recoveryNotice is a tinted line with an icon, for the outcome or an error.
func recoveryNotice(c *ui.Context, icon *ui.SVG, tint ui.Color, text string) {
	ui.Row(c).Gap(8).Padding(9, 11).Radius(radiusRow + 2).Background(tint.Alpha(0.10)).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icon).Size(13, 13).TextColor(tint).Margin(1, 0, 0, 0)
		ui.Text(c, text).FontSize(fsCallout).Grow(1).Shrink(1)
	})
}

func recoveryFooter(c *ui.Context, l l10n.Localizer, d *codexRecoveryDialog, open *bool, post func(func())) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(4, 0, 0, 0).Children(func() {
		if d.busy {
			ui.Spinner(c).Size(14, 14)
		}
		if d.failure != nil && !d.undo && !d.done && ghostButton(c, icRefresh, tx(l, xProjectRecheck), false, d.busy).Clicked() {
			d.run("load", post)
		}
		ui.Spacer(c).Grow(1)
		if d.done {
			if AccentPillButton(c, nil, tx(l, xProjectDone), d.busy).Clicked() {
				*open = false
			}
			return
		}
		if PillButton(c, nil, l.Text(l10n.KeyCancel), d.busy).Clicked() {
			*open = false
		}
		confirm := xProjectConfirm
		if d.undo {
			confirm = xCodexDesktopRestore
		}
		changed := errors.Is(d.failure, codexdesktop.ErrStateChanged) || errors.Is(d.failure, runway.ErrDesktopProjectTargetChanged)
		if AccentPillButton(c, nil, tx(l, confirm), d.busy || changed || d.emptyPreview()).Clicked() {
			d.run("apply", post)
		}
	})
}

func renderCodexRecoveryPreview(c *ui.Context, l l10n.Localizer, s codexdesktop.Summary, empty, done bool) {
	t := c.Theme()
	ui.Column(c).Gap(8).Children(func() {
		if !done {
			ui.Text(c, tx(l, xProjectPreviewHeading)).FontSize(fsCallout).FontWeight(600).TextColor(inkSecondary(t))
		}
		ui.Row(c).Gap(8).Label(tx(l, xProjectSummary, s.Projects, s.Threads, s.Skipped)).Children(func() {
			recoveryStat(c, tx(l, xProjectStatProjects), s.Projects, t.Text)
			recoveryStat(c, tx(l, xProjectStatThreads), s.Threads, t.Text)
			recoveryStat(c, tx(l, xProjectStatSkipped), s.Skipped, pick(s.Skipped > 0, sys(t, sysOrange), inkSecondary(t)))
		})
		if empty {
			dimText(c, tx(l, xProjectNothingRecoverable))
		}
		seen := map[string]bool{}
		for _, reason := range s.Reasons {
			text := desktopSkipReason(l, reason)
			if seen[text] {
				continue
			}
			seen[text] = true
			ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Icon(c, icInfo).Size(11, 11).TextColor(inkSecondary(t)).Margin(1, 0, 0, 0)
				ui.Text(c, text).FontSize(11).TextColor(inkSecondary(t)).Grow(1).Shrink(1)
			})
		}
	})
}

func recoveryStat(c *ui.Context, label string, n int, ink ui.Color) {
	cardColumn(c, surfaceSunkenKind, radiusRow+2).Padding(10, 12).Gap(2).Grow(1).Basis(0).Children(func() {
		ui.Text(c, strconv.Itoa(n)).FontSize(20).FontWeight(600).TextColor(ink)
		ui.Text(c, label).FontSize(11).TextColor(inkSecondary(c.Theme())).SingleLine()
	})
}

func desktopSourceName(l l10n.Localizer, source codexdesktop.Source, i int, accounts []codex.ManagedAccount, privacy bool) string {
	for _, a := range accounts {
		if (codexdesktop.Owner{UserID: a.UserID, AccountID: a.AccountID}).Key() == source.Key {
			return maskAccount(privacy, a.ResolvedDisplayName())
		}
	}
	return tx(l, xProjectSnapshot, i+1)
}

// desktopSourceLabel names a source card for assistive technology; project
// names never appear in privacy mode.
func desktopSourceLabel(l l10n.Localizer, source codexdesktop.Source, i int, accounts []codex.ManagedAccount, privacy bool) string {
	return desktopSourceName(l, source, i, accounts, privacy) + " · " + tx(l, xProjectSourceDetail, source.Projects, source.Threads)
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

func renderCodexRecoverySources(c *ui.Context, l l10n.Localizer, d *codexRecoveryDialog, accounts []codex.ManagedAccount, privacy bool, post func(func())) {
	if !d.loaded {
		return
	}
	if len(d.sources) == 0 {
		recoveryNotice(c, icInfo, sys(c.Theme(), sysGray), tx(l, xProjectNoSource))
		return
	}
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, tx(l, xProjectSourceHeading)).FontSize(fsCallout).FontWeight(600).TextColor(inkSecondary(c.Theme()))
		body := func() {
			for i, source := range d.sources {
				if recoverySourceCard(c, l, source, i, i == d.selected, d.busy, accounts, privacy).Clicked() && i != d.selected {
					d.selected, d.plan, d.failure = i, nil, nil
					d.run("preview", post)
				}
			}
		}
		if len(d.sources) > 3 {
			ui.Scroll(c.Key("recovery-sources")).MaxHeight(220).Gap(6).Children(body)
		} else {
			ui.Column(c).Gap(6).Children(body)
		}
	})
}

// recoverySourceCard is a radio card: the account or snapshot, its counts and,
// outside privacy mode, a few project names.
func recoverySourceCard(c *ui.Context, l l10n.Localizer, source codexdesktop.Source, i int, selected, busy bool, accounts []codex.ManagedAccount, privacy bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(source.Key)).Label(desktopSourceLabel(l, source, i, accounts, privacy)).Disabled(busy).
		Padding(10, 12).Gap(10).Radius(radiusRow + 2).AlignItems(ui.Start).Cursor(ui.CursorPointer)
	accent := brandInk(t, t.Accent)
	switch {
	case selected:
		b.Background(t.Accent.Alpha(0.10)).Border(1, t.Accent.Alpha(0.45))
	case b.Hovered() && !busy:
		b.Background(hoverNeutral(t)).Border(1, hairlineColor(t))
	default:
		b.Background(surfaceRaised(t)).Border(1, hairlineColor(t))
	}
	return b.Children(func() {
		if selected {
			ui.Icon(c, icCheckCircleFill).Size(14, 14).TextColor(accent).Margin(1, 0, 0, 0)
		} else {
			ui.Box(c).Size(14, 14).Radius(7).Border(1.5, inkTertiary(t)).Margin(1, 0, 0, 0)
		}
		ui.Column(c).Gap(3).Grow(1).Shrink(1).Children(func() {
			ui.Text(c, desktopSourceName(l, source, i, accounts, privacy)).FontSize(fsBody).FontWeight(600).SingleLine()
			ui.Text(c, tx(l, xProjectSourceDetail, source.Projects, source.Threads)).FontSize(11).TextColor(inkSecondary(t))
			if !privacy && len(source.Names) > 0 {
				names := strings.Join(source.Names[:min(4, len(source.Names))], ", ")
				if len(source.Names) > 4 {
					names += " …"
				}
				ui.Text(c, names).FontSize(11).TextColor(inkSecondary(t)).SingleLine()
			}
		})
	})
}

func (d *codexRecoveryDialog) emptyPreview() bool {
	return !d.undo && (d.plan == nil || d.plan.Summary.Projects == 0 && d.plan.Summary.Threads == 0)
}
