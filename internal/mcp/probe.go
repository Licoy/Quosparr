package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

const probeVersion = "2026-07-28"
const probeLimit = 2 << 20
const probeToolLimit = 500

type ProbeResult struct {
	ServerName, ServerVersion, ProtocolVersion string
	Tools                                      []Tool
	Truncated                                  bool
	CleanupFailed                              bool
}
type Tool struct {
	Name, Title, Description, InputSchema string
	Parameters                            []ToolParameter
}
type ToolParameter struct {
	Name, Type, Description string
	Required                bool
}

// ProbeError contains only stable classifications, never server or OS error text.
type ProbeError struct {
	Code, Stage                   string
	HTTPStatus, ExitCode, RPCCode int
	legacySupported               bool
}

func (e *ProbeError) Error() string           { return "mcp_probe_" + e.Code }
func probeErr(code, stage string) *ProbeError { return &ProbeError{Code: code, Stage: stage} }

type probeRPCError struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}
type probeEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *probeRPCError  `json:"error"`
	Method  string          `json:"method"`
}
type probeWire struct {
	call          func(context.Context, string, map[string]any, bool) (json.RawMessage, error)
	close         func()
	version       string
	cleanupFailed bool
}

// Probe contacts only the server explicitly supplied by the caller and lists tools;
// it never invokes a tool or advertises access to local files or user interaction.
func Probe(ctx context.Context, draft Draft) (result ProbeResult, err error) {
	if ValidateDraft(draft) != nil {
		return result, probeErr("invalid_draft", "connect")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	defer func() {
		if ctx.Err() != nil {
			code := "cancelled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				code = "timeout"
			}
			stage := "connect"
			var existing *ProbeError
			if errors.As(err, &existing) {
				stage = existing.Stage
			}
			err = probeErr(code, stage)
		}
	}()
	draft, secrets, expandErr := probeExpandDraft(draft)
	if expandErr != nil {
		return result, expandErr
	}
	if ValidateDraft(draft) != nil {
		return result, probeErr("invalid_draft", "connect")
	}
	headers := map[string]string{}
	for k, v := range draft.Headers {
		headers[k] = v
	}
	for k, key := range draft.EnvHeaders {
		v, ok := os.LookupEnv(key)
		if !ok || v == "" {
			return result, probeErr("missing_env", "connect")
		}
		headers[k] = v
	}
	if draft.BearerTokenEnv != "" {
		v, ok := os.LookupEnv(draft.BearerTokenEnv)
		if !ok || v == "" {
			return result, probeErr("missing_env", "connect")
		}
		headers["Authorization"] = "Bearer " + v
		secrets = append(secrets, v)
	}
	for k, v := range headers {
		secrets = append(secrets, v)
		if strings.EqualFold(k, "Authorization") {
			parts := strings.Fields(v)
			if len(parts) > 1 {
				secrets = append(secrets, parts[1:]...)
			}
		}
	}

	for _, v := range draft.Env {
		secrets = append(secrets, v)
	}
	var wire *probeWire
	if draft.Transport == "stdio" {
		wire, err = newProbeProcess(ctx, draft)
	} else {
		wire, err = newProbeHTTP(ctx, draft, headers)
	}
	if err != nil {
		return result, err
	}
	defer func() { wire.close(); result.CleanupFailed = wire.cleanupFailed }()
	result, err = probeList(ctx, wire, draft.Transport)
	if err != nil {
		return ProbeResult{}, err
	}
	clean := probeCleaner(secrets)
	result.ServerName = clean(result.ServerName)
	result.ServerVersion = clean(result.ServerVersion)
	for i := range result.Tools {
		t := &result.Tools[i]
		t.Name = clean(t.Name)
		t.Title = clean(t.Title)
		t.Description = clean(t.Description)
		t.InputSchema = clean(t.InputSchema)
		for j := range t.Parameters {
			p := &t.Parameters[j]
			p.Name = clean(p.Name)
			p.Type = clean(p.Type)
			p.Description = clean(p.Description)
		}
	}
	return result, nil
}

