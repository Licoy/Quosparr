package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/netx"
)

func probeTestContext() context.Context {
	return netx.WithContext(context.Background(), netx.SystemContext(netx.WithSystemProxy(func(*url.URL) (*url.URL, error) { return nil, nil })))
}
func probeReply(w http.ResponseWriter, id any, result any, sse bool) {
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if sse {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", raw)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	}
}
func probeDiscovery(tools bool) map[string]any {
	caps := map[string]any{}
	if tools {
		caps["tools"] = map[string]any{}
	}
	return map[string]any{"supportedVersions": []string{probeVersion}, "capabilities": caps, "_meta": map[string]any{"io.modelcontextprotocol/serverInfo": map[string]string{"name": "test-server", "version": "1"}}}
}
func probeTestTool(secret string) map[string]any {
	return map[string]any{"name": "test", "title": "Title", "description": secret, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]string{"type": "string", "description": secret}}, "required": []string{"value"}}}
}
func assertProbeCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *ProbeError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error = %#v; want %s", err, code)
	}
}

func TestProbeHTTPModernAndLegacy(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, sse := range []bool{false, true} {
			t.Run(fmt.Sprint(legacy, sse), func(t *testing.T) {
				t.Setenv("MCP_PROBE_TEST_SECRET", "very-private-token")
				var calls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						if r.Header.Get("Mcp-Session-Id") != "fake-session" {
							t.Error("cleanup missing session")
						}
						w.WriteHeader(204)
						return
					}
					var req struct {
						ID     any
						Method string
						Params map[string]any
					}
					if json.NewDecoder(r.Body).Decode(&req) != nil {
						t.Error("invalid request")
						return
					}
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer very-private-token" {
						t.Error("missing resolved header")
					}
					if !legacy {
						meta, _ := req.Params["_meta"].(map[string]any)
						if meta["io.modelcontextprotocol/protocolVersion"] != probeVersion || r.Header.Get("Mcp-Method") != req.Method {
							t.Error("missing metadata")
						}
					}
					switch req.Method {
					case "server/discover":
						if legacy {
							w.WriteHeader(400)
							return
						}
						probeReply(w, req.ID, probeDiscovery(true), sse)
					case "initialize":
						w.Header().Set("Mcp-Session-Id", "fake-session")
						probeReply(w, req.ID, map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "test-server", "version": "1"}}, sse)
					case "notifications/initialized":
						w.WriteHeader(202)
					case "tools/list":
						if legacy && r.Header.Get("Mcp-Session-Id") != "fake-session" {
							t.Error("missing session")
						}
						list := map[string]any{"tools": []any{probeTestTool("very-private-token")}}
						if req.Params["cursor"] == nil {
							list["nextCursor"] = "page2"
						}
						probeReply(w, req.ID, list, sse)
					default:
						t.Errorf("unexpected method %q", req.Method)
					}
				}))
				defer srv.Close()
				result, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer ${MCP_PROBE_TEST_SECRET}"}})
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Tools) != 2 || result.ServerName != "test-server" || len(result.Tools[0].Parameters) != 1 || !result.Tools[0].Parameters[0].Required {
					t.Fatalf("result=%+v", result)
				}
				if strings.Contains(result.Tools[0].Description, "private") || strings.Contains(result.Tools[0].InputSchema, "private") {
					t.Fatal("secret leaked")
				}
				if !legacy && result.ProtocolVersion != probeVersion {
					t.Fatal("wrong version")
				}
				if legacy && result.ProtocolVersion != "2025-11-25" {
					t.Fatal("wrong legacy version")
				}
			})
		}
	}
}
func TestProbeHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name, code    string
		status        int
		body, content string
	}{{"auth", "http_status", 401, `private-token`, "application/json"}, {"modern-header", "http_status", 400, `{"jsonrpc":"2.0","id":1,"error":{"code":-32020,"message":"private-token"}}`, "application/json"}, {"modern-version", "http_status", 400, `{"jsonrpc":"2.0","id":1,"error":{"code":-32022,"data":{"supported":["1900-01-01"]}}}`, "application/json"}, {"html", "invalid_response", 200, "<html>private-token</html>", "text/html"}, {"badjson", "invalid_response", 200, `{"hello":"private-token"}`, "application/json"}, {"large", "response_too_large", 200, strings.Repeat("x", probeLimit+1), "application/json"}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", tc.content)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			_, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL})
			assertProbeCode(t, err, tc.code)
			if calls != 1 {
				t.Fatalf("retried %d", calls)
			}
			if strings.Contains(err.Error(), "private-token") {
				t.Fatal("error leaked body")
			}
		})
	}
}
func TestProbeHTTPEmptyAndRepeatedCursor(t *testing.T) {
	for _, tools := range []bool{false, true} {
		t.Run(fmt.Sprint(tools), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     any
					Method string
				}
				json.NewDecoder(r.Body).Decode(&req)
				if req.Method == "server/discover" {
					probeReply(w, req.ID, probeDiscovery(tools), false)
				} else {
					probeReply(w, req.ID, map[string]any{"tools": []any{}, "nextCursor": "same"}, false)
				}
			}))
			defer srv.Close()
			result, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL})
			if tools {
				assertProbeCode(t, err, "invalid_response")
			} else if err != nil || len(result.Tools) != 0 {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}
