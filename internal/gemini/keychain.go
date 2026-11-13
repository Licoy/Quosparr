package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Official keychain item.
const (
	KeychainService = "gemini"
	KeychainAccount = "antigravity"

	keyringBase64Prefix = "go-keyring-base64:"
	keyringHexPrefix    = "go-keyring-encoded:"
	securityPath        = "/usr/bin/security"
	// securityExitNotFound is security's "item not found" status.
	securityExitNotFound = 44
	// securityLineLimit is the longest stdin line security -i reads in one
	// piece, without the newline.
	securityLineLimit = 4095
	securityTimeout   = 5 * time.Second
)

// Keychain holds the official credential JSON. Load returns
// ErrOfficialCredentialMissing when there is no item. Implementations do the
// platform encoding; callers see the plain JSON.
type Keychain interface {
	Load(ctx context.Context) ([]byte, error)
	Save(ctx context.Context, data []byte) error
	Remove(ctx context.Context) error
}

// CommandRunner runs /usr/bin/security. stdout carries the secret; stderr is
// discarded by the real runner. Tests supply a fake.
type CommandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) (stdout []byte, exitCode int, err error)
}

// SecurityKeychain is the macOS generic-password item read and written with
// /usr/bin/security. The value is "go-keyring-base64:" + base64(JSON), the
// encoding the Antigravity clients write. The secret only travels on stdin.
type SecurityKeychain struct {
	Service string
	Account string
	// Runner nil uses /usr/bin/security (macOS only).
	Runner CommandRunner
	// Timeout bounds each invocation; zero means 5 seconds.
	Timeout time.Duration
}

func (k SecurityKeychain) runner() CommandRunner {
	if k.Runner != nil {
		return k.Runner
	}
	return newSecurityRunner()
}

func (k SecurityKeychain) run(ctx context.Context, args []string, stdin []byte) ([]byte, int, error) {
	timeout := k.Timeout
	if timeout <= 0 {
		timeout = securityTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, code, err := k.runner().Run(ctx, securityPath, args, stdin)
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}
	return out, code, err
}

// Load runs `find-generic-password -s <service> -a <account> -w`.
func (k SecurityKeychain) Load(ctx context.Context) ([]byte, error) {
	out, code, err := k.run(ctx, []string{"find-generic-password", "-s", k.Service, "-a", k.Account, "-w"}, nil)
	switch {
	case err != nil:
		return nil, errors.Join(ErrKeychainUnavailable, err)
	case code == securityExitNotFound:
		return nil, ErrOfficialCredentialMissing
	case code != 0:
		return nil, ErrKeychainUnavailable
	}
	data, err := decodeKeychainValue(out)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Save writes one stdin line to `security -i`:
// add-generic-password -U -s <service> -a <account> -w "go-keyring-base64:…".
// -U updates the existing item and keeps its ACL. security -i exits 0 even
// when the command fails, so callers read the item back.
func (k SecurityKeychain) Save(ctx context.Context, data []byte) error {
	line, err := securityAddLine(k.Service, k.Account, data)
	if err != nil {
		return err
	}
	_, code, err := k.run(ctx, []string{"-i"}, []byte(line))
	if err != nil {
		return errors.Join(ErrKeychainUnavailable, err)
	}
	if code != 0 {
		return ErrKeychainUnavailable
	}
	return nil
}

// Remove runs `delete-generic-password`; a missing item is not an error.
func (k SecurityKeychain) Remove(ctx context.Context) error {
	_, code, err := k.run(ctx, []string{"delete-generic-password", "-s", k.Service, "-a", k.Account}, nil)
	if err != nil {
		return errors.Join(ErrKeychainUnavailable, err)
	}
	if code != 0 && code != securityExitNotFound {
		return ErrKeychainUnavailable
	}
	return nil
}

// securityAddLine is the stdin command for security -i. The value is base64
// with a fixed prefix, so quoting only has to protect service and account.
func securityAddLine(service, account string, data []byte) (string, error) {
	if !validSecurityArg(service) || !validSecurityArg(account) {
		return "", ErrKeychainUnavailable
	}
	value := keyringBase64Prefix + base64.StdEncoding.EncodeToString(data)
	line := "add-generic-password -U -s " + quoteSecurityArg(service) +
		" -a " + quoteSecurityArg(account) + " -w " + quoteSecurityArg(value)
	if len(line) > securityLineLimit {
		return "", ErrKeychainPayloadTooLarge
	}
	return line + "\n", nil
}

func validSecurityArg(s string) bool {
	return trimSpace(s) != "" && !strings.ContainsAny(s, "\r\n")
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

// decodeKeychainValue turns a stored value into the credential JSON. It
// accepts the go-keyring base64 and hex prefixes, raw JSON, and the hex
// dump security prints for values it does not consider text.
func decodeKeychainValue(raw []byte) ([]byte, error) {
	text := trimSpace(string(raw))
	if text == "" {
		return nil, ErrOfficialCredentialMalformed
	}
	if isHexString(text) {
		if decoded, err := hex.DecodeString(text); err == nil && len(decoded) > 0 {
			text = trimSpace(string(decoded))
		}
	}
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(text, keyringBase64Prefix):
		data, err = base64.StdEncoding.DecodeString(text[len(keyringBase64Prefix):])
	case strings.HasPrefix(text, keyringHexPrefix):
		data, err = hex.DecodeString(text[len(keyringHexPrefix):])
	default:
		data = []byte(text)
	}
	if err != nil || !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return nil, ErrOfficialCredentialMalformed
	}
	return bytes.TrimSpace(data), nil
}

func isHexString(s string) bool {
	if len(s)%2 != 0 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
