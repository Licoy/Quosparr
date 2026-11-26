package resettoday

import (
	"sort"
	"time"
)

// Snapshot is one decoded status feed with its state resolved at FetchedAt.
// It is immutable: every method takes the instant and calendar to evaluate at.
type Snapshot struct {
	SchemaVersion         int
	GeneratedAt           time.Time
	LastSuccessfulCheckAt *time.Time
	Monitor               Monitor
	Events                []Event
	GraceSchedules        []Event
	ResetTimeline         *Timeline
	// FetchedAt is the now the snapshot was decoded with.
	FetchedAt time.Time
	// Feed is the monitor that published the snapshot (zero: CodexFeed).
	Feed Feed

	state         State
	stateOverride *State
}

// State is the state resolved when the snapshot was decoded.
func (s *Snapshot) State() State { return s.state }

// NewStateSnapshot is the compatibility constructor for fixtures that only
// know a state (Swift init(state:fetchedAt:)).
func NewStateSnapshot(state State, fetchedAt time.Time) *Snapshot {
	s := &Snapshot{
		SchemaVersion:  1,
		GeneratedAt:    fetchedAt,
		Monitor:        Monitor{Status: MonitorOK},
		Events:         []Event{},
		GraceSchedules: []Event{},
		FetchedAt:      fetchedAt,
		state:          state,
		stateOverride:  &state,
	}
	if state == StateUnknown {
		code := "mock_unavailable"
		s.Monitor = Monitor{Status: MonitorDegraded, ErrorCode: &code}
	} else {
		at := fetchedAt
		s.LastSuccessfulCheckAt = &at
	}
	return s
}

func newSnapshot(r response, now time.Time, loc *time.Location) *Snapshot {
	s := &Snapshot{
		SchemaVersion:         r.schemaVersion,
		GeneratedAt:           r.generatedAt,
		LastSuccessfulCheckAt: r.lastSuccessfulCheckAt,
		Monitor:               r.monitor,
		Events:                r.events,
		GraceSchedules:        r.graceSchedules,
		ResetTimeline:         r.resetTimeline,
		FetchedAt:             now,
	}
	s.state = s.ResolvedState(now, loc)
	return s
}

// HasResetTimeline: official schemaVersion 1 feeds may omit it; when present
// it is authoritative.
func (s *Snapshot) HasResetTimeline() bool { return s.ResetTimeline != nil }

// FreshnessAt is the newest producer timestamp, used to decide staleness.
func (s *Snapshot) FreshnessAt() time.Time {
	at := s.GeneratedAt
	if s.LastSuccessfulCheckAt != nil && s.LastSuccessfulCheckAt.After(at) {
		at = *s.LastSuccessfulCheckAt
	}
	return at
}

func (s *Snapshot) isHealthy(now time.Time) bool {
	return s.Monitor.Status == MonitorOK && now.Sub(s.FreshnessAt()) <= StaleAfter
}

// ResolvedState re-evaluates the answer at now. Yes means the local calendar
// day has a reset, already effective or still scheduled later today. Only a
// degraded, missing or stale monitor is unknown; a healthy feed with only
// uncertain events means no.
func (s *Snapshot) ResolvedState(now time.Time, loc *time.Location) State {
	if s.stateOverride != nil {
		return *s.stateOverride
	}
	if !s.isHealthy(now) {
		return StateUnknown
	}
	if s.HasResetOnLocalDay(now, loc) {
		return StateYes
	}
	return StateNo
}

// HasUncertainNoSignalToday: the "no" is explained by same-day uncertain
// commentary rather than an empty event log.
func (s *Snapshot) HasUncertainNoSignalToday(now time.Time, loc *time.Location) bool {
	loc = locOrLocal(loc)
	if s.ResolvedState(now, loc) != StateNo {
		return false
	}
	for _, e := range s.Events {
		if e.Kind == KindUncertain && sameDay(e.AnnouncedAt, now, loc) {
			return true
		}
	}
	return false
}

// HasAlreadyEffectiveResetToday: a reset took effect on the local day.
func (s *Snapshot) HasAlreadyEffectiveResetToday(now time.Time, loc *time.Location) bool {
	loc = locOrLocal(loc)
	return len(s.completedResetEventsToday(now, loc)) > 0 || s.visibleManualCompletion(now, loc) != nil
}

// PrefersSameDayScheduleExplanation: the primary explanation is a pending
// same-day schedule. Timeline feeds make a confirmed completion primary.
func (s *Snapshot) PrefersSameDayScheduleExplanation(now time.Time, loc *time.Location) bool {
	loc = locOrLocal(loc)
	if s.HasResetTimeline() && s.HasAlreadyEffectiveResetToday(now, loc) {
		return false
	}
	return s.NextScheduledResetOnLocalDay(now, loc) != nil
}

