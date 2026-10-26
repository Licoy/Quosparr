package grok

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type tokenServer struct {
	status int
	body   string
	calls  atomic.Int32
	form   map[string]string
	header http.Header
}

func (s *tokenServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	_ = r.ParseForm()
	s.form = map[string]string{}
	for k := range r.PostForm {
		s.form[k] = r.PostForm.Get(k)
	}
	s.header = r.Header.Clone()
	if r.URL.String() != RefreshTokenURL || r.Method != http.MethodPost {
		http.Error(w, "unexpected request", http.StatusTeapot)
		return
	}
	w.WriteHeader(s.status)
	_, _ = w.Write([]byte(s.body))
}

func newRefresher(status int, body string) (TokenRefresher, *tokenServer) {
	srv := &tokenServer{status: status, body: body}
	return TokenRefresher{HTTP: handlerClient(srv)}, srv
}

func TestRefreshPostsRefreshGrantAndMergesTokens(t *testing.T) {
	r, srv := newRefresher(200, `{"access_token":" new-access ","refresh_token":"new-refresh","expires_in":3600}`)
	data := credSpec{email: "a@example.com", user: "a", access: "old-access", refresh: "old+refresh&=", expiresAt: "2020-01-01T00:00:00Z"}.json()
	out, err := r.Refresh(context.Background(), data, fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if srv.form["grant_type"] != "refresh_token" || srv.form["refresh_token"] != "old+refresh&=" || srv.form["client_id"] != "desktop-client" {
		t.Errorf("form = %v", srv.form)
	}
	if ct := srv.header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("content type = %q", ct)
	}
	if fieldOf(t, out, "key") != "new-access" || fieldOf(t, out, "refresh_token") != "new-refresh" ||
		fieldOf(t, out, "expires_at") != "2026-07-31T13:00:00Z" || fieldOf(t, out, "email") != "a@example.com" {
		t.Errorf("merged credential = %s", out)
	}
}

func TestRefreshKeepsRefreshTokenWithoutRotation(t *testing.T) {
	r, _ := newRefresher(200, `{"access_token":"n","expires_in":"120"}`)
	out, err := r.Refresh(context.Background(), cred("a@example.com", "a"), fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if fieldOf(t, out, "refresh_token") != "refresh-a" || fieldOf(t, out, "expires_at") != "2026-07-31T12:02:00Z" {
		t.Errorf("credential = %s", out)
	}
}

func TestRefreshStatusMapping(t *testing.T) {
	for _, status := range []int{400, 401, 403} {
		r, _ := newRefresher(status, `{"error":"whatever"}`)
		if _, err := r.Refresh(context.Background(), cred("a@example.com", "a"), fixtureNow); !errors.Is(err, ErrAuthenticationRequired) {
			t.Errorf("status %d: err = %v", status, err)
		}
	}
	r, _ := newRefresher(500, `oops`)
	_, err := r.Refresh(context.Background(), cred("a@example.com", "a"), fixtureNow)
	var req *TokenRequestError
	if !errors.As(err, &req) || req.Message != "token HTTP 500" {
		t.Errorf("err = %v", err)
	}
}

func TestRefreshBodyErrors(t *testing.T) {
	cases := []struct {
		body    string
		wantAut bool
		wantMal string
		wantReq string
	}{
		{body: `{"error":"invalid_grant"}`, wantAut: true},
		{body: `{"error":"invalid_token"}`, wantAut: true},
		{body: `{"error":"access_denied"}`, wantAut: true},
		{body: `{"error":"server_error","error_description":"try later"}`, wantReq: "token server rejected request"},
		{body: `{"error":"server_error"}`, wantReq: "token server rejected request"},
		{body: `{"expires_in":5}`, wantMal: "token refresh missing access_token"},
		{body: `not json`, wantMal: "token refresh parse failed"},
		{body: `[1]`, wantMal: "token refresh parse failed"},
	}
	for _, c := range cases {
		r, _ := newRefresher(200, c.body)
		_, err := r.Refresh(context.Background(), cred("a@example.com", "a"), fixtureNow)
		var mal *MalformedResponseError
		var req *TokenRequestError
		switch {
		case c.wantAut:
			if !errors.Is(err, ErrAuthenticationRequired) {
				t.Errorf("%s: err = %v", c.body, err)
			}
		case c.wantMal != "":
			if !errors.As(err, &mal) || mal.Message != c.wantMal {
				t.Errorf("%s: err = %v", c.body, err)
			}
		default:
			if !errors.As(err, &req) || req.Message != c.wantReq {
				t.Errorf("%s: err = %v", c.body, err)
			}
		}
	}
}

func TestRefreshErrorsNeverCarryTokens(t *testing.T) {
	r, _ := newRefresher(500, `{"refresh_token":"refresh-secret","access_token":"access-secret"}`)
	_, err := r.Refresh(context.Background(), credSpec{email: "a@example.com", user: "a", refresh: "refresh-secret"}.json(), fixtureNow)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("err = %v", err)
	}
}

