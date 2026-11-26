package codex

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/platform"
)

// API relay accounts: an OpenAI API key used against a third-party site. The
// key lives in the managed credential like any API key; the site (base URL),
// the model and the cached balance live on the index row. Making such an
// account the Codex login also writes the managed provider into config.toml
// (see ApplyManagedProvider) and any other switch takes it out again.

var (
	// ErrRelayBaseURL: the relay base URL is not an HTTPS URL.
	ErrRelayBaseURL = errors.New("codex: relay base URL is invalid")
	// ErrInvalidRelayBaseURL is the older name of ErrRelayBaseURL.
	ErrInvalidRelayBaseURL = ErrRelayBaseURL
	// ErrInvalidRelayModel: the relay model name is not usable text.
	ErrInvalidRelayModel = errors.New("codex: relay model is invalid")
	// ErrRelayKeyRequired: the API key is empty.
	ErrRelayKeyRequired = errors.New("codex: relay API key is required")
	// ErrRelayKeyInvalid: the API key holds whitespace or control characters.
	ErrRelayKeyInvalid = errors.New("codex: relay API key has invalid characters")
	// ErrRelayDuplicate: the account library already holds this API key.
	ErrRelayDuplicate = errors.New("codex: an account with this API key already exists")
	// ErrNotAPIKeyAccount: a relay profile belongs to API key accounts only.
	ErrNotAPIKeyAccount = errors.New("codex: relay profile requires an API key account")
)

const maxRelayModelRunes = 200

// RelayProfile is where an API key account points. An empty BaseURL means the
// account talks to OpenAI itself (a plain API key account) and Model is
// ignored.
type RelayProfile struct {
	BaseURL string
	Model   string
}

// normalizeRelayProfile validates p and returns it with the base URL in its
// OpenAI-compatible form (ending /v1).
func normalizeRelayProfile(p RelayProfile) (RelayProfile, error) {
	return parseRelayProfile(p, apiquota.NormalizeBaseURL)
}

func parseRelayProfile(p RelayProfile, normalize func(string) (apiquota.URLs, error)) (RelayProfile, error) {
	raw := strings.TrimSpace(p.BaseURL)
	if raw == "" {
		return RelayProfile{}, nil
	}
	urls, err := normalize(raw)
	if err != nil {
		return RelayProfile{}, fmt.Errorf("%w: %w", ErrInvalidRelayBaseURL, err)
	}
	model := strings.TrimSpace(p.Model)
	if !validRelayModel(model) {
		return RelayProfile{}, ErrInvalidRelayModel
	}
	return RelayProfile{BaseURL: urls.OpenAIBase, Model: model}, nil
}

func validRelayModel(model string) bool {
	if !utf8.ValidString(model) || utf8.RuneCountInString(model) > maxRelayModelRunes {
		return false
	}
	return strings.IndexFunc(model, unicode.IsControl) < 0
}

// ImportRelayAPIKey adds an API key account for a third-party site, or updates
// the profile of the account that already holds the same key. An empty
// p.BaseURL imports a plain API key and keeps the profile of an existing row.
// A non-empty alias replaces the stored alias. The managed copies are written;
// config.toml and the official auth.json are changed only when the account is
// the active one and the official login already is this key.
func (im *Importer) ImportRelayAPIKey(key string, p RelayProfile, alias string, makeActive bool) (ManagedAccount, error) {
	return im.importRelay(key, p, alias, makeActive, false)
}

// AddRelayAPIKey is ImportRelayAPIKey for a new account: a key the library
// already holds fails with ErrRelayDuplicate and changes nothing.
func (im *Importer) AddRelayAPIKey(key string, p RelayProfile, alias string, makeActive bool) (ManagedAccount, error) {
	return im.importRelay(key, p, alias, makeActive, true)
}