func modernProbeError(err error) bool {
	var p *ProbeError
	return errors.As(err, &p) && (p.RPCCode == -32020 || p.RPCCode == -32021 || p.RPCCode == -32022)
}
func probeList(ctx context.Context, w *probeWire, transport string) (ProbeResult, error) {
	r := ProbeResult{ProtocolVersion: probeVersion}
	w.version = probeVersion
	discoverCtx := ctx
	var cancel context.CancelFunc
	if transport == "stdio" {
		discoverCtx, cancel = context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
	}
	var raw json.RawMessage
	var err error
	if transport != "sse" {
		raw, err = w.call(discoverCtx, "server/discover", map[string]any{}, false)
	}
	legacy := transport == "sse"
	if err != nil {
		var p *ProbeError
		if errors.As(err, &p) && !modernProbeError(err) {
			legacy = transport == "stdio" && (p.Code == "rpc" || p.Code == "timeout") || p.HTTPStatus == 400
		}
		if errors.As(err, &p) && p.RPCCode == -32022 && p.legacySupported {
			legacy = true
		}
		if !legacy {
			return r, err
		}
	}
	var info struct {
		ProtocolVersion   string                         `json:"protocolVersion"`
		SupportedVersions []string                       `json:"supportedVersions"`
		Capabilities      map[string]json.RawMessage     `json:"capabilities"`
		ServerInfo        struct{ Name, Version string } `json:"serverInfo"`
		Meta              struct {
			ServerInfo struct{ Name, Version string } `json:"io.modelcontextprotocol/serverInfo"`
		} `json:"_meta"`
	}
	if legacy {
		w.version = "2025-11-25"
		raw, err = w.call(ctx, "initialize", map[string]any{"protocolVersion": w.version, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "Quosparr", "version": "1"}}, false)
	}
	if err != nil {
		return r, err
	}
	if json.Unmarshal(raw, &info) != nil || info.Capabilities == nil {
		return r, probeErr("invalid_response", "discover")
	}
	if legacy {
		switch info.ProtocolVersion {
		case "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05":
		default:
			return r, probeErr("protocol", "initialize")
		}
		w.version = info.ProtocolVersion
		if _, err = w.call(ctx, "notifications/initialized", map[string]any{}, true); err != nil {
			return r, err
		}
		r.ServerName, r.ServerVersion = info.ServerInfo.Name, info.ServerInfo.Version
	} else {
		found := false
		for _, v := range info.SupportedVersions {
			if v == probeVersion {
				found = true
			}
		}
		if !found {
			return r, probeErr("protocol", "discover")
		}
		r.ServerName, r.ServerVersion = info.Meta.ServerInfo.Name, info.Meta.ServerInfo.Version
	}
	r.ProtocolVersion = w.version
	toolsCapability, ok := info.Capabilities["tools"]
	if !ok {
		return r, nil
	}
	var toolsObject map[string]json.RawMessage
	if json.Unmarshal(toolsCapability, &toolsObject) != nil || toolsObject == nil {
		return r, probeErr("invalid_response", "discover")
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 20; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err = w.call(ctx, "tools/list", params, false)
		if err != nil {
			return r, err
		}
		var list struct {
			Tools      []json.RawMessage `json:"tools"`
			NextCursor string            `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &list) != nil || list.Tools == nil {
			return r, probeErr("invalid_response", "tools")
		}
		for _, data := range list.Tools {
			t, e := probeTool(data)
			if e != nil {
				return r, e
			}
			if len(r.Tools) >= probeToolLimit {
				r.Truncated = true
				return r, nil
			}
			r.Tools = append(r.Tools, t)
		}
		if list.NextCursor == "" {
			return r, nil
		}
		if seen[list.NextCursor] {
			return r, probeErr("invalid_response", "tools")
		}
		seen[list.NextCursor] = true
		if len(list.NextCursor) > 4096 {
			return r, probeErr("response_too_large", "tools")
		}
		cursor = list.NextCursor
	}
	r.Truncated = true
	return r, nil
}
