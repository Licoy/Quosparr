package codexapi

import (
	"context"
	"errors"
	"sync"
)

// WorkspaceMetadataUnavailable is the warning of a refresh whose quota
// succeeded but whose workspace name could not be read.
const WorkspaceMetadataUnavailable = "workspace_metadata_unavailable"

// AccountTarget is what the refresher needs to know about a managed account.
// The account storage layer builds the list; this package never reads it.
type AccountTarget struct {
	ID string
	// APIKey accounts have no ChatGPT quota; they are reported without a request.
	APIKey bool
	// HasWorkspaceName is true when the workspace name is already known.
	HasWorkspaceName bool
	// Tier is the last known subscription tier, used to decide whether the
	// workspace name is worth fetching.
	Tier SubscriptionTier
}

// QuotaRefreshResult is the outcome for one account. It identifies the account
// and carries the refresh generation, so the consumer can drop a stale result
// (Generations.Valid). The refresher persists nothing; the consumer applies
// Quota/WorkspaceName to the account record and the possibly refreshed Auth to
// its credential copy.
type QuotaRefreshResult struct {
	AccountID  string
	Generation uint64
	APIKey     bool
	// Auth is the credential used for the requests (after any token refresh).
	Auth            Auth
	Quota           *QuotaSnapshot
	ResetCredits    *ResetCreditsSnapshot
	ResetCreditsErr error
	WorkspaceName   string
	// Warning is set when the quota is valid but metadata failed.
	Warning string
	Err     error
	// NeedsReauth is true when Err means the account must sign in again.
	NeedsReauth bool
}

// ErrorMessage is the text stored as the account's last error: Err's message,
// else the warning, else "".
func (r QuotaRefreshResult) ErrorMessage() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return r.Warning
}

// AccountQuotaRefresher refreshes the quota of managed OAuth accounts with
// bounded concurrency.
type AccountQuotaRefresher struct {
	Client QuotaClient
	// Accounts lists the accounts, active one first (sidebar order).
	Accounts func(ctx context.Context) ([]AccountTarget, error)
	// Credential returns a valid credential for the account, refreshing and
	// persisting its tokens first when they are expired (ensureValidCredential).
	Credential func(ctx context.Context, accountID string) (Auth, error)
	// Generations is optional; when set each account refresh gets a generation.
	Generations *Generations
	// MaxConcurrent defaults to 3.
	MaxConcurrent int
}

// ErrAccountNotFound is the result error for an unknown account id.
var ErrAccountNotFound = errors.New("account not found")

// RefreshAll refreshes every account and returns the results in list order.
// Cancelling ctx stops the remaining work; accounts that never ran carry the
// context error. skipResetCreditsID skips only the credit request already
// performed by a current-account refresh; an empty id refreshes every account.
func (r AccountQuotaRefresher) RefreshAll(ctx context.Context, skipResetCreditsID string) []QuotaRefreshResult {
	targets, err := r.Accounts(ctx)
	if err != nil {
		return nil
	}
	results := make([]QuotaRefreshResult, len(targets))
	limit := r.MaxConcurrent
	if limit < 1 {
		limit = 3
	}
	slots := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i, target := range targets {
		i, target := i, target
		gen := r.begin(target.ID)
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				results[i] = QuotaRefreshResult{AccountID: target.ID, Generation: gen, Err: ctx.Err()}
				return
			}
			results[i] = r.refreshOne(ctx, target, gen, target.ID == skipResetCreditsID)
		}()
	}
	wg.Wait()
	return results
}

// Refresh refreshes one account.
func (r AccountQuotaRefresher) Refresh(ctx context.Context, accountID string) QuotaRefreshResult {
	targets, err := r.Accounts(ctx)
	if err != nil {
		return QuotaRefreshResult{AccountID: accountID, Err: err}
	}
	for _, target := range targets {
		if target.ID == accountID {
			return r.refreshOne(ctx, target, r.begin(accountID), false)
		}
	}
	return QuotaRefreshResult{AccountID: accountID, Err: ErrAccountNotFound}
}

func (r AccountQuotaRefresher) begin(accountID string) uint64 {
	if r.Generations == nil {
		return 0
	}
	return r.Generations.Begin(accountID)
}

func (r AccountQuotaRefresher) refreshOne(ctx context.Context, target AccountTarget, gen uint64, skipResetCredits bool) QuotaRefreshResult {
	result := QuotaRefreshResult{AccountID: target.ID, Generation: gen}
	if target.APIKey {
		// The ChatGPT quota endpoints do not apply to pure API-key accounts.
		result.APIKey = true
		return result
	}
	auth, err := r.Credential(ctx, target.ID)
	if err != nil {
		return failed(result, err)
	}
	result.Auth = auth
	quota, err := r.Client.FetchQuota(ctx, auth)
	if err != nil {
		return failed(result, err)
	}
	result.Quota = &quota
	if !skipResetCredits {
		credits, creditsErr := r.Client.FetchResetCredits(ctx, auth)
		result.ResetCreditsErr = creditsErr
		if creditsErr == nil {
			result.ResetCredits = &credits
		}
	}
	if shouldFetchWorkspaceName(target) {
		// Best effort: a failure must not discard the quota.
		if name, err := r.Client.FetchWorkspaceName(ctx, auth); err != nil {
			result.Warning = WorkspaceMetadataUnavailable
		} else {
			result.WorkspaceName = name
		}
	}
	return result
}

func failed(result QuotaRefreshResult, err error) QuotaRefreshResult {
	result.Err = err
	result.NeedsReauth = NeedsReauth(err)
	return result
}

func shouldFetchWorkspaceName(target AccountTarget) bool {
	if target.HasWorkspaceName {
		return false
	}
	switch target.Tier {
	case TierTeam, TierBusiness, TierBusinessPremium, TierEnterprise, TierEdu:
		return true
	}
	return false
}
