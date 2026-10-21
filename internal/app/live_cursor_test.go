package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/cursor"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

func cursorTestBridge(t *testing.T) (*cursorBridge, *atomic.Int32) {
	t.Helper()
	p := store.DefaultPreferences()
	p.CursorMonitoringEnabled = true
	p.Language = store.LanguageEnglish
	b := newCursorBridge(t.TempDir(), p, nil)
	b.now = func() time.Time { return time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC) }
	b.read = func(context.Context) (cursor.Auth, error) {
		return cursor.Auth{Token: "synthetic-account-a", Email: "a@example.invalid"}, nil
	}
	calls := &atomic.Int32{}
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		calls.Add(1)
		pct := 25.0
		return cursor.Snapshot{Plan: "test", UpdatedAt: b.now(), Pools: []cursor.Pool{{Kind: "auto", UsedPercent: &pct}}}, nil
	}
	return b, calls
}

func TestCursorMonitoringRequiresOptInAndDisableClearsState(t *testing.T) {
	b, calls := cursorTestBridge(t)
	p := store.DefaultPreferences()
	b.setPreferences(p, nil)
	reads := 0
	read := b.read
	b.read = func(ctx context.Context) (cursor.Auth, error) { reads++; return read(ctx) }
	b.Refresh(context.Background(), true)
	if reads != 0 || calls.Load() != 0 || b.State(b.now()).Availability != runway.CursorDisabled {
		t.Fatal("disabled monitoring touched credentials")
	}
	p.CursorMonitoringEnabled = true
	b.setPreferences(p, nil)
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if st := b.State(b.now()); len(st.Meters) != 1 || st.Meters[0].RemainingPercent != 75 || st.Email == "" {
		t.Fatal("quota did not reach view")
	}
	p.CursorMonitoringEnabled = false
	b.setPreferences(p, nil)
	st := b.State(b.now())
	if st.IdentityID != "" || st.Email != "" || len(st.Meters) != 0 || b.quota != nil {
		t.Fatal("disabled monitoring retained account state")
	}
}

func TestCursorStaleFailureRateLimitAndReauthentication(t *testing.T) {
	b, calls := cursorTestBridge(t)
	if err := b.Refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	b.Refresh(context.Background(), false)
	if calls.Load() != 1 {
		t.Fatal("normal refresh was not throttled")
	}
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		calls.Add(1)
		return cursor.Snapshot{}, &cursor.RateLimitedError{RetryAfter: time.Minute}
	}
	b.Refresh(context.Background(), true)
	st := b.State(b.now())
	if !st.Stale || st.Availability != runway.CursorFailed || len(st.Meters) != 1 || st.LastError == "" {
		t.Fatal("failed reading was marked fresh")
	}
	b.Refresh(context.Background(), true)
	if calls.Load() != 2 {
		t.Fatal("manual refresh bypassed Retry-After")
	}
	b.now = func() time.Time { return time.Date(2026, 10, 21, 8, 2, 0, 0, time.UTC) }
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		return cursor.Snapshot{}, cursor.ErrUnauthorized
	}
	b.Refresh(context.Background(), true)
	st = b.State(b.now())
	if st.Availability != runway.CursorReauthRequired || len(st.Meters) != 0 {
		t.Fatal("expired authentication kept quota")
	}
}

func TestCursorDropsResponseAfterExternalSwitchOrLogout(t *testing.T) {
	for _, logout := range []bool{false, true} {
		b, calls := cursorTestBridge(t)
		if err := b.Refresh(context.Background(), true); err != nil {
			t.Fatal(err)
		}
		b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
			calls.Add(1)
			b.read = func(context.Context) (cursor.Auth, error) {
				if logout {
					return cursor.Auth{}, cursor.ErrNotLoggedIn
				}
				return cursor.Auth{Token: "synthetic-account-b", Email: "b@example.invalid"}, nil
			}
			pct := 99.0
			return cursor.Snapshot{UpdatedAt: b.now(), Pools: []cursor.Pool{{Kind: "auto", UsedPercent: &pct}}}, nil
		}
		b.Refresh(context.Background(), true)
		st := b.State(b.now())
		if len(st.Meters) == 1 && st.Meters[0].RemainingPercent == 1 {
			t.Fatal("old account response was published")
		}
		if logout {
			if st.IdentityID != "" || st.Email != "" || len(st.Meters) != 0 || st.Availability != runway.CursorNotLoggedIn {
				t.Fatal("logout kept account state")
			}
			continue
		}
		// The switched account is fetched by the next scheduled refresh.
		b.Refresh(context.Background(), false)
		if st := b.State(b.now()); calls.Load() != 3 || st.Email != "b@example.invalid" {
			t.Fatal("switched account was not refetched")
		}
	}
}

