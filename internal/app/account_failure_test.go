package app

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/claudedesktop"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/store"
)

func TestAccountFailureSafeDetailsInEveryLanguage(t *testing.T) {
	const secret = "secret-test-value"
	cases := []struct {
		err  error
		code string
	}{
		{&os.PathError{Op: secret, Path: "/" + secret, Err: os.ErrPermission}, "permission"},
		{fmt.Errorf("%s: %w", secret, syscall.EROFS), "read_only"},
		{errors.Join(errors.New(secret), syscall.ENOSPC), "disk_full"},
		{&os.PathError{Path: secret, Err: os.ErrNotExist}, "not_found"},
		{&json.UnmarshalTypeError{Value: secret, Field: secret}, "invalid_data"},
		{&net.DNSError{Name: secret, Server: secret, Err: secret}, "dns"},
		{&url.Error{Op: secret, URL: "https://user:" + secret + "@example.test?key=" + secret, Err: context.DeadlineExceeded}, "timeout"},
		{x509.HostnameError{Host: secret}, "TLS"},
		{fmt.Errorf("%s: %w", secret, syscall.ECONNREFUSED), "connect"},
		{errors.New(secret), "unknown"},
	}
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		for _, tc := range cases {
			got := accountFailure(l, tx(l, xAccountSaveFailed), tc.err)
			if !strings.Contains(got, accountReasonText(l, tc.code)) || strings.Contains(got, secret) || strings.Contains(got, "accountFailure_") {
				t.Fatal("missing safe failure classification")
			}
		}
		for _, code := range []string{"read_keychain", "write_index", "read_usage", "refresh", "rollback_failed", "command", "not_private", "unsupported_version"} {
			if text := accountReasonText(l, code); text == "" || text == accountReasonText(l, "unknown") {
				t.Fatalf("missing classification translation: %s", code)
			}
		}
		if accountErrorReason(l, nil) != "" {
			t.Fatal("nil has an error reason")
		}
	}
}

func TestAccountFailureOperationStatusesAndUnknownCodes(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, tc := range []struct {
		err      error
		contains string
	}{
		{&claude.OperationError{Stage: claude.StageReadKeychain, Kind: claude.KindCommand, ExitCode: 36}, "Read Keychain: System command failed (exit 36)"},
		{&claude.OperationError{Stage: claude.StageOAuthExchange, Kind: claude.KindHTTP, HTTPStatus: 429}, "HTTP 429"},
		{&claude.OperationError{Stage: claude.StageReadUsage, Kind: claude.ErrorKind("connection_refused")}, "Connection could not be established"},
		{&codexapi.HTTPError{StatusCode: 503}, "HTTP 503"},
		{&apiquota.HTTPStatusError{Code: 502}, "HTTP 502"},
		{&claude.OperationError{Stage: "secret-test-value", Kind: "secret-test-value"}, "[unknown]"},
	} {
		got := accountErrorReason(l, tc.err)
		if !strings.Contains(got, tc.contains) || strings.Contains(got, "secret-test-value") {
			t.Fatal("status or diagnostic code was lost or unsafe")
		}
	}
}

func TestSavedClaudeLoginQuotaFailureKeepsSavedOutcome(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	a := &claudeLoginAttempt{cancel: func() {}, server: claude.NewLoginCallbackServer(claude.LoginSession{}, claude.LoginCallbackText{})}
	f.claudeLogin, f.busy = a, true
	err := &claude.OperationError{Stage: claude.StageReadUsage, Kind: claude.KindHTTP, HTTPStatus: 502}
	u.finishClaudeLogin(f, a, xClaudeOAuthQuotaFailed, err)
	if f.busy || !u.noticeOpen || !strings.Contains(u.notice, tx(u.L(), xClaudeOAuthQuotaFailed)) || !strings.Contains(u.notice, "502") || strings.Contains(u.notice, tx(u.L(), xOAuthStore)) {
		t.Fatal("saved account was reported as a login failure or lost quota reason")
	}
}

func TestDesktopPartialFailurePreservesResultsAndReason(t *testing.T) {
	l := l10n.New(l10n.English)
	got := desktopResultText(l, claudedesktop.SwitchResult{CLISwitched: true, PreviousDesktopRestored: true, ManualReopenRequired: true}, errors.Join(claudedesktop.ErrPersistence, &os.PathError{Path: "secret-test-value", Err: os.ErrPermission}))
	for _, want := range []string{tx(l, xDesktopPersistence), tx(l, xDesktopCLIChanged), tx(l, xDesktopRestored), tx(l, xDesktopRecovery), accountReasonText(l, "permission")} {
		if !strings.Contains(got, want) {
			t.Fatal("Desktop partial result or cause lost")
		}
	}
	if strings.Contains(got, "secret-test-value") {
		t.Fatal("Desktop failure leaked a path")
	}
}

