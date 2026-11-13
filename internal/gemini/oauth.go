package gemini

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Google OAuth endpoints and the Antigravity installed-app client.
const (
	AuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL     = "https://oauth2.googleapis.com/token"
	UserInfoURL  = "https://www.googleapis.com/oauth2/v2/userinfo"
	// OAuthClientID is the Antigravity desktop client.
	OAuthClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"

	callbackPath         = "/oauth-callback"
	defaultLoginLifetime = 5 * time.Minute
)

// OAuthScopes are the scopes the Antigravity clients request.
var OAuthScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
	"https://www.googleapis.com/auth/aicode",
}

// LoginSession holds ephemeral secrets; never persist or log it.
type LoginSession struct {
	AuthURL, RedirectURI, State, CodeVerifier string
	Port                                      uint16
	ExpiresAt                                 time.Time
}

func newLoginSession(port uint16, now time.Time, lifetime time.Duration) (LoginSession, error) {
	if lifetime <= 0 {
		lifetime = defaultLoginLifetime
	}
	secret := make([]byte, 64)
	if _, err := rand.Read(secret); err != nil {
		return LoginSession{}, ErrLoginExchange
	}
	s := LoginSession{
		State:        base64.RawURLEncoding.EncodeToString(secret[:32]),
		CodeVerifier: base64.RawURLEncoding.EncodeToString(secret[32:]),
		Port:         port,
		ExpiresAt:    now.Add(lifetime),
	}
	s.RedirectURI = fmt.Sprintf("http://localhost:%d%s", port, callbackPath)
	challenge := sha256.Sum256([]byte(s.CodeVerifier))
	query := url.Values{
		"client_id":              {OAuthClientID},
		"response_type":          {"code"},
		"redirect_uri":           {s.RedirectURI},
		"scope":                  {strings.Join(OAuthScopes, " ")},
		"access_type":            {"offline"},
		"prompt":                 {"consent"},
		"include_granted_scopes": {"true"},
		"state":                  {s.State},
		"code_challenge":         {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method":  {"S256"},
	}
	s.AuthURL = AuthorizeURL + "?" + query.Encode()
	return s, nil
}

// LoginCallbackText is the static page text of the callback.
type LoginCallbackText struct {
	Received        string
	Invalid         string
	AlreadyReceived string
}

// LoginCallbackServer listens on a random loopback port for one state-bound
// authorization code. It never stores credentials.
type LoginCallbackServer struct {
	session   LoginSession
	text      LoginCallbackText
	mu        sync.Mutex
	done      chan struct{}
	finished  bool
	code      string
	err       error
	listeners []net.Listener
	servers   []*http.Server
	timer     *time.Timer
}

// StartLoginCallback binds 127.0.0.1 on a random port (and ::1 on the same
// port when the system has IPv6), then builds the session for that port.
func StartLoginCallback(now time.Time, lifetime time.Duration, text ...LoginCallbackText) (*LoginCallbackServer, error) {
	messages := LoginCallbackText{
		Received:        "Authorization received; return to Quosparr to confirm the result.",
		Invalid:         "Invalid authorization callback",
		AlreadyReceived: "Authorization already received",
	}
	if len(text) > 0 {
		messages = text[0]
	}
	listeners, port, err := bindLoopback(net.Listen)
	if err != nil {
		return nil, err
	}
	session, err := newLoginSession(port, now, lifetime)
	if err != nil {
		closeListeners(listeners)
		return nil, err
	}
	s := &LoginCallbackServer{session: session, text: messages, done: make(chan struct{}), listeners: listeners}
	s.timer = time.AfterFunc(time.Until(session.ExpiresAt), func() { s.finish("", ErrLoginTimedOut) })
	for _, listener := range listeners {
		server := &http.Server{
			Handler: http.HandlerFunc(s.callback), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second,
			WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10,
		}
		s.servers = append(s.servers, server)
		go func() {
			if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
				s.finish("", ErrLoginCallback)
			}
		}()
	}
	return s, nil
}

