package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadGeminiReadsServerURLAndDisabled(t *testing.T) {
	source := testSource(t, ProviderGemini, "mcp_config.json")
	writeConfig(t, source, `{"mcpServers":{
"local":{"command":"/usr/local/bin/tool","args":["--token","SECRET"],"env":{"KEY":"SECRET"},"disabled":true},
"remote":{"serverUrl":"https://user:SECRET@example.test/mcp?key=SECRET","headers":{"Authorization":"Bearer SECRET"}},
"documented":{"url":"https://docs.example.test/mcp","disabled":false},
"odd":{"command":"x","disabled":"yes"}}}`)
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Server{}
	for _, server := range snapshot.Servers {
		byName[server.Name] = server
		if strings.Contains(server.Details, "SECRET") || strings.Contains(server.URL, "SECRET") {
			t.Fatalf("%s leaks a secret: %q", server.Name, server.Details)
		}
	}
	local, remote, documented, odd := byName["local"], byName["remote"], byName["documented"], byName["odd"]
	if local.Transport != "stdio" || local.Command != "tool" || local.Enabled || !local.Toggleable || !local.Deletable {
		t.Fatalf("local = %#v", local)
	}
	if remote.Transport != "http" || remote.URL != "https://example.test" || !remote.Enabled || !remote.Toggleable || remote.Problem != "" {
		t.Fatalf("remote = %#v", remote)
	}
	if documented.Transport != "http" || documented.URL != "https://docs.example.test" || !documented.Enabled {
		t.Fatalf("documented = %#v", documented)
	}
	if odd.Toggleable || !odd.Enabled {
		t.Fatalf("odd = %#v", odd)
	}
	if err := toggle(t, source, "odd", false); err == nil {
		t.Fatal("non-boolean disabled flag was rewritten")
	}
}

func TestLoadGeminiBlankFileHasNoServers(t *testing.T) {
	for _, content := range []string{"", " \n\t\n"} {
		source := testSource(t, ProviderGemini, "mcp_config.json")
		writeConfig(t, source, content)
		snapshot, err := Load(context.Background(), source)
		if err != nil || len(snapshot.Servers) != 0 || snapshot.Fingerprint == "" {
			t.Fatalf("snapshot = %#v, err = %v", snapshot, err)
		}
	}
	claude := testSource(t, ProviderClaude, "claude.json")
	writeConfig(t, claude, "  \n")
	if _, err := Load(context.Background(), claude); err == nil {
		t.Fatal("Claude blank file must stay invalid")
	}
	if _, err := PrepareAdd(context.Background(), claude, []Draft{{Name: "a", Transport: "stdio", Command: "x"}}); err == nil {
		t.Fatal("Claude blank file must stay unwritable")
	}
}

func TestSetEnabledGeminiWritesOfficialFormat(t *testing.T) {
	source := testSource(t, ProviderGemini, "mcp_config.json")
	const original = `{"zeta":{"b":1,"a":[1,2]},"mcpServers":{"x":{"command":"a&b<c>","foo":{"z":1.50,"a":2}},"y":{"serverUrl":"https://e.test/u","disabled":true}},"alpha":true}`
	writeConfig(t, source, original)
	if err := toggle(t, source, "x", false); err != nil {
		t.Fatal(err)
	}
	const disabled = `{
  "alpha": true,
  "mcpServers": {
    "x": {
      "command": "a\u0026b\u003cc\u003e",
      "disabled": true,
      "foo": {
        "z": 1.50,
        "a": 2
      }
    },
    "y": {
      "disabled": true,
      "serverUrl": "https://e.test/u"
    }
  },
  "zeta": {
    "b": 1,
    "a": [
      1,
      2
    ]
  }
}
`
	assertConfig(t, source, disabled)
	assertBackup(t, source, original)
	if err := toggle(t, source, "x", false); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, disabled)
	if err := toggle(t, source, "y", true); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, strings.Replace(disabled, `"disabled": true,
      "serverUrl"`, `"disabled": false,
      "serverUrl"`, 1))
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range snapshot.Servers {
		if server.Enabled != (server.Name == "y") {
			t.Fatalf("%s enabled = %v", server.Name, server.Enabled)
		}
	}
}

