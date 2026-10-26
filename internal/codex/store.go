// Package codex reads and writes Codex credentials and Quosparr's managed
// multi-account library. It never touches the network: token refresh and
// quota probes arrive as injected hooks.
//
// File boundaries (see AGENTS.md):
//
//   - ~/.quosparr/accounts/index.json holds account metadata only, never
//     tokens, and keeps fields it does not know across rewrites.
//   - ~/.quosparr/accounts/<id>/auth.json holds one account's credentials
//     (directory 0700, file 0600, atomic writes).
//   - The official ~/.codex/auth.json is written only by an explicit account
//     switch (Switcher.SwitchTo, Store.InstallCredentialAsOfficial), by OAuth
//     refresh write-back (Store.WriteRefreshedCredential), or explicit recovery
//     (Store.RepairOfficialAuth). Background sync never writes official auth.
package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/platform"
)

const (
	indexFileName = "index.json"
	authFileName  = "auth.json"
	dirPerm       = 0o700
	filePerm      = 0o600
)

// Store is the managed account library plus the official auth.json location.
// Both base paths are injectable so tests never touch the real directories.
// A Store serializes its own read-modify-write cycles; across processes the
// single-instance lock keeps writers apart.
type Store struct {
	root         string
	runwayHome   string
	officialAuth string
	mu           sync.Mutex

	// failpoint, set by tests, fails a named stage of a multi-file operation.
	failpoint func(stage string) error
}

// NewStore returns a store keeping accounts in <runwayHome>/accounts and
// treating <codexHome>/auth.json as the official login file.
func NewStore(codexHome, runwayHome string) *Store {
	return &Store{
		root:         filepath.Join(runwayHome, "accounts"),
		runwayHome:   runwayHome,
		officialAuth: filepath.Join(codexHome, authFileName),
	}
}

// DefaultStore uses ~/.codex and ~/.quosparr of the current user.
func DefaultStore() (*Store, error) {
	home, err := platform.Home()
	if err != nil {
		return nil, err
	}
	return NewStore(platform.CodexDir(home), platform.RunwayDir(home)), nil
}

// Root is the accounts directory.
func (s *Store) Root() string { return s.root }

// IndexPath is index.json.
func (s *Store) IndexPath() string { return filepath.Join(s.root, indexFileName) }

// OfficialAuthPath is the official ~/.codex/auth.json.
func (s *Store) OfficialAuthPath() string { return s.officialAuth }

// AccountDir is the directory of account id. The id is sanitized like the
// Swift app does so existing directory names stay valid.
func (s *Store) AccountDir(id string) string { return filepath.Join(s.root, sanitizePathComponent(id)) }

// CredentialPath is the managed auth.json of account id.
func (s *Store) CredentialPath(id string) string {
	return filepath.Join(s.AccountDir(id), authFileName)
}

// sanitizePathComponent keeps letters, marks, numbers, "-", "_" and "."; every
// other character becomes "_". An empty result is "account". A name made only
// of dots would address a parent directory, so those dots become "_" too.
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
		return "account"
	}
	if strings.Trim(name, ".") == "" {
		return strings.ReplaceAll(name, ".", "_")
	}
	return name
}

func (s *Store) ensureRoot() error {
	return platform.EnsurePrivateDirectory(s.root)
}

// LoadIndex reads index.json (an empty index when it does not exist yet).
// Legacy rows missing identity metadata are completed in memory from their
// credential; ids, directories, order and the active account are untouched.
// A corrupt index is an error, never silently reset.
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
	idx, err := ParseIndex(data)
	if err != nil {
		return AccountIndex{}, fmt.Errorf("codex: index.json is unreadable: %w", err)
	}
	for i := range idx.Accounts {
		if err := s.hydrateLegacyRow(&idx.Accounts[i]); err != nil {
			return AccountIndex{}, err
		}
	}
	return idx, nil
}

