package resettoday

import (
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
)

func TestEndpoints(t *testing.T) {
	if SiteURL != "https://didcodexreset.com/" || StatusURL != "https://didcodexreset.com/api/status.json" {
		t.Fatalf("urls = %q %q", SiteURL, StatusURL)
	}
}

func TestDecodesCompleteAPIV1Event(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	feed := newFeed(now, newEvent("reset_completed", "2026-07-28T11:55:00.000Z", post("123")))
	feed.EventsJSON = `{
      "kind": "reset_completed",
      "announcedAt": "2026-07-28T11:55:00.000Z",
      "effectiveAt": null,
      "scope": {"plans": ["all"], "windows": ["weekly", "five_hour"]},
      "source": {"handle": "thsottiaux", "postId": "123", "url": "https://x.com/thsottiaux/status/123"},
      "confidence": 0.98,
      "rationale": "Explicit Codex quota reset announcement.",
      "text": "I have reset usage limits for Codex."
    }`
	s := feed.mustDecode(t)
	if len(s.Events) != 1 {
		t.Fatalf("events = %d", len(s.Events))
	}
	e := s.Events[0]
	if s.SchemaVersion != 1 || !s.GeneratedAt.Equal(now) || s.LastSuccessfulCheckAt == nil || !s.LastSuccessfulCheckAt.Equal(now) {
		t.Errorf("header = %+v", s)
	}
	if s.Monitor.Status != MonitorOK || s.Monitor.ErrorCode != nil {
		t.Errorf("monitor = %+v", s.Monitor)
	}
	if e.Kind != KindResetCompleted || !e.AnnouncedAt.Equal(at(t, "2026-07-28T11:55:00Z")) || e.EffectiveAt != nil {
		t.Errorf("event = %+v", e)
	}
	if strings.Join(e.Scope.Plans, ",") != "all" || strings.Join(e.Scope.Windows, ",") != "weekly,five_hour" {
		t.Errorf("scope = %+v", e.Scope)
	}
	if e.Source.Origin != OriginX || e.Source.Handle == nil || *e.Source.Handle != "thsottiaux" || e.Source.PostID != "123" ||
		e.Source.URL == nil || *e.Source.URL != "https://x.com/thsottiaux/status/123" {
		t.Errorf("source = %+v", e.Source)
	}
	if e.Confidence != 0.98 || e.Rationale != "Explicit Codex quota reset announcement." || e.Text != "I have reset usage limits for Codex." {
		t.Errorf("event fields = %+v", e)
	}
	if s.State() != StateYes {
		t.Errorf("state = %v", s.State())
	}
	if got := s.EvidenceLine(english, now, time.UTC); got != "An explicit Codex quota reset was announced." {
		t.Errorf("en evidence = %q", got)
	}
	if got := s.EvidenceLine(chinese, now, time.UTC); got != "发现明确的 Codex 配额重置公告。" {
		t.Errorf("zh evidence = %q", got)
	}
	if got := s.EvidenceURL(now, time.UTC, l10n.English); got != "https://didcodexreset.com/history/1785239700000.html" {
		t.Errorf("url = %q", got)
	}
	if got := s.EvidenceURL(now, time.UTC, l10n.SimplifiedChinese); got != "https://didcodexreset.com/zh/history/1785239700000.html" {
		t.Errorf("zh url = %q", got)
	}
}

func TestCompletedResetUsesInjectedLocalDay(t *testing.T) {
	now := at(t, "2026-07-28T00:30:00Z")
	sg, la := mustLoc(t, "Asia/Singapore"), mustLoc(t, "America/Los_Angeles")
	tests := []struct {
		name      string
		announced string
		loc       *time.Location
		want      State
	}{
		{"singapore late", "2026-07-27T23:30:00Z", sg, StateYes},
		{"los angeles late", "2026-07-27T23:30:00Z", la, StateYes},
		{"singapore previous day", "2026-07-27T15:30:00Z", sg, StateNo},
		{"los angeles same day", "2026-07-27T15:30:00Z", la, StateYes},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFeed(now, newEvent("reset_completed", tc.announced)).checked("2026-07-28T00:30:00Z").using(tc.loc).mustDecode(t)
			if s.State() != tc.want {
				t.Fatalf("state = %v, want %v", s.State(), tc.want)
			}
		})
	}
}

