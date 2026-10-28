package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

type probeFrame struct {
	event, data string
	err         error
}

func newProbeHTTP(ctx context.Context, draft Draft, headers map[string]string) (*probeWire, error) {
	shared, err := netx.ClientFor(ctx, netx.WithoutCookies)
	if err != nil {
		return nil, probeErr("network", "connect")
	}
	client := *shared
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for k, v := range headers {
		if !probeHeaderAllowed(k, v) {
			return nil, probeErr("invalid_draft", "connect")
		}
	}
	w := &probeWire{}
	var id int
	session := ""
	endpoint := draft.URL
	streamCtx, streamCancel := context.WithCancel(ctx)
	var stream io.ReadCloser
	var closeOnce sync.Once
	w.close = func() {
		closeOnce.Do(func() {
			streamCancel()
			if stream != nil {
				stream.Close()
			}
			if session != "" && draft.Transport == "http" {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				defer cancel()
				req, e := http.NewRequestWithContext(cleanupCtx, http.MethodDelete, endpoint, nil)
				if e != nil {
					w.cleanupFailed = true
					return
				}
				probeHeaders(req, headers)
				req.Header.Set("Mcp-Session-Id", session)
				req.Header.Set("MCP-Protocol-Version", w.version)
				resp, e := client.Do(req)
				if e != nil {
					w.cleanupFailed = true
					return
				}
				resp.Body.Close()
				w.cleanupFailed = !(resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == 404 || resp.StatusCode == 405)
			}
		})
	}

	var events chan probeFrame
	if draft.Transport == "sse" {
		req, e := http.NewRequestWithContext(streamCtx, http.MethodGet, draft.URL, nil)
		if e != nil {
			return nil, probeErr("invalid_draft", "connect")
		}
		probeHeaders(req, headers)
		req.Header.Set("Accept", "text/event-stream")
		resp, e := client.Do(req)
		if e != nil {
			return nil, probeContextError(ctx, "connect", e)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, &ProbeError{Code: "http_status", Stage: "connect", HTTPStatus: resp.StatusCode}
		}
		if probeMedia(resp.Header.Get("Content-Type")) != "text/event-stream" {
			resp.Body.Close()
			return nil, probeErr("invalid_response", "connect")
		}
		stream = resp.Body
		events = make(chan probeFrame, 32)
		go probeSSE(streamCtx, resp.Body, events)
		select {
		case <-ctx.Done():
			w.close()
			return nil, probeContextError(ctx, "connect", ctx.Err())
		case frame := <-events:
			if frame.err != nil || frame.event != "endpoint" {
				w.close()
				return nil, probeErr("invalid_response", "connect")
			}
			base, _ := url.Parse(draft.URL)
			next, e := url.Parse(frame.data)
			if e != nil {
				w.close()
				return nil, probeErr("invalid_response", "connect")
			}
			next = base.ResolveReference(next)
			if !strings.EqualFold(base.Scheme, next.Scheme) || !strings.EqualFold(base.Host, next.Host) || next.User != nil || next.Fragment != "" {
				w.close()
				return nil, probeErr("invalid_response", "connect")
			}
			endpoint = next.String()
		}
	}
	w.call = func(callCtx context.Context, method string, params map[string]any, notification bool) (json.RawMessage, error) {
		id++
		current := id
		payload := probeRequest(w.version, method, params, current, notification)
		req, e := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if e != nil {
			return nil, probeErr("invalid_draft", probeStage(method))
		}
		probeHeaders(req, headers)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if w.version != "" {
			req.Header.Set("MCP-Protocol-Version", w.version)
		}
		if w.version == probeVersion {
			req.Header.Set("Mcp-Method", method)
		}
		if session != "" {
			req.Header.Set("Mcp-Session-Id", session)
		}
		resp, e := client.Do(req)
		if e != nil {
			return nil, probeContextError(callCtx, probeStage(method), e)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			out := &ProbeError{Code: "http_status", Stage: probeStage(method), HTTPStatus: resp.StatusCode}
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, probeLimit+1))
			var env probeEnvelope
			if len(raw) <= probeLimit && json.Unmarshal(raw, &env) == nil && env.JSONRPC == "2.0" && env.Error != nil {
				out.RPCCode = env.Error.Code
				out.legacySupported = probeLegacySupported(env.Error.Data)
			}
			return nil, out
		}
		if method == "initialize" {
			session = resp.Header.Get("Mcp-Session-Id")
			if !probeHeaderValue(session) {
				return nil, probeErr("invalid_response", "initialize")
			}
		}
		if notification {
			if resp.StatusCode != 202 && resp.StatusCode != 204 {
				return nil, probeErr("invalid_response", probeStage(method))
			}
			return nil, nil
		}
		if events != nil {
			return probeReadFrames(callCtx, events, current, method)
		}
		switch probeMedia(resp.Header.Get("Content-Type")) {
		case "application/json":
			raw, e := io.ReadAll(io.LimitReader(resp.Body, probeLimit+1))
			if e != nil {
				return nil, probeContextError(callCtx, probeStage(method), e)
			}
			if len(raw) > probeLimit {
				return nil, probeErr("response_too_large", probeStage(method))
			}
			return probeDecode(raw, current, method)
		case "text/event-stream":
			frames := make(chan probeFrame, 1)
			sseCtx, cancel := context.WithCancel(callCtx)
			defer cancel()
			go probeSSE(sseCtx, resp.Body, frames)
			return probeReadFrames(callCtx, frames, current, method)
		default:
			return nil, probeErr("invalid_response", probeStage(method))
		}
	}
	return w, nil
}

