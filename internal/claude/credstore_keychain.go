package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// CommandRunner runs /usr/bin/security. Tests supply a fake. The real runner
// is macOS-only. stdout is the password; the exit code distinguishes a missing
// item. stderr is discarded by the real runner so it cannot leak into logs.
type CommandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) (stdout []byte, exitCode int, err error)
}

// KeychainStore is the macOS generic-password item Claude Code uses.
// Service and Account are injectable. A nil Runner uses /usr/bin/security.
type KeychainStore struct {
	Service string
	Account string
	Runner  CommandRunner
}

const (
	securityPath    = "/usr/bin/security"
	keychainService = "Claude Code-credentials"
	// securityExitNotFound is security's "item not found" status.
	securityExitNotFound = 44
	// securityTimeout bounds one /usr/bin/security invocation.
	securityTimeout = 5 * time.Second
)

// KeychainServiceName is the generic-password service.
// An unset or empty CLAUDE_CONFIG_DIR uses "Claude Code-credentials".
// A set value uses that name plus "-" and the first 8 hex characters of the
// SHA-256 of the cleaned absolute config directory, even when that directory
// is the default ~/.claude. The branch is whether the variable is set.
func KeychainServiceName(envValue, home string) string {
	if strings.TrimSpace(envValue) == "" {
		return keychainService
	}
	sum := sha256.Sum256([]byte(ResolveConfigDir(envValue, home)))
	return keychainService + "-" + hex.EncodeToString(sum[:])[:8]
}

// KeychainAccountName is the keychain account. explicit wins. Otherwise
// userEnv (normally the USER variable) is used. Both empty yields "USER".
func KeychainAccountName(explicit, userEnv string) string {
	if t := strings.TrimSpace(explicit); t != "" {
		return t
	}
	if t := strings.TrimSpace(userEnv); t != "" {
		return t
	}
	return "USER"
}

func (k KeychainStore) runner() CommandRunner {
	if k.Runner != nil {
		return k.Runner
	}
	return newSecurityRunner()
}

func (k KeychainStore) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, securityTimeout)
}

// Load runs `find-generic-password -s <service> -a <account> -w`.
// Exit 44 is ErrNotLoggedIn. The password is not passed in argv.
func (k KeychainStore) Load(ctx context.Context) ([]byte, error) {
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()
	args := []string{"find-generic-password", "-s", k.Service, "-a", k.Account, "-w"}
	stdout, code, err := k.runner().Run(ctx, securityPath, args, nil)
	if ctx.Err() != nil {
		return nil, operationError(StageReadKeychain, ctx.Err())
	}
	if err != nil {
		return nil, operationError(StageReadKeychain, err)
	}
	if code == securityExitNotFound {
		return nil, ErrNotLoggedIn
	}
	if code != 0 {
		return nil, &OperationError{Stage: StageReadKeychain, Kind: KindCommand, ExitCode: code, cause: ErrCredentialStore}
	}
	data, err := decodeKeychainStdout(stdout)
	if err == nil {
		_, err = rawObject(data)
	}
	if err != nil {
		return nil, classifiedError(StageReadKeychain, KindInvalidData, err)
	}
	return data, nil
}

