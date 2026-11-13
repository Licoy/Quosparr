package gemini

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	storeDirName       = "accounts"
	indexFileName      = "gemini-index.json"
	credentialFileName = "credential.json"
	officialLockName   = ".gemini-official.lock"
	storeDirPerm       = 0o700
	storeFilePerm      = 0o600
)

// Store is the managed Gemini account library plus the official login
// locations. Both are injectable so tests never touch real data. A Store
// serializes its own read-modify-write cycles.
type Store struct {
	root     string
	Official *OfficialCredentials
	mu       sync.Mutex
}

// NewStore keeps accounts in <runwayHome>/accounts and reads the official
// login from the system keychain and the mirror files under home.
func NewStore(runwayHome, home string) *Store {
	return &Store{root: filepath.Join(runwayHome, storeDirName), Official: DefaultOfficialCredentials(home)}
}

// NewStoreAt keeps accounts in root with the given official locations.
func NewStoreAt(root string, official *OfficialCredentials) *Store {
	return &Store{root: root, Official: official}
}

// DefaultStore uses ~/.quosparr/accounts and the current user's login.
func DefaultStore() (*Store, error) {
	home, err := platform.Home()
	if err != nil {
		return nil, err
	}
	return NewStore(platform.RunwayDir(home), home), nil
}

// Root is the library directory.
func (s *Store) Root() string { return s.root }

// IndexPath is gemini-index.json.
func (s *Store) IndexPath() string { return filepath.Join(s.root, indexFileName) }

// AccountDir is the directory of account id.
func (s *Store) AccountDir(id string) string { return filepath.Join(s.root, sanitizePathComponent(id)) }

// CredentialPath is the managed credential of account id.
func (s *Store) CredentialPath(id string) string {
	return filepath.Join(s.AccountDir(id), credentialFileName)
}

func (s *Store) officialLockPath() string { return filepath.Join(s.root, officialLockName) }

// sanitizePathComponent keeps letters, marks, numbers, "-", "_" and "."; any
// other rune becomes "_", and so does a name made only of dots.
func sanitizePathComponent(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case unicode.IsLetter(r), unicode.IsMark(r), unicode.IsNumber(r), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if name == "" {
		return "gemini-account"
	}
	if strings.Trim(name, ".") == "" {
		return strings.ReplaceAll(name, ".", "_")
	}
	return name
}

func (s *Store) ensureRoot() error { return platform.EnsurePrivateDirectory(s.root) }

// LoadIndex reads the index (empty when absent). A corrupt index is an
// error, never silently reset.
func (s *Store) LoadIndex() (AccountIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadIndex()
}

func (s *Store) loadIndex() (AccountIndex, error) {
	data, err := os.ReadFile(s.IndexPath())
	if errors.Is(err, os.ErrNotExist) {
		return NewAccountIndex(), nil
	}
	if err != nil {
		return AccountIndex{}, &AccountIOError{Message: "Unable to read the Gemini account index.", Cause: err}
	}
	idx, err := ParseAccountIndex(data)
	if err != nil {
		return AccountIndex{}, &InvalidIndexError{Cause: err}
	}
	if idx.Version != IndexVersion {
		return AccountIndex{}, &UnsupportedIndexVersionError{Version: idx.Version}
	}
	return idx, nil
}

// SaveIndex writes the index atomically with mode 0600.
func (s *Store) SaveIndex(idx AccountIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveIndex(idx)
}

func (s *Store) saveIndex(idx AccountIndex) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	data, err := MarshalAccountIndex(idx)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(s.IndexPath()); err == nil && bytes.Equal(existing, data) {
		return nil
	}
	return platform.WriteFileAtomic(s.IndexPath(), data, storeFilePerm)
}

// LoadCredentialData returns the managed credential of id.
func (s *Store) LoadCredentialData(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCredentialData(id)
}

func (s *Store) loadCredentialData(id string) ([]byte, error) {
	data, err := os.ReadFile(s.CredentialPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, &CredentialMissingError{ID: id}
	}
	if err != nil {
		return nil, &AccountIOError{Message: "Unable to read the managed Gemini credential.", Cause: err}
	}
	return data, nil
}

// SaveCredentialData replaces the managed credential of an indexed account.
func (s *Store) SaveCredentialData(id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := ParseCredential(data); err != nil {
		return err
	}
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if _, ok := idx.Account(id); !ok {
		return &AccountNotFoundError{ID: id}
	}
	return s.writeCredential(id, data)
}

func (s *Store) writeCredential(id string, data []byte) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	if err := platform.EnsurePrivateDirectory(s.AccountDir(id)); err != nil {
		return err
	}
	path := s.CredentialPath(id)
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, data) && platform.VerifyOwnerOnly(path, storeFilePerm) == nil {
		return nil
	}
	return platform.WriteFileAtomic(path, data, storeFilePerm)
}

// UpsertOptions tunes Store.Upsert.
type UpsertOptions struct {
	// MakeCurrent marks the account current and stamps its last-used time.
	MakeCurrent bool
	// PreserveRefreshFailure keeps an earlier requiresReauth/lastError.
	PreserveRefreshFailure bool
	// Now is the clock; zero means time.Now().
	Now time.Time
}

