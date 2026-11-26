package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
)

// All keys below are synthetic test strings.
const (
	relayKey1 = "sk-relay-test-key-00000001"
	relayKey2 = "sk-relay-test-key-00000002"
)

func f64(v float64) *float64 { return &v }

func sampleAPIQuota() apiquota.Quota {
	return apiquota.Quota{
		Status: apiquota.StatusAvailable, Remaining: f64(12.5), Used: f64(7.5), Total: f64(20),
		Unit: apiquota.UnitUSD, Source: apiquota.SourceCreditGrants, FetchedAt: testNow,
	}
}

func relayImporter(e testEnv) *Importer {
	return &Importer{Store: e.store, Now: func() time.Time { return testNow }}
}

// ---- account fields ----------------------------------------------------

func TestRelayFieldsRoundTripThroughTheIndex(t *testing.T) {
	acc := NewManagedAccount("", APIKeyAuth(relayKey1), 0, "", "", "", testNow).
		withRelayProfile("https://relay.example.com/v1", "gpt-test").
		ApplyAPIQuota(sampleAPIQuota())
	idx := NewAccountIndex()
	idx.Upsert(acc)
	data := must(MarshalIndex(idx))
	if bytes.Contains(data, []byte(relayKey1)) {
		t.Fatal("the key leaked into index.json")
	}
	back := must(ParseIndex(data))
	got := back.Accounts[0]
	if got.APIBaseURL != "https://relay.example.com/v1" || got.APIModel != "gpt-test" || !got.IsRelay() {
		t.Fatalf("profile lost: %+v", got)
	}
	if got.APIQuota == nil || !reflect.DeepEqual(*got.APIQuota, sampleAPIQuota()) {
		t.Fatalf("quota lost: %+v", got.APIQuota)
	}
	if !strings.HasPrefix(got.DisplayName, "relay.example.com ···") {
		t.Fatalf("display name = %q", got.DisplayName)
	}
}

func TestPlainRowsOmitRelayKeysAndKeepUnknownOnes(t *testing.T) {
	plain := NewManagedAccount("", APIKeyAuth(relayKey1), 0, "", "", "", testNow)
	idx := NewAccountIndex()
	idx.Upsert(plain)
	data := string(must(MarshalIndex(idx)))
	for _, key := range []string{"apiBaseURL", "apiModel", "apiQuota"} {
		if strings.Contains(data, key) {
			t.Fatalf("plain row wrote %s:\n%s", key, data)
		}
	}
	if plain.IsRelay() || !strings.HasPrefix(plain.DisplayName, "API ···") {
		t.Fatalf("plain name = %q", plain.DisplayName)
	}

	// Unknown keys on a relay row survive a read-write cycle.
	relay := plain.withRelayProfile("https://r.example/v1", "")
	raw := must(json.Marshal(relay))
	var obj map[string]any
	noErr(t, json.Unmarshal(raw, &obj))
	obj["futureField"] = "keep-me"
	raw = must(json.Marshal(obj))
	var row ManagedAccount
	noErr(t, json.Unmarshal(raw, &row))
	again := string(must(json.Marshal(row)))
	if !strings.Contains(again, `"futureField":"keep-me"`) || !strings.Contains(again, `"apiBaseURL":"https://r.example/v1"`) {
		t.Fatalf("row = %s", again)
	}
}

func TestDamagedAPIQuotaCacheIsDroppedNotFatal(t *testing.T) {
	acc := NewManagedAccount("", APIKeyAuth(relayKey1), 0, "", "", "", testNow).withRelayProfile("https://r.example/v1", "")
	raw := must(json.Marshal(acc))
	var obj map[string]any
	noErr(t, json.Unmarshal(raw, &obj))
	obj["apiQuota"] = "not an object"
	var row ManagedAccount
	noErr(t, json.Unmarshal(must(json.Marshal(obj)), &row))
	if row.APIQuota != nil || row.APIBaseURL != "https://r.example/v1" {
		t.Fatalf("row = %+v", row)
	}
	if strings.Contains(string(must(json.Marshal(row))), "apiQuota") {
		t.Fatal("a damaged cache was written back")
	}
}

func TestRelayDisplayNameAndAlias(t *testing.T) {
	acc := NewManagedAccount("", APIKeyAuth(relayKey1), 0, "Work", "", "", testNow).withRelayProfile("https://relay.example.com:8443/v1", "")
	if acc.ResolvedDisplayName() != "Work" {
		t.Fatalf("alias must win: %q", acc.ResolvedDisplayName())
	}
	if !strings.HasPrefix(acc.DisplayName, "relay.example.com ···") {
		t.Fatalf("generated name = %q", acc.DisplayName)
	}
	back := acc.withRelayProfile("", "ignored")
	if back.IsRelay() || back.APIModel != "" || back.APIQuota != nil || !strings.HasPrefix(back.DisplayName, "API ···") {
		t.Fatalf("plain again = %+v", back)
	}
}

