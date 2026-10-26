package claude

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

const (
	usageEndpoint   = "https://api.anthropic.com/api/oauth/usage"
	profileEndpoint = "https://api.anthropic.com/api/oauth/profile"
	usageBetaHeader = "oauth-2025-04-20"
	usageVersion    = "2023-06-01"
	usageMaxBody    = 2 << 20
	usageTimeout    = 20 * time.Second
	// fallbackCLIVersion is the User-Agent version when the local claude CLI
	// version cannot be read. The identity string matches the CLI form
	// claude-cli/<version> (external, cli).
	fallbackCLIVersion = "2.1.299"
	cliEntrypoint      = "cli"
)

// UsageClient calls the Claude Code OAuth usage and profile endpoints.
// Requests go through netx unless HTTP is set (tests). TLS is not disabled.
// The URL asks for the reset grant block but has no skip_spend query, which
// would drop the extra usage spend. Version is the claude CLI version used in
// the User-Agent claude-cli/<version> (external, cli). An empty version is
// sent as 2.1.299.
type UsageClient struct {
	HTTP            *http.Client
	Version         string
	Now             func() time.Time
	Endpoint        string
	ProfileEndpoint string
}

// Fetch loads the usage JSON. The time is when the response was read.
func (c *UsageClient) Fetch(ctx context.Context, accessToken string) ([]byte, time.Time, error) {
	return c.get(ctx, c.usageURL(), accessToken)
}

// FetchProfile loads the profile JSON.
func (c *UsageClient) FetchProfile(ctx context.Context, accessToken string) ([]byte, time.Time, error) {
	data, at, err := c.get(ctx, c.profileURL(), accessToken)
	var detail *OperationError
	if errors.As(err, &detail) {
		copy := *detail
		copy.Stage = StageReadIdentity
		err = &copy
	} else {
		err = operationError(StageReadIdentity, err)
	}
	return data, at, err
}

func (c *UsageClient) usageURL() string {
	if c != nil && c.Endpoint != "" {
		return c.Endpoint + "?" + resetGrantQuery
	}
	return usageEndpoint + "?" + resetGrantQuery
}

func (c *UsageClient) profileURL() string {
	if c != nil && c.ProfileEndpoint != "" {
		return c.ProfileEndpoint
	}
	return profileEndpoint
}

func (c *UsageClient) now() time.Time {
	if c != nil && c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *UsageClient) get(ctx context.Context, rawURL, accessToken string) ([]byte, time.Time, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, usageTimeout)
		defer cancel()
	}
	client, err := c.client(ctx)
	if err != nil {
		return nil, time.Time{}, operationError(StageReadUsage, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, time.Time{}, classifiedError(StageReadUsage, KindInvalidData, errors.Join(ErrUsageStatus, err))
	}
	applyUsageHeaders(req.Header, accessToken, c.version())
	resp, err := client.Do(req)
	if err != nil {
		return nil, time.Time{}, operationError(StageReadUsage, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, usageMaxBody+1))
	if err != nil {
		return nil, time.Time{}, operationError(StageReadUsage, err)
	}
	if len(body) > usageMaxBody {
		return nil, time.Time{}, classifiedError(StageReadUsage, KindInvalidData, ErrUsageParse)
	}
	if err := usageStatus(resp); err != nil {
		return nil, time.Time{}, &OperationError{Stage: StageReadUsage, Kind: KindHTTP, HTTPStatus: resp.StatusCode, cause: err}
	}
	return body, c.now(), nil
}

func (c *UsageClient) version() string {
	if c == nil {
		return ""
	}
	return c.Version
}

func (c *UsageClient) client(ctx context.Context) (*http.Client, error) {
	if c != nil && c.HTTP != nil {
		return c.HTTP, nil
	}
	return netx.ClientFor(ctx, netx.WithoutCookies)
}

// UserAgent is the Claude Code client identity. An empty version becomes 2.1.299.
func UserAgent(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = fallbackCLIVersion
	}
	return "claude-cli/" + version + " (external, " + cliEntrypoint + ")"
}

func applyUsageHeaders(h http.Header, accessToken, version string) {
	h.Set("Authorization", "Bearer "+accessToken)
	h.Set("anthropic-beta", usageBetaHeader)
	h.Set("anthropic-version", usageVersion)
	h.Set("x-app", "cli")
	h.Set("User-Agent", UserAgent(version))
	h.Set("Accept", "application/json")
}

func usageStatus(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return &RateLimitedError{RetryAfter: retryAfter(resp), HasRetryAfter: hasRetryAfter(resp)}
	default:
		return ErrUsageStatus
	}
}

func hasRetryAfter(resp *http.Response) bool {
	return strings.TrimSpace(resp.Header.Get("Retry-After")) != ""
}

func retryAfter(resp *http.Response) time.Duration {
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		wait := time.Until(when)
		if wait < 0 {
			return 0
		}
		return wait
	}
	return 0
}
