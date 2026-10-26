package app

import (
	"context"
	"errors"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
)

// codexLoginFlow owns one attempt; browser and transport are injectable so
// tests only use loopback HTTP and temporary account stores.
type codexLoginFlow struct {
	store       *codex.Store
	openBrowser func(context.Context, string) error
	login       codexapi.OAuthLogin
	port        uint16
	lifetime    time.Duration
}

type codexLoginError struct {
	reason textKey
	cause  error
}

func (e *codexLoginError) Error() string { return string(e.reason) }
func (e *codexLoginError) Unwrap() error { return e.cause }

func (f codexLoginFlow) run(ctx context.Context) (codex.ManagedAccount, error) {
	session, err := codexapi.StartLoginSession(f.port, time.Now(), f.lifetime)
	if err != nil {
		return codex.ManagedAccount{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, session.ExpiresAt)
	defer cancel()
	server := codexapi.NewCallbackServer(session.Port)
	defer server.Cancel()
	if err := server.Start(); err != nil {
		return codex.ManagedAccount{}, err
	}
	if err := ctx.Err(); err != nil {
		return codex.ManagedAccount{}, err
	}
	if err := f.openBrowser(ctx, session.AuthURL); err != nil {
		if ctx.Err() != nil {
			return codex.ManagedAccount{}, ctx.Err()
		}
		return codex.ManagedAccount{}, &codexLoginError{xOAuthBrowser, err}
	}
	callback, err := server.WaitForCallback(ctx, time.Until(session.ExpiresAt))
	if err != nil {
		return codex.ManagedAccount{}, err
	}
	code, err := codexapi.AuthorizationCode(callback.String(), session.State)
	if err != nil {
		return codex.ManagedAccount{}, &codexLoginError{xOAuthCallback, err}
	}
	tokens, err := f.login.ExchangeCode(ctx, code, session, time.Now())
	if err != nil {
		return codex.ManagedAccount{}, &codexLoginError{xOAuthExchange, err}
	}
	if err := ctx.Err(); err != nil {
		return codex.ManagedAccount{}, err
	}
	auth := codex.Auth{AuthMode: tokens.AuthMode, PlanType: tokens.PlanType, LastRefresh: tokens.LastRefresh,
		Tokens: codex.Tokens{IDToken: tokens.Tokens.IDToken, AccessToken: tokens.Tokens.AccessToken,
			RefreshToken: tokens.Tokens.RefreshToken, AccountID: tokens.AccountID}}
	// Importing a login never installs it as the official Codex auth.
	account, err := f.store.Upsert(auth, codex.UpsertOptions{Now: time.Now()})
	if err != nil {
		return codex.ManagedAccount{}, &codexLoginError{xOAuthStore, err}
	}
	return account, nil
}

func (u *settingsUI) startCodexLogin(f *accountForm, rt *liveApp) {
	u.startCodexLoginFlow(f, rt, codexLoginFlow{store: rt.codex, openBrowser: openAccountBrowser})
}

func (u *settingsUI) startCodexLoginFlow(f *accountForm, rt *liveApp, flow codexLoginFlow) {
	if f.busy {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	l := u.L()
	u.accountAct(f, l.Text(l10n.KeyAccountsOAuthWaiting), func() string {
		defer cancel()
		_, err := flow.run(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, codexapi.ErrCancelled) {
			return l.Text(l10n.KeyAccountsOAuthCancelled)
		}
		if err != nil {
			return codexLoginFailure(l, err)
		}
		rt.svc.ReloadAccounts()
		return l.Textf(l10n.KeyAccountsImportSucceeded, 1)
	})
}

func codexLoginFailure(l l10n.Localizer, err error) string {
	var stage *codexLoginError
	switch {
	case errors.Is(err, codexapi.ErrPortInUse):
		return tx(l, xOAuthPort)
	case errors.Is(err, codexapi.ErrTimedOut), errors.Is(err, context.DeadlineExceeded):
		return tx(l, xOAuthTimeout)
	case errors.As(err, &stage):
		return tx(l, stage.reason)
	default:
		return l.Text(l10n.KeyAccountsOAuthFailed)
	}
}

func (u *settingsUI) cancelAccountTask() {
	if u.accounts != nil && u.accounts.desktop != nil {
		u.accounts.desktop.close()
	}
	if u.accounts != nil && u.accounts.cancel != nil {
		u.accounts.cancel()
	}
	if u.skills != nil {
		u.skills.cancelTasks()
	}
}

func (u *settingsUI) startGrokLogin(f *accountForm, rt *liveApp) {
	if f.busy {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	l := u.L()
	u.accountAct(f, l.Text(l10n.KeyAccountsOAuthWaiting), func() string {
		defer cancel()
		return rt.loginGrokContext(l, ctx)
	})
}
