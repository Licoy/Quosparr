package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

func TestReplaceOAuthKeepsMCPAndOtherKeys(t *testing.T) {
	official := []byte(`{"mcpOAuth":{"server":{"token":"mcp-token"}},"theme":"dark","claudeAiOauth":{"accessToken":"old","refreshToken":"old-refresh","scopes":["user:inference"]}}`)
	target := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token","refreshToken":"test-refresh-token","expiresAt":1700000000000,"scopes":["user:inference"],"subscriptionType":"pro"}}`)
	out, err := ReplaceOAuth(official, target)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	if string(bytes.TrimSpace(root["mcpOAuth"])) != `{"server":{"token":"mcp-token"}}` {
		t.Fatalf("mcpOAuth changed: %s", root["mcpOAuth"])
	}
	if string(bytes.TrimSpace(root["theme"])) != `"dark"` {
		t.Fatalf("theme = %s", root["theme"])
	}
	doc, err := ParseAuthDocument(out)
	if err != nil {
		t.Fatal(err)
	}
	if doc.OAuth().AccessToken != "test-access-token" || doc.OAuth().RefreshToken != "test-refresh-token" {
		t.Fatal("oauth was not replaced")
	}
	if !doc.ExpiresAt().Equal(time.UnixMilli(1700000000000)) {
		t.Fatal("expiry")
	}
	managed, err := ManagedDocument(official)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(managed, []byte("mcpOAuth")) || bytes.Contains(managed, []byte("mcp-token")) {
		t.Fatal("managed copy kept mcpOAuth")
	}
}

func TestApplyTokenRefreshKeepsSiblingKeys(t *testing.T) {
	doc := []byte(`{"mcpOAuth":{"server":{"token":"mcp-token"}},"claudeAiOauth":{"accessToken":"old","refreshToken":"old-refresh","scopes":["user:inference"],"subscriptionType":"max"}}`)
	out, err := ApplyTokenRefresh(doc, "test-access-token", "test-refresh-token", time.UnixMilli(1800000000000), true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`"mcpOAuth"`)) || !bytes.Contains(out, []byte("mcp-token")) {
		t.Fatal("sibling key dropped")
	}
	parsed, err := ParseAuthDocument(out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.OAuth().AccessToken != "test-access-token" || parsed.OAuth().RefreshToken != "test-refresh-token" {
		t.Fatal("tokens")
	}
	if len(parsed.OAuth().Scopes) != 1 || parsed.OAuth().Scopes[0] != "user:inference" {
		t.Fatal("scopes")
	}
	if _, err := ApplyTokenRefresh(doc, "only-access", "", time.Time{}, false); !errors.Is(err, ErrIncompleteRefresh) {
		t.Fatal(err)
	}
}

