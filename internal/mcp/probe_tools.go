package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func probeTool(raw json.RawMessage) (Tool, error) {
	if !probeJSONSafe(raw) {
		return Tool{}, probeErr("invalid_response", "tools")
	}
	var t struct {
		Name, Title, Description string
		InputSchema              json.RawMessage `json:"inputSchema"`
	}
	if json.Unmarshal(raw, &t) != nil || t.Name == "" {
		return Tool{}, probeErr("invalid_response", "tools")
	}
	if len(t.InputSchema) > 32768 {
		return Tool{}, probeErr("response_too_large", "tools")
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if json.Unmarshal(t.InputSchema, &schema) != nil || schema.Type != "object" {
		return Tool{}, probeErr("invalid_response", "tools")
	}
	formatted, err := json.MarshalIndent(json.RawMessage(t.InputSchema), "", "  ")
	if err != nil {
		return Tool{}, probeErr("invalid_response", "tools")
	}
	if len(formatted) > 32768 {
		return Tool{}, probeErr("response_too_large", "tools")
	}
	out := Tool{Name: t.Name, Title: t.Title, Description: t.Description, InputSchema: string(formatted)}
	keys := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(out.Parameters) >= 100 {
			break
		}
		var p struct {
			Type        json.RawMessage `json:"type"`
			Description string          `json:"description"`
		}
		if json.Unmarshal(schema.Properties[key], &p) != nil {
			return Tool{}, probeErr("invalid_response", "tools")
		}
		typ := strings.Trim(string(p.Type), "\"")
		required := false
		for _, k := range schema.Required {
			if k == key {
				required = true
			}
		}
		out.Parameters = append(out.Parameters, ToolParameter{key, typ, p.Description, required})
	}
	return out, nil
}
func probeText(s string, secrets []string) string { return probeCleaner(secrets)(s) }

func probeCleaner(secrets []string) func(string) string {
	unique := map[string]bool{}
	for _, v := range secrets {
		if v != "" {
			unique[v] = true
			escaped, _ := json.Marshal(v)
			if len(escaped) > 2 {
				unique[string(escaped[1:len(escaped)-1])] = true
			}
		}
	}
	values := make([]string, 0, len(unique))
	for v := range unique {
		values = append(values, v)
	}
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) == len(values[j]) {
			return values[i] < values[j]
		}
		return len(values[i]) > len(values[j])
	})
	pairs := make([]string, 0, len(values)*2)
	for _, v := range values {
		pairs = append(pairs, v, "[redacted]")
	}
	replacer := strings.NewReplacer(pairs...)
	return func(s string) string {
		// NewReplacer visits original input once, never reprocesses replacement text.
		s = replacer.Replace(s)
		s = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, s)
		if len(s) > 32768 {
			s = s[:32768]
			for !utf8.ValidString(s) {
				s = s[:len(s)-1]
			}
		}
		return s
	}
}

// Reuse the bounded, duplicate-key-rejecting JSON walker used for draft input.
// This caps indentation growth before schema pretty-printing, without resolving
// remote schemas or following references.
func probeJSONSafe(raw []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if draftJSONValue(dec, 0) != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}
