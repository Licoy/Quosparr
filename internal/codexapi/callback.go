package codexapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// Callback server errors (OAuthCallbackServer.ServerError).
var (
	ErrPortInUse = errors.New("OAuth callback port is in use")
	ErrTimedOut  = errors.New("OAuth callback timed out")
	ErrCancelled = errors.New("OAuth callback cancelled")
)

const defaultCallbackTimeout = 5 * time.Minute

const callbackPage = `<!doctype html><html><body style="font-family:system-ui;padding:2rem">
<h2>Sign-in complete</h2>
<p>You can close this window and return to Quosparr.</p>
</body></html>`

// CallbackServer is a minimal localhost HTTP listener that captures a single
// OAuth redirect on CallbackPath. It listens on the loopback interfaces only
// (IPv4, and IPv6 when available) because the redirect URI uses "localhost".
type CallbackServer struct {
	port uint16

	mu        sync.Mutex
	listeners []net.Listener
	result    chan callbackResult
	done      chan struct{}
	finished  bool
}

type callbackResult struct {
	url *url.URL
	err error
}

// NewCallbackServer creates a server for port (PreferredCallbackPort if 0).
func NewCallbackServer(port uint16) *CallbackServer {
	if port == 0 {
		port = PreferredCallbackPort
	}
	return &CallbackServer{port: port, result: make(chan callbackResult, 1), done: make(chan struct{})}
}

// Start binds the port so the caller can report ErrPortInUse before opening the
// browser. WaitForCallback calls it when needed; calling it twice is a no-op.
func (s *CallbackServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return ErrCancelled
	}
	if s.listeners != nil {
		return nil
	}
	addr := func(host string) string { return net.JoinHostPort(host, strconv.Itoa(int(s.port))) }
	v4, err := net.Listen("tcp4", addr("127.0.0.1"))
	if err != nil {
		return &CallbackListenError{Cause: err}
	}
	s.listeners = []net.Listener{v4}
	if v6, err := net.Listen("tcp6", addr("::1")); err == nil {
		s.listeners = append(s.listeners, v6)
	}
	handler := http.HandlerFunc(s.handle)
	for _, l := range s.listeners {
		srv := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      10 * time.Second,
		}
		go func() { _ = srv.Serve(l) }()
		go func() {
			<-s.done
			_ = srv.Close()
		}()
	}
	return nil
}

// WaitForCallback blocks until the browser reaches the callback URL and returns
// that URL (with the host the redirect URI names). A zero timeout means five
// minutes. Cancelling ctx or calling Cancel stops the wait and releases the
// port.
func (s *CallbackServer) WaitForCallback(ctx context.Context, timeout time.Duration) (*url.URL, error) {
	// A fast browser redirect can arrive after Start and before this wait.
	select {
	case r := <-s.result:
		return r.url, r.err
	default:
	}
	if timeout <= 0 {
		timeout = defaultCallbackTimeout
	}
	if err := s.Start(); err != nil {
		s.finish(callbackResult{err: err})
		r := <-s.result
		return r.url, r.err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-s.result:
		return r.url, r.err
	case <-timer.C:
		s.finish(callbackResult{err: ErrTimedOut})
	case <-ctx.Done():
		s.finish(callbackResult{err: ctx.Err()})
	}
	r := <-s.result
	return r.url, r.err
}

// Cancel ends a running or pending wait with ErrCancelled.
func (s *CallbackServer) Cancel() { s.finish(callbackResult{err: ErrCancelled}) }

func (s *CallbackServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != CallbackPath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close")
	_, _ = fmt.Fprint(w, callbackPage)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	target := &url.URL{Scheme: "http", Host: "localhost:" + strconv.Itoa(int(s.port)), Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	s.finish(callbackResult{url: target})
}

// finish delivers the first result, closes the listeners and ignores any later
// call.
func (s *CallbackServer) finish(r callbackResult) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	listeners := s.listeners
	s.mu.Unlock()
	s.result <- r
	// Give the response written by handle a moment to leave before the
	// connection is torn down.
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(s.done)
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
}
