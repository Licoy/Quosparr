package grok

import (
	"encoding/json"
	"errors"
	"sort"
	"time"
)

// IndexVersion is the grok-index.json schema version this package reads and writes.
const IndexVersion = 1

// OfficialStatusKind classifies the official ~/.grok/auth.json.
type OfficialStatusKind int

const (
	OfficialMissing OfficialStatusKind = iota
	OfficialMalformed
	OfficialUnreadable
	OfficialAPIKeyOnly
	OfficialUnsupported
	OfficialAuthenticated
	OfficialRequiresReauthentication
)

// OfficialCredentialStatus is the state of the official login file. Identity
// is set for the two states that carry a managed login.
type OfficialCredentialStatus struct {
	Kind     OfficialStatusKind
	Identity *CredentialIdentity
}

// HasManagedLogin reports whether the official file holds a managed login.
func (s OfficialCredentialStatus) HasManagedLogin() bool {
	return s.Kind == OfficialAuthenticated || s.Kind == OfficialRequiresReauthentication
}

// ManagedAccount is one row of grok-index.json. It never holds tokens.
// CachedQuota is the billing snapshot as JSON, kept verbatim: this package
// does not interpret it.
type ManagedAccount struct {
	ID             string
	SortIndex      int
	Identity       CredentialIdentity
	Alias          string
	CreatedAt      time.Time
	LastUsedAt     *time.Time
	LastQuotaAt    *time.Time
	RequiresReauth bool
	LastError      string
	CachedQuota    json.RawMessage

	extra         map[string]any
	identityExtra map[string]any
}

// Email is the identity's email.
func (a ManagedAccount) Email() string { return a.Identity.Email }

// UserID is the identity's user id.
func (a ManagedAccount) UserID() string { return a.Identity.UserID }

// TeamName is the identity's team name.
func (a ManagedAccount) TeamName() string { return a.Identity.TeamName }

// ResolvedDisplayName is the alias, else the identity's label.
func (a ManagedAccount) ResolvedDisplayName() string {
	return acctFirstNonEmpty(a.Alias, a.Identity.ResolvedDisplayName(), a.ID)
}

// ApplyQuota caches a successful billing reading (raw snapshot JSON and its
// update time) and clears any error.
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

// AccountIndex is grok-index.json: the accounts, their order and the current
// one. It never contains tokens.
type AccountIndex struct {
	Version          int
	CurrentAccountID string
	Accounts         []ManagedAccount

	extra map[string]any
}

// NewAccountIndex returns an empty index at the current version.
func NewAccountIndex() AccountIndex { return AccountIndex{Version: IndexVersion} }

// Account returns the row with id.
func (idx AccountIndex) Account(id string) (ManagedAccount, bool) {
	for _, a := range idx.Accounts {
		if a.ID == id {
			return a, true
		}
	}
	return ManagedAccount{}, false
}

// OrderedAccounts sorts by sort index, then by display name (localized,
// case-insensitive).
func (idx AccountIndex) OrderedAccounts() []ManagedAccount {
	out := append([]ManagedAccount(nil), idx.Accounts...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortIndex != out[j].SortIndex {
			return out[i].SortIndex < out[j].SortIndex
		}
		return acctCollateFolded(out[i].ResolvedDisplayName(), out[j].ResolvedDisplayName()) < 0
	})
	return out
}

// Upsert replaces the row with the same id, or appends a new one.
func (idx *AccountIndex) Upsert(acc ManagedAccount) {
	for i := range idx.Accounts {
		if idx.Accounts[i].ID == acc.ID {
			idx.Accounts[i] = acc
			return
		}
	}
	idx.Accounts = append(idx.Accounts, acc)
}

// ParseAccountIndex decodes grok-index.json content. Unknown fields are kept
// for the next write.
func ParseAccountIndex(data []byte) (AccountIndex, error) {
	root, err := acctParseObject(data)
	if err != nil {
		return AccountIndex{}, errors.New("grok-index.json is not a JSON object")
	}
	r := newAcctReader(root)
	idx := AccountIndex{Version: r.reqInt("version"), CurrentAccountID: r.optString("currentAccountID")}
	if raw, ok := r.value("accounts"); !ok {
		r.fail("accounts", "missing")
	} else if rows, isArray := raw.([]any); !isArray {
		r.fail("accounts", "expected an array")
	} else {
		idx.Accounts = make([]ManagedAccount, 0, len(rows))
		for _, row := range rows {
			acc, err := decodeManagedAccount(row)
			if err != nil {
				r.fail("accounts", err.Error())
				break
			}
			idx.Accounts = append(idx.Accounts, acc)
		}
	}
	if r.err != nil {
		return AccountIndex{}, r.err
	}
	idx.extra = r.rest()
	return idx, nil
}

