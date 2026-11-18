package speed

import (
	"math"
	"sort"
	"time"
)

// groupKey identifies one model at one reasoning effort and tier.
type groupKey struct {
	model, effort string
	tier          Tier
}

// family is the model and effort of a key: the tiers of one family are
// listed together so that they can be compared.
func (k groupKey) family() [2]string { return [2]string{k.model, k.effort} }

// tierRank orders the tiers inside a family: Standard, Fast, Ultrafast, Unknown.
func tierRank(t Tier) int {
	switch t {
	case TierStandard:
		return 0
	case TierFast:
		return 1
	case TierUltrafast:
		return 2
	}
	return 3
}

// acc collects the raw numbers of one group before they become a Stat.
type acc struct {
	ttfts         []time.Duration
	outputSamples int
	outputTokens  int64
	decode        time.Duration
}

func (a *acc) add(s Sample) {
	if s.HasTTFT {
		a.ttfts = append(a.ttfts, s.TTFT)
	}
	if hasOutput(s) {
		a.outputSamples++
		a.outputTokens += s.OutputTokens
		a.decode += s.Decode
	}
}

// weight is how many samples back the group: it orders the lists.
func (a *acc) weight() int { return len(a.ttfts) + a.outputSamples }

func (a *acc) stat(k groupKey) Stat {
	sorted := append([]time.Duration(nil), a.ttfts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return Stat{
		Model: k.model, Effort: k.effort, Tier: k.tier,
		Calls:   len(sorted),
		TTFTP50: percentile(sorted, 50), TTFTP90: percentile(sorted, 90),
		OutputTokens: a.outputTokens, Decode: a.decode,
	}
}

func hasOutput(s Sample) bool { return s.OutputTokens > 0 && s.Decode > 0 }

// usable normalizes a sample and reports whether it carries a measurement: it
// needs a model, a time inside [from, to], and a TTFT or an output part. A
// negative TTFT or an output part without tokens or time is not a measurement.
func usable(s Sample, from, to time.Time) (Sample, bool) {
	if s.Model == "" || s.At.Before(from) || s.At.After(to) {
		return Sample{}, false
	}
	if s.TTFT < 0 {
		s.HasTTFT, s.TTFT = false, 0
	}
	if !s.HasTTFT {
		s.TTFT = 0
	}
	if !hasOutput(s) {
		s.OutputTokens, s.Decode = 0, 0
	}
	return s, s.HasTTFT || hasOutput(s)
}

// percentile is the nearest-rank percentile (1..100) of sorted durations, 0
// for none.
func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(float64(p) / 100 * float64(len(sorted))))
	return sorted[min(max(rank, 1), len(sorted))-1]
}

// groupStats turns the accumulators into stats, the model and effort with
// the most samples first and its tiers next to each other; ties are broken by
// model and effort so the order never depends on map iteration.
func groupStats(groups map[groupKey]*acc) []Stat {
	keys := make([]groupKey, 0, len(groups))
	families := map[[2]string]int{}
	for k, a := range groups {
		keys = append(keys, k)
		families[k.family()] += a.weight()
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if wa, wb := families[a.family()], families[b.family()]; wa != wb {
			return wa > wb
		}
		if a.model != b.model {
			return a.model < b.model
		}
		if a.effort != b.effort {
			return a.effort < b.effort
		}
		return tierRank(a.tier) < tierRank(b.tier)
	})
	stats := make([]Stat, 0, len(keys))
	for _, k := range keys {
		stats = append(stats, groups[k].stat(k))
	}
	return stats
}

// sessionAcc is one session's samples while a report is built.
type sessionAcc struct {
	session Session
	groups  map[groupKey]*acc
}

func addTo(groups map[groupKey]*acc, s Sample) {
	k := groupKey{s.Model, s.Effort, s.Tier}
	if groups[k] == nil {
		groups[k] = &acc{}
	}
	groups[k].add(s)
}

// Build turns the sessions' samples into a report. Samples without a model, or
// older than Window before now, or after now, are dropped, as are samples with
// nothing to measure; a session left without samples is not listed. Without
// any Fast or Ultrafast sample the tiers are not told apart.
func Build(sessions []Session, now time.Time) Report {
	from := now.Add(-Window)
	tiered := hasAcceleratedSample(sessions, from, now)
	all := map[groupKey]*acc{}
	var kept []sessionAcc
	var first, last time.Time
	for _, session := range sessions {
		sa := sessionAcc{session: session, groups: map[groupKey]*acc{}}
		for _, raw := range session.Samples {
			s, ok := usable(raw, from, now)
			if !ok {
				continue
			}
			if !tiered {
				s.Tier = TierUnknown
			}
			addTo(all, s)
			addTo(sa.groups, s)
			if first.IsZero() || s.At.Before(first) {
				first = s.At
			}
			if s.At.After(last) {
				last = s.At
			}
		}
		if len(sa.groups) > 0 {
			kept = append(kept, sa)
		}
	}
	return Report{
		Groups:       groupStats(all),
		Sessions:     sessionStats(kept),
		Tiered:       tiered,
		From:         first,
		To:           last,
		CalculatedAt: now,
	}
}

func hasAcceleratedSample(sessions []Session, from, to time.Time) bool {
	for _, session := range sessions {
		for _, raw := range session.Samples {
			if _, ok := usable(raw, from, to); ok && (raw.Tier == TierFast || raw.Tier == TierUltrafast) {
				return true
			}
		}
	}
	return false
}

// sessionStats lists the sessions newest first.
func sessionStats(kept []sessionAcc) []SessionStat {
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i].session, kept[j].session
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})
	var stats []SessionStat
	for _, sa := range kept {
		stats = append(stats, SessionStat{
			ID: sa.session.ID, Title: sa.session.Title, ProjectName: sa.session.ProjectName,
			UpdatedAt: sa.session.UpdatedAt, Groups: groupStats(sa.groups),
		})
	}
	return stats
}