// hydrateLegacyRow fills identity metadata an old row never stored, and flags
// a row whose credential is gone or unreadable.
func (s *Store) hydrateLegacyRow(acc *ManagedAccount) error {
	if acc.AuthMode != AuthModeOAuth || (acc.SubjectID != "" && acc.AccountID != "" && acc.Email != "") {
		return nil
	}
	auth, err := s.loadCredential(acc.ID)
	var missing *CredentialMissingError
	switch {
	case errors.As(err, &missing):
		acc.RequiresReauth, acc.LastError = true, "credential_missing"
		return nil
	case errors.Is(err, ErrInvalidCredential):
		acc.RequiresReauth, acc.LastError = true, "invalid_credential"
		return nil
	case err != nil:
		return err
	}
	explicitUser, subject := oauthUserID(auth), oauthSubjectID(auth)
	if acc.SubjectID == "" {
		acc.SubjectID = subject
		// Older versions stored the subject in userId.
		if subject != "" && firstNonEmpty(acc.UserID) == subject {
			acc.UserID = explicitUser
		}
	}
	if acc.UserID == "" {
		acc.UserID = explicitUser
	}
	if ws := oauthAccountID(auth); ws != "" {
		acc.AccountID = ws
	}
	if acc.Email == "" {
		acc.Email = oauthEmail(auth)
	}
	if acc.WorkspaceName == "" {
		acc.WorkspaceName = oauthWorkspaceName(auth)
	}
	return nil
}

// SaveIndex writes index.json atomically with mode 0600.
func (s *Store) SaveIndex(idx AccountIndex) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveIndex(idx)
}

func (s *Store) saveIndex(idx AccountIndex) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	data, err := MarshalIndex(idx)
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.IndexPath(), data, filePerm)
}

// LoadCredential reads and decodes the managed credential of id.
func (s *Store) LoadCredential(id string) (Auth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCredential(id)
}

func (s *Store) loadCredential(id string) (Auth, error) {
	data, err := s.loadCredentialData(id)
	if err != nil {
		return Auth{}, err
	}
	return DecodeAuth(data)
}

// LoadCredentialData returns the raw credential file of id.
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

// SaveCredential writes the managed credential of id. The library may keep
// session or partial credentials; only official writes demand a usable one.
func (s *Store) SaveCredential(id string, auth Auth) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCredential(id, auth)
}

func (s *Store) saveCredential(id string, auth Auth) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	dir := s.AccountDir(id)
	if err := platform.EnsurePrivateDirectory(dir); err != nil {
		return err
	}
	path := s.CredentialPath(id)
	existing, _ := os.ReadFile(path)
	data, err := auth.MarshalFile(existing)
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(path, data, filePerm)
}

// LoadOfficialAuth reads the official ~/.codex/auth.json.
func (s *Store) LoadOfficialAuth() (Auth, error) {
	if err := s.CheckOfficialCredentialStore(); err != nil {
		return Auth{}, err
	}
	data, err := os.ReadFile(s.officialAuth)
	if err != nil {
		return Auth{}, err
	}
	return DecodeAuth(data)
}

// writeOfficial installs auth as the official login file. It is the single
// place the official file is written. Placeholder or truncated tokens are
// refused, and keys the file already holds are kept. Callers:
//   - Switcher.SwitchTo and Store.InstallCredentialAsOfficial (explicit user switch)
//   - Store.WriteRefreshedCredential (OAuth refresh write-back, current account only)
//   - Store.repairLocked, reached only from the explicit RepairOfficialAuth action
func (s *Store) writeOfficial(auth Auth, now time.Time) error {
	if err := s.CheckOfficialCredentialStore(); err != nil {
		return err
	}
	if auth.LoginUsability(now) == InvalidTokens {
		return ErrNotUsableAsCodexLogin
	}
	if err := platform.EnsurePrivateDirectory(filepath.Dir(s.officialAuth)); err != nil {
		return err
	}
	existing, _ := os.ReadFile(s.officialAuth)
	data, err := auth.MarshalFile(existing)
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.officialAuth, data, filePerm)
}

// InstallCredentialAsOfficial copies the managed credential of id into the
// official auth.json. It is an explicit user action (an account switch done
// without verification hooks); call it only for one.
func (s *Store) InstallCredentialAsOfficial(id string, now time.Time) (Auth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return Auth{}, err
	}
	if _, ok := idx.Account(id); !ok {
		return Auth{}, &AccountNotFoundError{ID: id}
	}
	auth, err := s.loadCredential(id)
	if err != nil {
		return Auth{}, err
	}
	if err := requireUsable(auth, now); err != nil {
		return Auth{}, err
	}
	return auth, s.writeOfficial(auth, now)
}

