package codex

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
)

// AuthMode is how a managed account signs in.
type AuthMode string

const (
	// AuthModeOAuth: a ChatGPT login (tokens).
	AuthModeOAuth AuthMode = "oauth"
	// AuthModeAPIKey: an OpenAI API key.
	AuthModeAPIKey AuthMode = "apiKey"
)

// QuotaWindow is one rate-limit window of a quota reading.
type QuotaWindow struct {
	UsedPercent   int
	WindowMinutes *int
	ResetsAt      *time.Time
}

// NamedQuotaWindow is an additional, named limit (a model-specific window).
type NamedQuotaWindow struct {
	Name   string
	Window QuotaWindow
}

// QuotaSummary is the part of a quota reading the account index caches. It is
// a plain input type so this package needs no network or API package.
type QuotaSummary struct {
	Plan       string
	Primary    QuotaWindow
	Secondary  *QuotaWindow
	Additional []NamedQuotaWindow
	UpdatedAt  time.Time
}

func clampPercent(n int) int { return max(0, min(100, n)) }

// CachedQuotaMeterRow is one meter row cached for the multi-account list.
type CachedQuotaMeterRow struct {
	ID            string
	WindowMinutes *int
	Name          string
	UsedPercent   int
	ResetsAt      *time.Time
	extra         object
}

// RemainingPercent is what is left of the window.
func (m CachedQuotaMeterRow) RemainingPercent() int { return max(0, 100-m.UsedPercent) }

func (m CachedQuotaMeterRow) toObject() object {
	b := newBuilder(m.extra)
	b.set("id", m.ID)
	b.intPtr("windowMinutes", m.WindowMinutes)
	b.str("name", m.Name)
	b.set("usedPercent", m.UsedPercent)
	b.timePtr("resetsAt", m.ResetsAt)
	return b.o
}

// MarshalJSON writes the row in the Swift schema.
func (m CachedQuotaMeterRow) MarshalJSON() ([]byte, error) { return encodeValue(m.toObject()) }

// UnmarshalJSON reads a row; the percentage is kept as stored.
func (m *CachedQuotaMeterRow) UnmarshalJSON(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return err
	}
	r := newReader(o)
	row := CachedQuotaMeterRow{
		ID:            r.reqString("id"),
		WindowMinutes: r.optInt("windowMinutes"),
		Name:          r.optString("name"),
		UsedPercent:   r.reqInt("usedPercent"),
		ResetsAt:      r.optTime("resetsAt"),
	}
	if r.err != nil {
		return r.err
	}
	row.extra = r.rest()
	*m = row
	return nil
}

// CachedAccountQuota is the quota cached on an account row (no secrets).
type CachedAccountQuota struct {
	Plan                   string
	PrimaryUsedPercent     int
	PrimaryResetsAt        *time.Time
	PrimaryWindowMinutes   *int
	SecondaryUsedPercent   *int
	SecondaryResetsAt      *time.Time
	SecondaryWindowMinutes *int
	AdditionalMeters       []CachedQuotaMeterRow
	UpdatedAt              time.Time
	extra                  object
}

// NewCachedQuota caches q: percentages are clamped to 0...100 and additional
// windows get the same ids as the Swift app ("additional-<n>-<name>").
func NewCachedQuota(q QuotaSummary) CachedAccountQuota {
	c := CachedAccountQuota{
		Plan:                 q.Plan,
		PrimaryUsedPercent:   clampPercent(q.Primary.UsedPercent),
		PrimaryResetsAt:      q.Primary.ResetsAt,
		PrimaryWindowMinutes: q.Primary.WindowMinutes,
		UpdatedAt:            q.UpdatedAt,
	}
	if s := q.Secondary; s != nil {
		pct := clampPercent(s.UsedPercent)
		c.SecondaryUsedPercent, c.SecondaryResetsAt, c.SecondaryWindowMinutes = &pct, s.ResetsAt, s.WindowMinutes
	}
	for i, n := range q.Additional {
		c.AdditionalMeters = append(c.AdditionalMeters, CachedQuotaMeterRow{
			ID:            "additional-" + itoa(i) + "-" + n.Name,
			WindowMinutes: n.Window.WindowMinutes,
			Name:          n.Name,
			UsedPercent:   clampPercent(n.Window.UsedPercent),
			ResetsAt:      n.Window.ResetsAt,
		})
	}
	return c
}

