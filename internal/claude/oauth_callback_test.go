package claude

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestOAuthLoginAutomaticCallback(t *testing.T) {
	s := testLoginSession(t)
	server := NewLoginCallbackServer(s)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Cancel()
	resp, err := http.Get(s.RedirectURI + "?code=automatic%23" + s.State + "&state=" + s.State)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Authorization received") || strings.Contains(string(body), "saved") {
		t.Fatal("invalid callback response")
	}
	code, err := server.Wait(context.Background())
	if err != nil || code != "automatic" {
		t.Fatal("automatic callback failed")
	}
}

func TestOAuthLoginIPv6Availability(t *testing.T) {
	for _, cause := range []error{syscall.EAFNOSUPPORT, syscall.EADDRNOTAVAIL, syscall.EADDRINUSE, errors.New("unexpected listen failure")} {
		s := testLoginSession(t)
		server := NewLoginCallbackServer(s)
		server.listen = func(network, address string) (net.Listener, error) {
			if strings.HasPrefix(address, "[::1]") {
				return nil, &net.OpError{Op: "listen", Err: cause}
			}
			return net.Listen(network, address)
		}
		err := server.Start()
		if errors.Is(cause, syscall.EAFNOSUPPORT) || errors.Is(cause, syscall.EADDRNOTAVAIL) {
			if err != nil {
				t.Fatal("unavailable IPv6 prevented IPv4 callback")
			}
			server.Cancel()
		} else if !errors.Is(err, ErrLoginPortInUse) {
			t.Fatal("unexpected bind error hidden")
		}
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", s.Port))
		if err != nil {
			t.Fatal("IPv4 listener leaked")
		}
		listener.Close()
	}
}

func TestOAuthLoginCancelClosesAcceptedConnection(t *testing.T) {
	s := testLoginSession(t)
	server := NewLoginCallbackServer(s)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp4", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// A full request confirms this connection was accepted and is now reusable.
	fmt.Fprintf(conn, "GET /invalid HTTP/1.1\r\nHost: localhost:%d\r\n\r\n", s.Port)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("context cancellation lost")
	}
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("accepted connection remains open")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("accepted connection leaked until deadline")
	}
}

func TestOAuthLoginExchangeTimeout(t *testing.T) {
	s := testLoginSession(t)
	s.ExpiresAt = time.Now().Add(20 * time.Millisecond)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
	defer server.Close()
	_, err := (OAuthClient{HTTP: server.Client(), TokenURL: server.URL}).Exchange(context.Background(), "private-code", s, time.Now())
	if !errors.Is(err, ErrLoginTimedOut) {
		t.Fatalf("wrong exchange timeout %v", err)
	}
}

func TestOAuthLoginLocalizedCallbackEscapesHTML(t *testing.T) {
	s := testLoginSession(t)
	text := LoginCallbackText{Received: `已收到授权 <script>alert("x")</script> & 返回应用`, Invalid: `无效授权 <b>请求</b>`, AlreadyReceived: `已收到授权 <img src=x>`}
	server := NewLoginCallbackServer(s, text)
	for _, tc := range []struct {
		path, want string
		status     int
	}{
		{"/wrong?untrusted=private-input", text.Invalid, http.StatusBadRequest},
		{"/callback?code=private-input&state=" + s.State, text.Received, http.StatusOK},
		{"/callback?code=private-input&state=" + s.State, text.AlreadyReceived, http.StatusConflict},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, s.RedirectURI[:strings.LastIndex(s.RedirectURI, "/")]+tc.path, nil)
		server.callback(response, request)
		body := response.Body.String()
		if response.Code != tc.status || !strings.Contains(body, html.EscapeString(tc.want)) {
			t.Fatal("localized callback text missing")
		}
		if strings.Contains(body, "<script>") || strings.Contains(body, "<b>") || strings.Contains(body, "<img") || strings.Contains(body, "private-input") {
			t.Fatal("unescaped or user-controlled callback content")
		}
	}
}
