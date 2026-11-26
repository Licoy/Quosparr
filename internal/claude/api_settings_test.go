package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	testRelayURL   = "https://relay.example/api?a=1&b=2"
	testRelayToken = "sk-relay-SECRET-TOKEN-123"
)

var settingsNow = time.Unix(1_800_000_000, 0).UTC()

func relayEnv() APIEnv {
	return APIEnv{BaseURL: testRelayURL, AuthToken: testRelayToken}
}

func decodeSettings(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, data)
	}
	return m
}

func envOf(t *testing.T, data []byte) map[string]any {
	t.Helper()
	env, _ := decodeSettings(t, data)["env"].(map[string]any)
	return env
}

func applyOK(t *testing.T, in string, env APIEnv, prev *APIEnv) string {
	t.Helper()
	out, err := ApplyAPISettings([]byte(in), env, prev)
	if err != nil {
		t.Fatalf("ApplyAPISettings: %v", err)
	}
	return string(out)
}

func TestApplyAPISettingsToEmptyInput(t *testing.T) {
	want := "{\n  \"env\": {\n    \"ANTHROPIC_AUTH_TOKEN\": \"" + testRelayToken + "\",\n    \"ANTHROPIC_BASE_URL\": \"" + testRelayURL + "\"\n  }\n}\n"
	for _, in := range []string{"", "  \n"} {
		if got := applyOK(t, in, relayEnv(), nil); got != want {
			t.Fatalf("got:\n%s\nwant:\n%s", got, want)
		}
	}
}

func TestApplyAPISettingsRequiresURLAndToken(t *testing.T) {
	for _, env := range []APIEnv{{BaseURL: "https://r.example"}, {AuthToken: testRelayToken}, {}} {
		if _, err := ApplyAPISettings(nil, env, nil); !errors.Is(err, ErrAPISettingsIncomplete) {
			t.Fatalf("env %+v: err = %v", env, err)
		}
	}
}

func TestApplyAPISettingsPreservesOtherKeysAndRemovesAPIKey(t *testing.T) {
	in := `{
  "permissions": {"allow": ["Bash(ls:*)"], "deny": []},
  "bigNumber": 12345678901234567890,
  "html": "<a & b>",
  "env": {"KEEP": "1", "ANTHROPIC_API_KEY": "sk-ant-conflict", "N": 5},
  "hooks": {}
}`
	got := applyOK(t, in, relayEnv(), nil)
	if !strings.HasSuffix(got, "}\n") || strings.Contains(got, "\t") {
		t.Fatalf("not 2-space pretty with trailing newline:\n%s", got)
	}
	if !strings.Contains(got, "\n  \"bigNumber\": 12345678901234567890,\n") {
		t.Fatalf("big number not preserved raw:\n%s", got)
	}
	if !strings.Contains(got, `"html": "<a & b>"`) || !strings.Contains(got, `a=1&b=2`) {
		t.Fatalf("HTML escaped:\n%s", got)
	}
	m := decodeSettings(t, []byte(got))
	env := m["env"].(map[string]any)
	if _, has := env["ANTHROPIC_API_KEY"]; has {
		t.Fatalf("ANTHROPIC_API_KEY not removed:\n%s", got)
	}
	if env["KEEP"] != "1" || env["N"] != float64(5) || env["ANTHROPIC_BASE_URL"] != testRelayURL || env["ANTHROPIC_AUTH_TOKEN"] != testRelayToken {
		t.Fatalf("env wrong:\n%s", got)
	}
	perms := m["permissions"].(map[string]any)
	if len(perms["allow"].([]any)) != 1 || m["hooks"] == nil {
		t.Fatalf("other keys changed:\n%s", got)
	}
	if again := applyOK(t, got, relayEnv(), nil); again != got {
		t.Fatalf("second apply is not idempotent:\n%s", again)
	}
}