// PrimaryRemainingPercent is what is left of the primary window.
func (c CachedAccountQuota) PrimaryRemainingPercent() int { return max(0, 100-c.PrimaryUsedPercent) }

// MeterRows lists primary, secondary (week by default), then additional rows.
func (c CachedAccountQuota) MeterRows() []CachedQuotaMeterRow {
	rows := []CachedQuotaMeterRow{{
		ID: "primary", WindowMinutes: c.PrimaryWindowMinutes,
		UsedPercent: c.PrimaryUsedPercent, ResetsAt: c.PrimaryResetsAt,
	}}
	if c.SecondaryUsedPercent != nil {
		minutes := 10080
		if c.SecondaryWindowMinutes != nil {
			minutes = *c.SecondaryWindowMinutes
		}
		rows = append(rows, CachedQuotaMeterRow{
			ID: "secondary", WindowMinutes: &minutes,
			UsedPercent: *c.SecondaryUsedPercent, ResetsAt: c.SecondaryResetsAt,
		})
	}
	return append(rows, c.AdditionalMeters...)
}

// MarshalJSON writes the quota in the Swift schema.
func (c CachedAccountQuota) MarshalJSON() ([]byte, error) {
	b := newBuilder(c.extra)
	b.str("plan", c.Plan)
	b.set("primaryUsedPercent", c.PrimaryUsedPercent)
	b.timePtr("primaryResetsAt", c.PrimaryResetsAt)
	b.intPtr("primaryWindowMinutes", c.PrimaryWindowMinutes)
	b.intPtr("secondaryUsedPercent", c.SecondaryUsedPercent)
	b.timePtr("secondaryResetsAt", c.SecondaryResetsAt)
	b.intPtr("secondaryWindowMinutes", c.SecondaryWindowMinutes)
	meters := c.AdditionalMeters
	if meters == nil {
		meters = []CachedQuotaMeterRow{}
	}
	b.set("additionalMeters", meters)
	b.set("updatedAt", FormatTime(c.UpdatedAt))
	return encodeValue(b.o)
}

// UnmarshalJSON reads a quota. Percentages are clamped, additionalMeters
// defaults to empty and updatedAt to the current time, like the Swift app.
func (c *CachedAccountQuota) UnmarshalJSON(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return err
	}
	r := newReader(o)
	q := CachedAccountQuota{
		Plan:                   r.optString("plan"),
		PrimaryUsedPercent:     clampPercent(r.reqInt("primaryUsedPercent")),
		PrimaryResetsAt:        r.optTime("primaryResetsAt"),
		PrimaryWindowMinutes:   r.optInt("primaryWindowMinutes"),
		SecondaryUsedPercent:   r.optInt("secondaryUsedPercent"),
		SecondaryResetsAt:      r.optTime("secondaryResetsAt"),
		SecondaryWindowMinutes: r.optInt("secondaryWindowMinutes"),
	}
	if q.SecondaryUsedPercent != nil {
		v := clampPercent(*q.SecondaryUsedPercent)
		q.SecondaryUsedPercent = &v
	}
	if raw, ok := r.raw("additionalMeters"); ok {
		if err := json.Unmarshal(raw, &q.AdditionalMeters); err != nil {
			r.fail("additionalMeters", "invalid")
		}
	}
	if t := r.optTime("updatedAt"); t != nil {
		q.UpdatedAt = *t
	} else {
		q.UpdatedAt = time.Now()
	}
	if r.err != nil {
		return r.err
	}
	q.extra = r.rest()
	*c = q
	return nil
}

