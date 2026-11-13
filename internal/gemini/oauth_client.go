package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

const (
	// RefreshSkew: an access token is renewed when it expires within this window.
	RefreshSkew       = 15 * time.Minute
	oauthTimeout      = 20 * time.Second
	maxResponseBytes  = 4 << 20
	contentTypeForm   = "application/x-www-form-urlencoded"
	contentTypeJSON   = "application/json"
	errorInvalidGrant = "invalid_grant"
)

// LoginResult is a completed browser login.
type LoginResult struct {
	Credential []byte
	Identity   Identity
}

// OAuthClient talks to Google's token and userinfo endpoints. Empty URLs use
// the production endpoints; a nil HTTP follows the current netx proxy.
type OAuthClient struct {
	HTTP        *http.Client
	TokenURL    string
	UserInfoURL string
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
}

// Exchange trades the authorization code for tokens and resolves the
// account. A login without a refresh token is refused: it could not be
// kept alive.
func (c OAuthClient) Exchange(ctx context.Context, code string, session LoginSession, now time.Time) (LoginResult, error) {
	if !now.Before(session.ExpiresAt) {
		return LoginResult{}, ErrLoginTimedOut
	}
	if trimSpace(code) == "" || session.CodeVerifier == "" || session.RedirectURI == "" {
		return LoginResult{}, ErrLoginExchange
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {session.RedirectURI},
		"client_id":     {OAuthClientID},
		"client_secret": {oauthClientSecret},
		"code_verifier": {session.CodeVerifier},
	}
	token, err := c.postToken(ctx, form)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return LoginResult{}, err
		}
		return LoginResult{}, errors.Join(ErrLoginExchange, err)
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresIn <= 0 {
		return LoginResult{}, ErrLoginExchange
	}
	cred := NewCredential(token.AccessToken, token.RefreshToken, token.IDToken, now.Add(time.Duration(token.ExpiresIn)*time.Second))
	identity, err := c.Identify(ctx, cred)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return LoginResult{}, err
		}
		return LoginResult{}, errors.Join(ErrLoginIdentity, err)
	}
	data, err := cred.Marshal()
	if err != nil {
		return LoginResult{}, ErrLoginExchange
	}
	return LoginResult{Credential: data, Identity: identity}, nil
}

// Refresh always exchanges the refresh token. A missing refresh token or
// invalid_grant is ErrAuthenticationRequired.
func (c OAuthClient) Refresh(ctx context.Context, cred Credential, now time.Time) (Credential, error) {
	if cred.RefreshToken == "" {
		return Credential{}, ErrAuthenticationRequired
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {cred.RefreshToken},
		"client_id":     {OAuthClientID},
		"client_secret": {oauthClientSecret},
	}
	token, err := c.postToken(ctx, form)
	if err != nil {
		return Credential{}, err
	}
	if token.AccessToken == "" {
		return Credential{}, &TokenRequestError{StatusCode: http.StatusOK, Cause: ErrQuotaParse}
	}
	return cred.WithRefreshedTokens(token.AccessToken, token.RefreshToken, token.IDToken, token.ExpiresIn, now), nil
}

// EnsureFresh refreshes cred when it expires within RefreshSkew.
func (c OAuthClient) EnsureFresh(ctx context.Context, cred Credential, now time.Time) (Credential, bool, error) {
	if !cred.NeedsRefresh(now, RefreshSkew) {
		return cred, false, nil
	}
	refreshed, err := c.Refresh(ctx, cred, now)
	if err != nil {
		return Credential{}, false, err
	}
	return refreshed, true, nil
}

// Identify names the account of cred: the id_token claims when they carry a
// subject and email, otherwise userinfo (merged with any claims).
func (c OAuthClient) Identify(ctx context.Context, cred Credential) (Identity, error) {
	claims := cred.IDTokenIdentity()
	if claims.Subject != "" && claims.Email != "" {
		return claims, nil
	}
	info, err := c.UserInfo(ctx, cred.AccessToken)
	if err != nil {
		if claims.Known() {
			return claims, nil
		}
		return Identity{}, err
	}
	identity := info.merge(claims)
	if !identity.Known() {
		return Identity{}, ErrIdentityUnknown
	}
	return identity, nil
}

// UserInfo reads id, email, name and picture for an access token.
func (c OAuthClient) UserInfo(ctx context.Context, accessToken string) (Identity, error) {
	if accessToken == "" {
		return Identity{}, ErrUnauthorized
	}
	endpoint := firstNonEmpty(c.UserInfoURL, UserInfoURL)
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Accept", contentTypeJSON)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	status, body, _, err := doRequest(ctx, c.HTTP, req)
	if err != nil {
		return Identity{}, transportError(ctx, "userinfo", err)
	}
	switch {
	case status == http.StatusUnauthorized:
		return Identity{}, ErrUnauthorized
	case status < 200 || status > 299:
		return Identity{}, &StatusError{Method: "userinfo", StatusCode: status}
	}
	var info struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if json.Unmarshal(body, &info) != nil {
		return Identity{}, ErrQuotaParse
	}
	return Identity{Subject: trimSpace(info.ID), Email: trimSpace(info.Email), Name: trimSpace(info.Name), Picture: trimSpace(info.Picture)}, nil
}

func (c OAuthClient) postToken(ctx context.Context, form url.Values) (tokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, firstNonEmpty(c.TokenURL, TokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Accept", contentTypeJSON)
	req.Header.Set("Content-Type", contentTypeForm)
	status, body, _, err := doRequest(ctx, c.HTTP, req)
	if err != nil {
		return tokenResponse{}, transportError(ctx, "token request", err)
	}
	var token tokenResponse
	parseErr := json.Unmarshal(body, &token)
	if token.Error == errorInvalidGrant {
		return tokenResponse{}, ErrAuthenticationRequired
	}
	if status < 200 || status > 299 {
		return tokenResponse{}, &TokenRequestError{StatusCode: status}
	}
	if parseErr != nil {
		return tokenResponse{}, &TokenRequestError{StatusCode: status, Cause: ErrQuotaParse}
	}
	return token, nil
}

// doRequest sends req without following redirects (no header or body may
// leave the endpoint) and reads at most maxResponseBytes.
func doRequest(ctx context.Context, client *http.Client, req *http.Request) (int, []byte, http.Header, error) {
	if client == nil {
		var err error
		if client, err = netx.ClientFor(ctx, netx.WithoutCookies); err != nil {
			return 0, nil, nil, err
		}
	}
	copied := *client
	copied.Jar = nil
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := copied.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return 0, nil, nil, err
	}
	if len(body) > maxResponseBytes {
		return 0, nil, nil, ErrQuotaParse
	}
	return resp.StatusCode, body, resp.Header, nil
}

// transportError keeps cancellation and timeouts recognizable and drops the
// transport text (it may contain the URL, never a token, but stays short).
func transportError(ctx context.Context, operation string, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var t interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &t) && t.Timeout()) {
		return &TimeoutError{Operation: operation}
	}
	return err
}
