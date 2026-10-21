package claude

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/licoy/quosparr/internal/apiquota"
)

// AddAPIAccount adds an API relay account. When an API account with the same
// base URL and key exists, that account's alias and profile are replaced and
// no second row is created; if it is the account Claude Code uses right now,
// settings.json follows. The key is stored only in the managed copy.
func (m *Module) AddAPIAccount(alias string, profile APIProfile, apiKey string) (ManagedAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.addAPI(alias, profile, apiKey)
}

// UpdateAPIAccount replaces the alias and the model fields of an API account.
// An empty profile.BaseURL keeps the stored one, and an empty apiKey keeps the
// stored key. A changed base URL or key drops the cached balance. If the
// account is the one in settings.json, the file follows.
func (m *Module) UpdateAPIAccount(id, alias string, profile APIProfile, apiKey string) (ManagedAccount, error) {
	return m.EditAPIAccount(id, APIEdit{Alias: alias, Profile: profile, Key: apiKey, Apply: true})
}

// APIEdit is one edit of an API account: the alias, the profile (an empty
// BaseURL keeps the stored one) and a new key (empty keeps the stored one).
// Apply rewrites settings.json when the account is the one Claude Code uses
// (see APIAccountInUse); without it the file is left alone.
type APIEdit struct {
	Alias   string
	Profile APIProfile
	Key     string
	Apply   bool
}

// EditAPIAccount is UpdateAPIAccount with the choice of leaving settings.json
// alone.
func (m *Module) EditAPIAccount(id string, e APIEdit) (ManagedAccount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateAPI(id, e.Alias, e.Profile, e.Key, e.Apply)
}

// APIAccountInUse reports whether settings.json points Claude Code at the API
// account id.
func (m *Module) APIAccountInUse(id string) bool {
	inUse, _ := m.APIAccountState(id)
	return inUse
}

// APIAccountState reports whether settings.json points at the API account id,
// and whether applying its stored URL or managed models would change the file.
func (m *Module) APIAccountState(id string) (inUse, outdated bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return false, false
	}
	live, key, found := m.liveAPIAccount(idx)
	if !found || live.ID != id {
		return false, false
	}
	m.Store.mu.Lock()
	defer m.Store.mu.Unlock()
	data, err := readFileIfExists(m.Store.SettingsPath())
	if err != nil {
		return true, false
	}
	outdated, err = apiSettingsOutdated(data, live.API.Env(key), live.appliedAPIEnv(key))
	return true, err == nil && outdated
}

// APIKey returns the stored key of an API account for internal callers such
// as a model-list request. It must not be logged or shown.
func (m *Module) APIKey(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	acc, err := m.reload(id)
	if err != nil {
		return "", err
	}
	if !acc.IsAPI() {
		return "", ErrNotAPIAccount
	}
	return m.Store.LoadAPIKey(id)
}

// ListAPIModels asks a relay for its model ids using the module's proxied
// client. The key is sent only to that relay.
func (m *Module) ListAPIModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	doer, err := m.apiDoer(ctx)
	if err != nil {
		return nil, err
	}
	return apiquota.FetchModels(ctx, doer, baseURL, apiKey, apiquota.ModelStyleAnthropic)
}

// ListAccountAPIModels is ListAPIModels for a stored API account.
func (m *Module) ListAccountAPIModels(ctx context.Context, id string) ([]string, error) {
	acc, err := m.reload(id)
	if err != nil {
		return nil, err
	}
	if !acc.IsAPI() || acc.API == nil {
		return nil, ErrNotAPIAccount
	}
	key, err := m.Store.LoadAPIKey(id)
	if err != nil {
		return nil, err
	}
	return m.ListAPIModels(ctx, acc.API.BaseURL, key)
}

func (m *Module) addAPI(alias string, profile APIProfile, rawKey string) (ManagedAccount, error) {
	key, err := normalizeAPIKey(rawKey)
	if err != nil {
		return ManagedAccount{}, err
	}
	base, err := NormalizeAPIBaseURL(profile.BaseURL)
	if err != nil {
		return ManagedAccount{}, err
	}
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	if existing, ok := m.findAPI(idx, base, key, ""); ok {
		profile.BaseURL = base
		return m.changeAPI(idx, existing, apiChange{alias: &alias, profile: &profile})
	}
	created := APIProfile{BaseURL: base, KeyHint: apiKeyHint(key)}.withModels(profile)
	acc := ManagedAccount{
		ID: freeAPIID(idx, base, key), Kind: KindAPI, Alias: strings.TrimSpace(alias), API: &created,
	}
	return m.Store.PutAPI(acc, key, m.now())
}

