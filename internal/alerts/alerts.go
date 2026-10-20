package alerts

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Kind is the type of an alert; the raw values are the Swift ones.
type Kind string

const (
	KindQuota                  Kind = "quota"
	KindResetCredit            Kind = "resetCredit"
	KindRateLimitResetDetected Kind = "rateLimitResetDetected"
	KindRateLimitResetUpcoming Kind = "rateLimitResetUpcoming"
)

// ResetType is the kind of rate-limit reset.
type ResetType string

const (
	ResetGlobal          ResetType = "global"
	ResetBanked          ResetType = "banked"
	ResetGlobalAndBanked ResetType = "global_and_banked"
)

// Includes reports whether r covers t.
func (r ResetType) Includes(t ResetType) bool { return r == t || r == ResetGlobalAndBanked }

// MergeResetTypes combines reset types; "" when the list is empty.
func MergeResetTypes(types []ResetType) ResetType {
	var global, banked bool
	for _, t := range types {
		global = global || t.Includes(ResetGlobal)
		banked = banked || t.Includes(ResetBanked)
	}
	switch {
	case global && banked:
		return ResetGlobalAndBanked
	case banked:
		return ResetBanked
	case global:
		return ResetGlobal
	}
	return ""
}

// ScheduleBasis says how a scheduled reset was derived.
type ScheduleBasis string

const (
	BasisExplicit            ScheduleBasis = "explicit"
	BasisContextualInference ScheduleBasis = "contextual_inference"
)

// Alert is one notification candidate. The JSON field names are the Swift ones;
// optional fields are omitted when nil.
type Alert struct {
	ID                string         `json:"id"`
	Kind              Kind           `json:"kind"`
	Name              string         `json:"name"`
	Threshold         *int           `json:"threshold,omitempty"`
	Date              *time.Time     `json:"date,omitempty"`
	EndDate           *time.Time     `json:"endDate,omitempty"`
	ResetType         *ResetType     `json:"resetType,omitempty"`
	ScheduleBasis     *ScheduleBasis `json:"scheduleBasis,omitempty"`
	ConfidencePercent *int           `json:"confidencePercent,omitempty"`
}

// Window is a quota window as far as alerts care.
type Window struct {
	UsedPercent float64
	ResetsAt    *time.Time
}

// NamedWindow is an additional (for example model-specific) quota window.
type NamedWindow struct {
	Name string
	Window
}

// QuotaInput is the quota snapshot reduced to its windows.
type QuotaInput struct {
	Primary    Window
	Secondary  *Window
	Additional []NamedWindow
}

// QuotaAlerts returns one alert per window at or over a threshold, using the
// highest threshold reached. The ID repeats until the window resets, so a
// store keeps it from alerting again within the same window.
func QuotaAlerts(q QuotaInput, thresholds []int) []Alert {
	rows := []NamedWindow{{"5-hour", q.Primary}}
	if q.Secondary != nil {
		rows = append(rows, NamedWindow{"Weekly", *q.Secondary})
	}
	rows = append(rows, q.Additional...)
	var out []Alert
	for _, row := range rows {
		threshold := -1
		if math.IsNaN(row.UsedPercent) || math.IsInf(row.UsedPercent, 0) || row.UsedPercent < 0 {
			continue
		}
		for _, t := range thresholds {
			if t >= 0 && t <= 100 && row.UsedPercent >= float64(t) && t > threshold {
				threshold = t
			}
		}
		if threshold < 0 {
			continue
		}
		var resetID int64
		if row.ResetsAt != nil {
			resetID = row.ResetsAt.Unix()
		}
		th := threshold
		out = append(out, Alert{
			ID:        fmt.Sprintf("quota:%s:%d:%d", row.Name, threshold, resetID),
			Kind:      KindQuota,
			Name:      row.Name,
			Threshold: &th,
			Date:      row.ResetsAt,
		})
	}
	return out
}

// ExpiringThreshold is how close to its expiry an available reset credit
// counts as expiring.
const ExpiringThreshold = 7 * 24 * time.Hour

// ResetCredit is a reset credit as far as alerts care.
type ResetCredit struct {
	ID               *string
	Status           string
	ExpiresAt        *time.Time
	RemainingSeconds float64
}

// Expiring reports an available credit with an expiry within ExpiringThreshold.
func (c ResetCredit) Expiring() bool {
	if c.Status != "available" || c.ExpiresAt == nil {
		return false
	}
	return c.RemainingSeconds <= ExpiringThreshold.Seconds()
}

// ResetCreditAlerts returns one alert per expiring credit.
func ResetCreditAlerts(credits []ResetCredit) []Alert {
	var out []Alert
	for _, c := range credits {
		if !c.Expiring() {
			continue
		}
		expiryID := c.ExpiresAt.Unix()
		creditID := fmt.Sprintf("%d", expiryID)
		if c.ID != nil {
			creditID = *c.ID
		}
		out = append(out, Alert{
			ID:   fmt.Sprintf("reset-credit:%s:%d", creditID, expiryID),
			Kind: KindResetCredit,
			Name: creditID,
			Date: c.ExpiresAt,
		})
	}
	return out
}

// Event is a reset-today event; only completed ones matter to the detection.
type Event struct {
	PostID    string
	Completed bool
	// Date is effectiveAt, or announcedAt when there is no effective time.
	Date      time.Time
	ResetType ResetType
}

// ManualCompletion is a reset the operator confirmed by hand.
type ManualCompletion struct {
	ID                   string
	CompletedAt          time.Time
	VisibleUntil         time.Time
	RepresentativePostID string
	SchedulePostIDs      []string
	// ScheduleTypes are the reset types of the completion's schedules.
	ScheduleTypes []ResetType
}

