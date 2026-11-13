package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
)

// geminiLoginAttempt is one Google sign-in: its callback listener, the
// pasted callback and the browser state. UI deliveries compare the pointer
// so a closed or retried login cannot receive stale results.
type geminiLoginAttempt struct {
	ctx              context.Context
	cancel           context.CancelFunc
	server           *gemini.LoginCallbackServer
	browser          func(context.Context, string) error
	input, problem   string
	waiting, opening bool
}

type geminiLoginFlow struct {
	callbackText gemini.LoginCallbackText
	module       *gemini.Module
	client       gemini.OAuthClient
	openBrowser  func(context.Context, string) error
	lifetime     time.Duration
}

func (flow geminiLoginFlow) prepare(ctx context.Context) (*geminiLoginAttempt, error) {
	server, err := gemini.StartLoginCallback(time.Now(), flow.lifetime, flow.callbackText)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, server.Session().ExpiresAt)
	return &geminiLoginAttempt{ctx: ctx, cancel: cancel, server: server, browser: flow.openBrowser, waiting: true}, nil
}

func (flow geminiLoginFlow) run(a *geminiLoginAttempt, stage func(textKey)) (textKey, error) {
	defer a.cancel()
	defer a.server.Cancel()
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	session := a.server.Session()
	if err := a.browser(a.ctx, session.AuthURL); err != nil {
		return "", &claudeLoginError{xOAuthBrowser, err}
	}
	code, err := a.server.Wait(a.ctx)
	if err != nil {
		return "", err
	}
	stage(xClaudeOAuthVerify)
	result, err := flow.client.Exchange(a.ctx, code, session, time.Now())
	if err != nil {
		return "", err
	}
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	stage(xClaudeOAuthSaving)
	account, err := flow.module.ImportBrowserLogin(a.ctx, result)
	if err != nil {
		return "", &claudeLoginError{xOAuthStore, err}
	}
	stage(xClaudeOAuthQuotaLoading)
	report := flow.module.Refresh(a.ctx, gemini.RefreshTarget{Kind: gemini.RefreshAccount, ID: account.ID})
	for _, outcome := range report.Outcomes {
		if outcome.Err != nil {
			return xClaudeOAuthQuotaFailed, outcome.Err
		}
	}
	return xClaudeOAuthAdded, nil
}

func (u *settingsUI) startGeminiLogin(f *accountForm, rt *liveApp) {
	if f.busy || !rt.geminiReady() {
		return
	}
	network, err := netx.Current()
	if err != nil {
		u.accountNotice(f, accountFailure(u.L(), tx(u.L(), xOAuthExchange), err))
		return
	}
	flow := geminiLoginFlow{module: rt.geminiMod, openBrowser: openAccountBrowser, callbackText: geminiCallbackText(u.L())}
	u.startGeminiLoginFlow(f, rt, flow, netx.WithContext(context.Background(), network))
}

func (u *settingsUI) startGeminiLoginFlow(f *accountForm, rt *liveApp, flow geminiLoginFlow, ctx context.Context) {
	if f.busy {
		return
	}
	a, err := flow.prepare(ctx)
	if err != nil {
		u.accountNotice(f, geminiLoginFailure(u.L(), err))
		return
	}
	f.geminiLogin, f.busy = a, true
	a.opening = true
	openBrowser := a.browser
	a.browser = func(ctx context.Context, raw string) error {
		err := openBrowser(ctx, raw)
		u.post(func() {
			if f.geminiLogin == a {
				a.opening = false
			}
		})
		return err
	}
	f.status = u.L().Text(l10n.KeyAccountsOAuthWaiting)
	f.cancel = func() { u.stopGeminiLogin(f, a) }
	stage := func(key textKey) {
		u.post(func() {
			if f.geminiLogin == a {
				a.waiting = false
				a.input, a.problem = "", ""
				f.status = tx(u.L(), key)
			}
		})
	}
	go func() {
		outcome, err := flow.run(a, stage)
		// Reload even if the window was closed after the durable account save.
		if outcome != "" && rt != nil && rt.geminiB != nil {
			rt.geminiB.reload(true)
		}
		u.post(func() { u.finishGeminiLogin(f, a, outcome, err) })
	}()
}

