package codex

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

// SwitchHooks are the network-facing steps of a switch, supplied by the
// caller so this package stays offline.
type SwitchHooks struct {
	// Refresh renews the tokens of auth in place (typically through
	// Auth.MergeRefreshResponse). A server refusal must be returned as, or
	// wrap, ErrReauthRequired. Nil means a needed refresh fails with
	// ErrRefreshUnavailable.
	Refresh func(ctx context.Context, auth *Auth) error
	// Validate proves the credential works, for example with a quota
	// request. It is skipped for API keys and when nil. A revoked credential
	// must be reported as, or wrap, ErrReauthRequired.
	Validate func(ctx context.Context, auth Auth) error
	// OnPersistError hears about a failure to record "needs re-authorization"
	// on the account row; the original error is still the one returned.
	OnPersistError func(error)
}

// Switcher switches the Codex CLI to a managed account.
type Switcher struct {
	Store *Store
	Hooks SwitchHooks
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	// failpoint, set by tests, fails a named stage of the commit phase.
	failpoint func(stage string) error
}

// SwitchResult is the account row and credential now in force.
type SwitchResult struct {
	Account ManagedAccount
	Auth    Auth
}

func (sw *Switcher) now() time.Time {
	if sw.Now != nil {
		return sw.Now()
	}
	return time.Now()
}

func (sw *Switcher) fail(stage string) error {
	if sw.failpoint == nil {
		return nil
	}
	return sw.failpoint(stage)
}

// SwitchTo makes accountID the Codex login by writing its credential to the
// official auth.json. The target is verified first (refreshing it when the
// access token has expired, then running the Validate hook); a failure there
// leaves the official login untouched. If writing the official file, the
// managed copy or the index then fails, all three are restored to their
// state before the commit and the restore is verified.
func (sw *Switcher) SwitchTo(ctx context.Context, accountID string) (SwitchResult, error) {
	commit, err := sw.PrepareSwitch(ctx, accountID)
	if err != nil {
		return SwitchResult{}, err
	}
	return commit()
}

// PrepareSwitch validates the credential before a caller stops its runtime;
// the returned commit retains the existing atomic file transaction.
func (sw *Switcher) PrepareSwitch(ctx context.Context, accountID string) (func() (SwitchResult, error), error) {
	if err := sw.Store.CheckOfficialCredentialStore(); err != nil {
		return nil, err
	}
	auth, err := sw.Store.LoadCredential(accountID)
	if err != nil {
		return nil, err
	}
	if err := requireUsable(auth, sw.now()); err != nil {
		return nil, err
	}
	auth, err = sw.verify(ctx, accountID, auth)
	if err != nil {
		return nil, err
	}
	// The refresh above may have rotated tokens, so re-check usability.
	if err := requireUsable(auth, sw.now()); err != nil {
		return nil, err
	}
	guard, err := sw.prepareGuard(accountID, auth)
	if err != nil {
		return nil, err
	}
	return func() (SwitchResult, error) {
		return sw.commit(accountID, auth, func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return guard()
		})
	}, nil
}

// EnsureValidCredential returns the managed credential of accountID, renewed
// when its access token has expired. Renewed tokens are stored through
// Store.WriteRefreshedCredential, so only the current account's official file
// is touched.
func (sw *Switcher) EnsureValidCredential(ctx context.Context, accountID string) (Auth, error) {
	auth, err := sw.Store.LoadCredential(accountID)
	if err != nil {
		return Auth{}, err
	}
	return sw.ensureValid(ctx, accountID, auth)
}

// verify refreshes and probes auth. A reauthorization failure marks the
// account row.
func (sw *Switcher) verify(ctx context.Context, accountID string, auth Auth) (Auth, error) {
	auth, err := sw.ensureValid(ctx, accountID, auth)
	if err == nil && !auth.IsAPIKey() && sw.Hooks.Validate != nil {
		err = sw.Hooks.Validate(ctx, auth)
	}
	if err != nil {
		if errors.Is(err, ErrReauthRequired) {
			sw.markReauth(accountID, err)
		}
		return Auth{}, err
	}
	return auth, nil
}

func (sw *Switcher) ensureValid(ctx context.Context, accountID string, auth Auth) (Auth, error) {
	if auth.IsAPIKey() {
		if auth.APIKey() == "" {
			return Auth{}, ErrInvalidCredential
		}
		return auth, nil
	}
	if !auth.CanRefreshOAuth() && auth.Tokens.AccessToken == "" {
		return Auth{}, ErrInvalidCredential
	}
	now := sw.now()
	if !IsTokenExpired(auth.Tokens.AccessToken, now) {
		return auth, nil
	}
	if !auth.CanRefreshOAuth() {
		// A session access token cannot be renewed without a refresh token.
		return Auth{}, ErrExpiredAccessWithoutRefresh
	}
	if sw.Hooks.Refresh == nil {
		return Auth{}, ErrRefreshUnavailable
	}
	working := auth
	if err := sw.Hooks.Refresh(ctx, &working); err != nil {
		return Auth{}, err
	}
	if _, err := sw.Store.WriteRefreshedCredential(accountID, working, now); err != nil {
		return Auth{}, err
	}
	return working, nil
}

func (sw *Switcher) markReauth(accountID string, cause error) {
	err := func() error {
		idx, err := sw.Store.LoadIndex()
		if err != nil {
			return err
		}
		acc, ok := idx.Account(accountID)
		if !ok {
			return &AccountNotFoundError{ID: accountID}
		}
		return sw.Store.UpdateMetadata(acc.ApplyError(cause.Error(), true))
	}()
	if err != nil && sw.Hooks.OnPersistError != nil {
		sw.Hooks.OnPersistError(err)
	}
}

