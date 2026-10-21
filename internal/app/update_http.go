package app

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/licoy/quosparr/internal/netx"
)

// ClientFunc returns the HTTP client for the requests made under ctx.
type ClientFunc func(ctx context.Context) (*http.Client, error)

// updaterTransport is the RoundTripper that replaces http.DefaultClient's
// transport. This is the ONLY place in the application that replaces or uses
// the global client, and it exists because mygo v0.6.2's updater hard-codes
// http.DefaultClient for every request it makes (feed, manifest, delta,
// archive) and has no client option. Nothing else in the app may use
// http.DefaultClient, http.DefaultTransport, http.Get or http.Post: all other
// code takes an explicit client from internal/netx (a test enforces this).
//
// Each request is sent by the transport of the client that applies to the
// request's context: the updater uses one context for the whole cycle (check
// and install), so they share one network context, a settings change applies
// from the next cycle, the user's manual proxy and credentials are honoured,
// and a broken proxy configuration fails the request. A context that is not
// an updater cycle still gets a client from netx with the update policy for
// the current configuration; there is never a bare direct transport.
type updaterTransport struct {
	clients *cycleClients
}

// ErrUpdateNoTransport: the client of the network context has no transport,
// and a request must not fall back to a default one.
var ErrUpdateNoTransport = errors.New("updates: the network client has no transport")

func (t updaterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	client, err := t.clients.Client(req.Context())
	if err != nil {
		return nil, err // blocked network: an error, never a direct connection
	}
	if client.Transport == nil {
		return nil, ErrUpdateNoTransport
	}
	return client.Transport.RoundTrip(req)
}

// configureUpdaterHTTP points the global client at provider's transport. It
// must run before the updater starts, once at start-up. It copies the old
// client, so no shared value is modified in place; it returns the function
// that puts the previous global client back (for tests).
func configureUpdaterHTTP(provider ClientFunc) (restore func()) {
	previous := http.DefaultClient
	client := *previous
	client.Transport = updaterTransport{clients: newCycleClients(provider)}
	http.DefaultClient = &client
	return func() { http.DefaultClient = previous }
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
