package mcp

import (
	"errors"
	"testing"
)

func TestParseDraftsRejectsNonStringLeaves(t *testing.T) {
	for _, field := range []string{"name", "transport", "type", "command", "url", "cwd", "bearer_token_env", "bearer_token_env_var", "args", "env", "headers", "http_headers", "env_headers", "env_http_headers"} {
		for _, invalid := range []string{"null", "42", "{}", "[]"} {
			value := invalid
			switch field {
			case "args":
				value = "[" + invalid + "]"
			case "env", "headers", "http_headers", "env_headers", "env_http_headers":
				value = `{"KEY":` + invalid + `}`
			}
			raw := []byte(`{"` + field + `":` + value + `}`)
			t.Run(field+"/"+invalid, func(t *testing.T) {
				_, err := ParseDrafts(raw)
				var detail *DraftError
				if !errors.As(err, &detail) || detail.Code != "type" {
					t.Fatalf("expected strict type error, got %v", err)
				}
			})
		}
	}
}

func TestParseDraftsPreservesEmptyStrings(t *testing.T) {
	raw := []byte(`{"name":"","transport":"","command":"","url":"","cwd":"","args":[""],"env":{"KEY":""},"headers":{"X-Key":""},"env_headers":{"X-Env":""},"bearer_token_env":""}`)
	drafts, err := ParseDrafts(raw)
	if err != nil || len(drafts) != 1 {
		t.Fatal("empty strings rejected", err)
	}
	d := drafts[0]
	if len(d.Args) != 1 || d.Args[0] != "" || len(d.Env) != 1 || d.Env["KEY"] != "" || len(d.Headers) != 1 || d.Headers["X-Key"] != "" || len(d.EnvHeaders) != 1 || d.EnvHeaders["X-Env"] != "" {
		t.Fatal("empty string values changed")
	}
	if _, err := ParseDrafts([]byte(`{"command":"x","args":[],"env":{},"headers":{},"env_headers":{}}`)); err != nil {
		t.Fatal("empty containers rejected", err)
	}
}
