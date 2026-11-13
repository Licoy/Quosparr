//go:build !darwin

package gemini

import (
	"context"
	"errors"

	"github.com/zalando/go-keyring"
)

// SystemKeychain is the official item in the Windows Credential Manager
// (target "gemini:antigravity") or the Linux Secret Service, read and
// written through go-keyring like the Antigravity clients do.
func SystemKeychain() Keychain {
	return KeyringKeychain{Service: KeychainService, Account: KeychainAccount}
}

func newSecurityRunner() CommandRunner { return unsupportedRunner{} }

type unsupportedRunner struct{}

func (unsupportedRunner) Run(context.Context, string, []string, []byte) ([]byte, int, error) {
	return nil, 1, ErrKeychainUnavailable
}

// KeyringKeychain stores the raw JSON through go-keyring.
type KeyringKeychain struct{ Service, Account string }

func (k KeyringKeychain) Load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, err := keyring.Get(k.Service, k.Account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrOfficialCredentialMissing
	}
	if err != nil {
		return nil, errors.Join(ErrKeychainUnavailable, err)
	}
	return decodeKeychainValue([]byte(v))
}

func (k KeyringKeychain) Save(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := keyring.Set(k.Service, k.Account, string(data)); err != nil {
		return errors.Join(ErrKeychainUnavailable, err)
	}
	return nil
}

func (k KeyringKeychain) Remove(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := keyring.Delete(k.Service, k.Account)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errors.Join(ErrKeychainUnavailable, err)
	}
	return nil
}
