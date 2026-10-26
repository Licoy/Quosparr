package app

import (
	"errors"
	"github.com/licoy/quosparr/internal/claude"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/l10n"
)

func renderClaudeLogin(c *ui.Context, u *settingsUI, f *accountForm) {
	a := f.claudeLogin
	if a == nil {
		return
	}
	l, open := u.L(), true
	ui.Modal(c.Key("claude-browser-login"), &open, func() {
		ui.Text(c, l.Text(l10n.KeyAccountsAddOAuth)).FontSize(17).FontWeight(600)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, f.status).FontSize(12)
		})
		ui.Text(c, tx(l, xClaudeOAuthHint)).FontSize(12).TextColor(c.Theme().TextMuted)
		if a.waiting {
			ui.Text(c, tx(l, xClaudeOAuthManual)).FontSize(12)
			ui.TextInput(c.Key("claude-login-callback"), &a.input).Label(tx(l, xClaudeOAuthManual))
			if a.problem != "" {
				ui.Text(c, a.problem).FontSize(12).TextColor(c.Theme().Danger)
			}
		}
		ui.Row(c).Gap(8).Children(func() {
			if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
				u.stopClaudeLogin(f, a)
			}
			if a.waiting {
				if PillButton(c, nil, tx(l, xClaudeOAuthReopen), a.opening).Clicked() {
					u.reopenClaudeLogin(f, a)
				}
				if PillButton(c, nil, tx(l, xClaudeOAuthSubmit), strings.TrimSpace(a.input) == "").Clicked() {
					u.submitClaudeLogin(a)
				}
			}
		})
	}).Width(560)
	if !open {
		u.stopClaudeLogin(f, a)
	}
}

func (u *settingsUI) submitClaudeLogin(a *claudeLoginAttempt) {
	raw := a.input
	a.input, a.problem = "", ""
	if err := a.server.Submit(raw); err != nil {
		a.problem = claudeLoginFailure(u.L(), err)
		if errors.Is(err, claude.ErrLoginCallback) {
			a.problem = tx(u.L(), xClaudeOAuthManualInvalid)
		}
	}
}

func (u *settingsUI) reopenClaudeLogin(f *accountForm, a *claudeLoginAttempt) {
	if f.claudeLogin != a || !a.waiting || a.opening {
		return
	}
	a.opening, a.problem = true, ""
	go func() {
		err := a.browser(a.ctx, a.session.AuthURL)
		u.post(func() {
			if f.claudeLogin != a || !a.waiting {
				return
			}
			a.opening = false
			if err != nil {
				a.problem = accountFailure(u.L(), tx(u.L(), xOAuthBrowser), err)
			}
		})
	}()
}