// CachedAccountResetCredits stores the reset-credit count, expiry times and fetch status.
// A nil count is unknown, while a pointer to zero is a successful zero reading.
type CachedAccountResetCredits struct {
	AvailableCount    *int        `json:"availableCount,omitempty"`
	CheckedAt         time.Time   `json:"checkedAt,omitempty"`
	UpdatedAt         *time.Time  `json:"updatedAt,omitempty"`
	LastError         string      `json:"lastError,omitempty"`
	AvailableExpiries []time.Time `json:"availableExpiries,omitempty"`
}

// NextAvailableExpiry returns the earliest still-future expiry for a known
// positive count; zero means no valid expiry details are available.
func (c CachedAccountResetCredits) NextAvailableExpiry(now time.Time) time.Time {
	if c.AvailableCount == nil || *c.AvailableCount <= 0 {
		return time.Time{}
	}
	var next time.Time
	for _, expiry := range c.AvailableExpiries {
		if expiry.After(now) && (next.IsZero() || expiry.Before(next)) {
			next = expiry
		}
	}
	return next
}

// ManagedAccount is the non-secret metadata of a managed Codex account, one
// row of index.json. Empty strings and nil pointers stand for absent values
// and are left out of the file.
type ManagedAccount struct {
	ID                    string
	SortIndex             int
	AuthMode              AuthMode
	Email                 string
	Username              string
	UserID                string
	SubjectID             string
	AccountID             string
	WorkspaceName         string
	DisplayName           string
	Alias                 string
	Note                  string
	PlanType              string
	SubscriptionExpiresAt *time.Time
	CreatedAt             time.Time
	LastUsedAt            *time.Time
	LastQuotaAt           *time.Time
	RequiresReauth        bool
	LastError             string
	CachedQuota           *CachedAccountQuota
	ResetCredits          *CachedAccountResetCredits
	// APIBaseURL, APIModel and APIQuota describe an API relay account (an API
	// key used against a third-party site): the normalized OpenAI-compatible
	// base URL (ending /v1), the model written to config.toml, and the cached
	// balance. All are empty for OAuth and plain OpenAI key accounts. APIQuota
	// holds no secrets.
	APIBaseURL string
	APIModel   string
	APIQuota   *apiquota.Quota
	extra      object
}

// IsRelay reports an API key account bound to a third-party base URL.
func (a ManagedAccount) IsRelay() bool {
	return a.AuthMode == AuthModeAPIKey && a.APIBaseURL != ""
}

// ResolvedDisplayName is the name to show: alias, display name, email,
// username, a shortened workspace id, then the id.
func (a ManagedAccount) ResolvedDisplayName() string {
	short := ""
	if a.AccountID != "" {
		short = shortenAccountID(a.AccountID)
	}
	return firstNonEmpty(a.Alias, a.DisplayName, a.Email, a.Username, short, a.ID)
}

// shortenAccountID is the row-label variant of shortenID: it shortens the raw
// text and leaves the untrimmed value of a short id alone.
func shortenAccountID(v string) string {
	r := []rune(v)
	if len(r) <= 9 {
		return v
	}
	return string(r[:5]) + "..." + string(r[len(r)-4:])
}

// Tier is the subscription tier from the plan type or the cached quota plan.
func (a ManagedAccount) Tier() Tier {
	plan := a.PlanType
	if plan == "" && a.CachedQuota != nil {
		plan = a.CachedQuota.Plan
	}
	return ResolveTier(plan, "")
}

// IdentityMarkerUserID is the user id (or subject) shown as an identity marker.
func (a ManagedAccount) IdentityMarkerUserID() string { return firstNonEmpty(a.UserID, a.SubjectID) }

// IdentityMarkerWorkspaceID is the workspace id shown as an identity marker.
func (a ManagedAccount) IdentityMarkerWorkspaceID() string { return firstNonEmpty(a.AccountID) }

