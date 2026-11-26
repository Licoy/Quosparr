package netx

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// ErrHTTPSRequired means an authenticated endpoint is not an HTTPS URL.
var ErrHTTPSRequired = errors.New("authenticated requests require HTTPS")

// DoAuthenticated sends an authenticated request over HTTPS. Redirects may
// only stay on the original origin, including redirects that replay a body.
// The caller's client is not modified. A rejected redirect is returned as the
// original response, so its destination does not appear in transport errors.
func DoAuthenticated(client *http.Client, req *http.Request) (*http.Response, error) {
	if !isHTTPS(req.URL) {
		return nil, ErrHTTPSRequired
	}
	origin := *req.URL
	checked := *client
	checked.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !sameHTTPSOrigin(&origin, next.URL) {
			return http.ErrUseLastResponse
		}
		if client.CheckRedirect != nil {
			if err := client.CheckRedirect(next, via); err != nil {
				return err
			}
			if !sameHTTPSOrigin(&origin, next.URL) {
				return http.ErrUseLastResponse
			}
		}
		return nil
	}
	return checked.Do(req)
}

func isHTTPS(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Opaque == ""
}

func sameHTTPSOrigin(a, b *url.URL) bool {
	return isHTTPS(a) && isHTTPS(b) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) && httpsPort(a) == httpsPort(b)
}

func httpsPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	return "443"
}
