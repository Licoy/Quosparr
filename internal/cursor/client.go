package cursor

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

const dashboardURL = "https://api2.cursor.sh/aiserver.v1.DashboardService/"
const maxResponseBytes = 1 << 20

type RateLimitedError struct{ RetryAfter time.Duration }

func (*RateLimitedError) Error() string { return "cursor: rate limited" }

// Client uses the desktop Connect unary protocol with an ephemeral Bearer token.
// HTTP and endpoint are test seams; production always uses the application's proxy.
type Client struct {
	HTTP     *http.Client
	endpoint string
}

func (c Client) Fetch(ctx context.Context, token string) (Snapshot, error) {
	if !validToken(token) {
		return Snapshot{}, ErrCredentials
	}
	client := c.HTTP
	if client == nil {
		var err error
		client, err = netx.ClientFor(ctx, netx.WithoutCookies)
		if err != nil {
			return Snapshot{}, ErrUnavailable
		}
	}
	// Never modify the shared netx client or follow a redirect carrying credentials.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base := c.endpoint
	if base == "" {
		base = dashboardURL
	}
	plan, err := c.request(ctx, &noRedirect, base, "GetPlanInfo", token)
	if err != nil {
		return Snapshot{}, err
	}
	usage, err := c.request(ctx, &noRedirect, base, "GetCurrentPeriodUsage", token)
	if err != nil {
		return Snapshot{}, err
	}
	out, err := ParseUsage(plan, usage, time.Now())
	if err != nil {
		return Snapshot{}, err
	}
	if !out.Shared && len(out.Pools) > 0 {
		// Spend only feeds the estimated amounts; the server percentages stand
		// without it. Authentication and rate limits still apply to the fetch.
		body, err := c.request(ctx, &noRedirect, base, "GetAggregatedUsageEvents", token)
		var limited *RateLimitedError
		if errors.Is(err, ErrUnauthorized) || errors.As(err, &limited) || ctx.Err() != nil {
			return Snapshot{}, err
		}
		if err == nil {
			if spend, err := ParseSpend(body); err == nil {
				ApplySpend(out.Pools, spend)
			}
		}
	}
	if out.Shared {
		body, err := c.request(ctx, &noRedirect, base, "GetClientVisibleCreditGrants", token)
		if err != nil {
			return Snapshot{}, err
		}
		out.Credits, err = ParseCredits(body)
		if err != nil {
			return Snapshot{}, err
		}
	}
	return out, nil
}

func (c Client) request(ctx context.Context, client *http.Client, base, method, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+method, http.NoBody)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Accept", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusTooManyRequests:
		wait := 5 * time.Minute
		v := resp.Header.Get("Retry-After")
		if seconds, err := strconv.ParseInt(v, 10, 32); err == nil && seconds >= 0 {
			wait = time.Duration(seconds) * time.Second
		} else if at, err := http.ParseTime(v); err == nil {
			wait = time.Until(at)
		}
		return nil, &RateLimitedError{RetryAfter: max(time.Second, wait)}
	case http.StatusOK:
	default:
		return nil, ErrUnavailable
	}
	if strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]) != "application/proto" {
		return nil, ErrResponse
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return nil, ErrResponse
	}
	return body, nil
}