func TestApplyAPISettingsMappingsAndModel(t *testing.T) {
	env := relayEnv()
	env.Model, env.OpusModel, env.SonnetModel, env.HaikuModel, env.FableModel = "m-main", "m-opus", "m-sonnet", "m-haiku", "m-fable"
	got := applyOK(t, `{"model":"user-model"}`, env, nil)
	m := decodeSettings(t, []byte(got))
	e := m["env"].(map[string]any)
	if m["model"] != "m-main" || e["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "m-opus" || e["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "m-sonnet" ||
		e["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "m-haiku" || e["ANTHROPIC_DEFAULT_FABLE_MODEL"] != "m-fable" {
		t.Fatalf("got:\n%s", got)
	}
	// An empty Model without a previous one leaves the user's model alone.
	got = applyOK(t, `{"model":"user-model"}`, relayEnv(), nil)
	if decodeSettings(t, []byte(got))["model"] != "user-model" {
		t.Fatalf("user model touched:\n%s", got)
	}
}

func TestApplyAPISettingsClearsPreviousMappingsOnlyWhenUnchanged(t *testing.T) {
	prev := relayEnv()
	prev.Model, prev.OpusModel, prev.SonnetModel, prev.HaikuModel, prev.FableModel = "p-main", "p-opus", "p-sonnet", "p-haiku", "p-fable"
	applied := applyOK(t, `{"theme":"dark"}`, prev, nil)

	// The user edited the sonnet mapping and the model by hand afterwards.
	edited := strings.Replace(applied, "p-sonnet", "hand-edited", 1)
	edited = strings.Replace(edited, "p-main", "hand-model", 1)
	next := APIEnv{BaseURL: "https://other.example", AuthToken: "sk-other", HaikuModel: "n-haiku"}
	got := applyOK(t, edited, next, &prev)
	m := decodeSettings(t, []byte(got))
	e := m["env"].(map[string]any)
	if _, has := e["ANTHROPIC_DEFAULT_OPUS_MODEL"]; has {
		t.Fatalf("stale opus mapping kept:\n%s", got)
	}
	if _, has := e["ANTHROPIC_DEFAULT_FABLE_MODEL"]; has {
		t.Fatalf("stale fable mapping kept:\n%s", got)
	}
	if e["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "hand-edited" {
		t.Fatalf("user-edited sonnet mapping removed:\n%s", got)
	}
	if e["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "n-haiku" || e["ANTHROPIC_BASE_URL"] != "https://other.example" {
		t.Fatalf("new env wrong:\n%s", got)
	}
	if m["model"] != "hand-model" || m["theme"] != "dark" {
		t.Fatalf("model handling wrong:\n%s", got)
	}

	// Untouched previous values are removed, including the model.
	got = applyOK(t, applied, next, &prev)
	m = decodeSettings(t, []byte(got))
	if _, has := m["model"]; has {
		t.Fatalf("stale model kept:\n%s", got)
	}
	if _, has := m["env"].(map[string]any)["ANTHROPIC_DEFAULT_SONNET_MODEL"]; has {
		t.Fatalf("stale sonnet mapping kept:\n%s", got)
	}
}

func TestApplyAPISettingsRejectsBadShapesWithoutEchoing(t *testing.T) {
	cases := []string{
		`[1,2]`,
		`null`,
		`"` + testRelayToken + `"`,
		`{"env": "` + testRelayToken + `"}`,
		`{"env": [1]}`,
		`{"env": null}`,
		`{"apiKeyHelper": "` + testRelayToken + `",`,
	}
	for _, in := range cases {
		_, errA := ApplyAPISettings([]byte(in), relayEnv(), nil)
		_, _, errR := RemoveAPISettings([]byte(in), relayEnv())
		_, _, errD := ReadAPISettings([]byte(in))
		for _, err := range []error{errA, errR, errD} {
			if !errors.Is(err, ErrSettingsInvalid) {
				t.Fatalf("input %q: err = %v, want ErrSettingsInvalid", in, err)
			}
			if strings.Contains(err.Error(), testRelayToken) {
				t.Fatalf("error echoes content: %v", err)
			}
		}
	}
}

func TestRemoveAPISettings(t *testing.T) {
	env := relayEnv()
	env.Model, env.OpusModel = "m-main", "m-opus"
	base := `{"theme":"dark","env":{"KEEP":"1"}}`
	applied := applyOK(t, base, env, nil)

	out, changed, err := RemoveAPISettings([]byte(applied), env)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	m := decodeSettings(t, out)
	if _, has := m["model"]; has || m["theme"] != "dark" || m["env"].(map[string]any)["KEEP"] != "1" || len(m["env"].(map[string]any)) != 1 {
		t.Fatalf("got:\n%s", out)
	}

	// env disappears when it ends up empty.
	onlyOurs := applyOK(t, `{"theme":"dark"}`, env, nil)
	out, changed, _ = RemoveAPISettings([]byte(onlyOurs), env)
	if !changed || strings.Contains(string(out), "\"env\"") || decodeSettings(t, out)["theme"] != "dark" {
		t.Fatalf("env not dropped (changed=%v):\n%s", changed, out)
	}
	out, changed, _ = RemoveAPISettings([]byte(`{"env":{"ANTHROPIC_BASE_URL":"`+testRelayURL+`"}}`), env)
	if !changed || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("got (changed=%v):\n%s", changed, out)
	}
}

