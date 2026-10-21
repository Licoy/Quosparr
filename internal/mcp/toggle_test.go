package mcp

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func toggle(t *testing.T, source Source, name string, enabled bool) error {
	t.Helper()
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return SetEnabled(context.Background(), source, selectServers(t, snapshot, name)[0], enabled, deleteTime)
}

func assertConfig(t *testing.T, source Source, want string) {
	t.Helper()
	got, err := os.ReadFile(source.Path)
	if err != nil || string(got) != want {
		t.Fatalf("config = %q, err = %v\nwant %q", got, err, want)
	}
}

func TestSetEnabledCodexInsertsAndFlipsOnlyTheEnabledKey(t *testing.T) {
	source := testSource(t, ProviderCodex, "config.toml")
	const original = `# user comment
model = "keep"

[mcp_servers.docs]
  url = "https://example.com/mcp" # docs
  startup_timeout_sec = 12

[mcp_servers.docs.http_headers]
Authorization = "Bearer SECRET"

[mcp_servers.other]
command = "tool"
enabled = true   # keep this note
`
	writeConfig(t, source, original)
	if err := toggle(t, source, "docs", false); err != nil {
		t.Fatal(err)
	}
	disabled := strings.Replace(original, "  startup_timeout_sec = 12\n", "  startup_timeout_sec = 12\nenabled = false\n", 1)
	assertConfig(t, source, disabled)
	assertBackup(t, source, original)
	if err := toggle(t, source, "other", false); err != nil {
		t.Fatal(err)
	}
	disabled = strings.Replace(disabled, "enabled = true   # keep this note", "enabled = false   # keep this note", 1)
	assertConfig(t, source, disabled)
	if err := toggle(t, source, "docs", true); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, strings.Replace(disabled, "12\nenabled = false\n", "12\nenabled = true\n", 1))
	snapshot, _ := Load(context.Background(), source)
	if !snapshot.Servers[0].Enabled || snapshot.Servers[1].Enabled {
		t.Fatalf("states = %#v", snapshot.Servers)
	}
}

func TestSetEnabledCodexAlreadyEnabledWritesNothing(t *testing.T) {
	source := testSource(t, ProviderCodex, "config.toml")
	writeConfig(t, source, "[mcp_servers.docs]\ncommand = \"tool\"")
	if err := toggle(t, source, "docs", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source.BackupDir); !os.IsNotExist(err) {
		t.Fatalf("no-op toggle created backups: %v", err)
	}
	if err := toggle(t, source, "docs", false); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "[mcp_servers.docs]\ncommand = \"tool\"\nenabled = false\n")
}

func TestSetEnabledKeepsCRLF(t *testing.T) {
	source := testSource(t, ProviderGrok, "config.toml")
	writeConfig(t, source, "[mcp_servers.docs]\r\ncommand = \"tool\"\r\n")
	if err := toggle(t, source, "docs", false); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "disabled_mcp_servers = [\"docs\"]\r\n\r\n[mcp_servers.docs]\r\ncommand = \"tool\"\r\nenabled = false\r\n")
}

func TestSetEnabledRejectsInlineAndClaudeEntries(t *testing.T) {
	source := testSource(t, ProviderCodex, "config.toml")
	const original = "[mcp_servers]\ninline = { command = \"tool\" }\ndotted.command = \"tool\"\n\n[mcp_servers.text]\ncommand = \"tool\"\nenabled = \"no\"\n"
	writeConfig(t, source, original)
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range snapshot.Servers {
		if server.Toggleable {
			t.Fatalf("%s is toggleable", server.Name)
		}
		server.Toggleable = true
		if err := SetEnabled(context.Background(), source, server, false, deleteTime); !errors.Is(err, errToggleUnsupported) {
			t.Fatalf("%s: err = %v", server.Name, err)
		}
	}
	assertConfig(t, source, original)

	claude := testSource(t, ProviderClaude, ".claude.json")
	writeConfig(t, claude, `{"mcpServers":{"docs":{"command":"tool"}}}`)
	snapshot, err = Load(context.Background(), claude)
	if err != nil || snapshot.Servers[0].Toggleable {
		t.Fatalf("claude snapshot = %#v, %v", snapshot, err)
	}
	if err := SetEnabled(context.Background(), claude, snapshot.Servers[0], false, deleteTime); !errors.Is(err, errToggleUnsupported) {
		t.Fatalf("claude err = %v", err)
	}
}

func TestSetEnabledRejectsChangedConfiguration(t *testing.T) {
	source := testSource(t, ProviderCodex, "config.toml")
	writeConfig(t, source, "[mcp_servers.docs]\ncommand = \"tool\"\n")
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	const changed = "[mcp_servers.docs]\ncommand = \"other\"\n"
	writeConfig(t, source, changed)
	if err := SetEnabled(context.Background(), source, snapshot.Servers[0], false, deleteTime); err == nil {
		t.Fatal("stale toggle succeeded")
	}
	assertConfig(t, source, changed)
}

