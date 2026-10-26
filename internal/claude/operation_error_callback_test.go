package claude

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
)

type callbackBindListener struct {
	net.Listener
	closed bool
}

func (l *callbackBindListener) Close() error { l.closed = true; return nil }
func TestCallbackBindPreservesPermissionAndCancellation(t *testing.T) {
	for _, failOn := range []int{1, 2} {
		for _, cause := range []error{syscall.EACCES, context.Canceled, syscall.EADDRINUSE} {
			server := NewLoginCallbackServer(LoginSession{})
			bound := &callbackBindListener{}
			calls := 0
			server.listen = func(string, string) (net.Listener, error) {
				calls++
				if calls == failOn {
					return nil, &net.OpError{Op: "listen", Err: &os.PathError{Op: "bind", Path: "/private-secret", Err: cause}}
				}
				return bound, nil
			}
			err := server.bindLoopback()
			kind := KindAddressInUse
			if errors.Is(cause, syscall.EACCES) {
				kind = KindPermission
			}
			if errors.Is(cause, context.Canceled) {
				kind = KindCanceled
			}
			requireOperation(t, err, StageOAuthCallback, kind)
			if !errors.Is(err, cause) || !errors.Is(err, ErrLoginPortInUse) {
				t.Fatal("listen cause lost")
			}
			if failOn == 2 && !bound.closed {
				t.Fatal("previous IPv4 binding leaked")
			}
			if len(server.listeners) != 0 {
				t.Fatal("failed callback retained listeners")
			}
		}
	}
}
func TestCallbackBindPreservesUnsupportedIPv6Policy(t *testing.T) {
	for _, cause := range []error{syscall.EAFNOSUPPORT, syscall.EADDRNOTAVAIL, syscall.EPROTONOSUPPORT} {
		server := NewLoginCallbackServer(LoginSession{})
		bound := &callbackBindListener{}
		calls := 0
		server.listen = func(string, string) (net.Listener, error) {
			calls++
			if calls == 1 {
				return bound, nil
			}
			return nil, cause
		}
		if err := server.bindLoopback(); err != nil {
			t.Fatal(err)
		}
		if bound.closed || len(server.listeners) != 1 {
			t.Fatal("IPv4 availability changed")
		}
	}
}
