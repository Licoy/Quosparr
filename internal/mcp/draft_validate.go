package mcp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var draftName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
var reservedHeaders = map[string]bool{"host": true, "content-length": true, "transfer-encoding": true, "connection": true, "upgrade": true, "trailer": true, "te": true, "proxy-authorization": true, "proxy-authenticate": true, "mcp-session-id": true, "mcp-protocol-version": true, "last-event-id": true, "accept": true, "content-type": true}

func ValidateDraft(d Draft) error {
	if !draftName.MatchString(d.Name) || len(d.Name) > 128 {
		return draftError("invalid", "name")
	}
	raw, _ := json.Marshal(d)
	if len(raw) > 1<<20 {
		return draftError("size", "draft")
	}
	switch d.Transport {
	case "stdio":
		if strings.TrimSpace(d.Command) == "" || hasControl(d.Command) {
			return draftError("invalid", "command")
		}
		if d.URL != "" || len(d.Headers) > 0 || len(d.EnvHeaders) > 0 || d.BearerTokenEnv != "" {
			return draftError("conflict", "transport")
		}
	case "http", "sse":
		u, err := url.Parse(d.URL)
		if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || hasControl(d.URL) || strings.Contains(d.URL, "${") {
			return draftError("invalid", "url")
		}
		if d.Command != "" || len(d.Args) > 0 || len(d.Env) > 0 || d.Cwd != "" {
			return draftError("conflict", "transport")
		}
	default:
		return draftError("unsupported", "transport")
	}
	if hasControl(d.Cwd) {
		return draftError("invalid", "cwd")
	}
	for _, arg := range d.Args {
		if strings.ContainsRune(arg, 0) {
			return draftError("invalid", "args")
		}
	}
	for key, value := range d.Env {
		if !envName.MatchString(key) || strings.ContainsRune(value, 0) {
			return draftError("invalid", "env")
		}
	}
	seen := map[string]bool{}
	for _, headers := range []map[string]string{d.Headers, d.EnvHeaders} {
		for key, value := range headers {
			canonical := strings.ToLower(key)
			if !headerName.MatchString(key) || reservedHeaders[canonical] || canonical == "mcp-method" || canonical == "mcp-name" || strings.HasPrefix(canonical, "mcp-param-") || seen[canonical] || hasControl(value) {
				return draftError("invalid", "headers")
			}
			seen[canonical] = true
		}
	}
	for _, value := range d.EnvHeaders {
		if !envName.MatchString(value) {
			return draftError("invalid", "env_headers")
		}
	}
	if d.BearerTokenEnv != "" {
		if !envName.MatchString(d.BearerTokenEnv) || seen[strings.ToLower(http.CanonicalHeaderKey("Authorization"))] {
			return draftError("conflict", "bearer_token_env")
		}
	}
	return nil
}

func hasControl(value string) bool { return strings.IndexFunc(value, unicode.IsControl) >= 0 }

func draftConfig(provider Provider, d Draft) (map[string]any, error) {
	if err := ValidateDraft(d); err != nil {
		return nil, err
	}
	if provider == ProviderGrok && (!regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`).MatchString(d.Name) || strings.HasSuffix(d.Name, "_") || strings.Contains(d.Name, "__")) {
		return nil, draftError("invalid", "name")
	}
	if provider == ProviderClaude && d.Cwd != "" {
		return nil, draftError("unsupported", "cwd")
	}
	if provider == ProviderGrok && d.Transport == "http" && strings.HasSuffix(d.URL, "/sse") {
		return nil, draftError("conflict", "transport")
	}
	if provider == ProviderCodex && d.Transport == "sse" {
		return nil, draftError("unsupported", "transport")
	}
	if provider == ProviderCodex && d.Transport == "stdio" {
		values := append([]string{d.Command, d.Cwd}, d.Args...)
		for _, value := range d.Env {
			values = append(values, value)
		}
		for _, value := range values {
			if strings.Contains(value, "${") {
				return nil, draftError("unsupported", "interpolation")
			}
		}
	}
	config := map[string]any{}
	if provider == ProviderClaude || d.Transport != "stdio" && provider == ProviderGrok {
		config["type"] = d.Transport
	}
	if d.Transport == "stdio" {
		config["command"] = d.Command
		if len(d.Args) > 0 {
			config["args"] = append([]string(nil), d.Args...)
		}
		if len(d.Env) > 0 {
			config["env"] = copyStrings(d.Env)
		}
		if d.Cwd != "" {
			config["cwd"] = d.Cwd
		}
		return config, nil
	}
	config["url"] = d.URL
	headers := copyStrings(d.Headers)
	envHeaders := copyStrings(d.EnvHeaders)
	bearer := d.BearerTokenEnv
	if provider == ProviderCodex {
		for key, value := range headers {
			if !strings.Contains(value, "${") {
				continue
			}
			name, ok := simpleVariable(value)
			if ok {
				envHeaders[key] = name
				delete(headers, key)
				continue
			}
			if strings.EqualFold(key, "Authorization") && strings.HasPrefix(value, "Bearer ") {
				if name, ok := simpleVariable(strings.TrimPrefix(value, "Bearer ")); ok && bearer == "" {
					bearer = name
					delete(headers, key)
					continue
				}
			}
			return nil, draftError("unsupported", "headers")
		}
		if len(headers) > 0 {
			config["http_headers"] = headers
		}
		if len(envHeaders) > 0 {
			config["env_http_headers"] = envHeaders
		}
		if bearer != "" {
			config["bearer_token_env_var"] = bearer
		}
	} else {
		for key, value := range envHeaders {
			headers[key] = "${" + value + "}"
		}
		if bearer != "" {
			headers["Authorization"] = "Bearer ${" + bearer + "}"
		}
		if len(headers) > 0 {
			config["headers"] = headers
		}
	}
	return config, nil
}
func simpleVariable(value string) (string, bool) {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return "", false
	}
	name := value[2 : len(value)-1]
	return name, envName.MatchString(name)
}
func copyStrings(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
