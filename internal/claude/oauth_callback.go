package claude

import (
	"context"
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

// LoginCallbackText is static UI text, selected before the callback starts.
type LoginCallbackText struct {
	Received        string
	Invalid         string
	AlreadyReceived string
}

// LoginCallbackServer accepts one state-bound result; it never stores credentials.
type LoginCallbackServer struct {
	session           LoginSession
	text              LoginCallbackText
	mu                sync.Mutex
	done              chan struct{}
	finished, started bool
	code              string
	err               error
	listeners         []net.Listener
	timer             *time.Timer
	servers           []*http.Server
	listen            func(string, string) (net.Listener, error)
}

func NewLoginCallbackServer(session LoginSession, text ...LoginCallbackText) *LoginCallbackServer {
	messages := LoginCallbackText{
		Received:        "Authorization received; return to Quosparr to confirm the result.",
		Invalid:         "Invalid authorization callback",
		AlreadyReceived: "Authorization already received",
	}
	if len(text) > 0 {
		messages = text[0]
	}
	return &LoginCallbackServer{session: session, text: messages, done: make(chan struct{})}
}

// Start binds loopback before opening a browser; systems without IPv6 use IPv4.
func (s *LoginCallbackServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.started {
		return ErrLoginCallback
	}
	if !time.Now().Before(s.session.ExpiresAt) {
		return ErrLoginTimedOut
	}
	if err := s.bindLoopback(); err != nil {
		return err
	}
	s.started = true
	s.timer = time.AfterFunc(time.Until(s.session.ExpiresAt), func() { s.finish("", ErrLoginTimedOut) })
	for _, listener := range s.listeners {
		server := &http.Server{Handler: http.HandlerFunc(s.callback), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
		s.servers = append(s.servers, server)
		go func() {
			err := server.Serve(listener)
			if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, http.ErrServerClosed) {
				s.finish("", ErrLoginCallback)
			}
		}()
	}
	return nil
}

func (s *LoginCallbackServer) bindLoopback() error {
	for _, host := range []string{"127.0.0.1", "::1"} {
		listen := s.listen
		if listen == nil {
			listen = net.Listen
		}
		listener, err := listen("tcp", net.JoinHostPort(host, strconv.Itoa(int(s.session.Port))))
		if err != nil {
			if host == "::1" && (errors.Is(err, syscall.EAFNOSUPPORT) || errors.Is(err, syscall.EADDRNOTAVAIL) || errors.Is(err, syscall.EPROTONOSUPPORT)) {
				continue
			}
			for _, bound := range s.listeners {
				bound.Close()
			}
			s.listeners = nil
			return operationError(StageOAuthCallback, errors.Join(ErrLoginPortInUse, err))
		}
		s.listeners = append(s.listeners, listener)
	}
	return nil
}

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
	for _, listener := range s.listeners {
		listener.Close()
	}
	for _, server := range s.servers {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrLoginTimedOut) {
			server.Close()
		} else {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if server.Shutdown(ctx) != nil {
					server.Close()
				}
			}()
		}
	}
	close(s.done)
	return true
}

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

func (s *LoginCallbackServer) Cancel() { s.finish("", context.Canceled) }

func (s *LoginCallbackServer) callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/callback" || !s.validHost(r.Host) {
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

// Submit accepts the complete callback URL or code#state, never a bare code.
func (s *LoginCallbackServer) Submit(raw string) error {
	raw = strings.TrimSpace(raw)
	var code string
	var err error
	if strings.Contains(raw, "://") {
		u, parseErr := url.Parse(raw)
		if parseErr != nil || u.Scheme != "http" || u.User != nil || u.Fragment != "" || !s.validHost(u.Host) || u.Path != "/callback" {
			return ErrLoginCallback
		}
		code, err = s.parseQuery(u.Query())
	} else {
		code, err = s.parseCode(raw, true)
	}
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
	if len(query["state"]) != 1 || query.Get("state") != s.session.State || s.session.State == "" {
		return "", ErrLoginCallback
	}
	if len(query["error"]) > 0 {
		if len(query["error"]) != 1 || query.Get("error") == "" || len(query["code"]) != 0 {
			return "", ErrLoginCallback
		}
		return "", ErrLoginDenied
	}
	if len(query["code"]) != 1 {
		return "", ErrLoginCallback
	}
	return s.parseCode(query.Get("code"), false)
}

func (s *LoginCallbackServer) parseCode(raw string, requireState bool) (string, error) {
	parts := strings.Split(raw, "#")
	if len(parts) > 2 || strings.TrimSpace(parts[0]) == "" || (requireState && len(parts) != 2) {
		return "", ErrLoginCallback
	}
	if len(parts) == 2 && (parts[1] != s.session.State || s.session.State == "") {
		return "", ErrLoginCallback
	}
	if strings.ContainsAny(parts[0], "\r\n\t ") {
		return "", ErrLoginCallback
	}
	return parts[0], nil
}
