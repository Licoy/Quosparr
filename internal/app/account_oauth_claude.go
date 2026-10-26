package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
)

// One attempt owns its listener and cancellation; UI deliveries compare the
// attempt pointer so a closed or retried login cannot receive stale results.
type claudeLoginAttempt struct {
	ctx              context.Context
	cancel           context.CancelFunc
	session          claude.LoginSession
	server           *claude.LoginCallbackServer
	browser          func(context.Context, string) error
	input, problem   string
	waiting, opening bool
}

type claudeLoginFlow struct {
	callbackText claude.LoginCallbackText
	module       *claude.Module
	client       claude.OAuthClient
	openBrowser  func(context.Context, string) error
	port         uint16
	lifetime     time.Duration
}

type claudeLoginError struct {
	reason textKey
	cause  error
}

func (e *claudeLoginError) Error() string { return string(e.reason) }
func (e *claudeLoginError) Unwrap() error { return e.cause }

func (flow claudeLoginFlow) prepare(ctx context.Context) (*claudeLoginAttempt, error) {
	session, err := claude.StartLoginSession(flow.port, time.Now(), flow.lifetime)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, session.ExpiresAt)
	server := claude.NewLoginCallbackServer(session, flow.callbackText)
	if err := server.Start(); err != nil {
		cancel()
		return nil, err
	}
	return &claudeLoginAttempt{ctx: ctx, cancel: cancel, session: session, server: server, browser: flow.openBrowser, waiting: true}, nil
}

func (flow claudeLoginFlow) run(a *claudeLoginAttempt, stage func(textKey)) (textKey, error) {
	defer a.cancel()
	defer a.server.Cancel()
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	if err := a.browser(a.ctx, a.session.AuthURL); err != nil {
		return "", &claudeLoginError{xOAuthBrowser, err}
	}
	code, err := a.server.Wait(a.ctx)
	if err != nil {
		return "", err
	}
	stage(xClaudeOAuthVerify)
	result, err := flow.client.Exchange(a.ctx, code, a.session, time.Now())
	if err != nil {
		return "", err
	}
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	stage(xClaudeOAuthSaving)
	account, err := flow.module.ImportBrowserLogin(a.ctx, result)
	if errors.Is(err, claude.ErrLoginAlreadyOfficial) {
		return xClaudeOAuthExists, nil
	}
	if err != nil {
		return "", &claudeLoginError{xOAuthStore, err}
	}
	stage(xClaudeOAuthQuotaLoading)
	if err := flow.module.Refresh(a.ctx, account.ID, true); err != nil {
		return xClaudeOAuthQuotaFailed, nil
	}
	return xClaudeOAuthAdded, nil
}

func (u *settingsUI) startClaudeLogin(f *accountForm, rt *liveApp) {
	if f.busy {
		return
	}
	network, err := netx.Current()
	if err != nil {
		u.accountNotice(f, tx(u.L(), xOAuthExchange))
		return
	}
	u.startClaudeLoginFlow(f, rt, claudeLoginFlow{module: rt.claudeMod, openBrowser: openAccountBrowser, callbackText: claudeCallbackText(u.L())}, netx.WithContext(context.Background(), network))
}

func (u *settingsUI) startClaudeLoginFlow(f *accountForm, rt *liveApp, flow claudeLoginFlow, ctx context.Context) {
	if f.busy {
		return
	}
	a, err := flow.prepare(ctx)
	if err != nil {
		u.accountNotice(f, claudeLoginFailure(u.L(), err))
		return
	}
	f.claudeLogin, f.busy = a, true
	a.opening = true
	openBrowser := a.browser
	a.browser = func(ctx context.Context, raw string) error {
		err := openBrowser(ctx, raw)
		u.post(func() {
			if f.claudeLogin == a {
				a.opening = false
			}
		})
		return err
	}
	f.status = u.L().Text(l10n.KeyAccountsOAuthWaiting)
	f.cancel = func() { u.stopClaudeLogin(f, a) }
	stage := func(key textKey) {
		u.post(func() {
			if f.claudeLogin == a {
				a.waiting = false
				a.input, a.problem = "", ""
				f.status = tx(u.L(), key)
			}
		})
	}
	go func() {
		outcome, err := flow.run(a, stage)
		// Reload even if the window was closed after the durable account save.
		if outcome != "" && rt != nil && rt.claudeB != nil {
			rt.claudeB.reload(true)
		}
		u.post(func() { u.finishClaudeLogin(f, a, outcome, err) })
	}()
}

func (u *settingsUI) finishClaudeLogin(f *accountForm, a *claudeLoginAttempt, outcome textKey, err error) {
	if f.claudeLogin != a {
		return
	}
	u.stopClaudeLogin(f, a)
	if err != nil {
		u.accountNotice(f, claudeLoginFailure(u.L(), err))
		return
	}
	u.accountNotice(f, tx(u.L(), outcome))
}

func (u *settingsUI) stopClaudeLogin(f *accountForm, a *claudeLoginAttempt) {
	a.cancel()
	a.server.Cancel()
	if f.claudeLogin != a {
		return
	}
	a.input, a.problem = "", ""
	f.claudeLogin, f.cancel, f.busy, f.status = nil, nil, false, ""
}

func claudeLoginFailure(l l10n.Localizer, err error) string {
	var stage *claudeLoginError
	switch {
	case errors.Is(err, claude.ErrLoginRollback):
		return tx(l, xClaudeOAuthRollback)
	case errors.Is(err, context.Canceled):
		return ""
	case errors.Is(err, claude.ErrLoginPortInUse):
		return tx(l, xClaudeOAuthPort)
	case errors.Is(err, claude.ErrLoginTimedOut), errors.Is(err, context.DeadlineExceeded):
		return tx(l, xOAuthTimeout)
	case errors.Is(err, claude.ErrLoginDenied):
		return tx(l, xClaudeOAuthDenied)
	case errors.Is(err, claude.ErrLoginCallback):
		return tx(l, xOAuthCallback)
	case errors.Is(err, claude.ErrLoginExchange):
		return tx(l, xOAuthExchange)
	case errors.Is(err, claude.ErrCredentialChanged):
		return tx(l, xClaudeOAuthChanged)
	case errors.Is(err, claude.ErrLoginIdentity), errors.Is(err, claude.ErrIdentityRequired):
		return tx(l, xClaudeOAuthIdentity)
	case errors.As(err, &stage):
		return tx(l, stage.reason)
	default:
		return l.Text(l10n.KeyAccountsOAuthFailed)
	}
}
