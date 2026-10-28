package runway

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/codexruntime"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/usage"
)

// AuthFailKind says why the local credential cannot be used.
type AuthFailKind int

// AuthFailKind values.
const (
	// AuthFileInvalid: missing, unreadable or placeholder credential.
	AuthFileInvalid AuthFailKind = iota
	// AuthExpired: the access token expired and cannot be renewed.
	AuthExpired
)

// AuthLoadError is a local credential failure (Swift RunwayModelAuthError.load).
// It never carries token text.
type AuthLoadError struct {
	Kind AuthFailKind
	Err  error
}

func (e *AuthLoadError) Error() string {
	if e.Kind == AuthExpired {
		return "auth expired"
	}
	return "auth file invalid"
}

// Unwrap exposes the cause for errors.Is.
func (e *AuthLoadError) Unwrap() error { return e.Err }

// IsAuthFailure reports whether err means "sign in again": a local credential
// failure or a server refusal (HTTP 401).
func IsAuthFailure(err error) bool {
	var load *AuthLoadError
	return errors.As(err, &load) || errors.Is(err, codexapi.ErrAuthenticationRequired)
}

// Sources is the Service's outside world: credentials, network, scans. Tests
// replace every field with a fake; NewLiveSources wires the real ones. Fields
// marked optional may be nil.
type Sources struct {
	// LoadValidAuth returns a usable credential, refreshing and persisting
	// its tokens when expired. With preferCached it returns cached as is.
	// Local credential failures must be *AuthLoadError.
	LoadValidAuth func(ctx context.Context, preferCached bool, cached *codex.Auth) (codex.Auth, error)
	// FetchQuota, FetchResetCredits, FetchDailyUsage and FetchProfileTokens
	// are the ChatGPT backend reads. A refused credential must satisfy
	// errors.Is(err, codexapi.ErrAuthenticationRequired).
	FetchQuota         func(ctx context.Context, a codexapi.Auth) (codexapi.QuotaSnapshot, error)
	FetchResetCredits  func(ctx context.Context, a codexapi.Auth) (codexapi.ResetCreditsSnapshot, error)
	FetchDailyUsage    func(ctx context.Context, a codexapi.Auth, startDate, endDate string, w usage.Window, calculatedAt time.Time) (usage.Summary, error)
	FetchProfileTokens func(ctx context.Context, a codexapi.Auth) (usage.ProfileTokenUsage, error)
	// ScanCost summarizes the local session logs for the queries.
	ScanCost func(ctx context.Context, q []usage.Query, calculatedAt time.Time, p usage.RefreshPolicy, progress *usage.ProgressReporter) (map[string]usage.Summary, error)

	// DryRunSessions / RepairSessions / ScanRecentSessions (optional) read or
	// repair the session index and list recent sessions.
	DryRunSessions     func(ctx context.Context) (sessions.Report, error)
	RepairSessions     func(ctx context.Context) (sessions.Report, error)
	ScanRecentSessions func(ctx context.Context, limit int) (sessions.Summary, error)
	// ScanSpeed (optional) measures the model speed of the newest local
	// sessions as of now.
	ScanSpeed func(ctx context.Context, now time.Time) (speed.Report, error)

	// Account library (optional). SyncOfficial aligns the library with the
	// official auth.json; LoadIndex reads the account list; SaveAccount
	// persists one account row; RecordAuth mirrors a freshly loaded
	// credential into the library; OfficialUsability tells why a 401 happened.
	SyncOfficial      func(now time.Time) (codex.AccountIndex, error)
	LoadIndex         func() (codex.AccountIndex, error)
	SaveAccount       func(acc codex.ManagedAccount) error
	RecordAuth        func(auth codex.Auth, quotaPlan string, now time.Time) error
	OfficialUsability func(now time.Time) (codex.Usability, bool)

	// FetchAPIQuota (optional) probes the balance of an API relay site; it
	// never fails, the outcome is in Quota.Status. LoadAPIKey (optional) reads
	// the API key of a managed account for it. Both are needed to poll relay
	// accounts other than the current login; the key must never be logged.
	FetchAPIQuota func(ctx context.Context, baseURL, apiKey string) apiquota.Quota
	LoadAPIKey    func(id string) (string, error)

	// SwitchAccount installs a managed account as the official login.
	SwitchAccount   func(ctx context.Context, id string) (codex.SwitchResult, error)
	PrepareSwitch   func(context.Context, string) (func() (codex.SwitchResult, error), error)
	RuntimeIdentity func() (codexruntime.Identity, error)
	LegacyStop      func() (func() error, error)
	RuntimeInspect  func(context.Context) (codexruntime.Snapshot, error)
	RuntimeStop     func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error)
	RuntimeStart    func(context.Context, codexruntime.Snapshot) (codexruntime.Snapshot, error)
	// RefreshAccounts / RefreshAccount poll the quota of managed accounts.
	// Results carry the account id and generation; Generations (shared with
	// the refresher) lets the Service drop stale ones.
	RefreshAccounts func(ctx context.Context, skipResetCreditsID string) []codexapi.QuotaRefreshResult
	RefreshAccount  func(ctx context.Context, id string) codexapi.QuotaRefreshResult
	Generations     *codexapi.Generations
}