func TestProxyAndLoginFailuresKeepSafeCauses(t *testing.T) {
	l := l10n.New(l10n.English)
	cause := &os.PathError{Path: "secret-test-value", Err: os.ErrPermission}
	login := loginFailure(ErrLoginRegister, cause)
	if !errors.Is(login, ErrLoginRegister) || !errors.Is(login, os.ErrPermission) || strings.Contains(login.Error(), "secret-test-value") || !strings.Contains(accountErrorReason(l, login), accountReasonText(l, "permission")) {
		t.Fatal("login cause was lost or exposed")
	}
	p := newPlatformCredentials(&netx.MemoryCredentialStore{Err: cause})
	_, err := p.Load("test")
	if !errors.Is(err, ErrCredentialStoreFailed) || !errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "secret-test-value") || !strings.Contains(proxyFailureText(l, err), accountReasonText(l, "permission")) {
		t.Fatal("credential store cause was lost or exposed")
	}
	if got := proxyFailureText(l, store.ErrProxyCredentialsUnavailable); got != l.Text(l10n.KeyProxyCredentialsUnavailable) {
		t.Fatal("specific missing-credential guidance was obscured")
	}
}

func TestSettingsProxyNetworkFailureShowsDNSReason(t *testing.T) {
	e := newEnv(t, nil)
	e.s.SetProxyHooks(ProxyHooks{Test: func(context.Context, store.NetworkProxyConfiguration, *ProxyCredentials, string) error {
		return &net.DNSError{Name: "secret-test-value", Err: "secret-test-value"}
	}})
	u := newSettingsUI(e.s, SettingsOptions{})
	u.testURL = store.DefaultProxyTestURL
	u.draft = ProxyDraft{Mode: store.ProxyModeHTTP, Host: "localhost", Port: "8080"}
	u.testProxy()
	awaitSettingsTask(t, e, &u.testTask)
	if !u.proxyMsgErr || !strings.Contains(u.proxyMsg, accountReasonText(u.L(), "dns")) || strings.Contains(u.proxyMsg, "secret-test-value") {
		t.Fatal("proxy test lost DNS reason or exposed transport details")
	}
}

func TestClaudeProfileHTTPFailureHasIdentityStageAndHTTPReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			fmt.Fprint(w, `{"access_token":"secret-test-value","refresh_token":"secret-test-value","expires_in":3600}`)
			return
		}
		http.Error(w, "secret-test-value", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	now := time.Now()
	session, err := claude.StartLoginSession(oauthTestPort(t), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	client := claude.OAuthClient{HTTP: server.Client(), TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile"}
	_, err = client.Exchange(context.Background(), "test-code", session, now)
	if err == nil {
		t.Fatal("profile HTTP error unexpectedly succeeded")
	}
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		got := claudeLoginFailure(l, err)
		if !strings.Contains(got, accountReasonText(l, "oauth_identity")) || !strings.Contains(got, "HTTP 503") || !strings.Contains(got, accountReasonText(l, "http")) || strings.Contains(got, accountReasonText(l, "invalid_data")) || strings.Contains(got, "secret-test-value") {
			t.Fatal("profile HTTP failure lost its stage or safe reason")
		}
	}
}

func TestAccountFailureRollbackSummaryKeepsBothStages(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		err := &claude.OperationError{Stage: claude.StageRollbackCredential, Kind: claude.KindRollbackFailed,
			Failure:  &claude.OperationError{Stage: claude.StageWriteIndex, Kind: claude.KindDiskFull},
			Rollback: &claude.OperationError{Stage: claude.StageRollbackCredential, Kind: claude.KindPermission},
		}
		got := accountErrorReason(l, err)
		for _, code := range []string{"rollback_credential", "rollback_failed", "original_failure", "write_index", "disk_full", "recovery_failure", "permission"} {
			if !strings.Contains(got, accountReasonText(l, code)) {
				t.Fatal("rollback summary lost a stage or cause")
			}
		}
	}
}