func probeRequest(version, method string, params map[string]any, id int, notification bool) []byte {
	if version == probeVersion {
		cloned := make(map[string]any, len(params)+1)
		for k, v := range params {
			cloned[k] = v
		}
		params = cloned
		params["_meta"] = map[string]any{"io.modelcontextprotocol/protocolVersion": version, "io.modelcontextprotocol/clientInfo": map[string]string{"name": "Quosparr", "version": "1"}, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	}
	envelope := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if !notification {
		envelope["id"] = id
	}
	raw, _ := json.Marshal(envelope)
	return append(raw, '\n')
}
func probeStage(method string) string {
	switch method {
	case "server/discover":
		return "discover"
	case "initialize":
		return "initialize"
	case "notifications/initialized":
		return "initialized"
	default:
		return "tools"
	}
}
func probeHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}
}
func probeMedia(s string) string { v, _, _ := mime.ParseMediaType(s); return v }
func probeHeaderValue(s string) bool {
	for _, r := range s {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}
func probeHeaderAllowed(k, v string) bool {
	canonical := strings.ToLower(k)
	return headerName.MatchString(k) && probeHeaderValue(v) && !reservedHeaders[canonical] && canonical != "mcp-method" && canonical != "mcp-name" && !strings.HasPrefix(canonical, "mcp-param-")
}

func probeContextError(ctx context.Context, stage string, err error) error {
	code := "network"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = "timeout"
	} else if ctx.Err() != nil {
		code = "cancelled"
	}
	return probeErr(code, stage)
}
func probeDecode(raw []byte, id int, method string) (json.RawMessage, error) {
	var env probeEnvelope
	if !probeJSONSafe(raw) || json.Unmarshal(raw, &env) != nil || env.JSONRPC != "2.0" || string(env.ID) != strconv.Itoa(id) || env.Method != "" || (env.Result == nil) == (env.Error == nil) {
		return nil, probeErr("invalid_response", probeStage(method))
	}
	if env.Error != nil {
		return nil, &ProbeError{Code: "rpc", Stage: probeStage(method), RPCCode: env.Error.Code, legacySupported: probeLegacySupported(env.Error.Data)}
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(env.Result, &object) != nil || object == nil {
		return nil, probeErr("invalid_response", probeStage(method))
	}
	return env.Result, nil
}

func probeLegacySupported(raw json.RawMessage) bool {
	var data struct {
		Supported []string `json:"supported"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return false
	}
	for _, v := range data.Supported {
		switch v {
		case "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05":
			return true
		}
	}
	return false
}
