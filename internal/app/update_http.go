package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/licoy/quosparr/internal/netx"
)

// ClientFunc returns the HTTP client for the requests made under ctx.
type ClientFunc func(ctx context.Context) (*http.Client, error)

// Update feeds of the two channels, as mygo build derives them from the
// staged mygo.json (Scripts/stage-v1-config.py): stable builds read the
// latest release, prerelease builds the fixed v1-beta release.
const (
	stableUpdateFeed = "https://github.com/Licoy/Quosparr/releases/latest/download/"
	betaUpdateFeed   = "https://github.com/Licoy/Quosparr/releases/download/v1-beta/"
)

// updateFeed returns the URL of the update manifest of the build with
// version for goos/goarch. The release workflow builds prerelease versions
// for the beta channel only and release versions for stable only.
func updateFeed(version, goos, goarch string) string {
	feed := stableUpdateFeed
	if strings.Contains(version, "-") {
		feed = betaUpdateFeed
	}
	return feed + "update-" + goos + "-" + goarch + ".json"
}

// updateSource is the source of mygo's updater (Updater.SetSource): the
// manifest at feed and the files it names, each fetched with the client of
// the update cycle. It exists because mygo v0.7.4's updater sends its own
// requests through the system proxy only and has no client option; with
// this source the user's manual proxy and credentials are honoured, and a
// broken proxy configuration fails the request.
//
// The updater uses one context for the whole cycle (check and install), so
// they share one network context and a settings change applies from the
// next cycle. A context that is not an updater cycle still gets a client
// from netx with the update policy for the current configuration; there is
// never a bare direct transport.
type updateSource struct {
	feed    string
	clients *cycleClients
}

// ErrUpdateNoTransport: the client of the network context has no transport,
// and a request must not fall back to a default one.
var ErrUpdateNoTransport = errors.New("updates: the network client has no transport")

func newUpdateSource(feed string, provider ClientFunc) updateSource {
	return updateSource{feed: feed, clients: newCycleClients(provider)}
}

// Manifest fetches the manifest of the feed. The app has no prerelease
// switch: the channel is fixed by the build (updateFeed).
func (s updateSource) Manifest(ctx context.Context, _ bool) (io.ReadCloser, error) {
	return s.Open(ctx, s.feed)
}

// Open fetches rawURL over HTTPS with the client of ctx's cycle.
func (s updateSource) Open(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("updates: refusing %s: updates need HTTPS", rawURL)
	}
	client, err := s.clients.Client(ctx)
	if err != nil {
		return nil, err // blocked network: an error, never a direct connection
	}
	if client.Transport == nil {
		return nil, ErrUpdateNoTransport
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	// ASCII only: GitHub's asset store rejects other bytes in User-Agent.
	req.Header.Set("User-Agent", "mygo-updater")
	// Only the transport, as mygo's own requests: the cycle's context
	// bounds the download, and redirects follow the default policy.
	resp, err := (&http.Client{Transport: client.Transport}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("updates: GET %s: %s", rawURL, resp.Status)
	}
	return resp.Body, nil
}

// cycleClients hands the updater one client for a whole update cycle: the
// background check or the manual one, with the install that follows. The
// updater passes the same context to every request of one cycle, so the
// context identifies the cycle. A cycle takes the network settings in force
// when its first request is made; a change of settings applies from the
// next cycle. A client is dropped (its idle connections closed) when its
// cycle's context is done or a newer cycle starts. A context that can never
// end is no cycle: it gets a fresh client each time and nothing is kept.
type cycleClients struct {
	provider ClientFunc

	mu     sync.Mutex
	cycle  context.Context
	client *http.Client
}

func newCycleClients(provider ClientFunc) *cycleClients {
	return &cycleClients{provider: provider}
}

// Client returns the client of the cycle ctx belongs to.
func (c *cycleClients) Client(ctx context.Context) (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil && c.cycle == ctx {
		return c.client, nil
	}
	client, err := c.provider(ctx)
	if err != nil {
		return nil, err // never falls back to a direct connection
	}
	if ctx.Done() == nil {
		return client, nil
	}
	if c.client != nil {
		c.client.CloseIdleConnections()
	}
	c.cycle, c.client = ctx, client
	context.AfterFunc(ctx, func() {
		client.CloseIdleConnections()
		c.mu.Lock()
		if c.client == client {
			c.cycle, c.client = nil, nil
		}
		c.mu.Unlock()
	})
	return client, nil
}

// netxUpdateClient is the production provider: the client of the network
// context in force, with the update policy (no cookies, long timeouts). The
// request carries no account credentials: it is a plain client.
func netxUpdateClient(ctx context.Context) (*http.Client, error) {
	return netx.ClientFor(ctx, netx.Update)
}

// updateRoute is nil when the network configuration can be used: the check
// that a manual update does not start with a broken proxy.
func updateRoute() error {
	_, err := netx.Current()
	return err
}