// DisplaysWorkspaceIdentity: only organization plans show the workspace.
func (a ManagedAccount) DisplaysWorkspaceIdentity() bool {
	switch a.Tier() {
	case TierTeam, TierBusiness, TierBusinessPremium, TierEnterprise, TierEdu:
		return true
	}
	return false
}

// NewManagedAccount builds the row for auth. id, when non-empty, overrides the
// derived stable id. quotaPlan, when known, beats the plan in the tokens.
func NewManagedAccount(id string, auth Auth, sortIndex int, alias, note, quotaPlan string, now time.Time) ManagedAccount {
	if auth.IsAPIKey() {
		return newAPIKeyAccount(id, auth, sortIndex, alias, note, quotaPlan, now)
	}
	display := DisplayForAuth(auth, quotaPlan, now)
	idc, acc := tokenClaims(auth)
	stable := id
	if stable == "" {
		stable = ManagedAccountID(auth)
	}
	name := display.DisplayName
	if name == "" {
		name = stable
	}
	return ManagedAccount{
		ID:            stable,
		SortIndex:     sortIndex,
		AuthMode:      AuthModeOAuth,
		Email:         display.Email,
		Username:      display.Username,
		UserID:        oauthUserID(auth),
		SubjectID:     oauthSubjectID(auth),
		AccountID:     display.AccountID,
		WorkspaceName: oauthWorkspaceName(auth),
		DisplayName:   name,
		Alias:         alias,
		Note:          note,
		// Live quota plan, then JWT claims, then auth-file fields.
		PlanType:              firstNonEmpty(quotaPlan, idc.plan(), acc.plan(), auth.AuthFilePlanType, auth.PlanType),
		SubscriptionExpiresAt: display.SubscriptionExpiresAt,
		CreatedAt:             now,
	}
}

func newAPIKeyAccount(id string, auth Auth, sortIndex int, alias, note, quotaPlan string, now time.Time) ManagedAccount {
	accountID := "api-" + APIKeyFingerprint(auth.APIKey())
	if id == "" {
		id = accountID
	}
	return ManagedAccount{
		ID:          id,
		SortIndex:   sortIndex,
		AuthMode:    AuthModeAPIKey,
		AccountID:   accountID,
		DisplayName: apiAccountName(accountID, ""),
		Alias:       alias,
		Note:        note,
		PlanType:    firstNonEmpty(quotaPlan, auth.PlanType, "api"),
		CreatedAt:   now,
	}
}

// apiAccountName is the default name of an API key row: "API ···<last 4 of the
// key fingerprint>", or "<host> ···<last 4>" for a relay. accountID is the
// "api-<fingerprint>" value.
func apiAccountName(accountID, baseURL string) string {
	tail := []rune(strings.TrimPrefix(accountID, "api-"))
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	label := "API"
	if u, err := url.Parse(baseURL); err == nil && baseURL != "" && u.Hostname() != "" {
		label = u.Hostname()
	}
	return label + " ···" + string(tail)
}

// withRelayProfile returns a with the relay profile set; an empty base URL
// makes it a plain API key account again. The balance cache is dropped when
// the base URL changes. The generated display name follows the profile; an
// alias, stored separately, still wins at display time.
func (a ManagedAccount) withRelayProfile(baseURL, model string) ManagedAccount {
	c := a
	if baseURL != a.APIBaseURL {
		c.APIQuota = nil
	}
	c.APIBaseURL, c.APIModel = baseURL, ""
	if baseURL != "" {
		c.APIModel = model
	}
	c.DisplayName = apiAccountName(a.AccountID, baseURL)
	return c
}

// ApplyAPIQuota caches a relay balance reading. It does not touch the error
// state of the account: a failed probe is a property of the site, not of the
// credential.
func (a ManagedAccount) ApplyAPIQuota(q apiquota.Quota) ManagedAccount {
	c := a
	c.APIQuota = &q
	return c
}

