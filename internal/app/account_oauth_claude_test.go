package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

func waitClaudeUI(t *testing.T, e *env, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.q.drain()
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Claude login UI did not settle")
}

func TestClaudeBrowserLoginMenuAndWaitingControls(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	f.kind = accountKindClaude
	rt := &liveApp{svc: runway.New(runway.Options{})}
	tt := ui.NewTester(func(c *ui.Context) {
		applyTheme(c)
		ui.Column(c).Fill().Padding(20).Children(func() { renderClaudeAccounts(c, u, f, rt) })
	}, 680, 480)
	if err := tt.Click(u.L().Text(l10n.KeyAccountsAdd)); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, label := range tt.Menu() {
		if label == u.L().Text(l10n.KeyAccountsAddOAuth) {
			found = true
		}
		if label == u.L().Text(l10n.KeyClaudeLoginCLI) {
			t.Fatal("old CLI login menu remains")
		}
	}
	if !found {
		t.Fatal("browser login menu missing")
	}
	tt.CloseMenu()
	var opened atomic.Int32
	flow := claudeLoginFlow{port: oauthTestPort(t), openBrowser: func(context.Context, string) error { opened.Add(1); return nil }}
	u.startClaudeLoginFlow(f, nil, flow, context.Background())
	a := f.claudeLogin
	defer u.cancelAccountTask()
	waitClaudeUI(t, e, func() bool { return opened.Load() == 1 && !a.opening })
	tt.Frame()
	if err := tt.Click(tx(u.L(), xClaudeOAuthReopen)); err != nil {
		t.Fatal(err)
	}
	waitClaudeUI(t, e, func() bool { return opened.Load() == 2 && !a.opening })
	a.input = "private-invalid-callback"
	tt.Frame()
	if err := tt.Click(tx(u.L(), xClaudeOAuthSubmit)); err != nil {
		t.Fatal(err)
	}
	if a.input != "" || a.problem != tx(u.L(), xClaudeOAuthManualInvalid) || !a.waiting {
		t.Fatal("invalid manual callback did not remain recoverable")
	}
	if os.Getenv("RUNWAY_UI_QA") != "" {
		dir := filepath.Join("..", "..", "build")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		exportReviewPNG(t, tt, filepath.Join(dir, "claude-browser-login-waiting.png"))
	}
	if err := tt.Click(u.L().Text(l10n.KeyCancel)); err != nil {
		t.Fatal(err)
	}
	if f.busy || f.cancel != nil || f.claudeLogin != nil || a.input != "" {
		t.Fatal("cancel retained login state")
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", flow.port))
	if err != nil {
		t.Fatal("callback port retained after cancel")
	}
	listener.Close()
}

func TestClaudeBrowserLoginWindowCloseAndStaleCompletion(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	started, release := make(chan struct{}), make(chan struct{})
	flow := claudeLoginFlow{port: oauthTestPort(t), openBrowser: func(context.Context, string) error {
		close(started)
		<-release
		return errors.New("private-browser-cause")
	}}
	u.startClaudeLoginFlow(f, nil, flow, context.Background())
	<-started
	u.cancelAccountTask()
	if f.busy || f.claudeLogin != nil || f.status != "" {
		t.Fatal("window close did not release login")
	}
	flow.openBrowser = func(context.Context, string) error { return nil }
	u.startClaudeLoginFlow(f, nil, flow, context.Background())
	fresh := f.claudeLogin
	if fresh == nil {
		t.Fatal("retry failed to bind callback port")
	}
	close(release)
	waitClaudeUI(t, e, func() bool { return !fresh.opening })
	// Force an obsolete completion after the new attempt is visible.
	u.finishClaudeLogin(f, &claudeLoginAttempt{}, xClaudeOAuthAdded, nil)
	waitClaudeUI(t, e, func() bool {
		e.q.mu.Lock()
		defer e.q.mu.Unlock()
		return len(e.q.fns) == 0
	})
	if f.claudeLogin != fresh || !f.busy || u.noticeOpen || f.status != u.L().Text(l10n.KeyAccountsOAuthWaiting) {
		t.Fatal("old attempt overwrote retry")
	}
	u.cancelAccountTask()
	waitClaudeUI(t, e, func() bool { return !f.busy })
}

func TestClaudeBrowserLoginFailureMessagesAreLocalizedAndRedacted(t *testing.T) {
	cases := []struct {
		err error
		key textKey
	}{
		{claude.ErrLoginPortInUse, xClaudeOAuthPort}, {claude.ErrLoginTimedOut, xOAuthTimeout}, {claude.ErrLoginCallback, xOAuthCallback}, {claude.ErrLoginDenied, xClaudeOAuthDenied}, {claude.ErrLoginExchange, xOAuthExchange}, {claude.ErrLoginIdentity, xClaudeOAuthIdentity},
		{&claudeLoginError{xOAuthStore, claude.ErrLoginRollback}, xClaudeOAuthRollback},
		{&claudeLoginError{xOAuthStore, errors.Join(claude.ErrLoginRollback, context.Canceled)}, xClaudeOAuthRollback},
		{&claudeLoginError{xOAuthStore, claude.ErrIdentityRequired}, xClaudeOAuthIdentity}, {&claudeLoginError{xOAuthStore, claude.ErrCredentialChanged}, xClaudeOAuthChanged},
		{&claudeLoginError{xOAuthBrowser, errors.New("private-cause")}, xOAuthBrowser}, {&claudeLoginError{xOAuthStore, errors.New("private-cause")}, xOAuthStore},
	}
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		for _, tc := range cases {
			message := claudeLoginFailure(l, fmt.Errorf("private-cause: %w", tc.err))
			if (message != tx(l, tc.key) && !strings.HasPrefix(message, tx(l, tc.key)+"\n")) || message == string(tc.key) || strings.Contains(message, "private-cause") {
				t.Fatal("unsafe or missing Claude login translation")
			}
		}
		if claudeLoginFailure(l, context.Canceled) != "" {
			t.Fatal("cancel reported as login failure")
		}
		for _, key := range []textKey{xClaudeOAuthVerify, xClaudeOAuthSaving, xClaudeOAuthAdded, xClaudeOAuthQuotaFailed, xClaudeOAuthHint, xClaudeOAuthManual, xClaudeOAuthReopen, xClaudeOAuthSubmit, xClaudeOAuthExists, xClaudeOAuthQuotaLoading, xClaudeCallbackReceived, xClaudeCallbackInvalid, xClaudeCallbackRepeated, xClaudeOAuthManualInvalid} {
			if tx(l, key) == "" || tx(l, key) == string(key) {
				t.Fatal("missing login text")
			}
		}
	}
}