func TestGrokLoginAndTokenReasonsIgnoreMessages(t *testing.T) {
	l := l10n.New(l10n.English)
	for _, kind := range []grok.OAuthErrorKind{grok.OAuthDiscoveryFailed, grok.OAuthInvalidEndpoint, grok.OAuthDeviceCodeFailed, grok.OAuthAuthorizationPending, grok.OAuthSlowDown, grok.OAuthExpired, grok.OAuthDenied, grok.OAuthTokenFailed, grok.OAuthMissingIdentity, grok.OAuthBrowserOpenFailed} {
		got := accountErrorReason(l, &grok.LoginFailedError{Message: "secret-test-value", Cause: &grok.OAuthError{Kind: kind, Message: "secret-test-value"}})
		if got == accountReasonText(l, "unknown") || strings.Contains(got, "secret-test-value") {
			t.Fatal("Grok error lost its category or leaked details")
		}
	}
	got := accountErrorReason(l, &grok.TokenRequestError{Message: "secret-test-value", StatusCode: 503})
	if !strings.Contains(got, "HTTP 503") || strings.Contains(got, "secret-test-value") {
		t.Fatal("token HTTP status missing or unsafe")
	}
}

func TestKnownAccountFailuresRemainActionableAndPrivate(t *testing.T) {
	cases := []error{
		grok.ErrCLINotInstalled, &grok.CLITooOldError{Version: "secret-test-value"}, grok.ErrAuthFileLocked,
		grok.ErrAPIKeyOnlyNotManageable, grok.ErrNoManagedCredential, &grok.CredentialIdentityMismatchError{ID: "secret-test-value"},
		&grok.ProcessesRunningError{PIDs: []int{12345}}, &grok.CurrentAccountCannotBeRemovedError{ID: "secret-test-value"},
		&claude.CurrentAccountCannotBeRemovedError{ID: "secret-test-value"},
		&codex.ProviderMismatchError{Expected: codex.Provider("secret-test-value"), Actual: codex.Provider("secret-test-value")},
		codex.ErrEmptySelection, codex.ErrCredentialStoreUnsupported,
		&codexapi.OAuthError{Description: "access_denied"}, &codexapi.OAuthError{Description: "OAuth state mismatch"}, &codexapi.OAuthError{Description: "OAuth code missing"},
	}
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		for _, err := range cases {
			got := accountErrorReason(l, fmt.Errorf("secret-test-value: %w", err))
			if got == "" || strings.Contains(got, "[unknown]") || strings.Contains(got, "secret-test-value") || strings.Contains(got, "12345") {
				t.Fatal("known provider error was not classified safely")
			}
		}
		err := &netx.Error{Kind: netx.ErrConnectionFailed, Cause: &net.DNSError{Name: "secret-test-value", Err: "secret-test-value"}}
		got := apiErrorText(l, err)
		if !strings.Contains(got, accountReasonText(l, "dns")) || strings.Contains(got, "secret-test-value") {
			t.Fatal("API proxy classification lost the underlying DNS cause")
		}
	}
}

func TestOAuthCallbackListenFailureDoesNotGuessPortConflict(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		for _, tc := range []struct {
			cause error
			code  string
		}{{syscall.EACCES, "permission"}, {syscall.EADDRINUSE, "address_in_use"}} {
			cause := &net.OpError{Op: "listen", Net: "tcp", Err: &os.SyscallError{Syscall: "secret-test-value", Err: tc.cause}}
			codexErr := errors.Join(codexapi.ErrPortInUse, cause)
			claudeErr := errors.Join(claude.ErrLoginPortInUse, cause)
			for _, got := range []string{codexLoginFailure(l, codexErr), claudeLoginFailure(l, claudeErr)} {
				if !strings.Contains(got, accountReasonText(l, tc.code)) || strings.Contains(got, "secret-test-value") {
					t.Fatal("callback listener cause was lost or leaked")
				}
				if tc.code == "permission" && strings.Contains(got, accountReasonText(l, "address_in_use")) {
					t.Fatal("permission failure was reported as port conflict")
				}
			}
		}
		if got := codexLoginFailure(l, codexapi.ErrPortInUse); got != tx(l, xOAuthPort) {
			t.Fatal("bare known port failure acquired an unknown detail")
		}
		if got := claudeLoginFailure(l, claude.ErrLoginPortInUse); got != tx(l, xClaudeOAuthPort) {
			t.Fatal("bare known callback failure acquired an unknown detail")
		}
		err := &claude.OperationError{Stage: claude.ErrorStage("oauth_callback"), Kind: claude.KindPermission}
		got := claudeLoginFailure(l, err)
		if !strings.Contains(got, accountReasonText(l, "oauth_callback")) || !strings.Contains(got, accountReasonText(l, "permission")) {
			t.Fatal("structured callback failure lost stage or reason")
		}
	}
}
