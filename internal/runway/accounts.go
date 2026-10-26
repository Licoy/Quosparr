package runway

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/usage"
)

// ErrSwitchBusy means another account switch is still running.
var ErrSwitchBusy = errors.New("runway: account switch in progress")

// reloadIndexLocked re-reads the account list. A failure is shown to the user
// instead of leaving a stale list unnoticed.
func (s *Service) reloadIndexLocked() {
	if s.src.LoadIndex == nil {
		return
	}
	idx, err := s.src.LoadIndex()
	if err != nil {
		s.lastErr = s.lc.Text(l10n.KeyAccountsImportFailed) + ": " + err.Error()
		return
	}
	s.index, s.indexLoaded = idx, true
}

// ReloadAccounts re-reads the account list (after the UI changed it through
// codex.Store: import, delete, reorder, alias, OAuth login).
func (s *Service) ReloadAccounts() {
	s.mu.Lock()
	s.reloadIndexLocked()
	s.publishLocked()
	s.mu.Unlock()
}

func toQuotaWindow(w codexapi.RateWindow) codex.QuotaWindow {
	out := codex.QuotaWindow{UsedPercent: w.UsedPercent}
	if w.WindowMinutes > 0 {
		m := w.WindowMinutes
		out.WindowMinutes = &m
	}
	if !w.ResetsAt.IsZero() {
		t := w.ResetsAt
		out.ResetsAt = &t
	}
	return out
}

// quotaSummary reduces a quota reading to what the account row caches.
func quotaSummary(q codexapi.QuotaSnapshot) codex.QuotaSummary {
	out := codex.QuotaSummary{Plan: q.Plan, Primary: toQuotaWindow(q.Primary), CreditsBalance: q.CreditsBalance, UpdatedAt: q.UpdatedAt}
	if q.Secondary != nil {
		w := toQuotaWindow(*q.Secondary)
		out.Secondary = &w
	}
	for _, a := range q.AdditionalWindows {
		out.Additional = append(out.Additional, codex.NamedQuotaWindow{Name: a.Name, Window: toQuotaWindow(a.Window)})
	}
	return out
}

// RefreshAllAccountQuotas polls the quota of every managed account. API relay
// accounts are probed for their balance instead; a plain API key makes no
// request.
func (s *Service) RefreshAllAccountQuotas() <-chan struct{} {
	if s.src.RefreshAccounts == nil && !s.relayProbing() {
		return closedChan()
	}
	return s.run(func() { s.refreshAccountsInline(s.root, "") })
}

// RefreshAccountQuota polls one managed account (a relay account's balance).
func (s *Service) RefreshAccountQuota(id string) <-chan struct{} {
	if s.src.RefreshAccount == nil && !s.relayProbing() {
		return closedChan()
	}
	s.mu.Lock()
	if s.refreshingIDs[id] {
		s.mu.Unlock()
		return closedChan()
	}
	s.refreshingIDs[id] = true
	epoch := s.epoch
	s.publishLocked()
	s.mu.Unlock()
	return s.run(func() {
		if acc, ok := s.relayAccount(id); ok {
			s.probeRelayAccount(s.root, acc)
		} else if s.src.RefreshAccount != nil {
			res := s.src.RefreshAccount(s.root, id)
			s.applyAccountResults([]codexapi.QuotaRefreshResult{res}, epoch)
		}
		s.mu.Lock()
		delete(s.refreshingIDs, id)
		s.publishLocked()
		s.mu.Unlock()
	})
}

// refreshAccountsInline polls every account; skipCurrentID is the account
// whose reset credits or relay balance the caller fetches itself.
func (s *Service) refreshAccountsInline(ctx context.Context, skipCurrentID string) {
	if s.src.RefreshAccounts == nil && !s.relayProbing() {
		return
	}
	s.mu.Lock()
	if s.accountsBusy {
		s.mu.Unlock()
		return
	}
	s.accountsBusy = true
	epoch := s.epoch
	var ids []string
	for _, a := range s.index.Accounts {
		ids = append(ids, a.ID)
		s.refreshingIDs[a.ID] = true
	}
	relays := slices.DeleteFunc(s.relayAccountsLocked(), func(a codex.ManagedAccount) bool { return a.ID == skipCurrentID })
	s.publishLocked()
	s.mu.Unlock()

	// Relay balances are probed alongside the OAuth quotas; a plain API key
	// is in neither and makes no request.
	var probed sync.WaitGroup
	if len(relays) > 0 {
		probed.Add(1)
		go func() { defer probed.Done(); s.probeRelayAccounts(ctx, relays) }()
	}
	if s.src.RefreshAccounts != nil {
		s.applyAccountResults(s.src.RefreshAccounts(ctx, skipCurrentID), epoch)
	}
	probed.Wait()

	s.mu.Lock()
	s.accountsBusy = false
	for _, id := range ids {
		delete(s.refreshingIDs, id)
	}
	s.publishLocked()
	s.mu.Unlock()
}

