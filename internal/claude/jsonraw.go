package claude

import (
	"bytes"
	"encoding/json"
	"errors"
)

func rawObject(data []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, ErrAuthInvalidRoot
	}
	return root, nil
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func marshalRawObject(m map[string]json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

func rawString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}

func oauthObject(data []byte) (json.RawMessage, error) {
	root, err := rawObject(data)
	if err != nil {
		return nil, err
	}
	raw, ok := root[oauthKey]
	if !ok || !isJSONObject(raw) {
		return nil, ErrAuthNoOAuth
	}
	return bytes.TrimSpace(raw), nil
}

// officialHasOAuth distinguishes a signed-out container from malformed data.
// Keep the original container available to callers for rechecks and switching.
func officialHasOAuth(data []byte) (bool, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return false, classifiedError(StageReadOfficial, KindInvalidData, ErrAuthInvalidRoot)
	}
	root, err := rawObject(data)
	if err != nil {
		return false, classifiedError(StageReadOfficial, KindInvalidData, err)
	}
	raw, present := root[oauthKey]
	if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false, nil
	}
	if !isJSONObject(raw) {
		return false, classifiedError(StageReadOfficial, KindInvalidData, ErrAuthNoOAuth)
	}
	return true, nil
}

func decodeOAuthMap(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, ErrAuthNoOAuth
	}
	return fields, nil
}

func stringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func rawInt64(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return 0, false
	}
	v, err := n.Int64()
	if err != nil {
		return 0, false
	}
	return v, true
}

// rejectTokenKeys reports an error when data contains credential key names.
func rejectTokenKeys(data []byte) error {
	if bytes.Contains(data, []byte(`"accessToken"`)) || bytes.Contains(data, []byte(`"refreshToken"`)) || bytes.Contains(data, []byte(`"apiKey"`)) {
		return errors.New("claude: account index must not contain tokens")
	}
	return nil
}