func (u *settingsUI) finishGeminiLogin(f *accountForm, a *geminiLoginAttempt, outcome textKey, err error) {
	if f.geminiLogin != a {
		return
	}
	u.stopGeminiLogin(f, a)
	if outcome != "" {
		u.accountNotice(f, accountFailure(u.L(), tx(u.L(), outcome), err))
		return
	}
	if err != nil {
		u.accountNotice(f, geminiLoginFailure(u.L(), err))
	}
}

func (u *settingsUI) stopGeminiLogin(f *accountForm, a *geminiLoginAttempt) {
	a.cancel()
	a.server.Cancel()
	if f.geminiLogin != a {
		return
	}
	a.input, a.problem = "", ""
	f.geminiLogin, f.cancel, f.busy, f.status = nil, nil, false, ""
}

func geminiLoginFailure(l l10n.Localizer, err error) string {
	var stage *claudeLoginError
	switch {
	case errors.Is(err, context.Canceled):
		return ""
	case errors.Is(err, gemini.ErrLoginTimedOut), errors.Is(err, context.DeadlineExceeded):
		return accountFailure(l, tx(l, xOAuthTimeout), err)
	case errors.As(err, &stage):
		return accountFailure(l, tx(l, stage.reason), stage.cause)
	default:
		return accountFailure(l, l.Text(l10n.KeyAccountsOAuthFailed), err)
	}
}

func geminiCallbackText(l l10n.Localizer) gemini.LoginCallbackText {
	return gemini.LoginCallbackText{Received: tx(l, xClaudeCallbackReceived), Invalid: tx(l, xClaudeCallbackInvalid), AlreadyReceived: tx(l, xClaudeCallbackRepeated)}
}

// renderGeminiLogin is the sign-in sheet: progress, the callback field
// while the browser is out, reopen and cancel.
func renderGeminiLogin(c *ui.Context, u *settingsUI, f *accountForm) {
	a := f.geminiLogin
	if a == nil {
		return
	}
	l, open := u.L(), true
	ui.Modal(c.Key("gemini-browser-login"), &open, func() {
		ui.Text(c, l.Text(l10n.KeyAccountsAddOAuth)).FontSize(17).FontWeight(600)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, f.status).FontSize(12)
		})
		ui.Text(c, l.Text(l10n.KeyGeminiOAuthHint)).FontSize(12).TextColor(c.Theme().TextMuted)
		if a.waiting {
			ui.Text(c, l.Text(l10n.KeyGeminiOAuthManual)).FontSize(12)
			ui.TextInput(c.Key("gemini-login-callback"), &a.input).Label(l.Text(l10n.KeyGeminiOAuthManual))
			if a.problem != "" {
				ui.Text(c, a.problem).FontSize(12).TextColor(c.Theme().Danger)
			}
		}
		ui.Row(c).Gap(8).Children(func() {
			if PillButton(c, nil, l.Text(l10n.KeyCancel), false).Clicked() {
				u.stopGeminiLogin(f, a)
			}
			if a.waiting {
				if PillButton(c, nil, tx(l, xClaudeOAuthReopen), a.opening).Clicked() {
					u.reopenGeminiLogin(f, a)
				}
				if PillButton(c, nil, tx(l, xClaudeOAuthSubmit), strings.TrimSpace(a.input) == "").Clicked() {
					u.submitGeminiLogin(a)
				}
			}
		})
	}).Width(560)
	if !open {
		u.stopGeminiLogin(f, a)
	}
}

func (u *settingsUI) submitGeminiLogin(a *geminiLoginAttempt) {
	raw := a.input
	a.input, a.problem = "", ""
	if err := a.server.Submit(raw); err != nil {
		a.problem = geminiLoginFailure(u.L(), err)
		if errors.Is(err, gemini.ErrLoginCallback) {
			a.problem = u.L().Text(l10n.KeyGeminiOAuthManualInvalid)
		}
	}
}

func (u *settingsUI) reopenGeminiLogin(f *accountForm, a *geminiLoginAttempt) {
	if f.geminiLogin != a || !a.waiting || a.opening {
		return
	}
	a.opening, a.problem = true, ""
	go func() {
		err := a.browser(a.ctx, a.server.Session().AuthURL)
		u.post(func() {
			if f.geminiLogin != a || !a.waiting {
				return
			}
			a.opening = false
			if err != nil {
				a.problem = accountFailure(u.L(), tx(u.L(), xOAuthBrowser), err)
			}
		})
	}()
}
