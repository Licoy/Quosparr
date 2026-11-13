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
)

// QuotaThrottle holds accounts back after a 429. Keys are account ids; a
// 429 is a wait, never a reason to rotate tokens.
type QuotaThrottle struct {
	mu    sync.Mutex
	state map[string]throttleState
}

type throttleState struct {
	backoff      time.Duration
	blockedUntil time.Time
}

// Wait reports how long id must still wait; zero means a request may start.
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

// Succeeded clears the backoff of id.
func (t *QuotaThrottle) Succeeded(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.state, id)
}