func TestManagedIndexOmitsTokensAndFileMode(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".claude")
	store := NewStoreAt(filepath.Join(home, "accounts"), config, home)
	body := []byte(`{"mcpOAuth":{"server":{"token":"mcp-token"}},"claudeAiOauth":{"accessToken":"test-access-token","refreshToken":"test-refresh-token","expiresAt":2000000000000,"subscriptionType":"max","rateLimitTier":"default_claude_max_20x"}}`)
	acc, err := store.Upsert(body, CredentialIdentity{
		AccountUUID: "acct-1", OrganizationUUID: "org-1", Email: "user@example.com", DisplayName: "User",
	}, UpsertOptions{MakeCurrent: true, Now: time.Unix(1_700_000_000, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if acc.Tier() != TierMax20x {
		t.Fatalf("tier %s", acc.Tier().DisplayName())
	}
	index, err := os.ReadFile(store.IndexPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(index)
	for _, forbidden := range []string{"accessToken", "refreshToken", "test-access-token", "test-refresh-token", "mcp-token"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("index contains %s", forbidden)
		}
	}
	cred, err := os.ReadFile(store.CredentialPath(acc.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(cred, []byte("test-access-token")) || bytes.Contains(cred, []byte("mcpOAuth")) {
		t.Fatal("managed credential shape")
	}
	for _, path := range []string{store.Root(), store.AccountDir(acc.ID)} {
		if err := platform.VerifyOwnerOnly(path, 0o700); err != nil {
			t.Fatal(path, err)
		}
	}
	for _, path := range []string{store.IndexPath(), store.CredentialPath(acc.ID)} {
		if err := platform.VerifyOwnerOnly(path, 0o600); err != nil {
			t.Fatal(path, err)
		}
	}
	pending, err := store.EnsurePendingHome("login")
	if err != nil {
		t.Fatal(err)
	}
	if err := platform.VerifyOwnerOnly(pending, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestKeychainStdinHasNoSecretAndReadback(t *testing.T) {
	payload := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token","refreshToken":"test-refresh-token"}}`)
	mem := &memSecurity{}
	store := KeychainStore{Service: "Claude Code-credentials", Account: "alice", Runner: mem}
	if err := store.Save(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	var stdin string
	for i, args := range mem.argSets {
		if mem.names[i] != "/usr/bin/security" {
			t.Fatal(mem.names[i])
		}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "test-access-token") || strings.Contains(joined, "test-refresh-token") {
			t.Fatal("secret in argv")
		}
		if len(args) == 1 && args[0] == "-i" {
			stdin = mem.stdins[i]
		}
		if d := mem.deadlines[i]; d <= time.Second || d > 5*time.Second {
			t.Fatalf("deadline %s", d)
		}
	}
	if stdin == "" || !strings.Contains(stdin, "add-generic-password -U ") || strings.Contains(stdin, "-T") {
		t.Fatalf("stdin %q", stdin)
	}
	if strings.Contains(stdin, "test-access-token") {
		t.Fatal("raw token on stdin")
	}
	wantHex := hex.EncodeToString(payload)
	if !strings.Contains(stdin, "-X "+wantHex) || !strings.Contains(stdin, `-s "Claude Code-credentials"`) || !strings.Contains(stdin, `-a "alice"`) {
		t.Fatalf("stdin %q", stdin)
	}
	if mem.password != string(payload) {
		t.Fatal("stored password")
	}
}

func TestKeychainOversizeExitAndRollback(t *testing.T) {
	mem := &memSecurity{}
	store := KeychainStore{Service: "Claude Code-credentials", Account: "alice", Runner: mem}
	big := []byte(`{"claudeAiOauth":{"accessToken":"` + strings.Repeat("a", securityLineLimit/2) + `"}}`)
	if err := store.Save(context.Background(), big); !errors.Is(err, ErrKeychainPayloadTooLarge) || len(mem.argSets) != 0 {
		t.Fatal(err)
	}
	fits := []byte(`{"claudeAiOauth":{"accessToken":"` + strings.Repeat("a", 1800) + `"}}`)
	line, err := securityAddLine("alice", "Claude Code-credentials", fits)
	if err != nil || len(line) > securityLineLimit+1 {
		t.Fatalf("line %d %v", len(line), err)
	}
	if _, err := store.Load(context.Background()); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatal(err)
	}
	original := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token","refreshToken":"old"}}`)
	mem.has = true
	mem.password = string(original)
	mem.finds = 0
	mem.corruptReadback = true
	next := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token-2","refreshToken":"test-refresh-token"}}`)
	if err := store.Save(context.Background(), next); !errors.Is(err, ErrKeychainVerify) {
		t.Fatal(err)
	}
	if mem.password != string(original) {
		t.Fatal("rollback did not restore the previous item")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestKeychainRefusesWhenPreviousCannotBeRestored(t *testing.T) {
	big := `{"claudeAiOauth":{"accessToken":"` + strings.Repeat("a", securityLineLimit/2) + `"}}`
	mem := &memSecurity{has: true, password: big}
	store := KeychainStore{Service: "Claude Code-credentials", Account: "alice", Runner: mem}
	next := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token-2","refreshToken":"test-refresh-token"}}`)
	if err := store.Save(context.Background(), next); !errors.Is(err, ErrKeychainPayloadTooLarge) {
		t.Fatal(err)
	}
	if mem.password != big || mem.writes != 0 {
		t.Fatal("an unrestorable item was overwritten")
	}
}

func TestKeychainRollbackDeletesNewItem(t *testing.T) {
	mem := &memSecurity{corruptReadback: true}
	store := KeychainStore{Service: "Claude Code-credentials", Account: "alice", Runner: mem}
	next := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token-2","refreshToken":"test-refresh-token"}}`)
	if err := store.Save(context.Background(), next); !errors.Is(err, ErrKeychainVerify) {
		t.Fatal(err)
	}
	if mem.has {
		t.Fatal("a failed first write left the item behind")
	}
}

func TestFallbackSaveNeverCreatesPlaintextFile(t *testing.T) {
	dir := t.TempDir()
	file := FileStore{Path: filepath.Join(dir, ".credentials.json")}
	mem := &memSecurity{}
	store := fallbackStore{primary: KeychainStore{Service: "Claude Code-credentials", Account: "alice", Runner: mem}, secondary: file}
	payload := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token","refreshToken":"test-refresh-token"}}`)
	if err := store.Save(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if ok, _ := file.Exists(context.Background()); ok || mem.password != string(payload) {
		t.Fatal("first save must go to the keychain")
	}
	mem.has, mem.password = false, ""
	if err := file.Save(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	next := []byte(`{"claudeAiOauth":{"accessToken":"test-access-token-2","refreshToken":"test-refresh-token"}}`)
	if err := store.Save(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if got, _ := file.Load(context.Background()); !bytes.Equal(got, next) || mem.has {
		t.Fatal("an existing file login must stay in the file")
	}
}

func TestKeychainServiceAndPaths(t *testing.T) {
	home := t.TempDir()
	if KeychainServiceName("", home) != "Claude Code-credentials" {
		t.Fatal("default service")
	}
	if KeychainServiceName("   ", home) != "Claude Code-credentials" {
		t.Fatal("blank service")
	}
	def := filepath.Join(home, ".claude")
	got := KeychainServiceName(def, home)
	sum := sha256Hex(ResolveConfigDir(def, home))
	if got != "Claude Code-credentials-"+sum[:8] {
		t.Fatalf("service %s", got)
	}
	if KeychainAccountName("alice", "bob") != "alice" || KeychainAccountName("", "bob") != "bob" || KeychainAccountName(" ", "") != "USER" {
		t.Fatal("account")
	}
	if ResolveConfigDir("", home) != def {
		t.Fatal("default config")
	}
	if ResolveConfigDir("~/work", home) != filepath.Join(home, "work") {
		t.Fatal("tilde")
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type memSecurity struct {
	password        string
	has             bool
	corruptReadback bool
	finds           int
	writes          int
	stdins          []string
	argSets         [][]string
	names           []string
	deadlines       []time.Duration
}

func (m *memSecurity) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if dl, ok := ctx.Deadline(); ok {
		m.deadlines = append(m.deadlines, time.Until(dl))
	}
	m.names = append(m.names, name)
	m.argSets = append(m.argSets, append([]string(nil), args...))
	m.stdins = append(m.stdins, string(stdin))
	if len(args) == 1 && args[0] == "-i" {
		raw, err := hex.DecodeString(hexField(string(stdin)))
		if err != nil {
			return nil, 1, nil
		}
		m.password = string(raw)
		m.has = true
		m.writes++
		return nil, 0, nil
	}
	if len(args) > 0 && args[0] == "delete-generic-password" {
		if !m.has {
			return nil, 44, nil
		}
		m.has, m.password = false, ""
		return nil, 0, nil
	}
	m.finds++
	if !m.has {
		return nil, 44, nil
	}
	if m.corruptReadback && m.finds == 2 {
		return []byte("{\"claudeAiOauth\":{\"accessToken\":\"wrong\",\"refreshToken\":\"wrong\"}}\n"), 0, nil
	}
	return []byte(m.password + "\n"), 0, nil
}

func hexField(line string) string {
	_, after, ok := strings.Cut(line, "-X ")
	if !ok {
		return ""
	}
	return strings.Fields(after)[0]
}