func TestWithIdentityKeepsRelayFields(t *testing.T) {
	acc := NewManagedAccount("", APIKeyAuth(relayKey1), 0, "", "", "", testNow).
		withRelayProfile("https://relay.example.com/v1", "m").ApplyAPIQuota(sampleAPIQuota())
	got := acc.WithIdentity(APIKeyAuth(relayKey1), "", testNow)
	if got.APIBaseURL != acc.APIBaseURL || got.APIModel != "m" || got.APIQuota == nil || got.DisplayName != acc.DisplayName {
		t.Fatalf("WithIdentity dropped the profile: %+v", got)
	}
	oauth := got.WithIdentity(fixtureAuth(t, "x", false), "", testNow)
	if oauth.APIBaseURL != "" || oauth.APIModel != "" || oauth.APIQuota != nil {
		t.Fatalf("an OAuth identity must not keep relay fields: %+v", oauth)
	}
}

func TestApplyErrorKeepsRelayFields(t *testing.T) {
	acc := NewManagedAccount("", APIKeyAuth(relayKey1), 0, "", "", "", testNow).
		withRelayProfile("https://relay.example.com/v1", "m").ApplyAPIQuota(sampleAPIQuota())
	got := acc.ApplyError("boom", true)
	if got.APIBaseURL == "" || got.APIQuota == nil {
		t.Fatalf("ApplyError dropped relay fields: %+v", got)
	}
}

// ---- import ------------------------------------------------------------

func TestImportRelayAPIKeyNormalizesAndStoresTheProfile(t *testing.T) {
	e := newEnv(t)
	im := relayImporter(e)
	acc := must(im.ImportRelayAPIKey("  "+relayKey1+"\n", RelayProfile{BaseURL: " https://Relay.Example.com/ ", Model: " gpt-test "}, "", true))
	if acc.APIBaseURL != "https://Relay.Example.com/v1" || acc.APIModel != "gpt-test" || !acc.IsRelay() {
		t.Fatalf("profile = %q %q", acc.APIBaseURL, acc.APIModel)
	}
	if !strings.HasPrefix(acc.DisplayName, "Relay.Example.com ···") {
		t.Fatalf("name = %q", acc.DisplayName)
	}
	idx := must(e.store.LoadIndex())
	if idx.ActiveAccountID != acc.ID || len(idx.Accounts) != 1 {
		t.Fatalf("index = %+v", idx)
	}
	raw, _ := os.ReadFile(e.store.IndexPath())
	if bytes.Contains(raw, []byte(relayKey1)) {
		t.Fatal("the key leaked into index.json")
	}
	auth := must(e.store.LoadCredential(acc.ID))
	if auth.APIKey() != relayKey1 {
		t.Fatal("credential not stored")
	}
	if _, err := os.Stat(e.store.ConfigPath()); !os.IsNotExist(err) {
		t.Fatal("import must not create config.toml for an account that is not installed")
	}
}

func TestImportRelayAPIKeyRejectsBadInput(t *testing.T) {
	im := relayImporter(newEnv(t))
	for _, base := range []string{"ftp://r.example", "not a url", "https://user:pw@r.example", "r.example"} {
		if _, err := im.ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: base}, "", false); !errors.Is(err, ErrInvalidRelayBaseURL) {
			t.Fatalf("%q: err = %v", base, err)
		}
	}
	if _, err := im.ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://r.example", Model: "a\nb"}, "", false); !errors.Is(err, ErrInvalidRelayModel) {
		t.Fatalf("model: err = %v", err)
	}
	if _, err := im.ImportRelayAPIKey("  ", RelayProfile{BaseURL: "https://r.example"}, "", false); !errors.Is(err, ErrRelayKeyRequired) {
		t.Fatalf("empty key: err = %v", err)
	}
	for _, bad := range []string{"sk-a b", "sk-a\nb", "sk-a\x00b", "sk-\xffb"} {
		if _, err := im.ImportRelayAPIKey(bad, RelayProfile{BaseURL: "https://r.example"}, "", false); !errors.Is(err, ErrRelayKeyInvalid) {
			t.Fatalf("key with bad characters accepted: err = %v", err)
		}
	}
	if idx := must(im.Store.LoadIndex()); len(idx.Accounts) != 0 {
		t.Fatalf("a rejected import left rows: %d", len(idx.Accounts))
	}
}

