package claude

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/licoy/quosparr/internal/platform"
)

// ErrLoginAlreadyOfficial means browser login names the subscription still
// installed in Claude Code, including while settings.json selects an API account.
var ErrLoginAlreadyOfficial = errors.New("claude: browser account is already the official login")

// ErrLoginRollback means a failed browser import could not restore its managed
// credential or newly created directory. It never contains credential text.
var ErrLoginRollback = errors.New("claude: browser account rollback failed")

// ImportBrowserLogin adds a verified browser result without installing it in
// Claude Code. Network identity lookup runs before either library lock is held.
func (m *Module) ImportBrowserLogin(ctx context.Context, result LoginResult) (ManagedAccount, error) {
	if err := ctx.Err(); err != nil {
		return ManagedAccount{}, err
	}
	doc, err := ParseAuthDocument(result.Credential)
	if err != nil {
		return ManagedAccount{}, err
	}
	if strings.TrimSpace(result.Identity.AccountUUID) == "" || strings.TrimSpace(result.Identity.OrganizationUUID) == "" {
		return ManagedAccount{}, classifiedError(StageReadIdentity, KindInvalidData, ErrIdentityRequired)
	}
	if doc.OAuth().AccessToken == "" || doc.OAuth().RefreshToken == "" || !doc.OAuth().HasExpiry || doc.AccessExpired(m.now()) {
		return ManagedAccount{}, ErrIncompleteRefresh
	}
	proof, err := m.browserOfficialProof(ctx)
	if err != nil {
		return ManagedAccount{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Store.mu.Lock()
	defer m.Store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ManagedAccount{}, err
	}
	if err := proof.recheck(ctx, m.Store); err != nil {
		return ManagedAccount{}, err
	}
	return m.importBrowserLocked(ctx, result, proof)
}

func (m *Module) importBrowserLocked(ctx context.Context, result LoginResult, proof browserOfficialProof) (ManagedAccount, error) {
	idx, err := m.Store.loadIndex()
	if err != nil {
		return ManagedAccount{}, operationError(StageReadIndex, err)
	}
	if proof.identity.AccountUUID != "" && proof.identity.StableID() == result.Identity.StableID() {
		return m.importBrowserOfficial(ctx, idx, proof)
	}
	doc, err := ParseAuthDocument(result.Credential)
	if err != nil {
		return ManagedAccount{}, err
	}
	acc := browserAccount(idx, result.Identity.withCredential(doc), doc, m.now())
	idx.Upsert(acc)
	if err := ctx.Err(); err != nil {
		return ManagedAccount{}, err
	}
	if err := m.Store.commitBrowser(browserCommit{acc, idx, result.Credential}, platform.WriteFileAtomic); err != nil {
		return ManagedAccount{}, err
	}
	return acc, nil
}

func (m *Module) importBrowserOfficial(ctx context.Context, idx AccountIndex, proof browserOfficialProof) (ManagedAccount, error) {
	acc, found := idx.Account(proof.identity.StableID())
	current, err := m.Store.browserOfficialCurrent(idx, proof)
	if err != nil {
		return ManagedAccount{}, err
	}
	changed := idx.CurrentAccountID != current
	idx.CurrentAccountID = current
	if found {
		if err := ctx.Err(); err != nil {
			return ManagedAccount{}, err
		}
		if changed {
			if err := m.Store.saveIndex(idx); err != nil {
				return ManagedAccount{}, operationError(StageWriteIndex, err)
			}
		}
		return acc, ErrLoginAlreadyOfficial
	}
	official, err := ParseAuthDocument(proof.data)
	if err != nil {
		return ManagedAccount{}, err
	}
	acc = browserAccount(idx, proof.identity.withCredential(official), official, m.now())
	acc.RequiresReauth = official.RequiresReauthentication(m.now())
	idx.Upsert(acc)
	if err := ctx.Err(); err != nil {
		return ManagedAccount{}, err
	}
	if err := m.Store.commitBrowser(browserCommit{acc, idx, proof.data}, platform.WriteFileAtomic); err != nil {
		return ManagedAccount{}, err
	}
	return acc, ErrLoginAlreadyOfficial
}

type browserOfficialProof struct {
	data     []byte
	present  bool
	identity CredentialIdentity
	api      *APIEnv
	sources  []browserIdentitySource
}

type browserIdentitySource struct {
	path    string
	data    []byte
	present bool
}

func (m *Module) browserOfficialProof(ctx context.Context) (browserOfficialProof, error) {
	var proof browserOfficialProof
	data, err := m.Store.LoadOfficial(ctx)
	if errors.Is(err, ErrNotLoggedIn) {
		return proof, nil
	}
	if err != nil {
		return proof, operationError(StageReadOfficial, err)
	}
	proof.data, proof.present = data, true
	api, source, err := readBrowserAPISettings(m.Store.SettingsPath())
	if err != nil {
		return proof, err
	}
	proof.api = api
	proof.sources = append(proof.sources, source)
	for _, path := range GlobalConfigCandidates(m.Store.ConfigDir(), m.Store.HomeDir()) {
		raw, _, present, err := readOptional(path)
		if err != nil {
			return proof, operationError(StageReadIdentity, err)
		}
		proof.sources = append(proof.sources, browserIdentitySource{path, raw, present})
		if proof.identity.AccountUUID == "" {
			identity, err := IdentityFromOAuthAccount(raw)
			if err != nil {
				return proof, classifiedError(StageReadIdentity, KindInvalidData, errors.Join(ErrIdentityRequired, err))
			}
			if identity.AccountUUID != "" {
				proof.identity = identity
			}
		}
	}
	if proof.identity.AccountUUID == "" || proof.identity.OrganizationUUID == "" {
		token, err := AccessTokenOf(data)
		if err != nil || token == "" || m.Usage == nil {
			return proof, classifiedError(StageReadIdentity, KindInvalidData, errors.Join(ErrIdentityRequired, err))
		}
		raw, _, err := m.Usage.FetchProfile(ctx, token)
		if err != nil {
			if ctx.Err() != nil {
				return proof, ctx.Err()
			}
			return proof, operationError(StageReadIdentity, err)
		}
		proof.identity, err = IdentityFromProfile(raw)
		if err != nil || proof.identity.AccountUUID == "" || proof.identity.OrganizationUUID == "" {
			return proof, classifiedError(StageReadIdentity, KindInvalidData, errors.Join(ErrIdentityRequired, err))
		}
	}
	return proof, nil
}

func (proof browserOfficialProof) recheck(ctx context.Context, store *Store) error {
	data, err := store.LoadOfficial(ctx)
	if errors.Is(err, ErrNotLoggedIn) && !proof.present {
		return nil
	}
	if err != nil {
		return operationError(StageReadOfficial, err)
	}
	if !proof.present || !bytes.Equal(data, proof.data) {
		return ErrCredentialChanged
	}
	for _, source := range proof.sources {
		raw, _, present, err := readOptional(source.path)
		if err != nil {
			return operationError(StageReadIdentity, err)
		}
		if present != source.present || !bytes.Equal(raw, source.data) {
			return ErrCredentialChanged
		}
	}
	return nil
}

func readBrowserAPISettings(path string) (*APIEnv, browserIdentitySource, error) {
	settings, _, present, err := readOptional(path)
	source := browserIdentitySource{path, settings, present}
	if err != nil {
		return nil, source, operationError(StageReadSettings, err)
	}
	env, err := ReadAPISettingsEnv(settings)
	if err != nil {
		return nil, source, classifiedError(StageReadSettings, KindInvalidData, err)
	}
	if env.BaseURL == "" || env.AuthToken == "" {
		return nil, source, nil
	}
	env.BaseURL, err = NormalizeAPIBaseURL(env.BaseURL)
	if err != nil {
		return nil, source, classifiedError(StageReadSettings, KindInvalidData, err)
	}
	return &env, source, nil
}
