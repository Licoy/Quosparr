package mcp

import (
	"bytes"
	"encoding/json"
	"github.com/BurntSushi/toml"
	"github.com/licoy/quosparr/internal/tomlstmt"
	"maps"
	"slices"
)

func addDocument(provider Provider, data []byte) (map[string]any, map[string]any, error) {
	root := map[string]any{}
	if provider == ProviderGemini && blankJSON(data) {
		data = nil
	}
	if len(data) > 0 {
		if jsonProvider(provider) {
			if err := validateUniqueJSON(data); err != nil {
				return nil, nil, draftError("syntax", "config")
			}
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.UseNumber()
			if decoder.Decode(&root) != nil || root == nil {
				return nil, nil, draftError("syntax", "config")
			}
		} else {
			var err error
			root, err = decodeTOML(data)
			if err != nil {
				return nil, nil, draftError("syntax", "config")
			}
		}
	}
	key := serversKey(provider)
	servers := map[string]any{}
	if raw, present := root[key]; present {
		var ok bool
		servers, ok = raw.(map[string]any)
		if !ok {
			return nil, nil, draftError("type", "servers")
		}
	}
	return root, servers, nil
}

func encodeAdd(provider Provider, data []byte, root, servers, additions map[string]any) ([]byte, error) {
	root = maps.Clone(root)
	servers = maps.Clone(servers)
	if len(additions) == 0 {
		return bytes.Clone(data), nil
	}
	key := serversKey(provider)
	for name, config := range additions {
		servers[name] = config
	}
	root[key] = servers
	var out []byte
	if provider == ProviderGemini {
		var err error
		if out, err = encodeGeminiAdd(data, additions); err != nil {
			return nil, err
		}
	} else if provider == ProviderClaude {
		var err error
		out, err = json.MarshalIndent(root, "", "  ")
		if err != nil {
			return nil, draftError("encode", "config")
		}
		out = append(out, '\n')
	} else {
		layout, err := layoutTOML(data)
		if err != nil {
			return nil, draftError("unsafe", "insertion")
		}
		var table []string
		for _, item := range layout.items {
			if item.Kind == tomlstmt.Header {
				table = item.Path
			}
			if item.Kind == tomlstmt.KeyVal && len(table) == 0 && len(item.Path) == 1 && item.Path[0] == key {
				return nil, draftError("unsafe", "insertion")
			}
		}
		var buf bytes.Buffer
		names := make([]string, 0, len(additions))
		for name := range additions {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			var entry bytes.Buffer
			if err := toml.NewEncoder(&entry).Encode(map[string]any{key: map[string]any{name: additions[name]}}); err != nil {
				return nil, draftError("encode", "config")
			}
			encoded := entry.Bytes()
			prefix := []byte("[mcp_servers]\n")
			if bytes.HasPrefix(encoded, prefix) {
				encoded = encoded[len(prefix):]
			}
			buf.Write(encoded)
		}
		eol := tomlstmt.DetectEOL(data)
		fragment := bytes.ReplaceAll(buf.Bytes(), []byte("\n"), []byte(eol))
		out = append(bytes.Clone(data), []byte(eol)...)
		out = append(out, fragment...)
	}
	after, _, err := addDocument(provider, out)
	if err != nil {
		return nil, draftError("unsafe", "insertion")
	}
	if !sameConfig(root, after) {
		return nil, draftError("unsafe", "insertion")
	}
	return out, nil
}

func jsonProvider(provider Provider) bool {
	return provider == ProviderClaude || provider == ProviderGemini
}

func serversKey(provider Provider) string {
	if jsonProvider(provider) {
		return "mcpServers"
	}
	return "mcp_servers"
}

// encodeGeminiAdd appends servers to the raw document so other keys and the
// existing entries keep their exact values; a blank file becomes a full one.
func encodeGeminiAdd(data []byte, additions map[string]any) ([]byte, error) {
	var document jsonDocument
	servers := map[string]json.RawMessage{}
	if !blankJSON(data) {
		var err error
		var existing map[string]json.RawMessage
		if document, existing, err = decodeJSON(data); err != nil {
			return nil, draftError("syntax", "config")
		}
		if existing != nil {
			servers = existing
		}
	}
	for name, config := range additions {
		raw, err := json.Marshal(config)
		if err != nil {
			return nil, draftError("encode", "config")
		}
		servers[name] = raw
	}
	out, err := encodeJSONDocument(ProviderGemini, document, servers)
	if err != nil {
		return nil, draftError("encode", "config")
	}
	return out, nil
}

// equivalentServer only removes documented defaults; all unknown fields remain
// part of the comparison so existing policies or credentials are never ignored.
func equivalentServer(a, b any) bool {
	normalize := func(value any) map[string]any {
		raw, _ := json.Marshal(value)
		var result map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&result) != nil {
			return nil
		}
		for _, key := range []string{"args", "env", "headers", "http_headers", "env_http_headers"} {
			switch v := result[key].(type) {
			case []any:
				if len(v) == 0 {
					delete(result, key)
				}
			case map[string]any:
				if len(v) == 0 {
					delete(result, key)
				}
			}
		}
		if enabled, ok := result["enabled"].(bool); ok && enabled {
			delete(result, "enabled")
		}
		if disabled, ok := result["disabled"].(bool); ok && !disabled {
			delete(result, "disabled")
		}
		if url, ok := result["serverUrl"]; ok {
			if _, exists := result["url"]; !exists {
				result["url"] = url
				delete(result, "serverUrl")
			}
		}
		inferred := "stdio"
		if url, ok := result["url"].(string); ok && url != "" {
			inferred = "http"
		}
		if result["type"] == inferred {
			delete(result, "type")
		}
		return result
	}
	left, right := normalize(a), normalize(b)
	return left != nil && right != nil && sameConfig(left, right)
}