func TestImportRelayAPIKeySameKeyUpdatesTheRow(t *testing.T) {
	e := newEnv(t)
	im := relayImporter(e)
	first := must(im.ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"}, "Mine", false))
	noErr(t, e.store.UpdateMetadata(first.ApplyAPIQuota(sampleAPIQuota())))

	// No base URL: the profile, alias and balance stay.
	kept := must(im.ImportRelayAPIKey(relayKey1, RelayProfile{}, "", false))
	if kept.ID != first.ID || kept.APIBaseURL != "https://a.example.com/v1" || kept.APIModel != "m1" || kept.Alias != "Mine" || kept.APIQuota == nil {
		t.Fatalf("kept = %+v", kept)
	}
	// A new base URL replaces the whole profile and drops the old balance.
	moved := must(im.ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://b.example.com/v1"}, "", false))
	if moved.ID != first.ID || moved.APIBaseURL != "https://b.example.com/v1" || moved.APIModel != "" || moved.APIQuota != nil || moved.Alias != "Mine" {
		t.Fatalf("moved = %+v", moved)
	}
	if idx := must(e.store.LoadIndex()); len(idx.Accounts) != 1 {
		t.Fatalf("rows = %d", len(idx.Accounts))
	}
	// A different key is a different row.
	other := must(im.ImportRelayAPIKey(relayKey2, RelayProfile{BaseURL: "https://b.example.com"}, "", false))
	if other.ID == first.ID || len(must(e.store.LoadIndex()).Accounts) != 2 {
		t.Fatal("a second key must be its own row")
	}
}

func TestPlainImportAPIKeyKeepsAnExistingProfile(t *testing.T) {
	e := newEnv(t)
	im := relayImporter(e)
	first := must(im.ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://a.example.com"}, "", false))
	again := must(im.ImportAPIKey(relayKey1, false))
	if again.ID != first.ID || !again.IsRelay() {
		t.Fatalf("a plain re-import dropped the profile: %+v", again)
	}
}

// ---- UpdateRelayProfile ------------------------------------------------

func readConfig(t *testing.T, e testEnv) string {
	t.Helper()
	data, err := os.ReadFile(e.store.ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	noErr(t, err)
	return string(data)
}

func writeConfig(t *testing.T, e testEnv, text string) {
	t.Helper()
	noErr(t, os.MkdirAll(e.codexHome, 0o700))
	noErr(t, os.WriteFile(e.store.ConfigPath(), []byte(text), 0o644))
}

// unsupportedConfig parses, but defines the managed provider in a form the
// editor refuses to touch.
const unsupportedConfig = "[model_providers]\nquosparr = { name = \"x\" }\n"

const userConfig = "# my codex config\nsandbox_mode = \"workspace-write\"\n\n[profiles.fast]\nreasoning = \"low\"\n"

func TestUpdateRelayProfileTurnsAKeyIntoARelayAndBack(t *testing.T) {
	e := newEnv(t)
	im := relayImporter(e)
	plain := must(im.ImportAPIKey(relayKey1, false))
	if plain.IsRelay() {
		t.Fatal("setup")
	}
	relay := must(e.store.UpdateRelayProfile(plain.ID, RelayProfile{BaseURL: "https://r.example.com/", Model: "gpt-x"}, testNow))
	if relay.APIBaseURL != "https://r.example.com/v1" || relay.APIModel != "gpt-x" || relay.ID != plain.ID {
		t.Fatalf("relay = %+v", relay)
	}
	saved := must(e.store.LoadIndex()).Accounts[0]
	if saved.APIBaseURL != relay.APIBaseURL || !strings.HasPrefix(saved.DisplayName, "r.example.com ···") {
		t.Fatalf("saved = %+v", saved)
	}
	back := must(e.store.UpdateRelayProfile(plain.ID, RelayProfile{BaseURL: "  ", Model: "ignored"}, testNow))
	if back.IsRelay() || back.APIModel != "" || !strings.HasPrefix(back.DisplayName, "API ···") {
		t.Fatalf("back = %+v", back)
	}
}

func TestUpdateRelayProfileErrors(t *testing.T) {
	e := newEnv(t)
	oauth := upsert(t, e.store, fixtureAuth(t, "one", false), true)
	if _, err := e.store.UpdateRelayProfile(oauth.ID, RelayProfile{BaseURL: "https://r.example"}, testNow); !errors.Is(err, ErrNotAPIKeyAccount) {
		t.Fatalf("oauth: %v", err)
	}
	var missing *AccountNotFoundError
	if _, err := e.store.UpdateRelayProfile("nope", RelayProfile{BaseURL: "https://r.example"}, testNow); !errors.As(err, &missing) {
		t.Fatalf("missing: %v", err)
	}
	key := must(relayImporter(e).ImportAPIKey(relayKey1, false))
	if _, err := e.store.UpdateRelayProfile(key.ID, RelayProfile{BaseURL: "ftp://x"}, testNow); !errors.Is(err, ErrInvalidRelayBaseURL) {
		t.Fatalf("url: %v", err)
	}
}

// installRelay makes a relay account the official login the way a switch does.
func installRelay(t *testing.T, e testEnv, profile RelayProfile) ManagedAccount {
	t.Helper()
	acc := must(relayImporter(e).ImportRelayAPIKey(relayKey1, profile, "", true))
	e.writeOfficial(t, APIKeyAuth(relayKey1))
	return acc
}

func TestUpdateRelayProfileOnInactiveAccountLeavesConfigAlone(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, userConfig)
	upsert(t, e.store, fixtureAuth(t, "one", false), true)
	other := must(relayImporter(e).ImportAPIKey(relayKey1, false))
	must(e.store.UpdateRelayProfile(other.ID, RelayProfile{BaseURL: "https://r.example.com"}, testNow))
	if readConfig(t, e) != userConfig {
		t.Fatal("config.toml changed for a non-active account")
	}
}

func TestUpdateRelayProfileOnActiveAccountFollowsInConfig(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, userConfig)
	acc := installRelay(t, e, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"})
	// The import ran before the official login held the key, so config.toml is
	// first written by this update.
	acc = must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://b.example.com", Model: "m2"}, testNow))
	active, base, err := ReadManagedProvider([]byte(readConfig(t, e)))
	noErr(t, err)
	if !active || base != "https://b.example.com/v1" {
		t.Fatalf("config = %q", readConfig(t, e))
	}
	if !strings.Contains(readConfig(t, e), `model = "m2"`) || !strings.Contains(readConfig(t, e), "[profiles.fast]") {
		t.Fatalf("config = %q", readConfig(t, e))
	}
	// Back to a plain key: the provider and the model go, the rest stays.
	must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{}, testNow))
	if got := readConfig(t, e); got != userConfig {
		t.Fatalf("config after removal:\n%s", got)
	}
	backups, _ := filepath.Glob(filepath.Join(e.runwayHome, "backups", "codex-config-*.toml"))
	if len(backups) == 0 {
		t.Fatal("config.toml was edited without a backup")
	}
}

func TestUpdateRelayProfileSkipsConfigWhenOfficialIsAnotherLogin(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, userConfig)
	acc := must(relayImporter(e).ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://a.example.com"}, "", true))
	e.writeOfficial(t, fixtureAuth(t, "someone", false))
	must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://b.example.com"}, testNow))
	if readConfig(t, e) != userConfig {
		t.Fatal("config.toml must not pair the relay with another login")
	}
}

func TestUpdateRelayProfileRollsBack(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, userConfig)
	acc := installRelay(t, e, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"})
	must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"}, testNow))
	configBefore, indexBefore := readConfig(t, e), must(os.ReadFile(e.store.IndexPath()))

	boom := errors.New("injected index failure")
	e.store.failpoint = func(stage string) error {
		if stage == "index" {
			return boom
		}
		return nil
	}
	_, err := e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://b.example.com", Model: "m2"}, testNow)
	if !errors.Is(err, boom) || errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("err = %v", err)
	}
	if readConfig(t, e) != configBefore || !bytes.Equal(must(os.ReadFile(e.store.IndexPath())), indexBefore) {
		t.Fatal("a failed update changed config.toml or the index")
	}
	e.store.failpoint = nil

	// A config.toml the editor cannot handle fails the update and changes nothing.
	writeConfig(t, e, unsupportedConfig)
	_, err = e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://b.example.com"}, testNow)
	if !errors.Is(err, ErrProviderConfigInvalid) && !errors.Is(err, ErrProviderConfigUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if readConfig(t, e) != unsupportedConfig || !bytes.Equal(must(os.ReadFile(e.store.IndexPath())), indexBefore) {
		t.Fatal("a failed update changed state")
	}
}

func TestImportRelayAPIKeyReappliesConfigOfTheInstalledKey(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, userConfig)
	installRelay(t, e, RelayProfile{BaseURL: "https://a.example.com"})
	must(relayImporter(e).ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://b.example.com", Model: "mm"}, "", true))
	_, base, err := ReadManagedProvider([]byte(readConfig(t, e)))
	noErr(t, err)
	if base != "https://b.example.com/v1" {
		t.Fatalf("config = %q", readConfig(t, e))
	}
}

// ---- switching ---------------------------------------------------------

type relaySwitchFixture struct {
	testEnv
	oauth, relay ManagedAccount
}

func newRelaySwitchFixture(t *testing.T, profile RelayProfile, config string) relaySwitchFixture {
	t.Helper()
	f := relaySwitchFixture{testEnv: newEnv(t)}
	f.oauth = upsert(t, f.store, fixtureAuth(t, "current", false), true)
	f.writeOfficial(t, fixtureAuth(t, "current", false))
	f.relay = must(relayImporter(f.testEnv).ImportRelayAPIKey(relayKey1, profile, "", false))
	if config != "" {
		writeConfig(t, f.testEnv, config)
	}
	return f
}

func (f relaySwitchFixture) switcher() *Switcher {
	return &Switcher{Store: f.store, Now: func() time.Time { return testNow }}
}

func (f relaySwitchFixture) switchTo(t *testing.T, id string) {
	t.Helper()
	if _, err := f.switcher().SwitchTo(context.Background(), id); err != nil {
		t.Fatal(err)
	}
}

func TestSwitchRelayRoundTripRestoresConfigBytes(t *testing.T) {
	for name, config := range map[string]string{"with-config": userConfig, "no-config": ""} {
		t.Run(name, func(t *testing.T) {
			f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com", Model: "gpt-test"}, config)
			f.switchTo(t, f.relay.ID)

			now := readConfig(t, f.testEnv)
			active, base, err := ReadManagedProvider([]byte(now))
			noErr(t, err)
			if !active || base != "https://relay.example.com/v1" || !strings.Contains(now, `model = "gpt-test"`) {
				t.Fatalf("relay config:\n%s", now)
			}
			if strings.Contains(now, relayKey1) {
				t.Fatal("the key was written to config.toml")
			}
			auth := must(f.store.LoadOfficialAuth())
			if !auth.IsAPIKey() || auth.APIKey() != relayKey1 {
				t.Fatal("official login is not the relay key")
			}
			if idx := must(f.store.LoadIndex()); idx.ActiveAccountID != f.relay.ID {
				t.Fatalf("active = %q", idx.ActiveAccountID)
			}

			f.switchTo(t, f.oauth.ID)
			if got := readConfig(t, f.testEnv); got != config {
				t.Fatalf("config after switching back:\n%q\nwant\n%q", got, config)
			}
			if auth := must(f.store.LoadOfficialAuth()); auth.IsAPIKey() || auth.APIKey() != "" {
				t.Fatal("official login is still the key")
			}
			if bytes.Contains(f.officialBytes(t), []byte(relayKey1)) {
				t.Fatal("the relay key stayed in the official auth.json after switching away")
			}
		})
	}
}

func TestSwitchRelayWithoutModelKeepsUserModel(t *testing.T) {
	config := "model = \"gpt-5\"\n" + userConfig
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com"}, config)
	f.switchTo(t, f.relay.ID)
	if !strings.Contains(readConfig(t, f.testEnv), `model = "gpt-5"`) {
		t.Fatalf("user model lost:\n%s", readConfig(t, f.testEnv))
	}
	f.switchTo(t, f.oauth.ID)
	if got := readConfig(t, f.testEnv); got != config {
		t.Fatalf("config = %q", got)
	}
}

func TestSwitchBetweenRelaysDropsTheOldModel(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://a.example.com", Model: "model-a"}, userConfig)
	second := must(relayImporter(f.testEnv).ImportRelayAPIKey(relayKey2, RelayProfile{BaseURL: "https://b.example.com"}, "", false))
	f.switchTo(t, f.relay.ID)
	f.switchTo(t, second.ID)
	got := readConfig(t, f.testEnv)
	_, base, err := ReadManagedProvider([]byte(got))
	noErr(t, err)
	if base != "https://b.example.com/v1" || strings.Contains(got, "model-a") {
		t.Fatalf("config:\n%s", got)
	}
	f.switchTo(t, f.oauth.ID)
	if got := readConfig(t, f.testEnv); got != userConfig {
		t.Fatalf("config = %q", got)
	}
}

func TestSwitchRelayRollsBackConfigOnLaterFailure(t *testing.T) {
	for _, stage := range []string{"config", "index"} {
		t.Run(stage, func(t *testing.T) {
			f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com", Model: "gpt-test"}, userConfig)
			files := []string{f.store.OfficialAuthPath(), f.store.CredentialPath(f.relay.ID), f.store.IndexPath(), f.store.ConfigPath()}
			before := map[string][]byte{}
			for _, p := range files {
				before[p], _ = os.ReadFile(p)
			}
			boom := errors.New("injected " + stage)
			sw := f.switcher()
			sw.failpoint = func(s string) error {
				if s == stage {
					return boom
				}
				return nil
			}
			if _, err := sw.SwitchTo(context.Background(), f.relay.ID); !errors.Is(err, boom) || errors.Is(err, ErrRollbackFailed) {
				t.Fatalf("err = %v", err)
			}
			for _, p := range files {
				got, _ := os.ReadFile(p)
				if !bytes.Equal(got, before[p]) {
					t.Fatalf("%s differs after rollback", filepath.Base(p))
				}
			}
		})
	}
}

func TestSwitchRelayRollbackRemovesANewConfig(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com"}, "")
	boom := errors.New("injected index")
	sw := f.switcher()
	sw.failpoint = func(s string) error {
		if s == "index" {
			return boom
		}
		return nil
	}
	if _, err := sw.SwitchTo(context.Background(), f.relay.ID); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(f.store.ConfigPath()); !os.IsNotExist(err) {
		t.Fatal("a failed switch left a new config.toml behind")
	}
}

func TestSwitchToRelayFailsCleanlyOnUnusableConfig(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com"}, unsupportedConfig)
	officialBefore := f.officialBytes(t)
	_, err := f.switcher().SwitchTo(context.Background(), f.relay.ID)
	if !errors.Is(err, ErrProviderConfigInvalid) && !errors.Is(err, ErrProviderConfigUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Equal(f.officialBytes(t), officialBefore) || readConfig(t, f.testEnv) != unsupportedConfig {
		t.Fatal("a failed switch changed files")
	}
	if idx := must(f.store.LoadIndex()); idx.ActiveAccountID != f.oauth.ID {
		t.Fatalf("active = %q", idx.ActiveAccountID)
	}
}

func TestPlainSwitchLeavesAnUnrelatedConfigAlone(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com"}, userConfig)
	other := upsert(t, f.store, fixtureAuth(t, "other", false), false)
	f.switchTo(t, other.ID)
	if readConfig(t, f.testEnv) != userConfig {
		t.Fatal("config.toml was touched by a plain switch")
	}
	if backups, _ := filepath.Glob(filepath.Join(f.runwayHome, "backups", "codex-config-*")); len(backups) != 0 {
		t.Fatal("a plain switch made a config backup")
	}
}

func TestPlainSwitchRemovesALeftoverManagedProvider(t *testing.T) {
	// The index says OAuth is active, but config.toml still names the managed
	// provider (the login was replaced outside Quosparr).
	leftover := userConfig
	var err error
	out, err := ApplyManagedProvider([]byte(leftover), ProviderConfig{Name: "Old", BaseURL: "https://old.example/v1"})
	noErr(t, err)
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com"}, string(out))
	other := upsert(t, f.store, fixtureAuth(t, "other", false), false)
	f.switchTo(t, other.ID)
	if got := readConfig(t, f.testEnv); got != userConfig {
		t.Fatalf("config = %q", got)
	}
}

func TestSwitchEditsConfigThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com"}, "")
	target := filepath.Join(t.TempDir(), "dotfiles-config.toml")
	noErr(t, os.WriteFile(target, []byte(userConfig), 0o644))
	noErr(t, os.MkdirAll(f.codexHome, 0o700))
	noErr(t, os.Symlink(target, f.store.ConfigPath()))

	f.switchTo(t, f.relay.ID)
	if info := must(os.Lstat(f.store.ConfigPath())); info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced")
	}
	if active, _, _ := ReadManagedProvider(must(os.ReadFile(target))); !active {
		t.Fatal("the link target was not updated")
	}
	f.switchTo(t, f.oauth.ID)
	if got := string(must(os.ReadFile(target))); got != userConfig {
		t.Fatalf("target = %q", got)
	}
	if info := must(os.Lstat(f.store.ConfigPath())); info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced on the way back")
	}
}

