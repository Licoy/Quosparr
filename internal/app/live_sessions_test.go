package app

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
)

func TestSessionActionsScanAllConfiguredProviders(t *testing.T) {
	rt, wantRoots := sessionTestRuntime(t)
	for _, provider := range store.CLIProviders() {
		got, err := rt.queryManagedSessions(context.Background(), provider)
		if err != nil || len(got) != 0 {
			t.Fatalf("queryManagedSessions(%s) = %#v, %v", provider, got, err)
		}
	}
	if got := rt.sessionTrustedRoots(); !reflect.DeepEqual(got, wantRoots) {
		t.Fatalf("sessionTrustedRoots() = %#v, want %#v", got, wantRoots)
	}
}

func TestSessionActionsRejectUnknownProvider(t *testing.T) {
	rt, _ := sessionTestRuntime(t)
	if _, err := rt.queryManagedSessions(context.Background(), store.Provider("other")); err == nil {
		t.Fatal("queryManagedSessions accepted an unknown provider")
	}
}

func sessionTestRuntime(t *testing.T) (*liveApp, map[string][]string) {
	t.Helper()
	home := t.TempDir()
	runway := filepath.Join(home, ".quosparr")
	codexHome := filepath.Join(home, ".codex")
	claudeHome := filepath.Join(home, ".claude")
	grokHome := filepath.Join(home, ".grok")
	codexStore := codex.NewStore(codexHome, runway)
	claudeStore := claude.NewStore(claudeHome, runway, home, claudeHome, "")
	grokStore := grok.NewStore(grokHome, runway)
	rt := &liveApp{
		codex: codexStore, claudeMod: claude.NewModule(claudeStore),
		grokMod: grok.NewModule(grokStore),
	}
	return rt, map[string][]string{
		sessions.ProviderCodex:  {filepath.Join(codexHome, "sessions"), filepath.Join(codexHome, "archived_sessions")},
		sessions.ProviderClaude: {filepath.Join(claudeHome, "projects")},
		sessions.ProviderGrok:   {filepath.Join(grokHome, "sessions")},
	}
}
