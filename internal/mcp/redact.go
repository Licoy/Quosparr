package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const hidden = "<hidden>"

func summarizeConnection(provider Provider, config map[string]any) (transport, command, rawURL, problem string) {
	command, _ = config["command"].(string)
	rawURL, _ = config["url"].(string)
	if rawURL == "" && provider == ProviderGemini {
		rawURL, _ = config["serverUrl"].(string)
	}
	declared, _ := config["type"].(string)
	transport = declared
	if declared != "" && !knownTransport(declared) {
		transport = "unknown"
		return transport, command, rawURL, "invalidTransport"
	}
	if command != "" && rawURL != "" {
		return transport, command, rawURL, "multipleTransports"
	}
	if declared == "" {
		switch {
		case command != "":
			transport = "stdio"
		case rawURL != "" && provider == ProviderClaude:
			transport, problem = "unknown", "missingType"
		case rawURL != "":
			transport = "http"
		default:
			problem = "missingTransport"
		}
		return
	}
	if declared == "stdio" && command == "" || declared != "stdio" && rawURL == "" {
		problem = "transportMismatch"
	}
	return
}

func knownTransport(value string) bool {
	switch value {
	case "stdio", "http", "sse", "ws", "streamable-http":
		return true
	default:
		return false
	}
}

func safeCommand(command string) string {
	if command == "" {
		return ""
	}
	base := filepath.Base(command)
	if strings.ContainsAny(base, "$ {}?#&=:/\\") {
		return hidden
	}
	for _, r := range base {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("._+-", r) {
			return hidden
		}
	}
	return base
}

func safeURL(raw string) string {
	if strings.ContainsAny(raw, "${}") {
		return hidden
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if raw != "" {
			return hidden
		}
		return ""
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
}

func redactedDetails(config map[string]any) (string, error) {
	redacted := make(map[string]any, len(config))
	for key, value := range config {
		redacted[key] = redactField(strings.ToLower(key), value)
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(redacted); err != nil {
		return "", fmt.Errorf("encode redacted MCP details: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}

func redactField(key string, value any) any {
	switch key {
	case "command":
		if text, ok := value.(string); ok {
			return safeCommand(text)
		}
	case "url", "serverurl":
		if text, ok := value.(string); ok {
			return safeURL(text)
		}
	case "args":
		if list, ok := value.([]any); ok {
			return hiddenList(len(list))
		}
	case "env", "headers", "http_headers", "env_http_headers":
		if values, ok := stringMap(value); ok {
			out := make(map[string]string, len(values))
			for name := range values {
				out[name] = hidden
			}
			return out
		}
	case "enabled", "disabled", "required", "startup_timeout_sec", "tool_timeout_sec", "startup_timeout_ms", "tool_timeout_ms":
		return safePrimitive(value)
	case "enabled_tools", "disabled_tools", "env_vars":
		return safeStringList(value)
	case "type":
		if text, ok := value.(string); ok && knownTransport(text) {
			return text
		}
	}
	return shape(value)
}

func shape(value any) any {
	switch typed := value.(type) {
	case string:
		return hidden
	case bool, int, int64, float64, json.Number:
		return hidden
	case []any:
		return hiddenList(len(typed))
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return fmt.Sprintf("<object:%s>", strings.Join(keys, ","))
	default:
		return fmt.Sprintf("<%T>", value)
	}
}

func safePrimitive(value any) any {
	switch typed := value.(type) {
	case bool, int, int64, json.Number:
		return typed
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "<non-finite>"
		}
		return typed
	default:
		return shape(value)
	}
}

func safeStringList(value any) any {
	list, ok := value.([]any)
	if !ok {
		return shape(value)
	}
	out := make([]any, len(list))
	for i, item := range list {
		if text, ok := item.(string); ok {
			out[i] = text
		} else {
			out[i] = shape(item)
		}
	}
	return out
}

func hiddenList(size int) []string {
	out := make([]string, size)
	for i := range out {
		out[i] = hidden
	}
	return out
}

func stringMap(value any) (map[string]any, bool) {
	values, ok := value.(map[string]any)
	return values, ok
}

func boolValue(value any, fallback bool) bool {
	if boolean, ok := value.(bool); ok {
		return boolean
	}
	return fallback
}
