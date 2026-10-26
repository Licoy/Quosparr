package claude

import (
	"bytes"
	"context"
	"encoding/json"
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
	indexFileName = "claude-index.json"
	authFileName  = "auth.json"
)

// Store is the managed Claude account library. Paths are injectable so tests
// never touch a real home directory. Official credentials are read and written
// only through Official. NewStoreAt points Official at the credentials file so
// unit tests do not open the macOS keychain. NewStore uses OpenOfficialStore.
type Store struct {
	root            string
	runwayHome      string
	configDir       string
	home            string
	configEnv       string
	keychainAccount string
	Official        OfficialCredentialStore
	mu              sync.Mutex
}

// NewStore keeps accounts under <runwayHome>/accounts and resolves the official
// credential from configEnv (the raw CLAUDE_CONFIG_DIR value).
func NewStore(configDir, runwayHome, home, configEnv, keychainAccount string) *Store {
	return &Store{
		root:            filepath.Join(runwayHome, "accounts"),
		runwayHome:      runwayHome,
		configDir:       configDir,
		home:            home,
		configEnv:       configEnv,
		keychainAccount: keychainAccount,
	}
}

// NewStoreAt keeps accounts in accountsRoot. Official is the credentials file
// in configDir, never the keychain.
func NewStoreAt(accountsRoot, configDir, home string) *Store {
	return &Store{
		root:      accountsRoot,
		configDir: configDir,
		home:      home,
		Official:  FileStore{Path: CredentialsFilePath(configDir)},
	}
}

// DefaultStore uses ~/.quosparr/accounts and the current CLAUDE_CONFIG_DIR.
func DefaultStore() (*Store, error) {
	home, err := platform.Home()
	if err != nil {
		return nil, err
	}
	env := os.Getenv("CLAUDE_CONFIG_DIR")
	return NewStore(ResolveConfigDir(env, home), platform.RunwayDir(home), home, env, ""), nil
}

// Root is the accounts directory.
func (s *Store) Root() string { return s.root }

// runwayHomeDir is the Quosparr data directory that holds accounts/ and
// backups/. A store built with NewStoreAt derives it from the accounts root.
func (s *Store) runwayHomeDir() string {
	if s.runwayHome != "" {
		return s.runwayHome
	}
	return filepath.Dir(s.root)
}

// ConfigDir is the Claude config directory.
func (s *Store) ConfigDir() string { return s.configDir }

// HomeDir is the home directory used to find .claude.json.
func (s *Store) HomeDir() string { return s.home }

// IndexPath is claude-index.json.
func (s *Store) IndexPath() string { return filepath.Join(s.root, indexFileName) }

// AccountDir is the directory of an account id.
func (s *Store) AccountDir(id string) string {
	return filepath.Join(s.root, sanitizePathComponent(id))
}

// CredentialPath is the managed auth.json of id.
func (s *Store) CredentialPath(id string) string {
	return filepath.Join(s.AccountDir(id), authFileName)
}

// PendingHome is the scratch directory of a login that this package does not start.
func (s *Store) PendingHome(id string) string {
	return filepath.Join(s.root, ".claude-pending-"+sanitizePathComponent(id))
}

func (s *Store) official() OfficialCredentialStore {
	if s.Official != nil {
		return s.Official
	}
	return OpenOfficialStore(s.configDir, s.home, s.keychainAccount, s.configEnv)
}

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
	if name == "" || strings.Trim(name, ".") == "" {
		return "claude-account"
	}
	return name
}

func (s *Store) ensureRoot() error { return platform.EnsurePrivateDirectory(s.root) }

// EnsurePendingHome creates the scratch directory. It fails if it already exists.
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

// LoadIndex reads claude-index.json. A missing file is an empty index.
func (s *Store) LoadIndex() (AccountIndex, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadIndex()
}

