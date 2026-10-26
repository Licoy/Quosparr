package grok

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/licoy/quosparr/internal/platform"
)

// Remove deletes account id: its credential directory first moves aside, the
// index is saved without the row, and only then is the moved directory
// deleted. A failure at any step puts everything back (verified); when that is
// not possible the error is a *PartialWriteError. The current account and the
// official auth.json are never touched.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	original, err := s.loadIndex()
	if err != nil {
		return err
	}
	if _, ok := original.Account(id); !ok {
		return &AccountNotFoundError{ID: id}
	}
	if original.CurrentAccountID == id {
		return &CurrentAccountCannotBeRemovedError{ID: id}
	}
	next := original
	next.Accounts = make([]ManagedAccount, 0, len(original.Accounts))
	for _, a := range original.Accounts {
		if a.ID != id {
			next.Accounts = append(next.Accounts, a)
		}
	}
	dir := s.AccountDir(id)
	snapshot, hadCredential, err := s.credentialSnapshot(id)
	if err != nil {
		return err
	}
	tombstone := filepath.Join(s.root, ".grok-remove-"+randomToken())
	moved := false

	err = func() error {
		if _, statErr := os.Stat(dir); statErr == nil {
			if err := os.Rename(dir, tombstone); err != nil {
				return err
			}
			moved = true
		}
		if err := s.saveIndex(next); err != nil {
			return err
		}
		if moved {
			return os.RemoveAll(tombstone)
		}
		return nil
	}()
	if err == nil {
		return nil
	}
	if rbErr := s.rollbackRemoval(id, original, dir, tombstone, moved, snapshot, hadCredential); rbErr != nil {
		return &PartialWriteError{Message: "The Grok account removal could not be rolled back safely."}
	}
	return &AccountIOError{Message: "Unable to remove the Grok account.", Cause: err}
}

func (s *Store) credentialSnapshot(id string) (data []byte, present bool, err error) {
	data, err = os.ReadFile(s.CredentialPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (s *Store) rollbackRemoval(
	id string, original AccountIndex, dir, tombstone string, moved bool, snapshot []byte, hadCredential bool,
) error {
	if _, err := os.Stat(tombstone); moved && err == nil {
		if _, statErr := os.Stat(dir); statErr == nil {
			return &PartialWriteError{Message: "The Grok credential directory already exists."}
		}
		if err := os.Rename(tombstone, dir); err != nil {
			return err
		}
	}
	if err := s.saveIndex(original); err != nil {
		return err
	}
	if hadCredential {
		if err := s.restoreCredentialSnapshot(id, snapshot); err != nil {
			return err
		}
	} else if _, err := os.Stat(dir); moved && err != nil {
		return &PartialWriteError{Message: "The Grok credential directory rollback could not be verified."}
	}
	reloaded, err := s.loadIndex()
	if err != nil || !indexesEqual(reloaded, original) {
		return &PartialWriteError{Message: "The Grok account index rollback could not be verified."}
	}
	return nil
}

// restoreCredentialSnapshot makes sure the credential file holds data with
// mode 0600 again, and verifies it.
func (s *Store) restoreCredentialSnapshot(id string, data []byte) error {
	path := s.CredentialPath(id)
	current, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(current, data) || platform.VerifyOwnerOnly(path, storeFilePerm) != nil {
		if err := s.saveCredentialData(id, data); err != nil {
			return err
		}
	}
	restored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(restored, data) || platform.VerifyOwnerOnly(path, storeFilePerm) != nil {
		return &PartialWriteError{Message: "The Grok credential rollback could not be verified."}
	}
	return nil
}

func randomToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(b[:])
}
