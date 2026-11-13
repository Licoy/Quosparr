package gemini

import (
	"context"
	"sync"
)

// SessionCache keeps the parse of each conversation database between scans.
// A database is read again only when its size or modification time, or
// those of its WAL, changed; entries of databases no scan listed are
// dropped. It holds token counts and timings, never conversation text, and
// is safe for concurrent use.
type SessionCache struct {
	mu    sync.Mutex
	files map[string]sessionCached
}

type sessionCached struct {
	state sessionFileState
	conv  sessionConversation
}

func (c *SessionCache) get(path string, state sessionFileState) (sessionConversation, bool) {
	if c == nil {
		return sessionConversation{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	hit, ok := c.files[path]
	if !ok || !hit.state.same(state) {
		return sessionConversation{}, false
	}
	return hit.conv, true
}

func (c *SessionCache) put(path string, state sessionFileState, conv sessionConversation) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil {
		c.files = map[string]sessionCached{}
	}
	c.files[path] = sessionCached{state: state, conv: conv}
}

// retain drops entries for databases outside listed.
func (c *SessionCache) retain(listed map[string]bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for path := range c.files {
		if !listed[path] {
			delete(c.files, path)
		}
	}
}

// sessionConversationOf returns the cached parse of an entry or reads it.
// The state of a fresh read replaces the listed state, so a database that
// changed after listing is cached under what was actually read.
func (s SessionScanner) sessionConversationOf(ctx context.Context, entry sessionEntry) (sessionConversation, error) {
	if conv, ok := s.Cache.get(entry.path, entry.state); ok {
		return conv, nil
	}
	conv, state, err := sessionReadConversation(ctx, entry.path)
	if err != nil {
		return sessionConversation{}, err
	}
	s.Cache.put(entry.path, state, conv)
	return conv, nil
}