// freeAPIID is the derived id, or a salted one in the rare case that an edited
// account still holds the id this base URL and key would get.
func freeAPIID(idx AccountIndex, base, key string) string {
	id := apiAccountID(base, key)
	for n := 2; ; n++ {
		if _, taken := idx.Account(id); !taken {
			return id
		}
		id = apiAccountID(base, key+"|"+strconv.Itoa(n))
	}
}

func (m *Module) updateAPI(id, alias string, profile APIProfile, apiKey string, apply bool) (ManagedAccount, error) {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return ManagedAccount{}, &AccountNotFoundError{ID: id}
	}
	return m.changeAPI(idx, acc, apiChange{alias: &alias, profile: &profile, key: apiKey, keepSettings: !apply})
}

// apiChange is one edit of an API account. A nil field is left alone.
type apiChange struct {
	alias   *string
	profile *APIProfile // model fields replaced; BaseURL replaced when not empty
	key     string      // new key; empty keeps the stored one
	// keepSettings leaves settings.json alone even for the account in use.
	keepSettings bool
}

func (m *Module) changeAPI(idx AccountIndex, acc ManagedAccount, ch apiChange) (ManagedAccount, error) {
	if !acc.IsAPI() || acc.API == nil {
		return ManagedAccount{}, ErrNotAPIAccount
	}
	oldKey, keyErr := m.Store.LoadAPIKey(acc.ID)
	newKey := oldKey
	if strings.TrimSpace(ch.key) != "" {
		var err error
		if newKey, err = normalizeAPIKey(ch.key); err != nil {
			return ManagedAccount{}, err
		}
	} else if keyErr != nil {
		return ManagedAccount{}, keyErr
	}
	profile, err := editedProfile(*acc.API, ch.profile)
	if err != nil {
		return ManagedAccount{}, err
	}
	if newKey != oldKey {
		profile.KeyHint = apiKeyHint(newKey)
	}
	if _, dup := m.findAPI(idx, profile.BaseURL, newKey, acc.ID); dup {
		return ManagedAccount{}, ErrAPIDuplicate
	}
	next := acc
	next.API = &profile
	if ch.alias != nil {
		next.Alias = strings.TrimSpace(*ch.alias)
	}
	if profile.BaseURL != acc.API.BaseURL || newKey != oldKey {
		next.APIQuota, next.LastQuotaAt, next.LastError = nil, nil, ""
	}
	return m.saveEditedAPI(idx, acc, next, oldKey, newKey, keyErr == nil, ch.keepSettings)
}

func editedProfile(old APIProfile, src *APIProfile) (APIProfile, error) {
	if src == nil {
		return old, nil
	}
	profile := old.withModels(*src)
	if strings.TrimSpace(src.BaseURL) != "" {
		base, err := NormalizeAPIBaseURL(src.BaseURL)
		if err != nil {
			return APIProfile{}, err
		}
		profile.BaseURL = base
	}
	return profile, nil
}

// saveEditedAPI writes next. When the account is the one in settings.json
// (and keepSettings is not set), the file is changed first and put back if the
// library cannot be written.
func (m *Module) saveEditedAPI(idx AccountIndex, old, next ManagedAccount, oldKey, newKey string, hadKey, keepSettings bool) (ManagedAccount, error) {
	live, _, isLive := m.liveAPIAccount(idx)
	active := isLive && live.ID == old.ID
	if active && next.AppliedAPIModels == nil {
		next.AppliedAPIModels = appliedAPIModels(old.API.Env(oldKey))
	}
	if active && next.AppliedAPIIdentity == "" {
		var err error
		next.AppliedAPIIdentity, err = appliedAPIIdentity(old.API.BaseURL, oldKey)
		if err != nil {
			return ManagedAccount{}, err
		}
	}
	storeKey := ""
	if newKey != oldKey || !hadKey {
		storeKey = newKey
	}
	if active && !keepSettings {
		previous := old.appliedAPIEnv(oldKey)
		return m.Store.applyAPIAccount(next, storeKey, next.API.Env(newKey), &previous, m.now(), false)
	}
	return m.Store.PutAPI(next, storeKey, m.now())
}

// findAPI returns the API account other than exceptID that has this normalized
// base URL and key.
func (m *Module) findAPI(idx AccountIndex, base, key, exceptID string) (ManagedAccount, bool) {
	for _, a := range idx.Accounts {
		if !a.IsAPI() || a.API == nil || a.ID == exceptID || a.API.BaseURL != base {
			continue
		}
		if stored, err := m.Store.LoadAPIKey(a.ID); err == nil && stored == key {
			return a, true
		}
	}
	return ManagedAccount{}, false
}

