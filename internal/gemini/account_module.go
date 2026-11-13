package gemini

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/platform"
)

// QuotaFunc reads a snapshot for an access token.
type QuotaFunc func(ctx context.Context, accessToken string) (QuotaSnapshot, error)

// CommandKind selects what Module.Apply does.
type CommandKind int

const (
	// CommandImportOfficial adopts the official Antigravity login.
	CommandImportOfficial CommandKind = iota
	// CommandMakeCurrent installs the account as the official login.
	CommandMakeCurrent
	// CommandRemove deletes a non-current account.
	CommandRemove
	// CommandSetAlias sets or clears (empty Alias) the alias.
	CommandSetAlias
	// CommandReorder reorders the accounts by IDs.
	CommandReorder
)

// Command is one account action for Module.Apply.
type Command struct {
	Kind  CommandKind
	ID    string
	Alias string
	IDs   []string
	// AllowWhileRunning switches even when Antigravity or agy is running.
	AllowWhileRunning bool
}

// Module orchestrates the Gemini library: official-login reconciliation,
// token keep-alive, switching, login, import and removal. Operations are
// serialized; Load and Apply return the state after the operation.
type Module struct {
	Store *Store
	// OAuth renews tokens and resolves identities (zero value: Google).
	OAuth OAuthClient
	// Quota reads a snapshot; nil uses a production QuotaClient.
	Quota QuotaFunc
	// RunningProcessIDs lists Antigravity processes; nil inspects the system.
	RunningProcessIDs func(ctx context.Context) ([]int, error)
	// AppVersion is stamped into exported packs.
	AppVersion string
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	once                  sync.Once
	sem                   chan struct{}
	pendingOfficialChange bool
	throttle              QuotaThrottle
}

// NewModule returns a Module over store with production defaults.
func NewModule(store *Store) *Module { return &Module{Store: store} }

func (m *Module) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Module) acquire(ctx context.Context) error {
	m.once.Do(func() { m.sem = make(chan struct{}, 1) })
	select {
	case m.sem <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-m.sem
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Module) release() { <-m.sem }

// Load reconciles with the official login and returns the state. An
// external change of the official account is reported once.
func (m *Module) Load(ctx context.Context) (AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return AccountState{}, err
	}
	defer m.release()
	return m.reconcileOfficialIdentity(ctx, true)
}

// Apply runs one account command and returns the state afterwards.
func (m *Module) Apply(ctx context.Context, cmd Command) (AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return AccountState{}, err
	}
	defer m.release()
	acknowledges := false
	var err error
	switch cmd.Kind {
	case CommandImportOfficial:
		err, acknowledges = m.importOfficial(ctx), true
	case CommandMakeCurrent:
		err, acknowledges = m.makeCurrent(ctx, cmd.ID, cmd.AllowWhileRunning), true
	case CommandRemove:
		err = m.Store.Remove(cmd.ID)
	case CommandSetAlias:
		err = m.setAlias(cmd.ID, cmd.Alias)
	case CommandReorder:
		err = m.Store.Reorder(cmd.IDs)
	}
	if err != nil {
		return AccountState{}, err
	}
	return m.stateAfter(ctx, acknowledges)
}

func (m *Module) stateAfter(ctx context.Context, acknowledges bool) (AccountState, error) {
	state, err := m.reconcileOfficialIdentity(ctx, false)
	if err != nil {
		return AccountState{}, err
	}
	if acknowledges {
		m.pendingOfficialChange = false
		state.OfficialIdentityChangedExternally = false
	}
	return state, nil
}

// ImportBrowserLogin files a completed browser login. The first account of
// an empty library without an official login also becomes the official one.
func (m *Module) ImportBrowserLogin(ctx context.Context, result LoginResult) (ManagedAccount, error) {
	if err := m.acquire(ctx); err != nil {
		return ManagedAccount{}, err
	}
	defer m.release()
	if !result.Identity.Known() {
		return ManagedAccount{}, ErrLoginIdentity
	}
	bootstrap, err := m.canBootstrap(ctx)
	if err != nil {
		return ManagedAccount{}, err
	}
	account, err := m.Store.Upsert(result.Credential, result.Identity, UpsertOptions{Now: m.now()})
	if err != nil {
		return ManagedAccount{}, err
	}
	if bootstrap {
		if err := m.installCurrentAccount(ctx, account.ID); err != nil {
			return ManagedAccount{}, err
		}
	}
	m.pendingOfficialChange = false
	return m.requireAccount(account.ID)
}