func TestProbeRedirectAndMissingEnv(t *testing.T) {
	var contacted atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted.Store(true) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer srv.Close()
	_, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer private-token"}})
	assertProbeCode(t, err, "http_status")
	if contacted.Load() {
		t.Fatal("followed redirect")
	}
	t.Setenv("MCP_PROBE_MISSING", "")
	_, err = Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL, BearerTokenEnv: "MCP_PROBE_MISSING"})
	assertProbeCode(t, err, "missing_env")
}
func TestProbeHTTPCancelAndTimeout(t *testing.T) {
	for _, cancelNow := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelNow), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); <-r.Context().Done() }))
			defer srv.Close()
			ctx, cancel := context.WithTimeout(probeTestContext(), 50*time.Millisecond)
			defer cancel()
			if cancelNow {
				cancel()
			}
			_, err := Probe(ctx, Draft{Name: "fake", Transport: "http", URL: srv.URL})
			if cancelNow {
				assertProbeCode(t, err, "cancelled")
			} else {
				assertProbeCode(t, err, "timeout")
			}
		})
	}
}
func TestProbeLegacySSE(t *testing.T) {
	frames := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: endpoint\ndata: /messages\n\n")
			w.(http.Flusher).Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case frame := <-frames:
					fmt.Fprint(w, frame)
					w.(http.Flusher).Flush()
				}
			}
		}
		var req struct {
			ID     any
			Method string
		}
		json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(202)
		if req.ID == nil {
			return
		}
		result := any(map[string]any{"tools": []any{probeTestTool("ok")}})
		if req.Method == "initialize" {
			result = map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "sse"}}
		}
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		frames <- fmt.Sprintf("event: message\ndata: %s\n\n", raw)
	}))
	defer srv.Close()
	result, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "sse", URL: srv.URL})
	if err != nil || result.ProtocolVersion != "2024-11-05" || len(result.Tools) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
}
func TestProbeRequestDoesNotMutateParams(t *testing.T) {
	params := map[string]any{"cursor": "x"}
	probeRequest(probeVersion, "tools/list", params, 1, false)
	if len(params) != 1 {
		t.Fatal("mutated params")
	}
}

func TestProbePaginationLimits(t *testing.T) {
	for _, many := range []bool{false, true} {
		t.Run(fmt.Sprint(many), func(t *testing.T) {
			var pages atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					ID     any
					Method string
				}
				json.NewDecoder(r.Body).Decode(&req)
				if req.Method == "server/discover" {
					probeReply(w, req.ID, probeDiscovery(true), false)
					return
				}
				page := pages.Add(1)
				list := []any{}
				if many {
					for i := 0; i < probeToolLimit+1; i++ {
						list = append(list, probeTestTool("safe"))
					}
				}
				probeReply(w, req.ID, map[string]any{"tools": list, "nextCursor": fmt.Sprint(page)}, false)
			}))
			defer srv.Close()
			result, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL})
			if err != nil || !result.Truncated {
				t.Fatalf("%+v %v", result, err)
			}
			if many && len(result.Tools) != probeToolLimit {
				t.Fatal("tool limit not enforced")
			}
			if !many && pages.Load() != 20 {
				t.Fatal("page limit not enforced")
			}
		})
	}
}
func TestProbeLegacySSERejectsCrossOriginEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: endpoint\ndata: http://127.0.0.1:1/should-not-be-contacted\n\n")
	}))
	defer srv.Close()
	_, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "sse", URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer private-token"}})
	assertProbeCode(t, err, "invalid_response")
}
func TestProbeSensitiveTextAndTemplates(t *testing.T) {
	t.Setenv("MCP_PROBE_TEMPLATE", "private-value")
	original := Draft{Name: "fake", Transport: "stdio", Command: "executable", Args: []string{"${MCP_PROBE_TEMPLATE}", "$LITERAL"}, Env: map[string]string{"TOKEN": "${MCP_PROBE_TEMPLATE}"}}
	expanded, _, err := probeExpandDraft(original)
	if err != nil || expanded.Args[0] != "private-value" || expanded.Args[1] != "$LITERAL" || expanded.Env["TOKEN"] != "private-value" {
		t.Fatalf("expand failed: %v", err)
	}
	if original.Args[0] != "${MCP_PROBE_TEMPLATE}" || original.Env["TOKEN"] != "${MCP_PROBE_TEMPLATE}" {
		t.Fatal("mutated draft")
	}
	secret := `quote"secret`
	raw, _ := json.Marshal(map[string]string{"value": secret})
	clean := probeText(string(raw)+"\x00", []string{secret})
	if strings.Contains(clean, "secret") || strings.ContainsRune(clean, 0) {
		t.Fatal("escaped secret or control leaked")
	}
	params := map[string]any{}
	raw = probeRequest(probeVersion, "server/discover", params, 1, false)
	if _, err := probeDecode(raw, 1, "server/discover"); err == nil {
		t.Fatal("server request treated as response")
	}
}