// applyAccountResults stores polled account quotas on their account rows.
// Results of a superseded refresh are dropped. Only a re-authorization demand
// of the active account touches the shown state; other accounts' failures
// stay on their own rows.
func (s *Service) applyAccountResults(results []codexapi.QuotaRefreshResult, epoch uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.src.LoadIndex != nil {
		s.reloadIndexLocked()
	}
	idx := s.index
	reauth := map[string]bool{}
	for _, r := range results {
		if s.src.Generations != nil && !s.src.Generations.Valid(r.AccountID, r.Generation) {
			continue
		}
		acc, ok := idx.Account(r.AccountID)
		if !ok {
			continue
		}
		switch {
		case r.Err != nil:
			acc = acc.ApplyError(r.ErrorMessage(), r.NeedsReauth)
			if r.NeedsReauth {
				reauth[r.AccountID] = true
			}
		case r.Quota != nil:
			acc = acc.ApplyQuota(quotaSummary(*r.Quota))
		default:
			continue
		}
		checkedAt := s.now()
		if r.ResetCredits != nil {
			acc = cacheAccountResetCredits(acc, *r.ResetCredits, checkedAt)
		} else if r.ResetCreditsErr != nil {
			acc = acc.ApplyResetCredits(nil, &checkedAt, "reset_credits_unavailable")
		}
		if r.WorkspaceName != "" {
			acc.WorkspaceName = r.WorkspaceName
		}
		idx.Upsert(acc)
		if s.src.SaveAccount != nil {
			if err := s.src.SaveAccount(acc); err != nil {
				s.lastErr = err.Error()
			}
		}
	}
	if s.src.LoadIndex != nil {
		s.reloadIndexLocked()
	} else {
		s.index = idx
	}
	if epoch == s.epoch && reauth[s.index.ActiveAccountID] {
		s.handleAuthFailureLocked(codexapi.ErrAuthenticationRequired)
	}
	s.publishLocked()
}

func (s *Service) indexForUpdate() codex.AccountIndex {
	if s.src.LoadIndex != nil {
		if idx, err := s.src.LoadIndex(); err == nil {
			return idx
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.index
}

// SwitchAccount installs a managed account as the Codex login, drops the
// previous account's data and reloads everything for the new one. It blocks
// until the switch is done; the refresh that follows runs in the background.
// Restarting the Codex app is left to the caller.
func (s *Service) SwitchAccount(ctx context.Context, id string) error {
	if s.src.SwitchAccount == nil {
		return errors.New("runway: account switching is not available")
	}
	s.mu.Lock()
	if s.switching {
		s.mu.Unlock()
		return ErrSwitchBusy
	}
	s.switching = true
	s.accountMessage = s.lc.Text(l10n.KeyAccountsSwitching)
	s.publishLocked()
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.switching = false
		s.publishLocked()
		s.mu.Unlock()
	}()

	res, err := s.src.SwitchAccount(ctx, id)
	if err != nil {
		s.mu.Lock()
		s.accountMessage = ""
		s.reloadIndexLocked()
		s.lastErr = s.switchFailureTextLocked(err)
		s.publishLocked()
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	auth := res.Auth
	s.clearAccountScopedLocked(&auth)
	s.mu.Unlock()
	s.cancelAccountScoped()
	s.mu.Lock()
	s.planHint = res.Account.PlanType
	s.reloadIndexLocked()
	s.accountMessage, s.lastErr = "", ""
	s.publishLocked()
	s.mu.Unlock()

	s.beginFullRefresh(usage.RefreshForce)
	s.RefreshTokenHeatmap(usage.RefreshForce)
	return nil
}

func (s *Service) switchFailureTextLocked(err error) string {
	lc := s.lc
	if key := CredentialModeErrorKey(err); key != "" {
		return lc.Text(key)
	}
	switch {
	case IsAuthFailure(err), errors.Is(err, codex.ErrReauthRequired):
		return lc.Text(l10n.KeyAccountsNeedsReauth)
	case errors.Is(err, codex.ErrExpiredAccessWithoutRefresh), errors.Is(err, codex.ErrRefreshUnavailable):
		return lc.Text(l10n.KeyAccountsSwitchSessionExpired)
	case errors.Is(err, codex.ErrNotUsableAsCodexLogin), errors.Is(err, codex.ErrInvalidCredential):
		return lc.Text(l10n.KeyAccountsSwitchInvalidCredential)
	}
	return lc.Text(l10n.KeyAccountsSwitchFailed) + ": " + err.Error()
}

// cacheCurrentResetCreditsLocked reuses an already fetched response, matching
// the credential explicitly so an active-row fallback cannot cross accounts.
// The caller holds s.mu and has validated the account epoch.
func (s *Service) cacheCurrentResetCreditsLocked(auth codex.Auth, credits *codexapi.ResetCreditsSnapshot) error {
	idx := s.index
	if s.src.LoadIndex != nil {
		loaded, err := s.src.LoadIndex()
		if err != nil {
			return err
		}
		idx = loaded
	}
	for _, acc := range idx.Accounts {
		if acc.AuthMode == codex.AuthModeAPIKey || !codex.AccountMatches(acc, auth) {
			continue
		}
		checkedAt := s.now()
		if credits == nil {
			acc = acc.ApplyResetCredits(nil, &checkedAt, "reset_credits_unavailable")
		} else {
			acc = cacheAccountResetCredits(acc, *credits, checkedAt)
		}
		if s.src.SaveAccount != nil {
			if err := s.src.SaveAccount(acc); err != nil {
				return err
			}
		}
		idx.Upsert(acc)
		s.index = idx
		return nil
	}
	return nil
}