func TestAddGeminiWritesCLIShape(t *testing.T) {
	cases := map[string]string{"missing": "", "empty": "", "blank": "  \n"}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			source := testSource(t, ProviderGemini, "config/mcp_config.json")
			if name != "missing" {
				if err := os.MkdirAll(filepath.Dir(source.Path), 0o700); err != nil {
					t.Fatal(err)
				}
				writeConfig(t, source, content)
			}
			drafts := []Draft{
				{Name: "local", Transport: "stdio", Command: "npx", Args: []string{"-y", "pkg"}, Env: map[string]string{"KEY": "SECRET"}},
				{Name: "remote", Transport: "http", URL: "https://example.test/mcp", Headers: map[string]string{"X-Key": "SECRET"}},
			}
			plan, err := PrepareAdd(context.Background(), source, drafts)
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range plan.Items {
				if strings.Contains(item.Details, "SECRET") {
					t.Fatalf("details leak: %q", item.Details)
				}
			}
			if _, err := Add(context.Background(), source, plan, time.Now()); err != nil {
				t.Fatal(err)
			}
			assertConfig(t, source, `{
  "mcpServers": {
    "local": {
      "args": [
        "-y",
        "pkg"
      ],
      "command": "npx",
      "disabled": false,
      "env": {
        "KEY": "SECRET"
      }
    },
    "remote": {
      "disabled": false,
      "headers": {
        "X-Key": "SECRET"
      },
      "serverUrl": "https://example.test/mcp"
    }
  }
}
`)
			again, err := PrepareAdd(context.Background(), source, drafts)
			if err != nil || again.Items[0].Status != "exists" || again.Items[1].Status != "exists" {
				t.Fatalf("same entries not recognized: %#v, %v", again.Items, err)
			}
		})
	}
}

func TestAddGeminiKeepsOtherKeysAndRawValues(t *testing.T) {
	source := testSource(t, ProviderGemini, "mcp_config.json")
	writeConfig(t, source, `{"other":{"z":1.50,"a":2},"mcpServers":{"keep":{"url":"https://k.test","unknown":{"b":1,"a":2}}}}`)
	plan, err := PrepareAdd(context.Background(), source, []Draft{{Name: "new", Transport: "stdio", Command: "srv"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Add(context.Background(), source, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, `{
  "mcpServers": {
    "keep": {
      "unknown": {
        "b": 1,
        "a": 2
      },
      "url": "https://k.test"
    },
    "new": {
      "command": "srv",
      "disabled": false
    }
  },
  "other": {
    "z": 1.50,
    "a": 2
  }
}
`)
	same, err := PrepareAdd(context.Background(), source, []Draft{{Name: "keep", Transport: "http", URL: "https://k.test"}})
	if err == nil {
		t.Fatalf("entry with unknown fields treated as equal: %#v", same.Items)
	}
}

func TestDraftGeminiRules(t *testing.T) {
	source := testSource(t, ProviderGemini, "mcp_config.json")
	reject := []Draft{
		{Name: "-flag", Transport: "stdio", Command: "x"},
		{Name: "a", Transport: "sse", URL: "https://e.test/sse"},
		{Name: "a", Transport: "stdio", Command: "x", Cwd: "/tmp"},
		{Name: "a", Transport: "http", URL: "https://e.test", EnvHeaders: map[string]string{"X": "VAR"}},
		{Name: "a", Transport: "http", URL: "https://e.test", BearerTokenEnv: "VAR"},
	}
	for _, d := range reject {
		if _, err := PrepareAdd(context.Background(), source, []Draft{d}); err == nil {
			t.Fatalf("draft accepted: %#v", d)
		}
	}
	drafts, err := ParseDrafts([]byte(`{"mcpServers":{"a":{"serverUrl":"https://e.test/mcp","disabled":false}}}`))
	if err != nil || len(drafts) != 1 || drafts[0].URL != "https://e.test/mcp" || drafts[0].Transport != "http" {
		t.Fatalf("drafts = %#v, err = %v", drafts, err)
	}
	if _, err := ParseDrafts([]byte(`{"mcpServers":{"a":{"command":"x","disabled":true}}}`)); err == nil {
		t.Fatal("disabled draft accepted")
	}
}

func TestDeleteGeminiKeepsOtherEntries(t *testing.T) {
	source := testSource(t, ProviderGemini, "mcp_config.json")
	const original = `{"keep":1,"mcpServers":{"a":{"command":"x"},"b":{"serverUrl":"https://e.test","disabled":true}}}`
	writeConfig(t, source, original)
	snapshot, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(context.Background(), source, selectServers(t, snapshot, "a", "b"), deleteTime); err != nil {
		t.Fatal(err)
	}
	assertConfig(t, source, "{\n  \"keep\": 1,\n  \"mcpServers\": {}\n}\n")
	assertBackup(t, source, original)
}