func (s *Store) loadIndex() (AccountIndex, error) {
	if err := s.ensureRoot(); err != nil {
		return AccountIndex{}, operationError(StageReadIndex, err)
	}
	data, err := os.ReadFile(s.IndexPath())
	if errors.Is(err, os.ErrNotExist) {
		return NewAccountIndex(), nil
	}
	if err != nil {
		return AccountIndex{}, operationError(StageReadIndex, err)
	}
	idx, err := ParseAccountIndex(data)
	if err != nil {
		return AccountIndex{}, operationError(StageReadIndex, err)
	}
	if idx.Version != IndexVersion {
		return AccountIndex{}, classifiedError(StageReadIndex, KindUnsupportedVersion, errors.New("claude: unsupported index version"))
	}
	return idx, nil
}

// ParseAccountIndex decodes claude-index.json. The error never includes the
// file body.
func ParseAccountIndex(data []byte) (AccountIndex, error) {
	var idx AccountIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return AccountIndex{}, classifiedError(StageReadIndex, KindInvalidData, err)
	}
	if idx.Accounts == nil {
		idx.Accounts = []ManagedAccount{}
	}
	normalizeAPIRows(idx.Accounts)
	return idx, nil
}

// normalizeAPIRows brings base URLs saved by earlier versions, which could
// keep a trailing /v1, to the form written to ANTHROPIC_BASE_URL now. The row
// is saved in that form with its next change.
func normalizeAPIRows(rows []ManagedAccount) {
	for i := range rows {
		if p := rows[i].API; p != nil {
			if base, err := NormalizeAPIBaseURL(p.BaseURL); err == nil && base != p.BaseURL {
				fixed := *p
				fixed.BaseURL = base
				rows[i].API = &fixed
			}
		}
	}
}

func (s *Store) saveIndex(idx AccountIndex) error {
	if err := s.ensureRoot(); err != nil {
		return operationError(StageWriteIndex, err)
	}
	if idx.Accounts == nil {
		idx.Accounts = []ManagedAccount{}
	}
	data, err := marshalIndex(idx)
	if err != nil {
		return operationError(StageWriteIndex, err)
	}
	if err := rejectTokenKeys(data); err != nil {
		return operationError(StageWriteIndex, err)
	}
	return operationError(StageWriteIndex, platform.WriteFileAtomic(s.IndexPath(), data, storeFilePerm))
}

func marshalIndex(idx AccountIndex) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(idx); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LoadCredentialData returns the managed auth.json of id.
func (s *Store) LoadCredentialData(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadCredential(id)
}

func (s *Store) loadCredential(id string) ([]byte, error) {
	data, err := os.ReadFile(s.CredentialPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, &AccountNotFoundError{ID: id}
	}
	return data, operationError(StageReadCredential, err)
}

// UpsertOptions tunes Upsert.
type UpsertOptions struct {
	MakeCurrent            bool
	PreserveRefreshFailure bool
	Now                    time.Time
}

func (o UpsertOptions) now() time.Time {
	if o.Now.IsZero() {
		return time.Now()
	}
	return o.Now
}

// Upsert stores a managed copy of data for identity and updates the index.
// The copy is only claudeAiOauth. identity.AccountUUID is required.
func (s *Store) Upsert(data []byte, identity CredentialIdentity, opt UpsertOptions) (ManagedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.upsert(data, identity, opt)
}

func (s *Store) upsert(data []byte, identity CredentialIdentity, opt UpsertOptions) (ManagedAccount, error) {
	if strings.TrimSpace(identity.AccountUUID) == "" {
		return ManagedAccount{}, ErrIdentityRequired
	}
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return ManagedAccount{}, err
	}
	now := opt.now()
	identity = identity.withCredential(doc)
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	id := identity.StableID()
	acc := ManagedAccount{
		ID: id, SortIndex: nextSortIndex(idx), Identity: identity, CreatedAt: now,
		SubscriptionType: doc.OAuth().SubscriptionType, RateLimitTier: doc.OAuth().RateLimitTier,
	}
	if existing, found := idx.Account(id); found {
		acc = existing
		acc.Identity = identity
		acc.SubscriptionType = doc.OAuth().SubscriptionType
		acc.RateLimitTier = doc.OAuth().RateLimitTier
	}
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
	if err := s.saveCredential(id, data); err != nil {
		return ManagedAccount{}, err
	}
	idx.Upsert(acc)
	if err := s.saveIndex(idx); err != nil {
		return ManagedAccount{}, err
	}
	return acc, nil
}

