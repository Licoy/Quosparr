package apiquota

import "time"

// round holds the answers of one probe round: the billing endpoints under one
// API root with the optional public token and status endpoints, or the DeepSeek
// balance endpoint.
type round struct {
	urls        billingURLs
	tokenURL    string
	deepSeekURL string
	grants      hit
	sub         hit
	usage       hit
	token       hit
	status      hit
	deepSeek    hit
}

// firstInvalid remembers the first inconsistency so it can be reported when
// no endpoint yields a usable quota.
type firstInvalid struct {
	msg, endpoint string
	set           bool
}

func (f *firstInvalid) keep(msg, endpoint string) {
	if !f.set {
		*f = firstInvalid{msg, endpoint, true}
	}
}

// normalizeToken applies the status metadata to a raw token hit and turns an
// inconsistent one into an invalid-data hit.
func (r round) normalizeToken() hit {
	if r.token.kind != hitToken {
		return r.token
	}
	var status *quotaStatus
	if r.status.kind == hitStatus {
		status = &r.status.status
	}
	t, err := normalizeTokenUsage(r.token.token, status)
	if err != nil {
		return hit{kind: hitInvalidData, msg: err.Error()}
	}
	return hit{kind: hitToken, token: t}
}

// interpret picks the best quota of a round. Source priority is DeepSeek
// balance, token usage, credit grants, subscription with usage, subscription
// alone, usage alone. The token hit must already be normalized (see
// normalizeToken).
func (r round) interpret(at time.Time) Quota {
	var invalid firstInvalid
	if r.deepSeek.kind == hitInvalidData {
		invalid.keep(r.deepSeek.msg, r.deepSeekURL)
	}
	if r.token.kind == hitInvalidData {
		invalid.keep(r.token.msg, r.tokenURL)
	}
	if q, ok := r.deepSeekQuota(&invalid, at); ok {
		return q
	}
	if q, ok := r.tokenQuota(&invalid, at); ok {
		return q
	}
	if q, ok := r.grantsQuota(&invalid, at); ok {
		return q
	}
	if q, ok := r.billingQuota(&invalid, at); ok {
		return q
	}
	return r.quiet(invalid, at)
}

func (r round) subExpiry() int64 {
	if r.sub.kind == hitSubscription {
		return r.sub.sub.expiresAt
	}
	return 0
}

// deepSeekQuota reads the official DeepSeek balance. It reports the remaining
// amount only: DeepSeek does not publish a quota total, so there is no used
// share to compute.
func (r round) deepSeekQuota(invalid *firstInvalid, at time.Time) (Quota, bool) {
	if r.deepSeek.kind != hitDeepSeekBalance {
		return Quota{}, false
	}
	b := r.deepSeek.deepSeek
	remaining := b.total
	q, err := available(SourceDeepSeekBalance, &remaining, nil, nil, false, b.currency, 0, r.deepSeekURL, at)
	if err != nil {
		invalid.keep(err.Error(), r.deepSeekURL)
		return Quota{}, false
	}
	return q, true
}

func (r round) tokenQuota(invalid *firstInvalid, at time.Time) (Quota, bool) {
	if r.token.kind != hitToken {
		return Quota{}, false
	}
	t := r.token.token
	expires := t.expiresAt
	if expires == 0 {
		expires = r.subExpiry()
	}
	q, err := available(SourceTokenUsage, t.remaining, t.used, t.total, t.unlimited, t.unit, expires, r.tokenURL, at)
	if err != nil {
		invalid.keep(err.Error(), r.tokenURL)
		return Quota{}, false
	}
	return q, true
}

func (r round) grantsQuota(invalid *firstInvalid, at time.Time) (Quota, bool) {
	if r.grants.kind != hitGrants {
		return Quota{}, false
	}
	g := r.grants.grants
	q, err := available(SourceCreditGrants, g.remaining, g.used, g.total, false, UnitUSD, r.subExpiry(), r.urls.creditGrants, at)
	if err != nil {
		invalid.keep(err.Error(), r.urls.creditGrants)
		return Quota{}, false
	}
	return q, true
}

