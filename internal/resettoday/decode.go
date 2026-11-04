package resettoday

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

// DecodeError is returned for a feed that is not valid schema version 1. The
// description never contains feed text beyond field names.
type DecodeError struct{ Reason string }

func (e *DecodeError) Error() string { return "reset-status feed: " + e.Reason }

func invalid(format string, args ...any) error {
	return &DecodeError{Reason: fmt.Sprintf(format, args...)}
}

// Decode parses and validates a status feed and resolves its state at now in
// loc, like RateLimitResetTodaySnapshot.decode(from:now:calendar:). The
// decoding is as strict as the Swift Decodable one: wrong types, explicit
// nulls where the Swift code rejects them, unknown enum values and timestamps
// without a time zone offset are all errors.
func Decode(data []byte, now time.Time, loc *time.Location) (*Snapshot, error) {
	resp, err := decodeResponse(data)
	if err != nil {
		return nil, err
	}
	if resp.schemaVersion != 1 {
		return nil, invalid("Unsupported reset-status schema version.")
	}
	if err := validateResponse(resp); err != nil {
		return nil, err
	}
	return newSnapshot(resp, now, loc), nil
}

type response struct {
	schemaVersion         int
	generatedAt           time.Time
	lastSuccessfulCheckAt *time.Time
	monitor               Monitor
	events                []Event
	graceSchedules        []Event
	resetTimeline         *Timeline
}

// obj is a decoded JSON object that remembers which keys were present, which
// the Swift rules need (an explicit null differs from a missing key).
type obj map[string]json.RawMessage

func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

func parseObject(raw json.RawMessage, label string) (obj, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, invalid("%s must be an object.", label)
	}
	var o obj
	if err := json.Unmarshal(trimmed, &o); err != nil {
		return nil, invalid("%s is not valid JSON.", label)
	}
	return o, nil
}

func (o obj) has(key string) bool { _, ok := o[key]; return ok }

// optional returns the value when the key is present and not null.
func (o obj) optional(key string) (json.RawMessage, bool) {
	raw, ok := o[key]
	if !ok || isNull(raw) {
		return nil, false
	}
	return raw, true
}

func (o obj) required(key string) (json.RawMessage, error) {
	raw, ok := o[key]
	if !ok || isNull(raw) {
		return nil, invalid("%s is required.", key)
	}
	return raw, nil
}

func decodeString(raw json.RawMessage, key string) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", invalid("%s must be a string.", key)
	}
	return s, nil
}

func (o obj) str(key string) (string, error) {
	raw, err := o.required(key)
	if err != nil {
		return "", err
	}
	return decodeString(raw, key)
}

