package codexapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// jwt builds an unsigned token with the payload, for claims decoding tests.
func jwt(t testing.TB, payload map[string]any) string {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(body) + "."
}

var fixedNow = time.Unix(1_782_710_000, 0).UTC()

// recorder is an httptest server that records requests and answers from a
// handler.
type recorder struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
}

type recorded struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Form   url.Values
	Body   string
}

func newRecorder(t *testing.T, handler func(r recorded) (int, string)) *recorder {
	t.Helper()
	rec := &recorder{}
	rec.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		item := recorded{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: string(body)}
		item.Form, _ = url.ParseQuery(string(body))
		rec.mu.Lock()
		rec.requests = append(rec.requests, item)
		rec.mu.Unlock()
		status, payload := handler(item)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(rec.Close)
	return rec
}

func (r *recorder) all() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recorded(nil), r.requests...)
}

func (r *recorder) last(t *testing.T) recorded {
	t.Helper()
	all := r.all()
	if len(all) == 0 {
		t.Fatal("no request recorded")
	}
	return all[len(all)-1]
}
