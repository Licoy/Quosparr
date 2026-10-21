// Package mcp reads and removes user-scoped MCP server declarations without
// starting servers or exposing credentials from their configuration.
package mcp

import (
	"context"
	"io/fs"
	"time"
)

type Provider string

const (
	ProviderCodex  Provider = "codex"
	ProviderClaude Provider = "claude"
	ProviderGrok   Provider = "grok"
)

type Source struct {
	Provider  Provider
	Path      string
	BackupDir string
}

type Server struct {
	ID          string
	Name        string
	Provider    Provider
	Path        string
	Transport   string
	Command     string
	URL         string
	Problem     string
	Details     string
	Enabled     bool
	Deletable   bool
	Toggleable  bool // a Codex / Grok table of its own whose enabled state can be rewritten
	Fingerprint string
	fileInfo    fs.FileInfo
}

type Snapshot struct {
	Servers     []Server
	Path        string
	Fingerprint string
	fileInfo    fs.FileInfo
}

type DeleteResult struct {
	Server Server
	Err    error
}

func Load(ctx context.Context, source Source) (Snapshot, error) {
	return load(ctx, source)
}

func Delete(ctx context.Context, source Source, selected []Server, now time.Time) ([]DeleteResult, error) {
	return deleteServers(ctx, source, selected, now)
}

// SetEnabled turns one scanned server on or off in its configuration after
// a backup, as the provider's own CLI records it.
func SetEnabled(ctx context.Context, source Source, server Server, enabled bool, now time.Time) error {
	return setEnabled(ctx, source, server, enabled, now)
}