// WithIdentity refreshes the identity fields from auth while keeping the
// row's id, alias, note, and cached data. Fresh JWT plan data beats stale
// metadata when no quota plan is given.
func (a ManagedAccount) WithIdentity(auth Auth, quotaPlan string, now time.Time) ManagedAccount {
	rebuilt := NewManagedAccount(a.ID, auth, a.SortIndex, a.Alias, a.Note, quotaPlan, now)
	c := a
	c.AuthMode = rebuilt.AuthMode
	c.Email = rebuilt.Email
	c.Username = rebuilt.Username
	c.UserID = firstNonEmpty(rebuilt.UserID, a.UserID)
	c.SubjectID = firstNonEmpty(rebuilt.SubjectID, a.SubjectID)
	c.AccountID = rebuilt.AccountID
	c.WorkspaceName = firstNonEmpty(rebuilt.WorkspaceName, a.WorkspaceName)
	if a.Alias == "" {
		c.DisplayName = rebuilt.DisplayName
		if rebuilt.AuthMode == AuthModeAPIKey {
			c.DisplayName = apiAccountName(rebuilt.AccountID, a.APIBaseURL)
		}
	}
	if rebuilt.AuthMode != AuthModeAPIKey {
		c.APIBaseURL, c.APIModel, c.APIQuota = "", "", nil
	}
	c.PlanType = firstNonEmpty(quotaPlan, rebuilt.PlanType, a.PlanType)
	c.SubscriptionExpiresAt = rebuilt.SubscriptionExpiresAt
	return c
}

// ApplyQuota caches a successful quota reading and clears any error.
func (a ManagedAccount) ApplyQuota(q QuotaSummary) ManagedAccount {
	c := a
	cached := NewCachedQuota(q)
	c.CachedQuota = &cached
	updated := q.UpdatedAt
	c.LastQuotaAt = &updated
	if plan := firstNonEmpty(q.Plan); plan != "" {
		c.PlanType = plan
	}
	c.RequiresReauth = false
	c.LastError = ""
	return c
}

// ApplyResetCredits caches a count or a safe error code, retaining the last
// successful count when a request fails.
func (a ManagedAccount) ApplyResetCredits(count *int, updatedAt *time.Time, message string) ManagedAccount {
	if a.ResetCredits != nil && updatedAt != nil && a.ResetCredits.CheckedAt.After(*updatedAt) {
		return a
	}
	c := a
	cache := CachedAccountResetCredits{}
	if a.ResetCredits != nil {
		cache = *a.ResetCredits
	}
	if count != nil {
		value := *count
		cache.AvailableCount = &value
		cache.UpdatedAt = updatedAt
		cache.AvailableExpiries = nil
	}
	if updatedAt != nil {
		cache.CheckedAt = *updatedAt
	}
	cache.LastError = message
	c.ResetCredits = &cache
	return c
}

// ApplyError records a failure; a re-authorization demand drops the quota.
func (a ManagedAccount) ApplyError(message string, requiresReauth bool) ManagedAccount {
	c := a
	c.LastError = message
	c.RequiresReauth = requiresReauth
	if requiresReauth {
		c.CachedQuota = nil
		c.LastQuotaAt = nil
	}
	return c
}

