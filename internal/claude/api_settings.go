package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	settingsFileName   = "settings.json"
	settingsBackupKeep = 10

	envBaseURL   = "ANTHROPIC_BASE_URL"
	envAuthToken = "ANTHROPIC_AUTH_TOKEN"
	envAPIKey    = "ANTHROPIC_API_KEY"
)

var (
	// ErrSettingsInvalid means settings.json is not a JSON object (or its env
	// is not). The file content is never echoed: it may hold credentials.
	ErrSettingsInvalid = errors.New("claude: settings.json is not a JSON object with an object env")
	// ErrAPISettingsIncomplete is returned when the base URL or token is empty.
	ErrAPISettingsIncomplete = errors.New("claude: API base URL and auth token are required")
)

// APIEnv is an API relay account as written into settings.json. Model and the
// four mapping fields are optional; empty means "leave to Claude Code".
type APIEnv struct {
	BaseURL, AuthToken, Model                      string
	OpusModel, SonnetModel, HaikuModel, FableModel string
}

type envField struct {
	key string
	get func(APIEnv) string
}

// mappingFields are the optional env entries of an APIEnv, in output order.
var mappingFields = []envField{
	{"ANTHROPIC_DEFAULT_OPUS_MODEL", func(e APIEnv) string { return e.OpusModel }},
	{"ANTHROPIC_DEFAULT_SONNET_MODEL", func(e APIEnv) string { return e.SonnetModel }},
	{"ANTHROPIC_DEFAULT_HAIKU_MODEL", func(e APIEnv) string { return e.HaikuModel }},
	{"ANTHROPIC_DEFAULT_FABLE_MODEL", func(e APIEnv) string { return e.FableModel }},
}

// ApplyAPISettings sets the relay entries and removes ANTHROPIC_API_KEY, which
// would conflict with ANTHROPIC_AUTH_TOKEN. previous is the env applied last
// time (nil when none): a mapping or model it set that the new env leaves empty
// is removed, but only while the live value still equals previous, so values
// the user changed by hand survive. Every other key keeps its raw value; keys
// are written in sorted order, pretty-printed with a trailing newline.
func ApplyAPISettings(data []byte, env APIEnv, previous *APIEnv) ([]byte, error) {
	env = trimmedAPIEnv(env)
	if env.BaseURL == "" || env.AuthToken == "" {
		return nil, ErrAPISettingsIncomplete
	}
	root, vars, err := parseSettings(data)
	if err != nil {
		return nil, err
	}
	vars[envBaseURL] = rawJSONString(env.BaseURL)
	vars[envAuthToken] = rawJSONString(env.AuthToken)
	delete(vars, envAPIKey)
	for _, f := range mappingFields {
		prev := ""
		if previous != nil {
			prev = f.get(*previous)
		}
		setOrClear(vars, f.key, f.get(env), prev)
	}
	setOrClear(root, "model", env.Model, previousModel(previous))
	if env.Model != "" {
		delete(vars, "ANTHROPIC_MODEL")
	}
	root["env"] = marshalCompact(vars)
	return encodeSettings(root)
}

// RemoveAPISettings removes every key whose live value equals what applied
// set, and drops env when that leaves it empty. The key also goes when it sits
// in ANTHROPIC_API_KEY (an imported setup), and the base URL when it names the
// same site however it is spelled. When nothing matched it returns
// data unchanged and changed=false.
func RemoveAPISettings(data []byte, applied APIEnv) ([]byte, bool, error) {
	applied = trimmedAPIEnv(applied)
	root, vars, err := parseSettings(data)
	if err != nil {
		return nil, false, err
	}
	envChanged := deleteBaseURL(vars, applied.BaseURL)
	envChanged = deleteIfEquals(vars, envAuthToken, applied.AuthToken) || envChanged
	envChanged = deleteIfEquals(vars, envAPIKey, applied.AuthToken) || envChanged
	for _, f := range mappingFields {
		envChanged = deleteIfEquals(vars, f.key, f.get(applied)) || envChanged
	}
	modelChanged := deleteIfEquals(root, "model", applied.Model)
	if !envChanged && !modelChanged {
		return data, false, nil
	}
	if envChanged {
		if len(vars) == 0 {
			delete(root, "env")
		} else {
			root["env"] = marshalCompact(vars)
		}
	}
	out, err := encodeSettings(root)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// ReadAPISettings returns the live ANTHROPIC_BASE_URL and the key Claude Code
// sends there: ANTHROPIC_AUTH_TOKEN, else ANTHROPIC_API_KEY. Callers must not
// log them.
func ReadAPISettings(data []byte) (baseURL, authToken string, err error) {
	env, err := ReadAPISettingsEnv(data)
	return env.BaseURL, env.AuthToken, err
}

// ReadAPISettingsEnv returns the live relay entries of settings.json, the
// model and the mapping included, trimmed. Callers must not log the token.
func ReadAPISettingsEnv(data []byte) (APIEnv, error) {
	root, vars, err := parseSettings(data)
	if err != nil {
		return APIEnv{}, err
	}
	env := APIEnv{
		BaseURL: stringField(vars[envBaseURL]), AuthToken: stringField(vars[envAuthToken]),
		Model:       stringField(root["model"]),
		OpusModel:   stringField(vars["ANTHROPIC_DEFAULT_OPUS_MODEL"]),
		SonnetModel: stringField(vars["ANTHROPIC_DEFAULT_SONNET_MODEL"]),
		HaikuModel:  stringField(vars["ANTHROPIC_DEFAULT_HAIKU_MODEL"]),
		FableModel:  stringField(vars["ANTHROPIC_DEFAULT_FABLE_MODEL"]),
	}
	if strings.TrimSpace(env.AuthToken) == "" {
		env.AuthToken = stringField(vars[envAPIKey])
	}
	return trimmedAPIEnv(env), nil
}

func trimmedAPIEnv(e APIEnv) APIEnv {
	for _, p := range []*string{&e.BaseURL, &e.AuthToken, &e.Model, &e.OpusModel, &e.SonnetModel, &e.HaikuModel, &e.FableModel} {
		*p = strings.TrimSpace(*p)
	}
	return e
}

func rawJSONString(v string) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // a Go string always encodes
	return bytes.TrimSpace(buf.Bytes())
}

