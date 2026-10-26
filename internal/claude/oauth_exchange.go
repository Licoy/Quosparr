package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

type LoginResult struct {
	Credential []byte
	Identity   CredentialIdentity
}

type OAuthClient struct {
	HTTP                 *http.Client
	TokenURL, ProfileURL string
}

type loginToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Account      struct {
		UUID string `json:"uuid"`
	} `json:"account"`
	Organization struct {
		UUID string `json:"uuid"`
	} `json:"organization"`
}

func (c OAuthClient) Exchange(ctx context.Context, code string, session LoginSession, now time.Time) (LoginResult, error) {
	if err := ctx.Err(); err != nil {
		return LoginResult{}, err
	}
	if !now.Before(session.ExpiresAt) {
		return LoginResult{}, ErrLoginTimedOut
	}
	if strings.TrimSpace(code) == "" || session.State == "" || session.CodeVerifier == "" {
		return LoginResult{}, ErrLoginExchange
	}
	ctx, cancel := context.WithTimeout(ctx, min(usageTimeout, session.ExpiresAt.Sub(now)))
	defer cancel()
	client, err := c.loginClient(ctx)
	if err != nil {
		return LoginResult{}, loginNetworkError(ctx, ErrLoginExchange)
	}
	endpoint := c.TokenURL
	if endpoint == "" {
		endpoint = TokenEndpointPrimary
	}
	grant := struct {
		GrantType    string `json:"grant_type"`
		Code         string `json:"code"`
		RedirectURI  string `json:"redirect_uri"`
		ClientID     string `json:"client_id"`
		CodeVerifier string `json:"code_verifier"`
		State        string `json:"state"`
	}{"authorization_code", code, session.RedirectURI, OAuthClientID, session.CodeVerifier, session.State}
	body, _ := json.Marshal(grant)
	payload, err := loginRequest(ctx, client, http.MethodPost, endpoint, body, "")
	if err != nil {
		return LoginResult{}, loginNetworkError(ctx, ErrLoginExchange)
	}
	var token loginToken
	if json.Unmarshal(payload, &token) != nil || strings.TrimSpace(token.AccessToken) == "" || strings.TrimSpace(token.RefreshToken) == "" || token.ExpiresIn <= 0 || token.ExpiresIn > int64((1<<63-1)/int64(time.Second)) {
		return LoginResult{}, ErrLoginExchange
	}
	return c.loginIdentity(ctx, client, token, now)
}

func (c OAuthClient) loginIdentity(ctx context.Context, client *http.Client, token loginToken, now time.Time) (LoginResult, error) {
	endpoint := c.ProfileURL
	if endpoint == "" {
		endpoint = profileEndpoint
	}
	raw, err := loginRequest(ctx, client, http.MethodGet, endpoint, nil, token.AccessToken)
	if err != nil {
		return LoginResult{}, loginNetworkError(ctx, ErrLoginIdentity)
	}
	identity, err := IdentityFromProfile(raw)
	if err != nil || identity.AccountUUID == "" || identity.OrganizationUUID == "" {
		return LoginResult{}, ErrLoginIdentity
	}
	if (token.Account.UUID != "" && token.Account.UUID != identity.AccountUUID) || (token.Organization.UUID != "" && token.Organization.UUID != identity.OrganizationUUID) {
		return LoginResult{}, ErrLoginIdentity
	}
	scopes := strings.Fields(token.Scope)
	if len(scopes) == 0 {
		scopes = strings.Fields(loginScopes)
	}
	expiry := now.Add(time.Duration(token.ExpiresIn) * time.Second)
	credential, err := json.Marshal(map[string]any{oauthKey: map[string]any{"accessToken": token.AccessToken, "refreshToken": token.RefreshToken, "expiresAt": expiry.UnixMilli(), "scopes": scopes}})
	if err != nil {
		return LoginResult{}, ErrLoginExchange
	}
	identity.HasAccessToken, identity.HasRefreshToken, identity.ExpiresAt = true, true, &expiry
	return LoginResult{Credential: credential, Identity: identity}, nil
}

func (c OAuthClient) loginClient(ctx context.Context) (*http.Client, error) {
	client := c.HTTP
	if client == nil {
		var err error
		client, err = netx.ClientFor(ctx, netx.WithoutCookies)
		if err != nil {
			return nil, err
		}
	}
	copy := *client
	copy.Jar = nil
	// Reject all redirects: neither authorization headers nor grant bodies may escape.
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy, nil
}

func loginRequest(ctx context.Context, client *http.Client, method, endpoint string, body []byte, access string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ErrLoginExchange
	}
	req.Header.Set("Accept", "application/json")
	if access != "" {
		applyUsageHeaders(req.Header, access, "")
	} else {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", UserAgent(""))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, ErrLoginExchange
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, usageMaxBody+1))
	if err != nil || len(raw) > usageMaxBody || resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, ErrLoginExchange
	}
	return raw, nil
}

func loginNetworkError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		if err == context.DeadlineExceeded {
			return ErrLoginTimedOut
		}
		return err
	}
	return fallback
}
