package claude

import (
	"context"
	"errors"
	"os"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	storeDirPerm  = 0o700
	storeFilePerm = 0o600
)

// FileStore is <config>/.credentials.json. Writes are atomic, mode 0600,
// and the parent directory is 0700. On Windows, platform.WriteFileAtomic
// applies the protected owner-only ACL and disables inheritance.
type FileStore struct {
	Path string
}

// Load reads the credentials file. A missing file is ErrNotLoggedIn.
func (f FileStore) Load(context.Context) ([]byte, error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotLoggedIn
	}
	if err != nil {
		return nil, operationError(StageReadOfficial, err)
	}
	return data, nil
}

// Save writes data atomically with owner-only permissions.
func (f FileStore) Save(_ context.Context, data []byte) error {
	return operationError(StageWriteCredential, platform.WriteFileAtomic(f.Path, data, storeFilePerm))
}

// Exists reports whether the file is present.
func (f FileStore) Exists(context.Context) (bool, error) {
	_, err := os.Stat(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, operationError(StageReadOfficial, err)
	}
	return true, nil
}

// Remove deletes the credentials file. A missing file is not an error.
func (f FileStore) Remove(context.Context) error {
	err := os.Remove(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return operationError(StageWriteCredential, err)
}
