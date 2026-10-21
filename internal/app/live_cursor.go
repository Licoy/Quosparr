package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/cursor"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

type cursorBridge struct {
	mu                    sync.Mutex
	gate                  chan struct{}
	enabled, refreshing   bool
	generation            uint64
	cancel                context.CancelFunc
	onChange              func()
	lc                    l10n.Localizer
	identity, email, plan string
	quota                 *cursor.Snapshot
	caps                  map[string]int64 // last estimated cap per pool, this account only
	err                   error
	nextPoll, retryAt     time.Time
	read                  func(context.Context) (cursor.Auth, error)
	fetch                 func(context.Context, string) (cursor.Snapshot, error)
	now                   func() time.Time
}

func newCursorBridge(home string, prefs store.Preferences, langs []string) *cursorBridge {
	dir := cursor.UserDataDir(runtime.GOOS, home, os.Getenv)
	return &cursorBridge{
		gate: make(chan struct{}, 1), enabled: prefs.CursorMonitoringEnabled,
		lc: l10n.ForPreference(string(prefs.Language), langs), now: time.Now,
		read:  func(ctx context.Context) (cursor.Auth, error) { return cursor.ReadAuth(ctx, dir) },
		fetch: (cursor.Client{}).Fetch,
	}
}

func (b *cursorBridge) SetOnChange(fn func()) { b.mu.Lock(); b.onChange = fn; b.mu.Unlock() }
func (b *cursorBridge) notify() {
	b.mu.Lock()
	fn := b.onChange
	b.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (b *cursorBridge) setPreferences(p store.Preferences, langs []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lc = l10n.ForPreference(string(p.Language), langs)
	if b.enabled == p.CursorMonitoringEnabled {
		return
	}
	b.enabled = p.CursorMonitoringEnabled
	b.generation++
	if b.cancel != nil {
		b.cancel()
	}
	b.clearLocked()
	b.refreshing = false
}

func (b *cursorBridge) clearLocked() {
	b.identity, b.email, b.plan = "", "", ""
	b.quota, b.err, b.caps = nil, nil, nil
	b.nextPoll, b.retryAt = time.Time{}, time.Time{}
}

func (b *cursorBridge) Refresh(ctx context.Context, force bool) error {
	select {
	case b.gate <- struct{}{}:
		defer func() { <-b.gate }()
	default:
		return nil
	}
	b.mu.Lock()
	if !b.enabled {
		b.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	gen := b.generation
	b.cancel, b.refreshing = cancel, true
	b.mu.Unlock()
	b.notify()
	defer func() {
		cancel()
		b.mu.Lock()
		if gen == b.generation {
			b.refreshing = false
			b.cancel = nil
		}
		b.mu.Unlock()
		b.notify()
	}()
	auth, err := b.read(ctx)
	if err != nil {
		b.authFailure(gen, err)
		return err
	}
	id, now := auth.ID(), b.now()
	b.mu.Lock()
	if gen != b.generation || !b.enabled {
		b.mu.Unlock()
		return nil
	}
	if id != b.identity {
		b.clearLocked()
	}
	b.identity, b.email, b.plan = id, auth.Email, auth.Plan
	if now.Before(b.retryAt) || !force && now.Before(b.nextPoll) {
		b.mu.Unlock()
		return nil
	}
	b.nextPoll = now.Add(5 * time.Minute)
	b.mu.Unlock()
	b.notify()
	quota, err := b.fetch(ctx, auth.Token)
	switched := false
	if ctx.Err() != nil {
		// An expired request keeps the last reading as stale; re-reading the
		// login with the same expired context would only report a store error.
		err = ctx.Err()
	} else {
		// Do not publish a response for an account that changed while in flight;
		// the next refresh picks up the new account.
		current, readErr := b.read(ctx)
		if readErr != nil {
			b.authFailure(gen, readErr)
			return readErr
		}
		switched = current.ID() != id
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if gen != b.generation || !b.enabled || ctx.Err() == context.Canceled {
		return nil
	}
	if switched {
		b.nextPoll = time.Time{}
		return nil
	}
	b.err = err
	if err == nil {
		b.keepCapsLocked(quota.Pools)
		b.quota = &quota
		b.retryAt = time.Time{}
		return nil
	}
	var limited *cursor.RateLimitedError
	if errors.As(err, &limited) {
		b.retryAt = b.now().Add(limited.RetryAfter)
	}
	if errors.Is(err, cursor.ErrUnauthorized) {
		b.quota = nil
	}
	return err
}

// keepCapsLocked remembers estimated caps and reuses them while a pool cannot
// be inverted (no spend yet, or at 100%).
func (b *cursorBridge) keepCapsLocked(pools []cursor.Pool) {
	for i := range pools {
		p := &pools[i]
		if p.LimitEstimated {
			if b.caps == nil {
				b.caps = map[string]int64{}
			}
			b.caps[p.Kind] = *p.LimitCents
		} else if limit, ok := b.caps[p.Kind]; ok && p.LimitCents == nil && p.UsedCents != nil {
			p.LimitCents, p.LimitEstimated = &limit, true
		}
	}
}

func (b *cursorBridge) authFailure(gen uint64, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if gen != b.generation || !b.enabled {
		return
	}
	// Only a logout or an unsupported login drops the account. A transient
	// store error keeps the reading (shown as stale), Retry-After and the
	// minimum interval.
	if errors.Is(err, cursor.ErrNotLoggedIn) || errors.Is(err, cursor.ErrUnsupportedLogin) {
		b.clearLocked()
	}
	b.err = err
}

func (b *cursorBridge) State(now time.Time) runway.CursorState {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := runway.CursorState{Availability: runway.CursorDisabled}
	if !b.enabled {
		return st
	}
	st.Availability = runway.CursorLoading
	st.Refreshing, st.IdentityID, st.Email, st.Plan = b.refreshing, b.identity, b.email, b.plan
	if b.quota != nil {
		st.Availability = runway.CursorReady
		st.UpdatedAt = b.quota.UpdatedAt
		if b.quota.Plan != "" {
			st.Plan = b.quota.Plan
		}
		cursorQuotaView(&st, *b.quota, b.lc, now)
	}
	if b.err != nil {
		st.Stale = b.quota != nil
		st.Availability, st.LastError = cursorErrorText(b.lc, b.err)
	}
	return st
}

func cursorErrorText(l l10n.Localizer, err error) (runway.CursorAvailability, string) {
	var limited *cursor.RateLimitedError
	switch {
	case errors.Is(err, cursor.ErrNotLoggedIn):
		return runway.CursorNotLoggedIn, l.Text(l10n.KeyCursorNoLogin)
	case errors.Is(err, cursor.ErrUnsupportedLogin):
		return runway.CursorFailed, l.Text(l10n.KeyCursorUnsupportedLogin)
	case errors.Is(err, cursor.ErrUnauthorized):
		return runway.CursorReauthRequired, l.Text(l10n.KeyCursorReauth)
	case errors.As(err, &limited):
		return runway.CursorFailed, l.Text(l10n.KeyCursorRateLimited)
	case errors.Is(err, cursor.ErrResponse):
		return runway.CursorFailed, l.Text(l10n.KeyCursorParseFailed)
	case errors.Is(err, cursor.ErrCredentials):
		return runway.CursorFailed, l.Text(l10n.KeyCursorCredentialsFailed)
	default:
		return runway.CursorFailed, l.Text(l10n.KeyCursorUnavailable)
	}
}

func cursorQuotaView(st *runway.CursorState, q cursor.Snapshot, l l10n.Localizer, now time.Time) {
	for _, p := range q.Pools {
		key := l10n.KeyCursorShared
		switch p.Kind {
		case "auto":
			key = l10n.KeyCursorAuto
		case "api":
			key = l10n.KeyCursorAPI
		}
		title := l.Text(key)
		if p.UsedPercent != nil {
			used := int(math.Round(min(100, max(0, *p.UsedPercent))))
			m := runway.NewMeter(title, runway.MeterWindow{UsedPercent: used, WindowMinutes: p.WindowMinutes, ResetsAt: p.ResetsAt}, now, nil, runway.SourceStandard)
			m.PreserveTitle = true
			m.Projection = nil
			st.Meters = append(st.Meters, m)
		}
		if p.UsedCents != nil && p.LimitCents != nil {
			value := cursorMoney(*p.UsedCents) + " / " + cursorMoney(*p.LimitCents)
			if p.LimitEstimated {
				value += " · " + l.Text(l10n.KeyCursorEstimated)
			}
			st.Lines = append(st.Lines, runway.DetailLine{Title: title, Value: value})
		} else if p.UsedCents != nil {
			st.Lines = append(st.Lines, runway.DetailLine{Title: title + " · " + l.Text(l10n.KeyUsed), Value: cursorMoney(*p.UsedCents)})
		} else if p.UsedPercent == nil {
			st.Lines = append(st.Lines, runway.DetailLine{Title: title, Value: l.Text(l10n.KeyStatusUnknown)})
		}
	}
	if q.OnDemandCents != nil {
		st.Lines = append(st.Lines, runway.DetailLine{Title: l.Text(l10n.KeyCursorOnDemand), Value: cursorMoney(*q.OnDemandCents)})
	}
	for _, credit := range q.Credits {
		title := credit.Name
		if title == "" {
			title = l.Text(l10n.KeyCursorCredit)
		}
		value := cursorMoney(credit.RemainingCents)
		if !credit.ExpiresAt.IsZero() {
			value += " · " + credit.ExpiresAt.In(time.Local).Format("2006-01-02")
		}
		st.Lines = append(st.Lines, runway.DetailLine{Title: title, Value: value})
	}
}

func cursorMoney(cents int64) string { return fmt.Sprintf("$%d.%02d", cents/100, cents%100) }
