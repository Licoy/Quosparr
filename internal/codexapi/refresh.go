package codexapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

// TokenSet is the tokens of one OAuth credential. In a refresh response, an
// empty IDToken or RefreshToken means the server did not send one.
type TokenSet struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
}

// MergedInto applies a refresh response over the current tokens: the access
// token is always replaced, the ID and refresh tokens only when the server
// returned a new one.
func (t TokenSet) MergedInto(current TokenSet) TokenSet {
	merged := current
	if t.IDToken != "" {
		merged.IDToken = t.IDToken
	}
	merged.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		merged.RefreshToken = t.RefreshToken
	}
	return merged
}

// TokenPersister stores rotated tokens. The auth and account storage layer
// implements it: for the current account it writes the official auth file and
// the account library copy, for any other account only the library copy.
// This package never writes a credential file.
type TokenPersister interface {
	SaveRefreshed(ctx context.Context, accountID string, tokens TokenSet, refreshedAt time.Time) error
}

// TokenRefresher exchanges a refresh token for a new token set.
type TokenRefresher struct {
	// HTTP nil resolves the current netx context for each request.
	HTTP *http.Client
	// TokenURL defaults to the OpenAI OAuth token endpoint.
	TokenURL string
}

func (r TokenRefresher) tokenURL() string {
	if r.TokenURL != "" {
		return r.TokenURL
	}
	return TokenEndpoint
}

// Refresh exchanges refreshToken. The Codex client id is sent first; if the
// server rejects it (invalid_client) the request is repeated without it for
// older grants. A refused or revoked grant is ErrAuthenticationRequired, any
// other failing status an *HTTPError. The returned set holds the raw response
// fields; use MergedInto to combine it with the stored tokens.
func (r TokenRefresher) Refresh(ctx context.Context, refreshToken string) (TokenSet, error) {
	if refreshToken == "" {
		return TokenSet{}, ErrAuthenticationRequired
	}
	data, err := r.post(ctx, refreshToken, true)
	if err == errClientIDRejected {
		data, err = r.post(ctx, refreshToken, false)
	}
	if err != nil {
		return TokenSet{}, err
	}
	return decodeTokenResponse(data)
}

// RefreshAndPersist refreshes current.RefreshToken, merges the response over
// current and hands the result to p. If persisting fails the merged tokens are
// still returned together with the error: the server may already have rotated
// the refresh token, so the caller must retry persisting rather than drop them.
func (r TokenRefresher) RefreshAndPersist(
	ctx context.Context, p TokenPersister, accountID string, current TokenSet, now time.Time,
) (TokenSet, error) {
	response, err := r.Refresh(ctx, current.RefreshToken)
	if err != nil {
		return current, err
	}
	merged := response.MergedInto(current)
	if p == nil {
		return merged, nil
	}
	if err := p.SaveRefreshed(ctx, accountID, merged, now); err != nil {
		return merged, fmt.Errorf("save refreshed tokens: %w", err)
	}
	return merged, nil
}

type clientIDRejected struct{}

func (clientIDRejected) Error() string { return "client id rejected" }

var errClientIDRejected error = clientIDRejected{}

func (r TokenRefresher) post(ctx context.Context, refreshToken string, includeClientID bool) ([]byte, error) {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refreshToken}}
	if includeClientID {
		form.Set("client_id", ClientID)
	}
	status, body, err := postForm(ctx, r.HTTP, r.tokenURL(), form, requestTimeout)
	if err != nil {
		return nil, err
	}
	if status >= 200 && status <= 299 {
		return body, nil
	}
	code := oauthErrorCode(body)
	if includeClientID && (status == 400 || status == 401) && code == "invalid_client" {
		return nil, errClientIDRejected
	}
	if status == 401 || (status == 400 && (code == "invalid_grant" || code == "invalid_token")) {
		return nil, ErrAuthenticationRequired
	}
	return nil, &HTTPError{StatusCode: status}
}

// oauthErrorCode reads "error" as a string or as an object with a "code".
func oauthErrorCode(body []byte) string {
	var payload struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	var text string
	if json.Unmarshal(payload.Error, &text) == nil {
		return text
	}
	var object struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(payload.Error, &object) == nil {
		return object.Code
	}
	return ""
}

func decodeTokenResponse(data []byte) (TokenSet, error) {
	var resp struct {
		IDToken      string  `json:"id_token"`
		AccessToken  *string `json:"access_token"`
		RefreshToken string  `json:"refresh_token"`
	}
	if err := json.Unmarshal(data, &resp); err != nil || resp.AccessToken == nil {
		return TokenSet{}, decodeErrorf("token response")
	}
	return TokenSet{IDToken: resp.IDToken, AccessToken: *resp.AccessToken, RefreshToken: resp.RefreshToken}, nil
}

// postForm sends a form POST and returns the status and body. The body is
// limited and is never part of an error.
func postForm(ctx context.Context, client *http.Client, rawURL string, form url.Values, timeout time.Duration) (int, []byte, error) {
	if client == nil {
		var err error
		if client, err = netx.ClientFor(ctx, netx.Standard); err != nil {
			return 0, nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := netx.DoAuthenticated(client, req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}