// apiAuth reduces a credential to what the API calls need.
func apiAuth(a codex.Auth) codexapi.Auth {
	return codexapi.Auth{IDToken: a.Tokens.IDToken, AccessToken: a.Tokens.AccessToken, AccountID: a.Tokens.AccountID}
}

// LiveConfig wires NewLiveSources. All paths live inside Store.
type LiveConfig struct {
	LegacyStop func() (func() error, error)
	Store      *codex.Store
	CodexHome  string
	Quota      codexapi.QuotaClient
	Refresher  codexapi.TokenRefresher
	// Repo is the local usage index; nil uses the direct scanner for costs.
	Repo   *usage.Repository
	Prices usage.PriceSource
	Now    func() time.Time
}

// NewLiveSources builds the production Sources.
func NewLiveSources(cfg LiveConfig) Sources {
	l := &live{LiveConfig: cfg, gens: &codexapi.Generations{}}
	if l.Now == nil {
		l.Now = time.Now
	}
	l.switcher = &codex.Switcher{Store: cfg.Store, Now: l.Now, Hooks: codex.SwitchHooks{
		Refresh:  l.refreshInPlace,
		Validate: l.validate,
	}}
	l.accounts = codexapi.AccountQuotaRefresher{
		Client:      cfg.Quota,
		Accounts:    l.targets,
		Credential:  l.credential,
		Generations: l.gens,
	}
	runtimeClient := codexruntime.Client{CodexHome: cfg.CodexHome}
	return Sources{
		LoadValidAuth:      l.loadValidAuth,
		FetchQuota:         cfg.Quota.FetchQuota,
		FetchResetCredits:  cfg.Quota.FetchResetCredits,
		FetchDailyUsage:    l.fetchDaily,
		FetchProfileTokens: l.fetchProfile,
		ScanCost:           l.scanCost,
		DryRunSessions: func(context.Context) (sessions.Report, error) {
			return sessions.Repairer{CodexHome: cfg.CodexHome}.DryRun()
		},
		RepairSessions: func(context.Context) (sessions.Report, error) {
			return sessions.Repairer{CodexHome: cfg.CodexHome}.Repair()
		},
		ScanRecentSessions: l.scanRecentSessions,
		ScanSpeed: func(ctx context.Context, now time.Time) (speed.Report, error) {
			ss, err := sessions.Scanner{CodexHome: cfg.CodexHome}.ScanSpeed(ctx, now)
			if err != nil {
				return speed.Report{}, err
			}
			return speed.Build(ss, now), nil
		},
		SyncOfficial: func(now time.Time) (codex.AccountIndex, error) {
			idx, _, err := cfg.Store.SyncFromOfficialAuth(now)
			return idx, err
		},
		LoadIndex:     cfg.Store.LoadIndex,
		SaveAccount:   cfg.Store.UpdateMetadata,
		RecordAuth:    l.recordAuth,
		FetchAPIQuota: l.fetchAPIQuota,
		LoadAPIKey:    l.loadAPIKey,
		OfficialUsability: func(now time.Time) (codex.Usability, bool) {
			a, err := cfg.Store.LoadOfficialAuth()
			if err != nil {
				return codex.InvalidTokens, false
			}
			return a.LoginUsability(now), true
		},
		SwitchAccount: l.switcher.SwitchTo,
		RuntimeIdentity: func() (codexruntime.Identity, error) {
			auth, err := cfg.Store.LoadOfficialAuth()
			if err != nil {
				return codexruntime.Identity{}, err
			}
			return runtimeTarget(codex.NewManagedAccount("", auth, 0, "", "", "", l.Now())), nil
		},
		LegacyStop:      cfg.LegacyStop,
		PrepareSwitch:   l.switcher.PrepareSwitch,
		RuntimeInspect:  runtimeClient.Inspect,
		RuntimeStop:     runtimeClient.Stop,
		RuntimeStart:    runtimeClient.Start,
		RefreshAccounts: l.accounts.RefreshAll,
		RefreshAccount:  l.accounts.Refresh,
		Generations:     l.gens,
	}
}

