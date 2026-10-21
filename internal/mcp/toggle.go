package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/tomlstmt"
)

var errToggleUnsupported = errors.New("this MCP entry cannot be switched safely here; edit the configuration file directly")

// tomlLayout locates the statements a toggle edits: each server's own
// [mcp_servers.<name>] table with its enabled key, and the root
// disabled_mcp_servers list Grok also consults.
type tomlLayout struct {
	items       []tomlstmt.Item
	servers     map[string]serverLayout
	list        int
	lastRootKey int
	firstHeader int
}

type serverLayout struct {
	header, enabled, lastKey int
}

func layoutTOML(data []byte) (tomlLayout, error) {
	items, err := tomlstmt.Lex(data)
	if err != nil {
		return tomlLayout{}, err
	}
	layout := tomlLayout{items: items, servers: map[string]serverLayout{}, list: -1, lastRootKey: -1, firstHeader: len(items)}
	var table []string
	current := ""
	for i, item := range items {
		switch item.Kind {
		case tomlstmt.Header:
			layout.firstHeader = min(layout.firstHeader, i)
			table, current = item.Path, ""
			if !item.Array && len(item.Path) == 2 && item.Path[0] == "mcp_servers" {
				current = item.Path[1]
				layout.servers[current] = serverLayout{header: i, enabled: -1, lastKey: -1}
			}
		case tomlstmt.KeyVal:
			if len(table) == 0 {
				layout.lastRootKey = i
				if len(item.Path) == 1 && item.Path[0] == "disabled_mcp_servers" {
					layout.list = i
				}
				continue
			}
			if current == "" {
				continue
			}
			server := layout.servers[current]
			server.lastKey = i
			if len(item.Path) == 1 && item.Path[0] == "enabled" {
				server.enabled = i
			}
			layout.servers[current] = server
		}
	}
	return layout, nil
}

// toggleable reports whether name has its own table, a plain boolean
// enabled key if any, and, for Grok, a disabled list in a form that can be
// rewritten.
func (l tomlLayout) toggleable(provider Provider, root map[string]any, name string) bool {
	server, ok := l.servers[name]
	config := serverConfig(root, name)
	if !ok || config == nil {
		return false
	}
	if value, exists := config["enabled"]; exists {
		if _, isBool := value.(bool); !isBool || server.enabled < 0 {
			return false
		}
	}
	if provider == ProviderGrok {
		if _, ok := disabledList(root); !ok {
			return false
		}
		if _, exists := root["disabled_mcp_servers"]; exists && l.list < 0 {
			return false
		}
	}
	return true
}

func serverConfig(root map[string]any, name string) map[string]any {
	servers, _ := root["mcp_servers"].(map[string]any)
	config, _ := servers[name].(map[string]any)
	return config
}

// disabledList reads Grok's root disabled_mcp_servers; false means it is
// present but not a list of names.
func disabledList(root map[string]any) ([]string, bool) {
	raw, exists := root["disabled_mcp_servers"]
	if !exists {
		return nil, true
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	names := make([]string, len(values))
	for i, value := range values {
		if names[i], ok = value.(string); !ok {
			return nil, false
		}
	}
	return names, true
}

func setEnabledTOML(data []byte, provider Provider, name string, enabled bool) ([]byte, error) {
	before, err := decodeTOML(data)
	if err != nil {
		return nil, err
	}
	if serverConfig(before, name) == nil {
		return nil, fmt.Errorf("MCP server %q was not found", name)
	}
	layout, err := layoutTOML(data)
	if err != nil || !layout.toggleable(provider, before, name) {
		return nil, errToggleUnsupported
	}
	server, eol := layout.servers[name], tomlstmt.DetectEOL(data)
	var edits []tomlstmt.Edit
	switch {
	case server.enabled >= 0:
		edit, ok := boolEdit(data, layout.items[server.enabled], enabled)
		if !ok {
			return nil, errToggleUnsupported
		}
		edits = append(edits, edit)
	case !enabled:
		at := server.header
		if server.lastKey >= 0 {
			at = server.lastKey
		}
		edits = append(edits, insertAfter(data, layout.items[at], "enabled = false", eol))
	}
	var list []string
	if provider == ProviderGrok {
		names, _ := disabledList(before)
		list = names
		switch {
		case enabled:
			list = slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == name })
		case !slices.Contains(names, name):
			list = append(slices.Clone(names), name)
		}
		if !slices.Equal(list, names) {
			edits = append(edits, layout.listEdit(data, list, eol))
		}
	}
	out := tomlstmt.ApplyEdits(data, edits)
	after, err := decodeTOML(out)
	if err != nil || !toggleExpected(provider, before, after, name, enabled, list) {
		return nil, errors.New("TOML edit changed configuration outside the selection")
	}
	return out, nil
}