// Exists reports whether Load finds an item. ErrNotLoggedIn is false, nil.
func (k KeychainStore) Exists(ctx context.Context) (bool, error) {
	_, err := k.Load(ctx)
	if errors.Is(err, ErrNotLoggedIn) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Save writes the JSON with `security -i` and one stdin line
// `add-generic-password -U -a <account> -s <service> -X <hex>`.
// -U updates an existing item. -T is not passed, so the item ACL is unchanged.
// The secret is only in the hex on stdin. security -i exits 0 even when a
// command fails, so the item is always read back. A line that would not fit
// the 4096-byte buffer is refused before anything is written, and so is a
// write whose previous item could not be put back the same way.
func (k KeychainStore) Save(ctx context.Context, data []byte) error {
	if _, err := oauthObject(data); err != nil {
		return err
	}
	if _, err := securityAddLine(k.Account, k.Service, data); err != nil {
		return err
	}
	previous, had, err := k.previous(ctx)
	if err != nil {
		return err
	}
	if had {
		if _, err := securityAddLine(k.Account, k.Service, previous); err != nil {
			return err
		}
	}
	if err := k.add(ctx, data); err != nil {
		rbErr := k.rollback(ctx, previous, had)
		if rbErr != nil {
			return rollbackOperationError(StageRollbackKeychain, err, rbErr)
		}
		return err
	}
	got, err := k.Load(ctx)
	if err == nil && sameKeychainBytes(data, got) {
		return nil
	}
	if rbErr := k.rollback(ctx, previous, had); rbErr != nil {
		return rollbackOperationError(StageRollbackKeychain, err, rbErr)
	}
	if err != nil {
		return operationError(StageWriteKeychain, err)
	}
	return classifiedError(StageWriteKeychain, KindInvalidData, ErrKeychainVerify)
}

// Remove runs `delete-generic-password -s <service> -a <account>`. A missing
// item is not an error. No secret is on argv.
func (k KeychainStore) Remove(ctx context.Context) error {
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()
	args := []string{"delete-generic-password", "-s", k.Service, "-a", k.Account}
	_, code, err := k.runner().Run(ctx, securityPath, args, nil)
	if ctx.Err() != nil {
		return operationError(StageRollbackKeychain, ctx.Err())
	}
	if err != nil {
		return operationError(StageRollbackKeychain, err)
	}
	if code != 0 && code != securityExitNotFound {
		return &OperationError{Stage: StageRollbackKeychain, Kind: KindCommand, ExitCode: code, cause: ErrCredentialStore}
	}
	return nil
}

func (k KeychainStore) previous(ctx context.Context) ([]byte, bool, error) {
	data, err := k.Load(ctx)
	if errors.Is(err, ErrNotLoggedIn) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// rollback puts the previous item back, or deletes the new one when there
// was none, and checks the result.
func (k KeychainStore) rollback(ctx context.Context, previous []byte, had bool) error {
	if !had {
		if err := k.Remove(ctx); err != nil {
			return err
		}
		return nil
	}
	if err := k.add(ctx, previous); err != nil {
		return err
	}
	got, err := k.Load(ctx)
	if err != nil {
		return operationError(StageRollbackKeychain, errors.Join(ErrKeychainVerify, err))
	}
	if !sameKeychainBytes(previous, got) {
		return classifiedError(StageRollbackKeychain, KindInvalidData, ErrKeychainVerify)
	}
	return nil
}

func (k KeychainStore) add(ctx context.Context, data []byte) error {
	line, err := securityAddLine(k.Account, k.Service, data)
	if err != nil {
		return err
	}
	ctx, cancel := k.withTimeout(ctx)
	defer cancel()
	_, code, err := k.runner().Run(ctx, securityPath, []string{"-i"}, []byte(line))
	if ctx.Err() != nil {
		return operationError(StageWriteKeychain, ctx.Err())
	}
	if err != nil {
		return operationError(StageWriteKeychain, err)
	}
	if code != 0 {
		return &OperationError{Stage: StageWriteKeychain, Kind: KindCommand, ExitCode: code, cause: ErrCredentialStore}
	}
	return nil
}

func sameKeychainBytes(want, got []byte) bool {
	return len(bytes.TrimSpace(want)) > 0 && bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(got))
}

// securityAddLine is the single stdin command. Account and service are quoted
// because the service name contains a space. The payload is hex, never raw.
func securityAddLine(account, service string, payload []byte) (string, error) {
	if err := validateSecurityArg(account); err != nil {
		return "", err
	}
	if err := validateSecurityArg(service); err != nil {
		return "", err
	}
	line := "add-generic-password -U -a " + quoteSecurityArg(account) +
		" -s " + quoteSecurityArg(service) +
		" -X " + hex.EncodeToString(payload)
	if len(line) > securityLineLimit {
		return "", ErrKeychainPayloadTooLarge
	}
	return line + "\n", nil
}

func validateSecurityArg(s string) error {
	if strings.TrimSpace(s) == "" || strings.ContainsAny(s, "\r\n") {
		return ErrCredentialStore
	}
	return nil
}

func quoteSecurityArg(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '\\' || r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func decodeKeychainStdout(stdout []byte) ([]byte, error) {
	text := strings.TrimRight(string(stdout), "\r\n")
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrCredentialStore
	}
	if strings.HasPrefix(text, "{") {
		return []byte(text), nil
	}
	if decoded, ok := decodeHexJSON(text); ok {
		return decoded, nil
	}
	return []byte(text), nil
}

func decodeHexJSON(text string) ([]byte, bool) {
	if len(text)%2 != 0 || !isHex(text) {
		return nil, false
	}
	raw, err := hex.DecodeString(text)
	if err != nil || len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	return raw, true
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