// billingQuota reads the legacy dashboard endpoints: subscription limit with
// month usage, the limit alone, or the usage alone.
func (r round) billingQuota(invalid *firstInvalid, at time.Time) (Quota, bool) {
	hasSub, hasUsage := r.sub.kind == hitSubscription, r.usage.kind == hitUsage
	var q Quota
	var err error
	var endpoint string
	switch {
	case hasSub && hasUsage:
		s := r.sub.sub
		used := usageToUSD(r.usage.usage.totalUsage)
		var remaining *float64
		if s.limitUSD != nil {
			rem := *s.limitUSD - used
			remaining = &rem
		}
		endpoint = r.urls.subscription
		q, err = available(SourceSubscriptionUsage, remaining, &used, s.limitUSD, false, UnitUSD, s.expiresAt, endpoint, at)
	case hasSub && r.sub.sub.limitUSD != nil:
		s := r.sub.sub
		endpoint = r.urls.subscription
		q, err = available(SourceSubscriptionOnly, nil, nil, s.limitUSD, false, UnitUSD, s.expiresAt, endpoint, at)
	case hasUsage:
		used := usageToUSD(r.usage.usage.totalUsage)
		endpoint = r.urls.usage
		q, err = available(SourceUsageOnly, nil, &used, nil, false, UnitUSD, r.subExpiry(), endpoint, at)
	default:
		return Quota{}, false
	}
	if err != nil {
		invalid.keep(err.Error(), endpoint)
		return Quota{}, false
	}
	return q, true
}

// quiet builds the no-quota result: invalid data first, then unauthorized,
// then the first transport or upstream error, else unsupported.
func (r round) quiet(invalid firstInvalid, at time.Time) Quota {
	if invalid.set {
		return quiet(StatusInvalidData, invalid.msg, invalid.endpoint, at)
	}
	hits := [...]hit{r.grants, r.sub, r.usage, r.token, r.deepSeek}
	for _, h := range hits {
		if h.kind == hitUnauthorized {
			return quiet(StatusUnauthorized, "", "", at)
		}
	}
	for _, h := range hits {
		if h.kind == hitError {
			return quiet(StatusError, h.msg, "", at)
		}
	}
	return quiet(StatusUnsupported, "", "", at)
}

func quiet(status Status, msg, endpoint string, at time.Time) Quota {
	return Quota{Status: status, Error: msg, Endpoint: endpoint, FetchedAt: at}
}

// available builds a usable quota after the consistency check. expiresAt is a
// unix timestamp in seconds, 0 for none.
func available(source Source, remaining, used, total *float64, unlimited bool, unit string, expiresAt int64, endpoint string, at time.Time) (Quota, error) {
	if !valuesConsistent(remaining, used, total, unlimited) {
		return Quota{}, errInconsistent
	}
	q := Quota{
		Status:    StatusAvailable,
		Used:      used,
		Unlimited: unlimited,
		Unit:      unit,
		Source:    source,
		Endpoint:  endpoint,
		FetchedAt: at,
	}
	if expiresAt > 0 {
		t := time.Unix(expiresAt, 0).UTC()
		q.ExpiresAt = &t
	}
	if !unlimited {
		q.Remaining = clampRemaining(remaining)
		q.Total = total
	}
	return q, nil
}

func clampRemaining(v *float64) *float64 {
	if v == nil {
		return nil
	}
	r := max(*v, 0)
	return &r
}

// combine merges the results of two rounds. A usable quota beats a failure;
// between two usable quotas the higher-priority source wins, between two
// failures the stronger diagnostic wins. Ties keep the first.
func combine(first, second Quota) Quota {
	firstOK, secondOK := first.Status == StatusAvailable, second.Status == StatusAvailable
	switch {
	case firstOK && secondOK:
		if first.Source.rank() >= second.Source.rank() {
			return first
		}
		return second
	case firstOK:
		return first
	case secondOK:
		return second
	case first.Status.rank() >= second.Status.rank():
		return first
	}
	return second
}

// needsFallback reports whether a second round at the origin without /v1
// could still improve the result: anything but token usage or credit grants.
func needsFallback(q Quota) bool {
	return q.Status != StatusAvailable || q.Source.rank() < SourceCreditGrants.rank()
}