func (im *Importer) importRelay(key string, p RelayProfile, alias string, makeActive, rejectExisting bool) (ManagedAccount, error) {
	apiKey := trimSpace(key)
	switch {
	case apiKey == "":
		return ManagedAccount{}, ErrRelayKeyRequired
	case !validRelayKey(apiKey):
		return ManagedAccount{}, ErrRelayKeyInvalid
	}
	profile, err := normalizeRelayProfile(p)
	if err != nil {
		return ManagedAccount{}, err
	}
	opt := UpsertOptions{MakeActive: makeActive, Now: im.now()}
	if profile.BaseURL != "" {
		opt.Relay = &profile
	}
	if a := strings.TrimSpace(alias); a != "" {
		opt.Alias = &a
	}
	return im.Store.upsertRelay(APIKeyAuth(apiKey), opt, rejectExisting)
}

// validRelayKey accepts printable text without whitespace: the key becomes an
// HTTP header value and a line of the credential file.
func validRelayKey(key string) bool {
	if !utf8.ValidString(key) {
		return false
	}
	return strings.IndexFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

// UpdateRelayProfile changes the base URL and model of an API key account; an
// empty p.BaseURL turns it back into a plain OpenAI key account. When the
// account is the login Codex uses, config.toml is updated too; if that or the
// index write fails, both files are put back.
func (s *Store) UpdateRelayProfile(id string, p RelayProfile, now time.Time) (ManagedAccount, error) {
	return s.EditRelayAccount(id, RelayEdit{Profile: p, Apply: true}, now)
}

// RelayEdit is one edit of an API key account. An empty Key keeps the stored
// key. Apply writes the edit into the official auth.json and config.toml when
// the account is the login Codex uses (see RelayInUse); without it they are
// left alone and Codex keeps the old settings until the next switch.
type RelayEdit struct {
	Profile RelayProfile
	Key     string
	Apply   bool
}

// RelayInUse reports whether the API key account id is the login Codex uses:
// the active account, whose key the official auth.json holds.
func (s *Store) RelayInUse(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	return err == nil && idx.ActiveAccountID == id && s.officialHoldsKey(id)
}

// EditRelayAccount changes the site, model and optionally the key of an API
// key account. The row keeps its id; a new key replaces the managed copy and
// the identity the official login is matched by, and drops the cached
// balance. Every file it writes is put back if a later step fails.
func (s *Store) EditRelayAccount(id string, e RelayEdit, now time.Time) (ManagedAccount, error) {
	profile, err := normalizeRelayProfile(e.Profile)
	if err != nil {
		return ManagedAccount{}, err
	}
	key := trimSpace(e.Key)
	if key != "" && !validRelayKey(key) {
		return ManagedAccount{}, ErrRelayKeyInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	prev, ok := idx.Account(id)
	if !ok {
		return ManagedAccount{}, &AccountNotFoundError{ID: id}
	}
	if prev.AuthMode != AuthModeAPIKey {
		return ManagedAccount{}, ErrNotAPIKeyAccount
	}
	if key != "" {
		if other, found := matchingAccount(idx, APIKeyAuth(key)); found && other.ID != id {
			return ManagedAccount{}, ErrRelayDuplicate
		}
	}
	inUse := idx.ActiveAccountID == id && s.officialHoldsKey(id)
	next := editedRelayRow(prev, profile, key, now)
	snaps := []fileSnapshot{
		takeSnapshot(s.IndexPath()), takeSnapshot(s.CredentialPath(id)),
		takeSnapshot(s.officialAuth), takeConfigSnapshot(s.ConfigPath()),
	}
	err = func() error {
		if key != "" {
			if err := s.saveCredential(id, APIKeyAuth(key)); err != nil {
				return err
			}
		}
		if inUse && e.Apply {
			if err := s.applyRelayEdit(id, key, next, prev, now); err != nil {
				return err
			}
		}
		if err := s.failAt("index"); err != nil {
			return err
		}
		idx.Upsert(next)
		return s.saveIndex(idx)
	}()
	if err != nil {
		return ManagedAccount{}, s.rollback(err, snaps)
	}
	return next, nil
}

// editedRelayRow is prev with the new profile and, when key is set, the
// identity of that key, without its balance or old errors.
func editedRelayRow(prev ManagedAccount, p RelayProfile, key string, now time.Time) ManagedAccount {
	next := prev
	if key != "" {
		next = next.WithIdentity(APIKeyAuth(key), "", now)
	}
	next = next.withRelayProfile(p.BaseURL, p.Model)
	if key != "" {
		next.APIQuota = nil
		next.RequiresReauth, next.LastError = false, ""
	}
	return next
}

// applyRelayEdit writes an edit of the account Codex uses: the new key into
// the official auth.json (read back to prove it), then config.toml.
func (s *Store) applyRelayEdit(id, key string, next, prev ManagedAccount, now time.Time) error {
	if key != "" {
		if err := s.writeOfficial(APIKeyAuth(key), now); err != nil {
			return err
		}
		if err := s.verifyInstalled(id, APIKeyAuth(key)); err != nil {
			return err
		}
	}
	return s.applyProviderConfig(next, &prev, now)
}

// upsertRelay is Upsert for an API key whose profile may change. When the row
// ends up active and the official login is this key, config.toml follows.
// With rejectExisting a key that already has a row is refused.
func (s *Store) upsertRelay(auth Auth, opt UpsertOptions, rejectExisting bool) (ManagedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	prev, found := matchingAccount(idx, auth)
	if found && rejectExisting {
		return ManagedAccount{}, ErrRelayDuplicate
	}
	id := ManagedAccountID(auth)
	if found {
		id = prev.ID
	}
	snaps := []fileSnapshot{
		takeSnapshot(s.IndexPath()), takeSnapshot(s.CredentialPath(id)), takeConfigSnapshot(s.ConfigPath()),
	}
	saved, err := s.upsert(auth, opt)
	if err == nil {
		err = s.syncAfterUpsert(saved, prev, found, opt.now())
	}
	if err != nil {
		return ManagedAccount{}, s.rollback(err, snaps)
	}
	return saved, nil
}

func (s *Store) syncAfterUpsert(saved, prev ManagedAccount, hadPrev bool, now time.Time) error {
	idx, err := s.loadIndex()
	if err != nil {
		return err
	}
	var before *ManagedAccount
	if hadPrev {
		before = &prev
	}
	return s.syncRelayConfig(idx.ActiveAccountID, before, saved, now)
}

// rollback restores snaps after cause. A restore that fails too is reported
// together with the cause.
func (s *Store) rollback(cause error, snaps []fileSnapshot) error {
	if rbErr := restoreSnapshots(snaps); rbErr != nil {
		return errors.Join(cause, ErrRollbackFailed, rbErr)
	}
	return cause
}

// failAt is the test hook for Store-level multi-file operations.
func (s *Store) failAt(stage string) error {
	if s.failpoint == nil {
		return nil
	}
	return s.failpoint(stage)
}

// syncRelayConfig brings config.toml in line with next after its profile
// changed, but only when next is the active account and the official
// auth.json already holds its key: otherwise the managed provider would be
// paired with a different login, and the next switch writes both anyway.
func (s *Store) syncRelayConfig(activeID string, prev *ManagedAccount, next ManagedAccount, now time.Time) error {
	if activeID != next.ID || next.AuthMode != AuthModeAPIKey || !s.officialHoldsKey(next.ID) {
		return nil
	}
	return s.applyProviderConfig(next, prev, now)
}

// officialHoldsKey reports whether the official auth.json holds the API key of
// the managed account id. Any trouble reading either means "no".
func (s *Store) officialHoldsKey(id string) bool {
	official, err := s.LoadOfficialAuth()
	if err != nil || !official.IsAPIKey() || official.APIKey() == "" {
		return false
	}
	managed, err := s.loadCredential(id)
	return err == nil && managed.IsAPIKey() && managed.APIKey() == official.APIKey()
}

// applyProviderConfig makes config.toml match target: a relay gets the managed
// provider, anything else loses it. prev is the account that was in force, so
// the model Quosparr wrote for a relay can be taken out again. A plain switch
// between accounts that never involved a relay leaves config.toml alone
// unless it names the managed provider, so an unrelated or unreadable
// config.toml cannot block it.
func (s *Store) applyProviderConfig(target ManagedAccount, prev *ManagedAccount, now time.Time) error {
	prevRelay := prev != nil && prev.IsRelay()
	if !target.IsRelay() && !prevRelay {
		data, err := os.ReadFile(s.ConfigPath())
		if err != nil || !bytes.Contains(data, []byte(ManagedProviderID)) {
			return nil
		}
	}
	prevModel, prevBase := "", ""
	if prevRelay {
		prevModel, prevBase = prev.APIModel, prev.APIBaseURL
	}
	return platform.EditFileWithBackup(s.ConfigPath(), s.configBackup(), now, func(old []byte) ([]byte, error) {
		return providerConfigFor(old, target, prevModel, prevBase)
	})
}

// providerConfigFor is the new config.toml content for target. prevModel is
// the model the previous relay account had written, removed when it would
// otherwise outlive a target without a model of its own. prevBase is the
// previous relay's site: a provider of the user's own selecting it is no
// longer selected when the target is not a relay, so a login never goes to
// the relay.
func providerConfigFor(old []byte, target ManagedAccount, prevModel, prevBase string) ([]byte, error) {
	if !target.IsRelay() {
		out, _, err := removeProvider(old, prevModel, prevBase)
		return out, err
	}
	if prevModel != "" && target.APIModel == "" {
		dropped, _, err := RemoveManagedProvider(old, prevModel)
		if err != nil {
			return nil, err
		}
		old = dropped
	}
	return ApplyManagedProvider(old, ProviderConfig{
		Name:    target.ResolvedDisplayName(),
		BaseURL: target.APIBaseURL,
		Model:   target.APIModel,
	})
}

// officialRelayProfile is the site config.toml points Codex at: the base URL
// of the provider model_provider selects, and the top-level model. ok is false
// for OpenAI itself and for a file that is missing or does not parse.
func (s *Store) officialRelayProfile() (RelayProfile, bool) {
	data, err := readFileIfExists(s.ConfigPath())
	if err != nil || len(data) == 0 {
		return RelayProfile{}, false
	}
	config, err := decodeConfig(data)
	if err != nil {
		return RelayProfile{}, false
	}
	model, _ := config["model"].(string)
	p, err := parseRelayProfile(RelayProfile{BaseURL: selectedProviderBaseURL(config), Model: model}, apiquota.NormalizeStoredBaseURL)
	if err != nil || p.BaseURL == "" {
		return RelayProfile{}, false
	}
	return p, true
}

// adoptRelayConfig gives the API key account of the official login the site
// config.toml selects when the account has no site yet, so a relay set up by
// hand or by another tool shows its balance. config.toml is only read.
func (s *Store) adoptRelayConfig(idx *AccountIndex, official Auth) error {
	if !official.IsAPIKey() {
		return nil
	}
	acc, ok := matchingAccount(*idx, official)
	if !ok || acc.AuthMode != AuthModeAPIKey || acc.IsRelay() {
		return nil
	}
	p, ok := s.officialRelayProfile()
	if !ok {
		return nil
	}
	idx.Upsert(acc.withRelayProfile(p.BaseURL, p.Model))
	return s.saveIndex(*idx)
}

// takeConfigSnapshot remembers config.toml (through a symlink, so a restore
// keeps the link). It is a user file, not a credential: no owner-only mode is
// imposed on restore.
func takeConfigSnapshot(path string) fileSnapshot {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	snap := takeSnapshot(path)
	snap.plain = true
	return snap
}
