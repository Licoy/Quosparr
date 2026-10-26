package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/platform"
)

// Persisted refresh error codes (ManagedAccount.LastError).
const (
	ErrorCodeCLIUnavailable        = "cli_unavailable"
	ErrorCodeAuthenticationNeeded  = "authentication_required"
	ErrorCodeTimeout               = "timeout"
	ErrorCodeBillingParseFailed    = "billing_parse_failed"
	ErrorCodeRefreshFailed         = "refresh_failed"
	ErrorCodeCancelled             = "cancelled"
	failureImportFailed            = "import failed"
	failureNoManageableCredential  = "no manageable Grok OAuth credential"
	failureInvalidCredential       = "invalid Grok credential"
	failureCredentialIdentityError = "credential identity mismatch"
)

// BillingFunc reads the quota of the account whose Grok home is homeDir. It
// returns the snapshot as JSON (stored as the account's cached quota) and the
// time the reading was taken. The billing package supplies the real one.
type BillingFunc func(ctx context.Context, homeDir string) (json.RawMessage, time.Time, error)

// CommandKind selects what Module.Apply does.
type CommandKind int

const (
	// CommandLoginOAuth runs the device-code login into a pending home and
	// adds the account.
	CommandLoginOAuth CommandKind = iota
	// CommandImportOfficial adopts the official ~/.grok login.
	CommandImportOfficial
	// CommandMakeCurrent installs the account into the official auth file.
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
	// AllowWhileRunning switches even when Grok processes are running.
	AllowWhileRunning bool
}

// Module orchestrates the Grok account library: official-login reconciliation,
// token keep-alive, switching, login, import and removal. Operations are
// serialized; Load and Apply return the state after the operation.
//
// The official auth.json is written only by installCurrentAccount (an explicit
// switch or first-login bootstrap) and by the current account's token refresh;
// refreshing any other account touches its managed copy only.
type Module struct {
	Store *Store
	// Refresher renews OAuth tokens (zero value: production endpoint).
	Refresher TokenRefresher
	// Login runs the device-code login (zero value: production endpoint).
	Login OAuthLogin
	// Billing reads a quota snapshot. When nil, a refresh only keeps the
	// tokens fresh and reports an outcome without a snapshot.
	Billing BillingFunc
	// RunningProcessIDs lists Grok processes; nil inspects the system.
	RunningProcessIDs func(ctx context.Context) ([]int, error)
	// IsAuthFailure adds error kinds (for example a billing client's own
	// authentication error) to the ones that mean "sign in again".
	IsAuthFailure func(error) bool
	// ErrorCode maps an error to a persisted code; returning "" falls back to
	// the built-in mapping.
	ErrorCode func(error) string
	// AppVersion is stamped into exported packs.
	AppVersion string
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	once                  sync.Once
	sem                   chan struct{}
	pendingOfficialChange bool
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
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Module) release() { <-m.sem }

// Load reconciles with the official login and returns the state. A change of
// the official identity made outside Quosparr is reported once.
func (m *Module) Load(ctx context.Context) (AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return AccountState{}, err
	}
	defer m.release()
	return m.reconcileOfficialIdentity(true)
}

// Refresh keeps tokens fresh and reads the quota of the targeted accounts.
// Per-account failures are in the outcomes.
func (m *Module) Refresh(ctx context.Context, target RefreshTarget) RefreshReport {
	started := m.now()
	if err := m.acquire(ctx); err != nil {
		return RefreshReport{Target: target, StartedAt: started, FinishedAt: m.now()}
	}
	defer m.release()

	var outcomes []RefreshOutcome
	state, err := m.reconcileOfficialIdentity(false)
	if err != nil {
		id := refreshFailureAccountID(target)
		outcomes = []RefreshOutcome{{AccountID: id, Err: m.accountError(err, id)}}
	} else {
		m.keepAllTokensFresh(ctx, state)
		for _, id := range refreshAccountIDs(target, state) {
			if ctx.Err() != nil {
				break
			}
			outcomes = append(outcomes, m.refreshAccount(ctx, id))
		}
	}
	return RefreshReport{Target: target, Outcomes: outcomes, StartedAt: started, FinishedAt: m.now()}
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
	case CommandLoginOAuth:
		err, acknowledges = m.loginOAuth(ctx), true
	case CommandImportOfficial:
		err, acknowledges = m.importOfficial(), true
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
	state, err := m.reconcileOfficialIdentity(false)
	if err != nil {
		return AccountState{}, err
	}
	if acknowledges {
		m.pendingOfficialChange = false
		state.OfficialIdentityChangedExternally = false
	}
	return state, nil
}