// MarshalJSON writes the row in the Swift schema.
func (a ManagedAccount) MarshalJSON() ([]byte, error) {
	b := newBuilder(a.extra)
	b.set("id", a.ID)
	b.set("sortIndex", a.SortIndex)
	b.set("authMode", string(a.AuthMode))
	b.str("email", a.Email)
	b.str("username", a.Username)
	b.str("userId", a.UserID)
	b.str("subjectId", a.SubjectID)
	b.str("accountId", a.AccountID)
	b.str("workspaceName", a.WorkspaceName)
	b.set("displayName", a.DisplayName)
	b.str("alias", a.Alias)
	b.str("note", a.Note)
	b.str("planType", a.PlanType)
	b.timePtr("subscriptionExpiresAt", a.SubscriptionExpiresAt)
	b.set("createdAt", FormatTime(a.CreatedAt))
	b.timePtr("lastUsedAt", a.LastUsedAt)
	b.timePtr("lastQuotaAt", a.LastQuotaAt)
	b.set("requiresReauth", a.RequiresReauth)
	b.str("lastError", a.LastError)
	if a.CachedQuota != nil {
		b.set("cachedQuota", a.CachedQuota)
	}
	if a.ResetCredits != nil {
		b.set("resetCredits", a.ResetCredits)
	}
	b.str("apiBaseURL", a.APIBaseURL)
	b.str("apiModel", a.APIModel)
	if a.APIQuota != nil {
		b.set("apiQuota", a.APIQuota)
	}
	return encodeValue(b.o)
}

// UnmarshalJSON reads a row. Like the Swift decoder it requires id, sortIndex,
// authMode, displayName, createdAt and requiresReauth.
func (a *ManagedAccount) UnmarshalJSON(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return err
	}
	r := newReader(o)
	acc := ManagedAccount{
		ID:                    r.reqString("id"),
		SortIndex:             r.reqInt("sortIndex"),
		AuthMode:              AuthMode(r.reqString("authMode")),
		Email:                 r.optString("email"),
		Username:              r.optString("username"),
		UserID:                r.optString("userId"),
		SubjectID:             r.optString("subjectId"),
		AccountID:             r.optString("accountId"),
		WorkspaceName:         r.optString("workspaceName"),
		DisplayName:           r.reqString("displayName"),
		Alias:                 r.optString("alias"),
		Note:                  r.optString("note"),
		PlanType:              r.optString("planType"),
		SubscriptionExpiresAt: r.optTime("subscriptionExpiresAt"),
		CreatedAt:             r.reqTime("createdAt"),
		LastUsedAt:            r.optTime("lastUsedAt"),
		LastQuotaAt:           r.optTime("lastQuotaAt"),
		RequiresReauth:        r.reqBool("requiresReauth"),
		LastError:             r.optString("lastError"),
		APIBaseURL:            r.optString("apiBaseURL"),
		APIModel:              r.optString("apiModel"),
	}
	if raw, ok := r.raw("apiQuota"); ok {
		// A cache is never worth failing the whole index for: drop a damaged one.
		var q apiquota.Quota
		if err := json.Unmarshal(raw, &q); err == nil && q.Status != "" {
			acc.APIQuota = &q
		}
	}
	if raw, ok := r.raw("resetCredits"); ok {
		var c CachedAccountResetCredits
		if err := json.Unmarshal(raw, &c); err != nil {
			r.fail("resetCredits", "invalid")
		}
		if c.AvailableCount != nil && *c.AvailableCount < 0 {
			r.fail("resetCredits", "negative availableCount")
		}
		acc.ResetCredits = &c
	}
	if raw, ok := r.raw("cachedQuota"); ok {
		var q CachedAccountQuota
		if err := json.Unmarshal(raw, &q); err != nil {
			r.fail("cachedQuota", "invalid")
		}
		acc.CachedQuota = &q
	}
	if r.err == nil && acc.AuthMode != AuthModeOAuth && acc.AuthMode != AuthModeAPIKey {
		r.fail("authMode", "unknown value")
	}
	if r.err != nil {
		return r.err
	}
	acc.extra = r.rest()
	*a = acc
	return nil
}

// IndexVersion is the index.json schema version this package writes.
const IndexVersion = 1

// AccountIndex is index.json: the accounts, their order, and the active one.
// It never contains tokens.
type AccountIndex struct {
	Version         int
	ActiveAccountID string
	Accounts        []ManagedAccount
	extra           object
}

