package runway

import (
	"strings"

	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/l10n"
)

// EstimateUnavailableText explains why no estimate could be extrapolated.
func EstimateUnavailableText(lc l10n.Localizer, reason estimate.UnavailableReason) string {
	key := l10n.KeyQuotaEstimateUnavailableWindow
	switch reason {
	case estimate.ReasonMissingCredits:
		key = l10n.KeyQuotaEstimateMissingCredits
	case estimate.ReasonZeroCreditsWithUsage:
		key = l10n.KeyQuotaEstimateZeroCreditsWithUsage
	case estimate.ReasonNoUsage:
		key = l10n.KeyQuotaEstimateNoUsage
	case estimate.ReasonStatsNotSynced:
		key = l10n.KeyQuotaEstimateStatsNotSynced
	case estimate.ReasonZeroPercent:
		key = l10n.KeyQuotaEstimateZeroPercent
	}
	return lc.Text(key)
}

// EstimateDataTimeText is "stats through <date> · updated <time>".
func EstimateDataTimeText(lc l10n.Localizer, s estimate.Snapshot) string {
	var parts []string
	if through := s.StatsThroughDate(); through != nil {
		parts = append(parts, lc.Textf(l10n.KeyQuotaEstimateStatsThrough, *through))
	}
	parts = append(parts, lc.Text(l10n.KeyLastUpdated)+" "+ResetCreditUpdatedAt(s.CalculatedAt, lc.Language(), nil))
	return strings.Join(parts, " · ")
}

// EstimateRefreshErrorText wraps a refresh error for display above the
// previous data; "" when there is no error.
func EstimateRefreshErrorText(lc l10n.Localizer, err string) string {
	if err == "" {
		return ""
	}
	return lc.Text(l10n.KeyQuotaEstimateShowingPreviousData) + "\n" + err
}