// HasResetOnLocalDay: any reset, completed or scheduled, lands on the local day.
func (s *Snapshot) HasResetOnLocalDay(now time.Time, loc *time.Location) bool {
	loc = locOrLocal(loc)
	if s.HasAlreadyEffectiveResetToday(now, loc) {
		return true
	}
	// Prefer same-day schedules even when an earlier future schedule falls tomorrow.
	return s.NextScheduledResetOnLocalDay(now, loc) != nil
}

// LatestEvent is the newest announced event.
func (s *Snapshot) LatestEvent() *Event {
	var best *Event
	for i := range s.Events {
		if best == nil || best.AnnouncedAt.Before(s.Events[i].AnnouncedAt) {
			best = &s.Events[i]
		}
	}
	return best
}

// ResetOccurrence is a reset that has taken effect.
type ResetOccurrence struct {
	At        time.Time
	ResetType ResetType
}

// LatestReset is the newest reset at or before now; same-instant resets merge
// their types.
func (s *Snapshot) LatestReset(now time.Time) *ResetOccurrence {
	var all []ResetOccurrence
	for _, e := range s.Events {
		if at := e.resetOccurrenceAt(); at != nil && !at.After(now) {
			all = append(all, ResetOccurrence{*at, e.ResetType})
		}
	}
	if s.ResetTimeline != nil {
		for _, m := range s.ResetTimeline.ManualCompletions {
			if !m.CompletedAt.After(now) {
				all = append(all, ResetOccurrence{m.CompletedAt, m.ResetType()})
			}
		}
	}
	if len(all) == 0 {
		return nil
	}
	latest := all[0].At
	for _, o := range all {
		if o.At.After(latest) {
			latest = o.At
		}
	}
	var types []ResetType
	for _, o := range all {
		if o.At.Equal(latest) {
			types = append(types, o.ResetType)
		}
	}
	t, ok := mergeTypes(types)
	if !ok {
		return nil
	}
	return &ResetOccurrence{At: latest, ResetType: t}
}

// LatestResetAt is the time of LatestReset.
func (s *Snapshot) LatestResetAt(now time.Time) *time.Time {
	if r := s.LatestReset(now); r != nil {
		return &r.At
	}
	return nil
}

// DisplayResetType is the type to show today: the merged completed types, else
// the next same-day schedule's type; nil when the state is unknown.
func (s *Snapshot) DisplayResetType(now time.Time, loc *time.Location) *ResetType {
	loc = locOrLocal(loc)
	if s.ResolvedState(now, loc) == StateUnknown {
		return nil
	}
	var types []ResetType
	for _, e := range s.completedResetEventsToday(now, loc) {
		types = append(types, e.ResetType)
	}
	for _, m := range s.VisibleManualCompletions(now) {
		if !m.CompletedAt.After(now) && sameDay(m.CompletedAt, now, loc) {
			types = append(types, m.ResetType())
		}
	}
	if t, ok := mergeTypes(types); ok {
		return &t
	}
	if next := s.NextScheduledResetOnLocalDay(now, loc); next != nil {
		t := next.Event.ResetType
		return &t
	}
	return nil
}

// UnconfirmedExpiredSchedule is the hosted "no" copy case where a published
// schedule expired without a completion. resetTimeline.nextSchedule is
// authoritative: a date window may still be open locally after the producer
// cleared nextSchedule and pointed recentNonCompletedPostId at the event.
func (s *Snapshot) UnconfirmedExpiredSchedule(now time.Time, loc *time.Location) *Event {
	if s.ResolvedState(now, loc) != StateNo || s.ResetTimeline == nil || s.ResetTimeline.RecentNonCompletedPostID == nil {
		return nil
	}
	postID := *s.ResetTimeline.RecentNonCompletedPostID
	var event *Event
	for i := range s.Events {
		if s.Events[i].Source.PostID == postID {
			event = &s.Events[i]
			break
		}
	}
	if event == nil || event.Kind != KindResetScheduled {
		return nil
	}
	if next := s.NextScheduledReset(now); next != nil && next.Event.Source.PostID == postID {
		return nil
	}
	return event
}

// NextScheduledReset is the next pending reset_scheduled window, if any.
func (s *Snapshot) NextScheduledReset(now time.Time) *PendingReset {
	return s.nextScheduledReset(now, nil, nil)
}

