package app

import (
	"fmt"
	"math"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

// grokSidebarAccount is the list row of a managed Grok account. LastError is
// the localized text of the persisted error code.
func grokSidebarAccount(lc l10n.Localizer, account grok.ManagedAccount) codex.ManagedAccount {
	row := codex.ManagedAccount{
		ID: account.ID, SortIndex: account.SortIndex, DisplayName: account.ResolvedDisplayName(),
		Email: account.Email(), WorkspaceName: account.TeamName(), Alias: account.Alias,
		RequiresReauth: account.RequiresReauth, LastQuotaAt: account.LastQuotaAt,
	}
	if account.LastError != "" {
		row.LastError = grokErrorCodeText(lc, account.LastError)
	}
	return row
}

// grokErrorCodeText is GrokAccountLastErrorPresentation: unknown codes read
// as a failed refresh.
func grokErrorCodeText(lc l10n.Localizer, code string) string {
	switch code {
	case grok.ErrorCodeCLIUnavailable:
		return lc.Text(l10n.KeyGrokCLIUnavailable)
	case grok.ErrorCodeAuthenticationNeeded:
		return lc.Text(l10n.KeyGrokReauthenticationRequired)
	case grok.ErrorCodeTimeout:
		return lc.Text(l10n.KeyGrokRefreshTimedOut)
	case grok.ErrorCodeBillingParseFailed:
		return lc.Text(l10n.KeyGrokBillingParseFailed)
	case grok.ErrorCodeCancelled:
		return lc.Text(l10n.KeyGrokRefreshCancelled)
	}
	return lc.Text(l10n.KeyGrokRefreshFailed)
}

// grokAccountQuota is the account-list quota line from the cached billing
// reading; false when the account has none or it cannot be read.
func grokAccountQuota(lc l10n.Localizer, account grok.ManagedAccount) (runway.GrokAccountQuota, bool) {
	if len(account.CachedQuota) == 0 {
		return runway.GrokAccountQuota{}, false
	}
	snap, err := grok.ParseBillingSnapshot(account.CachedQuota)
	if err != nil {
		return runway.GrokAccountQuota{}, false
	}
	q := runway.GrokAccountQuota{
		Title:       lc.Text(l10n.KeyGrokIncludedQuota),
		UsedPercent: clampPercent(int(math.Round(snap.IncludedUsagePercent))),
		IncludedUSD: grokIncludedUSD(snap),
		UpdatedAt:   snap.UpdatedAt,
	}
	q.Plan, q.PlanBillingSystem, q.PlanError = grokPlanProjection(snap)
	if snap.Period != nil {
		q.Title = periodTitle(lc, snap.Period.Kind)
		if snap.Period.ResetsAt != nil {
			q.ResetsAt = *snap.Period.ResetsAt
		}
	}
	q.Meters = grokMeters(lc, snap, q.UpdatedAt)
	if snap.PrepaidBalanceCents != nil {
		q.Details = append(q.Details, runway.DetailLine{Title: lc.Text(l10n.KeyGrokPrepaidBalance), Value: grokMoney(*snap.PrepaidBalanceCents)})
	}
	if text := grokOnDemandText(&snap); text != "" {
		q.Details = append(q.Details, runway.DetailLine{Title: lc.Text(l10n.KeyGrokOnDemandUsage), Value: text})
	}
	if snap.ResetCredits != nil {
		q.ResetCreditsKnown = true
		q.ResetCreditsUpdatedAt = snap.ResetCredits.UpdatedAt
		for _, token := range snap.ResetCredits.Tokens {
			if token.TokenID != "" && token.ValidityEnd != nil {
				q.ResetGrants = append(q.ResetGrants, runway.AccountResetGrant{Count: 1, EndsAt: *token.ValidityEnd})
			}
		}
	}

	return q, true
}

// grokIncludedUSD is "$used / $limit" when the included allowance is known.
func grokIncludedUSD(snap grok.BillingSnapshot) string {
	if snap.IncludedLimitCents == nil || *snap.IncludedLimitCents <= 0 {
		return ""
	}
	used := int64(0)
	if snap.IncludedUsedCents != nil && *snap.IncludedUsedCents > 0 {
		used = *snap.IncludedUsedCents
	}
	return grokCents(used) + " / " + grokCents(*snap.IncludedLimitCents)
}

func grokCents(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

// grokAccountQuotas collects the quota lines of all accounts by id.
func grokAccountQuotas(lc l10n.Localizer, accounts []grok.ManagedAccount) map[string]runway.GrokAccountQuota {
	var out map[string]runway.GrokAccountQuota
	for _, account := range accounts {
		q, ok := grokAccountQuota(lc, account)
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]runway.GrokAccountQuota{}
		}
		out[account.ID] = q
	}
	return out
}
