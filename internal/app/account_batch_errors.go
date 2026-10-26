package app

import (
	"errors"
	"slices"
	"strings"

	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

// accountBatchFailure shows counts and distinct safe reasons, never legacy
// failure strings because those can contain account names or file paths.
func accountBatchFailure(l l10n.Localizer, succeeded, failed int, causes []error) string {
	if failed == 0 {
		return ""
	}
	return tx(l, xAccountBatchResult, succeeded, failed) + ": " + accountFailureReasons(l, causes)
}

func accountFailureReasons(l l10n.Localizer, causes []error) string {
	reasons := make([]string, 0, len(causes))
	for _, err := range causes {
		if reason := accountErrorReason(l, err); reason != "" && !slices.Contains(reasons, reason) {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, accountErrorReason(l, errors.New("unclassified batch failure")))
	}
	return strings.Join(reasons, "; ")
}

func grokRefreshMessage(l l10n.Localizer, report grok.RefreshReport) string {
	var failures []error
	for _, outcome := range report.Outcomes {
		if outcome.Err != nil {
			failures = append(failures, outcome.Err)
		}
	}
	return accountBatchFailure(l, len(report.Outcomes)-len(failures), len(failures), failures)
}