// boolEdit replaces only the true / false token of an enabled statement,
// keeping its indentation and trailing comment.
func boolEdit(data []byte, item tomlstmt.Item, value bool) (tomlstmt.Edit, bool) {
	eq := bytes.IndexByte(data[item.Start:item.End], '=')
	if eq < 0 {
		return tomlstmt.Edit{}, false
	}
	p := item.Start + eq + 1
	for p < item.End && (data[p] == ' ' || data[p] == '\t') {
		p++
	}
	for _, token := range []string{"true", "false"} {
		if bytes.HasPrefix(data[p:item.End], []byte(token)) {
			return tomlstmt.Edit{Start: p, End: p + len(token), Text: strconv.FormatBool(value)}, true
		}
	}
	return tomlstmt.Edit{}, false
}

func insertAfter(data []byte, item tomlstmt.Item, line, eol string) tomlstmt.Edit {
	text := line + eol
	if item.End == item.Start || data[item.End-1] != '\n' {
		text = eol + text
	}
	return tomlstmt.Edit{Start: item.End, End: item.End, Text: text}
}

// listEdit rewrites disabled_mcp_servers to names, removing it when empty
// and otherwise adding it among the root keys, ahead of the first table
// and the comments attached to it.
func (l tomlLayout) listEdit(data []byte, names []string, eol string) tomlstmt.Edit {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = tomlstmt.Quote(name)
	}
	line := "disabled_mcp_servers = [" + strings.Join(quoted, ", ") + "]"
	if l.list >= 0 {
		item := l.items[l.list]
		if len(names) == 0 {
			end := item.End
			if next := l.list + 1; next < len(l.items) && l.items[next].Kind == tomlstmt.Blank &&
				(l.list == 0 || l.items[l.list-1].Kind == tomlstmt.Blank) {
				end = l.items[next].End
			}
			return tomlstmt.Edit{Start: item.Start, End: end}
		}
		if item.End > item.Start && data[item.End-1] == '\n' {
			line += eol
		}
		return tomlstmt.Edit{Start: item.Start, End: item.End, Text: line}
	}
	if l.lastRootKey >= 0 {
		return insertAfter(data, l.items[l.lastRootKey], line, eol)
	}
	at := l.firstHeader
	for at > 0 && l.items[at-1].Kind == tomlstmt.Comment {
		at--
	}
	start := len(data)
	if at < len(l.items) {
		start = l.items[at].Start
	}
	return tomlstmt.Edit{Start: start, End: start, Text: line + eol + eol}
}

func toggleExpected(provider Provider, before, after map[string]any, name string, enabled bool, list []string) bool {
	config := serverConfig(after, name)
	if config == nil {
		return false
	}
	value, exists := config["enabled"]
	if enabled && exists && value != true || !enabled && value != false {
		return false
	}
	grok := provider == ProviderGrok
	if grok {
		if got, ok := disabledList(after); !ok || !slices.Equal(got, list) {
			return false
		}
	}
	return reflect.DeepEqual(withoutToggle(before, name, grok), withoutToggle(after, name, grok))
}

func withoutToggle(root map[string]any, name string, list bool) map[string]any {
	out := maps.Clone(root)
	if list {
		delete(out, "disabled_mcp_servers")
	}
	servers := maps.Clone(out["mcp_servers"].(map[string]any))
	config := maps.Clone(serverConfig(root, name))
	delete(config, "enabled")
	servers[name] = config
	out["mcp_servers"] = servers
	return out
}

func setEnabled(ctx context.Context, source Source, server Server, enabled bool, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateSource(source, true); err != nil {
		return err
	}
	if source.Provider == ProviderClaude || !server.Toggleable {
		return errToggleUnsupported
	}
	original, info, exists, err := readRegular(source.Path)
	if err != nil || !exists {
		if err == nil {
			err = errors.New("MCP configuration no longer exists")
		}
		return sanitizeError(err)
	}
	if err := validateServer(source, server, info); err != nil {
		return err
	}
	if server.Fingerprint == "" || fileFingerprint(original, info) != server.Fingerprint {
		return errors.New("MCP configuration changed since it was scanned")
	}
	edit := func(data []byte) ([]byte, error) { return setEnabledTOML(data, source.Provider, server.Name, enabled) }
	out, err := edit(original)
	if err != nil {
		return err
	}
	if bytes.Equal(out, original) {
		return nil
	}
	if err := rewrite(ctx, source, original, info, server.Fingerprint, now, edit); err != nil {
		return err
	}
	snapshot, err := load(context.Background(), source)
	if err != nil {
		return errors.New("written MCP configuration could not be verified")
	}
	for _, written := range snapshot.Servers {
		if written.Name == server.Name && written.Enabled == enabled {
			return nil
		}
	}
	return fmt.Errorf("MCP server %q did not change state", server.Name)
}