// NewAccountIndex returns an empty index at the current version.
func NewAccountIndex() AccountIndex { return AccountIndex{Version: IndexVersion} }

// ParseIndex decodes index.json content.
func ParseIndex(data []byte) (AccountIndex, error) {
	var idx AccountIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return AccountIndex{}, err
	}
	return idx, nil
}

// MarshalIndex renders idx as index.json text (keys sorted, two-space indent,
// slashes escaped, matching the Swift file layout).
func MarshalIndex(idx AccountIndex) ([]byte, error) {
	raw, err := encodeValue(idx)
	if err != nil {
		return nil, err
	}
	return formatJSON(raw, formatOptions{pretty: true, escapeSlash: true})
}

// MarshalJSON writes the index in the Swift schema.
func (idx AccountIndex) MarshalJSON() ([]byte, error) {
	b := newBuilder(idx.extra)
	b.set("version", idx.Version)
	b.str("activeAccountId", idx.ActiveAccountID)
	accounts := idx.Accounts
	if accounts == nil {
		accounts = []ManagedAccount{}
	}
	b.set("accounts", accounts)
	return encodeValue(b.o)
}

// UnmarshalJSON reads the index; version and accounts are required.
func (idx *AccountIndex) UnmarshalJSON(data []byte) error {
	o, err := parseObject(data)
	if err != nil {
		return errors.New("index.json is not a JSON object")
	}
	r := newReader(o)
	out := AccountIndex{
		Version:         r.reqInt("version"),
		ActiveAccountID: r.optString("activeAccountId"),
	}
	raw, ok := r.raw("accounts")
	if !ok {
		r.fail("accounts", "missing")
	} else if err := json.Unmarshal(raw, &out.Accounts); err != nil {
		r.fail("accounts", "invalid: "+err.Error())
	}
	if r.err != nil {
		return r.err
	}
	out.extra = r.rest()
	*idx = out
	return nil
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

// Remove drops id; if it was active, the account with the lowest sort index
// (the first on a tie) becomes active, or none when the index is empty.
func (idx *AccountIndex) Remove(id string) {
	kept := idx.Accounts[:0:0]
	for _, a := range idx.Accounts {
		if a.ID != id {
			kept = append(kept, a)
		}
	}
	idx.Accounts = kept
	if idx.ActiveAccountID != id {
		return
	}
	idx.ActiveAccountID = ""
	sorted := append([]ManagedAccount(nil), kept...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].SortIndex < sorted[j].SortIndex })
	if len(sorted) > 0 {
		idx.ActiveAccountID = sorted[0].ID
	}
}

// ReindexSortOrder numbers the listed ids 0...n-1 in order, then the accounts
// not listed after them in their current order.
func (idx *AccountIndex) ReindexSortOrder(orderedIDs []string) {
	next := 0
	listed := map[string]bool{}
	for _, id := range orderedIDs {
		listed[id] = true
		for i := range idx.Accounts {
			if idx.Accounts[i].ID == id {
				idx.Accounts[i].SortIndex = next
				next++
				break
			}
		}
	}
	for i := range idx.Accounts {
		if !listed[idx.Accounts[i].ID] {
			idx.Accounts[i].SortIndex = next
			next++
		}
	}
}

// OrderedForSidebar lists the active account first, then by sort index, then
// by display name (case-insensitive).
func (idx AccountIndex) OrderedForSidebar() []ManagedAccount {
	out := append([]ManagedAccount(nil), idx.Accounts...)
	sort.SliceStable(out, func(i, j int) bool {
		l, r := out[i], out[j]
		if la, ra := l.ID == idx.ActiveAccountID, r.ID == idx.ActiveAccountID; la != ra {
			return la
		}
		if l.SortIndex != r.SortIndex {
			return l.SortIndex < r.SortIndex
		}
		return strings.ToLower(l.ResolvedDisplayName()) < strings.ToLower(r.ResolvedDisplayName())
	})
	return out
}
