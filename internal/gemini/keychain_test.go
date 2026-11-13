package gemini

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func TestSecurityAddLineCarriesBase64OnStdin(t *testing.T) {
	data := []byte(`{"token":{"access_token":"a","refresh_token":"r"}}`)
	line, err := securityAddLine(KeychainService, KeychainAccount, data)
	if err != nil {
		t.Fatal(err)
	}
	prefix := `add-generic-password -U -s "gemini" -a "antigravity" -w "go-keyring-base64:`
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "\"\n") {
		t.Fatalf("line shape: %q", line[:len(prefix)])
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(line, prefix), "\"\n")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != string(data) {
		t.Fatal("payload does not round-trip")
	}
	if _, err := securityAddLine(KeychainService, KeychainAccount, make([]byte, 4000)); !errors.Is(err, ErrKeychainPayloadTooLarge) {
		t.Fatalf("large payload: %v", err)
	}
	if _, err := securityAddLine("svc\n", KeychainAccount, data); err == nil {
		t.Fatal("newline in service must be refused")
	}
}

func TestDecodeKeychainValueForms(t *testing.T) {
	jsonText := `{"token":{"access_token":"a"}}`
	b64 := keyringBase64Prefix + base64.StdEncoding.EncodeToString([]byte(jsonText))
	for name, raw := range map[string]string{
		"base64":    b64 + "\n",
		"hexPrefix": keyringHexPrefix + hex.EncodeToString([]byte(jsonText)),
		"raw":       jsonText,
		"hexDump":   hex.EncodeToString([]byte(b64)),
	} {
		got, err := decodeKeychainValue([]byte(raw))
		if err != nil || string(got) != jsonText {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := decodeKeychainValue([]byte("plain text")); !errors.Is(err, ErrOfficialCredentialMalformed) {
		t.Fatalf("non-JSON: %v", err)
	}
}

type fakeRunner struct {
	calls  [][]string
	stdins [][]byte
	stored string
	code   int
}

func (r *fakeRunner) Run(_ context.Context, name string, args []string, stdin []byte) ([]byte, int, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	r.stdins = append(r.stdins, stdin)
	switch args[0] {
	case "find-generic-password":
		if r.stored == "" {
			return nil, securityExitNotFound, nil
		}
		return []byte(r.stored + "\n"), 0, nil
	case "-i":
		line := strings.TrimSpace(string(stdin))
		i := strings.Index(line, `-w "`)
		r.stored = strings.TrimSuffix(line[i+4:], `"`)
		return nil, r.code, nil
	case "delete-generic-password":
		r.stored = ""
	}
	return nil, 0, nil
}

func TestSecurityKeychainRoundTripKeepsSecretOffArgv(t *testing.T) {
	runner := &fakeRunner{}
	k := SecurityKeychain{Service: KeychainService, Account: KeychainAccount, Runner: runner}
	if _, err := k.Load(context.Background()); !errors.Is(err, ErrOfficialCredentialMissing) {
		t.Fatalf("missing item: %v", err)
	}
	data := []byte(`{"token":{"access_token":"secret-access","refresh_token":"secret-refresh"}}`)
	if err := k.Save(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	got, err := k.Load(context.Background())
	if err != nil || string(got) != string(data) {
		t.Fatalf("round trip: %v", err)
	}
	for _, call := range runner.calls {
		if call[0] != securityPath {
			t.Fatalf("not an absolute security path: %q", call[0])
		}
		for _, arg := range call {
			if strings.Contains(arg, "secret") || strings.Contains(arg, keyringBase64Prefix) {
				t.Fatal("secret material on argv")
			}
		}
	}
	if !strings.HasPrefix(runner.stored, keyringBase64Prefix) {
		t.Fatal("stored value must use the go-keyring base64 prefix")
	}
	if err := k.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
}
