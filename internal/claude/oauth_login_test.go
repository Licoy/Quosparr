package claude

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

func testLoginSession(t *testing.T) LoginSession {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()
	s, err := StartLoginSession(port, time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOAuthLoginSessionPKCE(t *testing.T) {
	s := testLoginSession(t)
	u, err := url.Parse(s.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	hash := sha256.Sum256([]byte(s.CodeVerifier))
	for key, want := range map[string]string{"client_id": OAuthClientID, "response_type": "code", "redirect_uri": s.RedirectURI, "state": s.State, "scope": loginScopes, "code": "true", "code_challenge_method": "S256", "code_challenge": base64.RawURLEncoding.EncodeToString(hash[:])} {
		if q.Get(key) != want {
			t.Errorf("wrong authorization parameter %s", key)
		}
	}
	other := testLoginSession(t)
	if s.State == other.State || s.CodeVerifier == other.CodeVerifier {
		t.Fatal("secrets reused")
	}
}

func TestOAuthLoginExchange(t *testing.T) {
	for _, tc := range []struct {
		name, token, profile string
		want                 error
	}{
		{"success", `{"access_token":"access-secret","refresh_token":"refresh-secret","expires_in":3600,"account":{"uuid":"a"},"organization":{"uuid":"o"},"scope":"user:profile"}`, `{"account":{"uuid":"a"},"organization":{"uuid":"o"}}`, nil},
		{"missing refresh", `{"access_token":"access-secret","expires_in":3600}`, `{}`, ErrLoginExchange},
		{"invalid expiry", `{"access_token":"access-secret","refresh_token":"refresh-secret","expires_in":0}`, `{}`, ErrLoginExchange},
		{"identity conflict", `{"access_token":"access-secret","refresh_token":"refresh-secret","expires_in":3600,"account":{"uuid":"wrong"}}`, `{"account":{"uuid":"a"},"organization":{"uuid":"o"}}`, ErrLoginIdentity},
		{"missing organization", `{"access_token":"access-secret","refresh_token":"refresh-secret","expires_in":3600}`, `{"account":{"uuid":"a"}}`, ErrLoginIdentity},
		{"oversized", strings.Repeat("x", usageMaxBody+1), `{}`, ErrLoginExchange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testLoginSession(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Cookie") != "" {
					t.Error("cookie leaked")
				}
				if r.URL.Path == "/token" {
					checkLoginGrant(t, r, s)
					fmt.Fprint(w, tc.token)
				} else {
					if r.Header.Get("Authorization") != "Bearer access-secret" {
						t.Error("missing bearer")
					}
					fmt.Fprint(w, tc.profile)
				}
			}))
			defer server.Close()
			result, err := (OAuthClient{HTTP: server.Client(), TokenURL: server.URL + "/token", ProfileURL: server.URL + "/profile"}).Exchange(context.Background(), "secret-code", s, time.Now())
			if !errors.Is(err, tc.want) {
				t.Fatalf("wrong result error %v", err)
			}
			if err != nil {
				return
			}
			doc, err := ParseAuthDocument(result.Credential)
			if err != nil || !doc.OAuth().HasExpiry || !doc.OAuth().ExpiresAt.After(time.Now()) || len(doc.OAuth().Scopes) != 1 || doc.OAuth().SubscriptionType != "" {
				t.Fatal("invalid credential")
			}
			if result.Identity.AccountUUID != "a" || result.Identity.OrganizationUUID != "o" {
				t.Fatal("invalid identity")
			}
		})
	}
}

