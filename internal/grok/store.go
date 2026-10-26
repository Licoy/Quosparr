package grok

import (
	"bytes"
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
	indexFileName = "grok-index.json"
	authFileName  = "auth.json"
	storeDirPerm  = 0o700
	storeFilePerm = 0o600
)

// Store is the managed Grok account library plus the official auth.json
// location. Both paths are injectable so tests never touch real directories.
// A Store serializes its own read-modify-write cycles; across processes the
// single-instance lock keeps writers apart.
type Store struct {
	root         string
	officialHome string
	mu           sync.Mutex
}

// NewStore returns a store keeping accounts in <runwayHome>/accounts and
// treating <officialHome>/auth.json as the official Grok login.
func NewStore(officialHome, runwayHome string) *Store {
	return &Store{root: filepath.Join(runwayHome, "accounts"), officialHome: officialHome}
}

// NewStoreAt is NewStore with the accounts directory given directly.
func NewStoreAt(accountsRoot, officialHome string) *Store {
	return &Store{root: accountsRoot, officialHome: officialHome}
}

// DefaultStore uses ~/.quosparr/accounts and the Grok home of the current
// user (GROK_HOME, else ~/.grok).
func DefaultStore() (*Store, error) {
	home, err := platform.Home()
	if err != nil {
		return nil, err
	}
	return NewStore(ResolveOfficialHome(os.Getenv("GROK_HOME"), home), platform.RunwayDir(home)), nil
}

// ResolveOfficialHome interprets the GROK_HOME setting: empty means ~/.grok,
// "~" and "~/x" are relative to home, anything else is cleaned and made absolute.
func ResolveOfficialHome(configured, home string) string {
	switch {
	case configured == "":
		return filepath.Join(home, ".grok")
	case configured == "~":
		return home
	case strings.HasPrefix(configured, "~/"):
		return filepath.Join(home, configured[2:])
	}
	if abs, err := filepath.Abs(configured); err == nil {
		return abs
	}
	return filepath.Clean(configured)
}

// Root is the accounts directory.
func (s *Store) Root() string { return s.root }

// OfficialHome is the Grok home directory.
func (s *Store) OfficialHome() string { return s.officialHome }

// IndexPath is grok-index.json.
func (s *Store) IndexPath() string { return filepath.Join(s.root, indexFileName) }

// OfficialAuthPath is the official auth.json.
func (s *Store) OfficialAuthPath() string { return filepath.Join(s.officialHome, authFileName) }

// AccountDir is the directory of account id. Ids are sanitized like the Swift
// app does so existing directory names stay valid.
func (s *Store) AccountDir(id string) string { return filepath.Join(s.root, sanitizePathComponent(id)) }

// CredentialPath is the managed auth.json of account id.
func (s *Store) CredentialPath(id string) string {
	return filepath.Join(s.AccountDir(id), authFileName)
}

// PendingHome is the scratch Grok home of a login in progress.
func (s *Store) PendingHome(id string) string {
	return filepath.Join(s.root, ".grok-pending-"+sanitizePathComponent(id))
}

// sanitizePathComponent keeps letters, marks, numbers, "-", "_" and "."; every
// other character becomes "_". An empty result is "grok-account". A name made
// only of dots would address the directory itself or its parent, so those
// dots become "_" too (the one deliberate difference from the Swift app,
// whose ids never take that shape).
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
		return "grok-account"
	}
	if strings.Trim(name, ".") == "" {
		return strings.ReplaceAll(name, ".", "_")
	}
	return name
}

func (s *Store) ensureRoot() error {
	return platform.EnsurePrivateDirectory(s.root)
}

// EnsurePendingHome creates the scratch home for a login. It fails when the
// directory already exists.
func (s *Store) EnsurePendingHome(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return "", err
	}
	dir := s.PendingHome(id)
	if err := os.Mkdir(dir, storeDirPerm); err != nil {
		return "", err
	}
	return dir, platform.ProtectOwnerOnly(dir, storeDirPerm)
}

// LoadIndex reads grok-index.json (an empty index when it does not exist). A
// corrupt index is an error, never silently reset.
func (s *Store) LoadIndex() (AccountIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadIndex()
}