type live struct {
	LiveConfig
	gens     *codexapi.Generations
	switcher *codex.Switcher
	accounts codexapi.AccountQuotaRefresher
}

func (l *live) scanRecentSessions(ctx context.Context, limit int) (sessions.Summary, error) {
	scanner := sessions.Scanner{CodexHome: l.CodexHome}
	if l.Repo == nil {
		scanner.Pricer = sessionPricerFor(l.priceBook(ctx))
	}
	recent, err := scanner.Scan(limit)
	if err != nil || len(recent.Items) == 0 || l.Repo == nil {
		return recent, err
	}
	queries := make([]usage.Query, 0, len(recent.Items))
	// Query every indexed event of the selected sources; request keys retain
	// the path and the repository captures one immutable book for the batch
	window := usage.Window{Start: time.Time{}, End: time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)}
	for _, item := range recent.Items {
		sourcePath, err := filepath.EvalSymlinks(item.SourcePath)
		if err != nil {
			return sessions.Summary{}, err
		}
		queries = append(queries, usage.Query{ID: item.ID, SourcePath: sourcePath, Window: window})
	}
	summaries, err := l.Repo.Summaries(ctx, queries, l.Now(), usage.RefreshIfChanged, nil)
	if err != nil {
		return sessions.Summary{}, err
	}
	for i := range recent.Items {
		summary := summaries[recent.Items[i].ID]
		recent.Items[i].EstimatedUSD = swiftDecimal(summary.EstimatedUSD)
		recent.Items[i].UnpricedTokens = summary.UnpricedTokens + max(0, int64(recent.Items[i].Totals.TotalTokens)-summary.Totals.TotalTokens)
	}
	return recent, nil
}

func (l *live) priceBook(ctx context.Context) usage.PriceBook {
	if l.Prices != nil {
		return l.Prices.PriceBook(ctx)
	}
	return usage.CurrentPriceBook()
}

func sessionPricer(model string, t sessions.Totals) (swiftjson.Decimal, bool) {
	return sessionPricerFor(usage.CurrentPriceBook())(model, t)
}

func sessionPricerFor(book usage.PriceBook) sessions.Pricer {
	return func(model string, t sessions.Totals) (swiftjson.Decimal, bool) {
		cost, ok := book.Cost(model, usage.Totals{
			TotalTokens: int64(t.TotalTokens), UncachedInputTokens: int64(t.UncachedInputTokens),
			CachedInputTokens: int64(t.CachedInputTokens), OutputTokens: int64(t.OutputTokens),
			Turns: int64(t.Turns), Threads: int64(t.Threads),
		})
		if !ok {
			return swiftjson.Decimal{}, false
		}
		d, err := swiftjson.ParseDecimal(cost.String())
		return d, err == nil
	}
}