func TestLoadGrokHonorsDisabledList(t *testing.T) {
	source := testSource(t, ProviderGrok, "config.toml")
	writeConfig(t, source, `disabled_mcp_servers = ["listed", "elsewhere"]

[mcp_servers.listed]
command = "tool"
enabled = true

[mcp_servers.field]
command = "tool"
enabled = false

[mcp_servers.on]
command = "tool"
`)
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range snapshot.Servers {
		if server.Enabled != (server.Name == "on") || !server.Toggleable {
			t.Fatalf("%s: enabled %v, toggleable %v", server.Name, server.Enabled, server.Toggleable)
		}
	}

	codex := testSource(t, ProviderCodex, "config.toml")
	writeConfig(t, codex, "disabled_mcp_servers = [\"docs\"]\n\n[mcp_servers.docs]\ncommand = \"tool\"\n")
	snapshot, err = Load(context.Background(), codex)
	if err != nil || !snapshot.Servers[0].Enabled {
		t.Fatalf("codex read Grok's list: %#v, %v", snapshot, err)
	}

	writeConfig(t, source, "disabled_mcp_servers = \"docs\"\n\n[mcp_servers.docs]\ncommand = \"tool\"\n")
	snapshot, err = Load(context.Background(), source)
	if err != nil || snapshot.Servers[0].Toggleable {
		t.Fatalf("malformed list snapshot = %#v, %v", snapshot, err)
	}
}

func TestSetEnabledGrokWritesListAndField(t *testing.T) {
	source := testSource(t, ProviderGrok, "config.toml")
	const original = `# grok settings

# docs server
[mcp_servers.docs]
command = "tool"

[mcp_servers.docs.env]
TOKEN = "SECRET"

[mcp_servers.other]
command = "tool"
`
	writeConfig(t, source, original)
	if err := toggle(t, source, "docs", false); err != nil {
		t.Fatal(err)
	}
	const disabled = `# grok settings

disabled_mcp_servers = ["docs"]

# docs server
[mcp_servers.docs]
command = "tool"
enabled = false

[mcp_servers.docs.env]
TOKEN = "SECRET"

[mcp_servers.other]
command = "tool"
`
	assertConfig(t, source, disabled)
	if err := toggle(t, source, "other", false); err != nil {
		t.Fatal(err)
	}
	if err := toggle(t, source, "docs", true); err != nil {
		t.Fatal(err)
	}
	if err := toggle(t, source, "other", true); err != nil {
		t.Fatal(err)
	}
	const enabled = `# grok settings

# docs server
[mcp_servers.docs]
command = "tool"
enabled = true

[mcp_servers.docs.env]
TOKEN = "SECRET"

[mcp_servers.other]
command = "tool"
enabled = true
`
	assertConfig(t, source, enabled)
}

func TestSetEnabledGrokAppendsAfterRootKeysAndClearsListOnly(t *testing.T) {
	source := testSource(t, ProviderGrok, "config.toml")
	writeConfig(t, source, "theme = \"dark\"\n\n[mcp_servers.docs]\ncommand = \"tool\"\n")
	if err := toggle(t, source, "docs", false); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "theme = \"dark\"\ndisabled_mcp_servers = [\"docs\"]\n\n[mcp_servers.docs]\ncommand = \"tool\"\nenabled = false\n")

	writeConfig(t, source, "disabled_mcp_servers = [\n    \"docs\",\n    \"elsewhere\",\n]\n\n[mcp_servers.docs]\ncommand = \"tool\"\n")
	if err := toggle(t, source, "docs", true); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "disabled_mcp_servers = [\"elsewhere\"]\n\n[mcp_servers.docs]\ncommand = \"tool\"\n")
}

func TestDeleteGrokPrunesDisabledList(t *testing.T) {
	source := testSource(t, ProviderGrok, "config.toml")
	writeConfig(t, source, "disabled_mcp_servers = [\"gone\", \"elsewhere\"]\n\n[mcp_servers.gone]\ncommand = \"tool\"\nenabled = false\n\n[mcp_servers.kept]\ncommand = \"tool\"\n")
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(context.Background(), source, selectServers(t, snapshot, "gone"), deleteTime); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "disabled_mcp_servers = [\"elsewhere\"]\n\n\n[mcp_servers.kept]\ncommand = \"tool\"\n")

	writeConfig(t, source, "disabled_mcp_servers = [\"kept\"]\n\n[mcp_servers.kept]\ncommand = \"tool\"\n")
	snapshot, _ = Load(context.Background(), source)
	if _, err := Delete(context.Background(), source, snapshot.Servers, deleteTime); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "")
}