// PreviewImportFiles previews packs and raw credential files without writing
// the library.
func (m *Module) PreviewImportFiles(ctx context.Context, paths []string) (ImportPreview, error) {
	if err := m.acquire(ctx); err != nil {
		return ImportPreview{}, err
	}
	defer m.release()
	return TransferImporter{Store: m.Store, Now: m.Now}.PreviewFiles(paths), nil
}

// ImportPreviewSelection commits previously previewed candidates (library
// only, like the Swift app: it never writes the official file).
func (m *Module) ImportPreviewSelection(
	ctx context.Context, candidates []ImportCandidate, selectedIDs []string,
) (ImportBatchResult, AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return ImportBatchResult{}, AccountState{}, err
	}
	defer m.release()

	importer := TransferImporter{Store: m.Store, Now: m.Now}
	batch, err := importer.ImportPreviewSelection(candidates, selectedIDs, true)
	if err != nil {
		return ImportBatchResult{}, AccountState{}, err
	}
	state, err := m.reconcileOfficialIdentity(false)
	if err != nil {
		return batch, AccountState{}, err
	}
	if len(batch.Succeeded) > 0 {
		m.pendingOfficialChange = false
		state.OfficialIdentityChangedExternally = false
	}
	return batch, state, nil
}

// ExportAccounts builds an in-memory pack of the selected accounts.
func (m *Module) ExportAccounts(ids []string) (codex.ExportResult, error) {
	return Exporter{Store: m.Store, AppVersion: m.AppVersion}.Export(ids, m.now())
}

// ImportPastedText imports pasted auth.json / credential JSON. Partial
// success is reported in the batch result.
func (m *Module) ImportPastedText(ctx context.Context, text string) (ImportBatchResult, AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return ImportBatchResult{}, AccountState{}, err
	}
	defer m.release()

	payloads := ParsePayloads(text)
	if len(payloads) == 0 {
		state, err := m.reconcileOfficialIdentity(false)
		return ImportBatchResult{Failures: []string{FailureNoCredentials}, FailureErrors: []error{ErrInvalidCredential}}, state, err
	}
	before, err := m.Store.LoadIndex()
	if err != nil {
		return ImportBatchResult{}, AccountState{}, err
	}
	official := m.Store.LoadOfficialCredentialStatus(m.now())
	canBootstrap := len(before.Accounts) == 0 && !official.HasManagedLogin()

	var result ImportBatchResult
	isFirst := true
	for i, data := range payloads {
		if err := ctx.Err(); err != nil {
			return result, AccountState{}, err
		}
		label := "json"
		if len(payloads) != 1 {
			label = "json[" + strconv.Itoa(i) + "]"
		}
		account, err := m.importPayload(data, before, official, isFirst && canBootstrap)
		if err != nil {
			result.Failures = append(result.Failures, label+": "+safeImportFailureMessage(err))
			result.FailureErrors = append(result.FailureErrors, err)
			continue
		}
		result.Succeeded = append(result.Succeeded, account)
		isFirst = false
	}
	state, err := m.reconcileOfficialIdentity(false)
	if err != nil {
		return result, AccountState{}, err
	}
	if len(result.Succeeded) > 0 {
		m.pendingOfficialChange = false
		state.OfficialIdentityChangedExternally = false
	}
	return result, state, nil
}

func (m *Module) importPayload(
	data []byte, before AccountIndex, official OfficialCredentialStatus, bootstrapping bool,
) (ManagedAccount, error) {
	doc, err := parseCredential(data)
	if err != nil {
		return ManagedAccount{}, err
	}
	account, err := m.Store.UpsertCredentialData(data, UpsertOptions{MakeCurrent: bootstrapping, Now: m.now()})
	if err != nil {
		return ManagedAccount{}, err
	}
	// Keep the official login in sync when bootstrapping, re-importing the
	// current account, or pasting the identity already signed in officially.
	if shouldInstallPasted(account.ID, doc, before, official, bootstrapping) {
		if err := m.installCurrentAccount(account.ID); err != nil {
			return ManagedAccount{}, err
		}
	}
	return account, nil
}

