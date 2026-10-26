package pricing

import (
	"context"
	"errors"
	"github.com/licoy/quosparr/internal/netx"
	"io"
	"net/http"
	"time"
)

const MaxResponseBytes = 16 << 20

var ErrResponseTooLarge = errors.New("pricing: response exceeds 16 MiB")

type Response struct {
	StatusCode int
	Body       []byte
	ETag       string
}
type Fetcher func(context.Context, string) (Response, error)

// Fetch resolves the saved proxy at request start, without a direct fallback.
func Fetch(ctx context.Context, etag string) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client, err := netx.ClientFor(ctx, netx.WithoutCookies)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, DefaultSourceURL, nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Quosparr/1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return fetchResponse(client, req)
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func fetchResponse(client httpDoer, req *http.Request) (Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return Response{}, err
	}
	if len(body) > MaxResponseBytes {
		return Response{}, ErrResponseTooLarge
	}
	return Response{resp.StatusCode, body, resp.Header.Get("ETag")}, nil
}