// commit writes the official file, the managed copy, config.toml (the managed
// provider of a relay account, or its removal) and the index as one
// all-or-nothing step.
func (sw *Switcher) commit(accountID string, auth Auth, check func() error) (SwitchResult, error) {
	s := sw.Store
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := check(); err != nil {
		return SwitchResult{}, err
	}
	if err := s.CheckOfficialCredentialStore(); err != nil {
		return SwitchResult{}, err
	}

	// Snapshot after any refresh above, so a rollback never reinstates a
	// refresh token the server has already rotated away.
	snaps := []fileSnapshot{
		takeSnapshot(s.officialAuth),
		takeSnapshot(s.CredentialPath(accountID)),
		takeSnapshot(s.IndexPath()),
		takeConfigSnapshot(s.ConfigPath()),
	}
	result, err := sw.commitLocked(accountID, auth)
	if err == nil {
		return result, nil
	}
	if rbErr := restoreSnapshots(snaps); rbErr != nil {
		return SwitchResult{}, errors.Join(err, ErrRollbackFailed, rbErr)
	}
	return SwitchResult{}, err
}

func (sw *Switcher) commitLocked(accountID string, auth Auth) (SwitchResult, error) {
	s, now := sw.Store, sw.now()
	idx, err := s.loadIndex()
	if err != nil {
		return SwitchResult{}, err
	}
	acc, ok := idx.Account(accountID)
	if !ok {
		return SwitchResult{}, &AccountNotFoundError{ID: accountID}
	}
	var previous *ManagedAccount
	if prev, ok := idx.Account(idx.ActiveAccountID); ok {
		previous = &prev
	}
	steps := []struct {
		stage string
		run   func() error
	}{
		{"official", func() error { return s.writeOfficial(auth, now) }},
		{"credential", func() error { return s.saveCredential(accountID, auth) }},
		{"verify", func() error { return s.verifyInstalled(accountID, auth) }},
		{"config", func() error { return s.applyProviderConfig(acc, previous, now) }},
		{"index", func() error {
			acc = acc.WithIdentity(auth, "", now)
			acc.CachedQuota, acc.LastQuotaAt = nil, nil
			acc.LastUsedAt = &now
			acc.RequiresReauth, acc.LastError = false, ""
			idx.Upsert(acc)
			idx.ActiveAccountID = accountID
			return s.saveIndex(idx)
		}},
	}
	for _, step := range steps {
		if err := sw.fail(step.stage); err != nil {
			return SwitchResult{}, err
		}
		if err := step.run(); err != nil {
			return SwitchResult{}, err
		}
	}
	return SwitchResult{Account: acc, Auth: auth}, nil
}

// verifyInstalled reads both files back and checks they hold auth.
func (s *Store) verifyInstalled(accountID string, auth Auth) error {
	for _, path := range []string{s.officialAuth, s.CredentialPath(accountID)} {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := DecodeAuth(data)
		if err != nil {
			return err
		}
		if !installedMatches(got, auth) {
			return errors.New("codex: installed credential does not match the target")
		}
	}
	return nil
}

// installedMatches compares a credential read back from disk with the target.
// An API key file legitimately keeps the OAuth tokens and last_refresh of the
// login it replaced (see Auth.MarshalFile), so for a key only the key itself
// and the API mode count.
func installedMatches(got, want Auth) bool {
	if want.IsAPIKey() {
		return got.IsAPIKey() && got.APIKey() == want.APIKey()
	}
	return got.Equal(want)
}

// fileSnapshot remembers a file's bytes and mode so it can be put back.
type fileSnapshot struct {
	path   string
	data   []byte
	mode   fs.FileMode
	exists bool
	// plain marks a user file such as config.toml: restored with its own mode
	// and without the owner-only policy of credential files.
	plain bool
}

func takeSnapshot(path string) fileSnapshot {
	snap := fileSnapshot{path: path}
	info, err := os.Stat(path)
	if err != nil {
		return snap
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return snap
	}
	snap.data, snap.mode, snap.exists = data, info.Mode().Perm(), true
	return snap
}

// restoreSnapshots puts every file back, then re-reads each to prove it.
func restoreSnapshots(snaps []fileSnapshot) error {
	var errs []error
	for _, snap := range snaps {
		if err := snap.restore(); err != nil {
			errs = append(errs, err)
		}
	}
	for _, snap := range snaps {
		if err := snap.verify(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (f fileSnapshot) restore() error {
	if !f.exists {
		if err := os.Remove(f.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	mode := f.mode
	if runtime.GOOS == "windows" && !f.plain {
		// Windows reports writable files as 0666; those mode bits cannot
		// select the ACL policy for credential/index rollback.
		mode = filePerm
	}
	return platform.WriteFileAtomic(f.path, f.data, mode)
}

func (f fileSnapshot) verify() error {
	data, err := os.ReadFile(f.path)
	if !f.exists {
		if err == nil {
			return errors.New("codex: file that did not exist before is still present")
		}
		return nil
	}
	if err != nil || !bytes.Equal(data, f.data) {
		return errors.New("codex: restored file differs from the original")
	}
	if runtime.GOOS == "windows" && !f.plain {
		return platform.VerifyOwnerOnly(f.path, filePerm)
	}
	return nil
}