func TestScheduledResetUsesLocalDay(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	pending := newFeed(now, newEvent("reset_scheduled", "2026-07-28T09:00:00Z", effective("2026-07-28T13:00:00Z"))).mustDecode(t)
	elapsed := newFeed(now, newEvent("reset_scheduled", "2026-07-27T22:00:00Z", effective("2026-07-28T11:00:00Z"))).mustDecode(t)

	if pending.State() != StateYes || pending.HasAlreadyEffectiveResetToday(now, time.UTC) ||
		pending.NextScheduledReset(now) == nil || pending.PrimaryEvidenceEvent(now, time.UTC).Kind != KindResetScheduled {
		t.Errorf("pending: state=%v", pending.State())
	}
	if elapsed.State() != StateNo || elapsed.HasAlreadyEffectiveResetToday(now, time.UTC) ||
		elapsed.NextScheduledReset(now) != nil || elapsed.VerdictPresentation(now, time.UTC).Reason != "grace" ||
		elapsed.PrimaryEvidenceEvent(now, time.UTC).Kind != KindResetScheduled {
		t.Errorf("elapsed: state=%v reason=%v", elapsed.State(), elapsed.VerdictPresentation(now, time.UTC).Reason)
	}
}

func TestDateOnlyScheduleExpandsToLocalWindow(t *testing.T) {
	now := at(t, "2026-08-09T14:17:20Z")
	sg := mustLoc(t, "Asia/Singapore")
	s := newFeed(now, newEvent("reset_scheduled", "2026-08-08T20:34:50Z", effective("2026-08-10T07:00:00Z"))).
		checked("2026-08-09T14:17:20Z").using(sg).mustDecode(t)
	next := s.NextScheduledReset(now)
	if next == nil || !next.IsRange ||
		!next.EffectiveAt.Equal(at(t, "2026-08-10T07:00:00Z")) || !next.EffectiveUntil.Equal(at(t, "2026-08-11T06:59:00Z")) {
		t.Fatalf("next = %+v", next)
	}
}

func TestDateOnlyWindowRemainsScheduledOnSecondLocalDate(t *testing.T) {
	now := at(t, "2026-08-11T02:00:20Z")
	sg := mustLoc(t, "Asia/Singapore")
	s := newFeed(now, newEvent("reset_scheduled", "2026-08-08T20:34:50Z", effective("2026-08-10T07:00:00Z"))).
		checked("2026-08-11T02:00:20Z").using(sg).mustDecode(t)
	if got := s.ResolvedState(now, sg); got != StateYes {
		t.Fatalf("state = %v", got)
	}
	next := s.NextScheduledResetOnLocalDay(now, sg)
	if next == nil || !next.EffectiveUntil.Equal(at(t, "2026-08-11T06:59:00Z")) {
		t.Fatalf("next = %+v", next)
	}
}

func TestCompletedResetOutranksSameDaySchedule(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	s := newFeed(now,
		newEvent("reset_completed", "2026-07-28T08:00:00.000Z", post("100")),
		newEvent("reset_scheduled", "2026-07-28T09:00:00.000Z", effective("2026-07-28T20:00:00.000Z"), post("200")),
	).mustDecode(t)
	if s.ResolvedState(now, time.UTC) != StateYes || !s.HasAlreadyEffectiveResetToday(now, time.UTC) ||
		s.VerdictPresentation(now, time.UTC).Reason != "completed" ||
		s.PrimaryEvidenceEvent(now, time.UTC).Source.PostID != "100" ||
		s.NextScheduledReset(now).Event.Source.PostID != "200" {
		t.Fatal("completed reset does not outrank the open schedule")
	}
}

func TestPrimaryEvidenceAndNextSchedulePreferActionableEvents(t *testing.T) {
	now := at(t, "2026-07-29T12:00:00Z")
	s := newFeed(now,
		newEvent("reset_scheduled", "2026-07-29T05:44:16.000Z", effective("2026-07-31T12:00:00.000Z"), post("2082341416681001277")),
		newEvent("reset_completed", "2026-07-29T04:09:02.000Z", post("2082317452755751098")),
	).mustDecode(t)
	if s.State() != StateYes || s.PrimaryEvidenceEvent(now, time.UTC).Source.PostID != "2082317452755751098" ||
		s.NextScheduledReset(now).Event.Source.PostID != "2082341416681001277" {
		t.Fatal("actionable events not preferred")
	}
	if got := s.EvidenceURL(now, time.UTC, l10n.English); got != "https://didcodexreset.com/history/1785298142000.html" {
		t.Errorf("url = %q", got)
	}
}