// ResetType merges the schedules' types; global when there are none.
func (m ManualCompletion) ResetType() ResetType {
	if t := MergeResetTypes(m.ScheduleTypes); t != "" {
		return t
	}
	return ResetGlobal
}

// UpcomingReset is the next scheduled reset (the Swift
// nextScheduledResetSummary).
type UpcomingReset struct {
	EffectiveAt    time.Time
	EffectiveUntil time.Time
	IsRange        bool
	PostID         string
	ResetType      ResetType
	// Confidence is 0..1.
	Confidence    float64
	ScheduleBasis *ScheduleBasis
}

// ResetTodayState is one fetched reset-today snapshot reduced to what the
// alerts read.
type ResetTodayState struct {
	// Unavailable is a verdict of reason "unavailable" at the decision time
	// (monitor not ok, or data stale); no alert is produced for it.
	Unavailable       bool
	Events            []Event
	ManualCompletions []ManualCompletion
	// Upcoming is the next scheduled reset, nil when there is none.
	Upcoming *UpcomingReset
}

type occurrence struct {
	id        string
	aliases   map[string]struct{}
	name      string
	date      time.Time
	resetType ResetType
}

// ResetTodayAlerts alerts when a reset became newly detected today, or when a
// scheduled reset is within 30 or 60 minutes. "Today" is the calendar day of
// now in loc (the viewer's local day), not the UTC prefix of the feed times.
// previous is nil on the first successful load, which stays silent for
// detection so a launch does not announce existing resets.
func ResetTodayAlerts(previous *ResetTodayState, current ResetTodayState, now time.Time, loc *time.Location) []Alert {
	if current.Unavailable {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	var alerts []Alert
	if previous != nil {
		seen := map[string]struct{}{}
		for _, o := range resetOccurrences(*previous, now, loc) {
			for a := range o.aliases {
				seen[a] = struct{}{}
			}
		}
		day := startOfDay(now, loc).Unix()
		for _, o := range resetOccurrences(current, now, loc) {
			if overlaps(o.aliases, seen) {
				continue
			}
			date, rt := o.date, o.resetType
			alerts = append(alerts, Alert{
				ID:        fmt.Sprintf("rate-limit-reset:detected:%s:%d", o.id, day),
				Kind:      KindRateLimitResetDetected,
				Name:      o.name,
				Date:      &date,
				ResetType: &rt,
			})
		}
	}
	if next := current.Upcoming; next != nil {
		remaining := next.EffectiveAt.Sub(now).Seconds()
		if remaining <= 0 {
			return alerts
		}
		// The tighter threshold wins so a late refresh fires only once.
		minutes := int(math.Ceil(remaining / 60))
		threshold := 0
		switch {
		case minutes <= 30:
			threshold = 30
		case minutes <= 60:
			threshold = 60
		}
		if threshold != 0 {
			date, rt := next.EffectiveAt, next.ResetType
			percent := DisplayedPercent(next.Confidence)
			a := Alert{
				ID:                fmt.Sprintf("rate-limit-reset:upcoming:%s:%d:%d", next.PostID, threshold, next.EffectiveAt.Unix()),
				Kind:              KindRateLimitResetUpcoming,
				Name:              next.PostID,
				Threshold:         &threshold,
				Date:              &date,
				ResetType:         &rt,
				ScheduleBasis:     next.ScheduleBasis,
				ConfidencePercent: &percent,
			}
			if next.IsRange {
				end := next.EffectiveUntil
				a.EndDate = &end
			}
			alerts = append(alerts, a)
		}
	}
	return alerts
}

// DisplayedPercent is the whole percentage shown for a 0..1 confidence.
func DisplayedPercent(confidence float64) int { return int(math.Round(confidence * 100)) }

func startOfDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func sameDay(a, b time.Time, loc *time.Location) bool {
	ay, am, ad := a.In(loc).Date()
	by, bm, bd := b.In(loc).Date()
	return ay == by && am == bm && ad == bd
}

func overlaps(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}

func resetOccurrences(s ResetTodayState, now time.Time, loc *time.Location) []occurrence {
	var out []occurrence
	for _, e := range s.Events {
		if !e.Completed || e.Date.After(now) || !sameDay(e.Date, now, loc) {
			continue
		}
		out = append(out, occurrence{
			id: e.PostID, aliases: map[string]struct{}{e.PostID: {}}, name: e.PostID, date: e.Date, resetType: e.ResetType,
		})
	}
	for _, m := range s.ManualCompletions {
		if now.After(m.VisibleUntil) || m.CompletedAt.After(now) || !sameDay(m.CompletedAt, now, loc) {
			continue
		}
		timeID := fmt.Sprintf("at:%d", m.CompletedAt.Unix())
		aliases := map[string]struct{}{timeID: {}, m.ID: {}, m.RepresentativePostID: {}}
		for _, p := range m.SchedulePostIDs {
			aliases[p] = struct{}{}
		}
		out = append(out, occurrence{id: timeID, aliases: aliases, name: m.RepresentativePostID, date: m.CompletedAt, resetType: m.ResetType()})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].date.Equal(out[j].date) {
			return out[i].id < out[j].id
		}
		return out[i].date.Before(out[j].date)
	})
	return coalesce(out)
}

// coalesce merges occurrences that share an alias into the first of them.
func coalesce(in []occurrence) []occurrence {
	var unique []occurrence
	for _, o := range in {
		merged := false
		for i := range unique {
			if overlaps(unique[i].aliases, o.aliases) {
				for a := range o.aliases {
					unique[i].aliases[a] = struct{}{}
				}
				merged = true
				break
			}
		}
		if !merged {
			unique = append(unique, o)
		}
	}
	return unique
}