// --- settings.json follows the library --------------------------------------

// liveAPIAccount is the API account whose base URL and key are exactly the
// ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN of settings.json. A library
// without API accounts never reads the file; an unreadable file matches
// nothing.
func (m *Module) liveAPIAccount(idx AccountIndex) (ManagedAccount, string, bool) {
	if !hasAPIAccount(idx) {
		return ManagedAccount{}, "", false
	}
	base, token, err := m.Store.ReadAPISettingsFile()
	token = strings.TrimSpace(token)
	if err != nil || token == "" {
		return ManagedAccount{}, "", false
	}
	live, err := NormalizeAPIBaseURL(base)
	if err != nil {
		return ManagedAccount{}, "", false
	}
	for _, a := range idx.Accounts {
		if !a.IsAPI() || a.API == nil || a.API.BaseURL != live {
			continue
		}
		if key, err := m.Store.LoadAPIKey(a.ID); err == nil && key == token {
			return a, key, true
		}
	}
	return ManagedAccount{}, "", false
}

func hasAPIAccount(idx AccountIndex) bool {
	for _, a := range idx.Accounts {
		if a.IsAPI() {
			return true
		}
	}
	return false
}

// currentAPI is the API account Claude Code uses: the one in settings.json
// (live), else the API account the index calls current when its key is still
// stored (a stale index: the env was removed by hand).
func (m *Module) currentAPI(idx AccountIndex) (acc ManagedAccount, key string, live, ok bool) {
	if a, k, found := m.liveAPIAccount(idx); found {
		return a, k, true, true
	}
	a, found := idx.Account(idx.CurrentAccountID)
	if !found || !a.IsAPI() || a.API == nil {
		return ManagedAccount{}, "", false, false
	}
	k, err := m.Store.LoadAPIKey(a.ID)
	if err != nil {
		return ManagedAccount{}, "", false, false
	}
	return a, k, false, true
}

// importAPISettings adds the relay settings.json points Claude Code at when no
// API account has that base URL and key yet, so a relay set up by hand or by
// another tool shows up like a login does. settings.json is only read; a file
// that cannot be read or holds no complete relay imports nothing.
func (m *Module) importAPISettings(idx AccountIndex) (AccountIndex, error) {
	env, err := m.Store.ReadAPISettingsEnvFile()
	if err != nil || env.BaseURL == "" || env.AuthToken == "" {
		return idx, nil
	}
	key, err := normalizeAPIKey(env.AuthToken)
	if err != nil {
		return idx, nil
	}
	base, err := NormalizeAPIBaseURL(env.BaseURL)
	if err != nil {
		return idx, nil
	}
	if _, found := m.findAPI(idx, base, key, ""); found {
		return idx, nil
	}
	profile := APIProfile{BaseURL: base, KeyHint: apiKeyHint(key)}.withModels(APIProfile{
		Model: env.Model, OpusModel: env.OpusModel, SonnetModel: env.SonnetModel,
		HaikuModel: env.HaikuModel, FableModel: env.FableModel,
	})
	identity, err := appliedAPIIdentity(base, key)
	if err != nil {
		return idx, err
	}
	acc := ManagedAccount{ID: freeAPIID(idx, base, key), Kind: KindAPI, API: &profile}
	if previous, ok := idx.Account(idx.CurrentAccountID); ok && previous.IsAPI() && previous.AppliedAPIModels != nil && previous.AppliedAPIIdentity != "" && previous.AppliedAPIIdentity == identity {
		acc.AppliedAPIModels = previous.AppliedAPIModels
		acc.AppliedAPIIdentity = previous.AppliedAPIIdentity
	}
	if _, err := m.Store.PutAPI(acc, key, m.now()); err != nil {
		return idx, err
	}
	return m.Store.LoadIndex()
}

// followAPISettings makes the account in settings.json current. ok is false
// when settings.json holds no managed API account, and nothing is changed.
func (m *Module) followAPISettings(idx AccountIndex) (officialLogin, bool, error) {
	live, _, found := m.liveAPIAccount(idx)
	if !found {
		return officialLogin{}, false, nil
	}
	if idx.CurrentAccountID != live.ID {
		if err := m.Store.SetCurrentAccountID(live.ID, m.now()); err != nil {
			return officialLogin{}, true, err
		}
	}
	return officialLogin{ID: live.ID, API: true}, true, nil
}