func TestFutureDayScheduleAloneIsNo(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	s := newFeed(now, newEvent("reset_scheduled", "2026-07-28T09:00:00Z", effective("2026-07-31T12:00:00Z"))).mustDecode(t)
	if s.State() != StateNo || s.NextScheduledReset(now) == nil || s.PrimaryEvidenceEvent(now, time.UTC).Kind != KindResetScheduled {
		t.Fatal("future schedule")
	}
	if got := s.EvidenceLine(english, now, time.UTC); got != "An explicit Codex quota reset was scheduled." {
		t.Errorf("evidence = %q", got)
	}
}

func TestNonResetEventDoesNotCount(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	for _, kind := range []string{"banked_reset", "limit_increase"} {
		if s := newFeed(now, newEvent(kind, "2026-07-28T11:00:00Z")).mustDecode(t); s.State() != StateNo {
			t.Errorf("%s: state = %v", kind, s.State())
		}
	}
}

func TestConfirmedResetWinsOverUncertain(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	only := newFeed(now, newEvent("uncertain", "2026-07-28T11:00:00Z")).mustDecode(t)
	both := newFeed(now,
		newEvent("reset_completed", "2026-07-28T10:00:00Z", post("456")),
		newEvent("uncertain", "2026-07-28T11:00:00Z", post("789")),
	).mustDecode(t)
	if only.State() != StateNo || !only.HasUncertainNoSignalToday(now, time.UTC) || both.State() != StateYes {
		t.Fatalf("states = %v/%v", only.State(), both.State())
	}
}

func TestHealthyUncertainFeedResolvesToNo(t *testing.T) {
	now := at(t, "2026-08-02T08:10:25Z")
	s := newFeed(now, newEvent("uncertain", "2026-08-02T02:43:02Z")).checked("2026-08-02T08:10:25Z").mustDecode(t)
	if s.State() != StateNo || !s.HasUncertainNoSignalToday(now, time.UTC) || s.PrimaryEvidenceEvent(now, time.UTC) != nil ||
		s.EvidenceLine(english, now, time.UTC) != "" || s.EvidenceLine(chinese, now, time.UTC) != "" {
		t.Fatal("uncertain only feed")
	}
}

func TestDegradedOrStaleMonitorForcesUnknown(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	ev := newEvent("reset_completed", "2026-07-28T11:00:00Z")
	degraded := newFeed(now, ev).monitor("degraded", "request_failed").mustDecode(t)
	stale := newFeed(now, ev).generated("2026-07-27T05:59:59Z").checked("2026-07-27T05:59:59Z").mustDecode(t)
	missing := newFeed(now, ev).checked("").mustDecode(t)
	if degraded.State() != StateUnknown || degraded.Monitor.ErrorCode == nil || *degraded.Monitor.ErrorCode != "request_failed" {
		t.Errorf("degraded = %v", degraded.State())
	}
	if stale.State() != StateUnknown {
		t.Errorf("stale = %v", stale.State())
	}
	if missing.State() != StateYes {
		t.Errorf("missing success = %v", missing.State())
	}
}

func TestThirtyHourBoundaryRemainsFresh(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	s := newFeed(now, newEvent("reset_completed", "2026-07-28T11:00:00Z")).generated("2026-07-27T06:00:00Z").checked("2026-07-27T06:00:00Z").mustDecode(t)
	if s.State() != StateYes {
		t.Fatalf("state = %v", s.State())
	}
}

func TestFreshFeedWithoutEventsMeansNo(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	s := newFeed(now).mustDecode(t)
	if len(s.Events) != 0 || s.State() != StateNo {
		t.Fatalf("state = %v events = %d", s.State(), len(s.Events))
	}
}

