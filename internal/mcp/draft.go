package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

// Draft contains only supported, in-memory connection settings; values are never logged.
type Draft struct {
	Name, Transport, Command, URL, Cwd string
	Args                               []string
	Env, Headers, EnvHeaders           map[string]string
	BearerTokenEnv                     string
}

// DraftError contains stable localization keys, never user input.
type DraftError struct {
	Code, Field  string
	Line, Column int
}

func (e *DraftError) Error() string       { return "MCP " + e.Code + ": " + e.Field }
func draftError(code, field string) error { return &DraftError{Code: code, Field: field} }

// ParseDrafts accepts a single connection or a CLI server map wrapper.
func ParseDrafts(data []byte) ([]Draft, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return nil, draftError("size", "json")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := draftJSONValue(dec, 0); err != nil {
		if e, ok := err.(*DraftError); ok && e.Code == "syntax" {
			e.Line, e.Column = jsonPosition(data, dec.InputOffset())
		}
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		line, column := jsonPosition(data, dec.InputOffset())
		return nil, &DraftError{Code: "syntax", Field: "json", Line: line, Column: column}
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil || root == nil {
		return nil, draftError("object", "json")
	}
	var wrapped json.RawMessage
	for _, key := range []string{"mcpServers", "mcp_servers"} {
		if value, ok := root[key]; ok {
			if wrapped != nil {
				return nil, draftError("conflict", "json")
			}
			wrapped = value
		}
	}
	if wrapped == nil {
		d, err := parseDraft(root)
		return []Draft{d}, err
	}
	if len(root) != 1 {
		return nil, draftError("unsupported", "json")
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(wrapped, &servers) != nil || len(servers) == 0 || len(servers) > 100 {
		return nil, draftError("object", "servers")
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Draft, 0, len(names))
	for _, name := range names {
		var obj map[string]json.RawMessage
		if json.Unmarshal(servers[name], &obj) != nil || obj == nil {
			return nil, draftError("object", "server")
		}
		d, err := parseDraft(obj)
		if err != nil {
			return nil, err
		}
		if d.Name != "" && d.Name != name {
			return nil, draftError("conflict", "name")
		}
		d.Name = name
		out = append(out, d)
	}
	return out, nil
}

func draftJSONValue(dec *json.Decoder, depth int) error {
	if depth > 32 {
		return draftError("depth", "json")
	}
	token, err := dec.Token()
	if err != nil {
		return draftError("syntax", "json")
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	for dec.More() {
		if delimiter == '{' {
			key, err := dec.Token()
			if err != nil {
				return draftError("syntax", "json")
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return draftError("duplicate", "json")
			}
			seen[name] = true
		}
		if err := draftJSONValue(dec, depth+1); err != nil {
			return err
		}
	}
	end, err := dec.Token()
	if err != nil || (delimiter == '{' && end != json.Delim('}')) || (delimiter == '[' && end != json.Delim(']')) {
		return draftError("syntax", "json")
	}
	return nil
}

func parseDraft(obj map[string]json.RawMessage) (Draft, error) {
	var d Draft
	seen := map[string]bool{}
	for key, raw := range obj {
		canonical := key
		switch key {
		case "type":
			canonical = "transport"
		case "http_headers":
			canonical = "headers"
		case "env_http_headers":
			canonical = "env_headers"
		case "bearer_token_env_var":
			canonical = "bearer_token_env"
		}
		if seen[canonical] {
			return d, draftError("conflict", canonical)
		}
		seen[canonical] = true
		var target any
		switch canonical {
		case "name":
			target = &d.Name
		case "transport":
			target = &d.Transport
		case "command":
			target = &d.Command
		case "url":
			target = &d.URL
		case "cwd":
			target = &d.Cwd
		case "args":
			target = &d.Args
		case "env":
			target = &d.Env
		case "headers":
			target = &d.Headers
		case "env_headers":
			target = &d.EnvHeaders
		case "bearer_token_env":
			target = &d.BearerTokenEnv
		default:
			return d, draftError("unsupported", "field")
		}
		if !strictDraftValue(raw, target) {
			return d, draftError("type", canonical)
		}
	}
	if d.Transport == "" {
		if d.URL != "" {
			d.Transport = "http"
		} else {
			d.Transport = "stdio"
		}
	}
	d.Transport = strings.ToLower(d.Transport)
	if d.Transport == "streamable-http" || d.Transport == "streamable_http" {
		d.Transport = "http"
	}
	return d, nil
}

func jsonPosition(data []byte, offset int64) (int, int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	line, column := 1, 1
	for _, b := range data[:offset] {
		if b == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}

// strictDraftValue checks every leaf before encoding/json can coerce null into
// a string's zero value; empty strings, arrays and maps remain valid inputs.
func strictDraftValue(raw json.RawMessage, target any) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch target.(type) {
	case *string:
		if _, ok := value.(string); !ok {
			return false
		}
	case *[]string:
		values, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range values {
			if _, ok := item.(string); !ok {
				return false
			}
		}
	case *map[string]string:
		values, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for _, item := range values {
			if _, ok := item.(string); !ok {
				return false
			}
		}
	default:
		return false
	}
	return json.Unmarshal(raw, target) == nil
}
