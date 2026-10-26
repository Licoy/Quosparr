package runway

import (
	"context"
	"sync"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/usage"
)

// API relay accounts and plain API keys. When the Codex login is an API key
// there is no ChatGPT backend to ask: no quota, reset credits, estimate,
// official daily usage or profile series. A relay account (an API key with a
// base URL) has a balance on its site instead; it is probed through
// Sources.FetchAPIQuota and cached on the account row, which is the single
// source of the balance the views show. Local scans run as for any account.

// relayProbeConcurrency bounds the sites probed at the same time when the
// account list is polled.
const relayProbeConcurrency = 3

// keyModeLocked reports whether the shown credential is an API key.
func (s *Service) keyModeLocked() bool { return s.auth != nil && s.auth.IsAPIKey() }

// currentRowLocked is the managed row of the shown credential. Unlike
// currentAccountIDLocked it never falls back to the active account, so a
// relay site is never paired with someone else's key.
func (s *Service) currentRowLocked() (codex.ManagedAccount, bool) {
	if s.auth == nil {
		return codex.ManagedAccount{}, false
	}
	for _, a := range s.index.Accounts {
		if codex.AccountMatches(a, *s.auth) {
			return a, true
		}
	}
	return codex.ManagedAccount{}, false
}

// relayProbing reports whether relay balances can be probed at all.
func (s *Service) relayProbing() bool { return s.src.FetchAPIQuota != nil }

// relayAccountsLocked lists the relay rows that can be polled.
func (s *Service) relayAccountsLocked() []codex.ManagedAccount {
	if !s.relayProbing() || s.src.LoadAPIKey == nil {
		return nil
	}
	var out []codex.ManagedAccount
	for _, a := range s.index.Accounts {
		if a.IsRelay() {
			out = append(out, a)
		}
	}
	return out
}

// relayAccount finds the pollable relay row with the given id.
func (s *Service) relayAccount(id string) (codex.ManagedAccount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range s.relayAccountsLocked() {
		if a.ID == id {
			return a, true
		}
	}
	return codex.ManagedAccount{}, false
}

// beginProbe marks a relay account as being probed; false when it already is,
// in which case the running probe updates the row for everyone.
func (s *Service) beginProbe(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.probes[id] {
		return false
	}
	s.probes[id] = true
	return true
}

func (s *Service) endProbe(id string) {
	s.mu.Lock()
	delete(s.probes, id)
	s.mu.Unlock()
}

// storeRelayQuota caches a probe result on the row of id, unless the row was
// removed or now points at another site. Only the cache changes; the
// credential is not touched.
func (s *Service) storeRelayQuota(id, baseURL string, q apiquota.Quota) {
	idx := s.indexForUpdate()
	acc, ok := idx.Account(id)
	if !ok || acc.APIBaseURL != baseURL || keepCachedBalance(acc.APIQuota, q) {
		return
	}
	acc = acc.ApplyAPIQuota(q)
	if s.src.SaveAccount != nil {
		if err := s.src.SaveAccount(acc); err != nil {
			return
		}
	}
	s.mu.Lock()
	if s.src.LoadIndex != nil {
		s.reloadIndexLocked()
	} else {
		s.index.Upsert(acc)
	}
	s.publishLocked()
	s.mu.Unlock()
}

// keepCachedBalance reports whether a transient failure should leave the last
// good balance (with its own reading time) in place. A rejected key or a site
// without a balance endpoint still replaces it.
func keepCachedBalance(prev *apiquota.Quota, next apiquota.Quota) bool {
	if prev == nil || prev.Status != apiquota.StatusAvailable {
		return false
	}
	return next.Status == apiquota.StatusError || next.Status == apiquota.StatusInvalidData
}

// probeRelay asks the site of acc for its balance with key and caches the
// answer. A cancelled probe stores nothing.
func (s *Service) probeRelay(ctx context.Context, acc codex.ManagedAccount, key string) {
	if !s.beginProbe(acc.ID) {
		return
	}
	defer s.endProbe(acc.ID)
	q := s.src.FetchAPIQuota(ctx, acc.APIBaseURL, key)
	if ctx.Err() != nil {
		return
	}
	s.storeRelayQuota(acc.ID, acc.APIBaseURL, q)
}