func TestRemoveAPISettingsKeepsValuesTheUserChanged(t *testing.T) {
	env := relayEnv()
	env.Model, env.OpusModel = "m-main", "m-opus"
	applied := applyOK(t, `{}`, env, nil)
	edited := strings.Replace(applied, testRelayToken, "sk-user-own", 1)
	edited = strings.Replace(edited, "m-main", "other-model", 1)
	out, changed, err := RemoveAPISettings([]byte(edited), env)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	m := decodeSettings(t, out)
	e := m["env"].(map[string]any)
	if e["ANTHROPIC_AUTH_TOKEN"] != "sk-user-own" || m["model"] != "other-model" {
		t.Fatalf("user values removed:\n%s", out)
	}
	if _, has := e["ANTHROPIC_BASE_URL"]; has {
		t.Fatalf("matching base url kept:\n%s", out)
	}
	if _, has := e["ANTHROPIC_DEFAULT_OPUS_MODEL"]; has {
		t.Fatalf("matching mapping kept:\n%s", out)
	}
}

func TestRemoveAPISettingsNothingMatchedReturnsInputUnchanged(t *testing.T) {
	in := "{ \"env\" : { \"ANTHROPIC_BASE_URL\" : \"https://mine.example\" }, \"a\":1 }"
	out, changed, err := RemoveAPISettings([]byte(in), relayEnv())
	if err != nil || changed || string(out) != in {
		t.Fatalf("changed=%v err=%v out=%q", changed, err, out)
	}
	if out, changed, err := RemoveAPISettings(nil, relayEnv()); err != nil || changed || len(out) != 0 {
		t.Fatalf("empty input: %q %v %v", out, changed, err)
	}
	// An empty applied env never matches an empty live value.
	out, changed, _ = RemoveAPISettings([]byte(`{"env":{"ANTHROPIC_BASE_URL":""},"model":""}`), APIEnv{})
	if changed {
		t.Fatalf("empty applied matched: %s", out)
	}
}

func TestReadAPISettings(t *testing.T) {
	url, token, err := ReadAPISettings([]byte(applyOK(t, `{}`, relayEnv(), nil)))
	if err != nil || url != testRelayURL || token != testRelayToken {
		t.Fatalf("url=%q err=%v", url, err)
	}
	for _, in := range []string{"", "{}", `{"env":{}}`, `{"env":{"ANTHROPIC_BASE_URL":5}}`} {
		url, token, err = ReadAPISettings([]byte(in))
		if err != nil || url != "" || token != "" {
			t.Fatalf("input %q: url=%q err=%v", in, url, err)
		}
	}
}

// --- file wrappers ----------------------------------------------------------

func newSettingsStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	configDir, runway := filepath.Join(root, ".claude"), filepath.Join(root, ".quosparr")
	return NewStore(configDir, runway, root, "", "test-account"), configDir, runway
}

