package codexapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

// DefaultBaseURL is the ChatGPT backend API root.
const DefaultBaseURL = "https://chatgpt.com/backend-api"

const (
	userAgent          = "Quosparr/1"
	requestTimeout     = 20 * time.Second
	workspaceTimeout   = 8 * time.Second
	maxResponseBytes   = 16 << 20
	maxErrorBodyToDrop = 4 << 10
)

// QuotaClient calls the ChatGPT backend with a bearer token. A nil HTTP client
// resolves the current netx context for every request, so a client created
// once follows later proxy changes; an explicit client stays fixed.
type QuotaClient struct {
	HTTP    *http.Client
	BaseURL string
	// Now stamps decoded snapshots; nil means time.Now.
	Now func() time.Time
}

func (c QuotaClient) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c QuotaClient) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return DefaultBaseURL
}

func (c QuotaClient) client(ctx context.Context) (*http.Client, error) {
	if c.HTTP != nil {
		return c.HTTP, nil
	}
	return netx.ClientFor(ctx, netx.Standard)
}

// FetchQuota reads the 5 hour, weekly and additional rate limit windows.
func (c QuotaClient) FetchQuota(ctx context.Context, auth Auth) (QuotaSnapshot, error) {
	data, err := c.get(ctx, c.baseURL()+"/wham/usage", auth, requestTimeout)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	return DecodeQuota(data, c.now())
}

// FetchResetCredits reads the reset credits of the account.
func (c QuotaClient) FetchResetCredits(ctx context.Context, auth Auth) (ResetCreditsSnapshot, error) {
	data, err := c.get(ctx, c.baseURL()+"/wham/rate-limit-reset-credits", auth, requestTimeout)
	if err != nil {
		return ResetCreditsSnapshot{}, err
	}
	return DecodeResetCredits(data, c.now())
}

// FetchTokenUsageProfile reads the daily token series of the Codex profile.
func (c QuotaClient) FetchTokenUsageProfile(ctx context.Context, auth Auth) (TokenUsageProfile, error) {
	data, err := c.get(ctx, c.baseURL()+"/wham/profiles/me", auth, requestTimeout)
	if err != nil {
		return TokenUsageProfile{}, err
	}
	return DecodeTokenUsageProfile(data)
}

// FetchDailyWorkspaceUsageRaw returns the body of the daily workspace usage
// analytics for the inclusive YYYY-MM-DD range. The cost layer decodes it.
func (c QuotaClient) FetchDailyWorkspaceUsageRaw(ctx context.Context, auth Auth, startDate, endDate string) ([]byte, error) {
	// Same query item order as the Swift client.
	raw := "start_date=" + url.QueryEscape(startDate) + "&end_date=" + url.QueryEscape(endDate) + "&group_by=day"
	return c.get(ctx, c.baseURL()+"/wham/analytics/daily-workspace-usage-counts?"+raw, auth, requestTimeout)
}

// FetchWorkspaceName reads the display name of the selected workspace. It is
// best-effort metadata: callers must not let a failure block a quota refresh or
// an account switch. It returns "" when there is no name or no account id.
func (c QuotaClient) FetchWorkspaceName(ctx context.Context, auth Auth) (string, error) {
	accountID := auth.OAuthAccountID()
	if auth.AccessToken == "" || accountID == "" {
		return "", nil
	}
	data, err := c.get(ctx, c.baseURL()+"/accounts", auth, workspaceTimeout)
	if err != nil {
		return "", err
	}
	return DecodeWorkspaceName(data, accountID)
}

// DecodeWorkspaceName finds the workspace with exactly accountID in the
// accounts response and returns its trimmed, non-empty name.
func DecodeWorkspaceName(data []byte, accountID string) (string, error) {
	var resp struct {
		Items []struct {
			ID   string  `json:"id"`
			Name *string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", decodeErrorf("accounts: %v", err)
	}
	for _, item := range resp.Items {
		if item.ID == accountID {
			return derefTrim(item.Name), nil
		}
	}
	return "", nil
}

func derefTrim(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func (c QuotaClient) get(ctx context.Context, rawURL string, auth Auth, timeout time.Duration) ([]byte, error) {
	client, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if accountID := auth.OAuthAccountID(); accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	resp, err := netx.DoAuthenticated(client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyToDrop))
		return nil, ErrAuthenticationRequired
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyToDrop))
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	return data, nil
}
