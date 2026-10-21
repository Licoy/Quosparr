package claude

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

// settingsSnapshot retains original bytes only in memory for a failed switch.
type settingsSnapshot struct {
	path              string
	original, applied []byte
	mode              os.FileMode
	existed           bool
}

func (s *Store) editAPISettings(now time.Time, edit func([]byte) ([]byte, error)) (settingsSnapshot, error) {
	snap := settingsSnapshot{path: s.SettingsPath(), mode: storeFilePerm}
	if resolved, err := filepath.EvalSymlinks(snap.path); err == nil {
		snap.path = resolved
	}
	info, err := os.Stat(snap.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return snap, err
	}
	if err == nil {
		snap.existed, snap.mode = true, info.Mode().Perm()
	}
	err = platform.EditFileWithBackup(snap.path, s.settingsBackup(), now, func(old []byte) ([]byte, error) {
		snap.original = append([]byte(nil), old...)
		out, err := edit(old)
		snap.applied = append([]byte(nil), out...)
		return out, err
	})
	return snap, err
}

func (snap settingsSnapshot) restore() error {
	current, err := os.ReadFile(snap.path)
	if errors.Is(err, os.ErrNotExist) && !snap.existed {
		return nil
	}
	if err != nil {
		return fmt.Errorf("claude: settings rollback read failed: %w", err)
	}
	if !bytes.Equal(current, snap.applied) {
		return errors.New("claude: settings changed during rollback")
	}
	if !snap.existed {
		return os.Remove(snap.path)
	}
	if err := platform.WriteFileAtomic(snap.path, snap.original, snap.mode); err != nil {
		return fmt.Errorf("claude: settings rollback failed: %w", err)
	}
	return nil
}

// applyAPIAccount commits settings, the applied model record and optionally the
// current account in one Store critical section and one index replacement.
func (s *Store) applyAPIAccount(acc ManagedAccount, key string, env APIEnv, previous *APIEnv, now time.Time, makeCurrent bool) (ManagedAccount, error) {
	env = trimmedAPIEnv(env)
	identity, err := appliedAPIIdentity(env.BaseURL, env.AuthToken)
	if err != nil {
		return ManagedAccount{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, err := s.editAPISettings(now, func(old []byte) ([]byte, error) { return ApplyAPISettings(old, env, previous) })
	if err != nil {
		return ManagedAccount{}, err
	}
	acc.AppliedAPIModels = appliedAPIModels(env)
	acc.AppliedAPIIdentity = identity
	saved, err := s.putAPI(acc, key, now, makeCurrent)
	if err != nil {
		return ManagedAccount{}, errors.Join(err, snap.restore())
	}
	return saved, nil
}

func (s *Store) removeAPIForSwitch(env APIEnv, now time.Time) (settingsSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.editAPISettings(now, func(old []byte) ([]byte, error) { out, _, err := RemoveAPISettings(old, env); return out, err })
}

func (s *Store) restoreAPISettings(snap settingsSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return snap.restore()
}
