package claude

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
)

// IndexVersion is the claude-index.json schema version.
const IndexVersion = 1

// ManagedAccount is one row of claude-index.json. It never holds tokens.
type ManagedAccount struct {
	ID               string             `json:"id"`
	SortIndex        int                `json:"sortIndex"`
	Identity         CredentialIdentity `json:"identity"`
	SubscriptionType string             `json:"subscriptionType,omitempty"`
	RateLimitTier    string             `json:"rateLimitTier,omitempty"`
	Alias            string             `json:"alias,omitempty"`
	CreatedAt        time.Time          `json:"createdAt"`
	LastUsedAt       *time.Time         `json:"lastUsedAt,omitempty"`
	LastQuotaAt      *time.Time         `json:"lastQuotaAt,omitempty"`
	RequiresReauth   bool               `json:"requiresReauth"`
	LastError        string             `json:"lastError,omitempty"`
	CachedQuota      json.RawMessage    `json:"cachedQuota,omitempty"`

	// Kind is KindAPI for an API relay account and empty for a subscription
	// (OAuth) account. API holds the relay profile and APIQuota the last
	// balance reading; neither contains the key.
	Kind               string            `json:"kind,omitempty"`
	API                *APIProfile       `json:"api,omitempty"`
	APIQuota           *apiquota.Quota   `json:"apiQuota,omitempty"`
	AppliedAPIModels   *AppliedAPIModels `json:"appliedAPIModels,omitempty"`
	AppliedAPIIdentity string            `json:"appliedAPIIdentity,omitempty"`
}

// ResolvedDisplayName is the alias, else the identity label. An API account
// without an alias is "<host> ···<key hint>".
func (a ManagedAccount) ResolvedDisplayName() string {
	if a.IsAPI() {
		return firstNonEmpty(a.Alias, a.apiLabel(), a.ID)
	}
	return firstNonEmpty(a.Alias, a.Identity.ResolvedDisplayName(), a.ID)
}

// Tier is the badge for this account's subscription fields.
func (a ManagedAccount) Tier() TierKind {
	return TierFrom(a.SubscriptionType, a.RateLimitTier)
}

// ApplyQuota caches a usage body and clears the error.
func (a ManagedAccount) ApplyQuota(snapshot json.RawMessage, updatedAt time.Time) ManagedAccount {
	a.CachedQuota = append(json.RawMessage(nil), snapshot...)
	t := updatedAt
	a.LastQuotaAt = &t
	a.RequiresReauth = false
	a.LastError = ""
	return a
}

// ApplyError records a failure and keeps the cached quota.
func (a ManagedAccount) ApplyError(message string, requiresReauth bool) ManagedAccount {
	a.LastError = message
	a.RequiresReauth = requiresReauth
	return a
}

// AccountIndex is claude-index.json. Serialization must not contain tokens.
type AccountIndex struct {
	Version          int              `json:"version"`
	CurrentAccountID string           `json:"currentAccountID,omitempty"`
	Accounts         []ManagedAccount `json:"accounts"`
}

// NewAccountIndex returns an empty index at the current version.
func NewAccountIndex() AccountIndex {
	return AccountIndex{Version: IndexVersion, Accounts: []ManagedAccount{}}
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

// OrderedAccounts sorts by sort index, then by display name.
func (idx AccountIndex) OrderedAccounts() []ManagedAccount {
	out := append([]ManagedAccount(nil), idx.Accounts...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortIndex != out[j].SortIndex {
			return out[i].SortIndex < out[j].SortIndex
		}
		return nameLess(out[i].ResolvedDisplayName(), out[j].ResolvedDisplayName())
	})
	return out
}

func nameLess(a, b string) bool {
	al, bl := strings.ToLower(a), strings.ToLower(b)
	if al != bl {
		return al < bl
	}
	return a < b
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