// switchToAPI writes the target into settings.json and makes it current.
// Credentials in the keychain or .credentials.json and .claude.json are left
// alone. Settings an earlier API account set are cleaned up.
func (m *Module) switchToAPI(ctx context.Context, idx AccountIndex, target ManagedAccount) error {
	if target.API == nil {
		return ErrNotAPIAccount
	}
	key, err := m.Store.LoadAPIKey(target.ID)
	if err != nil {
		return err
	}
	env := target.API.Env(key)
	prev, prevKey, _, hasPrev := m.currentAPI(idx)
	var previous *APIEnv
	if hasPrev {
		e := prev.appliedAPIEnv(prevKey)
		previous = &e
	}
	if _, err := m.Store.applyAPIAccount(target, "", env, previous, m.now(), true); err != nil {
		return err
	}
	m.refreshUsage(ctx, target.ID, true)
	return nil
}

// switchFromAPI removes the current API account's settings, installs the OAuth
// target, and puts the settings back if the install fails. live is false when
// settings.json no longer held the account: it is cleaned, never restored.
func (m *Module) switchFromAPI(ctx context.Context, id string, prev ManagedAccount, key string, live bool) error {
	env := prev.appliedAPIEnv(key)
	snap, err := m.Store.removeAPIForSwitch(env, m.now())
	if err != nil {
		return err
	}
	err = m.switchToOAuth(ctx, id)
	if err == nil || !live {
		return err
	}
	return errors.Join(err, m.Store.restoreAPISettings(snap), m.Store.SetCurrentAccountID(prev.ID, m.now()))
}

// --- balance ----------------------------------------------------------------

// apiDoer is the proxied HTTP client of the module. There is no direct
// fallback: when the network context refuses, the error is returned.
func (m *Module) apiDoer(ctx context.Context) (apiquota.Doer, error) {
	if m.APIClient != nil {
		return m.APIClient, nil
	}
	client, err := m.Usage.client(ctx)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// refreshAPI reads the relay balance of acc and caches it on the row. A read
// skipped only for spacing keeps the cached balance without an error. A failed
// read keeps the cached balance and records a short error code.
func (m *Module) refreshAPI(ctx context.Context, acc ManagedAccount, force bool) error {
	if acc.API == nil {
		return ErrNotAPIAccount
	}
	key, err := m.Store.LoadAPIKey(acc.ID)
	if err != nil {
		m.noteAPIError(acc.ID, ErrorCodeAPI)
		return err
	}
	slot := acc.API.BaseURL + "|" + key
	if _, ok := m.throttle().Allow(slot, m.now(), force); !ok {
		return nil
	}
	doer, err := m.apiDoer(ctx)
	if err != nil {
		m.noteAPIError(acc.ID, ErrorCodeAPI)
		return err
	}
	m.throttle().Commit(slot, m.now())
	q := sanitizeAPIQuota(apiquota.Probe(ctx, doer, acc.API.BaseURL, key, m.now()), key)
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.storeAPIQuota(acc.ID, q)
}

func (m *Module) storeAPIQuota(id string, q apiquota.Quota) error {
	acc, err := m.reload(id)
	if err != nil {
		return err
	}
	switch q.Status {
	case apiquota.StatusAvailable, apiquota.StatusUnsupported:
		// A site without a balance endpoint is not a failure.
		return m.Store.UpdateMetadata(acc.ApplyAPIQuota(q, m.now()))
	case apiquota.StatusUnauthorized:
		if err := m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeAPIUnauthorized, false)); err != nil {
			return err
		}
		return ErrAPIUnauthorized
	}
	if err := m.Store.UpdateMetadata(acc.ApplyError(ErrorCodeAPI, false)); err != nil {
		return err
	}
	if q.Error != "" {
		return fmt.Errorf("%w: %s", ErrAPIUnavailable, q.Error)
	}
	return ErrAPIUnavailable
}

func (m *Module) noteAPIError(id, code string) {
	if acc, err := m.reload(id); err == nil {
		_ = m.Store.UpdateMetadata(acc.ApplyError(code, false))
	}
}

// sanitizeAPIQuota drops the key from every text field once more before the
// reading is cached in the index.
func sanitizeAPIQuota(q apiquota.Quota, key string) apiquota.Quota {
	if key == "" {
		return q
	}
	q.Error = strings.ReplaceAll(q.Error, key, "<redacted>")
	q.Endpoint = strings.ReplaceAll(q.Endpoint, key, "<redacted>")
	if strings.Contains(strings.ToUpper(q.Unit), strings.ToUpper(key)) {
		q.Unit = ""
	}
	return q
}