func settingsBackups(t *testing.T, runway string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(runway, "backups", "claude-settings-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStoreSettingsPathAndRunwayHome(t *testing.T) {
	s, configDir, runway := newSettingsStore(t)
	if s.SettingsPath() != filepath.Join(configDir, "settings.json") || s.runwayHomeDir() != runway {
		t.Fatalf("paths: %s %s", s.SettingsPath(), s.runwayHomeDir())
	}
	at := NewStoreAt(filepath.Join(runway, "accounts"), configDir, filepath.Dir(configDir))
	if at.runwayHomeDir() != runway {
		t.Fatalf("NewStoreAt runway home = %s, want %s", at.runwayHomeDir(), runway)
	}
}

func TestStoreSettingsFileLifecycle(t *testing.T) {
	s, configDir, runway := newSettingsStore(t)
	if url, token, err := s.ReadAPISettingsFile(); err != nil || url != "" || token != "" {
		t.Fatalf("missing file: %q %v", url, err)
	}
	if err := s.RemoveAPISettingsFile(relayEnv(), settingsNow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.SettingsPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove created the file: %v", err)
	}
	if err := s.ApplyAPISettingsFile(relayEnv(), nil, settingsNow); err != nil {
		t.Fatal(err)
	}
	if len(settingsBackups(t, runway)) != 0 {
		t.Fatal("a new file needs no backup")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(s.SettingsPath())
		di, _ := os.Stat(configDir)
		if info.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
			t.Fatalf("modes: file %v dir %v", info.Mode().Perm(), di.Mode().Perm())
		}
	}
	url, token, err := s.ReadAPISettingsFile()
	if err != nil || url != testRelayURL || token != testRelayToken {
		t.Fatalf("read: %q %v", url, err)
	}
	if err := s.RemoveAPISettingsFile(relayEnv(), settingsNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(s.SettingsPath()); strings.TrimSpace(string(got)) != "{}" {
		t.Fatalf("after remove: %q", got)
	}
	if n := len(settingsBackups(t, runway)); n != 1 {
		t.Fatalf("backups = %d, want 1", n)
	}
}

func TestStoreSettingsBackupPermissionsAndUnchangedApply(t *testing.T) {
	s, configDir, runway := newSettingsStore(t)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	orig := `{"theme":"dark"}`
	if err := os.WriteFile(s.SettingsPath(), []byte(orig), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyAPISettingsFile(relayEnv(), nil, settingsNow); err != nil {
		t.Fatal(err)
	}
	backups := settingsBackups(t, runway)
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	if b, _ := os.ReadFile(backups[0]); string(b) != orig {
		t.Fatalf("backup is not the pre-edit file: %s", b)
	}
	if runtime.GOOS != "windows" {
		bi, _ := os.Stat(backups[0])
		di, _ := os.Stat(filepath.Dir(backups[0]))
		ci, _ := os.Stat(s.SettingsPath())
		if bi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 || ci.Mode().Perm() != 0o640 {
			t.Fatalf("modes: backup %v dir %v settings %v", bi.Mode().Perm(), di.Mode().Perm(), ci.Mode().Perm())
		}
	}
	before, _ := os.ReadFile(s.SettingsPath())
	if err := s.ApplyAPISettingsFile(relayEnv(), nil, settingsNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.SettingsPath())
	if !bytes.Equal(before, after) || len(settingsBackups(t, runway)) != 1 {
		t.Fatal("unchanged apply wrote or backed up")
	}
}

func TestStoreSettingsBackupRotationKeepsNewestTen(t *testing.T) {
	s, _, runway := newSettingsStore(t)
	for i := 0; i < 14; i++ {
		env := APIEnv{BaseURL: "https://r" + string(rune('a'+i)) + ".example", AuthToken: "tok"}
		if err := s.ApplyAPISettingsFile(env, nil, settingsNow.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	backups := settingsBackups(t, runway)
	if len(backups) != 10 {
		t.Fatalf("backups = %d, want 10", len(backups))
	}
	for _, b := range backups {
		data, _ := os.ReadFile(b)
		for _, pruned := range []string{"//ra.", "//rb.", "//rc."} {
			if bytes.Contains(data, []byte(pruned)) {
				t.Fatalf("backup %s should have been pruned", filepath.Base(b))
			}
		}
	}
}

func TestStoreSettingsErrorsNeverLeakTokenAndLeaveFileAlone(t *testing.T) {
	s, configDir, runway := newSettingsStore(t)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	bad := `{"apiKeyHelper": "` + testRelayToken + `",`
	if err := os.WriteFile(s.SettingsPath(), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, readErr := s.ReadAPISettingsFile()
	for _, err := range []error{
		s.ApplyAPISettingsFile(relayEnv(), nil, settingsNow),
		s.RemoveAPISettingsFile(relayEnv(), settingsNow),
		readErr,
	} {
		if err == nil || strings.Contains(err.Error(), testRelayToken) {
			t.Fatalf("err = %v", err)
		}
	}
	if got, _ := os.ReadFile(s.SettingsPath()); string(got) != bad {
		t.Fatal("bad settings file was modified")
	}
	if n := len(settingsBackups(t, runway)); n != 0 {
		t.Fatalf("failed edits made %d backups", n)
	}
}

func TestApplyAPISettingsRejectsLegacyHTTPRelay(t *testing.T) {
	env := APIEnv{BaseURL: "http://relay.example.test", AuthToken: "test-token"}
	if _, err := ApplyAPISettings(nil, env, nil); !errors.Is(err, ErrAPIBaseURL) {
		t.Fatalf("ApplyAPISettings() error = %v, want ErrAPIBaseURL", err)
	}
}
