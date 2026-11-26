package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/platform"
)

const (
	keyX = "sk-test-relay-key-x"
	keyY = "sk-test-relay-key-y"

	relayTokenBody  = `{"code":true,"data":{"total_available":750000,"total_used":250000,"total_granted":1000000,"unlimited_quota":false}}`
	relayStatusBody = `{"success":true,"data":{"quota_per_unit":500000,"quota_display_type":"USD"}}`
)

// fakeRelay is a relay site with a token-usage balance endpoint.
type fakeRelay struct {
	mu     sync.Mutex
	status int
	hits   int
	auths  []string
	srv    *httptest.Server
}

func newFakeRelay(t *testing.T) *fakeRelay {
	t.Helper()
	r := &fakeRelay{status: http.StatusOK}
	r.srv = httptest.NewTLSServer(r)
	t.Cleanup(r.srv.Close)
	return r
}

func (r *fakeRelay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	status := r.status
	r.hits++
	r.auths = append(r.auths, req.Header.Get("Authorization"))
	r.mu.Unlock()
	if status != http.StatusOK {
		w.WriteHeader(status)
		return
	}
	switch req.URL.Path {
	case "/api/usage/token":
		_, _ = w.Write([]byte(relayTokenBody))
	case "/api/status":
		_, _ = w.Write([]byte(relayStatusBody))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (r *fakeRelay) setStatus(code int) {
	r.mu.Lock()
	r.status = code
	r.mu.Unlock()
}

func (r *fakeRelay) hitCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits
}

func (r *fakeRelay) url() string { return r.srv.URL }

type apiRig struct {
	*syncRig
	relay *fakeRelay
}

func newAPIRig(t *testing.T) *apiRig {
	t.Helper()
	r := &apiRig{syncRig: newSyncRig(t), relay: newFakeRelay(t)}
	r.mod.APIClient = r.relay.srv.Client()
	return r
}

func (r *apiRig) addAPI(t *testing.T, alias, key string, p APIProfile) ManagedAccount {
	t.Helper()
	p.BaseURL = r.relay.url()
	acc, err := r.mod.AddAPIAccount(alias, p, key)
	if err != nil {
		t.Fatal(err)
	}
	return acc
}

func (r *apiRig) index(t *testing.T) AccountIndex {
	t.Helper()
	idx, err := r.store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func (r *apiRig) settings(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(r.store.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func (r *apiRig) writeSettings(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(r.store.SettingsPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func liveEnv(m map[string]any) map[string]any {
	env, _ := m["env"].(map[string]any)
	return env
}

func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return data
}

func TestAddAPIAccountKeepsTheKeyOutOfTheIndex(t *testing.T) {
	r := newAPIRig(t)
	acc := r.addAPI(t, "", keyX, APIProfile{Model: " opus-latest ", OpusModel: "relay-opus"})
	if !acc.IsAPI() || acc.Kind != KindAPI || acc.API == nil {
		t.Fatalf("row %+v", acc)
	}
	wantBase := r.relay.url()
	if acc.API.BaseURL != wantBase || acc.API.Model != "opus-latest" || acc.API.OpusModel != "relay-opus" {
		t.Fatalf("profile %+v", acc.API)
	}
	if acc.ID != apiAccountID(wantBase, keyX) || !strings.HasPrefix(acc.ID, "claude-api-") {
		t.Fatalf("id %q", acc.ID)
	}
	if len(acc.API.KeyHint) != 4 || strings.Contains(keyX, acc.API.KeyHint) {
		t.Fatalf("hint %q", acc.API.KeyHint)
	}
	host := strings.TrimPrefix(wantBase, "https://")
	if got := acc.ResolvedDisplayName(); got != host+" ···"+acc.API.KeyHint {
		t.Fatalf("display name %q", got)
	}

	index := fileBytes(t, r.store.IndexPath())
	for _, forbidden := range []string{keyX, "apiKey", "accessToken", "refreshToken"} {
		if bytes.Contains(index, []byte(forbidden)) {
			t.Fatalf("index contains %q:\n%s", forbidden, index)
		}
	}
	if !bytes.Contains(index, []byte(`"kind": "api"`)) {
		t.Fatalf("index has no kind:\n%s", index)
	}
	cred := fileBytes(t, r.store.CredentialPath(acc.ID))
	if strings.TrimSpace(string(cred)) != `{"apiKey":"`+keyX+`"}` {
		t.Fatalf("managed copy %s", cred)
	}
	for _, p := range []string{r.store.AccountDir(acc.ID)} {
		if err := platform.VerifyOwnerOnly(p, 0o700); err != nil {
			t.Fatal(p, err)
		}
	}
	if err := platform.VerifyOwnerOnly(r.store.CredentialPath(acc.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	if key, err := r.store.LoadAPIKey(acc.ID); err != nil || key != keyX {
		t.Fatal("load key", err)
	}
	if k, err := r.mod.APIKey(acc.ID); err != nil || k != keyX {
		t.Fatal("module key", err)
	}
}

func TestReAddSameBaseAndKeyUpdatesTheRow(t *testing.T) {
	r := newAPIRig(t)
	first := r.addAPI(t, "Relay", keyX, APIProfile{SonnetModel: "s1"})
	again := r.addAPI(t, "Renamed", keyX, APIProfile{SonnetModel: "s2", HaikuModel: "h2"})
	if again.ID != first.ID {
		t.Fatalf("a second row was created: %q %q", first.ID, again.ID)
	}
	idx := r.index(t)
	if len(idx.Accounts) != 1 {
		t.Fatalf("accounts %d", len(idx.Accounts))
	}
	got := idx.Accounts[0]
	if got.Alias != "Renamed" || got.API.SonnetModel != "s2" || got.API.HaikuModel != "h2" || got.SortIndex != first.SortIndex {
		t.Fatalf("row %+v %+v", got, got.API)
	}
	other := r.addAPI(t, "", keyY, APIProfile{})
	if other.ID == first.ID || len(r.index(t).Accounts) != 2 {
		t.Fatal("another key must be another account")
	}
	// Different spelling of the same base URL is the same relay.
	p := APIProfile{BaseURL: r.relay.url() + "/messages/?x=1"}
	same, err := r.mod.AddAPIAccount("again", p, keyX)
	if err != nil || same.ID != first.ID {
		t.Fatalf("normalized re-add: %q %v", same.ID, err)
	}
}

func TestAddAPIAccountRejectsBadInputWithoutEchoingTheKey(t *testing.T) {
	r := newAPIRig(t)
	cases := []struct {
		name string
		p    APIProfile
		key  string
		want error
	}{
		{"no key", APIProfile{BaseURL: r.relay.url()}, "  ", ErrAPIKeyRequired},
		{"spaced key", APIProfile{BaseURL: r.relay.url()}, "sk bad key", ErrAPIKeyInvalid},
		{"control char", APIProfile{BaseURL: r.relay.url()}, "sk-\x01bad", ErrAPIKeyInvalid},
		{"no url", APIProfile{}, keyX, ErrAPIBaseURL},
		{"bad scheme", APIProfile{BaseURL: "ftp://relay.example.com"}, keyX, ErrAPIBaseURL},
		{"userinfo", APIProfile{BaseURL: "https://u:" + keyX + "@relay.example.com"}, keyX, ErrAPIBaseURL},
	}
	for _, c := range cases {
		_, err := r.mod.AddAPIAccount("", c.p, c.key)
		if !errors.Is(err, c.want) {
			t.Fatalf("%s: %v", c.name, err)
		}
		if strings.Contains(err.Error(), keyX) || strings.Contains(err.Error(), "relay.example.com") {
			t.Fatalf("%s: error echoes input: %v", c.name, err)
		}
	}
	if len(r.index(t).Accounts) != 0 {
		t.Fatal("a rejected add left a row")
	}
}

func TestUpdateAPIAccountEditsProfileAndHonorsDuplicates(t *testing.T) {
	r := newAPIRig(t)
	x := r.addAPI(t, "", keyX, APIProfile{OpusModel: "old-opus"})
	y := r.addAPI(t, "", keyY, APIProfile{})
	ctx := context.Background()
	if err := r.mod.Refresh(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	if acc, _ := r.index(t).Account(x.ID); acc.APIQuota == nil {
		t.Fatal("no cached balance to keep")
	}

	updated, err := r.mod.UpdateAPIAccount(x.ID, "Work", APIProfile{Model: "m", FableModel: "f"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Alias != "Work" || updated.API.Model != "m" || updated.API.FableModel != "f" || updated.API.OpusModel != "" {
		t.Fatalf("mapping fields are replaced as given: %+v", updated.API)
	}
	if updated.API.BaseURL != x.API.BaseURL || updated.API.KeyHint != x.API.KeyHint || updated.ID != x.ID {
		t.Fatal("an empty base URL and key keep the stored ones")
	}
	if updated.APIQuota == nil {
		t.Fatal("a mapping edit must keep the cached balance")
	}

	rotated, err := r.mod.UpdateAPIAccount(x.ID, "Work", APIProfile{}, "sk-test-rotated-key")
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != x.ID || rotated.API.KeyHint == x.API.KeyHint || rotated.APIQuota != nil || rotated.LastQuotaAt != nil {
		t.Fatalf("a new key keeps the id, changes the hint and drops the balance: %+v", rotated)
	}
	if key, _ := r.store.LoadAPIKey(x.ID); key != "sk-test-rotated-key" {
		t.Fatal("rotated key was not stored")
	}
	if bytes.Contains(fileBytes(t, r.store.IndexPath()), []byte("sk-test-rotated-key")) {
		t.Fatal("index holds the key")
	}

	if _, err := r.mod.UpdateAPIAccount(x.ID, "", APIProfile{}, keyY); !errors.Is(err, ErrAPIDuplicate) {
		t.Fatalf("duplicate base+key: %v", err)
	}
	if _, err := r.mod.UpdateAPIAccount(y.ID, "", APIProfile{BaseURL: "nope"}, ""); !errors.Is(err, ErrAPIBaseURL) {
		t.Fatalf("bad base: %v", err)
	}
	if _, err := r.mod.UpdateAPIAccount("ghost", "", APIProfile{}, ""); err == nil {
		t.Fatal("unknown id")
	}
	oauth := seedAccount(t, r.store, "acct-o", "org-o", false, r.now.Add(time.Hour))
	if _, err := r.mod.UpdateAPIAccount(oauth, "", APIProfile{}, ""); !errors.Is(err, ErrNotAPIAccount) {
		t.Fatalf("oauth account: %v", err)
	}
	// The command forms reach the same code.
	if _, err := r.mod.Apply(ctx, Command{Kind: CommandUpdateAPI, ID: y.ID, Alias: "via command", API: &APIProfile{Model: "cm"}}); err != nil {
		t.Fatal(err)
	}
	if acc, _ := r.index(t).Account(y.ID); acc.Alias != "via command" || acc.API.Model != "cm" {
		t.Fatalf("command update %+v", acc)
	}
	st, err := r.mod.Apply(ctx, Command{Kind: CommandAddAPI, Alias: "new", APIKey: "sk-test-third", API: &APIProfile{BaseURL: r.relay.url()}})
	if err != nil || len(st.Accounts) != 4 {
		t.Fatalf("command add: %d %v", len(st.Accounts), err)
	}
}

func TestEditingTheCurrentAPIAccountRewritesSettings(t *testing.T) {
	r := newAPIRig(t)
	r.writeSettings(t, `{"permissions":{"allow":["Bash"]},"env":{"FOO":"bar"}}`)
	x := r.addAPI(t, "", keyX, APIProfile{OpusModel: "old-opus", Model: "old-model"})
	ctx := context.Background()
	if err := r.mod.Switch(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	env := liveEnv(r.settings(t))
	if env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "old-opus" {
		t.Fatalf("env %v", env)
	}
	if _, err := r.mod.UpdateAPIAccount(x.ID, "", APIProfile{SonnetModel: "new-sonnet"}, ""); err != nil {
		t.Fatal(err)
	}
	got := r.settings(t)
	env = liveEnv(got)
	if _, stale := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; stale || env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "new-sonnet" || env["FOO"] != "bar" {
		t.Fatalf("env after edit %v", env)
	}
	if _, stale := got["model"]; stale {
		t.Fatalf("top-level model not cleared: %v", got)
	}
	if env["ANTHROPIC_AUTH_TOKEN"] != keyX {
		t.Fatal("token")
	}

	// Rotating the key of the current account changes the live token too, and
	// the account stays the one Sync finds.
	if _, err := r.mod.UpdateAPIAccount(x.ID, "", APIProfile{SonnetModel: "new-sonnet"}, "sk-test-rotated-key"); err != nil {
		t.Fatal(err)
	}
	if liveEnv(r.settings(t))["ANTHROPIC_AUTH_TOKEN"] != "sk-test-rotated-key" {
		t.Fatal("live token was not rotated")
	}
	if id, err := r.mod.Sync(ctx); err != nil || id != x.ID {
		t.Fatalf("sync after rotation: %q %v", id, err)
	}

	// Re-adding the same base+key refreshes the live file as well.
	if _, err := r.mod.AddAPIAccount("", APIProfile{BaseURL: r.relay.url(), HaikuModel: "readded"}, "sk-test-rotated-key"); err != nil {
		t.Fatal(err)
	}
	if liveEnv(r.settings(t))["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "readded" {
		t.Fatal("re-add did not reach settings.json")
	}
}

func TestEditingTheCurrentAPIAccountWithoutApplyKeepsSettings(t *testing.T) {
	r := newAPIRig(t)
	x := r.addAPI(t, "", keyX, APIProfile{SonnetModel: "old-sonnet"})
	if err := r.mod.Switch(context.Background(), x.ID, true); err != nil {
		t.Fatal(err)
	}
	if !r.mod.APIAccountInUse(x.ID) {
		t.Fatal("switched account is not in use")
	}
	before, err := os.ReadFile(r.store.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	edited, err := r.mod.EditAPIAccount(x.ID, APIEdit{Alias: "Work", Profile: APIProfile{SonnetModel: "new-sonnet"}, Key: "sk-test-rotated-key"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(r.store.SettingsPath())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("save only rewrote settings.json")
	}
	if edited.Alias != "Work" || edited.API == nil || edited.API.SonnetModel != "new-sonnet" {
		t.Fatalf("edited = %+v", edited)
	}
	if r.mod.APIAccountInUse(x.ID) {
		t.Fatal("settings.json still holds the old key, not this account's")
	}
}

// loginOAuth makes account the official Claude Code login and returns its id.
func (r *apiRig) loginOAuth(t *testing.T, access, account string) string {
	t.Helper()
	r.login(t, access, account)
	id, err := r.mod.Sync(context.Background())
	if err != nil || id == "" {
		t.Fatalf("login sync %q %v", id, err)
	}
	return id
}

func TestSwitchOAuthToAPIToAPIAndBack(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	const original = `{"permissions":{"allow":["Bash"]},"theme":"dark","env":{"FOO":"bar"}}`
	r.writeSettings(t, original)
	a := r.loginOAuth(t, "test-access-a", "acct-a")
	officialPath := filepath.Join(r.config, ".credentials.json")
	claudeJSON := filepath.Join(r.home, ".claude.json")
	officialBefore, configBefore := fileBytes(t, officialPath), fileBytes(t, claudeJSON)

	x := r.addAPI(t, "", keyX, APIProfile{Model: "x-model", OpusModel: "x-opus", SonnetModel: "x-sonnet"})
	y := r.addAPI(t, "", keyY, APIProfile{HaikuModel: "y-haiku"})

	if err := r.mod.Switch(ctx, x.ID, false); err != nil {
		t.Fatal(err)
	}
	got := r.settings(t)
	env := liveEnv(got)
	if env["ANTHROPIC_BASE_URL"] != r.relay.url() || env["ANTHROPIC_AUTH_TOKEN"] != keyX ||
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "x-opus" || env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "x-sonnet" ||
		got["model"] != "x-model" || env["FOO"] != "bar" || got["theme"] != "dark" {
		t.Fatalf("settings after OAuth->API: %v", got)
	}
	if !bytes.Equal(fileBytes(t, officialPath), officialBefore) || !bytes.Equal(fileBytes(t, claudeJSON), configBefore) {
		t.Fatal("an API switch must not touch the official credentials or .claude.json")
	}
	if idx := r.index(t); idx.CurrentAccountID != x.ID {
		t.Fatalf("current %q", idx.CurrentAccountID)
	}
	if acc, _ := r.index(t).Account(x.ID); acc.APIQuota == nil || acc.APIQuota.Status != apiquota.StatusAvailable {
		t.Fatal("the switch must read the balance once")
	}

	if err := r.mod.Switch(ctx, y.ID, false); err != nil {
		t.Fatal(err)
	}
	got = r.settings(t)
	env = liveEnv(got)
	if env["ANTHROPIC_AUTH_TOKEN"] != keyY || env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "y-haiku" || env["FOO"] != "bar" {
		t.Fatalf("settings after API->API: %v", got)
	}
	if _, left := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; left {
		t.Fatal("the previous account's mapping must be cleaned up")
	}
	if _, left := got["model"]; left {
		t.Fatal("the previous account's model must be cleaned up")
	}

	if err := r.mod.Switch(ctx, a, false); err != nil {
		t.Fatal(err)
	}
	var want, have map[string]any
	if err := json.Unmarshal([]byte(original), &want); err != nil {
		t.Fatal(err)
	}
	have = r.settings(t)
	if !reflect.DeepEqual(want, have) {
		t.Fatalf("settings.json was not restored:\n got %v\nwant %v", have, want)
	}
	if idx := r.index(t); idx.CurrentAccountID != a {
		t.Fatalf("current %q", idx.CurrentAccountID)
	}
	doc, err := ParseAuthDocument(fileBytes(t, officialPath))
	if err != nil || doc.OAuth().AccessToken != "test-access-a" {
		t.Fatal("official login", err)
	}
}

func TestSwitchToAPIHonorsRunningProcessGuard(t *testing.T) {
	r := newAPIRig(t)
	x := r.addAPI(t, "", keyX, APIProfile{})
	r.mod.RunningProcessIDs = func(context.Context) ([]int, error) { return []int{7}, nil }
	var running *ProcessesRunningError
	if err := r.mod.Switch(context.Background(), x.ID, false); !errors.As(err, &running) {
		t.Fatalf("guard: %v", err)
	}
	if _, err := os.Stat(r.store.SettingsPath()); err == nil {
		t.Fatal("settings.json written despite the guard")
	}
	if _, err := r.mod.Apply(context.Background(), Command{Kind: CommandSwitch, ID: x.ID, AllowWhileRunning: true}); err != nil {
		t.Fatal(err)
	}
	if liveEnv(r.settings(t))["ANTHROPIC_AUTH_TOKEN"] != keyX {
		t.Fatal("confirmed switch did not apply")
	}
}

func TestSwitchToOAuthRestoresAPISettingsWhenInstallFails(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	r.writeSettings(t, `{"env":{"FOO":"bar"}}`)
	x := r.addAPI(t, "", keyX, APIProfile{OpusModel: "x-opus", Model: "x-model"})
	if err := r.mod.Switch(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	b := seedAccount(t, r.store, "acct-b", "org-b", false, r.now.Add(time.Hour))
	before := r.settings(t)

	// The target's managed copy is gone: the switch fails after the API
	// settings were removed.
	if err := os.Remove(r.store.CredentialPath(b)); err != nil {
		t.Fatal(err)
	}
	var missing *AccountNotFoundError
	if err := r.mod.Switch(ctx, b, true); !errors.As(err, &missing) {
		t.Fatalf("expected the original error, got %v", err)
	}
	if !reflect.DeepEqual(before, r.settings(t)) {
		t.Fatalf("settings not restored:\n got %v\nwant %v", r.settings(t), before)
	}
	if idx := r.index(t); idx.CurrentAccountID != x.ID {
		t.Fatalf("current %q after rollback", idx.CurrentAccountID)
	}

	// The install itself fails: the official store refuses the write.
	c := seedAccount(t, r.store, "acct-c", "org-c", false, r.now.Add(time.Hour))
	r.store.Official = refusingStore{}
	err := r.mod.Switch(ctx, c, true)
	if err == nil || !errors.Is(err, errRefused) {
		t.Fatalf("expected the store error, got %v", err)
	}
	if !reflect.DeepEqual(before, r.settings(t)) {
		t.Fatalf("settings not restored after install failure:\n got %v\nwant %v", r.settings(t), before)
	}
	if idx := r.index(t); idx.CurrentAccountID != x.ID {
		t.Fatalf("current %q after install failure", idx.CurrentAccountID)
	}
}

var errRefused = errors.New("test: write refused")

// refusingStore has no login and rejects every write.
type refusingStore struct{}

func (refusingStore) Load(context.Context) ([]byte, error) { return nil, ErrNotLoggedIn }
func (refusingStore) Save(context.Context, []byte) error   { return errRefused }
func (refusingStore) Exists(context.Context) (bool, error) { return false, nil }

func TestSwitchAwayFromAStaleAPIAccountDoesNotRestoreIt(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.Switch(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	// The user removes the env by hand; the index still calls X current.
	r.writeSettings(t, `{"env":{"FOO":"bar"}}`)
	b := seedAccount(t, r.store, "acct-b", "org-b", false, r.now.Add(time.Hour))
	if err := os.Remove(r.store.CredentialPath(b)); err != nil {
		t.Fatal(err)
	}
	if err := r.mod.Switch(ctx, b, true); err == nil {
		t.Fatal("expected failure")
	}
	if _, back := liveEnv(r.settings(t))["ANTHROPIC_AUTH_TOKEN"]; back {
		t.Fatal("rollback re-applied settings the user had removed")
	}
}

func TestSyncPrefersAnAPIAccountInSettings(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	a := r.loginOAuth(t, "test-access-a", "acct-a")
	x := r.addAPI(t, "", keyX, APIProfile{})
	copyBefore := fileBytes(t, r.store.CredentialPath(a))

	// settings.json names X (as if applied earlier or by hand).
	if err := r.store.ApplyAPISettingsFile(x.API.Env(keyX), nil, r.now); err != nil {
		t.Fatal(err)
	}
	// The official login now belongs to somebody new: it must not be imported.
	r.login(t, "test-access-new", "acct-new")
	id, err := r.mod.Sync(ctx)
	if err != nil || id != x.ID {
		t.Fatalf("API account in settings.json must win: %q %v", id, err)
	}
	idx := r.index(t)
	if idx.CurrentAccountID != x.ID || len(idx.Accounts) != 2 {
		t.Fatalf("index current=%q accounts=%d", idx.CurrentAccountID, len(idx.Accounts))
	}
	if !bytes.Equal(fileBytes(t, r.store.CredentialPath(a)), copyBefore) {
		t.Fatal("the OAuth copy was overwritten")
	}

	// A different token on the same relay is not X: it is added as a new API
	// account, which Claude Code uses, so the OAuth login is still not imported.
	r.writeSettings(t, `{"env":{"ANTHROPIC_BASE_URL":"`+r.relay.url()+`","ANTHROPIC_AUTH_TOKEN":"someone-elses-token"}}`)
	id, err = r.mod.Sync(ctx)
	if err != nil || id == x.ID || id == "" {
		t.Fatalf("an unmanaged token must become its own account: %q %v", id, err)
	}
	if got, _ := r.index(t).Account(id); !got.IsAPI() || len(r.index(t).Accounts) != 3 {
		t.Fatalf("the unmanaged relay was not imported: %+v", got)
	}

	// Back to X, then remove the env by hand: the library follows the login.
	if err := r.store.ApplyAPISettingsFile(x.API.Env(keyX), nil, r.now); err != nil {
		t.Fatal(err)
	}
	if id, _ := r.mod.Sync(ctx); id != x.ID {
		t.Fatal("expected X again")
	}
	if err := r.store.RemoveAPISettingsFile(x.API.Env(keyX), r.now); err != nil {
		t.Fatal(err)
	}
	id, err = r.mod.Sync(ctx)
	if err != nil || id == x.ID || id == "" {
		t.Fatalf("after the env is gone the OAuth login must be followed: %q %v", id, err)
	}
	if idx := r.index(t); idx.CurrentAccountID != id {
		t.Fatalf("index current %q want %q", idx.CurrentAccountID, id)
	}
}

func TestSyncClearsAStaleAPICurrentWithoutLogin(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.Switch(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := r.store.RemoveAPISettingsFile(x.API.Env(keyX), r.now); err != nil {
		t.Fatal(err)
	}
	if id, err := r.mod.Sync(ctx); err != nil || id != "" {
		t.Fatalf("no env and no login: %q %v", id, err)
	}
	if idx := r.index(t); idx.CurrentAccountID != "" {
		t.Fatal("current id not cleared")
	}
	// A broken settings.json matches nothing and does not fail the sync.
	r.writeSettings(t, `not json`)
	if _, err := r.mod.Sync(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestImportOfficialWhileAnAPIAccountIsLive(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.Switch(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	r.login(t, "test-access-a", "acct-a")
	if _, err := r.mod.Apply(ctx, Command{Kind: CommandImportOfficial}); err != nil {
		t.Fatal(err)
	}
	idx := r.index(t)
	if len(idx.Accounts) != 2 || idx.CurrentAccountID != x.ID {
		t.Fatalf("the login is stored and X stays current: current=%q accounts=%d", idx.CurrentAccountID, len(idx.Accounts))
	}
}

func TestRefreshAPIAccountCachesTheBalance(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.Refresh(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	acc, _ := r.index(t).Account(x.ID)
	q := acc.APIQuota
	if q == nil || q.Status != apiquota.StatusAvailable || q.Unit != apiquota.UnitUSD || q.Remaining == nil || *q.Remaining != 1.5 || q.Total == nil || *q.Total != 2 {
		t.Fatalf("quota %+v", q)
	}
	if acc.LastQuotaAt == nil || !acc.LastQuotaAt.Equal(r.now) || acc.LastError != "" || acc.RequiresReauth {
		t.Fatalf("row %+v", acc)
	}
	r.relay.mu.Lock()
	for _, a := range r.relay.auths {
		if a != "" && a != "Bearer "+keyX {
			t.Errorf("unexpected Authorization %q", a)
		}
	}
	r.relay.mu.Unlock()
	if bytes.Contains(fileBytes(t, r.store.IndexPath()), []byte(keyX)) {
		t.Fatal("index holds the key")
	}

	// Spacing: a plain refresh a minute later reads nothing and keeps the cache.
	hits := r.relay.hitCount()
	r.now = r.now.Add(time.Minute)
	if err := r.mod.Refresh(ctx, x.ID, false); err != nil || r.relay.hitCount() != hits {
		t.Fatalf("spaced read: %v hits %d->%d", err, hits, r.relay.hitCount())
	}
	// A forced refresh within 30 seconds is spaced too; after that it reads.
	if err := r.mod.Refresh(ctx, x.ID, true); err != nil || r.relay.hitCount() == hits {
		t.Fatalf("forced read: %v hits %d->%d", err, hits, r.relay.hitCount())
	}
}

func TestRefreshAPIAccountRecordsShortErrorsAndKeepsTheCache(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "", keyX, APIProfile{})
	if err := r.mod.Refresh(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	r.now = r.now.Add(time.Hour)

	r.relay.setStatus(http.StatusUnauthorized)
	err := r.mod.Refresh(ctx, x.ID, true)
	if !errors.Is(err, ErrAPIUnauthorized) {
		t.Fatalf("401: %v", err)
	}
	acc, _ := r.index(t).Account(x.ID)
	if acc.LastError != ErrorCodeAPIUnauthorized || acc.APIQuota == nil || acc.RequiresReauth {
		t.Fatalf("after 401: %+v", acc)
	}

	r.now = r.now.Add(time.Hour)
	r.relay.setStatus(http.StatusInternalServerError)
	err = r.mod.Refresh(ctx, x.ID, true)
	if !errors.Is(err, ErrAPIUnavailable) || strings.Contains(err.Error(), keyX) {
		t.Fatalf("500: %v", err)
	}
	acc, _ = r.index(t).Account(x.ID)
	if acc.LastError != ErrorCodeAPI || acc.APIQuota == nil {
		t.Fatalf("after 500: %+v", acc)
	}
	index := fileBytes(t, r.store.IndexPath())
	if bytes.Contains(index, []byte(keyX)) {
		t.Fatalf("index holds the key:\n%s", index)
	}

	// Recovery clears the error.
	r.now = r.now.Add(time.Hour)
	r.relay.setStatus(http.StatusOK)
	if err := r.mod.Refresh(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	if acc, _ = r.index(t).Account(x.ID); acc.LastError != "" {
		t.Fatalf("error not cleared: %q", acc.LastError)
	}

	// A canceled context records nothing.
	r.now = r.now.Add(time.Hour)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := r.mod.Refresh(canceled, x.ID, true); err == nil {
		t.Fatal("canceled refresh must fail")
	}
	if acc, _ = r.index(t).Account(x.ID); acc.LastError != "" {
		t.Fatalf("cancel recorded an error: %q", acc.LastError)
	}
}

func TestRefreshAPIAccountKeepsAnUnsupportedRelayWithoutError(t *testing.T) {
	r := newAPIRig(t)
	x := r.addAPI(t, "", keyX, APIProfile{})
	r.relay.setStatus(http.StatusNotFound)
	if err := r.mod.Refresh(context.Background(), x.ID, true); err != nil {
		t.Fatal(err)
	}
	acc, _ := r.index(t).Account(x.ID)
	if acc.APIQuota == nil || acc.APIQuota.Status != apiquota.StatusUnsupported || acc.LastError != "" || acc.LastQuotaAt == nil {
		t.Fatalf("unsupported relay: %+v", acc)
	}
}

func TestRefreshAPIAccountNeverFallsBackToADirectConnection(t *testing.T) {
	relay := newFakeRelay(t)
	t.Cleanup(func() { netx.Configure(nil) })
	r := newSyncRig(t)
	acc, err := r.mod.AddAPIAccount("", APIProfile{BaseURL: relay.url()}, keyX)
	if err != nil {
		t.Fatal(err)
	}
	blocked := errors.New("test: proxy is broken")
	netx.Block(blocked)
	err = r.mod.Refresh(context.Background(), acc.ID, true)
	if !errors.Is(err, blocked) {
		t.Fatalf("expected the proxy error, got %v", err)
	}
	if relay.hitCount() != 0 {
		t.Fatal("a request reached the relay around the proxy settings")
	}
	idx, err := r.store.LoadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if row, _ := idx.Account(acc.ID); row.LastError != ErrorCodeAPI {
		t.Fatalf("last error %q", row.LastError)
	}
	if _, err := r.mod.ListAccountAPIModels(context.Background(), acc.ID); !errors.Is(err, blocked) || relay.hitCount() != 0 {
		t.Fatalf("model list must use the same client: %v", err)
	}
}

func TestOAuthOnlyPathsRejectAPIAccounts(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "Relay", keyX, APIProfile{})
	a := seedAccount(t, r.store, "acct-a", "org-a", false, r.now.Add(time.Hour))

	if err := r.store.RefreshManaged(ctx, x.ID, RefreshClient{}, r.now); !errors.Is(err, ErrAPIAccount) {
		t.Fatalf("token refresh of an API account: %v", err)
	}
	if _, err := ParsePasted(`{"apiKey":"` + keyX + `"}`); !errors.Is(err, ErrPastedCredential) {
		t.Fatalf("paste: %v", err)
	}
	if _, err := r.mod.Apply(ctx, Command{Kind: CommandPaste, Text: string(fileBytes(t, r.store.CredentialPath(x.ID)))}); err == nil {
		t.Fatal("pasting the managed API copy must fail")
	}
	if _, err := r.mod.UpdateAPIAccount(a, "", APIProfile{}, ""); !errors.Is(err, ErrNotAPIAccount) {
		t.Fatal(err)
	}
	if _, err := r.mod.APIKey(a); !errors.Is(err, ErrNotAPIAccount) {
		t.Fatal(err)
	}

	// Export leaves API accounts out entirely.
	exp := Exporter{Store: r.store, AppVersion: "test"}
	if _, err := exp.Export([]string{x.ID}, r.now); !errors.Is(err, codex.ErrNoAccountsExported) {
		t.Fatalf("export of only API accounts: %v", err)
	}
	res, err := exp.Export([]string{a, x.ID}, r.now)
	if err != nil || res.ExportedCount != 1 || len(res.Failures) != 1 {
		t.Fatalf("mixed export: %+v %v", res, err)
	}
	pack, _ := json.Marshal(res.Pack)
	if bytes.Contains(pack, []byte(keyX)) || bytes.Contains(pack, []byte("apiKey")) {
		t.Fatal("pack contains API material")
	}
	for _, f := range res.Failures {
		if strings.Contains(f, keyX) {
			t.Fatal("failure text echoes the key")
		}
	}
}

func TestRemoveAPIAccount(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	x := r.addAPI(t, "", keyX, APIProfile{})
	y := r.addAPI(t, "", keyY, APIProfile{})
	if err := r.mod.Switch(ctx, x.ID, true); err != nil {
		t.Fatal(err)
	}
	var current *CurrentAccountCannotBeRemovedError
	if _, err := r.mod.Apply(ctx, Command{Kind: CommandRemove, ID: x.ID}); !errors.As(err, &current) {
		t.Fatalf("the current API account cannot be removed: %v", err)
	}
	st, err := r.mod.Apply(ctx, Command{Kind: CommandRemove, ID: y.ID})
	if err != nil || len(st.Accounts) != 1 {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(r.store.AccountDir(y.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the managed key copy survived the removal")
	}
	if _, err := r.mod.Apply(ctx, Command{Kind: CommandSetAlias, ID: x.ID, Alias: "Renamed"}); err != nil {
		t.Fatal(err)
	}
	if acc, _ := r.index(t).Account(x.ID); acc.Alias != "Renamed" || acc.ResolvedDisplayName() != "Renamed" {
		t.Fatalf("alias %+v", acc)
	}
}

func TestAPIAccountRowRoundTripsThroughTheIndex(t *testing.T) {
	r := newAPIRig(t)
	x := r.addAPI(t, "Relay", keyX, APIProfile{Model: "m", OpusModel: "o", SonnetModel: "s", HaikuModel: "h", FableModel: "f"})
	if err := r.mod.Refresh(context.Background(), x.ID, true); err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Accounts []map[string]json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(fileBytes(t, r.store.IndexPath()), &raw); err != nil || len(raw.Accounts) != 1 {
		t.Fatal(err)
	}
	for _, field := range []string{"kind", "api", "apiQuota"} {
		if _, ok := raw.Accounts[0][field]; !ok {
			t.Fatalf("missing %q in the row", field)
		}
	}
	got, _ := r.index(t).Account(x.ID)
	if got.API == nil || got.API.FableModel != "f" || got.API.KeyHint != x.API.KeyHint || got.APIQuota == nil {
		t.Fatalf("round trip %+v", got)
	}
	// OAuth rows stay free of the new fields.
	seedAccount(t, r.store, "acct-a", "org-a", false, r.now.Add(time.Hour))
	if err := json.Unmarshal(fileBytes(t, r.store.IndexPath()), &raw); err != nil {
		t.Fatal(err)
	}
	for _, row := range raw.Accounts {
		var id string
		_ = json.Unmarshal(row["id"], &id)
		if strings.HasPrefix(id, "claude-api-") {
			continue
		}
		for _, field := range []string{"kind", "api", "apiQuota"} {
			if _, ok := row[field]; ok {
				t.Fatalf("OAuth row %s has %q", id, field)
			}
		}
	}
}

func TestListAPIModelsUsesTheModuleClientAndKey(t *testing.T) {
	var mu sync.Mutex
	var sawKey, sawVersion string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		sawKey, sawVersion = req.Header.Get("x-api-key"), req.Header.Get("anthropic-version")
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":[{"id":"relay-b"},{"id":"relay-a"}]}`))
	}))
	t.Cleanup(srv.Close)
	r := newAPIRig(t)
	r.mod.APIClient = srv.Client()
	acc, err := r.mod.AddAPIAccount("", APIProfile{BaseURL: srv.URL}, keyX)
	if err != nil {
		t.Fatal(err)
	}
	models, err := r.mod.ListAccountAPIModels(context.Background(), acc.ID)
	if err != nil || !reflect.DeepEqual(models, []string{"relay-a", "relay-b"}) {
		t.Fatalf("models %v %v", models, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if sawKey != keyX || sawVersion == "" {
		t.Fatalf("headers key=%q version=%q", sawKey, sawVersion)
	}
}

func TestSyncImportsARelayConfiguredOutsideQuosparr(t *testing.T) {
	r := newAPIRig(t)
	ctx := context.Background()
	r.loginOAuth(t, "test-access-a", "acct-a")
	body := `{"model":"relay-main","env":{"ANTHROPIC_BASE_URL":"` + r.relay.url() + `/","ANTHROPIC_API_KEY":"` + keyY + `","ANTHROPIC_DEFAULT_OPUS_MODEL":"relay-opus","KEEP":"1"}}`
	r.writeSettings(t, body)
	id, err := r.mod.Sync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	idx := r.index(t)
	acc, ok := idx.Account(id)
	if !ok || !acc.IsAPI() || idx.CurrentAccountID != id || len(idx.Accounts) != 2 {
		t.Fatalf("imported = %+v current=%q accounts=%d", acc, idx.CurrentAccountID, len(idx.Accounts))
	}
	if acc.API.BaseURL != r.relay.url() || acc.API.Model != "relay-main" || acc.API.OpusModel != "relay-opus" {
		t.Fatalf("profile = %+v", acc.API)
	}
	if key, err := r.store.LoadAPIKey(id); err != nil || key != keyY {
		t.Fatal("the key was not stored in the managed copy")
	}
	if strings.Contains(string(fileBytes(t, r.store.IndexPath())), keyY) {
		t.Fatal("the key leaked into the index")
	}
	if string(fileBytes(t, r.store.SettingsPath())) != body {
		t.Fatal("sync rewrote settings.json")
	}
	// A second sync finds the same account instead of adding another.
	if again, err := r.mod.Sync(ctx); err != nil || again != id || len(r.index(t).Accounts) != 2 {
		t.Fatalf("second sync: %q %v", again, err)
	}
}

func TestRemoveAPISettingsCleansAnImportedSetup(t *testing.T) {
	in := `{"env":{"ANTHROPIC_BASE_URL":"https://relay.example.com/","ANTHROPIC_API_KEY":"sk-imported","KEEP":"1"}}`
	applied := APIEnv{BaseURL: "https://relay.example.com", AuthToken: "sk-imported"}
	out, changed, err := RemoveAPISettings([]byte(in), applied)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if env := liveEnv(m); len(env) != 1 || env["KEEP"] != "1" {
		t.Fatalf("env = %v", env)
	}
}

func TestAV1BaseURLIsWrittenWithoutTheV1Segment(t *testing.T) {
	r := newAPIRig(t)
	acc := r.addAPI(t, "", keyX, APIProfile{BaseURL: r.relay.url() + "/v1/"})
	if acc.API.BaseURL != r.relay.url() {
		t.Fatalf("stored base %q", acc.API.BaseURL)
	}
	if err := r.mod.Switch(context.Background(), acc.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := liveEnv(r.settings(t))["ANTHROPIC_BASE_URL"]; got != r.relay.url() {
		t.Fatalf("ANTHROPIC_BASE_URL = %v, Claude Code would request /v1/v1/messages", got)
	}
}

func TestALegacyV1BaseIsStillTheAccountInUseAndCanBeRewritten(t *testing.T) {
	r := newAPIRig(t)
	acc := r.addAPI(t, "", keyX, APIProfile{SonnetModel: "s"})
	legacy := r.relay.url() + "/v1"
	// An earlier version stored and wrote the base with its /v1.
	data, err := os.ReadFile(r.store.IndexPath())
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"baseURL": "`+r.relay.url()+`"`), []byte(`"baseURL": "`+legacy+`"`), 1)
	if !bytes.Contains(data, []byte(legacy)) {
		t.Fatal("setup: index has no base URL to rewrite")
	}
	if err := os.WriteFile(r.store.IndexPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
	r.writeSettings(t, `{"env":{"ANTHROPIC_BASE_URL":"`+legacy+`","ANTHROPIC_AUTH_TOKEN":"`+keyX+`","ANTHROPIC_DEFAULT_SONNET_MODEL":"s"}}`)

	if got := r.index(t).Accounts[0].API.BaseURL; got != r.relay.url() {
		t.Fatalf("legacy row read as %q", got)
	}
	inUse, outdated := r.mod.APIAccountState(acc.ID)
	if !inUse || !outdated {
		t.Fatalf("state inUse=%v outdated=%v", inUse, outdated)
	}
	if id, err := r.mod.Sync(context.Background()); err != nil || id != acc.ID || len(r.index(t).Accounts) != 1 {
		t.Fatalf("sync %q %v: a legacy spelling must not import a second account", id, err)
	}
	if _, err := r.mod.EditAPIAccount(acc.ID, APIEdit{Profile: APIProfile{SonnetModel: "s"}, Apply: true}); err != nil {
		t.Fatal(err)
	}
	if got := liveEnv(r.settings(t))["ANTHROPIC_BASE_URL"]; got != r.relay.url() {
		t.Fatalf("apply left ANTHROPIC_BASE_URL = %v", got)
	}
	if inUse, outdated := r.mod.APIAccountState(acc.ID); !inUse || outdated {
		t.Fatalf("after apply inUse=%v outdated=%v", inUse, outdated)
	}
}
