package app

import (
	"context"
	"errors"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/claudedesktop"
	"github.com/licoy/quosparr/internal/l10n"
)

// One dialog owns its cancellation; results only update the same attempt.
// Cleanup runs before UI delivery, including when its owner window closed.
type desktopDialog struct {
	rt                                       *liveApp
	id, name, path                           string
	profile                                  claudedesktop.Profile
	busy, consent, confirmed, openOnly, done bool
	activeBinding                            bool
	identityLoading, identityReady           bool
	loadAccount                              func(context.Context) (claude.AccountState, error)
	outcome                                  *claudedesktop.SwitchResult
	problem                                  string
	failure                                  error
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	post                                     func(func())
	privacy                                  bool
}

func newDesktopDialog(rt *liveApp, id string, privacy bool) *desktopDialog {
	ctx, cancel := context.WithCancel(context.Background())
	d := &desktopDialog{rt: rt, id: id, ctx: ctx, cancel: cancel, post: runOnMain, privacy: privacy}
	if rt == nil || rt.claudeDesktop == nil || rt.claudeMod == nil {
		return d
	}
	if id != "" {
		d.loadAccount = rt.claudeMod.Load
	}
	var err error
	d.profile, err = rt.claudeDesktop.Status(ctx, id)
	if err != nil {
		d.problem = "failed"
		d.failure = err
	}
	d.path = d.profile.ApplicationPath
	return d
}

func (d *desktopDialog) close() {
	if d == nil || d.done {
		return
	}
	d.cancel()
	if !d.busy {
		if d.activeBinding {
			d.run("cancel")
		} else {
			d.done = true
		}
	}
}

func (d *desktopDialog) run(action string) {
	if d.busy {
		return
	}
	if d.rt == nil || d.rt.claudeDesktop == nil {
		d.done = true
		return
	}
	d.busy = true
	d.problem = ""
	d.failure = nil
	d.outcome = nil
	ctx, path, post, previous := d.ctx, d.path, d.post, d.profile
	ownedBinding := d.activeBinding || action == "begin"
	go func() {
		p := previous
		var err error
		var r claudedesktop.SwitchResult
		m := d.rt.claudeDesktop
		switch action {
		case "begin":
			p, err = m.BeginBinding(ctx, d.id, path)
		case "confirm":
			p, err = m.ConfirmBinding(ctx, d.id)
		case "open":
			r, err = m.Open(ctx, d.id)
		case "recover":
			r, err = m.Recover(context.Background())
		case "cancel":
			err = errors.Join(err, m.CancelBinding(context.Background(), d.id))
		}
		closing := ctx.Err() != nil
		if closing && action != "cancel" && ownedBinding {
			status, statusErr := m.Status(context.Background(), d.id)
			if statusErr != nil {
				err = errors.Join(err, statusErr)
			} else if status.Pending {
				cleanupErr := m.CancelBinding(context.Background(), d.id)
				err = errors.Join(err, cleanupErr)
				if cleanupErr == nil {
					r.PreviousDesktopRestored = true
				}
			}
		}
		if closing && harmlessDesktopCancellation(err) {
			err = nil
		}
		if err != nil {
			status, statusErr := m.Status(context.Background(), d.id)
			if statusErr == nil {
				p = status
			} else {
				err = errors.Join(err, statusErr)
			}
		}
		deliver := func() {
			d.busy = false
			d.profile = p
			if err != nil || action == "open" || action == "recover" {
				d.outcome = &r
			}
			if action == "begin" && err == nil && !closing {
				d.activeBinding = true
			}
			if err != nil {
				d.problem = "failed"
				d.consent = false
				d.failure = err
				if closing {
					d.activeBinding = false
				}
				d.ctx, d.cancel = context.WithCancel(context.Background())
				return
			}
			if closing || action == "cancel" || action == "confirm" {
				d.done = true
			}
			if action == "recover" {
				status, statusErr := m.Status(context.Background(), d.id)
				if statusErr == nil {
					d.profile = status
				} else {
					d.failure = statusErr
				}
			}
			if action == "open" || action == "recover" {
				d.problem = "opened"
				d.outcome = &r
			}
		}
		if post != nil {
			post(deliver)
		} else {
			deliver()
		}
	}()
}

