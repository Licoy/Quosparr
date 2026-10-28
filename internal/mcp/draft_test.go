package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

func TestParseDraftsStrictAndSecretSafe(t *testing.T) {
	for _, raw := range []string{
		`{"mcpServers":{"one":{"command":"x"},"one":{"command":"y"}}}`,
		`{"command":"x","command":"SECRET"}`,
		`{"command":"x","SECRET":"secret"}`,
		`{"url":"https://host","headers":{"X-A":"a"},"http_headers":{"X-B":"b"}}`,
		`{"command":"x","args":null}`,
	} {
		_, err := ParseDrafts([]byte(raw))
		if err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("invalid input accepted or secret leaked")
		}
	}
	drafts, err := ParseDrafts([]byte(`{"mcp_servers":{"remote":{"url":"https://host/path?key=SECRET","http_headers":{"X-Key":"SECRET"}}}}`))
	if err != nil || len(drafts) != 1 || drafts[0].Headers["X-Key"] != "SECRET" {
		t.Fatal("supported input rejected")
	}
	_, err = ParseDrafts([]byte(strings.Repeat(`[`, 34) + `0` + strings.Repeat(`]`, 34)))
	if err == nil {
		t.Fatal("deep JSON accepted")
	}
}

func TestDraftConversionAndValidation(t *testing.T) {
	d := Draft{Name: "one", Transport: "http", URL: "https://host", Headers: map[string]string{"Authorization": "Bearer ${TOKEN}", "X-Full": "${HEADER}"}}
	config, err := draftConfig(ProviderCodex, d)
	if err != nil || config["bearer_token_env_var"] != "TOKEN" || config["env_http_headers"].(map[string]string)["X-Full"] != "HEADER" {
		t.Fatal("variable mapping failed")
	}
	d.Headers["X-Bad"] = "prefix-${TOKEN}"
	if _, err := draftConfig(ProviderCodex, d); err == nil {
		t.Fatal("complex interpolation silently converted")
	}
	for _, header := range []string{"Host", "Content-Length", "Mcp-Session-Id", "X\r\nBad"} {
		d.Headers = map[string]string{header: "value"}
		if ValidateDraft(d) == nil {
			t.Fatal("unsafe header accepted")
		}
	}
	d.Headers = map[string]string{"X": "bad\r\nvalue"}
	if ValidateDraft(d) == nil {
		t.Fatal("header injection accepted")
	}
	for _, url := range []string{"file:///tmp/a", "https://u:p@host", "https://host/#frag", "https://host/${TOKEN}"} {
		d.Headers = nil
		d.URL = url
		if ValidateDraft(d) == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	if _, err := draftConfig(ProviderClaude, Draft{Name: "x", Transport: "stdio", Command: "x", Cwd: "/tmp"}); err == nil {
		t.Fatal("Claude cwd silently dropped")
	}
	if _, err := draftConfig(ProviderCodex, Draft{Name: "x", Transport: "sse", URL: "https://host"}); err == nil {
		t.Fatal("Codex SSE accepted")
	}
}

func TestAddPreservesExistingAndBindsPlan(t *testing.T) {
	for _, provider := range []Provider{ProviderCodex, ProviderGrok, ProviderClaude} {
		t.Run(string(provider), func(t *testing.T) {
			source := testSource(t, provider, "config")
			original := "# keep comment\nanswer=42\n[mcp_servers.keep]\ncommand=\"old\"\nunknown=\"keep\"\n"
			if provider == ProviderClaude {
				original = `{"answer":9007199254740993,"projects":{"secret":"keep"},"mcpServers":{"keep":{"command":"old","unknown":"keep"}}}`
			}
			writeConfig(t, source, original)
			if err := os.Chmod(source.Path, 0644); err != nil {
				t.Fatal(err)
			}
			drafts := []Draft{{Name: "new", Transport: "stdio", Command: "server", Args: []string{"original"}, Env: map[string]string{"KEY": "SECRET"}}}
			plan, err := PrepareAdd(context.Background(), source, drafts)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(plan.Items[0].Details, "SECRET") {
				t.Fatal("secret in details")
			}
			drafts[0].Args[0] = "MUTATED"
			drafts[0].Env["KEY"] = "MUTATED"
			plan.Items[0].Name = "MUTATED"
			result, err := Add(context.Background(), source, plan, time.Now())
			if err != nil || len(result.Added) != 1 {
				t.Fatal("add failed", err)
			}
			data, err := os.ReadFile(source.Path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "original") || strings.Contains(string(data), "MUTATED") || !strings.Contains(string(data), "unknown") {
				t.Fatal("plan changed or existing data lost")
			}
			if provider != ProviderClaude && !strings.HasPrefix(string(data), original) {
				t.Fatal("TOML content changed")
			}
			if err := platform.VerifyOwnerOnly(source.Path, 0600); err != nil {
				t.Fatal(err)
			}
			again, err := PrepareAdd(context.Background(), source, []Draft{{Name: "new", Transport: "stdio", Command: "server", Args: []string{"original"}, Env: map[string]string{"KEY": "SECRET"}}})
			if err != nil || again.Items[0].Status != "exists" {
				t.Fatal("same configuration not recognized", err)
			}
			_, err = PrepareAdd(context.Background(), source, []Draft{{Name: "new", Transport: "stdio", Command: "different"}})
			if err == nil {
				t.Fatal("name conflict accepted")
			}
		})
	}
}

func TestAddMissingAndConcurrentChanges(t *testing.T) {
	source := testSource(t, ProviderCodex, "missing/home/config.toml")
	d := []Draft{{Name: "new", Transport: "stdio", Command: "server"}}
	plan, err := PrepareAdd(context.Background(), source, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(source.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("prepare created directory")
	}
	if _, err := Add(context.Background(), source, plan, time.Now()); err != nil {
		t.Fatal(err)
	}
	plan, err = PrepareAdd(context.Background(), source, []Draft{{Name: "next", Transport: "stdio", Command: "server"}})
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte("# external change\n")
	if err := os.WriteFile(source.Path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(context.Background(), source, plan, time.Now()); err == nil {
		t.Fatal("concurrent edit overwritten")
	}
	got, _ := os.ReadFile(source.Path)
	if string(got) != string(changed) {
		t.Fatal("changed despite failure")
	}
}

func TestAddUnsafeInsertAndBackupFailureWriteNothing(t *testing.T) {
	for _, original := range []string{`mcp_servers = { keep = { command = "x" } }`, "disabled_mcp_servers=[\"new\"]\n"} {
		source := testSource(t, ProviderGrok, "config.toml")
		writeConfig(t, source, original)
		_, err := PrepareAdd(context.Background(), source, []Draft{{Name: "new", Transport: "stdio", Command: "server"}})
		if err == nil {
			t.Fatal("unsafe insertion accepted")
		}
		got, _ := os.ReadFile(source.Path)
		if string(got) != original {
			t.Fatal("prepare wrote config")
		}
	}
	source := testSource(t, ProviderClaude, "config.json")
	writeConfig(t, source, `{"keep":true}`)
	plan, err := PrepareAdd(context.Background(), source, []Draft{{Name: "new", Transport: "stdio", Command: "server"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source.BackupDir, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(context.Background(), source, plan, time.Now()); err == nil {
		t.Fatal("backup failure ignored")
	}
	got, _ := os.ReadFile(source.Path)
	if string(got) != `{"keep":true}` {
		t.Fatal("written without backup")
	}
}

func TestAddMissingTargetAndParentIdentityRechecked(t *testing.T) {
	for _, change := range []string{"target", "parent", "ancestor", "symlink"} {
		t.Run(change, func(t *testing.T) {
			source := testSource(t, ProviderClaude, "config.json")
			parent := filepath.Dir(source.Path)
			if change == "ancestor" {
				source.Path = filepath.Join(parent, "missing", "config.json")
			}
			plan, err := PrepareAdd(context.Background(), source, []Draft{{Name: "new", Transport: "stdio", Command: "server"}})
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "target":
				if err := os.WriteFile(source.Path, []byte(`{"external":true}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent", "ancestor":
				if err := os.Rename(parent, parent+"-old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(parent, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				target := filepath.Join(t.TempDir(), "target")
				if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, source.Path); err != nil {
					t.Skip("symlinks unavailable")
				}
			}
			_, err = Add(context.Background(), source, plan, time.Now())
			if err == nil {
				t.Fatal("changed target accepted")
			}
			if change == "parent" || change == "ancestor" {
				var detail *DraftError
				if !errors.As(err, &detail) || detail.Code != "changed" || detail.Field != "parent" {
					t.Fatalf("parent replacement was not detected: %v", err)
				}
				if _, err := os.Lstat(source.Path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("configuration written after parent replacement: %v", err)
				}
			}
		})
	}
}

func TestDraftDefaultsAndSyntaxPosition(t *testing.T) {
	if !equivalentServer(map[string]any{"command": "x", "type": "stdio", "args": []any{}, "enabled": true}, map[string]any{"command": "x"}) {
		t.Fatal("documented defaults are not equivalent")
	}
	if equivalentServer(map[string]any{"command": "x", "enabled": false}, map[string]any{"command": "x"}) {
		t.Fatal("disabled entry treated as enabled")
	}
	_, err := ParseDrafts([]byte("{\n\"command\":\n}"))
	var detail *DraftError
	if !errors.As(err, &detail) || detail.Line < 2 || detail.Column < 1 {
		t.Fatal("missing safe syntax position")
	}
	drafts, err := ParseDrafts([]byte(`{"type":"streamable-http","url":"https://host"}`))
	if err != nil || drafts[0].Transport != "http" {
		t.Fatal("transport alias rejected")
	}
}

func TestAddCRLFAndInputsRemainUnchanged(t *testing.T) {
	source := testSource(t, ProviderCodex, "config.toml")
	original := "# keep\r\nmodel=\"example\"\r\n"
	writeConfig(t, source, original)
	root, servers, err := addDocument(source.Provider, []byte(original))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(root)
	output, err := encodeAdd(source.Provider, []byte(original), root, servers, map[string]any{"new": map[string]any{"command": "server"}})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(root)
	if string(before) != string(after) || len(servers) != 0 {
		t.Fatal("input maps mutated")
	}
	if strings.Contains(strings.ReplaceAll(string(output), "\r\n", ""), "\n") {
		t.Fatal("mixed line endings")
	}
	if sameConfig(math.NaN(), math.NaN()) {
		t.Fatal("failed encodings treated as equal")
	}
	if _, err := draftConfig(ProviderGrok, Draft{Name: "new", Transport: "http", URL: "https://host/sse"}); err == nil {
		t.Fatal("Grok HTTP silently became SSE")
	}
}

func TestDraftPreservesLiteralHeaderCharacters(t *testing.T) {
	d := Draft{Name: "new", Transport: "http", URL: "https://host/$path", Headers: map[string]string{"Cookie": "session=value", "X-Static": "cost$5{ok}"}}
	config, err := draftConfig(ProviderCodex, d)
	if err != nil {
		t.Fatal(err)
	}
	headers := config["http_headers"].(map[string]string)
	if headers["Cookie"] != "session=value" || headers["X-Static"] != "cost$5{ok}" {
		t.Fatal("literal header value changed")
	}
}
