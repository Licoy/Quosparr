// Package mcp manages user-scoped MCP declarations and explicitly requested
// connection previews without exposing credentials in summaries or errors.
//
// Gemini (Antigravity) keeps one global mcp_config.json shared by the
// desktop App and the agy CLI: {"mcpServers": {name: {...}}} with stdio
// "command" / "args" / "env", HTTP "serverUrl" (the documented "url" is
// read too) / "headers", and a per-server "disabled" boolean. The official
// CLI rewrites the whole file through Go's encoding/json: two-space
// indentation, sorted object keys, HTML-escaped strings and a final newline,
// with other top-level keys and unknown server fields kept verbatim. An
// empty or whitespace-only file is a configuration without servers. Only
// stdio and streamable HTTP are supported; the CLI rejects "sse".
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
	ProviderGemini Provider = "gemini"
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
	Toggleable  bool // a Codex / Grok table of its own, or a Gemini entry whose disabled flag is absent or boolean
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