func shouldInstallPasted(
	accountID string, doc AuthDocument, before AccountIndex, official OfficialCredentialStatus, bootstrapping bool,
) bool {
	if bootstrapping || before.CurrentAccountID == accountID {
		return true
	}
	if official.Identity != nil {
		id := official.Identity.StableID()
		return id == accountID || id == doc.StableID()
	}
	return false
}

func safeImportFailureMessage(err error) string {
	switch {
	case errors.Is(err, ErrNoManagedCredential), errors.Is(err, ErrAPIKeyOnlyNotManageable):
		return failureNoManageableCredential
	case errors.Is(err, ErrInvalidCredential), errors.Is(err, ErrOfficialCredentialMalformed):
		return failureInvalidCredential
	}
	var mismatch *CredentialIdentityMismatchError
	if errors.As(err, &mismatch) {
		return failureCredentialIdentityError
	}
	return failureImportFailed
}

// reconcileOfficialIdentity makes the library follow the official login: the
// official identity becomes (or is added as) the current account.
func (m *Module) reconcileOfficialIdentity(consumingExternalChange bool) (AccountState, error) {
	previous, err := m.Store.LoadIndex()
	if err != nil {
		return AccountState{}, err
	}
	status := m.Store.LoadOfficialCredentialStatus(m.now())
	currentID := ""
	switch status.Kind {
	case OfficialAuthenticated, OfficialRequiresReauthentication:
		data, err := m.Store.LoadOfficialCredentialData()
		if err != nil {
			return AccountState{}, err
		}
		account, err := m.Store.UpsertCredentialData(data, UpsertOptions{
			MakeCurrent: true, PreserveRefreshFailure: true, Now: m.now(),
		})
		if err != nil {
			return AccountState{}, err
		}
		currentID = account.ID
	default:
		if err := m.Store.SetCurrentAccountID("", m.now()); err != nil {
			return AccountState{}, err
		}
	}
	index, err := m.Store.LoadIndex()
	if err != nil {
		return AccountState{}, err
	}
	changed := previous.CurrentAccountID != "" && previous.CurrentAccountID != currentID
	if changed {
		m.pendingOfficialChange = true
	}
	report := changed || (consumingExternalChange && m.pendingOfficialChange)
	if consumingExternalChange {
		m.pendingOfficialChange = false
	}
	return AccountState{
		Official:                          status,
		CurrentAccountID:                  currentID,
		Accounts:                          index.OrderedAccounts(),
		OfficialIdentityChangedExternally: report,
	}, nil
}

