package resettoday

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

// Hosted endpoints of CodexFeed.
const (
	SiteURL   = "https://didcodexreset.com/"
	StatusURL = "https://didcodexreset.com/api/status.json"
)

const (
	requestTimeout   = 20 * time.Second
	maxResponseBytes = 4 << 20
)

// ErrBadServerResponse is a non-2xx answer (URLError.badServerResponse).
var ErrBadServerResponse = errors.New("reset-today: bad server response")

// Client fetches the status feed. A nil HTTP resolves the current netx context
// for every request, so it follows later proxy changes; tests set HTTP (and
// StatusURL) to an httptest server.
type Client struct {
	HTTP *http.Client
	// Feed is the monitor to read (zero: CodexFeed).
	Feed      Feed
	StatusURL string
	// DevMock, when set, returns a local fixture instead of calling the network.
	DevMock *DevMockKind
}

func (c Client) statusURL() string {
	if c.StatusURL != "" {
		return c.StatusURL
	}
	return c.Feed.StatusURL()
}

func httpClient(ctx context.Context, c *http.Client, policy netx.Policy) (*http.Client, error) {
	if c != nil {
		return c, nil
	}
	return netx.ClientFor(ctx, policy)
}

// FetchStatus downloads and decodes the feed, resolving its state at now in
// loc. The request times out after 20 seconds.
func (c Client) FetchStatus(ctx context.Context, now time.Time, loc *time.Location) (*Snapshot, error) {
	if c.DevMock != nil {
		return DevMock(*c.DevMock, now, loc), nil
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	client, err := httpClient(ctx, c.HTTP, netx.Standard)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.statusURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, ErrBadServerResponse
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, ErrBadServerResponse
	}
	return c.Feed.Decode(data, now, loc)
}
