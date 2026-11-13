package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

type jsonDocument map[string]json.RawMessage

func loadJSON(source Source, data []byte, fingerprint string) ([]Server, error) {
	_, servers, err := decodeJSON(data)
	if err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(servers))
	for name, raw := range servers {
		config, err := decodeJSONObject(raw)
		if err != nil {
			return nil, errors.New("MCP server entry must be an object")
		}
		server, err := baseServer(source, name, fingerprint, config)
		if err != nil {
			return nil, err
		}
		if source.Provider == ProviderGemini {
			disabled, exists := config["disabled"]
			flag, isBool := disabled.(bool)
			server.Enabled = !flag
			server.Toggleable = !exists || isBool
		}
		out = append(out, server)
	}
	return out, nil
}

func decodeJSON(data []byte) (jsonDocument, map[string]json.RawMessage, error) {
	if err := validateUniqueJSON(data); err != nil {
		return nil, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document jsonDocument
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, nil, errors.New("invalid JSON object")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, nil, errors.New("JSON has trailing content")
	}
	raw, exists := document["mcpServers"]
	if !exists {
		return document, nil, nil
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
		return nil, nil, errors.New("mcpServers must be an object")
	}
	return document, servers, nil
}

func validateUniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := validateJSONValue(decoder); err != nil {
		return errors.New("invalid JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("JSON has trailing content")
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("duplicate JSON object key")
			}
			seen[key] = struct{}{}
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

// blankJSON reports a file Gemini treats as having no servers yet.
func blankJSON(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

// setEnabledJSON flips one Gemini server's disabled flag, keeping every other
// top-level key and server field verbatim.
func setEnabledJSON(data []byte, name string, enabled bool) ([]byte, error) {
	document, servers, err := decodeJSON(data)
	if err != nil {
		return nil, err
	}
	raw, ok := servers[name]
	if !ok {
		return nil, fmt.Errorf("MCP server %q was not found", name)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errToggleUnsupported
	}
	if current, exists := fields["disabled"]; exists {
		var flag bool
		if err := json.Unmarshal(current, &flag); err != nil {
			return nil, errToggleUnsupported
		}
		if flag == !enabled {
			return data, nil
		}
	}
	fields["disabled"] = json.RawMessage(strconv.FormatBool(!enabled))
	if servers[name], err = json.Marshal(fields); err != nil {
		return nil, errors.New("cannot encode MCP server")
	}
	out, err := encodeJSONDocument(ProviderGemini, document, servers)
	if err != nil {
		return nil, err
	}
	if !jsonToggleExpected(data, out, name, enabled) {
		return nil, errors.New("JSON edit changed configuration outside the selection")
	}
	return out, nil
}

// encodeJSONDocument writes the document the way the official clients do:
// two-space indentation, sorted keys and a final newline, with nested values
// kept as they were read.
// Gemini also rewrites each server object with sorted fields; Claude keeps
// server objects exactly as read.
func encodeJSONDocument(provider Provider, document jsonDocument, servers map[string]json.RawMessage) ([]byte, error) {
	if provider == ProviderGemini {
		for name, raw := range servers {
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil {
				continue
			}
			sorted, err := json.Marshal(fields)
			if err != nil {
				return nil, errors.New("cannot encode MCP server")
			}
			servers[name] = sorted
		}
	}
	replacement, err := json.Marshal(servers)
	if err != nil {
		return nil, errors.New("cannot encode mcpServers")
	}
	if document == nil {
		document = jsonDocument{}
	}
	document["mcpServers"] = replacement
	out, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, errors.New("cannot encode configuration")
	}
	return append(out, '\n'), nil
}

func jsonToggleExpected(before, after []byte, name string, enabled bool) bool {
	old, err := decodeJSONObject(before)
	if err != nil {
		return false
	}
	updated, err := decodeJSONObject(after)
	if err != nil {
		return false
	}
	config := jsonServer(updated, name)
	if config == nil || config["disabled"] != !enabled {
		return false
	}
	delete(config, "disabled")
	if previous := jsonServer(old, name); previous != nil {
		delete(previous, "disabled")
	}
	return reflect.DeepEqual(old, updated)
}

func jsonServer(root map[string]any, name string) map[string]any {
	servers, _ := root["mcpServers"].(map[string]any)
	config, _ := servers[name].(map[string]any)
	return config
}

func decodeJSONObject(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("not an object")
	}
	return value, nil
}

func deleteJSON(data []byte, provider Provider, names map[string]struct{}) ([]byte, error) {
	document, servers, err := decodeJSON(data)
	if err != nil {
		return nil, err
	}
	for name := range names {
		if _, ok := servers[name]; !ok {
			return nil, fmt.Errorf("MCP server %q was not found", name)
		}
		delete(servers, name)
	}
	out, err := encodeJSONDocument(provider, document, servers)
	if err != nil {
		return nil, err
	}
	_, remaining, err := decodeJSON(out)
	if err != nil {
		return nil, errors.New("edited JSON is invalid")
	}
	for name := range names {
		if _, ok := remaining[name]; ok {
			return nil, fmt.Errorf("MCP server %q was not removed", name)
		}
	}
	return out, nil
}
