package resettoday

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/testzone"
)

// swiftCase is one feed evaluated by the Swift releases' core in one time
// zone; the fixture is frozen.
type swiftCase struct {
	Name        string       `json:"name"`
	Feed        string       `json:"feed"`
	NowMs       int64        `json:"nowMs"`
	Zone        string       `json:"zone"`
	DecodeError bool         `json:"decodeError"`
	State       string       `json:"state"`
	Probes      []swiftProbe `json:"probes"`
}

type swiftSummary struct {
	Start      int64   `json:"start"`
	End        int64   `json:"end"`
	Range      bool    `json:"range"`
	Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
	Basis      *string `json:"basis"`
	PostID     string  `json:"postID"`
}

type swiftDayNext struct {
	PostID string `json:"postID"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Range  bool   `json:"range"`
}

type swiftLatest struct {
	At   int64  `json:"at"`
	Type string `json:"type"`
}

type swiftHint struct {
	Type string  `json:"type"`
	Text string  `json:"text"`
	Ago  *string `json:"ago"`
}

type swiftDetail struct {
	Type    *string  `json:"type"`
	Label   string   `json:"label"`
	Percent *string  `json:"percent"`
	Time    *string  `json:"time"`
	Plain   string   `json:"plain"`
	Tokens  []string `json:"tokens"`
}

type swiftProbe struct {
	At          int64           `json:"at"`
	State       string          `json:"state"`
	Reason      string          `json:"reason"`
	ShowsYes    bool            `json:"showsYes"`
	IsCompleted bool            `json:"isCompleted"`
	IsScheduled bool            `json:"isScheduled"`
	ResetType   *string         `json:"resetType"`
	Percent     *int            `json:"percent"`
	Band        *string         `json:"band"`
	Confidence  *float64        `json:"confidence"`
	Basis       *string         `json:"basis"`
	CompletedAt *int64          `json:"completedAt"`
	Evidence    *string         `json:"evidence"`
	WinStart    *int64          `json:"winStart"`
	WinEnd      *int64          `json:"winEnd"`
	WinRange    *bool           `json:"winRange"`
	Summary     *swiftSummary   `json:"summary"`
	DayNext     *swiftDayNext   `json:"dayNext"`
	Next        *string         `json:"next"`
	Transitions []int64         `json:"transitions"`
	EvidenceEN  *string         `json:"evidenceEN"`
	EvidenceURL *string         `json:"evidenceURL"`
	Display     *string         `json:"display"`
	Latest      *swiftLatest    `json:"latest"`
	Effective   bool            `json:"effectiveToday"`
	Uncertain   bool            `json:"uncertain"`
	SameDay     bool            `json:"sameDay"`
	Unconfirmed *swiftHint      `json:"unconfirmed"`
	AlertsNil   json.RawMessage `json:"alertsNil"`
	AlertsEmpty json.RawMessage `json:"alertsEmpty"`
	AlertsSelf  json.RawMessage `json:"alertsSelf"`

	EvidenceZH    *string         `json:"evidenceZH"`
	EvidenceURLZH *string         `json:"evidenceURLZH"`
	DetailEN      *swiftDetail    `json:"detailEN"`
	DetailZH      *swiftDetail    `json:"detailZH"`
	NoneHintEN    *swiftHint      `json:"noneHintEN"`
	NoneHintZH    *swiftHint      `json:"noneHintZH"`
	Widget        json.RawMessage `json:"widget"`
}

func loadSwiftCases(t testing.TB) []swiftCase {
	t.Helper()
	f, err := os.Open("testdata/swift_cases.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var cases []swiftCase
	if err := json.NewDecoder(zr).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 100 {
		t.Fatalf("only %d swift cases", len(cases))
	}
	return cases
}

func msOf(t time.Time) int64 { return t.UnixMilli() }

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrMs(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := msOf(*t)
	return &v
}

// TestSwiftDifferential replays every Swift-generated case and compares the
// decode result, the verdict, schedule summaries, transition dates, evidence
// copy, alert candidates and the widget timeline probe by probe.
func TestSwiftDifferential(t *testing.T) {
	cases := loadSwiftCases(t)
	var probes, decodeErrors int
	for _, c := range cases {
		loc, err := testzone.LoadLocation(c.Zone)
		if err != nil {
			t.Fatal(err)
		}
		now := time.UnixMilli(c.NowMs).UTC()
		snap, err := Decode([]byte(c.Feed), now, loc)
		if c.DecodeError {
			decodeErrors++
			if err == nil {
				t.Errorf("%s/%s: Swift rejects the feed, Go accepts it", c.Name, c.Zone)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s/%s: Swift accepts the feed, Go rejects it: %v", c.Name, c.Zone, err)
			continue
		}
		if string(snap.State()) != c.State {
			t.Errorf("%s/%s: state %s want %s", c.Name, c.Zone, snap.State(), c.State)
		}
		for i, p := range c.Probes {
			probes++
			compareProbe(t, c, i, snap, loc, p)
		}
	}
	if decodeErrors == 0 || probes < 1000 {
		t.Fatalf("fixture too small: %d probes, %d decode errors", probes, decodeErrors)
	}
	t.Logf("%d cases, %d probes, %d decode errors", len(cases), probes, decodeErrors)
}

func compareProbe(t *testing.T, c swiftCase, idx int, snap *Snapshot, loc *time.Location, p swiftProbe) {
	t.Helper()
	label := func(field string) string {
		return c.Name + "/" + c.Zone + " probe " + time.UnixMilli(p.At).UTC().Format(time.RFC3339) + " " + field
	}
	bad := func(field string, got, want any) {
		t.Errorf("%s: got %v want %v", label(field), got, want)
	}
	now := time.UnixMilli(p.At).UTC()
	en, zh := l10n.New(l10n.English), l10n.New(l10n.SimplifiedChinese)

	if got := string(snap.ResolvedState(now, loc)); got != p.State {
		bad("state", got, p.State)
	}
	v := snap.VerdictPresentation(now, loc)
	if string(v.Reason) != p.Reason {
		bad("reason", v.Reason, p.Reason)
	}
	if v.ShowsYes != p.ShowsYes || v.IsCompleted != p.IsCompleted || v.IsScheduled != p.IsScheduled {
		bad("flags", []bool{v.ShowsYes, v.IsCompleted, v.IsScheduled}, []bool{p.ShowsYes, p.IsCompleted, p.IsScheduled})
	}
	var gotType *string
	if v.ResetType != nil {
		s := string(*v.ResetType)
		gotType = &s
	}
	if !reflect.DeepEqual(gotType, p.ResetType) {
		bad("resetType", strOrEmpty(gotType), strOrEmpty(p.ResetType))
	}
	if !reflect.DeepEqual(v.Percent, p.Percent) {
		bad("percent", v.Percent, p.Percent)
	}
	var gotBand *string
	if v.Band != nil {
		s := string(*v.Band)
		gotBand = &s
	}
	if !reflect.DeepEqual(gotBand, p.Band) {
		bad("band", strOrEmpty(gotBand), strOrEmpty(p.Band))
	}
	if !reflect.DeepEqual(v.Confidence, p.Confidence) {
		bad("confidence", v.Confidence, p.Confidence)
	}
	if string(v.ScheduleBasis) != strOrEmpty(p.Basis) {
		bad("basis", v.ScheduleBasis, strOrEmpty(p.Basis))
	}
	if !reflect.DeepEqual(ptrMs(v.CompletedAt), p.CompletedAt) {
		bad("completedAt", ptrMs(v.CompletedAt), p.CompletedAt)
	}
	evidence := ""
	if v.EvidenceEvent != nil {
		evidence = v.EvidenceEvent.Source.PostID
	}
	if evidence != strOrEmpty(p.Evidence) {
		bad("evidence", evidence, strOrEmpty(p.Evidence))
	}
	var ws, we *int64
	var wr *bool
	if v.ScheduleWindow != nil {
		s, e, r := msOf(v.ScheduleWindow.StartAt), msOf(v.ScheduleWindow.EndAt), v.ScheduleWindow.IsRange
		ws, we, wr = &s, &e, &r
	}
	if !reflect.DeepEqual(ws, p.WinStart) || !reflect.DeepEqual(we, p.WinEnd) || !reflect.DeepEqual(wr, p.WinRange) {
		bad("window", []any{ws, we, wr}, []any{p.WinStart, p.WinEnd, p.WinRange})
	}

	sum := snap.NextScheduledResetSummary(now)
	switch {
	case sum == nil && p.Summary != nil:
		bad("summary", nil, *p.Summary)
	case sum != nil && p.Summary == nil:
		bad("summary", sum.Window, nil)
	case sum != nil:
		got := swiftSummary{msOf(sum.Window.StartAt), msOf(sum.Window.EndAt), sum.Window.IsRange, string(sum.ResetType),
			sum.Confidence, nil, sum.Event.Source.PostID}
		if sum.ScheduleBasis != "" {
			b := string(sum.ScheduleBasis)
			got.Basis = &b
		}
		if !reflect.DeepEqual(got, *p.Summary) {
			bad("summary", got, *p.Summary)
		}
	}
	day := snap.NextScheduledResetOnLocalDay(now, loc)
	switch {
	case day == nil && p.DayNext != nil, day != nil && p.DayNext == nil:
		bad("dayNext", day, p.DayNext)
	case day != nil:
		got := swiftDayNext{day.Event.Source.PostID, msOf(day.EffectiveAt), msOf(day.EffectiveUntil), day.IsRange}
		if got != *p.DayNext {
			bad("dayNext", got, *p.DayNext)
		}
	}
	next := ""
	if n := snap.NextScheduledReset(now); n != nil {
		next = n.Event.Source.PostID
	}
	if next != strOrEmpty(p.Next) {
		bad("next", next, strOrEmpty(p.Next))
	}

	got := []int64{}
	for _, d := range snap.TransitionDates(now, loc) {
		got = append(got, msOf(d))
	}
	if want := p.Transitions; !reflect.DeepEqual(got, append([]int64{}, want...)) {
		bad("transitions", got, want)
	}

	if g := snap.EvidenceLine(en, now, loc); g != strOrEmpty(p.EvidenceEN) {
		bad("evidenceEN", g, strOrEmpty(p.EvidenceEN))
	}
	if g := snap.EvidenceURL(now, loc, l10n.English); g != strOrEmpty(p.EvidenceURL) {
		bad("evidenceURL", g, strOrEmpty(p.EvidenceURL))
	}
	if idx == 0 {
		if g := snap.EvidenceLine(zh, now, loc); g != strOrEmpty(p.EvidenceZH) {
			bad("evidenceZH", g, strOrEmpty(p.EvidenceZH))
		}
		if g := snap.EvidenceURL(now, loc, l10n.SimplifiedChinese); g != strOrEmpty(p.EvidenceURLZH) {
			bad("evidenceURLZH", g, strOrEmpty(p.EvidenceURLZH))
		}
	}
	display := ""
	if d := snap.DisplayResetType(now, loc); d != nil {
		display = string(*d)
	}
	if display != strOrEmpty(p.Display) {
		bad("display", display, strOrEmpty(p.Display))
	}
	var latest *swiftLatest
	if l := snap.LatestReset(now); l != nil {
		latest = &swiftLatest{msOf(l.At), string(l.ResetType)}
	}
	if !reflect.DeepEqual(latest, p.Latest) {
		bad("latest", latest, p.Latest)
	}
	if g := snap.HasAlreadyEffectiveResetToday(now, loc); g != p.Effective {
		bad("effectiveToday", g, p.Effective)
	}
	if g := snap.HasUncertainNoSignalToday(now, loc); g != p.Uncertain {
		bad("uncertain", g, p.Uncertain)
	}
	if g := snap.PrefersSameDayScheduleExplanation(now, loc); g != p.SameDay {
		bad("sameDay", g, p.SameDay)
	}
	var hint *swiftHint
	if h := snap.UnconfirmedScheduleHint(en, now, loc); h != nil {
		hint = &swiftHint{Type: string(h.ResetType), Text: h.Text}
	}
	if !reflect.DeepEqual(hint, p.Unconfirmed) {
		bad("unconfirmed", hint, p.Unconfirmed)
	}

	// Alerts: the candidates must serialize exactly like the Swift ones.
	empty, err := Decode([]byte(emptyFeedFor(snap, now)), now, loc)
	if err != nil {
		t.Fatalf("%s: empty feed: %v", label("alerts"), err)
	}
	for name, pair := range map[string]struct {
		previous *Snapshot
		want     json.RawMessage
	}{
		"alertsNil":   {nil, p.AlertsNil},
		"alertsEmpty": {empty, p.AlertsEmpty},
		"alertsSelf":  {snap, p.AlertsSelf},
	} {
		list := Alerts(pair.previous, snap, now, loc)
		data, err := swiftjson.Marshal(list, false)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(pair.want) {
			bad(name, string(data), string(pair.want))
		}
	}

	if idx == 0 {
		data, err := swiftjson.Marshal(NewSource(snap, loc).Widget(now), false)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(p.Widget) {
			bad("widget", string(data), string(p.Widget))
		}
	}
}

// emptyFeedFor mirrors the generator's feed with no events, stamped at now.
func emptyFeedFor(_ *Snapshot, now time.Time) string {
	ts := now.UTC().Format(time.RFC3339)
	return `{"schemaVersion":1,"generatedAt":"` + ts + `","lastSuccessfulCheckAt":"` + ts +
		`","monitor":{"status":"ok","errorCode":null},"events":[]}`
}

// TestWidgetMatchesExportFixtures feeds the real verdict code into
// export.BuildResetToday and checks it against the timelines internal/export's
// own Swift generator produced for the same four feeds.
func TestWidgetMatchesExportFixtures(t *testing.T) {
	raw, err := os.ReadFile("../export/testdata/swift_widget_reset_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]json.RawMessage
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	now := at(t, "2026-07-28T12:00:00Z")
	feeds := map[string]feedFixture{
		"upcoming": newFeed(now, newEvent("reset_scheduled", "2026-07-28T11:00:00Z",
			effective("2026-07-28T13:00:00Z"), precision("datetime"), basis("explicit"))),
		"completedWithFuture": newFeed(now,
			newEvent("reset_completed", "2026-07-28T11:00:00Z", effective("2026-07-28T11:00:00Z"), post("100")),
			newEvent("reset_scheduled", "2026-07-28T11:30:00Z", effective("2026-07-29T13:00:00.123Z"),
				precision("datetime"), post("200"))),
		"none":  newFeed(now),
		"stale": newFeed(now).generated("2026-07-26T12:00:00Z").checked("2026-07-26T12:00:00Z"),
	}
	for name, feed := range feeds {
		t.Run(name, func(t *testing.T) {
			snap, err := Decode([]byte(feed.JSON()), now, time.UTC)
			if err != nil {
				t.Fatal(err)
			}
			data, err := swiftjson.Marshal(NewSource(snap, time.UTC).Widget(now), false)
			if err != nil {
				t.Fatal(err)
			}
			var gotV, wantV any
			if json.Unmarshal(data, &gotV) != nil || json.Unmarshal(want[name], &wantV) != nil {
				t.Fatal("bad json")
			}
			if !reflect.DeepEqual(gotV, wantV) {
				t.Fatalf("widget differs from Swift\n got: %s\nwant: %s", data, want[name])
			}
		})
	}
}