func (l *live) loadValidAuth(ctx context.Context, preferCached bool, cached *codex.Auth) (codex.Auth, error) {
	if err := l.Store.CheckOfficialCredentialStore(); err != nil {
		return codex.Auth{}, &AuthLoadError{Kind: AuthFileInvalid, Err: err}
	}
	now := l.Now()
	var auth codex.Auth
	if preferCached && cached != nil {
		auth = *cached
	} else {
		a, err := l.Store.LoadOfficialAuth()
		if err != nil {
			return codex.Auth{}, &AuthLoadError{Kind: AuthFileInvalid, Err: err}
		}
		auth = a
	}
	switch auth.LoginUsability(now) {
	case codex.InvalidTokens:
		return codex.Auth{}, &AuthLoadError{Kind: AuthFileInvalid}
	case codex.ExpiredAccessWithoutRefresh:
		return codex.Auth{}, &AuthLoadError{Kind: AuthExpired}
	}
	// Access-token-only sessions are used while the JWT is valid; they never
	// hit the refresh endpoint.
	if !auth.IsAPIKey() && codex.IsTokenExpired(auth.Tokens.AccessToken, now) {
		if !auth.CanRefreshOAuth() {
			return codex.Auth{}, &AuthLoadError{Kind: AuthExpired}
		}
		return l.refreshCurrent(ctx, auth, now)
	}
	return auth, nil
}

// refreshCurrent renews the official credential. The persister writes the
// managed copy and, for the current account, the official file.
func (l *live) refreshCurrent(ctx context.Context, auth codex.Auth, now time.Time) (codex.Auth, error) {
	id := l.managedID(auth)
	cur := codexapi.TokenSet{IDToken: auth.Tokens.IDToken, AccessToken: auth.Tokens.AccessToken, RefreshToken: auth.Tokens.RefreshToken}
	var persister codexapi.TokenPersister = TokenPersister{Store: l.Store}
	if id == "" {
		persister = discardPersister{}
	}
	ts, err := l.Refresher.RefreshAndPersist(ctx, persister, id, cur, now)
	if ts.AccessToken == "" {
		if codexapi.NeedsReauth(err) {
			return codex.Auth{}, &AuthLoadError{Kind: AuthExpired, Err: err}
		}
		return codex.Auth{}, err
	}
	// A persist failure still leaves valid tokens for this run.
	auth.Tokens.IDToken, auth.Tokens.AccessToken, auth.Tokens.RefreshToken = ts.IDToken, ts.AccessToken, ts.RefreshToken
	auth.LastRefresh = codexapi.FormatTimestamp(now)
	return auth, nil
}

type discardPersister struct{}

func (discardPersister) SaveRefreshed(context.Context, string, codexapi.TokenSet, time.Time) error {
	return nil
}

func (l *live) managedID(auth codex.Auth) string {
	idx, err := l.Store.LoadIndex()
	if err != nil {
		return ""
	}
	for _, a := range idx.Accounts {
		if codex.AccountMatches(a, auth) {
			return a.ID
		}
	}
	return ""
}

func (l *live) refreshInPlace(ctx context.Context, a *codex.Auth) error {
	ts, err := l.Refresher.Refresh(ctx, a.Tokens.RefreshToken)
	if err != nil {
		if codexapi.NeedsReauth(err) {
			return errors.Join(codex.ErrReauthRequired, err)
		}
		return err
	}
	m := ts.MergedInto(codexapi.TokenSet{IDToken: a.Tokens.IDToken, AccessToken: a.Tokens.AccessToken, RefreshToken: a.Tokens.RefreshToken})
	a.Tokens.IDToken, a.Tokens.AccessToken, a.Tokens.RefreshToken = m.IDToken, m.AccessToken, m.RefreshToken
	a.LastRefresh = codexapi.FormatTimestamp(l.Now())
	return nil
}

func (l *live) validate(ctx context.Context, a codex.Auth) error {
	_, err := l.Quota.FetchQuota(ctx, apiAuth(a))
	if codexapi.NeedsReauth(err) {
		return errors.Join(codex.ErrReauthRequired, err)
	}
	return err
}

func (l *live) targets(context.Context) ([]codexapi.AccountTarget, error) {
	idx, err := l.Store.LoadIndex()
	if err != nil {
		return nil, err
	}
	var out []codexapi.AccountTarget
	for _, a := range idx.OrderedForSidebar() {
		out = append(out, codexapi.AccountTarget{
			ID:               a.ID,
			APIKey:           a.AuthMode == codex.AuthModeAPIKey,
			HasWorkspaceName: a.WorkspaceName != "",
			Tier:             codexapi.SubscriptionTier(a.Tier()),
		})
	}
	return out, nil
}