// Upsert adds or updates the account of identity (matched by subject, else
// email), stores data as its credential and returns the row.
func (s *Store) Upsert(data []byte, identity Identity, opt UpsertOptions) (ManagedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cred, err := ParseCredential(data)
	if err != nil {
		return ManagedAccount{}, err
	}
	if !identity.Known() {
		return ManagedAccount{}, ErrIdentityUnknown
	}
	now := opt.Now
	if now.IsZero() {
		now = time.Now()
	}
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	acc, found := idx.matching(identity)
	if found {
		acc.Identity = identity.merge(acc.Identity)
	} else {
		acc = ManagedAccount{ID: identity.StableID(), SortIndex: idx.nextSortIndex(), Identity: identity, CreatedAt: now}
	}
	reauth := cred.RequiresReauthentication(now)
	if opt.PreserveRefreshFailure {
		acc.RequiresReauth = acc.RequiresReauth || reauth
	} else {
		acc.RequiresReauth, acc.LastError = reauth, ""
	}
	if opt.MakeCurrent {
		stamp := now
		acc.LastUsedAt = &stamp
		idx.CurrentAccountID = acc.ID
	}
	if err := s.writeCredential(acc.ID, data); err != nil {
		return ManagedAccount{}, err
	}
	idx.Upsert(acc)
	if err := s.saveIndex(idx); err != nil {
		return ManagedAccount{}, err
	}
	return acc, nil
}

// AccountForLogin returns the account whose managed copy carries the same
// refresh token as cred, without any network call.
func (s *Store) AccountForLogin(cred Credential) (ManagedAccount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cred.RefreshToken == "" {
		return ManagedAccount{}, false
	}
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, false
	}
	for _, a := range idx.Accounts {
		data, err := s.loadCredentialData(a.ID)
		if err != nil {
			continue
		}
		if managed, err := ParseCredential(data); err == nil && sameLogin(managed, cred) {
			return a, true
		}
	}
	return ManagedAccount{}, false
}

// UpdateMetadata replaces the row with the same id.
func (s *Store) UpdateMetadata(acc ManagedAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if _, ok := idx.Account(acc.ID); !ok {
		return &AccountNotFoundError{ID: acc.ID}
	}
	idx.Upsert(acc)
	return s.saveIndex(idx)
}

// SetCurrentAccountID marks id current (empty clears it). It changes the
// index only, never the official login.
func (s *Store) SetCurrentAccountID(id string, lastUsedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if id != "" {
		acc, ok := idx.Account(id)
		if !ok {
			return &AccountNotFoundError{ID: id}
		}
		acc.LastUsedAt = &lastUsedAt
		idx.Upsert(acc)
	}
	idx.CurrentAccountID = id
	return s.saveIndex(idx)
}

// Reorder renumbers the sort order to follow ids; unknown ids are skipped and
// unlisted accounts follow in their current order.
func (s *Store) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	ordered := idx.OrderedAccounts()
	next := 0
	listed := map[string]bool{}
	position := map[string]int{}
	for _, id := range ids {
		if _, ok := idx.Account(id); ok && !listed[id] {
			listed[id] = true
			position[id] = next
			next++
		}
	}
	for _, a := range ordered {
		if !listed[a.ID] {
			position[a.ID] = next
			next++
		}
	}
	for i := range idx.Accounts {
		idx.Accounts[i].SortIndex = position[idx.Accounts[i].ID]
	}
	return s.saveIndex(idx)
}

// Remove deletes a non-current account: its directory moves aside, the index
// is saved without the row, then the moved directory is deleted. A failure
// puts everything back; when that fails the error is a *PartialWriteError.
// The official login is never touched.
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
	next.Accounts = nil
	for _, a := range original.Accounts {
		if a.ID != id {
			next.Accounts = append(next.Accounts, a)
		}
	}
	dir := s.AccountDir(id)
	tombstone := filepath.Join(s.root, ".gemini-remove-"+randomToken())
	moved := false
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, tombstone); err != nil {
			return &AccountIOError{Message: "Unable to remove the Gemini account.", Cause: err}
		}
		moved = true
	}
	if err := s.saveIndex(next); err != nil {
		if moved {
			if rbErr := os.Rename(tombstone, dir); rbErr != nil {
				return &PartialWriteError{Message: "The Gemini account removal could not be rolled back.", Cause: rbErr}
			}
		}
		if reloaded, rbErr := s.loadIndex(); rbErr != nil || !indexesEqual(reloaded, original) {
			return &PartialWriteError{Message: "The Gemini account index rollback could not be verified.", Cause: rbErr}
		}
		return &AccountIOError{Message: "Unable to remove the Gemini account.", Cause: err}
	}
	if moved {
		_ = os.RemoveAll(tombstone)
	}
	return nil
}

func indexesEqual(a, b AccountIndex) bool {
	x, errA := MarshalAccountIndex(a)
	y, errB := MarshalAccountIndex(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

func randomToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(b[:])
}
