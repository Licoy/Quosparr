package runway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
)

func TestResolveCycleRangeAPIKeyUsesCalendarMonths(t *testing.T) {
	f := newFake(t)
	f.setAuth(codex.APIKeyAuth(plainKey), nil)
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	s, _ := newService(t, f, func(o *Options) { o.Now = func() time.Time { return now } })

	current, err := s.ResolveCycleRange(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := s.ResolveCycleRange(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !current.Window.Start.Equal(want) || !current.Window.End.Equal(now) {
		t.Errorf("current = %v - %v", current.Window.Start, current.Window.End)
	}
	if start, end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !previous.Window.Start.Equal(start) || !previous.Window.End.Equal(end) {
		t.Errorf("previous = %v - %v, want %v - %v", previous.Window.Start, previous.Window.End, start, end)
	}
	if f.count("quota") != 0 || f.count("scan") != 0 {
		t.Errorf("range resolution fetched quota or scanned logs: quota=%d scan=%d", f.count("quota"), f.count("scan"))
	}
}

func TestResolveCycleRangeRejectsMissingSubscriptionCycle(t *testing.T) {
	f := newFake(t)
	f.quotaFn = func(context.Context, codexapi.Auth) (codexapi.QuotaSnapshot, error) {
		return codexapi.QuotaSnapshot{}, nil
	}
	s, _ := newService(t, f, nil)
	if _, err := s.ResolveCycleRange(context.Background(), false); !errors.Is(err, ErrCycleUnavailable) {
		t.Fatalf("error = %v, want ErrCycleUnavailable", err)
	}
	if f.count("scan") != 0 {
		t.Errorf("range resolution scanned local costs %d times", f.count("scan"))
	}
}