func TestRelayRowSurvivesOfficialSync(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.com", Model: "gpt-test"}, "")
	f.switchTo(t, f.relay.ID)
	idx, status, err := f.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	if status != OfficialSynced {
		t.Fatalf("status = %v", status)
	}
	got, _ := idx.Account(f.relay.ID)
	if !got.IsRelay() || got.APIModel != "gpt-test" || idx.ActiveAccountID != f.relay.ID {
		t.Fatalf("relay row after sync: %+v", got)
	}
	// Cached balance survives a switch (it belongs to the site, not the session).
	noErr(t, f.store.UpdateMetadata(got.ApplyAPIQuota(sampleAPIQuota())))
	f.switchTo(t, f.oauth.ID)
	f.switchTo(t, f.relay.ID)
	again, _ := must(f.store.LoadIndex()).Account(f.relay.ID)
	if again.APIQuota == nil {
		t.Fatal("switching dropped the cached balance")
	}
}

func TestAddRelayAPIKeyRefusesAKeyTheLibraryHolds(t *testing.T) {
	e := newEnv(t)
	im := relayImporter(e)
	first := must(im.AddRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"}, "Mine", false))
	files := []string{e.store.IndexPath(), e.store.CredentialPath(first.ID)}
	read := func() [][]byte {
		var out [][]byte
		for _, p := range files {
			out = append(out, must(os.ReadFile(p)))
		}
		return out
	}
	before := read()

	for _, base := range []string{"https://a.example.com", "https://b.example.com", ""} {
		if _, err := im.AddRelayAPIKey("  "+relayKey1+" ", RelayProfile{BaseURL: base, Model: "other"}, "Other", true); !errors.Is(err, ErrRelayDuplicate) {
			t.Fatalf("base %q: err = %v", base, err)
		}
	}
	if !reflect.DeepEqual(before, read()) {
		t.Fatal("a refused add changed the library")
	}
	row := must(e.store.LoadIndex())
	if got, _ := row.Account(first.ID); got.APIBaseURL != "https://a.example.com/v1" || got.APIModel != "m1" || got.Alias != "Mine" {
		t.Fatalf("existing row changed: %+v", got)
	}
	if _, err := im.AddRelayAPIKey(relayKey2, RelayProfile{BaseURL: "https://a.example.com"}, "", false); err != nil {
		t.Fatalf("a different key on the same site: %v", err)
	}
	if _, err := im.AddRelayAPIKey("", RelayProfile{BaseURL: "https://a.example.com"}, "", false); !errors.Is(err, ErrRelayKeyRequired) {
		t.Fatalf("empty key: %v", err)
	}
}

// ---- relays configured outside Quosparr -------------------------------

const externalRelayConfig = "model_provider = \"other\"\nmodel = \"glm-4.6\"\n\n[model_providers.other]\nname = \"Other\"\nbase_url = \"https://other.example.com/\"\nwire_api = \"responses\"\nrequires_openai_auth = true\n"

func TestSyncAdoptsTheRelayConfigTomlSelects(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, externalRelayConfig)
	e.writeOfficial(t, APIKeyAuth(relayKey1))
	idx, _, err := e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	if len(idx.Accounts) != 1 {
		t.Fatalf("accounts = %d", len(idx.Accounts))
	}
	acc := idx.Accounts[0]
	if !acc.IsRelay() || acc.APIBaseURL != "https://other.example.com/v1" || acc.APIModel != "glm-4.6" || idx.ActiveAccountID != acc.ID {
		t.Fatalf("adopted = %+v active=%q", acc, idx.ActiveAccountID)
	}
	if readConfig(t, e) != externalRelayConfig {
		t.Fatal("sync rewrote config.toml")
	}
	// A site chosen by the user afterwards is not replaced by the next sync.
	edited := must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://mine.example.com"}, testNow))
	idx, _, err = e.store.SyncFromOfficialAuth(testNow)
	noErr(t, err)
	if got, _ := idx.Account(acc.ID); got.APIBaseURL != edited.APIBaseURL {
		t.Fatalf("sync replaced the edited site: %q", got.APIBaseURL)
	}
}