func nextSortIndex(idx AccountIndex) int {
	next := 0
	for _, a := range idx.Accounts {
		next = max(next, a.SortIndex+1)
	}
	return next
}

func (s *Store) saveCredential(id string, data []byte) error {
	managed, err := ManagedDocument(data)
	if err != nil {
		return operationError(StageWriteCredential, err)
	}
	if err := platform.EnsurePrivateDirectory(s.AccountDir(id)); err != nil {
		return operationError(StageWriteCredential, err)
	}
	return operationError(StageWriteCredential, platform.WriteFileAtomic(s.CredentialPath(id), managed, storeFilePerm))
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

// SetCurrentAccountID marks id current. An empty id clears it.
func (s *Store) SetCurrentAccountID(id string, lastUsedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setCurrent(id, lastUsedAt)
}

func (s *Store) setCurrent(id string, lastUsedAt time.Time) error {
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

// Reorder renumbers sort indexes to follow ids. Unknown ids are skipped.
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

// LoadOfficial reads the official credential blob.
func (s *Store) LoadOfficial(ctx context.Context) ([]byte, error) {
	data, err := s.official().Load(ctx)
	return data, operationError(StageReadOfficial, err)
}

// SaveOfficial writes the official credential blob. Callers are the explicit
// account switch and its rollback.
func (s *Store) SaveOfficial(ctx context.Context, data []byte) error {
	if _, err := ParseAuthDocument(data); err != nil {
		return err
	}
	return operationError(StageWriteCredential, s.official().Save(ctx, data))
}

// RefreshManaged rotates the refresh token of a non-current account and writes
// both new tokens into the managed copy only. The current account is refused.
// invalid_grant marks the account as needing a new login and leaves the old
// tokens in place. This is not used to dodge a usage 429.
func (s *Store) RefreshManaged(ctx context.Context, id string, client RefreshClient, now time.Time) error {
	original, err := s.credentialIfRefreshable(id)
	if err != nil {
		return err
	}
	doc, err := ParseAuthDocument(original)
	if err != nil {
		return err
	}
	access, refresh, expiresIn, err := client.Refresh(ctx, doc.OAuth().RefreshToken)
	if errors.Is(err, ErrInvalidGrant) {
		_ = s.markReauth(id)
		return err
	}
	if err != nil {
		return err
	}
	expiresAt, hasExpiry := ExpiryFromRefresh(now, expiresIn)
	updated, err := ApplyTokenRefresh(original, access, refresh, expiresAt, hasExpiry)
	if err != nil {
		return err
	}
	return s.storeRefreshed(id, original, updated)
}

func (s *Store) credentialIfRefreshable(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return nil, &AccountNotFoundError{ID: id}
	}
	if acc.IsAPI() {
		return nil, ErrAPIAccount
	}
	if idx.CurrentAccountID == id {
		return nil, ErrCurrentAccountNotRefreshed
	}
	return s.loadCredential(id)
}

func (s *Store) storeRefreshed(id string, original, updated []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	if idx.CurrentAccountID == id {
		return ErrCurrentAccountNotRefreshed
	}
	current, err := s.loadCredential(id)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return ErrCredentialChanged
	}
	if err := s.saveCredential(id, updated); err != nil {
		return err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return &AccountNotFoundError{ID: id}
	}
	acc.RequiresReauth = false
	acc.LastError = ""
	idx.Upsert(acc)
	return s.saveIndex(idx)
}

func (s *Store) markReauth(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return &AccountNotFoundError{ID: id}
	}
	acc.RequiresReauth = true
	acc.LastError = ErrorCodeReauth
	idx.Upsert(acc)
	return s.saveIndex(idx)
}