func bindLoopback(listen func(string, string) (net.Listener, error)) ([]net.Listener, uint16, error) {
	var lastErr error
	for range 5 {
		v4, err := listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, 0, errors.Join(ErrLoginPortInUse, err)
		}
		port := v4.Addr().(*net.TCPAddr).Port
		v6, err := listen("tcp", net.JoinHostPort("::1", strconv.Itoa(port)))
		if err == nil {
			return []net.Listener{v4, v6}, uint16(port), nil
		}
		if errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) || errors.Is(err, syscall.EPROTONOSUPPORT) {
			return []net.Listener{v4}, uint16(port), nil
		}
		v4.Close()
		lastErr = err
	}
	return nil, 0, errors.Join(ErrLoginPortInUse, lastErr)
}

func closeListeners(listeners []net.Listener) {
	for _, l := range listeners {
		l.Close()
	}
}

// Session is the login session bound to this server's port.
func (s *LoginCallbackServer) Session() LoginSession { return s.session }

func (s *LoginCallbackServer) finish(code string, err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return false
	}
	if err == nil && !time.Now().Before(s.session.ExpiresAt) {
		code, err = "", ErrLoginTimedOut
	}
	s.finished, s.code, s.err = true, code, err
	if s.timer != nil {
		s.timer.Stop()
	}
	closeListeners(s.listeners)
	for _, server := range s.servers {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if server.Shutdown(ctx) != nil {
				server.Close()
			}
		}()
	}
	close(s.done)
	return true
}

// Wait returns the authorization code, or why there is none.
func (s *LoginCallbackServer) Wait(ctx context.Context) (string, error) {
	select {
	case <-s.done:
	case <-ctx.Done():
		s.finish("", ctx.Err())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.code, s.err
}

// Cancel stops waiting.
func (s *LoginCallbackServer) Cancel() { s.finish("", context.Canceled) }

func (s *LoginCallbackServer) callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != callbackPath || !s.validHost(r.Host) {
		s.writePage(w, http.StatusBadRequest, s.text.Invalid)
		return
	}
	code, err := s.parseQuery(r.URL.Query())
	if err != nil && !errors.Is(err, ErrLoginDenied) {
		s.writePage(w, http.StatusBadRequest, s.text.Invalid)
		return
	}
	if !s.finish(code, err) {
		s.writePage(w, http.StatusConflict, s.text.AlreadyReceived)
		return
	}
	s.writePage(w, http.StatusOK, s.text.Received)
}

func (s *LoginCallbackServer) writePage(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(status)
	fmt.Fprintf(w, "<!doctype html><meta charset=\"utf-8\"><title>Quosparr</title><p>%s</p>", html.EscapeString(text))
}

func (s *LoginCallbackServer) validHost(host string) bool {
	h, p, err := net.SplitHostPort(host)
	return err == nil && p == strconv.Itoa(int(s.session.Port)) && (h == "localhost" || h == "127.0.0.1" || h == "::1")
}

// Submit accepts a pasted callback: the full redirect URL, or its query
// string ("code=…&state=…"). A bare code is refused because it carries no
// state.
func (s *LoginCallbackServer) Submit(raw string) error {
	raw = trimSpace(raw)
	var query url.Values
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "http" || u.User != nil || !s.validHost(u.Host) || u.Path != callbackPath {
			return ErrLoginCallback
		}
		query = u.Query()
	} else {
		parsed, err := url.ParseQuery(strings.TrimPrefix(raw, "?"))
		if err != nil {
			return ErrLoginCallback
		}
		query = parsed
	}
	code, err := s.parseQuery(query)
	if err != nil && !errors.Is(err, ErrLoginDenied) {
		return err
	}
	if !time.Now().Before(s.session.ExpiresAt) {
		s.finish("", ErrLoginTimedOut)
		return ErrLoginTimedOut
	}
	if !s.finish(code, err) {
		return ErrLoginCallback
	}
	return err
}

func (s *LoginCallbackServer) parseQuery(query url.Values) (string, error) {
	if len(query["state"]) != 1 || s.session.State == "" || query.Get("state") != s.session.State {
		return "", ErrLoginCallback
	}
	if len(query["error"]) > 0 {
		if len(query["code"]) != 0 || query.Get("error") == "" {
			return "", ErrLoginCallback
		}
		return "", ErrLoginDenied
	}
	code := query.Get("code")
	if len(query["code"]) != 1 || trimSpace(code) == "" || strings.ContainsAny(code, "\r\n\t ") {
		return "", ErrLoginCallback
	}
	return code, nil
}
