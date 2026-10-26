package grok

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Defaults of TokenRefresher.
const (
	// RefreshTokenURL is the OAuth token endpoint of xAI.
	RefreshTokenURL = OAuthIssuer + "/oauth2/token"
	// RefreshSkew: a token is refreshed when it expires within this window.
	RefreshSkew = 120 * time.Second

	refreshRequestTimeout = 20 * time.Second
)

// AccountTimeoutError: a request ran into its timeout.
type AccountTimeoutError struct{ Operation string }

func (e *AccountTimeoutError) Error() string { return "grok: " + e.Operation + " timed out" }

// TokenRefresher renews Grok OAuth access tokens with the standard
// refresh_token grant. It never writes files: callers persist the returned
// auth.json bytes to the right home (the managed copy for a non-current
// account; official file and managed copy for the current one).
type TokenRefresher struct {
	// HTTP nil resolves the current netx context for each request.
	HTTP *http.Client
	// TokenURL defaults to RefreshTokenURL.
	TokenURL string
	// Skew defaults to RefreshSkew when zero; negative values mean no skew.
	Skew time.Duration
	// ClientIDFallback is used when the credential names no client id; it
	// defaults to OAuthClientID.
	ClientIDFallback string
}

// EnsureResult is the outcome of EnsureFresh.
type EnsureResult struct {
	Data       []byte
	DidRefresh bool
	Document   AuthDocument
}

func (r TokenRefresher) tokenURL() string {
	if r.TokenURL != "" {
		return r.TokenURL
	}
	return RefreshTokenURL
}

func (r TokenRefresher) skew() time.Duration {
	switch {
	case r.Skew == 0:
		return RefreshSkew
	case r.Skew < 0:
		return 0
	}
	return r.Skew
}

// EnsureFresh refreshes only when the access token is missing or about to
// expire and a refresh token exists.
func (r TokenRefresher) EnsureFresh(ctx context.Context, data []byte, now time.Time) (EnsureResult, error) {
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return EnsureResult{}, err
	}
	if !r.NeedsRefresh(doc, now) {
		return EnsureResult{Data: data, Document: doc}, nil
	}
	if !doc.Identity.HasRefreshToken {
		return EnsureResult{}, ErrAuthenticationRequired
	}
	refreshed, err := r.Refresh(ctx, data, now)
	if err != nil {
		return EnsureResult{}, err
	}
	refreshedDoc, err := ParseAuthDocument(refreshed)
	if err != nil {
		return EnsureResult{}, err
	}
	return EnsureResult{Data: refreshed, DidRefresh: true, Document: refreshedDoc}, nil
}

// Refresh always exchanges the refresh token and merges the answer into data.
func (r TokenRefresher) Refresh(ctx context.Context, data []byte, now time.Time) ([]byte, error) {
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return nil, err
	}
	refreshToken, err := doc.RefreshToken()
	if err != nil {
		return nil, err
	}
	clientID := firstOf(doc.Identity.ClientID, r.ClientIDFallback, OAuthClientID)
	tokens, err := r.postRefresh(ctx, refreshToken, clientID)
	if err != nil {
		return nil, err
	}
	return ApplyTokenRefresh(data, tokens.access, tokens.refresh, tokens.expiresIn, now)
}

// NeedsRefresh reports whether the access token should be renewed now.
func (r TokenRefresher) NeedsRefresh(doc AuthDocument, now time.Time) bool {
	if doc.Identity.Kind == AuthKindLegacySession {
		// A web session cookie cannot be renewed with OAuth.
		return false
	}
	if !doc.Identity.HasAccessToken {
		return doc.Identity.HasRefreshToken
	}
	skew := r.skew()
	if exp := doc.Identity.ExpiresAt; exp != nil {
		return exp.Sub(now) <= skew
	}
	if access, err := doc.AccessToken(); err == nil {
		return jwtExpiresWithin(access, now, skew)
	}
	return doc.Identity.HasRefreshToken
}

// jwtExpiresWithin is true when the token expires within skew of now; a token
// that cannot be decoded or has no numeric exp counts as expired.
func jwtExpiresWithin(token string, now time.Time, skew time.Duration) bool {
	claims := jwtClaims(token)
	if claims == nil {
		return true
	}
	number, ok := claims["exp"].(interface{ Float64() (float64, error) })
	if !ok {
		return true
	}
	exp, err := number.Float64()
	if err != nil {
		return true
	}
	expiry := time.Unix(0, 0).Add(time.Duration(exp * float64(time.Second)))
	return expiry.Sub(now) <= skew
}

type refreshedTokens struct {
	access    string
	refresh   string
	expiresIn int
}

func (r TokenRefresher) postRefresh(ctx context.Context, refreshToken, clientID string) (refreshedTokens, error) {
	form := "grant_type=refresh_token&refresh_token=" + formEncode(refreshToken) + "&client_id=" + formEncode(clientID)
	if _, err := url.Parse(r.tokenURL()); err != nil {
		return refreshedTokens{}, &TokenRequestError{Message: "invalid token URL"}
	}
	status, body, err := oauthDo(ctx, r.HTTP, http.MethodPost, r.tokenURL(), strings.NewReader(form), refreshRequestTimeout)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return refreshedTokens{}, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return refreshedTokens{}, &AccountTimeoutError{Operation: "token-refresh"}
		}
		return refreshedTokens{}, &TokenRequestError{Message: "token transport failed", Cause: err}
	}
	switch {
	case status >= 200 && status <= 299:
	case status == 400 || status == 401 || status == 403:
		return refreshedTokens{}, ErrAuthenticationRequired
	default:
		return refreshedTokens{}, &TokenRequestError{Message: fmt.Sprintf("token HTTP %d", status), StatusCode: status}
	}
	object, err := acctParseObject(body)
	if err != nil {
		return refreshedTokens{}, &MalformedResponseError{Message: "token refresh parse failed"}
	}
	if code := oauthString(object["error"]); code != "" {
		switch code {
		case "invalid_grant", "invalid_token", "access_denied":
			return refreshedTokens{}, ErrAuthenticationRequired
		}
		return refreshedTokens{}, &TokenRequestError{Message: "token server rejected request", StatusCode: status}
	}
	access := oauthString(object["access_token"])
	if access == "" {
		return refreshedTokens{}, &MalformedResponseError{Message: "token refresh missing access_token"}
	}
	tokens := refreshedTokens{access: access, refresh: oauthString(object["refresh_token"])}
	if n, ok := oauthNumber(object["expires_in"]); ok {
		tokens.expiresIn = n
	} else if s, ok := object["expires_in"].(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			tokens.expiresIn = n
		}
	}
	return tokens, nil
}

func isTimeout(err error) bool {
	var t interface{ Timeout() bool }
	return errors.As(err, &t) && t.Timeout()
}