func TestSyncLeavesPlainKeysWithoutARelay(t *testing.T) {
	for name, config := range map[string]string{
		"none":   "",
		"openai": "model_provider = \"openai\"\n",
		"no-url": "model_provider = \"oss\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			if config != "" {
				writeConfig(t, e, config)
			}
			e.writeOfficial(t, APIKeyAuth(relayKey1))
			idx, _, err := e.store.SyncFromOfficialAuth(testNow)
			noErr(t, err)
			if len(idx.Accounts) != 1 || idx.Accounts[0].IsRelay() {
				t.Fatalf("accounts = %+v", idx.Accounts)
			}
		})
	}
}

func TestSwitchAwayFromAnAdoptedRelayDeselectsIt(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://other.example.com", Model: "glm-4.6"}, externalRelayConfig)
	f.writeOfficial(t, APIKeyAuth(relayKey1))
	idx := must(f.store.LoadIndex())
	idx.ActiveAccountID = f.relay.ID
	noErr(t, f.store.saveIndex(idx))

	f.switchTo(t, f.oauth.ID)
	got := readConfig(t, f.testEnv)
	if strings.Contains(got, "model_provider =") || strings.Contains(got, `model = "glm-4.6"`) {
		t.Fatalf("the relay is still selected:\n%s", got)
	}
	if !strings.Contains(got, "[model_providers.other]") || !strings.Contains(got, "https://other.example.com/") {
		t.Fatalf("the user's provider table was removed:\n%s", got)
	}
}

