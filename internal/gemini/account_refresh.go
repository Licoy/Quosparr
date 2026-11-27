package gemini

import (
	"context"
	"errors"
	"time"
)

// Refresh follows the official login and reads the quota of the targeted
// accounts under the read policy (see quotaGate). Per-account failures are
// in the outcomes.
func (m *Module) Refresh(ctx context.Context, target RefreshTarget) RefreshReport {
	started := m.now()
	if err := m.acquire(ctx); err != nil {
		return RefreshReport{Target: target, StartedAt: started, FinishedAt: m.now()}
	}
	defer m.release()
	var outcomes []RefreshOutcome
	state, err := m.reconcileOfficialIdentity(ctx, false)
	if err != nil {
		outcomes = []RefreshOutcome{{AccountID: refreshFailureAccountID(target), Err: err}}
	} else {
		for _, id := range refreshAccountIDs(target, state) {
			if ctx.Err() != nil {
				break
			}
			outcomes = append(outcomes, m.refreshAccount(ctx, id))
		}
	}
	return RefreshReport{Target: target, Outcomes: outcomes, StartedAt: started, FinishedAt: m.now()}
}

// Sync follows the official login without reading any quota.
func (m *Module) Sync(ctx context.Context) error {
	if err := m.acquire(ctx); err != nil {
		return err
	}
	defer m.release()
	_, err := m.reconcileOfficialIdentity(ctx, false)
	return err
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
	// "All" reads only the current account: reading every account in one
	// burst is the pattern most likely to get them flagged.
	if state.CurrentAccountID != "" {
		return []string{state.CurrentAccountID}
	}
	return nil
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

func (m *Module) quota() QuotaFunc {
	if m.Quota != nil {
		return m.Quota
	}
	client := &QuotaClient{Now: m.Now}
	return client.Fetch
}

func (m *Module) refreshAccount(ctx context.Context, id string) RefreshOutcome {
	index, err := m.Store.LoadIndex()
	if err != nil {
		return RefreshOutcome{AccountID: id, Err: err}
	}
	account, ok := index.Account(id)
	if !ok {
		return RefreshOutcome{AccountID: id, Err: &AccountNotFoundError{ID: id}}
	}
	retained := len(account.CachedQuota) > 0
	if err := m.quotaGate(ctx, account); err != nil {
		return RefreshOutcome{AccountID: id, Err: err, RetainedStaleSnapshot: retained}
	}
	m.throttle.Attempted(id, m.now())
	snapshot, err := m.readQuota(ctx, id, id == index.CurrentAccountID)
	if err != nil {
		return m.refreshFailure(ctx, id, err, retained)
	}
	raw, err := MarshalQuotaSnapshot(snapshot)
	if err == nil {
		var updated ManagedAccount
		if updated, err = m.requireAccount(id); err == nil {
			err = m.Store.UpdateMetadata(updated.ApplyQuota(raw, takenAt(snapshot, m.now())))
		}
	}
	if err != nil {
		return RefreshOutcome{AccountID: id, Err: err, RetainedStaleSnapshot: retained}
	}
	m.throttle.Succeeded(id)
	return RefreshOutcome{AccountID: id, Snapshot: raw}
}

// quotaGate is the read policy, checked before any request: reads are on,
// QuotaMinInterval has passed since the account's last attempt and its last
// stored reading, no 429 backoff holds, and an Antigravity client is
// running. A refused read stores no error and keeps the cached reading.
func (m *Module) quotaGate(ctx context.Context, account ManagedAccount) error {
	if m.QuotaEnabled == nil || !m.QuotaEnabled() {
		return ErrQuotaDisabled
	}
	now := m.now()
	if wait := m.throttle.Wait(account.ID, now); wait > 0 {
		return &RateLimitedError{RetryAfter: wait, HasRetryAfter: true}
	}
	wait := m.throttle.Cooldown(account.ID, now)
	if account.LastQuotaAt != nil {
		wait = max(wait, account.LastQuotaAt.Add(QuotaMinInterval).Sub(now))
	}
	if wait = min(wait, QuotaMinInterval); wait > 0 {
		return &QuotaCooldownError{RetryAfter: wait}
	}
	pids, err := m.runningPIDs(ctx)
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		return ErrClientNotRunning
	}
	return nil
}

