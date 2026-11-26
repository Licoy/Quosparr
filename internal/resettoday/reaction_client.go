package resettoday

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/netx"
)

const maxReactionBytes = 1 << 20

// ReactionClient talks to the hosted reaction service. It sends the stored
// visitor ID as the hr_react cookie by hand (the HTTP client keeps no cookies)
// and stores a new ID the service mints. The ID is never logged.
type ReactionClient struct {
	// HTTP nil resolves the netx context without cookies for every request.
	HTTP *http.Client
	// Feed is the monitor whose reactions to read (zero: CodexFeed).
	Feed Feed
	// URL overrides the feed's reaction URL (tests).
	URL string
	// Store persists the visitor ID; an empty Path keeps it in memory only for
	// the process (nothing is loaded or saved).
	Store export.ReactionStore
	// DevMock, when set, serves a local counter instead of the network.
	DevMock *DevMockKind

	mu        sync.Mutex
	mockCount int
}

// DefaultReactionStore is ~/.quosparr/reaction-visitor.json.
func DefaultReactionStore() export.ReactionStore {
	home, err := os.UserHomeDir()
	if err != nil {
		return export.ReactionStore{}
	}
	return export.ReactionStore{Path: export.DefaultReactionPath(home)}
}

// AntigravityReactionStore is ~/.quosparr/agy-reaction-visitor.json. The two
// services mint their own visitor IDs, so each keeps its own file.
func AntigravityReactionStore() export.ReactionStore {
	home, err := os.UserHomeDir()
	if err != nil {
		return export.ReactionStore{}
	}
	return export.ReactionStore{Path: filepath.Join(filepath.Dir(export.DefaultReactionPath(home)), "agy-reaction-visitor.json")}
}

func (c *ReactionClient) url() string {
	if c.URL != "" {
		return c.URL
	}
	return c.Feed.ReactionURL()
}

func (c *ReactionClient) nextMock(increment bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mockCount == 0 {
		c.mockCount = 266
	}
	if increment {
		c.mockCount++
	}
	return c.mockCount
}

func (c *ReactionClient) visitorID() (string, bool) {
	if c.Store.Path == "" {
		return "", false
	}
	return c.Store.LoadVisitorID()
}

func (c *ReactionClient) newRequest(ctx context.Context, method string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.url(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if id, ok := c.visitorID(); ok {
		req.Header.Set("Cookie", ReactionCookieName+"="+id)
	}
	if method == http.MethodPost {
		req.Header.Set("Origin", c.Feed.Origin())
		req.Header.Set("Referer", c.Feed.Referer())
	}
	return req, nil
}

// do sends the request and returns the status and body. The visitor ID of the
// response is saved even when the status is an error.
func (c *ReactionClient) do(ctx context.Context, method string) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	client, err := httpClient(ctx, c.HTTP, netx.WithoutCookies)
	if err != nil {
		return 0, nil, err
	}
	req, err := c.newRequest(ctx, method)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReactionBytes))
	if err != nil {
		return 0, nil, err
	}
	if id, ok := VisitorIDFromResponse(resp); ok && c.Store.Path != "" {
		_ = c.Store.SaveVisitorID(id)
	}
	return resp.StatusCode, body, nil
}

// Fetch reads the counter. A 404 or an "disabled" envelope is
// ErrReactionDisabled; any other non-2xx status is ErrReactionUnavailable; an
// answer that is not an ok envelope with data is ErrReactionInvalidPayload.
func (c *ReactionClient) Fetch(ctx context.Context) (ReactionSnapshot, error) {
	if c.DevMock != nil {
		return ReactionDevMock(*c.DevMock, c.nextMock(false)), nil
	}
	status, body, err := c.do(ctx, http.MethodGet)
	if err != nil {
		return ReactionSnapshot{}, err
	}
	if status == http.StatusNotFound {
		return ReactionSnapshot{}, ErrReactionDisabled
	}
	if status < 200 || status > 299 {
		return ReactionSnapshot{}, ErrReactionUnavailable
	}
	envelope, err := DecodeReactionEnvelope(body)
	if err != nil {
		return ReactionSnapshot{}, ErrReactionInvalidPayload
	}
	if envelope.Error != nil && *envelope.Error == "disabled" {
		return ReactionSnapshot{}, ErrReactionDisabled
	}
	if !envelope.OK || envelope.Data == nil {
		return ReactionSnapshot{}, ErrReactionInvalidPayload
	}
	return *envelope.Data, nil
}

// Click posts one reaction. An undecodable answer is {OK: false}; a non-2xx
// answer is not OK and carries the envelope's error or one derived from the
// status, plus the envelope's data.
func (c *ReactionClient) Click(ctx context.Context) (ReactionPostResult, error) {
	if c.DevMock != nil {
		n := c.nextMock(true)
		data := ReactionDevMock(*c.DevMock, n)
		return ReactionPostResult{OK: true, Data: &data}, nil
	}
	status, body, err := c.do(ctx, http.MethodPost)
	if err != nil {
		return ReactionPostResult{}, err
	}
	parsed, err := DecodeReactionEnvelope(body)
	if err != nil {
		return ReactionPostResult{OK: false}, nil
	}
	if status < 200 || status > 299 {
		e := reactionStatusError(status)
		if parsed.Error != nil {
			e = *parsed.Error
		}
		return ReactionPostResult{OK: false, Error: &e, Data: parsed.Data}, nil
	}
	return parsed, nil
}

func reactionStatusError(code int) string {
	switch code {
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "disabled"
	case http.StatusTooManyRequests:
		return "rate_limited"
	}
	return "unavailable"
}