// probeRelayAccount polls one relay row with its managed key. A key that
// cannot be read makes no request.
func (s *Service) probeRelayAccount(ctx context.Context, acc codex.ManagedAccount) {
	key, err := s.src.LoadAPIKey(acc.ID)
	if err != nil || key == "" {
		return
	}
	s.probeRelay(ctx, acc, key)
}

// probeRelayAccounts polls rows concurrently, at most relayProbeConcurrency at
// a time.
func (s *Service) probeRelayAccounts(ctx context.Context, rows []codex.ManagedAccount) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, relayProbeConcurrency)
	for _, acc := range rows {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func() {
			defer func() { <-slots; wg.Done() }()
			s.probeRelayAccount(ctx, acc)
		}()
	}
	wg.Wait()
}

// probeCurrentRelay refreshes the balance of the shown credential when it is a
// relay account. A plain API key makes no request.
func (s *Service) probeCurrentRelay(ctx context.Context, auth codex.Auth) {
	if !s.relayProbing() {
		return
	}
	s.mu.Lock()
	row, ok := s.currentRowLocked()
	s.mu.Unlock()
	if ok && row.IsRelay() {
		s.probeRelay(ctx, row, auth.APIKey())
	}
}

// markKeyModeReady clears the failure marks a signed-out or failed ChatGPT
// refresh left: an API key has nothing to fail on.
func (s *Service) markKeyModeReady(auth codex.Auth, gen uint64) {
	s.apply(auth, gen, func() { s.quotaErr, s.statusFail, s.loggedOut = "", false, false })
}

// refreshKeyMode is the Codex part of a full refresh for an API key login:
// the relay balance, then the local scans.
func (s *Service) refreshKeyMode(ctx context.Context, auth codex.Auth, gen uint64, policy usage.RefreshPolicy, local localScope) {
	s.markKeyModeReady(auth, gen)
	if s.enter(SectionQuota) {
		s.probeCurrentRelay(ctx, auth)
		s.leaveAccount(gen, SectionQuota)
	}
	s.scanAfterQuota(ctx, auth, gen, codexapi.QuotaSnapshot{}, policy, local)
}

// refreshKeyQuota is the manual quota refresh for an API key login.
func (s *Service) refreshKeyQuota(ctx context.Context, auth codex.Auth, gen uint64) {
	s.markKeyModeReady(auth, gen)
	s.probeCurrentRelay(ctx, auth)
	s.apply(auth, gen, func() { s.lastErr = "" })
}

// apiStatusText is the status item text of an API key without a balance.
const apiStatusText = "API"

// keyViewLocked fills the quota part of the Codex view for an API key login:
// the relay balance when the site reported one, otherwise just "API". A failed
// or unsupported probe is a property of the site and shows on the balance line
// only; a plain key has nothing to report and no error text.
func (s *Service) keyViewLocked(v *CodexView) {
	lc := s.lc
	v.APIKeyMode = true
	v.StatusText, v.QuotaText = apiStatusText, apiStatusText
	row, ok := s.currentRowLocked()
	if !ok || !row.IsRelay() {
		return
	}
	v.APIBaseURL = row.APIBaseURL
	if row.APIQuota == nil {
		return
	}
	q := *row.APIQuota
	v.APIBalance = &q
	if m, ok := APIBalanceMeter(lc, q); ok {
		v.Meters = []Meter{m}
		v.StatusText = APIBalanceText(q)
		v.QuotaText = APIBalanceDetail(q)
		v.QuotaLines = []DetailLine{{lc.Text(l10n.KeyWidgetBalance), APIBalanceDetail(q)}}
		return
	}
	if q.Error != "" {
		v.QuotaLines = []DetailLine{{lc.Text(l10n.KeyError), q.Error}}
	}
}
