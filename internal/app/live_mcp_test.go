package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/store"
)

func TestMCPActionsUseConfiguredGlobalFiles(t *testing.T) {
	home := t.TempDir()
	runway := filepath.Join(home, ".quosparr")
	codexHome := filepath.Join(home, "codex-home")
	claudeHome := filepath.Join(home, "claude-home")
	grokHome := filepath.Join(home, "grok-home")
	rt := &liveApp{
		codex:     codex.NewStore(codexHome, runway),
		claudeMod: claude.NewModule(claude.NewStore(claudeHome, runway, home, claudeHome, "")),
		grokMod:   grok.NewModule(grok.NewStore(grokHome, runway)),
	}
	files := map[store.Provider]struct {
		path string
		body string
	}{
		store.ProviderCodex:  {filepath.Join(codexHome, "config.toml"), "[mcp_servers.alpha]\ncommand = \"tool\"\n"},
		store.ProviderClaude: {filepath.Join(claudeHome, ".claude.json"), `{"mcpServers":{"alpha":{"command":"tool"}}}`},
		store.ProviderGrok:   {filepath.Join(grokHome, "config.toml"), "[mcp_servers.alpha]\ncommand = \"tool\"\n"},
	}
	for provider, fixture := range files {
		if err := os.MkdirAll(filepath.Dir(fixture.path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixture.path, []byte(fixture.body), 0o600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := rt.queryMCP(context.Background(), provider)
		if err != nil || snapshot.Path != fixture.path || len(snapshot.Servers) != 1 || snapshot.Servers[0].Name != "alpha" {
			t.Fatalf("queryMCP(%s) = %#v, %v", provider, snapshot, err)
		}
		results, err := rt.deleteMCP(context.Background(), provider, snapshot.Servers)
		if err != nil || len(results) != 1 || results[0].Err != nil {
			t.Fatalf("deleteMCP(%s) = %#v, %v", provider, results, err)
		}
		after, err := rt.queryMCP(context.Background(), provider)
		if err != nil || len(after.Servers) != 0 {
			t.Fatalf("queryMCP(%s) after delete = %#v, %v", provider, after, err)
		}
	}
}

func TestMCPActionsMissingConfigAndUnknownProvider(t *testing.T) {
	rt, _ := sessionTestRuntime(t)
	for _, provider := range store.CLIProviders() {
		snapshot, err := rt.queryMCP(context.Background(), provider)
		if err != nil || snapshot.Path == "" || len(snapshot.Servers) != 0 {
			t.Fatalf("queryMCP(%s) missing config = %#v, %v", provider, snapshot, err)
		}
	}
	if _, err := rt.queryMCP(context.Background(), store.Provider("other")); err == nil {
		t.Fatal("queryMCP accepted an unknown provider")
	}
	if _, err := rt.deleteMCP(context.Background(), store.Provider("other"), nil); err == nil {
		t.Fatal("deleteMCP accepted an unknown provider")
	}
	if _, err := (&liveApp{}).queryMCP(context.Background(), store.ProviderCodex); !errors.Is(err, errMCPUnavailable) {
		t.Fatalf("missing store error = %v", err)
	}
}