func (o obj) optStr(key string) (*string, error) {
	raw, ok := o.optional(key)
	if !ok {
		return nil, nil
	}
	s, err := decodeString(raw, key)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func decodeDate(raw json.RawMessage, key string) (time.Time, error) {
	s, err := decodeString(raw, key)
	if err != nil {
		return time.Time{}, err
	}
	t, err := swiftjson.ParseDate(s)
	if err != nil {
		return time.Time{}, invalid("%s must be an RFC 3339 timestamp with timezone offset.", key)
	}
	return t.UTC(), nil
}

func (o obj) date(key string) (time.Time, error) {
	raw, err := o.required(key)
	if err != nil {
		return time.Time{}, err
	}
	return decodeDate(raw, key)
}

func (o obj) optDate(key string) (*time.Time, error) {
	raw, ok := o.optional(key)
	if !ok {
		return nil, nil
	}
	t, err := decodeDate(raw, key)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func decodeFloat(raw json.RawMessage, key string) (float64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '"' {
		return 0, invalid("%s must be a number.", key)
	}
	var f float64
	if err := json.Unmarshal(trimmed, &f); err != nil {
		return 0, invalid("%s must be a number.", key)
	}
	return f, nil
}

func (o obj) float(key string) (float64, error) {
	raw, err := o.required(key)
	if err != nil {
		return 0, err
	}
	return decodeFloat(raw, key)
}

func (o obj) stringList(key string) ([]string, error) {
	raw, err := o.required(key)
	if err != nil {
		return nil, err
	}
	return decodeStringList(raw, key)
}

func decodeStringList(raw json.RawMessage, key string) ([]string, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, invalid("%s must be an array.", key)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if isNull(item) {
			return nil, invalid("%s must not contain null.", key)
		}
		s, err := decodeString(item, key)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (o obj) object(key string) (obj, error) {
	raw, err := o.required(key)
	if err != nil {
		return nil, err
	}
	return parseObject(raw, key)
}

// decodeList decodes an array whose items are all required (no nulls).
func decodeList[T any](raw json.RawMessage, key string, item func(json.RawMessage) (T, error)) ([]T, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, invalid("%s must be an array.", key)
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if isNull(it) {
			return nil, invalid("%s must not contain null.", key)
		}
		v, err := item(it)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func decodeResponse(data []byte) (response, error) {
	o, err := parseObject(data, "feed")
	if err != nil {
		return response{}, err
	}
	var r response
	raw, err := o.required("schemaVersion")
	if err != nil {
		return r, err
	}
	version, err := decodeFloat(raw, "schemaVersion")
	if err != nil || version != math.Trunc(version) {
		return r, invalid("schemaVersion must be an integer.")
	}
	r.schemaVersion = int(version)
	if r.generatedAt, err = o.date("generatedAt"); err != nil {
		return r, err
	}
	if r.lastSuccessfulCheckAt, err = o.optDate("lastSuccessfulCheckAt"); err != nil {
		return r, err
	}
	monitor, err := o.object("monitor")
	if err != nil {
		return r, err
	}
	if r.monitor, err = decodeMonitor(monitor); err != nil {
		return r, err
	}
	raw, err = o.required("events")
	if err != nil {
		return r, err
	}
	if r.events, err = decodeList(raw, "events", decodeEvent); err != nil {
		return r, err
	}
	if o.has("graceSchedules") {
		raw, err = o.required("graceSchedules")
		if err != nil {
			return r, err
		}
		if r.graceSchedules, err = decodeList(raw, "graceSchedules", decodeEvent); err != nil {
			return r, err
		}
	} else {
		r.graceSchedules = []Event{}
	}
	if raw, ok := o.optional("resetTimeline"); ok {
		t, err := decodeTimeline(raw)
		if err != nil {
			return r, err
		}
		r.resetTimeline = &t
	}
	return r, nil
}

func decodeMonitor(o obj) (Monitor, error) {
	status, err := o.str("status")
	if err != nil {
		return Monitor{}, err
	}
	m := Monitor{Status: MonitorStatus(status)}
	if m.Status != MonitorOK && m.Status != MonitorDegraded {
		return Monitor{}, invalid("monitor status is not known.")
	}
	m.ErrorCode, err = o.optStr("errorCode")
	return m, err
}

func decodeEnum[T ~string](o obj, key string, allowed ...T) (T, bool, error) {
	var zero T
	raw, ok := o.optional(key)
	if !ok {
		return zero, false, nil
	}
	s, err := decodeString(raw, key)
	if err != nil {
		return zero, false, err
	}
	for _, a := range allowed {
		if string(a) == s {
			return a, true, nil
		}
	}
	return zero, false, invalid("%s has an unknown value.", key)
}

func requiredEnum[T ~string](o obj, key string, allowed ...T) (T, error) {
	var zero T
	if _, err := o.required(key); err != nil {
		return zero, err
	}
	v, _, err := decodeEnum(o, key, allowed...)
	return v, err
}

func decodeEvent(raw json.RawMessage) (Event, error) {
	o, err := parseObject(raw, "event")
	if err != nil {
		return Event{}, err
	}
	var e Event
	if e.Kind, err = requiredEnum(o, "kind", KindResetCompleted, KindResetScheduled, KindBankedReset, KindLimitIncrease, KindUncertain); err != nil {
		return e, err
	}
	// Feeds published before reset types are global. An explicit null is
	// invalid and must not be treated as an omitted field.
	e.ResetType = ResetGlobal
	if o.has("resetType") {
		if e.ResetType, err = requiredEnum(o, "resetType", ResetGlobal, ResetBanked, ResetGlobalAndBanked); err != nil {
			return e, err
		}
	}
	if e.AnnouncedAt, err = o.date("announcedAt"); err != nil {
		return e, err
	}
	if e.EffectiveAt, err = o.optDate("effectiveAt"); err != nil {
		return e, err
	}
	if e.SchedulePrecision, _, err = decodeEnum(o, "schedulePrecision", PrecisionDate, PrecisionDatetime); err != nil {
		return e, err
	}
	if e.ScheduleBasis, _, err = decodeEnum(o, "scheduleBasis", BasisExplicit, BasisContextualInference); err != nil {
		return e, err
	}
	if o.has("scheduleGraceHours") {
		hours, err := o.float("scheduleGraceHours")
		if err != nil {
			return e, err
		}
		e.ScheduleGraceHours = &hours
	}
	scope, err := o.object("scope")
	if err != nil {
		return e, err
	}
	if e.Scope.Plans, err = scope.stringList("plans"); err != nil {
		return e, err
	}
	if e.Scope.Windows, err = scope.stringList("windows"); err != nil {
		return e, err
	}
	source, err := o.object("source")
	if err != nil {
		return e, err
	}
	if e.Source, err = decodeSource(source); err != nil {
		return e, err
	}
	if e.Confidence, err = o.float("confidence"); err != nil {
		return e, err
	}
	if e.Rationale, err = o.str("rationale"); err != nil {
		return e, err
	}
	e.Text, err = o.str("text")
	return e, err
}

func decodeSource(o obj) (EventSource, error) {
	var s EventSource
	var err error
	if s.PostID, err = o.str("postId"); err != nil {
		return s, err
	}
	if s.Handle, err = o.optStr("handle"); err != nil {
		return s, err
	}
	if s.URL, err = o.optStr("url"); err != nil {
		return s, err
	}
	if s.URL != nil {
		if _, err := parseURL(*s.URL); err != nil {
			return s, invalid("url is not a valid URL.")
		}
	}
	origin, ok, err := decodeEnum(o, "origin", OriginX, OriginOperator)
	if err != nil {
		return s, err
	}
	switch {
	case ok:
		s.Origin = origin
	case IsOperatorEventID(s.PostID):
		s.Origin = OriginOperator
	default:
		s.Origin = OriginX
	}
	return s, nil
}

func parseURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("empty URL")
	}
	return url.Parse(raw)
}

func decodeTimeline(raw json.RawMessage) (Timeline, error) {
	o, err := parseObject(raw, "resetTimeline")
	if err != nil {
		return Timeline{}, err
	}
	t := Timeline{
		FulfilledSchedules: []FulfilledSchedule{},
		FulfilledBanked:    []FulfilledBanked{},
		ManualCompletions:  []ManualCompletion{},
		SuppressedPostIDs:  []string{},
	}
	if raw, ok := o.optional("nextSchedule"); ok {
		next, err := decodeEvent(raw)
		if err != nil {
			return t, err
		}
		t.NextSchedule = &next
	}
	if t.RecentNonCompletedPostID, err = o.optStr("recentNonCompletedPostId"); err != nil {
		return t, err
	}
	if raw, ok := o.optional("fulfilledSchedules"); ok {
		if t.FulfilledSchedules, err = decodeList(raw, "fulfilledSchedules", decodeFulfilledSchedule); err != nil {
			return t, err
		}
	}
	if raw, ok := o.optional("fulfilledBanked"); ok {
		if t.FulfilledBanked, err = decodeList(raw, "fulfilledBanked", decodeFulfilledBanked); err != nil {
			return t, err
		}
	}
	if raw, ok := o.optional("manualCompletions"); ok {
		if t.ManualCompletions, err = decodeList(raw, "manualCompletions", decodeManualCompletion); err != nil {
			return t, err
		}
	}
	if raw, ok := o.optional("suppressedPostIds"); ok {
		if t.SuppressedPostIDs, err = decodeStringList(raw, "suppressedPostIds"); err != nil {
			return t, err
		}
	}
	return t, nil
}

func decodeFulfilledSchedule(raw json.RawMessage) (FulfilledSchedule, error) {
	o, err := parseObject(raw, "fulfilled schedule")
	if err != nil {
		return FulfilledSchedule{}, err
	}
	var f FulfilledSchedule
	rawSchedule, err := o.required("schedule")
	if err != nil {
		return f, err
	}
	if f.Schedule, err = decodeEvent(rawSchedule); err != nil {
		return f, err
	}
	window, err := o.object("window")
	if err != nil {
		return f, err
	}
	if f.Window.StartAt, err = window.date("startAt"); err != nil {
		return f, err
	}
	if f.Window.EndAt, err = window.date("endAt"); err != nil {
		return f, err
	}
	if f.Window.Precision, err = requiredEnum(window, "precision", PrecisionDate, PrecisionDatetime); err != nil {
		return f, err
	}
	if f.CompletionPostID, err = o.str("completionPostId"); err != nil {
		return f, err
	}
	if f.CompletedAt, err = o.date("completedAt"); err != nil {
		return f, err
	}
	if f.VisibleUntil, err = o.date("visibleUntil"); err != nil {
		return f, err
	}
	if f.FulfillmentOrigin, err = o.optStr("fulfillmentOrigin"); err != nil {
		return f, err
	}
	f.ClusterID, err = o.optStr("clusterId")
	return f, err
}

func decodeFulfilledBanked(raw json.RawMessage) (FulfilledBanked, error) {
	o, err := parseObject(raw, "fulfilled banked")
	if err != nil {
		return FulfilledBanked{}, err
	}
	var f FulfilledBanked
	rawBanked, err := o.required("banked")
	if err != nil {
		return f, err
	}
	if f.Banked, err = decodeEvent(rawBanked); err != nil {
		return f, err
	}
	if f.CompletionPostID, err = o.str("completionPostId"); err != nil {
		return f, err
	}
	if f.CompletedAt, err = o.date("completedAt"); err != nil {
		return f, err
	}
	f.VisibleUntil, err = o.date("visibleUntil")
	return f, err
}

func decodeManualCompletion(raw json.RawMessage) (ManualCompletion, error) {
	o, err := parseObject(raw, "manual completion")
	if err != nil {
		return ManualCompletion{}, err
	}
	var m ManualCompletion
	if m.ID, err = o.str("id"); err != nil {
		return m, err
	}
	if m.CompletedAt, err = o.date("completedAt"); err != nil {
		return m, err
	}
	if m.VisibleUntil, err = o.date("visibleUntil"); err != nil {
		return m, err
	}
	if m.RepresentativePostID, err = o.str("representativePostId"); err != nil {
		return m, err
	}
	if m.SchedulePostIDs, err = o.stringList("schedulePostIds"); err != nil {
		return m, err
	}
	rawSchedules, err := o.required("schedules")
	if err != nil {
		return m, err
	}
	if m.Schedules, err = decodeList(rawSchedules, "schedules", decodeEvent); err != nil {
		return m, err
	}
	m.FulfillmentOrigin, err = o.str("fulfillmentOrigin")
	return m, err
}

var monitorErrorCodes = map[string]bool{
	"configuration_error": true, "request_failed": true, "invalid_response": true, "uncited_source": true,
}

func validateResponse(r response) error {
	switch r.monitor.Status {
	case MonitorOK:
		if r.monitor.ErrorCode != nil {
			return invalid("A healthy monitor must not publish an error code.")
		}
	case MonitorDegraded:
		if r.monitor.ErrorCode == nil || !monitorErrorCodes[*r.monitor.ErrorCode] {
			return invalid("A degraded monitor must publish an error code.")
		}
	}
	if len(r.events) > 50 {
		return invalid("The reset-status feed contains too many events.")
	}
	byID := map[string]Event{}
	for _, e := range r.events {
		if err := validateEvent(e); err != nil {
			return err
		}
		if _, dup := byID[e.Source.PostID]; dup {
			return invalid("Events must have unique post IDs.")
		}
		byID[e.Source.PostID] = e
	}
	graceIDs := map[string]bool{}
	for _, e := range r.graceSchedules {
		if err := validateEvent(e); err != nil {
			return err
		}
		if e.Kind != KindResetScheduled {
			return invalid("Grace schedules must contain scheduled resets.")
		}
		if e.ScheduleGraceHours == nil {
			return invalid("Grace schedules must include scheduleGraceHours.")
		}
		if graceIDs[e.Source.PostID] {
			return invalid("Grace schedules must have unique post IDs.")
		}
		graceIDs[e.Source.PostID] = true
		if dup, ok := byID[e.Source.PostID]; ok && !eventsEqual(dup, e) {
			return invalid("Events and grace schedules must agree for duplicate post IDs.")
		}
	}
	if r.resetTimeline != nil {
		return validateTimeline(*r.resetTimeline)
	}
	return nil
}

func validateTimeline(t Timeline) error {
	if next := t.NextSchedule; next != nil {
		if err := validateEvent(*next); err != nil {
			return err
		}
		if next.Kind != KindResetScheduled {
			return invalid("nextSchedule must be a scheduled reset.")
		}
		if !next.ResetType.Includes(ResetGlobal) {
			return invalid("nextSchedule must include a global reset.")
		}
	}
	if t.RecentNonCompletedPostID != nil {
		if err := validateEventID(*t.RecentNonCompletedPostID, "recentNonCompletedPostId"); err != nil {
			return err
		}
	}
	fulfilled := map[string]bool{}
	for _, f := range t.FulfilledSchedules {
		if err := validateEvent(f.Schedule); err != nil {
			return err
		}
		if f.Schedule.Kind != KindResetScheduled {
			return invalid("A fulfilled schedule must contain a scheduled event.")
		}
		if err := validateEventID(f.CompletionPostID, "completionPostId"); err != nil {
			return err
		}
		if f.VisibleUntil.Before(f.CompletedAt) {
			return invalid("Fulfilled schedule visibility cannot end before completion.")
		}
		if err := validatePublishedWindow(f.Window); err != nil {
			return err
		}
		id := f.Schedule.Source.PostID
		if fulfilled[id] {
			return invalid("Fulfilled schedules must have unique post IDs.")
		}
		fulfilled[id] = true
	}
	for _, f := range t.FulfilledBanked {
		if err := validateEvent(f.Banked); err != nil {
			return err
		}
		if f.Banked.Kind != KindBankedReset {
			return invalid("A fulfilled banked origin must contain a banked reset event.")
		}
		if err := validateEventID(f.CompletionPostID, "completionPostId"); err != nil {
			return err
		}
		if f.VisibleUntil.Before(f.CompletedAt) {
			return invalid("Fulfilled banked visibility cannot end before completion.")
		}
	}
	manual := map[string]bool{}
	for _, m := range t.ManualCompletions {
		if err := validateManual(m); err != nil {
			return err
		}
		if manual[m.ID] {
			return invalid("Manual completions must have unique IDs.")
		}
		manual[m.ID] = true
	}
	suppressed := map[string]bool{}
	for _, id := range t.SuppressedPostIDs {
		if err := validateEventID(id, "suppressedPostIds"); err != nil {
			return err
		}
		if suppressed[id] {
			return invalid("Suppressed post IDs must be unique.")
		}
		suppressed[id] = true
	}
	return nil
}

func validateManual(m ManualCompletion) error {
	if !strings.HasPrefix(m.ID, "manual:") {
		return invalid("Manual completion IDs must start with manual:.")
	}
	if m.VisibleUntil.Before(m.CompletedAt) {
		return invalid("Manual completion visibility cannot end before completion.")
	}
	if m.FulfillmentOrigin != "manual" {
		return invalid("Manual completions must use fulfillmentOrigin manual.")
	}
	if err := validateEventID(m.RepresentativePostID, "representativePostId"); err != nil {
		return err
	}
	unique := map[string]bool{}
	for _, id := range m.SchedulePostIDs {
		unique[id] = true
	}
	if len(m.SchedulePostIDs) == 0 || len(unique) != len(m.SchedulePostIDs) {
		return invalid("Manual completion schedule post IDs must be unique and non-empty.")
	}
	for _, id := range m.SchedulePostIDs {
		if err := validateEventID(id, "schedulePostIds"); err != nil {
			return err
		}
	}
	if !unique[m.RepresentativePostID] {
		return invalid("Manual completion representative must be one of its schedules.")
	}
	if len(m.Schedules) != len(m.SchedulePostIDs) {
		return invalid("Manual completion schedules must match schedulePostIds.")
	}
	for i, s := range m.Schedules {
		if s.Source.PostID != m.SchedulePostIDs[i] {
			return invalid("Manual completion schedules must match schedulePostIds.")
		}
	}
	for _, s := range m.Schedules {
		if err := validateEvent(s); err != nil {
			return err
		}
		if s.Kind != KindResetScheduled {
			return invalid("Manual completion schedules must be scheduled events.")
		}
	}
	return nil
}

func validatePublishedWindow(w PublishedWindow) error {
	switch w.Precision {
	case PrecisionDatetime:
		if !w.EndAt.Equal(w.StartAt) {
			return invalid("A datetime reset window must be a single instant.")
		}
	case PrecisionDate:
		if !w.EndAt.After(w.StartAt) {
			return invalid("A date reset window must span a calendar day.")
		}
	}
	return nil
}

func validateEvent(e Event) error {
	if math.IsNaN(e.Confidence) || math.IsInf(e.Confidence, 0) || e.Confidence < 0 || e.Confidence > 1 {
		return invalid("Event confidence must be between zero and one.")
	}
	if err := validateSource(e.Source, e.Kind); err != nil {
		return err
	}
	if e.Text == "" {
		return invalid("Event text must be a non-empty original post body.")
	}
	if !acceptsRationale(e) {
		return invalid("Event rationale must be the derived explanation for its kind.")
	}
	if h := e.ScheduleGraceHours; h != nil {
		pending := e.AnnouncedAt
		if e.EffectiveAt != nil {
			pending = *e.EffectiveAt
		}
		_, ok := e.graceDeadline(pending)
		if math.IsNaN(*h) || math.IsInf(*h, 0) || *h < 0 || !ok {
			return invalid("Schedule grace hours must be finite, non-negative, and calculable.")
		}
	}
	noSchedule := e.SchedulePrecision == PrecisionNone && e.ScheduleBasis == "" && e.ScheduleGraceHours == nil
	switch e.Kind {
	case KindResetCompleted:
		if !noSchedule {
			return invalid("Schedule metadata is only allowed for scheduled events.")
		}
	case KindResetScheduled:
		if e.EffectiveAt == nil {
			return invalid("A scheduled reset must include its effective time.")
		}
	case KindBankedReset:
		if e.ResetType == ResetGlobalAndBanked {
			return invalid("A banked reset origin cannot combine reset types.")
		}
		if e.EffectiveAt != nil {
			return invalid("This event kind cannot include an effective time.")
		}
		if !noSchedule {
			return invalid("Schedule metadata is only allowed for scheduled events.")
		}
	case KindLimitIncrease, KindUncertain:
		if e.ResetType != ResetGlobal {
			return invalid("This event kind cannot publish a reset type.")
		}
		if e.EffectiveAt != nil {
			return invalid("This event kind cannot include an effective time.")
		}
		if !noSchedule {
			return invalid("Schedule metadata is only allowed for scheduled events.")
		}
	}
	return nil
}

func validateSource(s EventSource, kind EventKind) error {
	if s.IsOperator() {
		if kind != KindResetCompleted && kind != KindResetScheduled {
			return invalid("Operator events must be completed or scheduled resets.")
		}
		if !IsOperatorEventID(s.PostID) || s.Handle != nil || s.URL != nil {
			return invalid("Operator event source requires an op_ event id without handle or URL.")
		}
		return nil
	}
	bad := invalid("Event source must identify a valid @thsottiaux X post.")
	if IsOperatorEventID(s.PostID) || !IsXPostID(s.PostID) || s.Handle == nil || *s.Handle != "thsottiaux" || s.URL == nil {
		return bad
	}
	u, err := parseURL(*s.URL)
	if err != nil {
		return bad
	}
	var path []string
	for _, c := range strings.Split(u.Path, "/") {
		if c != "" {
			path = append(path, c)
		}
	}
	if strings.ToLower(u.Scheme) != "https" || strings.ToLower(u.Hostname()) != "x.com" ||
		len(path) != 3 || !strings.EqualFold(path[0], "thsottiaux") || path[1] != "status" || path[2] != s.PostID {
		return bad
	}
	return nil
}

func validateEventID(id, label string) error {
	if !IsEventID(id) {
		return invalid("%s must be a numeric X post ID or operator event id.", label)
	}
	return nil
}

// rationaleSubjects names the reset type in the derived explanation.
func rationale(resetType ResetType, global, banked, both string) string {
	switch resetType {
	case ResetBanked:
		return banked
	case ResetGlobalAndBanked:
		return both
	}
	return global
}

// acceptsRationale: the schema enumerates one explanation per kind. The
// uncertain kind also accepts one legacy wording for cached payloads.
func acceptsRationale(e Event) bool {
	var want []string
	switch e.Kind {
	case KindResetCompleted:
		if e.Source.IsOperator() {
			want = []string{rationale(e.ResetType,
				"Operator-confirmed Codex quota reset without an X announcement.",
				"Operator-confirmed Codex reset-bank credit without an X announcement.",
				"Operator-confirmed Codex global reset and reset-bank credit without an X announcement.")}
		} else {
			want = []string{rationale(e.ResetType,
				"Explicit Codex quota reset announcement.",
				"Explicit Codex reset-bank credit announcement.",
				"Explicit Codex global reset and reset-bank credit announcement.")}
		}
	case KindResetScheduled:
		switch {
		case e.Source.IsOperator():
			want = []string{rationale(e.ResetType,
				"Operator-confirmed Codex quota reset schedule without an X announcement.",
				"Operator-confirmed Codex reset-bank credit schedule without an X announcement.",
				"Operator-confirmed Codex global reset and reset-bank credit schedule without an X announcement.")}
		case e.ScheduleBasis == BasisContextualInference:
			want = []string{rationale(e.ResetType,
				"High-probability Codex quota reset preview inferred from context.",
				"High-probability Codex reset-bank credit preview inferred from context.",
				"High-probability Codex global reset and reset-bank credit preview inferred from context.")}
		default:
			want = []string{rationale(e.ResetType,
				"Explicit Codex quota reset schedule.",
				"Explicit Codex reset-bank credit schedule.",
				"Explicit Codex global reset and reset-bank credit schedule.")}
		}
	case KindBankedReset:
		want = []string{"Banked reset announcement; not a completed reset."}
	case KindLimitIncrease:
		want = []string{"Quota limit increase announcement; not a reset."}
	case KindUncertain:
		want = []string{"Not a clear reset signal.", "Relevant announcement could not be classified safely."}
	}
	for _, w := range want {
		if e.Rationale == w {
			return true
		}
	}
	return false
}

func eventsEqual(a, b Event) bool {
	return a.Kind == b.Kind && a.ResetType == b.ResetType && a.AnnouncedAt.Equal(b.AnnouncedAt) &&
		timePtrEqual(a.EffectiveAt, b.EffectiveAt) && a.SchedulePrecision == b.SchedulePrecision &&
		a.ScheduleBasis == b.ScheduleBasis && floatPtrEqual(a.ScheduleGraceHours, b.ScheduleGraceHours) &&
		stringsEqual(a.Scope.Plans, b.Scope.Plans) && stringsEqual(a.Scope.Windows, b.Scope.Windows) &&
		a.Source.Origin == b.Source.Origin && a.Source.PostID == b.Source.PostID &&
		stringPtrEqual(a.Source.Handle, b.Source.Handle) && stringPtrEqual(a.Source.URL, b.Source.URL) &&
		a.Confidence == b.Confidence && a.Rationale == b.Rationale && a.Text == b.Text
}

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func stringPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
