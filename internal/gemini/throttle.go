package gemini

import (
	"sync"
	"time"
)

const (
	// RateLimitBackoffStart is the first wait after a 429 without Retry-After.
	RateLimitBackoffStart = 5 * time.Minute
	// RateLimitBackoffMax caps the exponential backoff.
	RateLimitBackoffMax = time.Hour
	// QuotaMinInterval is the least time between two quota reads of one
	// account, successful or not.
	QuotaMinInterval = 15 * time.Minute
)

// QuotaThrottle spaces the quota reads of each account: QuotaMinInterval
// after every attempt and a backoff after a 429. Keys are account ids; a
// 429 is a wait, never a reason to rotate tokens.
type QuotaThrottle struct {
	mu    sync.Mutex
	state map[string]throttleState
}

type throttleState struct {
	backoff      time.Duration
	blockedUntil time.Time
	lastAttempt  time.Time
}

// Attempted records that a quota read of id starts at now.
func (t *QuotaThrottle) Attempted(id string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		t.state = map[string]throttleState{}
	}
	st := t.state[id]
	st.lastAttempt = now
	t.state[id] = st
}

// Cooldown reports how long id must still wait after its last attempt.
func (t *QuotaThrottle) Cooldown(id string, now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	last := t.state[id].lastAttempt
	if last.IsZero() {
		return 0
	}
	return max(last.Add(QuotaMinInterval).Sub(now), 0)
}

// Wait reports how long a 429 still holds id back; zero means none does.
func (t *QuotaThrottle) Wait(id string, now time.Time) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if until := t.state[id].blockedUntil; until.After(now) {
		return until.Sub(now)
	}
	return 0
}

// ObserveRateLimit records a 429 and returns the wait: Retry-After when the
// server sent one, else 5 minutes doubling up to an hour.
func (t *QuotaThrottle) ObserveRateLimit(id string, now time.Time, e *RateLimitedError) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		t.state = map[string]throttleState{}
	}
	st := t.state[id]
	wait := max(e.RetryAfter, 0)
	if !e.HasRetryAfter {
		wait = RateLimitBackoffStart
		if st.backoff > 0 {
			wait = min(st.backoff*2, RateLimitBackoffMax)
		}
		st.backoff = wait
	}
	if until := now.Add(wait); until.After(st.blockedUntil) {
		st.blockedUntil = until
	}
	t.state[id] = st
	return wait
}

// Succeeded clears the 429 backoff of id; the cooldown stays.
func (t *QuotaThrottle) Succeeded(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if st, ok := t.state[id]; ok {
		t.state[id] = throttleState{lastAttempt: st.lastAttempt}
	}
}