func (s *Store) loadIndex() (AccountIndex, error) {
	if err := s.ensureRoot(); err != nil {
		return AccountIndex{}, err
	}
	data, err := os.ReadFile(s.IndexPath())
	if errors.Is(err, os.ErrNotExist) {
		return NewAccountIndex(), nil
	}
	if err != nil {
		return AccountIndex{}, err
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

// SaveIndex writes grok-index.json atomically with mode 0600.
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
	if existing, err := os.ReadFile(s.IndexPath()); err == nil && bytes.Equal(existing, data) && fileModeIs(s.IndexPath(), storeFilePerm) {
		return nil
	}
	return platform.WriteFileAtomic(s.IndexPath(), data, storeFilePerm)
}

func fileModeIs(path string, perm os.FileMode) bool {
	return platform.VerifyOwnerOnly(path, perm) == nil
}

// indexesEqual compares two indexes by their serialized form.
func indexesEqual(a, b AccountIndex) bool {
	x, errA := MarshalAccountIndex(a)
	y, errB := MarshalAccountIndex(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// LoadCredentialData returns the raw managed credential file of id.
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
	return data, err
}

// SaveCredentialData stores the managed credential of id. The credential must
// belong to the identity of the account id (or be the one its id hashes from).
func (s *Store) SaveCredentialData(id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCredentialData(id, data)
}

func (s *Store) saveCredentialData(id string, data []byte) error {
	doc, err := parseCredential(data)
	if err != nil {
		return err
	}
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if doc.StableID() != id {
		existing, ok := idx.Account(id)
		if !ok || !identitiesMatch(existing.Identity, doc.Identity) {
			return &CredentialIdentityMismatchError{ID: id}
		}
	}
	return s.saveCredentialUnchecked(id, data)
}

func (s *Store) saveCredentialUnchecked(id string, data []byte) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	dir := s.AccountDir(id)
	if err := platform.EnsurePrivateDirectory(dir); err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.CredentialPath(id), data, storeFilePerm)
}

// parseCredential parses data, mapping the document errors to account errors.
func parseCredential(data []byte) (AuthDocument, error) {
	doc, err := ParseAuthDocument(data)
	switch {
	case errors.Is(err, ErrAuthNoManagedCredential):
		return AuthDocument{}, ErrNoManagedCredential
	case err != nil:
		return AuthDocument{}, ErrInvalidCredential
	}
	return doc, nil
}

// UpsertOptions tunes Store.UpsertCredentialData.
type UpsertOptions struct {
	// MakeCurrent marks the account current and stamps its last-used time.
	MakeCurrent bool
	// PreserveRefreshFailure keeps an earlier requiresReauth/lastError when
	// the credential itself looks fine (the account was flagged by a refresh).
	PreserveRefreshFailure bool
	// Now is the clock; zero means time.Now().
	Now time.Time
}

func (o UpsertOptions) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// UpsertCredentialData adds or updates the account data belongs to (matched by
// identity), stores its credential and returns the final row.
func (s *Store) UpsertCredentialData(data []byte, opt UpsertOptions) (ManagedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsertCredentialData(data, opt)
}

func (s *Store) upsertCredentialData(data []byte, opt UpsertOptions) (ManagedAccount, error) {
	now := opt.now()
	doc, err := parseCredential(data)
	if err != nil {
		return ManagedAccount{}, err
	}
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	existing, found := matchingAccount(idx, doc.Identity)
	id := doc.StableID()
	acc := ManagedAccount{ID: id, SortIndex: nextSortIndex(idx), Identity: doc.Identity, CreatedAt: now}
	if found {
		acc, id = existing, existing.ID
	}
	acc.Identity = doc.Identity
	reauth := doc.RequiresReauthentication(now)
	if opt.PreserveRefreshFailure {
		acc.RequiresReauth = acc.RequiresReauth || reauth
	} else {
		acc.RequiresReauth, acc.LastError = reauth, ""
	}
	if opt.MakeCurrent {
		stamp := now
		acc.LastUsedAt = &stamp
		idx.CurrentAccountID = id
	}
	if err := s.saveCredentialUnchecked(id, data); err != nil {
		return ManagedAccount{}, err
	}
	idx.Upsert(acc)
	if err := s.saveIndex(idx); err != nil {
		return ManagedAccount{}, err
	}
	return acc, nil
}

func matchingAccount(idx AccountIndex, identity CredentialIdentity) (ManagedAccount, bool) {
	for _, a := range idx.Accounts {
		if identitiesMatch(a.Identity, identity) {
			return a, true
		}
	}
	return ManagedAccount{}, false
}