func TestResolvedStateChangesAtLocalMidnight(t *testing.T) {
	before := at(t, "2026-07-28T23:59:00Z")
	s := newFeed(before, newEvent("reset_completed", "2026-07-28T20:00:00Z")).checked("2026-07-28T23:50:00Z").mustDecode(t)
	if s.State() != StateYes {
		t.Fatal("not yes before midnight")
	}
	next := at(t, "2026-07-29T00:01:00Z")
	if s.ResolvedState(next, time.UTC) != StateNo || s.PrimaryEvidenceEvent(next, time.UTC) != nil ||
		s.EvidenceLine(english, next, time.UTC) != "" || s.EvidenceURL(next, time.UTC, l10n.English) != "" {
		t.Fatal("state did not roll over")
	}
}

func TestUTCDatePrefixIsNotLocalDay(t *testing.T) {
	now := at(t, "2026-08-01T09:25:19Z")
	la, cn := mustLoc(t, "America/Los_Angeles"), mustLoc(t, "Asia/Shanghai")
	mk := func(loc *time.Location) *Snapshot {
		return newFeed(now, newEvent("reset_completed", "2026-08-01T03:32:37.000Z")).checked("2026-08-01T09:25:19Z").using(loc).mustDecode(t)
	}
	sla, scn := mk(la), mk(cn)
	if sla.State() != StateNo || scn.State() != StateYes || sla.HasAlreadyEffectiveResetToday(now, la) ||
		!scn.HasAlreadyEffectiveResetToday(now, cn) || sla.PrimaryEvidenceEvent(now, la) != nil ||
		sla.EvidenceLine(english, now, la) != "" || scn.PrimaryEvidenceEvent(now, cn).Kind != KindResetCompleted {
		t.Fatal("local day mismatch")
	}
}

func TestDetectedAlertFollowsLocalDay(t *testing.T) {
	now := at(t, "2026-08-01T09:25:19Z")
	la, cn := mustLoc(t, "America/Los_Angeles"), mustLoc(t, "Asia/Shanghai")
	previous := newFeed(now).checked("2026-08-01T09:25:19Z").mustDecode(t)
	current := newFeed(now, newEvent("reset_completed", "2026-08-01T03:32:37.000Z")).checked("2026-08-01T09:25:19Z").mustDecode(t)
	if got := Alerts(previous, current, now, la); len(got) != 0 {
		t.Errorf("LA alerts = %+v", got)
	}
	if got := Alerts(previous, current, now, cn); len(got) != 1 || got[0].Kind != "rateLimitResetDetected" {
		t.Errorf("CN alerts = %+v", got)
	}
}

