package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginCallbackBuildsGoogleURLAndAcceptsBrowserRedirect(t *testing.T) {
	server, err := StartLoginCallback(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Cancel()
	s := server.Session()
	u, err := url.Parse(s.AuthURL)
	if err != nil || !strings.HasPrefix(s.AuthURL, AuthorizeURL+"?") {
		t.Fatalf("auth url: %v", err)
	}
	q := u.Query()
	want := map[string]string{
		"client_id": OAuthClientID, "response_type": "code", "access_type": "offline", "prompt": "consent",
		"include_granted_scopes": "true", "code_challenge_method": "S256", "state": s.State,
		"redirect_uri": fmt.Sprintf("http://localhost:%d/oauth-callback", s.Port),
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	if q.Get("scope") != strings.Join(OAuthScopes, " ") || q.Get("code_challenge") == "" {
		t.Fatal("scope or challenge missing")
	}
	if q.Has("client_secret") {
		t.Fatal("the secret never goes to the browser")
	}
	bad, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/oauth-callback?state=wrong&code=c", s.Port))
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong state: %d", bad.StatusCode)
	}
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/oauth-callback?state=%s&code=4/abc&scope=x&authuser=0", s.Port, s.State))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	code, err := server.Wait(context.Background())
	if err != nil || code != "4/abc" {
		t.Fatalf("code = %q, %v", code, err)
	}
}

func TestLoginCallbackSubmitPastedURL(t *testing.T) {
	server, err := StartLoginCallback(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Cancel()
	s := server.Session()
	if err := server.Submit("4/abc"); !errors.Is(err, ErrLoginCallback) {
		t.Fatalf("bare code: %v", err)
	}
	if err := server.Submit(fmt.Sprintf("http://localhost:%d/oauth-callback?state=%s&error=access_denied", s.Port, s.State)); !errors.Is(err, ErrLoginDenied) {
		t.Fatalf("denied: %v", err)
	}
	if _, err := server.Wait(context.Background()); !errors.Is(err, ErrLoginDenied) {
		t.Fatalf("wait: %v", err)
	}

	server2, err := StartLoginCallback(time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer server2.Cancel()
	s2 := server2.Session()
	if err := server2.Submit(fmt.Sprintf("  http://localhost:%d/oauth-callback?code=4%%2Fxyz&state=%s  ", s2.Port, s2.State)); err != nil {
		t.Fatal(err)
	}
	if code, err := server2.Wait(context.Background()); err != nil || code != "4/xyz" {
		t.Fatalf("code = %q, %v", code, err)
	}
}

// oauthServer fakes Google's token and userinfo endpoints.
type oauthServer struct {
	*httptest.Server
	refreshCalls int
	lastForm     url.Values
	tokenStatus  int
	tokenBody    string
	userinfo     string
}

func newOAuthServer(t *testing.T) *oauthServer {
	o := &oauthServer{userinfo: `{"id":"sub-1","email":"one@example.com","name":"One"}`}
	o.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = r.ParseForm()
			o.lastForm = r.PostForm
			if r.PostForm.Get("grant_type") == "refresh_token" {
				o.refreshCalls++
			}
			status := o.tokenStatus
			if status == 0 {
				status = http.StatusOK
			}
			w.WriteHeader(status)
			body := o.tokenBody
			if body == "" {
				body = fmt.Sprintf(`{"access_token":"fresh-%d","expires_in":3599,"token_type":"Bearer"}`, o.refreshCalls)
			}
			_, _ = w.Write([]byte(body))
		case "/userinfo":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(o.userinfo))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(o.Close)
	return o
}

func (o *oauthServer) client() OAuthClient {
	return OAuthClient{HTTP: o.Server.Client(), TokenURL: o.URL + "/token", UserInfoURL: o.URL + "/userinfo"}
}

func TestExchangeBuildsCredentialAndIdentity(t *testing.T) {
	o := newOAuthServer(t)
	o.tokenBody = `{"access_token":"acc","refresh_token":"ref","expires_in":3600}`
	session := LoginSession{RedirectURI: "http://localhost:1/oauth-callback", State: "s", CodeVerifier: "v", ExpiresAt: testNow.Add(time.Minute)}
	result, err := o.client().Exchange(context.Background(), "4/code", session, testNow)
	if err != nil {
		t.Fatal(err)
	}
	f := o.lastForm
	if f.Get("grant_type") != "authorization_code" || f.Get("code") != "4/code" || f.Get("code_verifier") != "v" ||
		f.Get("redirect_uri") != session.RedirectURI || f.Get("client_id") != OAuthClientID || f.Get("client_secret") == "" {
		t.Fatal("exchange form is incomplete")
	}
	if result.Identity.Subject != "sub-1" || result.Identity.Email != "one@example.com" {
		t.Fatalf("identity = %+v", result.Identity)
	}
	var doc map[string]any
	if err := json.Unmarshal(result.Credential, &doc); err != nil || doc["auth_method"] != "consumer" {
		t.Fatalf("credential document: %v", err)
	}
	c := mustCred(t, result.Credential)
	if c.AccessToken != "acc" || c.RefreshToken != "ref" || c.TokenType != "Bearer" || !c.Expiry.Equal(testNow.Add(time.Hour)) {
		t.Fatal("credential tokens")
	}

	o.tokenBody = `{"access_token":"acc","expires_in":3600}`
	if _, err := o.client().Exchange(context.Background(), "c", session, testNow); !errors.Is(err, ErrLoginExchange) {
		t.Fatalf("no refresh token: %v", err)
	}
}

func TestRefreshKeepsOldTokensAndMapsInvalidGrant(t *testing.T) {
	o := newOAuthServer(t)
	old := NewCredential("a0", "r0", "i0", testNow)
	got, err := o.client().Refresh(context.Background(), old, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "fresh-1" || got.RefreshToken != "r0" || got.IDToken != "i0" || !got.Expiry.Equal(testNow.Add(3599*time.Second)) {
		t.Fatalf("refreshed = %+v", got)
	}
	if o.lastForm.Get("refresh_token") != "r0" || o.lastForm.Get("client_secret") == "" {
		t.Fatal("refresh form is incomplete")
	}
	o.tokenBody = `{"access_token":"a1","refresh_token":"r1","id_token":"i1","expires_in":10}`
	got, _ = o.client().Refresh(context.Background(), old, testNow)
	if got.RefreshToken != "r1" || got.IDToken != "i1" {
		t.Fatal("new tokens must replace the old ones")
	}
	o.tokenStatus, o.tokenBody = http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`
	if _, err := o.client().Refresh(context.Background(), old, testNow); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("invalid_grant: %v", err)
	}
	o.tokenStatus, o.tokenBody = http.StatusInternalServerError, `{}`
	if _, err := o.client().Refresh(context.Background(), old, testNow); !isErr[*TokenRequestError](err) {
		t.Fatalf("5xx: %v", err)
	}
	if _, err := o.client().Refresh(context.Background(), NewCredential("a", "", "", testNow), testNow); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("no refresh token: %v", err)
	}
}

func TestIdentifyPrefersIDTokenClaims(t *testing.T) {
	o := newOAuthServer(t)
	o.userinfo = `{"id":"wrong"}`
	id, err := o.client().Identify(context.Background(), NewCredential("a", "r", idToken("sub-9", "nine@example.com"), testNow))
	if err != nil || id.Subject != "sub-9" {
		t.Fatalf("identity = %+v, %v", id, err)
	}
}