func nextSortIndex(idx AccountIndex) int {
	next := 0
	for _, a := range idx.Accounts {
		next = max(next, a.SortIndex+1)
	}
	return next
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

// SetCurrentAccountID marks id current and stamps its last-used time; an empty
// id clears the current account. It changes the index only, never the
// official file.
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
// accounts not listed follow in their current order.
func (s *Store) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	next := 0
	listed := map[string]bool{}
	for _, id := range ids {
		listed[id] = true
		for i := range idx.Accounts {
			if idx.Accounts[i].ID == id {
				idx.Accounts[i].SortIndex = next
				next++
				break
			}
		}
	}
	for i := range idx.Accounts {
		if !listed[idx.Accounts[i].ID] {
			idx.Accounts[i].SortIndex = next
			next++
		}
	}
	return s.saveIndex(idx)
}

// LoadOfficialCredentialData returns the official auth.json bytes.
func (s *Store) LoadOfficialCredentialData() ([]byte, error) {
	data, err := os.ReadFile(s.OfficialAuthPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrOfficialCredentialMissing
	}
	return data, err
}

// LoadOfficialCredentialStatus classifies the official login file.
func (s *Store) LoadOfficialCredentialStatus(now time.Time) OfficialCredentialStatus {
	data, err := s.LoadOfficialCredentialData()
	if errors.Is(err, ErrOfficialCredentialMissing) {
		return OfficialCredentialStatus{Kind: OfficialMissing}
	}
	if err != nil {
		return OfficialCredentialStatus{Kind: OfficialUnreadable}
	}
	doc, err := ParseAuthDocument(data)
	switch {
	case errors.Is(err, ErrAuthNoManagedCredential):
		if authHasAPIKeyScope(data) {
			return OfficialCredentialStatus{Kind: OfficialAPIKeyOnly}
		}
		return OfficialCredentialStatus{Kind: OfficialUnsupported}
	case err != nil:
		return OfficialCredentialStatus{Kind: OfficialMalformed}
	}
	identity := doc.Identity
	if doc.RequiresReauthentication(now) {
		return OfficialCredentialStatus{Kind: OfficialRequiresReauthentication, Identity: &identity}
	}
	return OfficialCredentialStatus{Kind: OfficialAuthenticated, Identity: &identity}
}

// SaveOfficialAuthDataAtomically installs data as the official auth.json. It
// is one of the two places that file is written (see restoreOfficialAuthData
// for the other). Callers:
//   - Module.installCurrentAccount (explicit user switch, first-login bootstrap)
//   - Module.ensureValidCredential and the forced re-refresh in refreshAccount,
//     for the current account only
func (s *Store) SaveOfficialAuthDataAtomically(data []byte) error {
	if _, err := parseCredential(data); err != nil {
		return err
	}
	return s.writeOfficial(data)
}

func (s *Store) writeOfficial(data []byte) error {
	if err := platform.EnsurePrivateDirectory(s.officialHome); err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.OfficialAuthPath(), data, storeFilePerm)
}

// restoreOfficialAuthData puts the official file back to a previous state
// (nil: absent). It undoes a failed switch and is called from nowhere else.
func (s *Store) restoreOfficialAuthData(data []byte, hadFile bool) error {
	if hadFile {
		return s.writeOfficial(data)
	}
	if err := os.Remove(s.OfficialAuthPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// identitiesMatch reports whether two identities are the same person: equal
// stable ids, else the same principal (then user, then email) with compatible teams.
func identitiesMatch(a, b CredentialIdentity) bool {
	if a.StableID() == b.StableID() {
		return true
	}
	if l, r := acctFirstNonEmpty(a.PrincipalID), acctFirstNonEmpty(b.PrincipalID); l != "" && r != "" {
		return strings.EqualFold(l, r) && teamsCompatible(a, b)
	}
	if l, r := acctFirstNonEmpty(a.UserID), acctFirstNonEmpty(b.UserID); l != "" && r != "" {
		return strings.EqualFold(l, r) && teamsCompatible(a, b)
	}
	if l, r := acctFirstNonEmpty(a.Email), acctFirstNonEmpty(b.Email); l != "" && r != "" {
		return strings.EqualFold(l, r) && teamsCompatible(a, b)
	}
	return false
}

func teamsCompatible(a, b CredentialIdentity) bool {
	l, r := acctFirstNonEmpty(a.TeamID), acctFirstNonEmpty(b.TeamID)
	if l == "" || r == "" {
		return true
	}
	return strings.EqualFold(l, r)
}