func TestRefreshTimeoutAndCancellation(t *testing.T) {
	r := TokenRefresher{HTTP: &http.Client{Transport: timeoutTransport{}}}
	_, err := r.Refresh(context.Background(), cred("a@example.com", "a"), fixtureNow)
	var timeout *AccountTimeoutError
	if !errors.As(err, &timeout) || timeout.Operation != "token-refresh" {
		t.Errorf("err = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := &tokenServer{status: 200, body: `{"access_token":"x"}`}
	_, err = TokenRefresher{HTTP: handlerClient(srv)}.Refresh(ctx, cred("a@example.com", "a"), fixtureNow)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestRefreshWithoutRefreshToken(t *testing.T) {
	r, srv := newRefresher(200, `{"access_token":"x"}`)
	data := []byte(`{"https://auth.x.ai::desktop-client":{"key":"k","user_id":"u"}}`)
	if _, err := r.Refresh(context.Background(), data, fixtureNow); err == nil {
		t.Error("expected an error")
	}
	if srv.calls.Load() != 0 {
		t.Error("a request was sent without a refresh token")
	}
}

func TestEnsureFresh(t *testing.T) {
	valid := cred("a@example.com", "a")
	expired := credSpec{email: "a@example.com", user: "a", expiresAt: "2020-01-01T00:00:00Z"}.json()
	soon := credSpec{email: "a@example.com", user: "a", expiresAt: "2026-07-31T12:01:00Z"}.json()
	legacy := []byte(`{"https://accounts.x.ai/sign-in":{"key":"k","refresh_token":"r","user_id":"l","expires_at":"2020-01-01T00:00:00Z"}}`)
	noRefresh := []byte(`{"https://auth.x.ai::desktop-client":{"key":"k","user_id":"u","expires_at":"2020-01-01T00:00:00Z"}}`)

	for _, c := range []struct {
		name      string
		data      []byte
		refreshed bool
		err       error
	}{
		{"valid", valid, false, nil},
		{"expired", expired, true, nil},
		{"within skew", soon, true, nil},
		{"legacy never refreshes", legacy, false, nil},
		{"expired without refresh token", noRefresh, false, ErrAuthenticationRequired},
	} {
		r, srv := newRefresher(200, `{"access_token":"fresh","expires_in":3600}`)
		res, err := r.EnsureFresh(context.Background(), c.data, fixtureNow)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: err = %v", c.name, err)
			continue
		}
		if err == nil && (res.DidRefresh != c.refreshed || (srv.calls.Load() == 1) != c.refreshed) {
			t.Errorf("%s: didRefresh=%v calls=%d", c.name, res.DidRefresh, srv.calls.Load())
		}
		if err == nil && c.refreshed && fieldOf(t, res.Data, "key") != "fresh" {
			t.Errorf("%s: token not replaced", c.name)
		}
		if err == nil && !c.refreshed && string(res.Data) != string(c.data) {
			t.Errorf("%s: data changed without a refresh", c.name)
		}
	}
}

func jwtWithExp(exp int64) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc([]byte(fmt.Sprintf(`{"exp":%d}`, exp))) + ".sig"
}

func TestNeedsRefreshFallsBackToJWTExpiry(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	doc := func(access string, refresh bool) AuthDocument {
		body := fmt.Sprintf(`{"https://auth.x.ai::c":{"key":%q,"user_id":"u"`, access)
		if refresh {
			body += `,"refresh_token":"r"`
		}
		d, err := ParseAuthDocument([]byte(body + `}}`))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	r := TokenRefresher{}
	if r.NeedsRefresh(doc(jwtWithExp(now.Unix()+3600), true), now) {
		t.Error("a JWT valid for an hour needs no refresh")
	}
	if !r.NeedsRefresh(doc(jwtWithExp(now.Unix()+60), true), now) {
		t.Error("a JWT inside the skew needs a refresh")
	}
	if !r.NeedsRefresh(doc(jwtWithExp(now.Unix()-5), false), now) {
		t.Error("an expired JWT counts as expired even without a refresh token")
	}
	if !r.NeedsRefresh(doc("opaque-token", true), now) {
		t.Error("an unreadable expiry counts as expired")
	}
}
