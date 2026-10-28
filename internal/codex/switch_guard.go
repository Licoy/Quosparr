package codex

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
)

var ErrSwitchFilesChanged = errors.New("codex: login files changed during switch preparation")

type switchFileGuard struct {
	path string
	info os.FileInfo
	data []byte
}

func captureSwitchFiles(paths ...string) ([]switchFileGuard, error) {
	out := make([]switchFileGuard, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			out = append(out, switchFileGuard{path: path})
			continue
		}
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, switchFileGuard{path: path, info: info, data: data})
	}
	return out, nil
}

func verifySwitchFiles(files []switchFileGuard) error {
	for _, f := range files {
		info, err := os.Lstat(f.path)
		if f.info == nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return ErrSwitchFilesChanged
		}
		if err != nil || !os.SameFile(f.info, info) || f.info.Mode() != info.Mode() || f.info.Size() != info.Size() || !f.info.ModTime().Equal(info.ModTime()) {
			return ErrSwitchFilesChanged
		}
		data, err := os.ReadFile(f.path)
		if err != nil || !bytes.Equal(f.data, data) {
			return ErrSwitchFilesChanged
		}
	}
	return nil
}

// switchRowGuard excludes presentation metadata and live quota caches.
type switchRowGuard struct {
	id, account, email, user, subject, baseURL, model string
	mode                                              AuthMode
}

func guardSwitchRow(row ManagedAccount) switchRowGuard {
	return switchRowGuard{row.ID, row.AccountID, row.Email, row.UserID, row.SubjectID, row.APIBaseURL, row.APIModel, row.AuthMode}
}
func (sw *Switcher) prepareGuard(accountID string, auth Auth) (func() error, error) {
	s := sw.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, err := s.loadCredential(accountID)
	if err != nil {
		return nil, err
	}
	// Validation can wait for the network; do not overwrite a replacement credential.
	if stored.AuthMode != auth.AuthMode || stored.APIKey() != auth.APIKey() || stored.Tokens.IDToken != auth.Tokens.IDToken || stored.Tokens.AccessToken != auth.Tokens.AccessToken || stored.Tokens.RefreshToken != auth.Tokens.RefreshToken || stored.Tokens.AccountID != auth.Tokens.AccountID {
		return nil, ErrSwitchFilesChanged
	}
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	target, ok := idx.Account(accountID)
	if !ok {
		return nil, ErrSwitchFilesChanged
	}
	previous, _ := idx.Account(idx.ActiveAccountID)
	files, err := captureSwitchFiles(s.officialAuth, s.CredentialPath(accountID), s.ConfigPath())
	if err != nil {
		return nil, err
	}
	return func() error {
		if err := verifySwitchFiles(files); err != nil {
			return err
		}
		current, err := s.loadIndex()
		if err != nil {
			return err
		}
		next, ok := current.Account(accountID)
		before, _ := current.Account(current.ActiveAccountID)
		if !ok || current.ActiveAccountID != idx.ActiveAccountID || guardSwitchRow(next) != guardSwitchRow(target) || guardSwitchRow(before) != guardSwitchRow(previous) {
			return ErrSwitchFilesChanged
		}
		return nil
	}, nil
}