// canBootstrap: the library is empty and there is no official login.
func (m *Module) canBootstrap(ctx context.Context) (bool, error) {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return false, err
	}
	if len(idx.Accounts) > 0 {
		return false, nil
	}
	_, err = m.Store.Official.Load(ctx)
	return errors.Is(err, ErrOfficialCredentialMissing), nil
}

// ImportPastedText imports pasted credential JSON. Partial success is
// reported in the batch result.
func (m *Module) ImportPastedText(ctx context.Context, text string) (ImportBatchResult, AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return ImportBatchResult{}, AccountState{}, err
	}
	defer m.release()
	payloads := ParsePayloads(text)
	if len(payloads) == 0 {
		state, err := m.reconcileOfficialIdentity(ctx, false)
		return ImportBatchResult{Failures: []string{FailureNoCredentials}, FailureErrors: []error{ErrInvalidCredential}}, state, err
	}
	entries := make([]importEntry, len(payloads))
	for i, data := range payloads {
		label := "json"
		if len(payloads) != 1 {
			label = "json[" + strconv.Itoa(i) + "]"
		}
		entries[i] = importEntry{label: label, data: data}
	}
	result, err := m.importEntries(ctx, entries, true)
	if err != nil {
		return result, AccountState{}, err
	}
	state, err := m.stateAfter(ctx, len(result.Succeeded) > 0)
	return result, state, err
}

type importEntry struct {
	label    string
	data     []byte
	alias    string
	identity Identity
}

// importEntries files each credential. With installCurrent, the first one
// bootstraps an empty library and a re-imported current account is also
// written to the official login.
func (m *Module) importEntries(ctx context.Context, entries []importEntry, installCurrent bool) (ImportBatchResult, error) {
	before, err := m.Store.LoadIndex()
	if err != nil {
		return ImportBatchResult{}, err
	}
	bootstrap, err := m.canBootstrap(ctx)
	if err != nil {
		return ImportBatchResult{}, err
	}
	var result ImportBatchResult
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		account, err := m.importOne(ctx, e, before, installCurrent, bootstrap)
		if err != nil {
			result.Failures = append(result.Failures, e.label+": "+safeImportFailureMessage(err))
			result.FailureErrors = append(result.FailureErrors, err)
			continue
		}
		result.Succeeded = append(result.Succeeded, account)
		bootstrap = false
	}
	return result, nil
}

func (m *Module) importOne(ctx context.Context, e importEntry, before AccountIndex, installCurrent, bootstrap bool) (ManagedAccount, error) {
	cred, err := ParseCredential(e.data)
	if err != nil {
		return ManagedAccount{}, err
	}
	identity := e.identity
	if !identity.Known() {
		if identity, err = m.identify(ctx, cred); err != nil {
			return ManagedAccount{}, err
		}
	}
	account, err := m.Store.Upsert(e.data, identity, UpsertOptions{Now: m.now()})
	if err != nil {
		return ManagedAccount{}, err
	}
	if alias := trimSpace(e.alias); alias != "" {
		account.Alias = alias
		if err := m.Store.UpdateMetadata(account); err != nil {
			return ManagedAccount{}, err
		}
	}
	if installCurrent && (bootstrap || before.CurrentAccountID == account.ID) {
		if err := m.installCurrentAccount(ctx, account.ID); err != nil {
			return ManagedAccount{}, err
		}
	}
	return account, nil
}

func safeImportFailureMessage(err error) string {
	var mismatch *CredentialIdentityMismatchError
	switch {
	case errors.Is(err, ErrInvalidCredential):
		return failureInvalidCredential
	case errors.Is(err, ErrIdentityUnknown), errors.Is(err, ErrUnauthorized), errors.Is(err, ErrAuthenticationRequired):
		return failureIdentityUnknown
	case errors.As(err, &mismatch):
		return failureCredentialIdentityError
	}
	return failureImportFailed
}

// identify names the account of cred: a managed copy with the same refresh
// token, else the id_token claims, else userinfo (refreshing an expired
// access token in memory first).
func (m *Module) identify(ctx context.Context, cred Credential) (Identity, error) {
	if account, ok := m.Store.AccountForLogin(cred); ok {
		return account.Identity, nil
	}
	if claims := cred.IDTokenIdentity(); claims.Subject != "" && claims.Email != "" {
		return claims, nil
	}
	if cred.NeedsRefresh(m.now(), 0) {
		refreshed, err := m.OAuth.Refresh(ctx, cred, m.now())
		if err != nil {
			return Identity{}, err
		}
		cred = refreshed
	}
	return m.OAuth.Identify(ctx, cred)
}

