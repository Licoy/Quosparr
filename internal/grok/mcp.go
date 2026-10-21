package grok

import (
	"context"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/mcp"
)

// LoadMCP reads user-scoped MCP declarations from Grok config.toml.
func (s *Store) LoadMCP(ctx context.Context) (mcp.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return mcp.Snapshot{}, err
	}
	return mcp.Load(ctx, source)
}

// DeleteMCP removes selected user-scoped declarations under the Store lock.
func (s *Store) DeleteMCP(ctx context.Context, selected []mcp.Server, now time.Time) ([]mcp.DeleteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return nil, err
	}
	return mcp.Delete(ctx, source, selected, now)
}

// SetMCPEnabled turns one scanned user-scoped server on or off under the
// Store lock shared with skill configuration writes.
func (s *Store) SetMCPEnabled(ctx context.Context, server mcp.Server, enabled bool, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return err
	}
	return mcp.SetEnabled(ctx, source, server, enabled, now)
}

func (s *Store) mcpSource() (mcp.Source, error) {
	path, err := filepath.Abs(filepath.Join(s.OfficialHome(), "config.toml"))
	if err != nil {
		return mcp.Source{}, err
	}
	backup, err := filepath.Abs(filepath.Join(filepath.Dir(s.root), "backups"))
	return mcp.Source{
		Provider:  mcp.ProviderGrok,
		Path:      path,
		BackupDir: backup,
	}, err
}