func (l *live) credential(ctx context.Context, id string) (codexapi.Auth, error) {
	auth, err := l.switcher.EnsureValidCredential(ctx, id)
	if err != nil {
		if errors.Is(err, codex.ErrReauthRequired) {
			return codexapi.Auth{}, errors.Join(codexapi.ErrAuthenticationRequired, err)
		}
		return codexapi.Auth{}, err
	}
	return apiAuth(auth), nil
}

func (l *live) fetchDaily(ctx context.Context, a codexapi.Auth, start, end string, w usage.Window, at time.Time) (usage.Summary, error) {
	raw, err := l.Quota.FetchDailyWorkspaceUsageRaw(ctx, a, start, end)
	if err != nil {
		return usage.Summary{}, err
	}
	return usage.DecodeAnalytics(raw, w, at, start, end)
}

func (l *live) fetchProfile(ctx context.Context, a codexapi.Auth) (usage.ProfileTokenUsage, error) {
	p, err := l.Quota.FetchTokenUsageProfile(ctx, a)
	if err != nil {
		return usage.ProfileTokenUsage{}, err
	}
	days := make(map[string]int64, len(p.DailyTokens))
	for d, n := range p.DailyTokens {
		days[d] = int64(n)
	}
	return usage.ProfileTokenUsage{DailyTokens: days, StatsAsOf: p.StatsAsOf, GeneratedAt: p.GeneratedAt}, nil
}

// fetchAPIQuota probes a relay site through the proxied client. There is no
// direct fallback: a broken proxy setting is reported, never bypassed.
func (l *live) fetchAPIQuota(ctx context.Context, baseURL, apiKey string) apiquota.Quota {
	now := l.Now()
	client, err := netx.ClientFor(ctx, netx.WithoutCookies)
	if err != nil {
		return apiquota.Quota{Status: apiquota.StatusError, Error: err.Error(), FetchedAt: now}
	}
	return apiquota.Probe(ctx, client, baseURL, apiKey, now)
}

// loadAPIKey reads the API key of a managed account.
func (l *live) loadAPIKey(id string) (string, error) {
	auth, err := l.Store.LoadCredential(id)
	if err != nil {
		return "", err
	}
	if !auth.IsAPIKey() || auth.APIKey() == "" {
		return "", codex.ErrInvalidCredential
	}
	return auth.APIKey(), nil
}

func (l *live) scanCost(ctx context.Context, q []usage.Query, at time.Time, p usage.RefreshPolicy, progress *usage.ProgressReporter) (map[string]usage.Summary, error) {
	if l.Repo == nil {
		book := l.priceBook(ctx)
		scanner := usage.Scanner{CodexHome: l.CodexHome, Prices: book}
		out := make(map[string]usage.Summary, len(q))
		for _, query := range q {
			summary, err := scanner.ScanAPIEquivalent(ctx, query.Window, at)
			if err != nil {
				return nil, err
			}
			out[query.ID] = summary
		}
		return out, nil
	}
	return l.Repo.Summaries(ctx, q, at, p, progress)
}

// recordAuth mirrors the freshly loaded credential into the library: the
// stored copy is replaced only by an equal-or-better one, and the row's
// identity metadata is refreshed. It never touches the official file.
func (l *live) recordAuth(auth codex.Auth, quotaPlan string, now time.Time) error {
	idx, err := l.Store.LoadIndex()
	if err != nil || idx.ActiveAccountID == "" || auth.LoginUsability(now) != codex.Usable {
		return err
	}
	acc, ok := idx.Account(idx.ActiveAccountID)
	if !ok || !codex.AccountMatches(acc, auth) {
		return nil
	}
	prev, perr := l.Store.LoadCredential(acc.ID)
	if perr != nil || !prev.Equal(auth) {
		if perr != nil || prev.LoginUsability(now) != codex.Usable || auth.LoginUsability(now) == codex.Usable {
			if err := l.Store.SaveCredential(acc.ID, auth); err != nil {
				return err
			}
		}
	}
	return l.Store.UpdateMetadata(acc.WithIdentity(auth, quotaPlan, now))
}
