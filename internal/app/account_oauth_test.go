package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/platform"
)

func oauthTestPort(t *testing.T) uint16 {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return uint16(listener.Addr().(*net.TCPAddr).Port)
}

func TestAccountWindowCloseCancelsLoginAndClearsBusy(t *testing.T) {
	e := newEnv(t, nil)
	_ = e.s.Load()
	u := newSettingsUI(e.s, SettingsOptions{})
	f := accountFormOf(u)
	started := make(chan struct{})
	flow := codexLoginFlow{port: oauthTestPort(t), openBrowser: func(context.Context, string) error { close(started); return nil }}
	u.startCodexLoginFlow(f, nil, flow)
	if !f.busy || f.cancel == nil {
		t.Fatal("login did not publish cancellable state")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("login did not start")
	}
	u.cancelAccountTask()
	deadline := time.Now().Add(time.Second)
	for f.busy && time.Now().Before(deadline) {
		e.q.drain()
		time.Sleep(time.Millisecond)
	}
	if f.busy || f.cancel != nil || f.status != "" || u.noticeOpen {
		t.Fatal("window close left account task running")
	}
}

func TestCodexLoginFailureMessagesDoNotExposeCauses(t *testing.T) {
	for _, language := range l10n.Languages() {
		l := l10n.New(language)
		for _, reason := range []textKey{xOAuthBrowser, xOAuthCallback, xOAuthExchange, xOAuthStore} {
			message := codexLoginFailure(l, &codexLoginError{reason: reason, cause: errors.New("secret-test-value")})
			if message == "" || message == string(reason) || message == "secret-test-value" {
				t.Fatal("missing or unsafe localized error")
			}
		}
	}
}

func TestCodexOAuthImportsWithoutSwitchingOfficial(t *testing.T) {
	dir := t.TempDir()
	official := filepath.Join(dir, "codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(official), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"OPENAI_API_KEY":"temporary-official-test-key"}`)
	if err := os.WriteFile(official, original, 0600); err != nil {
		t.Fatal(err)
	}
	tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code_verifier") == "" {
			t.Error("invalid exchange form")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"temporary-access-token","refresh_token":"temporary-refresh-token"}`))
	}))
	defer tokenServer.Close()
	store := codex.NewStore(filepath.Dir(official), filepath.Join(dir, "runway"))
	flow := codexLoginFlow{store: store, port: oauthTestPort(t), login: codexapi.OAuthLogin{HTTP: tokenServer.Client(), TokenURL: tokenServer.URL}}
	flow.openBrowser = func(_ context.Context, raw string) error {
		auth, err := url.Parse(raw)
		if err != nil {
			return err
		}
		callback := auth.Query().Get("redirect_uri") + "?code=test-code&state=" + url.QueryEscape(auth.Query().Get("state"))
		response, err := http.Get(callback)
		if err == nil {
			response.Body.Close()
		}
		return err
	}
	if _, err := flow.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(official)
	if err != nil || string(actual) != string(original) {
		t.Fatal("OAuth overwrote official credential")
	}
	index, err := store.LoadIndex()
	if err != nil || len(index.Accounts) != 1 {
		t.Fatal("managed account missing")
	}
	if err := platform.VerifyOwnerOnly(filepath.Join(store.Root(), index.Accounts[0].ID, "auth.json"), 0600); err != nil {
		t.Fatalf("managed credential permissions: %v", err)
	}
}

func TestCodexOAuthFailuresEndWithoutImport(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout", "state", "browser", "port"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			store := codex.NewStore(filepath.Join(dir, "codex"), filepath.Join(dir, "runway"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			flow := codexLoginFlow{store: store, port: oauthTestPort(t), lifetime: 40 * time.Millisecond}
			flow.openBrowser = func(_ context.Context, raw string) error {
				switch mode {
				case "cancel":
					cancel()
				case "browser":
					return errors.New("fake browser failure")
				case "state":
					auth, _ := url.Parse(raw)
					response, err := http.Get(auth.Query().Get("redirect_uri") + "?code=fake&state=wrong")
					if err == nil {
						response.Body.Close()
					}
					return err
				}
				return nil
			}
			if mode == "port" {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				flow.port = uint16(listener.Addr().(*net.TCPAddr).Port)
				flow.openBrowser = func(context.Context, string) error { t.Fatal("browser opened before binding callback"); return nil }
			}
			if _, err := flow.run(ctx); err == nil {
				t.Fatal("failure was hidden")
			}
			index, err := store.LoadIndex()
			if err != nil || len(index.Accounts) != 0 {
				t.Fatal("failed OAuth imported account")
			}
		})
	}
}
