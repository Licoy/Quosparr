package gemini

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// IndexVersion is the gemini-index.json schema version.
const IndexVersion = 1

// ManagedAccount is one row of gemini-index.json. It never holds tokens.
// CachedQuota is a QuotaSnapshot as JSON (see ParseQuotaSnapshot).
type ManagedAccount struct {
	ID             string          `json:"id"`
	SortIndex      int             `json:"sortIndex"`
	Identity       Identity        `json:"identity"`
	Alias          string          `json:"alias,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	LastUsedAt     *time.Time      `json:"lastUsedAt,omitempty"`
	LastQuotaAt    *time.Time      `json:"lastQuotaAt,omitempty"`
	RequiresReauth bool            `json:"requiresReauth"`
	LastError      string          `json:"lastError,omitempty"`
	CachedQuota    json.RawMessage `json:"cachedQuota,omitempty"`
}

// Email is the identity's email.
func (a ManagedAccount) Email() string { return a.Identity.Email }

// ResolvedDisplayName is the alias, else the identity's label, else the id.
func (a ManagedAccount) ResolvedDisplayName() string {
	return firstNonEmpty(a.Alias, a.Identity.ResolvedDisplayName(), a.ID)
}

// ApplyQuota caches a successful reading and clears any error.
func (a ManagedAccount) ApplyQuota(snapshot json.RawMessage, updatedAt time.Time) ManagedAccount {
	a.CachedQuota = append(json.RawMessage(nil), snapshot...)
	t := updatedAt
	a.LastQuotaAt = &t
	a.RequiresReauth = false
	a.LastError = ""
	return a
}

// ApplyError records a failure and keeps the cached quota.
func (a ManagedAccount) ApplyError(code string, requiresReauth bool) ManagedAccount {
	a.LastError = code
	a.RequiresReauth = requiresReauth
	return a
}

// AccountIndex is gemini-index.json.
type AccountIndex struct {
	Version          int              `json:"version"`
	CurrentAccountID string           `json:"currentAccountID,omitempty"`
	Accounts         []ManagedAccount `json:"accounts"`
}

// NewAccountIndex returns an empty index at the current version.
func NewAccountIndex() AccountIndex {
	return AccountIndex{Version: IndexVersion, Accounts: []ManagedAccount{}}
}

// ParseAccountIndex decodes gemini-index.json.
func ParseAccountIndex(data []byte) (AccountIndex, error) {
	var idx AccountIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return AccountIndex{}, err
	}
	seen := map[string]bool{}
	for _, a := range idx.Accounts {
		if trimSpace(a.ID) == "" || seen[a.ID] {
			return AccountIndex{}, errors.New("account ids must be unique and non-empty")
		}
		seen[a.ID] = true
	}
	if idx.Accounts == nil {
		idx.Accounts = []ManagedAccount{}
	}
	return idx, nil
}

// MarshalAccountIndex renders idx with two-space indentation.
func MarshalAccountIndex(idx AccountIndex) ([]byte, error) {
	if idx.Accounts == nil {
		idx.Accounts = []ManagedAccount{}
	}
	return json.MarshalIndent(idx, "", "  ")
}

// Account returns the row with id.
func (idx AccountIndex) Account(id string) (ManagedAccount, bool) {
	for _, a := range idx.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	return ManagedAccount{}, false
}

// OrderedAccounts sorts by sort index, then by display name (case-insensitive).
func (idx AccountIndex) OrderedAccounts() []ManagedAccount {
	out := append([]ManagedAccount(nil), idx.Accounts...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortIndex != out[j].SortIndex {
			return out[i].SortIndex < out[j].SortIndex
		}
		return strings.ToLower(out[i].ResolvedDisplayName()) < strings.ToLower(out[j].ResolvedDisplayName())
	})
	return out
}

// Upsert replaces the row with the same id, or appends it.
func (idx *AccountIndex) Upsert(acc ManagedAccount) {
	for i := range idx.Accounts {
		if idx.Accounts[i].ID == acc.ID {
			idx.Accounts[i] = acc
			return
		}
	}
	idx.Accounts = append(idx.Accounts, acc)
}

func (idx AccountIndex) matching(identity Identity) (ManagedAccount, bool) {
	for _, a := range idx.Accounts {
		if a.ID == identity.StableID() || identitiesMatch(a.Identity, identity) {
			return a, true
		}
	}
	return ManagedAccount{}, false
}

func (idx AccountIndex) nextSortIndex() int {
	next := 0
	for _, a := range idx.Accounts {
		next = max(next, a.SortIndex+1)
	}
	return next
}

// OfficialStatusKind classifies the official login.
type OfficialStatusKind int

const (
	OfficialMissing OfficialStatusKind = iota
	OfficialMalformed
	OfficialUnreadable
	// OfficialUnidentified: a login exists but its account could not be
	// determined (no matching managed copy, no id_token, userinfo failed).
	OfficialUnidentified
	OfficialAuthenticated
	OfficialRequiresReauthentication
)

// OfficialCredentialStatus is the state of the official login.
type OfficialCredentialStatus struct {
	Kind     OfficialStatusKind
	Identity *Identity
}

// HasLogin reports whether an official login exists in a usable form.
func (s OfficialCredentialStatus) HasLogin() bool {
	return s.Kind == OfficialAuthenticated || s.Kind == OfficialRequiresReauthentication || s.Kind == OfficialUnidentified
}

// AccountState is what the module reports after an operation.
type AccountState struct {
	Official         OfficialCredentialStatus
	CurrentAccountID string
	Accounts         []ManagedAccount
	// OfficialIdentityChangedExternally is true when the official login
	// changed outside Quosparr since the last time the user saw it.
	OfficialIdentityChangedExternally bool
}

// RefreshTargetKind selects which accounts a refresh covers.
type RefreshTargetKind int

const (
	RefreshCurrent RefreshTargetKind = iota
	RefreshAccount
	RefreshAll
)

// RefreshTarget is what to refresh. ID is used with RefreshAccount.
type RefreshTarget struct {
	Kind RefreshTargetKind
	ID   string
}

// RefreshOutcome is the result for one account.
type RefreshOutcome struct {
	AccountID string
	// Snapshot is the reading as JSON, nil on failure.
	Snapshot json.RawMessage
	Err      error
	// RetainedStaleSnapshot is true when a failure left an older reading.
	RetainedStaleSnapshot bool
}

// RefreshReport lists the outcomes of one refresh pass.
type RefreshReport struct {
	Target     RefreshTarget
	Outcomes   []RefreshOutcome
	StartedAt  time.Time
	FinishedAt time.Time
}

// ImportBatchResult lists the accounts an import added or updated.
type ImportBatchResult struct {
	Succeeded     []ManagedAccount
	Failures      []string
	FailureErrors []error `json:"-"`
}

// SuccessCount is the number of imported accounts.
func (r ImportBatchResult) SuccessCount() int { return len(r.Succeeded) }

// FailureCount is the number of failures.
func (r ImportBatchResult) FailureCount() int { return len(r.Failures) }
