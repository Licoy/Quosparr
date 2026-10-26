package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

const (
	// TokenEndpointPrimary is tried first. TokenEndpointFallback is used only
	// after a 404 or 405 from the primary endpoint.
	TokenEndpointPrimary  = "https://platform.claude.com/v1/oauth/token"
	TokenEndpointFallback = "https://console.anthropic.com/v1/oauth/token"
	// OAuthClientID is the Claude Code public client id.
	OAuthClientID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
)

// RefreshClient rotates a non-current account's refresh token.
// The current account is never passed here by Store.RefreshManaged.
// A 429 from usage must not cause a refresh; this client is only for expiry.
type RefreshClient struct {
	HTTP     *http.Client
	Primary  string
	Fallback string
}

type refreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

// Refresh posts the refresh grant. Both tokens must come back. invalid_grant
// is ErrInvalidGrant. The primary URL is retried on the fallback only for
// HTTP 404 and 405.
func (c RefreshClient) Refresh(ctx context.Context, refreshToken string) (access, refresh string, expiresIn int, err error) {
	if strings.TrimSpace(refreshToken) == "" {
		return "", "", 0, ErrRefreshFailed
	}
	access, refresh, expiresIn, status, err := c.post(ctx, c.primary(), refreshToken)
	if err != nil {
		return "", "", 0, err
	}
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		access, refresh, expiresIn, status, err = c.post(ctx, c.fallback(), refreshToken)
		if err != nil {
			return "", "", 0, err
		}
	}
	return finishRefresh(access, refresh, expiresIn, status)
}

func (c RefreshClient) primary() string {
	if c.Primary != "" {
		return c.Primary
	}
	return TokenEndpointPrimary
}

func (c RefreshClient) fallback() string {
	if c.Fallback != "" {
		return c.Fallback
	}
	return TokenEndpointFallback
}

func (c RefreshClient) post(ctx context.Context, rawURL, refreshToken string) (string, string, int, int, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     OAuthClientID,
	})
	if err != nil {
		return "", "", 0, 0, classifiedError(StageRefresh, KindInvalidData, errors.Join(ErrRefreshFailed, err))
	}
	client, err := c.client(ctx)
	if err != nil {
		return "", "", 0, 0, operationError(StageRefresh, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return "", "", 0, 0, classifiedError(StageRefresh, KindInvalidData, errors.Join(ErrRefreshFailed, err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, 0, operationError(StageRefresh, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, usageMaxBody+1))
	if err != nil {
		return "", "", 0, 0, operationError(StageRefresh, errors.Join(ErrRefreshFailed, err))
	}
	var parsed refreshResponse
	if len(bytes.TrimSpace(payload)) > 0 && json.Unmarshal(payload, &parsed) != nil {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			return "", "", 0, resp.StatusCode, nil
		}
		kind := KindInvalidData
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			kind = KindHTTP
		}
		return "", "", 0, resp.StatusCode, &OperationError{Stage: StageRefresh, Kind: kind, HTTPStatus: resp.StatusCode, cause: ErrRefreshFailed}
	}
	if parsed.Error == "invalid_grant" {
		return "", "", 0, resp.StatusCode, &OperationError{Stage: StageRefresh, Kind: KindHTTP, HTTPStatus: resp.StatusCode, cause: ErrInvalidGrant}
	}
	return parsed.AccessToken, parsed.RefreshToken, parsed.ExpiresIn, resp.StatusCode, nil
}

func finishRefresh(access, refresh string, expiresIn, status int) (string, string, int, error) {
	if status < 200 || status > 299 {
		return "", "", 0, &OperationError{Stage: StageRefresh, Kind: KindHTTP, HTTPStatus: status, cause: ErrRefreshFailed}
	}
	if access == "" || refresh == "" {
		return "", "", 0, ErrIncompleteRefresh
	}
	return access, refresh, expiresIn, nil
}

func (c RefreshClient) client(ctx context.Context) (*http.Client, error) {
	if c.HTTP != nil {
		return c.HTTP, nil
	}
	return netx.ClientFor(ctx, netx.WithoutCookies)
}

// ExpiryFromRefresh converts expires_in seconds into an absolute time.
func ExpiryFromRefresh(now time.Time, expiresIn int) (time.Time, bool) {
	if expiresIn <= 0 {
		return time.Time{}, false
	}
	return now.Add(time.Duration(expiresIn) * time.Second), true
}