func TestProbeHTTPSessionCleanup(t *testing.T) {
	for _, status := range []int{204, 404, 405, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var cleaned atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					cleaned.Store(true)
					w.WriteHeader(status)
					return
				}
				var req struct {
					ID     any
					Method string
				}
				json.NewDecoder(r.Body).Decode(&req)
				switch req.Method {
				case "server/discover":
					w.WriteHeader(400)
				case "initialize":
					w.Header().Set("Mcp-Session-Id", "session")
					probeReply(w, req.ID, map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}}, false)
				case "notifications/initialized":
					w.WriteHeader(202)
				}
			}))
			defer srv.Close()
			result, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL})
			if err != nil || !cleaned.Load() || result.CleanupFailed != (status == 500) {
				t.Fatalf("cleanup result %+v %v", result, err)
			}
		})
	}
}
func TestProbeHTTPNeverChangesTransportOrBypassesProxy(t *testing.T) {
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets.Add(1)
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	_, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL})
	assertProbeCode(t, err, "http_status")
	if gets.Load() != 0 {
		t.Fatal("silently changed to SSE")
	}
	ctx := netx.WithContext(context.Background(), netx.SystemContext(netx.WithSystemProxy(func(*url.URL) (*url.URL, error) { return nil, errors.New("private-proxy-error") })))
	_, err = Probe(ctx, Draft{Name: "fake", Transport: "http", URL: srv.URL})
	assertProbeCode(t, err, "network")
}

func TestProbeRejectsDeepOrOversizedSchemas(t *testing.T) {
	deep := `{"name":"deep","inputSchema":{"type":"object","x":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}}`
	if _, err := probeTool(json.RawMessage(deep)); err == nil {
		t.Fatal("deep schema accepted")
	}
	huge := `{"name":"huge","inputSchema":{"type":"object","description":"` + strings.Repeat("x", 33000) + `"}}`
	_, err := probeTool(json.RawMessage(huge))
	assertProbeCode(t, err, "response_too_large")
}

func TestProbeRedactionDoesNotExpandRepeatedSecrets(t *testing.T) {
	secrets := []string{"token", "t", "r", "e", "d", "a", "c", "[", "]"}
	for i := 0; i < 1000; i++ {
		secrets = append(secrets, "t", "token")
	}
	cleaner := probeCleaner(secrets)
	if got := cleaner("token"); got != "[redacted]" {
		t.Fatalf("replacement was reprocessed: %q", got)
	}
	if got := cleaner(strings.Repeat("t", 100000)); len(got) > 32768 {
		t.Fatal("redaction output exceeded bound")
	}
}

func TestProbeEnvAuthorizationTokenIsRedacted(t *testing.T) {
	t.Setenv("MCP_PROBE_AUTH_HEADER", "Bearer  lone-secret")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     any
			Method string
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "server/discover" {
			probeReply(w, req.ID, probeDiscovery(true), false)
		} else {
			probeReply(w, req.ID, map[string]any{"tools": []any{probeTestTool("lone-secret")}}, false)
		}
	}))
	defer srv.Close()
	result, err := Probe(probeTestContext(), Draft{Name: "fake", Transport: "http", URL: srv.URL, EnvHeaders: map[string]string{"authorization": "MCP_PROBE_AUTH_HEADER"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Tools[0].Description != "[redacted]" {
		t.Fatal("resolved Authorization token leaked")
	}
}
