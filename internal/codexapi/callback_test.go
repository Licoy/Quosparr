package codexapi

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func freePort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}

func get(t *testing.T, port uint16, path string) (int, string) {
	t.Helper()
	resp, err := http.Get("http://127.0.0.1:" + strconv.Itoa(int(port)) + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestCallbackServerCapturesRedirect(t *testing.T) {
	port := freePort(t)
	s := NewCallbackServer(port)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		u   string
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		u, err := s.WaitForCallback(context.Background(), 5*time.Second)
		if err != nil {
			done <- outcome{err: err}
			return
		}
		done <- outcome{u: u.String()}
	}()
	if status, _ := get(t, port, "/other"); status != 404 {
		t.Fatalf("other path status %d", status)
	}
	if status, body := get(t, port, CallbackPath+"?code=abc&state=st"); status != 200 || body == "" {
		t.Fatalf("callback %d", status)
	}
	select {
	case o := <-done:
		want := "http://localhost:" + strconv.Itoa(int(port)) + "/auth/callback?code=abc&state=st"
		if o.err != nil || o.u != want {
			t.Fatalf("%+v want %s", o, want)
		}
		if code, err := AuthorizationCode(o.u, "st"); err != nil || code != "abc" {
			t.Fatalf("%q %v", code, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no callback")
	}
}

func TestCallbackServerPortInUse(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := NewCallbackServer(uint16(l.Addr().(*net.TCPAddr).Port))
	if err := s.Start(); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("Start: %v", err)
	} else {
		var bind *net.OpError
		if !errors.As(err, &bind) {
			t.Fatal("Start discarded the underlying listener failure")
		}
	}
	if _, err := NewCallbackServer(uint16(l.Addr().(*net.TCPAddr).Port)).WaitForCallback(context.Background(), time.Second); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("Wait: %v", err)
	}
}

func TestCallbackServerTimeoutCancelAndContext(t *testing.T) {
	s := NewCallbackServer(freePort(t))
	if _, err := s.WaitForCallback(context.Background(), 50*time.Millisecond); !errors.Is(err, ErrTimedOut) {
		t.Fatalf("timeout: %v", err)
	}

	s = NewCallbackServer(freePort(t))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	errc := make(chan error, 1)
	go func() { _, err := s.WaitForCallback(context.Background(), 5*time.Second); errc <- err }()
	time.Sleep(50 * time.Millisecond)
	s.Cancel()
	if err := <-errc; !errors.Is(err, ErrCancelled) {
		t.Fatalf("cancel: %v", err)
	}

	port := freePort(t)
	s = NewCallbackServer(port)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.WaitForCallback(ctx, 5*time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx: %v", err)
	}
	// The port is released after the wait ends.
	deadline := time.Now().Add(2 * time.Second)
	for {
		l, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(int(port)))
		if err == nil {
			l.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("port not released")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCallbackServerCancelBeforeStart(t *testing.T) {
	s := NewCallbackServer(freePort(t))
	s.Cancel()
	if _, err := s.WaitForCallback(context.Background(), time.Second); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v", err)
	}
}