func TestRemoveProviderKeepsAnUnrelatedSelection(t *testing.T) {
	out, changed, err := removeProvider([]byte(externalRelayConfig), "", "https://elsewhere.example.com/v1")
	noErr(t, err)
	if changed || string(out) != externalRelayConfig {
		t.Fatalf("an unrelated provider was touched:\n%s", out)
	}
}

// ---- EditRelayAccount ---------------------------------------------------

func TestEditRelayAccountReplacesTheKeyInPlace(t *testing.T) {
	e := newEnv(t)
	acc := must(relayImporter(e).ImportRelayAPIKey(relayKey1, RelayProfile{BaseURL: "https://a.example.com"}, "", false))
	edited := must(e.store.EditRelayAccount(acc.ID, RelayEdit{Profile: RelayProfile{BaseURL: "https://a.example.com"}, Key: " " + relayKey2 + " "}, testNow))
	if edited.ID != acc.ID || !AccountMatches(edited, APIKeyAuth(relayKey2)) || AccountMatches(edited, APIKeyAuth(relayKey1)) {
		t.Fatalf("edited = %+v", edited)
	}
	if cred := must(e.store.LoadCredential(acc.ID)); cred.APIKey() != relayKey2 {
		t.Fatal("managed copy kept the old key")
	}
	if len(must(e.store.LoadIndex()).Accounts) != 1 {
		t.Fatal("a new key added a row")
	}
	if _, err := os.Stat(e.officialPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("an inactive account wrote the official login")
	}
}

