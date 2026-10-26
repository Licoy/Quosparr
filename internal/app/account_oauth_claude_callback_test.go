package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/claude"
)

func TestClaudeBrowserLoginManualCallback(t *testing.T) {
	for _, fullURL := range []bool{false, true} {
		e := newEnv(t, nil)
		_ = e.s.Load()
		u := newSettingsUI(e.s, SettingsOptions{})
		f := accountFormOf(u)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "private-token-response", http.StatusBadGateway)
		}))
		flow := claudeLoginFlow{port: oauthTestPort(t), openBrowser: func(context.Context, string) error { return nil }, client: claude.OAuthClient{HTTP: server.Client(), TokenURL: server.URL}}
		u.startClaudeLoginFlow(f, nil, flow, context.Background())
		a := f.claudeLogin
		waitClaudeUI(t, e, func() bool { return !a.opening })
		a.input = "private-code#" + a.session.State
		if fullURL {
			a.input = a.session.RedirectURI + "?code=private-code&state=" + url.QueryEscape(a.session.State)
		}
		u.submitClaudeLogin(a)
		if a.problem != "" || a.input != "" {
			t.Fatal("valid callback was rejected or retained")
		}
		waitClaudeUI(t, e, func() bool { return !f.busy })
		if !u.noticeOpen || !strings.HasPrefix(u.notice, tx(u.L(), xOAuthExchange)+"\n") {
			t.Fatal("manual callback did not reach exchange")
		}
		server.Close()
	}
}

func TestClaudeBrowserLoginCancellationTimeoutAndBrowserFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout", "browser"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			flow := claudeLoginFlow{port: oauthTestPort(t), lifetime: 40 * time.Millisecond}
			flow.openBrowser = func(context.Context, string) error {
				if mode == "cancel" {
					cancel()
				}
				if mode == "browser" {
					return errors.New("private-browser-error")
				}
				return nil
			}
			a, err := flow.prepare(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = flow.run(a, func(textKey) { t.Fatal("failed attempt advanced beyond callback") })
			if err == nil {
				t.Fatal("failed login reported success")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancel identity was lost")
			}
		})
	}
}
