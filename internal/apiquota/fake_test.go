package apiquota

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type reply struct {
	status int
	body   string
}

type seen struct {
	path, query, auth, apiKey, version, accept string
}

// fakeSite serves canned replies per URL path (404 for unknown paths) and
// records every request.
type fakeSite struct {
	mu       sync.Mutex
	routes   map[string]reply
	requests []seen
	srv      *httptest.Server
}

func newFakeSite(t *testing.T, routes map[string]reply) *fakeSite {
	t.Helper()
	f := &fakeSite{routes: routes}
	f.srv = httptest.NewTLSServer(f)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, seen{
		path:    r.URL.Path,
		query:   r.URL.RawQuery,
		auth:    r.Header.Get("Authorization"),
		apiKey:  r.Header.Get("x-api-key"),
		version: r.Header.Get("anthropic-version"),
		accept:  r.Header.Get("Accept"),
	})
	rep, ok := f.routes[r.URL.Path]
	f.mu.Unlock()
	if !ok {
		rep = reply{status: http.StatusNotFound}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rep.status)
	_, _ = w.Write([]byte(rep.body))
}

func (f *fakeSite) url() string { return f.srv.URL }

func (f *fakeSite) client() Doer { return f.srv.Client() }

func (f *fakeSite) all() []seen {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seen(nil), f.requests...)
}

func (f *fakeSite) find(path string) (seen, bool) {
	for _, r := range f.all() {
		if r.path == path {
			return r, true
		}
	}
	return seen{}, false
}

func (f *fakeSite) paths() string {
	var b strings.Builder
	for _, r := range f.all() {
		b.WriteString(r.path + "\n")
	}
	return b.String()
}

// doerFunc adapts a function to Doer.
type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