// NextScheduledResetOnLocalDay is the next pending schedule that intersects
// the local day of now.
func (s *Snapshot) NextScheduledResetOnLocalDay(now time.Time, loc *time.Location) *PendingReset {
	return s.nextScheduledReset(now, &now, locOrLocal(loc))
}

// ScheduledResetWindow is the window of a scheduled event, nil for other kinds.
//
// Explicit producer precision is authoritative. Only legacy feeds that omit
// schedulePrecision keep the midnight-in-the-source-calendar-is-a-date rule.
func (s *Snapshot) ScheduledResetWindow(e Event) *ScheduleWindow {
	if e.Kind != KindResetScheduled || e.EffectiveAt == nil {
		return nil
	}
	start := *e.EffectiveAt
	src := sourceLocation()
	var isRange bool
	switch e.SchedulePrecision {
	case PrecisionDate:
		isRange = true
	case PrecisionDatetime:
		isRange = false
	default:
		l := start.In(src)
		isRange = l.Hour() == 0 && l.Minute() == 0 && l.Second() == 0
	}
	if !isRange {
		return &ScheduleWindow{StartAt: start, EndAt: start}
	}
	return &ScheduleWindow{
		StartAt: startOfDay(start, src),
		EndAt:   startOfNextDay(start, src).Add(-time.Minute),
		IsRange: true,
	}
}

// VisibleManualCompletions are the manual completions still shown at now,
// newest first (ties by representative post ID, descending).
func (s *Snapshot) VisibleManualCompletions(now time.Time) []ManualCompletion {
	if s.ResetTimeline == nil {
		return nil
	}
	var out []ManualCompletion
	for _, m := range s.ResetTimeline.ManualCompletions {
		if !now.After(m.VisibleUntil) {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CompletedAt.Equal(out[j].CompletedAt) {
			return out[i].CompletedAt.After(out[j].CompletedAt)
		}
		return out[i].RepresentativePostID > out[j].RepresentativePostID
	})
	return out
}

func (s *Snapshot) nextScheduledReset(now time.Time, day *time.Time, loc *time.Location) *PendingReset {
	if s.HasResetTimeline() {
		return s.nextTimelineSchedule(now, day, loc)
	}
	var best *PendingReset
	for _, e := range s.Events {
		if c := s.pendingSchedule(e, now, day, loc); c != nil && (best == nil || c.EffectiveAt.Before(best.EffectiveAt)) {
			best = c
		}
	}
	return best
}

func (s *Snapshot) nextTimelineSchedule(now time.Time, day *time.Time, loc *time.Location) *PendingReset {
	t := s.ResetTimeline
	var best *PendingReset
	if t.NextSchedule != nil {
		best = s.pendingSchedule(*t.NextSchedule, now, day, loc)
	}
	suppressed := stringSet(t.SuppressedPostIDs)
	for _, e := range s.Events {
		if e.Kind != KindResetScheduled || !e.ResetType.Includes(ResetBanked) || suppressed[e.Source.PostID] {
			continue
		}
		if c := s.pendingSchedule(e, now, day, loc); c != nil && (best == nil || c.EffectiveAt.Before(best.EffectiveAt)) {
			best = c
		}
	}
	return best
}

func (s *Snapshot) pendingSchedule(e Event, now time.Time, day *time.Time, loc *time.Location) *PendingReset {
	w := s.ScheduledResetWindow(e)
	if w == nil || !w.PendingUntil().After(now) {
		return nil
	}
	if day != nil && loc != nil && !intersectsLocalDay(*w, *day, loc) {
		return nil
	}
	return &PendingReset{EffectiveAt: w.StartAt, EffectiveUntil: w.EndAt, IsRange: w.IsRange, Event: e}
}

func (s *Snapshot) completedResetEventsToday(now time.Time, loc *time.Location) []Event {
	var out []Event
	for _, e := range s.Events {
		if at := e.resetOccurrenceAt(); at != nil && !at.After(now) && sameDay(*at, now, loc) {
			out = append(out, e)
		}
	}
	return out
}

func (s *Snapshot) visibleManualCompletion(now time.Time, loc *time.Location) *ManualCompletion {
	for _, m := range s.VisibleManualCompletions(now) {
		if !m.CompletedAt.After(now) && sameDay(m.CompletedAt, now, loc) {
			m := m
			return &m
		}
	}
	return nil
}

func intersectsLocalDay(w ScheduleWindow, day time.Time, loc *time.Location) bool {
	start := startOfDay(day, loc)
	end := startOfNextDay(day, loc)
	return !w.EndAt.Before(start) && w.StartAt.Before(end)
}

func stringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, i := range items {
		set[i] = true
	}
	return set
}