func refreshAccountIDs(target RefreshTarget, state AccountState) []string {
	switch target.Kind {
	case RefreshCurrent:
		if state.CurrentAccountID != "" {
			return []string{state.CurrentAccountID}
		}
		return nil
	case RefreshAccount:
		return []string{target.ID}
	}
	ids := make([]string, 0, len(state.Accounts))
	if state.CurrentAccountID != "" {
		for _, a := range state.Accounts {
			if a.ID == state.CurrentAccountID {
				ids = append(ids, a.ID)
			}
		}
	}
	for _, a := range state.Accounts {
		if a.ID != state.CurrentAccountID {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func refreshFailureAccountID(target RefreshTarget) string {
	switch target.Kind {
	case RefreshCurrent:
		return "current"
	case RefreshAccount:
		return target.ID
	}
	return "all"
}

// keepAllTokensFresh renews the OAuth tokens of every managed account, so
// accounts that are not current do not expire while idle. Failures do not stop
// the pass; an authentication failure flags the account for re-sign-in.
func (m *Module) keepAllTokensFresh(ctx context.Context, state AccountState) {
	for _, account := range state.Accounts {
		if ctx.Err() != nil {
			return
		}
		_, err := m.ensureValidCredential(ctx, account.ID, account.ID == state.CurrentAccountID)
		if err == nil || !m.isAuthFailure(err) {
			continue
		}
		if updated, err2 := m.requireAccount(account.ID); err2 == nil {
			_ = m.Store.UpdateMetadata(updated.ApplyError(m.persistedErrorCode(err), true))
		}
	}
}

// ensureValidCredential makes sure the account's credential holds a fresh
// access token. The current account refreshes the official file and mirrors it
// into its managed copy; any other account refreshes its managed copy only and
// never touches the official file.
func (m *Module) ensureValidCredential(ctx context.Context, id string, isCurrent bool) ([]byte, error) {
	data, err := m.loadCredentialFor(id, isCurrent)
	if err != nil {
		return nil, err
	}
	result, err := m.Refresher.EnsureFresh(ctx, data, m.now())
	if err != nil {
		return nil, err
	}
	if !result.DidRefresh {
		return result.Data, nil
	}
	if err := m.persistRefreshed(id, isCurrent, data, result.Data); err != nil {
		return nil, err
	}
	if account, err := m.requireAccount(id); err == nil {
		account.Identity = result.Document.Identity
		if account.RequiresReauth || account.LastError != "" {
			account.RequiresReauth = result.Document.RequiresReauthentication(m.now())
			if !account.RequiresReauth {
				account.LastError = ""
			}
		}
		_ = m.Store.UpdateMetadata(account)
	}
	return result.Data, nil
}

func (m *Module) loadCredentialFor(id string, isCurrent bool) ([]byte, error) {
	if isCurrent {
		data, err := m.Store.LoadOfficialCredentialData()
		if err != nil {
			return nil, err
		}
		doc, err := parseCredential(data)
		if err != nil {
			return nil, err
		}
		matches, err := m.matchesStoredAccount(doc.Identity, id)
		if err != nil {
			return nil, err
		}
		if !matches && doc.StableID() != id {
			return nil, &CredentialIdentityMismatchError{ID: id}
		}
		return data, nil
	}
	return m.Store.LoadCredentialData(id)
}

// persistRefreshed keeps rotated tokens in the managed copy even if the CLI
// logged out, switched, or refreshed during the network request. The two byte
// snapshots plus account/target are needed for the compare-before-write check.
func (m *Module) persistRefreshed(id string, isCurrent bool, original, data []byte) error {
	if err := m.Store.SaveCredentialData(id, data); err != nil {
		return err
	}
	if !isCurrent {
		return nil
	}
	lock, err := platform.AcquireLock(m.Store.OfficialAuthPath() + ".lock")
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	index, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	current, err := m.Store.LoadOfficialCredentialData()
	if err != nil {
		return err
	}
	if index.CurrentAccountID != id || !bytes.Equal(current, original) {
		return &CredentialIdentityMismatchError{ID: id}
	}
	return m.Store.SaveOfficialAuthDataAtomically(data)
}

func (m *Module) refreshAccount(ctx context.Context, id string) RefreshOutcome {
	index, err := m.Store.LoadIndex()
	if err != nil {
		return RefreshOutcome{AccountID: id, Err: m.accountError(err, id)}
	}
	account, ok := index.Account(id)
	if !ok {
		return RefreshOutcome{AccountID: id, Err: &AccountNotFoundError{ID: id}}
	}
	isCurrent := id == index.CurrentAccountID
	home := m.Store.AccountDir(id)
	if isCurrent {
		home = m.Store.OfficialHome()
	}

	// A fresh access token right before billing; a no-op when still valid.
	if _, err := m.ensureValidCredential(ctx, id, isCurrent); err != nil {
		return m.refreshFailureOutcome(err, account)
	}
	var original []byte
	if !isCurrent {
		if original, err = m.Store.LoadCredentialData(id); err != nil {
			return m.refreshFailureOutcome(err, account)
		}
	}

	snapshot, takenAt, billingErr := m.readBilling(ctx, id, isCurrent, account, home)

	var finalizeErr error
	if original != nil {
		finalizeErr, err = m.finalizeManagedCredential(id, original)
		if err != nil {
			return m.refreshFailureOutcome(err, account)
		}
	} else if err := m.mirrorOfficialCredential(id); err != nil {
		finalizeErr = err
	}

	if billingErr != nil {
		return m.refreshFailureOutcome(billingErr, account)
	}
	if finalizeErr != nil {
		return m.refreshFailureOutcome(finalizeErr, account)
	}
	if err := ctx.Err(); err != nil {
		return m.refreshFailureOutcome(err, account)
	}
	updated, err := m.requireAccount(id)
	if err == nil && snapshot != nil {
		err = m.Store.UpdateMetadata(updated.ApplyQuota(snapshot, takenAt))
	}
	if err != nil {
		return m.refreshFailureOutcome(err, account)
	}
	return RefreshOutcome{AccountID: id, Snapshot: snapshot}
}

// readBilling reads the quota, with one retry after a forced token refresh
// when the API rejects the access token.
func (m *Module) readBilling(
	ctx context.Context, id string, isCurrent bool, account ManagedAccount, home string,
) (json.RawMessage, time.Time, error) {
	if m.Billing == nil {
		return nil, time.Time{}, nil
	}
	snapshot, takenAt, err := m.Billing(ctx, home)
	if err == nil || !m.isAuthFailure(err) || !account.Identity.HasRefreshToken {
		return snapshot, takenAt, err
	}
	data, err := m.loadCredentialFor(id, isCurrent)
	if err != nil {
		return nil, time.Time{}, err
	}
	forced, err := m.Refresher.Refresh(ctx, data, m.now())
	if err != nil {
		return nil, time.Time{}, err
	}
	if err := m.persistRefreshed(id, isCurrent, data, forced); err != nil {
		return nil, time.Time{}, err
	}
	return m.Billing(ctx, home)
}

// finalizeManagedCredential checks that the managed copy still belongs to the
// account after billing; when not, the original bytes are restored and the
// validation error is returned as first value. The second value is a failure
// to restore.
func (m *Module) finalizeManagedCredential(id string, original []byte) (validation, fatal error) {
	validation = m.validateManagedCredential(id)
	if validation == nil {
		return nil, nil
	}
	if err := m.Store.SaveCredentialData(id, original); err != nil {
		return nil, &PartialWriteError{Message: "Managed Grok credential rollback could not be verified."}
	}
	if current, err := m.Store.LoadCredentialData(id); err != nil || !bytes.Equal(current, original) {
		return nil, &PartialWriteError{Message: "Managed Grok credential rollback could not be verified."}
	}
	return validation, nil
}

func (m *Module) refreshFailureOutcome(err error, account ManagedAccount) RefreshOutcome {
	failure := m.accountError(err, account.ID)
	retained := len(account.CachedQuota) > 0
	var partial *PartialWriteError
	if errors.As(failure, &partial) {
		return RefreshOutcome{AccountID: account.ID, Err: failure, RetainedStaleSnapshot: retained}
	}
	updated, lookupErr := m.requireAccount(account.ID)
	if lookupErr == nil {
		lookupErr = m.Store.UpdateMetadata(updated.ApplyError(m.persistedErrorCode(err), m.isAuthFailure(err)))
	}
	if lookupErr != nil {
		return RefreshOutcome{AccountID: account.ID, Err: m.accountError(lookupErr, account.ID), RetainedStaleSnapshot: retained}
	}
	return RefreshOutcome{AccountID: account.ID, Err: failure, RetainedStaleSnapshot: retained}
}

// mirrorOfficialCredential copies the official credential into the current
// account's managed copy.
func (m *Module) mirrorOfficialCredential(accountID string) error {
	data, err := m.Store.LoadOfficialCredentialData()
	if err != nil {
		return err
	}
	doc, err := parseCredential(data)
	if err != nil {
		return err
	}
	matches, err := m.matchesStoredAccount(doc.Identity, accountID)
	if err != nil {
		return err
	}
	if doc.StableID() != accountID && !matches {
		_, _ = m.reconcileOfficialIdentity(false)
		return &CredentialIdentityMismatchError{ID: accountID}
	}
	return m.Store.SaveCredentialData(accountID, data)
}

func (m *Module) validateManagedCredential(id string) error {
	data, err := m.Store.LoadCredentialData(id)
	if err != nil {
		return err
	}
	doc, err := parseCredential(data)
	if err != nil {
		return err
	}
	matches, err := m.matchesStoredAccount(doc.Identity, id)
	if err != nil {
		return err
	}
	if doc.StableID() != id && !matches {
		return &CredentialIdentityMismatchError{ID: id}
	}
	return m.Store.SaveCredentialData(id, data)
}

// matchesStoredAccount compares a credential identity with the identity the
// library recorded for the account: same stable id, else same principal (or
// user) with the same team.
func (m *Module) matchesStoredAccount(identity CredentialIdentity, accountID string) (bool, error) {
	account, err := m.requireAccount(accountID)
	if err != nil {
		return false, err
	}
	stored := account.Identity
	if stored.StableID() == identity.StableID() {
		return true, nil
	}
	if l, r := stored.PrincipalID, identity.PrincipalID; l != "" && r != "" {
		return strings.EqualFold(l, r) && teamsMatchStrict(stored.TeamID, identity.TeamID), nil
	}
	if l, r := stored.UserID, identity.UserID; l != "" && r != "" {
		return strings.EqualFold(l, r) && teamsMatchStrict(stored.TeamID, identity.TeamID), nil
	}
	return false, nil
}

func teamsMatchStrict(l, r string) bool {
	switch {
	case l != "" && r != "":
		return strings.EqualFold(l, r)
	case l == "" && r == "":
		return true
	}
	return false
}

func (m *Module) loginOAuth(ctx context.Context) error {
	pendingID := "login-" + randomToken()
	pendingHome, err := m.Store.EnsurePendingHome(pendingID)
	if err != nil {
		return err
	}
	if err := m.runLogin(ctx, pendingHome); err != nil {
		_ = os.RemoveAll(pendingHome)
		return err
	}
	if err := os.RemoveAll(pendingHome); err != nil && !errors.Is(err, os.ErrNotExist) {
		return &AccountIOError{Message: "Unable to clean up the pending Grok login directory.", Cause: err}
	}
	return nil
}

func (m *Module) runLogin(ctx context.Context, pendingHome string) error {
	if err := m.Login.Login(ctx, pendingHome); err != nil {
		return loginError(ctx, err)
	}
	if ctx.Err() != nil {
		return ErrLoginCancelled
	}
	data, err := os.ReadFile(pendingHome + string(os.PathSeparator) + authFileName)
	if err != nil {
		return &LoginFailedError{Message: "Grok OAuth login failed.", Cause: err}
	}
	if _, err := parseCredential(data); err != nil {
		return err
	}
	before, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	official := m.Store.LoadOfficialCredentialStatus(m.now())
	account, err := m.Store.UpsertCredentialData(data, UpsertOptions{Now: m.now()})
	if err != nil {
		return err
	}
	if len(before.Accounts) == 0 && !official.HasManagedLogin() {
		return m.installCurrentAccount(account.ID)
	}
	return nil
}

func loginError(ctx context.Context, err error) error {
	var oauth *OAuthError
	switch {
	case errors.Is(err, context.Canceled) || ctx.Err() != nil:
		return ErrLoginCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return &LoginFailedError{Message: "Grok OAuth login timed out.", Cause: err}
	case errors.As(err, &oauth):
		return &LoginFailedError{Message: safeLoginMessage(oauth), Cause: err}
	}
	return &LoginFailedError{Message: "Grok OAuth login failed.", Cause: err}
}

func safeLoginMessage(err *OAuthError) string {
	switch err.Kind {
	case OAuthExpired:
		return "Grok device code expired. Try signing in again."
	case OAuthDenied:
		return "Grok device authorization was denied."
	case OAuthBrowserOpenFailed:
		return "Could not open the browser for Grok sign-in."
	case OAuthMissingIdentity:
		return "Grok sign-in completed but no user identity was returned."
	}
	return "Grok OAuth login failed."
}

func (m *Module) importOfficial() error {
	status := m.Store.LoadOfficialCredentialStatus(m.now())
	switch status.Kind {
	case OfficialAuthenticated, OfficialRequiresReauthentication:
		data, err := m.Store.LoadOfficialCredentialData()
		if err != nil {
			return err
		}
		_, err = m.Store.UpsertCredentialData(data, UpsertOptions{MakeCurrent: true, Now: m.now()})
		return err
	case OfficialMissing:
		return ErrOfficialCredentialMissing
	case OfficialAPIKeyOnly:
		return ErrAPIKeyOnlyNotManageable
	}
	return ErrOfficialCredentialMalformed
}

func (m *Module) makeCurrent(ctx context.Context, id string, allowWhileRunning bool) error {
	if _, err := m.requireAccount(id); err != nil {
		return err
	}
	if !allowWhileRunning {
		inspect := m.RunningProcessIDs
		if inspect == nil {
			inspect = systemGrokProcessIDs
		}
		pids, err := inspect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &AccountIOError{Message: "Unable to inspect running Grok processes.", Cause: err}
		}
		if len(pids) > 0 {
			return &ProcessesRunningError{PIDs: pids}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Renew the access token on the managed copy before installing it.
	if _, err := m.ensureValidCredential(ctx, id, false); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.installCurrentAccount(id)
}

// installCurrentAccount makes id the official login: the managed scopes of
// the official auth.json are replaced by the account's credential, other
// scopes (API keys, custom providers) stay. A failure rolls the official file
// and the index back. This and the current account's token refresh are the
// only writers of the official file.
func (m *Module) installCurrentAccount(id string) error {
	targetData, err := m.Store.LoadCredentialData(id)
	if err != nil {
		return err
	}
	target, err := parseCredential(targetData)
	if err != nil {
		return err
	}
	account, err := m.requireAccount(id)
	if err != nil {
		return err
	}
	if target.StableID() != id && account.Identity.StableID() != target.Identity.StableID() {
		return &CredentialIdentityMismatchError{ID: id}
	}

	lock, err := platform.AcquireLock(m.Store.OfficialAuthPath() + ".lock")
	if err != nil {
		if errors.Is(err, platform.ErrLocked) {
			return ErrAuthFileLocked
		}
		return &AccountIOError{Message: "Unable to open the Grok credential lock.", Cause: err}
	}
	defer func() { _ = lock.Release() }()
	return m.installLocked(id, target, targetData)
}

func (m *Module) installLocked(id string, target AuthDocument, targetData []byte) error {
	originalIndex, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	official, err := m.Store.LoadOfficialCredentialData()
	if errors.Is(err, ErrOfficialCredentialMissing) {
		official, err = nil, nil
	}
	if err != nil {
		return err
	}
	if official != nil {
		// Keep the identity being replaced in the library. An API-key-only
		// official file is preserved by the scope merge below.
		if _, err := m.Store.UpsertCredentialData(official, UpsertOptions{Now: m.now()}); err != nil &&
			!errors.Is(err, ErrNoManagedCredential) {
			return err
		}
	}
	merged, err := ReplaceManagedScopes(official, targetData)
	if err != nil {
		return err
	}
	if err := m.switchOfficial(id, target, merged); err != nil {
		if rbErr := m.rollbackSwitch(official, originalIndex); rbErr != nil {
			return &PartialWriteError{Message: "Official Grok credential rollback could not be verified."}
		}
		if isAccountError(err) {
			return err
		}
		return &AccountIOError{Message: "Unable to switch the official Grok credential.", Cause: err}
	}
	return nil
}

func (m *Module) switchOfficial(id string, target AuthDocument, merged []byte) error {
	if err := m.Store.SaveOfficialAuthDataAtomically(merged); err != nil {
		return err
	}
	installedData, err := m.Store.LoadOfficialCredentialData()
	if err != nil {
		return err
	}
	installed, err := parseCredential(installedData)
	if err != nil {
		return err
	}
	if installed.StableID() != target.StableID() {
		return &AccountIOError{Message: "Official Grok credential verification failed."}
	}
	return m.Store.SetCurrentAccountID(id, m.now())
}

func (m *Module) rollbackSwitch(official []byte, index AccountIndex) error {
	if err := m.Store.restoreOfficialAuthData(official, official != nil); err != nil {
		return err
	}
	if current, err := m.Store.LoadIndex(); err != nil || !indexesEqual(current, index) {
		if err := m.Store.SaveIndex(index); err != nil {
			return err
		}
	}
	if official != nil {
		if current, err := m.Store.LoadOfficialCredentialData(); err != nil || !bytes.Equal(current, official) {
			return &AccountIOError{Message: "Official Grok credential rollback verification failed.", Cause: err}
		}
	} else if _, err := os.Stat(m.Store.OfficialAuthPath()); err == nil {
		return &AccountIOError{Message: "Official Grok credential removal could not be verified."}
	}
	current, err := m.Store.LoadIndex()
	if err != nil || !indexesEqual(current, index) {
		return &AccountIOError{Message: "Grok account index rollback verification failed.", Cause: err}
	}
	return nil
}

func (m *Module) setAlias(id, alias string) error {
	account, err := m.requireAccount(id)
	if err != nil {
		return err
	}
	account.Alias = strings.TrimFunc(alias, unicode.IsSpace)
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

// isAccountError reports whether err is one of the account errors that is
// passed to callers unchanged (everything else becomes a refresh failure).
func isAccountError(err error) bool {
	for _, sentinel := range []error{
		ErrOfficialCredentialMissing, ErrOfficialCredentialMalformed, ErrAPIKeyOnlyNotManageable,
		ErrNoManagedCredential, ErrAuthFileLocked, ErrLoginCancelled, ErrInvalidCredential,
	} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	var (
		notFound   *AccountNotFoundError
		missing    *CredentialMissingError
		mismatch   *CredentialIdentityMismatchError
		current    *CurrentAccountCannotBeRemovedError
		running    *ProcessesRunningError
		version    *UnsupportedIndexVersionError
		login      *LoginFailedError
		refresh    *RefreshFailedError
		tooOld     *CLITooOldError
		ioErr      *AccountIOError
		partialErr *PartialWriteError
	)
	return errors.As(err, &notFound) || errors.As(err, &missing) || errors.As(err, &mismatch) ||
		errors.As(err, &current) || errors.As(err, &running) || errors.As(err, &version) ||
		errors.As(err, &login) || errors.As(err, &refresh) || errors.As(err, &tooOld) ||
		errors.As(err, &ioErr) || errors.As(err, &partialErr)
}

// accountError turns any error into an account error for a refresh outcome.
func (m *Module) accountError(err error, accountID string) error {
	if isAccountError(err) {
		return err
	}
	if strings.Contains(strings.ToLower(err.Error()), "method not found") {
		return &CLITooOldError{Version: "unknown"}
	}
	return &RefreshFailedError{AccountID: accountID, Message: m.persistedErrorCode(err), Cause: err}
}

// isAuthFailure reports whether err means the user has to sign in again.
func (m *Module) isAuthFailure(err error) bool {
	if errors.Is(err, ErrAuthenticationRequired) {
		return true
	}
	var malformed *MalformedResponseError
	if errors.As(err, &malformed) && strings.Contains(strings.ToLower(malformed.Message), "token") {
		return true
	}
	return m.IsAuthFailure != nil && m.IsAuthFailure(err)
}

// persistedErrorCode is the short code stored in ManagedAccount.LastError.
func (m *Module) persistedErrorCode(err error) string {
	if m.ErrorCode != nil {
		if code := m.ErrorCode(err); code != "" {
			return code
		}
	}
	var (
		malformed *MalformedResponseError
		timeout   *AccountTimeoutError
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
	)
	switch {
	case errors.Is(err, ErrCLINotInstalled):
		return ErrorCodeCLIUnavailable
	case errors.Is(err, ErrAuthenticationRequired):
		return ErrorCodeAuthenticationNeeded
	case errors.Is(err, context.Canceled):
		return ErrorCodeCancelled
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout):
		return ErrorCodeTimeout
	case errors.As(err, &malformed):
		lower := strings.ToLower(malformed.Message)
		switch {
		case strings.Contains(lower, "token"):
			return ErrorCodeAuthenticationNeeded
		case strings.Contains(lower, "billing"):
			return ErrorCodeBillingParseFailed
		}
		return ErrorCodeRefreshFailed
	case errors.As(err, &syntaxErr), errors.As(err, &typeErr):
		return ErrorCodeBillingParseFailed
	}
	return ErrorCodeRefreshFailed
}
