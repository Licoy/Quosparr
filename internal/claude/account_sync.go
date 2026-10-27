package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
)

// officialLogin is the Claude Code login as the library sees it after a sync.
// Data is nil when Claude Code is not logged in. Known is false when neither
// oauthAccount nor the profile endpoint named the account; ID is then the
// previous current id and the login was not stored.
type officialLogin struct {
	ID    string
	Data  []byte
	Known bool
	// API is set when settings.json names a managed API account: the OAuth
	// login was not looked at and ID is that account.
	API bool
}

// Sync makes the library follow the Claude Code login: a login of a known
// account becomes, or is added as, the current account, and no login clears
// the current id. It returns the current account id.
func (m *Module) Sync(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	login, err := m.syncOfficial(ctx)
	return login.ID, err
}

func (m *Module) syncOfficial(ctx context.Context) (officialLogin, error) {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return officialLogin{}, err
	}
	// An API account in settings.json outranks the subscription login: it is
	// what Claude Code uses. A relay the library does not know yet is added
	// first; the subscription login is not imported or overwritten then.
	if idx, err = m.importAPISettings(idx); err != nil {
		return officialLogin{}, err
	}
	if login, ok, err := m.followAPISettings(idx); ok || err != nil {
		return login, err
	}
	// settings.json holds no managed API account any more (the env was removed
	// by hand): an API account the index still calls current is stale, and the
	// library goes back to following the subscription login.
	if cur, found := idx.Account(idx.CurrentAccountID); found && cur.IsAPI() {
		if err := m.Store.SetCurrentAccountID("", m.now()); err != nil {
			return officialLogin{}, err
		}
		idx.CurrentAccountID = ""
	}
	return m.syncOAuth(ctx, idx, true)
}

// syncOAuth reads the Claude Code subscription login. follow makes the login
// current (and clears the current id when there is none); without it the login
// is only stored.
func (m *Module) syncOAuth(ctx context.Context, idx AccountIndex, follow bool) (officialLogin, error) {
	data, err := m.Store.LoadOfficial(ctx)
	if err == nil {
		var hasOAuth bool
		hasOAuth, err = officialHasOAuth(data)
		if err == nil && !hasOAuth {
			err = ErrNotLoggedIn
		}
	}
	if errors.Is(err, ErrNotLoggedIn) {
		if follow && idx.CurrentAccountID != "" {
			if err := m.Store.SetCurrentAccountID("", m.now()); err != nil {
				return officialLogin{}, err
			}
		}
		return officialLogin{}, nil
	}
	if err != nil {
		return officialLogin{}, err
	}
	identity, err := m.officialIdentity(ctx, data)
	if err != nil {
		return officialLogin{}, err
	}
	if identity.AccountUUID == "" {
		return officialLogin{ID: idx.CurrentAccountID, Data: data}, nil
	}
	id := identity.StableID()
	if id == idx.CurrentAccountID && m.copyMatches(id, data) {
		return officialLogin{ID: id, Data: data, Known: true}, nil
	}
	acc, err := m.Store.Upsert(data, identity, UpsertOptions{
		MakeCurrent: follow, PreserveRefreshFailure: id == idx.CurrentAccountID, Now: m.now(),
	})
	if err != nil {
		return officialLogin{}, err
	}
	return officialLogin{ID: acc.ID, Data: data, Known: true}, nil
}

// copyMatches reports whether the managed copy already holds this login.
func (m *Module) copyMatches(id string, data []byte) bool {
	want, err := ManagedDocument(data)
	if err != nil {
		return false
	}
	got, err := os.ReadFile(m.Store.CredentialPath(id))
	return err == nil && bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want))
}

// officialIdentity reads oauthAccount. When it names no account, the profile
// endpoint is asked once per access token, so a login without .claude.json
// is still stored under the right account.
func (m *Module) officialIdentity(ctx context.Context, data []byte) (CredentialIdentity, error) {
	identity, err := ReadOAuthIdentity(m.Store.ConfigDir(), m.Store.HomeDir())
	if err != nil || identity.AccountUUID != "" {
		return identity, err
	}
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return CredentialIdentity{}, err
	}
	token := doc.OAuth().AccessToken
	if m.Usage == nil || token == "" || doc.AccessExpired(m.now()) {
		return CredentialIdentity{}, nil
	}
	key := tokenKey(token)
	if m.profileKey == key {
		return m.profileIdentity, nil
	}
	raw, _, err := m.Usage.FetchProfile(ctx, token)
	if err != nil {
		return CredentialIdentity{}, nil
	}
	profile, err := IdentityFromProfile(raw)
	if err != nil {
		return CredentialIdentity{}, nil
	}
	m.profileKey, m.profileIdentity = key, profile
	return profile, nil
}
