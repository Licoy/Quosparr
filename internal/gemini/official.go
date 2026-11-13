package gemini

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/licoy/quosparr/internal/platform"
)

// Official mirror files under the user's home.
const (
	officialDirName     = ".gemini"
	officialTokenFile   = "jetski-standalone-oauth-token"
	officialCLIDirName  = "antigravity-cli"
	officialCLITokenRel = "antigravity-oauth-token"
	officialFilePerm    = 0o600
)

// OfficialFiles are the mirror files of home, in read order.
func OfficialFiles(home string) []string {
	dir := filepath.Join(home, officialDirName)
	return []string{
		filepath.Join(dir, officialTokenFile),
		filepath.Join(dir, officialCLIDirName, officialCLITokenRel),
	}
}

// OfficialCredentials is where the Antigravity clients keep their login:
// the keychain item plus mirror files. Both are injectable.
type OfficialCredentials struct {
	// Keychain nil uses SystemKeychain().
	Keychain Keychain
	// Files are read in order after the keychain and written only when
	// they already exist.
	Files []string
}

// DefaultOfficialCredentials is the system keychain plus the files of home.
func DefaultOfficialCredentials(home string) *OfficialCredentials {
	return &OfficialCredentials{Keychain: SystemKeychain(), Files: OfficialFiles(home)}
}

func (o *OfficialCredentials) keychain() Keychain {
	if o.Keychain != nil {
		return o.Keychain
	}
	return SystemKeychain()
}

// Load returns the official credential JSON: the keychain item, else the
// first mirror file that exists. When the keychain fails and no file exists,
// the keychain error is returned.
func (o *OfficialCredentials) Load(ctx context.Context) ([]byte, error) {
	data, keyErr := o.keychain().Load(ctx)
	if keyErr == nil {
		return data, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	for _, path := range o.Files {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, &AccountIOError{Message: "Unable to read the Antigravity credential file.", Cause: err}
		}
		return bytes.TrimSpace(data), nil
	}
	return nil, keyErr
}

// officialState is the previous content of every official target.
type officialState struct {
	keychain    []byte
	hadKeychain bool
	files       []fileState
}

type fileState struct {
	path    string
	data    []byte
	existed bool
}

func (o *OfficialCredentials) capture(ctx context.Context) (officialState, error) {
	var st officialState
	data, err := o.keychain().Load(ctx)
	switch {
	case err == nil:
		st.keychain, st.hadKeychain = data, true
	case errors.Is(err, ErrOfficialCredentialMissing):
	case errors.Is(err, ErrOfficialCredentialMalformed):
		// An unreadable item is overwritten; it cannot be put back as is.
		st.hadKeychain = false
	default:
		return officialState{}, err
	}
	for _, path := range o.Files {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			st.files = append(st.files, fileState{path: path, data: data, existed: true})
		case errors.Is(err, os.ErrNotExist):
			st.files = append(st.files, fileState{path: path})
		default:
			return officialState{}, &AccountIOError{Message: "Unable to read the Antigravity credential file.", Cause: err}
		}
	}
	return st, nil
}

// Install writes data to the keychain and to the mirror files that exist,
// and reads each back. On failure everything is restored; a failed restore
// is a *PartialWriteError.
func (o *OfficialCredentials) Install(ctx context.Context, data []byte) error {
	if _, err := ParseCredential(data); err != nil {
		return err
	}
	st, err := o.capture(ctx)
	if err != nil {
		return err
	}
	return o.installOver(ctx, data, st)
}

func (o *OfficialCredentials) installOver(ctx context.Context, data []byte, st officialState) error {
	err := o.write(ctx, data, st)
	if err == nil {
		return nil
	}
	if rbErr := o.restore(context.WithoutCancel(ctx), st); rbErr != nil {
		return &PartialWriteError{Message: "The Antigravity credential could not be restored.", Cause: errors.Join(err, rbErr)}
	}
	return err
}

func (o *OfficialCredentials) write(ctx context.Context, data []byte, st officialState) error {
	kc := o.keychain()
	if err := kc.Save(ctx, data); err != nil {
		return err
	}
	got, err := kc.Load(ctx)
	if err != nil {
		return errors.Join(ErrKeychainVerify, err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(data)) {
		return ErrKeychainVerify
	}
	for _, f := range st.files {
		if !f.existed {
			continue
		}
		if err := writeOfficialFile(f.path, data); err != nil {
			return err
		}
	}
	return nil
}

// restore puts every target back and verifies it.
func (o *OfficialCredentials) restore(ctx context.Context, st officialState) error {
	kc := o.keychain()
	var errs []error
	if st.hadKeychain {
		if err := kc.Save(ctx, st.keychain); err != nil {
			errs = append(errs, err)
		} else if got, err := kc.Load(ctx); err != nil || !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(st.keychain)) {
			errs = append(errs, errors.Join(ErrKeychainVerify, err))
		}
	} else if err := kc.Remove(ctx); err != nil {
		errs = append(errs, err)
	} else if _, err := kc.Load(ctx); !errors.Is(err, ErrOfficialCredentialMissing) {
		errs = append(errs, ErrKeychainVerify)
	}
	for _, f := range st.files {
		if !f.existed {
			continue
		}
		if current, err := os.ReadFile(f.path); err == nil && bytes.Equal(current, f.data) {
			continue
		}
		if err := platform.WriteFileAtomic(f.path, f.data, officialFilePerm); err != nil {
			errs = append(errs, err)
			continue
		}
		if current, err := os.ReadFile(f.path); err != nil || !bytes.Equal(current, f.data) {
			errs = append(errs, errors.New("gemini: credential file rollback mismatch"))
		}
	}
	return errors.Join(errs...)
}

func writeOfficialFile(path string, data []byte) error {
	if err := platform.WriteFileAtomic(path, data, officialFilePerm); err != nil {
		return &AccountIOError{Message: "Unable to write the Antigravity credential file.", Cause: err}
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		return &AccountIOError{Message: "Antigravity credential file verification failed.", Cause: err}
	}
	return nil
}
