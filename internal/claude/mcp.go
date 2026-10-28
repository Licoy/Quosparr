package claude

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/mcp"
)

// MCPConfigPath is the user-scoped MCP file for this config home. A custom
// CLAUDE_CONFIG_DIR never falls back to the default home account.
func (s *Store) MCPConfigPath() string {
	if strings.TrimSpace(s.configEnv) == "" && filepath.Clean(s.configDir) == filepath.Clean(filepath.Join(s.home, ".claude")) {
		return filepath.Join(s.home, ".claude.json")
	}
	return filepath.Join(s.configDir, ".claude.json")
}

// LoadMCP reads only top-level user-scoped MCP declarations.
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

func (s *Store) mcpSource() (mcp.Source, error) {
	path, err := filepath.Abs(s.MCPConfigPath())
	if err != nil {
		return mcp.Source{}, err
	}
	backup, err := filepath.Abs(filepath.Join(s.runwayHomeDir(), "backups"))
	return mcp.Source{Provider: mcp.ProviderClaude, Path: path, BackupDir: backup}, err
}

// PrepareMCPAdd captures a user-reviewed addition under the shared Store lock.
func (s *Store) PrepareMCPAdd(ctx context.Context, drafts []mcp.Draft) (mcp.AddPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return mcp.AddPlan{}, err
	}
	return mcp.PrepareAdd(ctx, source, drafts)
}

// AddMCP commits exactly the prepared addition under the shared Store lock.
func (s *Store) AddMCP(ctx context.Context, plan mcp.AddPlan, now time.Time) (mcp.AddResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	source, err := s.mcpSource()
	if err != nil {
		return mcp.AddResult{}, err
	}
	return mcp.Add(ctx, source, plan, now)
}
