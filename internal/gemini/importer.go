package gemini

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/codex"
)

// Failure strings the UI maps to localized text (shared with Codex).
const (
	FailureNoCredentials   = codex.FailureNoCredentials
	FailureNothingSelected = codex.FailureNothingSelected
)

// ParsePayloads turns pasted text into credential documents that
// ParseCredential accepts.
//
// Accepted shapes:
//   - the official document ({"token":{…},"auth_method",…});
//   - a bare token object (access_token / refresh_token, camelCase too, with
//     expiry as RFC 3339 or expiry_date as Unix milliseconds, id_token kept);
//   - a JSON array of either form;
//   - newline-delimited JSON objects.
//
// The whole text is tried first, then each line (when more than one line
// holds a credential), then the first balanced {…} and […] slices. Payloads
// with the same refresh token are kept once.
func ParsePayloads(text string) [][]byte {
	trimmed := trimSpace(text)
	if trimmed == "" {
		return nil
	}
	if parsed := payloadsFromJSON(trimmed); len(parsed) > 0 {
		return uniquePayloads(parsed)
	}
	var lines [][]byte
	for _, line := range strings.Split(trimmed, "\n") {
		lines = append(lines, payloadsFromJSON(trimSpace(line))...)
	}
	if len(lines) > 1 {
		return uniquePayloads(lines)
	}
	for _, candidate := range jsonPayloadCandidates(trimmed)[1:] {
		if parsed := payloadsFromJSON(candidate); len(parsed) > 0 {
			return uniquePayloads(parsed)
		}
	}
	return uniquePayloads(lines)
}

func payloadsFromJSON(text string) [][]byte {
	var value any
	if json.Unmarshal([]byte(text), &value) != nil {
		return nil
	}
	return payloadsFrom(value)
}

func payloadsFrom(value any) [][]byte {
	switch v := value.(type) {
	case []any:
		var out [][]byte
		for _, item := range v {
			out = append(out, payloadsFrom(item)...)
		}
		return out
	case map[string]any:
		if _, ok := v["token"].(map[string]any); ok {
			data, err := json.Marshal(v)
			if err == nil {
				if _, err := ParseCredential(data); err == nil {
					return [][]byte{data}
				}
			}
			return nil
		}
		if data := wrapTokenObject(v); data != nil {
			return [][]byte{data}
		}
	}
	return nil
}

// wrapTokenObject builds the official document from a bare token object.
func wrapTokenObject(o map[string]any) []byte {
	access := firstNonEmpty(anyString(o["access_token"]), anyString(o["accessToken"]))
	refresh := firstNonEmpty(anyString(o["refresh_token"]), anyString(o["refreshToken"]))
	if access == "" && refresh == "" {
		return nil
	}
	var expiry time.Time
	if s := firstNonEmpty(anyString(o["expiry"]), anyString(o["expiresAt"])); s != "" {
		expiry, _ = time.Parse(time.RFC3339Nano, s)
	} else if ms, ok := o["expiry_date"].(float64); ok && ms > 0 {
		expiry = time.UnixMilli(int64(ms))
	}
	cred := NewCredential(access, refresh, firstNonEmpty(anyString(o["id_token"]), anyString(o["idToken"])), expiry)
	if method := anyString(o["auth_method"]); method != "" {
		cred.AuthMethod = method
	}
	data, err := cred.Marshal()
	if err != nil {
		return nil
	}
	if _, err := ParseCredential(data); err != nil {
		return nil
	}
	return data
}

func anyString(v any) string {
	s, _ := v.(string)
	return trimSpace(s)
}

func uniquePayloads(payloads [][]byte) [][]byte {
	seen := map[string]bool{}
	var out [][]byte
	for _, data := range payloads {
		cred, err := ParseCredential(data)
		if err != nil {
			continue
		}
		key := firstNonEmpty(cred.RefreshToken, cred.AccessToken)
		if !seen[key] {
			seen[key] = true
			out = append(out, data)
		}
	}
	return out
}

// jsonPayloadCandidates is the whole text plus its first balanced {…} and
// […] slices (the one that opens first is tried first), for pastes with
// prose around the JSON.
func jsonPayloadCandidates(text string) []string {
	pairs := [][2]byte{{'{', '}'}, {'[', ']'}}
	if a, o := strings.IndexByte(text, '['), strings.IndexByte(text, '{'); a >= 0 && (o < 0 || a < o) {
		pairs[0], pairs[1] = pairs[1], pairs[0]
	}
	candidates := []string{text}
	for _, pair := range pairs {
		if slice, ok := firstBalancedSlice(text, pair[0], pair[1]); ok && slice != text {
			candidates = append(candidates, slice)
		}
	}
	return candidates
}

func firstBalancedSlice(text string, open, closing byte) (string, bool) {
	start := strings.IndexByte(text, open)
	if start < 0 {
		return "", false
	}
	depth, inString, escaped := 0, false, false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return text[start : i+1], true
			}
		}
	}
	return "", false
}