func TestClaudeBrowserOAuthImportsAndReportsQuotaSeparately(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		quotaFails bool
		official   string
	}{
		{"added", false, ""}, {"quota-failed", true, ""},
		{"missing-oauth", false, `{"mcpOAuth":{"preserve":true}}`},
		{"null-oauth", false, `{"claudeAiOauth":null,"mcpOAuth":{"preserve":true}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			quotaFails := scenario.quotaFails
			dir := t.TempDir()
			config := filepath.Join(dir, "claude")
			if err := os.MkdirAll(config, 0700); err != nil {
				t.Fatal(err)
			}
			official := filepath.Join(config, ".credentials.json")
			original := []byte(`{"claudeAiOauth":{"accessToken":"old-test-token","refreshToken":"old-refresh","expiresAt":2000000000000},"mcpOAuth":{"preserve":true}}`)
			if scenario.official != "" {
				original = []byte(scenario.official)
			}
			if err := os.WriteFile(official, original, 0600); err != nil {
				t.Fatal(err)
			}
			global := []byte(`{"oauthAccount":{"accountUuid":"old","organizationUuid":"org","emailAddress":"old@example.com"}}`)
			if scenario.official != "" {
				global = []byte(`{"oauthAccount":{"accountUuid":"new","organizationUuid":"org"}}`)
			}
			if err := os.WriteFile(filepath.Join(dir, ".claude.json"), global, 0600); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/token":
					fmt.Fprint(w, `{"access_token":"new-test-token","refresh_token":"new-refresh","expires_in":3600}`)
				case "/profile":
					fmt.Fprint(w, `{"account":{"uuid":"new"},"organization":{"uuid":"org"}}`)
				case "/usage":
					if quotaFails {
						http.Error(w, "private-response-body", http.StatusBadGateway)
					} else {
						fmt.Fprint(w, `{"five_hour":{"utilization":12,"resets_at":null}}`)
					}
				default:
					t.Error("unexpected endpoint")
				}
			}))
			defer server.Close()
			store := claude.NewStoreAt(filepath.Join(dir, "quosparr", "accounts"), config, dir)
			module := claude.NewModule(store)
			module.Usage = &claude.UsageClient{HTTP: server.Client(), Endpoint: server.URL + "/usage"}
			flow := claudeLoginFlow{module: module, port: oauthTestPort(t), client: claude.OAuthClient{HTTP: server.Client(), TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile"}}
			flow.openBrowser = func(_ context.Context, raw string) error {
				auth, _ := url.Parse(raw)
				callback := auth.Query().Get("redirect_uri") + "?code=test-code&state=" + url.QueryEscape(auth.Query().Get("state"))
				resp, err := server.Client().Get(callback)
				if err == nil {
					resp.Body.Close()
				}
				return err
			}
			a, err := flow.prepare(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			stages := []textKey{}
			outcome, err := flow.run(a, func(key textKey) { stages = append(stages, key) })
			want := xClaudeOAuthAdded
			if quotaFails {
				want = xClaudeOAuthQuotaFailed
			}
			if (err != nil) != quotaFails || outcome != want {
				t.Fatalf("flow outcome=%s error=%v", outcome, err)
			}
			if len(stages) != 3 || stages[0] != xClaudeOAuthVerify || stages[1] != xClaudeOAuthSaving || stages[2] != xClaudeOAuthQuotaLoading {
				t.Fatal("missing login stages")
			}
			index, err := store.LoadIndex()
			if err != nil || len(index.Accounts) != 1 || index.CurrentAccountID != "" {
				t.Fatal("browser login switched or lost account")
			}
			actual, err := os.ReadFile(official)
			if err != nil || string(actual) != string(original) {
				t.Fatal("official credential changed")
			}
			actual, err = os.ReadFile(filepath.Join(dir, ".claude.json"))
			if err != nil || string(actual) != string(global) {
				t.Fatal("official identity changed")
			}
			raw, err := os.ReadFile(store.IndexPath())
			if err != nil || strings.Contains(string(raw), "new-test-token") || strings.Contains(string(raw), "private-response-body") {
				t.Fatal("index leaked credential or response")
			}
		})
	}
}