// MarshalAccountIndex renders idx as grok-index.json text: two-space indent,
// keys sorted, slashes unescaped, no trailing newline (JSONEncoder layout).
func MarshalAccountIndex(idx AccountIndex) ([]byte, error) {
	node := acctNode(idx.extra)
	node["version"] = acctInt(idx.Version)
	if idx.CurrentAccountID != "" {
		node["currentAccountID"] = idx.CurrentAccountID
	}
	rows := make([]any, 0, len(idx.Accounts))
	for _, a := range idx.Accounts {
		row, err := a.node()
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	node["accounts"] = rows
	return acctMarshalJSON(node, acctEncoder, true)
}

func (a ManagedAccount) node() (map[string]any, error) {
	o := acctNode(a.extra)
	o["id"] = a.ID
	o["sortIndex"] = acctInt(a.SortIndex)
	o["identity"] = a.identityNode()
	if a.Alias != "" {
		o["alias"] = a.Alias
	}
	o["createdAt"] = acctFormatMillis(a.CreatedAt)
	if a.LastUsedAt != nil {
		o["lastUsedAt"] = acctFormatMillis(*a.LastUsedAt)
	}
	if a.LastQuotaAt != nil {
		o["lastQuotaAt"] = acctFormatMillis(*a.LastQuotaAt)
	}
	o["requiresReauth"] = a.RequiresReauth
	if a.LastError != "" {
		o["lastError"] = a.LastError
	}
	if len(a.CachedQuota) > 0 {
		quota, err := acctParseJSON(a.CachedQuota)
		if err != nil {
			return nil, errors.New("cached quota is not valid JSON")
		}
		o["cachedQuota"] = quota
	}
	return o, nil
}

func (a ManagedAccount) identityNode() map[string]any {
	c := a.Identity
	o := acctNode(a.identityExtra)
	o["kind"] = string(c.Kind)
	o["scope"] = c.Scope
	for key, value := range map[string]string{
		"issuer": c.Issuer, "clientID": c.ClientID, "email": c.Email, "userID": c.UserID,
		"principalID": c.PrincipalID, "principalType": c.PrincipalType, "teamID": c.TeamID, "teamName": c.TeamName,
	} {
		if value != "" {
			o[key] = value
		}
	}
	if c.ExpiresAt != nil {
		o["expiresAt"] = acctFormatMillis(*c.ExpiresAt)
	}
	o["hasAccessToken"] = c.HasAccessToken
	o["hasRefreshToken"] = c.HasRefreshToken
	return o
}

func decodeManagedAccount(v any) (ManagedAccount, error) {
	o, ok := v.(map[string]any)
	if !ok {
		return ManagedAccount{}, errors.New("account row is not an object")
	}
	r := newAcctReader(o)
	acc := ManagedAccount{
		ID:        r.reqString("id"),
		SortIndex: r.reqInt("sortIndex"),
		Alias:     r.optString("alias"),
	}
	if id, ok := r.object("identity"); ok {
		acc.Identity, acc.identityExtra = decodeCredentialIdentity(id, r)
	} else {
		r.fail("identity", "missing")
	}
	acc.CreatedAt = r.reqDate("createdAt")
	acc.LastUsedAt = r.optDate("lastUsedAt")
	acc.LastQuotaAt = r.optDate("lastQuotaAt")
	acc.RequiresReauth = r.boolean("requiresReauth")
	acc.LastError = r.optString("lastError")
	if quota, ok := r.value("cachedQuota"); ok {
		var err error
		acc.CachedQuota, err = acctMarshalJSON(quota, acctEncoder, false)
		if err != nil {
			return ManagedAccount{}, err
		}
	}
	if r.err != nil {
		return ManagedAccount{}, r.err
	}
	acc.extra = r.rest()
	return acc, nil
}

// decodeCredentialIdentity reads the nested identity object; a failure is
// recorded on the parent reader.
func decodeCredentialIdentity(o map[string]any, parent *acctReader) (CredentialIdentity, map[string]any) {
	r := newAcctReader(o)
	c := CredentialIdentity{
		Kind:          AuthKind(r.reqString("kind")),
		Scope:         r.reqString("scope"),
		Issuer:        r.optString("issuer"),
		ClientID:      r.optString("clientID"),
		Email:         r.optString("email"),
		UserID:        r.optString("userID"),
		PrincipalID:   r.optString("principalID"),
		PrincipalType: r.optString("principalType"),
		TeamID:        r.optString("teamID"),
		TeamName:      r.optString("teamName"),
		ExpiresAt:     r.optDate("expiresAt"),
	}
	c.HasAccessToken = r.boolean("hasAccessToken")
	c.HasRefreshToken = r.boolean("hasRefreshToken")
	if r.err == nil && c.Kind != AuthKindOAuth && c.Kind != AuthKindLegacySession {
		r.fail("kind", "unknown value")
	}
	if r.err != nil {
		parent.fail("identity", r.err.Error())
		return CredentialIdentity{}, nil
	}
	return c, r.rest()
}

func (r *acctReader) optDate(key string) *time.Time {
	if _, ok := r.value(key); !ok {
		return nil
	}
	s := r.optString(key)
	if r.err != nil {
		return nil
	}
	t, ok := acctParseDate(s)
	if !ok {
		r.fail(key, "unrecognized date")
		return nil
	}
	return &t
}

func (r *acctReader) reqDate(key string) time.Time {
	p := r.optDate(key)
	if p == nil {
		r.fail(key, "missing")
		return time.Time{}
	}
	return *p
}

// AccountState is what the module reports after reading the official login
// and the library.
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
	// Snapshot is the billing reading as JSON, nil on failure.
	Snapshot json.RawMessage
	Err      error
	// RetainedStaleSnapshot is true when a failure left an older cached
	// reading in place.
	RetainedStaleSnapshot bool
}

// RefreshReport lists the outcomes of one refresh pass.
type RefreshReport struct {
	Target     RefreshTarget
	Outcomes   []RefreshOutcome
	StartedAt  time.Time
	FinishedAt time.Time
}

// ImportBatchResult counts the accounts an import added or updated.
type ImportBatchResult struct {
	Succeeded     []ManagedAccount
	Failures      []string
	FailureErrors []error `json:"-"`
}

// SuccessCount is the number of imported accounts.
func (r ImportBatchResult) SuccessCount() int { return len(r.Succeeded) }

// FailureCount is the number of failures.
func (r ImportBatchResult) FailureCount() int { return len(r.Failures) }