func requireUsable(auth Auth, now time.Time) error {
	switch auth.LoginUsability(now) {
	case ExpiredAccessWithoutRefresh:
		return ErrExpiredAccessWithoutRefresh
	case InvalidTokens:
		return ErrNotUsableAsCodexLogin
	}
	return nil
}

// DeleteAccount removes id from the index and deletes its credential
// directory. The official auth.json is not touched.
func (s *Store) DeleteAccount(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	idx.Remove(id)
	if err := s.saveIndex(idx); err != nil {
		return err
	}
	return os.RemoveAll(s.AccountDir(id))
}

// UpsertOptions tunes Store.Upsert.
type UpsertOptions struct {
	// MakeActive marks the account active. The first account ever added
	// becomes active regardless.
	MakeActive bool
	// Alias and Note replace the stored values when non-nil.
	Alias, Note *string
	// Relay sets the API relay profile of an API key account (base URL
	// already normalized). Nil keeps the profile of an existing row.
	Relay *RelayProfile
	// Now is the clock; zero means time.Now().
	Now time.Time
}

func (o UpsertOptions) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// Upsert adds or updates the account that auth belongs to (matched by stable
// identity) and stores its credential. It returns the final row.
func (s *Store) Upsert(auth Auth, opt UpsertOptions) (ManagedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsert(auth, opt)
}

func (s *Store) upsert(auth Auth, opt UpsertOptions) (ManagedAccount, error) {
	now := opt.now()
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	existing, found := matchingAccount(idx, auth)
	sortIndex := nextSortIndex(idx)
	alias, note := pick(opt.Alias, existing.Alias, found), pick(opt.Note, existing.Note, found)
	created := now
	if found {
		sortIndex, created = existing.SortIndex, existing.CreatedAt
	}
	id := ""
	if found {
		id = existing.ID
	}
	acc := NewManagedAccount(id, auth, sortIndex, alias, note, "", created)
	if found {
		acc.extra = existing.extra
		acc.LastUsedAt = existing.LastUsedAt
		acc.UserID = firstNonEmpty(acc.UserID, existing.UserID)
		acc.SubjectID = firstNonEmpty(acc.SubjectID, existing.SubjectID)
		acc.WorkspaceName = firstNonEmpty(acc.WorkspaceName, existing.WorkspaceName)
		if acc.AuthMode == AuthModeAPIKey && existing.AuthMode == AuthModeAPIKey {
			acc = acc.withRelayProfile(existing.APIBaseURL, existing.APIModel)
			acc.APIQuota = existing.APIQuota
		}
		// Stale meters and errors go so the row refreshes after a re-import.
	}
	if opt.Relay != nil && acc.AuthMode == AuthModeAPIKey {
		acc = acc.withRelayProfile(opt.Relay.BaseURL, opt.Relay.Model)
	}
	if err := s.saveCredential(acc.ID, auth); err != nil {
		return ManagedAccount{}, err
	}
	idx.Upsert(acc)
	if opt.MakeActive || idx.ActiveAccountID == "" {
		idx.ActiveAccountID = acc.ID
		acc.LastUsedAt = &now
		idx.Upsert(acc)
	}
	if err := s.saveIndex(idx); err != nil {
		return ManagedAccount{}, err
	}
	if saved, ok := idx.Account(acc.ID); ok {
		return saved, nil
	}
	return acc, nil
}

