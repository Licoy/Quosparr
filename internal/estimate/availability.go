package estimate

// UnavailableReason says why a Snapshot has no extrapolated estimate.
type UnavailableReason string

const (
	// ReasonMissingCredits: a day's totals or the Credits of a used day were
	// not reported, so the sum would be a guess.
	ReasonMissingCredits UnavailableReason = "missingCredits"
	// ReasonZeroCreditsWithUsage: the server reported zero Credits for days
	// that did use tokens.
	ReasonZeroCreditsWithUsage UnavailableReason = "zeroCreditsWithUsage"
	// ReasonNoUsage: no day of the cycle has any usage, and the quota window
	// shows none either.
	ReasonNoUsage UnavailableReason = "noUsage"
	// ReasonStatsNotSynced: the quota window shows usage, but no day of the
	// cycle has any yet; the daily statistics lag behind the window.
	ReasonStatsNotSynced UnavailableReason = "statsNotSynced"
	// ReasonZeroPercent: the quota window shows 0% used.
	ReasonZeroPercent UnavailableReason = "zeroPercent"
	// ReasonUnavailableWindow: there is no window with a length and a reset
	// time to anchor the cycle.
	ReasonUnavailableWindow UnavailableReason = "unavailableWindow"
)

func reasonPtr(r UnavailableReason) *UnavailableReason { return &r }

// evaluate is QuotaEstimateUnavailableReason.evaluate. The order of the checks
// decides which reason is reported when several apply.
func evaluate(meter *Window, rows []Row) *UnavailableReason {
	if meter == nil || meter.WindowMinutes == nil || *meter.WindowMinutes <= 0 || meter.ResetsAt == nil {
		return reasonPtr(ReasonUnavailableWindow)
	}
	for _, r := range rows {
		if !r.TotalsReported {
			return reasonPtr(ReasonMissingCredits)
		}
	}
	var used []Row
	for _, r := range rows {
		if r.hasUsage() {
			used = append(used, r)
		}
	}
	if len(used) == 0 {
		if meter.UsedPercentExact > 0 {
			return reasonPtr(ReasonStatsNotSynced)
		}
		return reasonPtr(ReasonNoUsage)
	}
	for _, r := range used {
		if !r.CreditsReported {
			return reasonPtr(ReasonMissingCredits)
		}
	}
	if !(meter.UsedPercentExact > 0) {
		return reasonPtr(ReasonZeroPercent)
	}
	for _, r := range used {
		if r.Credits > 0 {
			return nil
		}
	}
	return reasonPtr(ReasonZeroCreditsWithUsage)
}

// CreditsComplete is true when at least one row has reported Credits, every
// row has reported totals, and every used row has reported Credits.
func (s Snapshot) CreditsComplete() bool {
	any := false
	for _, r := range s.CurrentRows {
		if r.CreditsReported {
			any = true
		}
		if !r.TotalsReported || (r.hasUsage() && !r.CreditsReported) {
			return false
		}
	}
	return any
}

// APIEquivalentUSD is the token-priced cost of the cycle. It is separate from
// the Credits-to-USD conversion, and nil unless every row is known and every
// used row could be priced.
func (s Snapshot) APIEquivalentUSD() *float64 {
	if len(s.CurrentRows) == 0 {
		return nil
	}
	sum := 0.0
	for _, r := range s.CurrentRows {
		if !r.TotalsReported || (r.hasUsage() && r.USD == nil) {
			return nil
		}
		if r.USD != nil {
			sum += *r.USD
		}
	}
	return &sum
}

// StatsThroughDate is the latest day with a row, also one before the cycle,
// so lagging statistics show how far they reach; nil without rows.
func (s Snapshot) StatsThroughDate() *string {
	latest := s.LatestStatsDate
	for _, r := range s.CurrentRows {
		if r.Date > latest {
			latest = r.Date
		}
	}
	if latest == "" {
		return nil
	}
	return &latest
}