func TestDecodesLiveStatusPayloadShape(t *testing.T) {
	now := at(t, "2026-08-01T09:25:19Z")
	const data = `{
      "schemaVersion": 1,
      "generatedAt": "2026-08-01T09:25:19.679Z",
      "lastSuccessfulCheckAt": "2026-08-01T09:25:19.679Z",
      "monitor": {"status": "ok", "errorCode": null},
      "events": [{
        "kind": "reset_completed",
        "announcedAt": "2026-08-01T03:32:37.000Z",
        "effectiveAt": null,
        "scope": {"plans": ["all"], "windows": ["unknown"]},
        "source": {"handle": "thsottiaux", "postId": "2083395449814229287", "url": "https://x.com/thsottiaux/status/2083395449814229287"},
        "confidence": 0.99,
        "rationale": "Explicit Codex quota reset announcement.",
        "text": "I have reset usage limits for Codex and ChatGPT Work."
      }]
    }`
	cn := mustLoc(t, "Asia/Shanghai")
	s, err := Decode([]byte(data), now, cn)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Events) != 1 || s.Events[0].Text != "I have reset usage limits for Codex and ChatGPT Work." ||
		!s.Events[0].AnnouncedAt.Equal(at(t, "2026-08-01T03:32:37Z")) || s.ResolvedState(now, cn) != StateYes ||
		s.EvidenceLine(english, now, cn) != "An explicit Codex quota reset was announced." {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestRejectsTimestampsWithoutTimezone(t *testing.T) {
	_, err := Decode([]byte(`{"schemaVersion":1,"generatedAt":"2026-08-01T12:00:00","lastSuccessfulCheckAt":"2026-08-01T12:00:00Z","monitor":{"status":"ok","errorCode":null},"events":[]}`),
		at(t, "2026-08-01T12:00:00Z"), nil)
	if err == nil {
		t.Fatal("decoded a timestamp without an offset")
	}
}

func TestSameDayScheduleEvidencePrefersSoonest(t *testing.T) {
	now := at(t, "2026-08-01T12:00:00Z")
	s := newFeed(now,
		newEvent("reset_scheduled", "2026-08-01T10:00:00.000Z", effective("2026-08-01T20:00:00.000Z"), post("100")),
		newEvent("reset_scheduled", "2026-08-01T11:00:00.000Z", effective("2026-08-01T15:00:00.000Z"), post("200")),
	).checked("2026-08-01T12:00:00Z").mustDecode(t)
	if s.ResolvedState(now, time.UTC) != StateYes || s.NextScheduledReset(now).Event.Source.PostID != "200" ||
		s.NextScheduledResetOnLocalDay(now, time.UTC).Event.Source.PostID != "200" ||
		s.PrimaryEvidenceEvent(now, time.UTC).Source.PostID != "200" {
		t.Fatal("soonest schedule not preferred")
	}
}

func TestPastDayCompletedResetIsNotTodayEvidence(t *testing.T) {
	now := at(t, "2026-08-18T03:15:00Z")
	completed := newEvent("reset_completed", "2026-08-11T00:27:44Z", post("2086972802457063486"))
	scheduled := newEvent("reset_scheduled", "2026-08-13T01:01:37Z", effective("2026-08-13T02:01:37Z"),
		precision("datetime"), basis("explicit"), post("2087706104814023111"))
	manual := `{
      "id": "manual:cl_stale_evidence",
      "completedAt": "2026-08-13T04:35:00.000Z",
      "visibleUntil": "2026-08-23T04:35:00.000Z",
      "representativePostId": "2087706104814023111",
      "schedulePostIds": ["2087706104814023111"],
      "schedules": [` + scheduled.JSON() + `],
      "fulfillmentOrigin": "manual"
    }`
	s := newFeed(now, completed).checked("2026-08-18T01:00:02Z").
		withTimeline(timelineJSON("", "", "", manual, "")).mustDecode(t)
	last := s.LatestResetAt(now)
	if s.ResolvedState(now, time.UTC) != StateNo || s.NextScheduledReset(now) != nil ||
		last == nil || !last.Equal(at(t, "2026-08-13T04:35:00Z")) ||
		s.PrimaryEvidenceEvent(now, time.UTC) != nil || s.EvidenceLine(english, now, time.UTC) != "" ||
		s.EvidenceLine(chinese, now, time.UTC) != "" || s.EvidenceURL(now, time.UTC, l10n.English) != "" {
		t.Fatal("past-day completion leaked into today's evidence")
	}
}

func TestRejectsInconsistentFeedData(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	completed := newEvent("reset_completed", "2026-07-28T11:00:00Z")
	custom := func(rationale, text string) feedFixture {
		f := newFeed(now)
		f.EventsJSON = `{
          "kind": "reset_completed", "announcedAt": "2026-07-28T11:00:00Z", "effectiveAt": null,
          "scope": {"plans": ["all"], "windows": ["weekly"]},
          "source": {"handle": "thsottiaux", "postId": "123", "url": "https://x.com/thsottiaux/status/123"},
          "confidence": 0.98, "rationale": "` + rationale + `", "text": "` + text + `"}`
		return f
	}
	tests := []struct {
		name string
		feed feedFixture
	}{
		{"ok monitor with error code", newFeed(now, completed).monitor("ok", "request_failed")},
		{"scheduled without effectiveAt", newFeed(now, newEvent("reset_scheduled", "2026-07-28T11:00:00Z"))},
		{"limit increase with effectiveAt", newFeed(now, newEvent("limit_increase", "2026-07-28T11:00:00Z", effective("2026-07-28T11:30:00Z")))},
		{"copied post text as rationale", custom("Copied post text is not a derived explanation.", "I have reset usage limits.")},
		{"empty text", custom("Explicit Codex quota reset announcement.", "")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { wantDecodeError(t, tc.feed) })
	}
}

func TestAcceptsCurrentAndLegacyUncertainRationales(t *testing.T) {
	now := at(t, "2026-07-28T12:00:00Z")
	current := newFeed(now, newEvent("uncertain", "2026-07-28T11:00:00Z")).mustDecode(t)
	if current.State() != StateNo || current.Events[0].Rationale != "Not a clear reset signal." {
		t.Errorf("current = %v", current.Events[0].Rationale)
	}
	f := newFeed(now)
	f.EventsJSON = `{
      "kind": "uncertain", "announcedAt": "2026-07-28T11:00:00Z", "effectiveAt": null,
      "scope": {"plans": ["all"], "windows": ["unknown"]},
      "source": {"handle": "thsottiaux", "postId": "123", "url": "https://x.com/thsottiaux/status/123"},
      "confidence": 0.5, "rationale": "Relevant announcement could not be classified safely.", "text": "Unclear post."}`
	legacy := f.mustDecode(t)
	if legacy.State() != StateNo || legacy.Events[0].Rationale != "Relevant announcement could not be classified safely." {
		t.Errorf("legacy = %v", legacy.Events[0].Rationale)
	}
}

func TestRejectsUnsupportedSchemaVersion(t *testing.T) {
	_, err := Decode([]byte(`{"schemaVersion":2,"generatedAt":"2026-07-28T12:00:00Z","lastSuccessfulCheckAt":"2026-07-28T12:00:00Z","monitor":{"status":"ok","errorCode":null},"events":[]}`),
		at(t, "2026-07-28T12:00:00Z"), nil)
	if err == nil {
		t.Fatal("decoded schema version 2")
	}
}

func TestDevelopmentFixturesUseEventSemantics(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	yes, no, unknown := DevMock(MockYes, now, time.UTC), DevMock(MockNo, now, time.UTC), DevMock(MockUnknown, now, time.UTC)
	if yes.State() != StateYes || len(yes.Events) == 0 || yes.Events[0].Kind != KindResetCompleted ||
		no.State() != StateNo || unknown.State() != StateUnknown {
		t.Fatal("dev mock semantics")
	}
	if _, ok := ParseDevMockKind("yes-countdown"); ok {
		t.Error("yes-countdown still parses")
	}
}

// Operator events -----------------------------------------------------------

const operatorPostID = "op_4c549b7d3147b644968bb73a"

func opCompleted(announced string, opts ...eventOption) eventFixture {
	all := append([]eventOption{rtype("global"), effective(announced)}, opts...)
	return operatorEvent("reset_completed", announced, operatorPostID, all...)
}

func TestDecodesOperatorConfirmedReset(t *testing.T) {
	now := at(t, "2026-08-25T16:00:00Z")
	s := newFeed(now, opCompleted("2026-08-25T14:30:00Z")).checked("2026-08-25T16:00:00Z").mustDecode(t)
	e := s.Events[0]
	if e.Source.Origin != OriginOperator || !e.Source.IsOperator() || e.Source.Handle != nil || e.Source.URL != nil ||
		e.Source.PostID != operatorPostID || e.Kind != KindResetCompleted || e.ResetType != ResetGlobal {
		t.Fatalf("event = %+v", e)
	}
	reset := s.LatestReset(now)
	if s.ResolvedState(now, time.UTC) != StateYes || reset == nil || !reset.At.Equal(at(t, "2026-08-25T14:30:00Z")) || reset.ResetType != ResetGlobal {
		t.Errorf("latest = %+v", reset)
	}
	if got := s.EvidenceLine(chinese, now, time.UTC); got != "运营确认已重置，Tibo 未发 X。" {
		t.Errorf("evidence = %q", got)
	}
	if got := s.EvidenceURL(now, time.UTC, l10n.English); got != "https://didcodexreset.com/history/1787668200000.html" {
		t.Errorf("url = %q", got)
	}
}

func TestInfersOperatorOriginFromEventID(t *testing.T) {
	now := at(t, "2026-08-25T16:00:00Z")
	s := newFeed(now, opCompleted("2026-08-25T14:30:00Z", noOrigin())).checked("2026-08-25T16:00:00Z").mustDecode(t)
	if s.Events[0].Source.Origin != OriginOperator || s.Events[0].Source.PostID != operatorPostID {
		t.Fatalf("source = %+v", s.Events[0].Source)
	}
}

func TestOperatorCompletionOutranksOlderXReset(t *testing.T) {
	now := at(t, "2026-08-25T16:00:00Z")
	older := newEvent("reset_completed", "2026-08-24T00:46:51Z", post("2091688655828246890"))
	s := newFeed(now, opCompleted("2026-08-25T14:30:00Z"), older).checked("2026-08-25T16:00:00Z").mustDecode(t)
	reset := s.LatestReset(now)
	if reset == nil || !reset.At.Equal(at(t, "2026-08-25T14:30:00Z")) || reset.ResetType != ResetGlobal {
		t.Fatalf("latest = %+v", reset)
	}
}

func TestRejectsOperatorSourceWithXFields(t *testing.T) {
	now := at(t, "2026-08-25T16:00:00Z")
	wantDecodeError(t, newFeed(now, opCompleted("2026-08-25T14:30:00Z", handle("thsottiaux"))))
	wantDecodeError(t, newFeed(now, opCompleted("2026-08-25T14:30:00Z", withURL())))
}

func TestRejectsUnsupportedOperatorEventKinds(t *testing.T) {
	now := at(t, "2026-08-25T16:00:00Z")
	for _, kind := range []string{"uncertain", "banked_reset"} {
		wantDecodeError(t, newFeed(now, operatorEvent(kind, "2026-08-25T14:00:00Z", operatorPostID)))
	}
}

func TestAcceptsOperatorCompletionPostIDOnFulfilledSchedules(t *testing.T) {
	now := at(t, "2026-08-25T16:00:00Z")
	scheduled := newEvent("reset_scheduled", "2026-08-24T12:00:00Z", effective("2026-08-25T14:00:00Z"),
		precision("datetime"), post("2091688655828246890"))
	timeline := `{
      "nextSchedule": null, "recentNonCompletedPostId": null,
      "fulfilledSchedules": [{
        "schedule": ` + scheduled.JSON() + `,
        "window": {"startAt": "2026-08-25T14:00:00.000Z", "endAt": "2026-08-25T14:00:00.000Z", "precision": "datetime"},
        "completionPostId": "` + operatorPostID + `",
        "completedAt": "2026-08-25T14:30:00.000Z",
        "visibleUntil": "2026-09-04T14:30:00.000Z"
      }]}`
	s := newFeed(now, opCompleted("2026-08-25T14:30:00Z")).checked("2026-08-25T16:00:00Z").withTimeline(timeline).mustDecode(t)
	if s.ResetTimeline.FulfilledSchedules[0].CompletionPostID != operatorPostID || s.ResolvedState(now, time.UTC) != StateYes {
		t.Fatal("operator completion on fulfilled schedule")
	}
}

func TestExpiredOperatorScheduleIsUnconfirmedNo(t *testing.T) {
	now := at(t, "2026-09-02T13:48:03Z")
	const scheduledID = "op_77973cda8bc5d0be9afde39e"
	scheduled := operatorEvent("reset_scheduled", "2026-09-01T00:12:30Z", scheduledID,
		rtype("global"), effective("2026-09-01T14:00:00Z"), precision("date"), basis("explicit"))
	completed := newEvent("reset_completed", "2026-08-31T02:34:27Z", post("2094252447271366730"))
	s := newFeed(now, scheduled, completed).checked("2026-09-02T13:48:03Z").
		withTimeline(timelineJSON("", scheduledID, "", "", "")).mustDecode(t)
	hint := s.UnconfirmedScheduleHint(chinese, now, time.UTC)
	p := s.VerdictPresentation(now, time.UTC)
	if s.ResolvedState(now, time.UTC) != StateNo || s.NextScheduledReset(now) != nil ||
		p.Reason != "expiredUnconfirmed" || p.EvidenceEvent != nil || p.Confidence != nil || p.ScheduleWindow != nil ||
		s.PrimaryEvidenceEvent(now, time.UTC) != nil || s.EvidenceLine(chinese, now, time.UTC) != "" {
		t.Fatalf("presentation = %+v", p)
	}
	if hint == nil || hint.ResetType != ResetGlobal || hint.Text != "已排期的全局重置时间已过，但未得到确认" {
		t.Fatalf("hint = %+v", hint)
	}
	if h := s.UnconfirmedScheduleHint(english, now, time.UTC); h == nil || h.Text != "The scheduled Global reset time passed without confirmation" {
		t.Errorf("en hint = %+v", h)
	}
}

func TestPendingOperatorScheduleIsYes(t *testing.T) {
	now := at(t, "2026-09-01T08:00:00Z")
	scheduled := operatorEvent("reset_scheduled", "2026-09-01T00:12:30Z", operatorPostID,
		rtype("global"), effective("2026-09-01T18:00:00Z"), precision("datetime"), basis("explicit"))
	s := newFeed(now, scheduled).checked("2026-09-01T08:00:00Z").
		withTimeline(timelineJSON(scheduled.JSON(), operatorPostID, "", "", "")).mustDecode(t)
	if s.ResolvedState(now, time.UTC) != StateYes || s.NextScheduledReset(now).Event.Source.PostID != operatorPostID ||
		s.UnconfirmedExpiredSchedule(now, time.UTC) != nil {
		t.Fatal("pending operator schedule")
	}
	if got := s.EvidenceLine(chinese, now, time.UTC); got != "运营确认已排期重置，Tibo 未发 X。" {
		t.Errorf("evidence = %q", got)
	}
}

func TestRejectsInvalidRecentNonCompletedPostID(t *testing.T) {
	now := at(t, "2026-09-02T13:48:03Z")
	wantDecodeError(t, newFeed(now, newEvent("reset_completed", "2026-08-31T02:34:27Z")).
		withTimeline(timelineJSON("", "not-an-id", "", "", "")))
}

func TestAcceptsOperatorSuppressedPostID(t *testing.T) {
	now := at(t, "2026-09-01T08:00:00Z")
	global := newEvent("reset_scheduled", "2026-09-01T00:00:00Z", rtype("global"), effective("2026-09-01T16:00:00Z"),
		precision("datetime"), post("100"))
	banked := operatorEvent("reset_scheduled", "2026-09-01T00:12:30Z", operatorPostID,
		rtype("banked"), effective("2026-09-01T14:00:00Z"), precision("datetime"), basis("explicit"))
	s := newFeed(now, global, banked).checked("2026-09-01T08:00:00Z").
		withTimeline(timelineJSON(global.JSON(), "100", "", "", `"`+operatorPostID+`"`)).mustDecode(t)
	if got := s.ResetTimeline.SuppressedPostIDs; len(got) != 1 || got[0] != operatorPostID {
		t.Fatalf("suppressed = %v", got)
	}
	if next := s.NextScheduledReset(now); next == nil || next.Event.Source.PostID != "100" {
		t.Errorf("suppressed operator schedule still wins: %+v", next)
	}
}

func TestRejectsInvalidSuppressedPostID(t *testing.T) {
	now := at(t, "2026-09-01T08:00:00Z")
	wantDecodeError(t, newFeed(now, newEvent("reset_completed", "2026-08-31T02:34:27Z")).
		withTimeline(timelineJSON("", "", "", "", `"op_not-an-id"`)))
}

func TestAcceptsOperatorManualCompletionSchedule(t *testing.T) {
	now := at(t, "2026-09-01T20:00:00Z")
	scheduled := operatorEvent("reset_scheduled", "2026-09-01T00:12:30Z", operatorPostID,
		rtype("global"), effective("2026-09-01T14:00:00Z"), precision("datetime"), basis("explicit"))
	manual := `{"id": "manual:cl_abd57edf90eb450aa54bd48c010854af", "completedAt": "2026-09-01T14:30:00.000Z",
      "visibleUntil": "2026-09-11T14:30:00.000Z", "representativePostId": "` + operatorPostID + `",
      "schedulePostIds": ["` + operatorPostID + `"], "schedules": [` + scheduled.JSON() + `], "fulfillmentOrigin": "manual"}`
	s := newFeed(now, scheduled).checked("2026-09-01T20:00:00Z").
		withTimeline(timelineJSON("", "", "", manual, `"`+operatorPostID+`"`)).mustDecode(t)
	if m := s.ResetTimeline.ManualCompletions; len(m) != 1 || m[0].RepresentativePostID != operatorPostID {
		t.Fatalf("manual = %+v", m)
	}
}
