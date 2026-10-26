package claudedesktop

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeDesktopChild struct{ done chan error }

func (c fakeDesktopChild) Wait() error { return <-c.done }

func TestDesktopStartupObservation(t *testing.T) {
	for _, mode := range []string{"alive", "early-success-exit", "early-error-exit", "cannot-start", "cancel-before", "cancel-after"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			defer close(done)
			calls := 0
			start := func(exe string, env []string) (desktopChild, error) {
				calls++
				if exe != "/fixture/Claude" || len(env) != 1 || env[0] != "PATH=/fixture" {
					t.Fatal("wrong child request")
				}
				switch mode {
				case "cannot-start":
					return nil, errors.New("fixture failure")
				case "early-success-exit":
					done <- nil
				case "early-error-exit":
					done <- errors.New("fixture failure")
				case "cancel-after":
					cancel()
				}
				return fakeDesktopChild{done}, nil
			}
			if mode == "cancel-before" {
				cancel()
			}
			err := observeDesktopLaunch(ctx, "/fixture/Claude", []string{"PATH=/fixture"}, start, 5*time.Millisecond)
			if (err == nil) != (mode == "alive") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if mode == "cancel-before" && calls != 0 {
				t.Fatal("started cancelled request")
			}
		})
	}
}