func previousModel(previous *APIEnv) string {
	if previous == nil {
		return ""
	}
	return strings.TrimSpace(previous.Model)
}

// setOrClear writes want when non-empty; otherwise it removes key if it still
// holds the value Quosparr applied before.
func setOrClear(obj map[string]json.RawMessage, key, want, previous string) {
	if want != "" {
		obj[key] = rawJSONString(want)
		return
	}
	deleteIfEquals(obj, key, strings.TrimSpace(previous))
}

// deleteBaseURL removes ANTHROPIC_BASE_URL when it is base, or the same site
// spelled differently (a trailing slash, a /v1 suffix).
func deleteBaseURL(vars map[string]json.RawMessage, base string) bool {
	if deleteIfEquals(vars, envBaseURL, strings.TrimSpace(base)) {
		return true
	}
	live, errLive := NormalizeAPIBaseURL(stringField(vars[envBaseURL]))
	want, errWant := NormalizeAPIBaseURL(base)
	if errLive != nil || errWant != nil || live != want {
		return false
	}
	delete(vars, envBaseURL)
	return true
}

func deleteIfEquals(obj map[string]json.RawMessage, key, value string) bool {
	raw, ok := obj[key]
	if !ok || value == "" || stringField(raw) != value {
		return false
	}
	delete(obj, key)
	return true
}

// parseSettings returns the root object and its env object (empty when env is
// absent). Both maps are fresh copies of the raw members.
func parseSettings(data []byte) (root, vars map[string]json.RawMessage, err error) {
	root, err = rawSettingsObject(data)
	if err != nil {
		return nil, nil, err
	}
	vars = map[string]json.RawMessage{}
	raw, ok := root["env"]
	if !ok {
		return root, vars, nil
	}
	if !isJSONObject(raw) {
		return nil, nil, ErrSettingsInvalid
	}
	if err := json.Unmarshal(raw, &vars); err != nil || vars == nil {
		return nil, nil, ErrSettingsInvalid
	}
	return root, vars, nil
}

func rawSettingsObject(data []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil, ErrSettingsInvalid
	}
	return root, nil
}

func marshalCompact(m map[string]json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	// Raw members came from valid JSON, so encoding cannot fail.
	_ = enc.Encode(m)
	return bytes.TrimSpace(buf.Bytes())
}

func encodeSettings(root map[string]json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		return nil, ErrSettingsInvalid
	}
	return buf.Bytes(), nil
}

// --- files ------------------------------------------------------------------

// SettingsPath is <config dir>/settings.json.
func (s *Store) SettingsPath() string { return filepath.Join(s.configDir, settingsFileName) }

func (s *Store) settingsBackup() platform.BackupPolicy {
	return platform.BackupPolicy{
		Dir:    filepath.Join(s.runwayHomeDir(), "backups"),
		Prefix: "claude-settings-",
		Ext:    ".json",
		Keep:   settingsBackupKeep,
	}
}

// ApplyAPISettingsFile applies env to settings.json, backing the current file
// up first. Nothing is written when the content would not change.
func (s *Store) ApplyAPISettingsFile(env APIEnv, previous *APIEnv, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.EditFileWithBackup(s.SettingsPath(), s.settingsBackup(), now, func(old []byte) ([]byte, error) {
		return ApplyAPISettings(old, env, previous)
	})
}

// RemoveAPISettingsFile removes what ApplyAPISettingsFile wrote.
func (s *Store) RemoveAPISettingsFile(applied APIEnv, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.EditFileWithBackup(s.SettingsPath(), s.settingsBackup(), now, func(old []byte) ([]byte, error) {
		out, _, err := RemoveAPISettings(old, applied)
		return out, err
	})
}

// ReadAPISettingsEnvFile is ReadAPISettingsEnv on the file; a missing file has
// no entries.
func (s *Store) ReadAPISettingsEnvFile() (APIEnv, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := readFileIfExists(s.SettingsPath())
	if err != nil {
		return APIEnv{}, err
	}
	return ReadAPISettingsEnv(data)
}

// ReadAPISettingsFile reads the live relay entries; a missing file has none.
func (s *Store) ReadAPISettingsFile() (baseURL, authToken string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := readFileIfExists(s.SettingsPath())
	if err != nil {
		return "", "", err
	}
	return ReadAPISettings(data)
}

func readFileIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