func TestEditRelayAccountRejectsAKeyOfAnotherRow(t *testing.T) {
	e := newEnv(t)
	im := relayImporter(e)
	acc := must(im.ImportAPIKey(relayKey1, false))
	must(im.ImportAPIKey(relayKey2, false))
	if _, err := e.store.EditRelayAccount(acc.ID, RelayEdit{Key: relayKey2}, testNow); !errors.Is(err, ErrRelayDuplicate) {
		t.Fatalf("err = %v", err)
	}
	if _, err := e.store.EditRelayAccount(acc.ID, RelayEdit{Key: "not a key"}, testNow); !errors.Is(err, ErrRelayKeyInvalid) {
		t.Fatalf("err = %v", err)
	}
	if cred := must(e.store.LoadCredential(acc.ID)); cred.APIKey() != relayKey1 {
		t.Fatal("a refused edit changed the key")
	}
}

func TestEditRelayAccountInUseWritesOnlyWhenApplied(t *testing.T) {
	setup := func(t *testing.T) (testEnv, ManagedAccount) {
		e := newEnv(t)
		writeConfig(t, e, userConfig)
		acc := installRelay(t, e, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"})
		must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://a.example.com", Model: "m1"}, testNow))
		if !e.store.RelayInUse(acc.ID) {
			t.Fatal("installed relay is not in use")
		}
		return e, acc
	}
	edit := RelayEdit{Profile: RelayProfile{BaseURL: "https://b.example.com", Model: "m2"}, Key: relayKey2}

	t.Run("save only", func(t *testing.T) {
		e, acc := setup(t)
		configBefore, officialBefore := readConfig(t, e), e.officialBytes(t)
		must(e.store.EditRelayAccount(acc.ID, edit, testNow))
		if readConfig(t, e) != configBefore || !bytes.Equal(e.officialBytes(t), officialBefore) {
			t.Fatal("save only rewrote the official files")
		}
		// The login still holds the old key, which is no longer this row's.
		if e.store.RelayInUse(acc.ID) {
			t.Fatal("a row with a key the login does not hold is in use")
		}
	})
	t.Run("apply", func(t *testing.T) {
		e, acc := setup(t)
		applied := edit
		applied.Apply = true
		must(e.store.EditRelayAccount(acc.ID, applied, testNow))
		if official := must(e.store.LoadOfficialAuth()); official.APIKey() != relayKey2 {
			t.Fatal("apply did not install the new key")
		}
		if _, base, _ := ReadManagedProvider([]byte(readConfig(t, e))); base != "https://b.example.com/v1" || !strings.Contains(readConfig(t, e), `model = "m2"`) {
			t.Fatalf("config = %q", readConfig(t, e))
		}
		if !e.store.RelayInUse(acc.ID) {
			t.Fatal("the edited row no longer matches the login")
		}
	})
}

func TestEditRelayAccountRollsBackAnAppliedKey(t *testing.T) {
	e := newEnv(t)
	writeConfig(t, e, userConfig)
	acc := installRelay(t, e, RelayProfile{BaseURL: "https://a.example.com"})
	must(e.store.UpdateRelayProfile(acc.ID, RelayProfile{BaseURL: "https://a.example.com"}, testNow))
	configBefore, officialBefore := readConfig(t, e), e.officialBytes(t)
	indexBefore := must(os.ReadFile(e.store.IndexPath()))

	boom := errors.New("injected index failure")
	e.store.failpoint = func(stage string) error {
		if stage == "index" {
			return boom
		}
		return nil
	}
	_, err := e.store.EditRelayAccount(acc.ID, RelayEdit{Profile: RelayProfile{BaseURL: "https://b.example.com"}, Key: relayKey2, Apply: true}, testNow)
	if !errors.Is(err, boom) || errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("err = %v", err)
	}
	if readConfig(t, e) != configBefore || !bytes.Equal(e.officialBytes(t), officialBefore) || !bytes.Equal(must(os.ReadFile(e.store.IndexPath())), indexBefore) {
		t.Fatal("a failed edit left changes behind")
	}
	if cred := must(e.store.LoadCredential(acc.ID)); cred.APIKey() != relayKey1 {
		t.Fatal("a failed edit kept the new managed key")
	}
}

func TestLegacyHTTPRelayCannotPrepareSwitch(t *testing.T) {
	f := newRelaySwitchFixture(t, RelayProfile{BaseURL: "https://relay.example.test"}, userConfig)
	legacy := f.relay.withRelayProfile("http://relay.example.test/v1", "")
	noErr(t, f.store.UpdateMetadata(legacy))
	before := f.officialBytes(t)
	if _, err := f.switcher().PrepareSwitch(context.Background(), legacy.ID); !errors.Is(err, ErrRelayBaseURL) {
		t.Fatalf("PrepareSwitch() error = %v, want ErrRelayBaseURL", err)
	}
	if !bytes.Equal(f.officialBytes(t), before) || readConfig(t, f.testEnv) != userConfig {
		t.Fatal("rejected preparation changed the live account")
	}
}