// readQuota reads the account with a fresh access token; a 401 forces one
// token refresh and one retry, and a second 401 means signing in again.
func (m *Module) readQuota(ctx context.Context, id string, isCurrent bool) (QuotaSnapshot, error) {
	cred, err := m.freshCredential(ctx, id, isCurrent, false)
	if err != nil {
		return QuotaSnapshot{}, err
	}
	snapshot, err := m.quota()(ctx, cred.AccessToken)
	if !errors.Is(err, ErrUnauthorized) {
		return snapshot, err
	}
	if cred, err = m.freshCredential(ctx, id, isCurrent, true); err != nil {
		return QuotaSnapshot{}, err
	}
	snapshot, err = m.quota()(ctx, cred.AccessToken)
	if errors.Is(err, ErrUnauthorized) {
		return QuotaSnapshot{}, errors.Join(ErrAuthenticationRequired, err)
	}
	return snapshot, err
}

// freshCredential returns the account's credential with a valid access
// token. The current account reads and refreshes the official login and
// mirrors it into the managed copy; any other account refreshes its managed
// copy only and never touches the official login.
func (m *Module) freshCredential(ctx context.Context, id string, isCurrent, force bool) (Credential, error) {
	var data []byte
	var cred Credential
	var err error
	if isCurrent {
		data, cred, err = m.officialFor(ctx, id)
	} else if data, err = m.Store.LoadCredentialData(id); err == nil {
		cred, err = ParseCredential(data)
	}
	if err != nil {
		return Credential{}, err
	}
	if !force && !cred.NeedsRefresh(m.now(), RefreshSkew) {
		return cred, nil
	}
	refreshed, err := m.OAuth.Refresh(ctx, cred, m.now())
	if err != nil {
		return Credential{}, err
	}
	out, err := refreshed.Marshal()
	if err != nil {
		return Credential{}, err
	}
	if err := m.Store.SaveCredentialData(id, out); err != nil {
		return Credential{}, err
	}
	if isCurrent {
		if err := m.syncOfficial(ctx, id, data, out); err != nil {
			return Credential{}, err
		}
	}
	return refreshed, nil
}

// refreshFailure persists the error code (and, for a validation request,
// the link in the cached snapshot) and builds the outcome.
func (m *Module) refreshFailure(ctx context.Context, id string, err error, retained bool) RefreshOutcome {
	var (
		limited    *RateLimitedError
		validation *ValidationRequiredError
		partial    *PartialWriteError
	)
	if errors.As(err, &partial) || errors.Is(err, context.Canceled) && ctx.Err() != nil {
		return RefreshOutcome{AccountID: id, Err: err, RetainedStaleSnapshot: retained}
	}
	if errors.As(err, &limited) {
		m.throttle.ObserveRateLimit(id, m.now(), limited)
	}
	account, lookupErr := m.requireAccount(id)
	if lookupErr != nil {
		return RefreshOutcome{AccountID: id, Err: err, RetainedStaleSnapshot: retained}
	}
	updated := account.ApplyError(ErrorCode(err), requiresReauth(err))
	if errors.As(err, &validation) {
		if raw, mErr := withValidation(account.CachedQuota, Validation{Reason: validation.Reason, URL: validation.URL}, m.now()); mErr == nil {
			updated.CachedQuota = raw
			retained = true
		}
	}
	_ = m.Store.UpdateMetadata(updated)
	return RefreshOutcome{AccountID: id, Err: err, RetainedStaleSnapshot: retained}
}

func takenAt(s QuotaSnapshot, now time.Time) time.Time {
	if s.UpdatedAt.IsZero() {
		return now
	}
	return s.UpdatedAt
}