func TestOAuthLoginRedirectAndNetworkFailures(t *testing.T) {
	leaked := false
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked = true }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	s := testLoginSession(t)
	_, err := (OAuthClient{HTTP: redirect.Client(), TokenURL: redirect.URL}).Exchange(context.Background(), "private-code", s, time.Now())
	if !errors.Is(err, ErrLoginExchange) || leaked {
		t.Fatal("redirect was not blocked")
	}
	netx.Block(errors.New("private proxy details"))
	defer netx.Configure(nil)
	_, err = (OAuthClient{}).Exchange(context.Background(), "private-code", s, time.Now())
	if !errors.Is(err, ErrLoginExchange) || strings.Contains(err.Error(), "private") {
		t.Fatal("network error leaked")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (OAuthClient{}).Exchange(ctx, "private-code", s, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}

func TestOAuthLoginBadCallbacksDoNotConsume(t *testing.T) {
	s := testLoginSession(t)
	server := NewLoginCallbackServer(s)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Cancel()
	for _, input := range []string{"bare-code", "code#wrong", s.RedirectURI + "?code=code&state=wrong", s.RedirectURI + "?code=code%23wrong&state=" + s.State, "http://example.com/callback?code=code&state=" + s.State} {
		if !errors.Is(server.Submit(input), ErrLoginCallback) {
			t.Fatal("invalid callback accepted")
		}
	}
	for _, tc := range []struct{ method, path, host string }{{"POST", "/callback", ""}, {"GET", "/wrong", ""}, {"GET", "/callback", "example.com"}, {"GET", "/callback?state=wrong&code=code", ""}} {
		req, _ := http.NewRequest(tc.method, "http://127.0.0.1:"+fmt.Sprint(s.Port)+tc.path, nil)
		if tc.host != "" {
			req.Host = tc.host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatal("invalid callback status")
		}
	}
	if err := server.Submit("accepted#" + s.State); err != nil {
		t.Fatal(err)
	}
	code, err := server.Wait(context.Background())
	if err != nil || code != "accepted" {
		t.Fatal("valid callback consumed")
	}
	if err := server.Submit("again#" + s.State); !errors.Is(err, ErrLoginCallback) {
		t.Fatal("duplicate accepted")
	}
}

func TestOAuthLoginLifecycle(t *testing.T) {
	for _, kind := range []string{"cancel", "timeout", "denied"} {
		t.Run(kind, func(t *testing.T) {
			s := testLoginSession(t)
			if kind == "timeout" {
				s.ExpiresAt = time.Now().Add(20 * time.Millisecond)
			}
			server := NewLoginCallbackServer(s)
			if err := server.Start(); err != nil {
				t.Fatal(err)
			}
			want := ErrLoginTimedOut
			if kind == "cancel" {
				server.Cancel()
				want = context.Canceled
			}
			if kind == "denied" {
				server.Submit(s.RedirectURI + "?error=access_denied&state=" + s.State)
				want = ErrLoginDenied
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := server.Wait(ctx)
			if !errors.Is(err, want) {
				t.Fatalf("wrong lifecycle result %v", err)
			}
			listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", s.Port))
			if err != nil {
				t.Fatal("port not released")
			}
			listener.Close()
		})
	}
	s := testLoginSession(t)
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if !errors.Is(NewLoginCallbackServer(s).Start(), ErrLoginPortInUse) {
		t.Fatal("port conflict not reported")
	}
}

func TestOAuthLoginManualAutomaticRace(t *testing.T) {
	s := testLoginSession(t)
	server := NewLoginCallbackServer(s)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Cancel()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); server.Submit("manual#" + s.State) }()
	go func() {
		defer wg.Done()
		resp, err := http.Get(s.RedirectURI + "?code=automatic&state=" + s.State)
		if err == nil {
			resp.Body.Close()
		}
	}()
	wg.Wait()
	code, err := server.Wait(context.Background())
	if err != nil || (code != "manual" && code != "automatic") {
		t.Fatal("race produced invalid result")
	}
}

func checkLoginGrant(t *testing.T, r *http.Request, s LoginSession) {
	t.Helper()
	raw, _ := io.ReadAll(r.Body)
	wantPrefix := `{"grant_type":"authorization_code","code":"secret-code","redirect_uri":`
	if !strings.HasPrefix(string(raw), wantPrefix) || r.Header.Get("User-Agent") != UserAgent("") || r.Header.Get("Accept") != "application/json" {
		t.Error("wrong grant wire format")
	}
	var grant map[string]string
	if json.Unmarshal(raw, &grant) != nil {
		t.Error("invalid grant")
	}
	for k, v := range map[string]string{"grant_type": "authorization_code", "code": "secret-code", "redirect_uri": s.RedirectURI, "client_id": OAuthClientID, "code_verifier": s.CodeVerifier, "state": s.State} {
		if grant[k] != v {
			t.Errorf("wrong grant field %s", k)
		}
	}
}