// reconcileOfficialIdentity makes the library follow the official login:
// its account becomes (or is added as) the current one. A missing or
// malformed login clears the current account; an unreadable or
// unidentified one leaves the index as it is.
func (m *Module) reconcileOfficialIdentity(ctx context.Context, consuming bool) (AccountState, error) {
	previous, err := m.Store.LoadIndex()
	if err != nil {
		return AccountState{}, err
	}
	status, currentID, err := m.followOfficial(ctx, previous.CurrentAccountID)
	if err != nil {
		return AccountState{}, err
	}
	index, err := m.Store.LoadIndex()
	if err != nil {
		return AccountState{}, err
	}
	changed := previous.CurrentAccountID != "" && previous.CurrentAccountID != currentID
	if changed {
		m.pendingOfficialChange = true
	}
	report := changed || (consuming && m.pendingOfficialChange)
	if consuming {
		m.pendingOfficialChange = false
	}
	return AccountState{
		Official: status, CurrentAccountID: currentID, Accounts: index.OrderedAccounts(),
		OfficialIdentityChangedExternally: report,
	}, nil
}

func (m *Module) followOfficial(ctx context.Context, previousID string) (OfficialCredentialStatus, string, error) {
	data, err := m.Store.Official.Load(ctx)
	if ctx.Err() != nil {
		return OfficialCredentialStatus{}, "", ctx.Err()
	}
	var cred Credential
	if err == nil {
		cred, err = ParseCredential(data)
		if err != nil {
			err = ErrOfficialCredentialMalformed
		}
	}
	switch {
	case errors.Is(err, ErrOfficialCredentialMissing):
		return OfficialCredentialStatus{Kind: OfficialMissing}, "", m.Store.SetCurrentAccountID("", m.now())
	case errors.Is(err, ErrOfficialCredentialMalformed):
		return OfficialCredentialStatus{Kind: OfficialMalformed}, "", m.Store.SetCurrentAccountID("", m.now())
	case err != nil:
		return OfficialCredentialStatus{Kind: OfficialUnreadable}, previousID, nil
	}
	identity, err := m.identify(ctx, cred)
	if err != nil {
		if ctx.Err() != nil {
			return OfficialCredentialStatus{}, "", ctx.Err()
		}
		return OfficialCredentialStatus{Kind: OfficialUnidentified}, previousID, nil
	}
	account, err := m.Store.Upsert(data, identity, UpsertOptions{MakeCurrent: true, PreserveRefreshFailure: true, Now: m.now()})
	if err != nil {
		return OfficialCredentialStatus{}, "", err
	}
	kind := OfficialAuthenticated
	if cred.RequiresReauthentication(m.now()) {
		kind = OfficialRequiresReauthentication
	}
	id := account.Identity
	return OfficialCredentialStatus{Kind: kind, Identity: &id}, account.ID, nil
}

func (m *Module) importOfficial(ctx context.Context) error {
	data, err := m.Store.Official.Load(ctx)
	if err != nil {
		return err
	}
	cred, err := ParseCredential(data)
	if err != nil {
		return ErrOfficialCredentialMalformed
	}
	identity, err := m.identify(ctx, cred)
	if err != nil {
		return err
	}
	_, err = m.Store.Upsert(data, identity, UpsertOptions{MakeCurrent: true, Now: m.now()})
	return err
}

func (m *Module) makeCurrent(ctx context.Context, id string, allowWhileRunning bool) error {
	if _, err := m.requireAccount(id); err != nil {
		return err
	}
	if !allowWhileRunning {
		inspect := m.RunningProcessIDs
		if inspect == nil {
			inspect = SystemProcessIDs
		}
		pids, err := inspect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &AccountIOError{Message: "Unable to inspect running Antigravity processes.", Cause: err}
		}
		if len(pids) > 0 {
			return &ProcessesRunningError{PIDs: pids}
		}
	}
	// Renew the managed copy before installing it.
	if _, err := m.freshCredential(ctx, id, false, false); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.installCurrentAccount(ctx, id)
}

