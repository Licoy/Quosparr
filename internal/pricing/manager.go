package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/licoy/quosparr/internal/platform"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Status struct {
	LastAttempt time.Time `json:"lastAttempt"`
	LastSuccess time.Time `json:"lastSuccess"`
	LastChanged time.Time `json:"lastChanged"`
	ETag        string    `json:"etag"`
	Version     string    `json:"version"`
	Error       string    `json:"-"`
	Running     bool      `json:"-"`
}
type Config struct {
	CachePath string
	Fetch     Fetcher
	Now       func() time.Time
}
type cacheDocument struct {
	Schema int     `json:"schema"`
	Source string  `json:"source"`
	Status Status  `json:"status"`
	Models []Model `json:"models"`
}
type refreshFlight struct {
	done chan struct{}
	err  error
}
type Manager struct {
	cfg      Config
	mu       sync.Mutex
	snapshot Snapshot
	status   Status
	flight   *refreshFlight
	cancel   context.CancelFunc
	closed   bool
}

// NewManager reads cache only; construction and Snapshot never request the network.
func NewManager(cfg Config) *Manager {
	if cfg.Fetch == nil {
		cfg.Fetch = Fetch
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	m := &Manager{cfg: cfg}
	if cfg.CachePath == "" {
		return m
	}
	file, err := os.Open(cfg.CachePath)
	if errors.Is(err, os.ErrNotExist) {
		return m
	}
	if err != nil {
		m.status.Error = "pricing: cache read failed"
		return m
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxResponseBytes+1))
	if err != nil {
		m.status.Error = "pricing: cache read failed"
		return m
	}
	var doc cacheDocument
	if len(data) > MaxResponseBytes || json.Unmarshal(data, &doc) != nil || doc.Schema != 1 || doc.Source != DefaultSourceURL || !usableModels(doc.Models) {
		m.status.Error = "pricing: invalid cache"
		return m
	}
	for _, model := range doc.Models {
		if err := validateModel(model); err != nil {
			m.status.Error = "pricing: invalid cache"
			return m
		}
	}
	snap, err := NewSnapshot(doc.Models)
	if err != nil {
		m.status.Error = "pricing: invalid cache"
		return m
	}
	if snap.Version() != doc.Status.Version {
		m.status.Error = "pricing: cache version mismatch"
		return m
	}
	m.snapshot = snap
	m.status = doc.Status
	return m
}
func validateModel(m Model) error {
	if m.ID == "" || m.ProviderID == "" || strings.ContainsRune(m.ID, 0) || strings.ContainsRune(m.ProviderID, 0) {
		return ErrInvalidCatalog
	}
	if m.SourceURL != "" {
		u, err := url.Parse(m.SourceURL)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return ErrInvalidCatalog
		}
	}
	if err := validateRate(m.Rate); err != nil {
		return err
	}
	if m.LegacyLong != nil {
		if err := validateRate(*m.LegacyLong); err != nil {
			return err
		}
	}
	for _, t := range m.Tiers {
		if t.Size < 0 {
			return ErrInvalidCatalog
		}
		if err := validateRate(t.Rate); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot }
func (m *Manager) Status() Status     { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if m.cancel != nil {
		m.cancel()
	}
}

// Refresh coalesces concurrent requests; LastAttempt is the request start time.
func (m *Manager) Refresh(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("pricing: manager closed")
	}
	if f := m.flight; f != nil {
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.done:
			return f.err
		}
	}
	f := &refreshFlight{done: make(chan struct{})}
	m.flight = f
	m.status.LastAttempt = m.cfg.Now()
	m.status.Running = true
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	m.cancel = cancel
	etag := m.status.ETag
	m.mu.Unlock()
	err := m.refresh(ctx, etag)
	cancel()
	m.mu.Lock()
	m.status.Running = false
	if err != nil {
		m.status.Error = err.Error()
	}
	f.err = err
	m.flight = nil
	m.cancel = nil
	close(f.done)
	m.mu.Unlock()
	return err
}
func (m *Manager) refresh(ctx context.Context, etag string) error {
	resp, err := m.cfg.Fetch(ctx, etag)
	if err != nil {
		return fmt.Errorf("pricing: fetch failed: %w", err)
	}
	if len(resp.Body) > MaxResponseBytes {
		return ErrResponseTooLarge
	}
	m.mu.Lock()
	snap := m.snapshot
	status := m.status
	m.mu.Unlock()
	switch resp.StatusCode {
	case 304:
		if snap.Version() == "" {
			return errors.New("pricing: 304 without cached catalog")
		}
	case 200:
		snap, err = Parse(resp.Body)
		if err != nil {
			return err
		}
		status.ETag = resp.ETag
	default:
		return fmt.Errorf("pricing: HTTP status %d", resp.StatusCode)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	status.LastSuccess = m.cfg.Now()
	status.Error = ""
	status.Running = false
	if status.Version != snap.Version() {
		status.LastChanged = status.LastSuccess
	}
	status.Version = snap.Version()
	doc := cacheDocument{1, DefaultSourceURL, status, snap.Models()}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.cfg.CachePath != "" {
		if err := platform.WriteFileAtomic(m.cfg.CachePath, data, 0o600); err != nil {
			return fmt.Errorf("pricing: cache save failed: %w", err)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("pricing: manager closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.snapshot = snap
	m.status = status
	m.status.Running = true
	return nil
}
