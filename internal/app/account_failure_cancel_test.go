package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
)

type accountTestTimeout struct{}

func (accountTestTimeout) Error() string   { return "secret-test-value" }
func (accountTestTimeout) Timeout() bool   { return true }
func (accountTestTimeout) Temporary() bool { return true }

func TestProxyWrappedTransportTimeoutKeepsSpecificReason(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		err := &netx.Error{Kind: netx.ErrConnectionFailed, Cause: accountTestTimeout{}}
		for _, got := range []string{accountErrorReason(l, err), apiErrorText(l, err), proxyFailureText(l, err)} {
			if !strings.Contains(got, accountReasonText(l, "timeout")) || strings.Contains(got, "secret-test-value") {
				t.Fatal("wrapped transport timeout became generic or unsafe")
			}
		}
	}
}

func TestOAuthCanceledRollbackRemainsVisibleThroughRealFlows(t *testing.T) {
	failures := []error{claude.ErrLoginRollback, claude.ErrSwitchRollback, codex.ErrRollbackFailed,
		&claude.OperationError{Stage: claude.StageRollbackKeychain, Kind: claude.KindRollbackFailed},
	}
	for _, failure := range failures {
		joined := errors.Join(context.Canceled, failure, errors.New("secret-test-value"))
		for _, provider := range []string{"codex", "claude"} {
			e := newEnv(t, nil)
			_ = e.s.Load()
			u := newSettingsUI(e.s, SettingsOptions{})
			f := accountFormOf(u)
			browser := func(context.Context, string) error { return joined }
			if provider == "codex" {
				u.startCodexLoginFlow(f, nil, codexLoginFlow{port: oauthTestPort(t), openBrowser: browser})
			} else {
				u.startClaudeLoginFlow(f, nil, claudeLoginFlow{port: oauthTestPort(t), openBrowser: browser}, context.Background())
			}
			waitClaudeUI(t, e, func() bool { return !f.busy })
			if !u.noticeOpen || !strings.Contains(u.notice, accountReasonText(u.L(), "rollback_failed")) || strings.Contains(u.notice, "secret-test-value") {
				t.Fatal("canceled rollback was hidden or unsafe")
			}
		}
	}
}

func TestOrdinaryOAuthCancellationKeepsExistingSilentBehavior(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		e := newEnv(t, nil)
		_ = e.s.Load()
		u := newSettingsUI(e.s, SettingsOptions{})
		f := accountFormOf(u)
		browser := func(context.Context, string) error { return context.Canceled }
		if provider == "codex" {
			u.startCodexLoginFlow(f, nil, codexLoginFlow{port: oauthTestPort(t), openBrowser: browser})
		} else {
			u.startClaudeLoginFlow(f, nil, claudeLoginFlow{port: oauthTestPort(t), openBrowser: browser}, context.Background())
		}
		waitClaudeUI(t, e, func() bool { return !f.busy })
		if u.noticeOpen {
			t.Fatal("ordinary cancellation produced a failure notice")
		}
	}
}

func TestCodexBrowserErrorKeepsRollbackWhenContextIsActuallyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flow := codexLoginFlow{port: oauthTestPort(t), openBrowser: func(context.Context, string) error {
		cancel()
		return errors.Join(context.Canceled, codex.ErrRollbackFailed)
	}}
	_, err := flow.run(ctx)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, codex.ErrRollbackFailed) {
		t.Fatal("browser cancellation discarded rollback failure")
	}
}