// installCurrentAccount makes id the official login. The login it replaces
// is kept in the library first (a switch refuses to lose an unidentified
// login). The keychain and existing mirror files are written and verified,
// then the index; any failure restores all of them.
func (m *Module) installCurrentAccount(ctx context.Context, id string) error {
	data, err := m.Store.LoadCredentialData(id)
	if err != nil {
		return err
	}
	target, err := ParseCredential(data)
	if err != nil {
		return err
	}
	lock, err := platform.AcquireLock(m.Store.officialLockPath())
	if err != nil {
		if errors.Is(err, platform.ErrLocked) {
			return ErrOfficialLocked
		}
		return &AccountIOError{Message: "Unable to open the Antigravity credential lock.", Cause: err}
	}
	defer func() { _ = lock.Release() }()

	if err := m.keepReplacedLogin(ctx, target); err != nil {
		return err
	}
	originalIndex, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	official := m.Store.Official
	st, err := official.capture(ctx)
	if err != nil {
		return err
	}
	if err := official.installOver(ctx, data, st); err != nil {
		return err
	}
	if err := m.Store.SetCurrentAccountID(id, m.now()); err != nil {
		rollback := context.WithoutCancel(ctx)
		rbErr := errors.Join(official.restore(rollback, st), m.Store.SaveIndex(originalIndex))
		if rbErr != nil {
			return &PartialWriteError{Message: "The Gemini account switch could not be rolled back.", Cause: errors.Join(err, rbErr)}
		}
		return &AccountIOError{Message: "Unable to switch the Gemini account.", Cause: err}
	}
	return nil
}

func (m *Module) keepReplacedLogin(ctx context.Context, target Credential) error {
	data, err := m.Store.Official.Load(ctx)
	if errors.Is(err, ErrOfficialCredentialMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	cred, err := ParseCredential(data)
	if err != nil || sameLogin(cred, target) {
		return nil
	}
	identity, err := m.identify(ctx, cred)
	if err != nil {
		return &AccountIOError{Message: "The current Antigravity login could not be saved before switching.", Cause: err}
	}
	_, err = m.Store.Upsert(data, identity, UpsertOptions{PreserveRefreshFailure: true, Now: m.now()})
	return err
}

func (m *Module) setAlias(id, alias string) error {
	account, err := m.requireAccount(id)
	if err != nil {
		return err
	}
	account.Alias = trimSpace(alias)
	return m.Store.UpdateMetadata(account)
}

func (m *Module) requireAccount(id string) (ManagedAccount, error) {
	index, err := m.Store.LoadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	account, ok := index.Account(id)
	if !ok {
		return ManagedAccount{}, &AccountNotFoundError{ID: id}
	}
	return account, nil
}

// officialFor loads the official login and checks that it is account id's.
func (m *Module) officialFor(ctx context.Context, id string) ([]byte, Credential, error) {
	data, err := m.Store.Official.Load(ctx)
	if err != nil {
		return nil, Credential{}, err
	}
	cred, err := ParseCredential(data)
	if err != nil {
		return nil, Credential{}, ErrOfficialCredentialMalformed
	}
	managedData, err := m.Store.LoadCredentialData(id)
	if err != nil {
		return nil, Credential{}, err
	}
	managed, err := ParseCredential(managedData)
	if err != nil {
		return nil, Credential{}, err
	}
	account, err := m.requireAccount(id)
	if err != nil {
		return nil, Credential{}, err
	}
	if !sameLogin(cred, managed) && !identitiesMatch(cred.IDTokenIdentity(), account.Identity) {
		return nil, Credential{}, &CredentialIdentityMismatchError{ID: id}
	}
	return data, cred, nil
}

// syncOfficial writes refreshed tokens of the current account back to the
// official login, unless the official login changed meanwhile (then the
// client's own copy wins and the next reconcile picks it up).
func (m *Module) syncOfficial(ctx context.Context, id string, original, refreshed []byte) error {
	lock, err := platform.AcquireLock(m.Store.officialLockPath())
	if err != nil {
		if errors.Is(err, platform.ErrLocked) {
			return ErrOfficialLocked
		}
		return &AccountIOError{Message: "Unable to open the Antigravity credential lock.", Cause: err}
	}
	defer func() { _ = lock.Release() }()
	index, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	current, err := m.Store.Official.Load(ctx)
	if err != nil {
		return err
	}
	if index.CurrentAccountID != id || !bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(original)) {
		return nil
	}
	return m.Store.Official.Install(ctx, refreshed)
}