func matchingAccount(idx AccountIndex, auth Auth) (ManagedAccount, bool) {
	for _, a := range idx.Accounts {
		if AccountMatches(a, auth) {
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

// pick returns *override when set, else the existing value of a found row.
func pick(override *string, existing string, found bool) string {
	switch {
	case override != nil:
		return *override
	case found:
		return existing
	}
	return ""
}

// UpdateMetadata replaces the row with the same id.
func (s *Store) UpdateMetadata(acc ManagedAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateMetadata(acc)
}

func (s *Store) updateMetadata(acc ManagedAccount) error {
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	existing, ok := idx.Account(acc.ID)
	if !ok {
		return &AccountNotFoundError{ID: acc.ID}
	}
	// Older whole-row metadata writes must not erase a newer credit refresh.
	if existing.ResetCredits != nil && (acc.ResetCredits == nil || existing.ResetCredits.CheckedAt.After(acc.ResetCredits.CheckedAt)) {
		acc.ResetCredits = existing.ResetCredits
	}
	idx.Upsert(acc)
	return s.saveIndex(idx)
}

// SetActiveAccount marks id active and stamps its last-used time; an empty id
// clears the active account. It changes the index only, never the official file.
func (s *Store) SetActiveAccount(id string, lastUsedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if id == "" {
		idx.ActiveAccountID = ""
		return s.saveIndex(idx)
	}
	acc, ok := idx.Account(id)
	if !ok {
		return &AccountNotFoundError{ID: id}
	}
	acc.LastUsedAt = &lastUsedAt
	idx.Upsert(acc)
	idx.ActiveAccountID = id
	return s.saveIndex(idx)
}

// Reorder renumbers the sort order to follow ids.
func (s *Store) Reorder(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	idx.ReindexSortOrder(ids)
	return s.saveIndex(idx)
}

// ImportOfficialAuth adds the official login to the library. It reads the
// official file and writes only managed copies.
func (s *Store) ImportOfficialAuth(makeActive bool, now time.Time) (ManagedAccount, error) {
	auth, err := s.LoadOfficialAuth()
	if err != nil {
		return ManagedAccount{}, err
	}
	return s.Upsert(auth, UpsertOptions{MakeActive: makeActive, Now: now})
}

// OfficialStatus describes the official auth.json after a sync.
type OfficialStatus int

const (
	// OfficialSynced: the official login is usable and now reflected in the index.
	OfficialSynced OfficialStatus = iota
	// OfficialMissing: there is no readable official auth.json.
	OfficialMissing
	// OfficialUnusable: the file decodes but holds placeholder, truncated or
	// expired tokens. Nothing was imported.
	OfficialUnusable
)

// SyncFromOfficialAuth makes the index follow the official login: when its
// identity differs from the active account it is imported or updated and
// marked active. A managed credential that can refresh is never replaced by a
// weaker official snapshot, and a usable credential is never replaced by an
// unusable one.
//
// Missing or invalid official credentials clear the current-account marker,
// without restoring a login the user may have deliberately removed.
func (s *Store) SyncFromOfficialAuth(now time.Time) (AccountIndex, OfficialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return AccountIndex{}, OfficialMissing, err
	}
	official, err := s.LoadOfficialAuth()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		err = errors.Join(err, s.clearActive(&idx))
		return idx, OfficialMissing, err
	}
	if official.LoginUsability(now) != Usable {
		var err error
		managed, loadErr := s.loadCredential(idx.ActiveAccountID)
		if loadErr != nil || managed.LoginUsability(now) != Usable {
			err = s.markActiveInvalid(&idx)
		}
		err = errors.Join(err, s.clearActive(&idx))
		return idx, OfficialUnusable, err
	}
	if existing, ok := matchingAccount(idx, official); ok && len(idx.Accounts) > 0 {
		err = s.adoptOfficial(&idx, existing, official, now)
	} else {
		_, err = s.upsert(official, UpsertOptions{MakeActive: true, Now: now})
		if err == nil {
			idx, err = s.loadIndex()
		}
	}
	if err == nil {
		err = s.adoptRelayConfig(&idx, official)
	}
	return idx, OfficialSynced, err
}

func (s *Store) clearActive(idx *AccountIndex) error {
	if idx.ActiveAccountID == "" {
		return nil
	}
	idx.ActiveAccountID = ""
	return s.saveIndex(*idx)
}

// markActiveInvalid records the invalid official login and propagates IO errors.
func (s *Store) markActiveInvalid(idx *AccountIndex) error {
	active, ok := idx.Account(idx.ActiveAccountID)
	if !ok {
		return nil
	}
	active.RequiresReauth = true
	active.LastError = "invalid credential"
	idx.Upsert(active)
	return s.saveIndex(*idx)
}

// adoptOfficial records that the official login belongs to existing.
func (s *Store) adoptOfficial(idx *AccountIndex, existing ManagedAccount, official Auth, now time.Time) error {
	current, err := s.loadCredential(existing.ID)
	haveCurrent := err == nil
	replace := !haveCurrent || shouldReplaceManaged(current, official, now)
	source := official
	if replace {
		if err := s.saveCredential(existing.ID, official); err != nil {
			return err
		}
	} else {
		source = current
	}
	updated := existing.WithIdentity(source, existing.PlanType, now)
	updated.LastUsedAt = &now
	updated.RequiresReauth, updated.LastError = false, ""
	idx.Upsert(updated)
	idx.ActiveAccountID = existing.ID
	return s.saveIndex(*idx)
}

// shouldReplaceManaged prefers a managed credential that can refresh over a
// weaker official snapshot, and a usable one over an unusable one.
func shouldReplaceManaged(existing, incoming Auth, now time.Time) bool {
	switch {
	case existing.Equal(incoming):
		return false
	case existing.CanRefreshOAuth() && !incoming.CanRefreshOAuth():
		return false
	case existing.LoginUsability(now) == Usable && incoming.LoginUsability(now) != Usable:
		return false
	}
	return true
}

// RepairOfficialAuth reinstalls the best usable managed credential (the active
// one first) as the official login and returns the updated index, only when
// the user explicitly requests recovery.
func (s *Store) RepairOfficialAuth(now time.Time) (AccountIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.repairLocked(now)
}

// repairLocked walks the active account, then the rest in index order. Each
// unusable credential is flagged requiresReauth; the first usable one is
// written to the official file and becomes active. With nothing usable the
// index (with the flags) is saved and returned without an error.
func (s *Store) repairLocked(now time.Time) (AccountIndex, error) {
	idx, err := s.loadIndex()
	if err != nil {
		return AccountIndex{}, err
	}
	for _, id := range repairOrder(idx) {
		auth, err := s.loadCredential(id)
		if err != nil {
			continue
		}
		if auth.LoginUsability(now) != Usable {
			if acc, ok := idx.Account(id); ok {
				acc.RequiresReauth = true
				idx.Upsert(acc)
			}
			continue
		}
		if err := s.writeOfficial(auth, now); err != nil {
			return AccountIndex{}, err
		}
		idx.ActiveAccountID = id
		acc, ok := idx.Account(id)
		if !ok {
			acc = NewManagedAccount(id, auth, 0, "", "", "", now)
		}
		acc = acc.WithIdentity(auth, acc.PlanType, now)
		acc.RequiresReauth, acc.LastError, acc.LastUsedAt = false, "", &now
		idx.Upsert(acc)
		return idx, s.saveIndex(idx)
	}
	return idx, s.saveIndex(idx)
}

func repairOrder(idx AccountIndex) []string {
	var ids []string
	if idx.ActiveAccountID != "" {
		ids = append(ids, idx.ActiveAccountID)
	}
	for _, a := range idx.Accounts {
		if a.ID != idx.ActiveAccountID {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// RefreshWrite says where a refreshed credential was written.
type RefreshWrite struct {
	// Managed: the managed copy was updated.
	Managed bool
	// Official: the official auth.json was updated too.
	Official bool
	// OfficialSkipped: the account is current but the official file holds a
	// different login (changed outside Quosparr), so it was left alone.
	OfficialSkipped bool
}

// WriteRefreshedCredential stores tokens obtained by a refresh. The managed
// copy is always updated. When id is the current account the official
// auth.json is updated as well, unless it now holds a different login (the
// user signed in elsewhere): overwriting it would log that login out. A
// non-current account never touches the official file.
func (s *Store) WriteRefreshedCredential(id string, auth Auth, now time.Time) (RefreshWrite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return RefreshWrite{}, err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return RefreshWrite{}, &AccountNotFoundError{ID: id}
	}
	if err := s.saveCredential(id, auth); err != nil {
		return RefreshWrite{}, err
	}
	res := RefreshWrite{Managed: true}
	if idx.ActiveAccountID != id {
		return res, nil
	}
	official, err := s.LoadOfficialAuth()
	if errors.Is(err, os.ErrNotExist) || (err == nil && !AccountMatches(acc, official)) {
		res.OfficialSkipped = true
		return res, nil
	}
	if err != nil {
		return res, err
	}
	if err := s.writeOfficial(auth, now); err != nil {
		return res, err
	}
	res.Official = true
	return res, nil
}
