package claude

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/licoy/quosparr/internal/platform"
)

// switchValue contains bytes only in memory, never in logs or metadata.
type switchValue struct {
	path             string
	old, next        []byte
	mode             os.FileMode
	existed, written bool
}

func fileSwitchValue(path string) (*switchValue, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	data, mode, had, err := readOptional(path)
	if err != nil {
		return nil, err
	}
	if !had {
		mode = storeFilePerm
	}
	return &switchValue{path: path, old: data, next: data, mode: mode, existed: had}, nil
}

func (f *switchValue) check(applied bool) error {
	data, _, had, err := readOptional(f.path)
	if err != nil {
		return err
	}
	expected, exists := f.old, f.existed
	if applied {
		expected, exists = f.next, true
	}
	if had != exists || !bytes.Equal(data, expected) {
		return fmt.Errorf("claude: coordinated switch file changed")
	}
	return nil
}

func (f *switchValue) restore() error {
	if f.existed {
		return platform.WriteFileAtomic(f.path, f.old, f.mode)
	}
	if err := os.Remove(f.path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
