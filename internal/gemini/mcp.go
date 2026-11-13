package gemini

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/mcp"
)

var errMCPHomeUnknown = errors.New("gemini: Antigravity data directory unknown")

// MCPConfigPath is the MCP configuration shared by the Antigravity App and
// agy: <GeminiHome>/config/mcp_config.json.
func (s *Store) MCPConfigPath() string {
	if s.geminiHome == "" {
		return ""
	}
	return filepath.Join(s.geminiHome, "config", "mcp_config.json")
}

// LoadMCP reads the global MCP declarations of the Antigravity clients.
func (s *Store) LoadMCP(ctx context.Context) (mcp.Snapshot, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return mcp.Snapshot{}, err
	}
	return mcp.Load(ctx, source)
}

// DeleteMCP removes the selected declarations after a backup.
func (s *Store) DeleteMCP(ctx context.Context, selected []mcp.Server, now time.Time) ([]mcp.DeleteResult, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return nil, err
	}
	return mcp.Delete(ctx, source, selected, now)
}

// SetMCPEnabled writes one scanned server's "disabled" flag after a backup.
func (s *Store) SetMCPEnabled(ctx context.Context, server mcp.Server, enabled bool, now time.Time) error {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return err
	}
	return mcp.SetEnabled(ctx, source, server, enabled, now)
}

// PrepareMCPAdd captures a user-reviewed addition.
func (s *Store) PrepareMCPAdd(ctx context.Context, drafts []mcp.Draft) (mcp.AddPlan, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return mcp.AddPlan{}, err
	}
	return mcp.PrepareAdd(ctx, source, drafts)
}

// AddMCP commits exactly the prepared addition.
func (s *Store) AddMCP(ctx context.Context, plan mcp.AddPlan, now time.Time) (mcp.AddResult, error) {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return mcp.AddResult{}, err
	}
	return mcp.Add(ctx, source, plan, now)
}

// mcpSource backs up into <Quosparr data>/backups, beside the account
// library, like the other providers.
func (s *Store) mcpSource() (mcp.Source, error) {
	if s.geminiHome == "" {
		return mcp.Source{}, errMCPHomeUnknown
	}
	path, err := filepath.Abs(s.MCPConfigPath())
	if err != nil {
		return mcp.Source{}, err
	}
	backup, err := filepath.Abs(filepath.Join(filepath.Dir(s.root), "backups"))
	return mcp.Source{Provider: mcp.ProviderGemini, Path: path, BackupDir: backup}, err
}