func renderDesktopDialog(c *ui.Context, l l10n.Localizer, slot **desktopDialog, rt *liveApp, post func(func())) {
	d := *slot
	if d == nil {
		return
	}
	d.post = post
	d.loadIdentity(post)
	if d.done {
		*slot = nil
		return
	}
	open := true
	ui.Modal(c.Key("claude-desktop-dialog"), &open, func() {
		title := xDesktopBind
		if d.openOnly {
			title = xDesktopOpen
		}
		if d.profile.RecoveryRequired && !d.activeBinding {
			title = xDesktopRecoveryTitle
		}
		ui.Text(c, tx(l, title)).FontSize(17).FontWeight(600)
		ui.Text(c, d.name).FontSize(12)
		if d.identityLoading {
			ui.Text(c, tx(l, xDesktopLoading)).FontSize(11)
		}
		ui.Text(c, tx(l, xDesktopWarning)).FontSize(12)
		if d.problem != "" {
			key := xDesktopFailed
			if d.problem == "opened" {
				key = xDesktopStarted
			}
			text := tx(l, key)
			if d.failure != nil {
				text = desktopFailureText(l, d.failure)
			}
			if d.outcome != nil {
				text = desktopResultText(l, *d.outcome, d.failure)
			}
			ui.Text(c, text).FontSize(12).TextColor(c.Theme().TextMuted)
		}
		if d.profile.RecoveryRequired && !d.activeBinding {
			ui.Text(c, tx(l, xDesktopRecovery)).FontSize(12)
			ui.Checkbox(c.Key("desktop-recovery-consent"), &d.consent, tx(l, xDesktopConsent)).Disabled(d.busy)
			if PillButton(c, nil, tx(l, xDesktopRecover), d.busy || !d.consent).Clicked() {
				d.run("recover")
			}
		} else if d.profile.Pending {
			ui.Text(c, tx(l, xDesktopPending)).FontSize(12)
			ui.Checkbox(c.Key("desktop-confirm-account"), &d.confirmed, tx(l, xDesktopConfirmIdentity)).Disabled(d.busy)
			if PillButton(c, nil, tx(l, xDesktopFinish), d.busy || !d.confirmed || !d.identityReady).Clicked() {
				d.run("confirm")
			}
		} else {
			renderDesktopStart(c, l, d)
		}
		if d.busy {
			ui.Spinner(c).Size(14, 14)
		}
		if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
			d.close()
		}
	}).Width(560)
	if !open {
		d.close()
	}
}

func renderDesktopStart(c *ui.Context, l l10n.Localizer, d *desktopDialog) {
	if !d.profile.IsBound() {
		ui.Text(c, tx(l, xDesktopUnbound)).FontSize(12)
	}
	if d.profile.IsBound() {
		ui.Text(c, tx(l, xDesktopBound)).FontSize(12)
	}
	ui.Text(c, tx(l, xDesktopPath)).FontSize(11).TextColor(c.Theme().TextMuted)
	ui.TextInput(c.Key("desktop-app-path"), &d.path).Label(tx(l, xDesktopPath)).Disabled(d.busy || d.openOnly)
	if PillButton(c, nil, tx(l, xDesktopChoose), d.busy || d.openOnly).Clicked() {
		opts := mygo.OpenDialogOptions{Title: tx(l, xDesktopChoose)}
		if runtime.GOOS == "darwin" {
			opts.Filters = []mygo.FileFilter{{Name: ".app", Extensions: []string{"app"}}}
		}
		if runtime.GOOS == "windows" {
			opts.Filters = []mygo.FileFilter{{Name: ".exe", Extensions: []string{"exe"}}}
		}
		paths, err := mygo.Dialog.Open(opts)
		if err != nil {
			d.problem = "failed"
		} else if len(paths) > 0 {
			d.path = paths[0]
		}
	}
	ui.Checkbox(c.Key("desktop-consent"), &d.consent, tx(l, xDesktopConsent)).Disabled(d.busy)
	title, action := xDesktopStart, "begin"
	if d.openOnly {
		title, action = xDesktopOpen, "open"
	}
	if PillButton(c, nil, tx(l, title), d.busy || !d.consent || !d.identityReady || d.rt == nil || d.rt.claudeDesktop == nil).Clicked() {
		d.run(action)
	}
}

func desktopSwitchOption(c *ui.Context, l l10n.Localizer, rt *liveApp, id string, on *bool, busy bool, bind func()) {
	bound := desktopBound(rt, id)
	ui.Checkbox(c.Key("desktop-switch"), on, tx(l, xDesktopSwitch)).Disabled(busy || !bound)
	ui.Text(c, tx(l, xDesktopWarning)).FontSize(11).TextColor(c.Theme().TextMuted)
	if !bound {
		ui.Text(c, tx(l, xDesktopUnbound)).FontSize(11)
		if PillButton(c, nil, tx(l, xDesktopBind), busy).Clicked() {
			bind()
		}
	}
}
