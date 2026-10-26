package app

import (
	"context"
	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/l10n"
)

// A requested cancel can close silently only if no meaningful operation or
// restoration failure accompanied it; joined errors retain those failures.
func harmlessDesktopCancellation(err error) bool {
	if err == nil || err == context.Canceled {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !harmlessDesktopCancellation(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		cause := wrapped.Unwrap()
		return cause != nil && harmlessDesktopCancellation(cause)
	}
	return false
}

func (d *desktopDialog) loadIdentity(post func(func())) {
	if d.loadAccount == nil || d.identityLoading || d.identityReady {
		return
	}
	d.identityLoading = true
	load, ctx := d.loadAccount, d.ctx
	go func() {
		state, err := load(ctx)
		var identity claude.CredentialIdentity
		found := false
		for _, a := range state.Accounts {
			if a.ID == d.id && !a.IsAPI() {
				identity = a.Identity
				found = true
				break
			}
		}
		post(func() {
			if ctx.Err() != nil {
				return
			}
			d.identityLoading = false
			d.loadAccount = nil
			if err != nil || !found {
				d.problem = "failed"
				d.failure = err
				return
			}
			organization := identity.OrganizationName
			if organization == "" {
				organization = identity.OrganizationUUID
			}
			d.name = maskAccount(d.privacy, identity.ResolvedDisplayName()) + " / " + maskAccount(d.privacy, organization)
			d.identityReady = true
		})
	}()
}

// Recovery is independent of account rows, including a removed account or a
// library containing only API accounts after an interrupted operation.
func desktopRecoveryButton(c *ui.Context, l l10n.Localizer, slot **desktopDialog, rt *liveApp) {
	if *slot != nil || rt == nil || rt.claudeDesktop == nil {
		return
	}
	p, err := rt.claudeDesktop.Status(context.Background(), "")
	if err != nil {
		ui.Text(c, desktopFailureText(l, err)).FontSize(11)
		return
	}
	if p.RecoveryRequired && PillButton(c, nil, tx(l, xDesktopRecover), false).Clicked() {
		*slot = newDesktopDialog(rt, "", true)
	}
}