func TestCursorTokenRotationKeepsAccountResponse(t *testing.T) {
	b, _ := cursorTestBridge(t)
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		b.read = func(context.Context) (cursor.Auth, error) {
			return cursor.Auth{Token: "synthetic-account-a-rotated", Email: "a@example.invalid"}, nil
		}
		pct := 40.0
		return cursor.Snapshot{UpdatedAt: b.now(), Pools: []cursor.Pool{{Kind: "auto", UsedPercent: &pct}}}, nil
	}
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if st := b.State(b.now()); len(st.Meters) != 1 || st.Meters[0].RemainingPercent != 60 || st.LastError != "" {
		t.Fatal("token rotation discarded the response")
	}
}

func TestCursorDisableCancelsAndDiscardsInFlightRequest(t *testing.T) {
	b, _ := cursorTestBridge(t)
	started, done := make(chan struct{}), make(chan struct{})
	b.fetch = func(ctx context.Context, _ string) (cursor.Snapshot, error) {
		close(started)
		<-ctx.Done()
		return cursor.Snapshot{}, ctx.Err()
	}
	go func() { defer close(done); b.Refresh(context.Background(), true) }()
	<-started
	b.setPreferences(store.DefaultPreferences(), nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
	if st := b.State(b.now()); st.Availability != runway.CursorDisabled || st.Refreshing || st.IdentityID != "" {
		t.Fatal("disabled request published state")
	}
}

func TestCursorUnknownQuotaDoesNotManufactureMeter(t *testing.T) {
	var st runway.CursorState
	used, limit := int64(300), int64(0)
	cursorQuotaView(&st, cursor.Snapshot{Pools: []cursor.Pool{{Kind: "auto", UsedCents: &used, LimitCents: &limit}}}, l10n.New(l10n.English), time.Now())
	if len(st.Meters) != 0 || len(st.Lines) != 1 {
		t.Fatal("unknown percentage became a meter")
	}
	if _, err := (&liveApp{}).queryDetailCost(context.Background(), store.ProviderCursor, CostQuery{}); !errors.Is(err, errDetailUnavailable) {
		t.Fatal("Cursor cost fell through to Codex")
	}
}

func TestCursorKeepsEstimatedCapWhenPoolCannotInvert(t *testing.T) {
	b, _ := cursorTestBridge(t)
	pct, used, limit := 20.0, int64(2000), int64(10000)
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		return cursor.Snapshot{UpdatedAt: b.now(), Pools: []cursor.Pool{{Kind: "api", UsedPercent: &pct, UsedCents: &used, LimitCents: &limit, LimitEstimated: true}}}, nil
	}
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	full, spent := 100.0, int64(10500)
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		return cursor.Snapshot{UpdatedAt: b.now(), Pools: []cursor.Pool{{Kind: "api", UsedPercent: &full, UsedCents: &spent}}}, nil
	}
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	st := b.State(b.now())
	if len(st.Lines) != 1 || st.Lines[0].Value != "$105.00 / $100.00 · estimated" {
		t.Fatalf("lines = %+v", st.Lines)
	}
}

func TestCursorTimeoutKeepsStaleReadingWithoutStoreError(t *testing.T) {
	b, _ := cursorTestBridge(t)
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	b.fetch = func(ctx context.Context, _ string) (cursor.Snapshot, error) {
		<-ctx.Done()
		return cursor.Snapshot{}, ctx.Err()
	}
	b.read = func(ctx context.Context) (cursor.Auth, error) {
		if ctx.Err() != nil {
			return cursor.Auth{}, cursor.ErrCredentials
		}
		return cursor.Auth{Token: "synthetic-account-a", Email: "a@example.invalid"}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	b.Refresh(ctx, true)
	st := b.State(b.now())
	if !st.Stale || len(st.Meters) != 1 || st.Email == "" || errors.Is(b.err, cursor.ErrCredentials) {
		t.Fatal("timeout dropped the reading or reported a store error")
	}
}

func TestCursorTransientReadErrorKeepsRetryAfter(t *testing.T) {
	b, calls := cursorTestBridge(t)
	if err := b.Refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	b.fetch = func(context.Context, string) (cursor.Snapshot, error) {
		calls.Add(1)
		return cursor.Snapshot{}, &cursor.RateLimitedError{RetryAfter: time.Hour}
	}
	b.Refresh(context.Background(), true)
	read := b.read
	b.read = func(context.Context) (cursor.Auth, error) { return cursor.Auth{}, cursor.ErrCredentials }
	b.Refresh(context.Background(), true)
	if st := b.State(b.now()); !st.Stale || len(st.Meters) != 1 || st.Email == "" {
		t.Fatal("transient store error dropped the account reading")
	}
	b.read = read
	b.Refresh(context.Background(), true)
	if calls.Load() != 2 {
		t.Fatal("store error reset Retry-After")
	}
}
