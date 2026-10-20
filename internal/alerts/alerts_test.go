package alerts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

var legacyQuotaThresholds = []int{80, 95, 100}

func at(t testing.TB, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func ptr[T any](v T) *T { return &v }

func completed(t testing.TB, id string, rt ResetType, when string) Event {
	return Event{PostID: id, Completed: true, Date: at(t, when), ResetType: rt}
}

func schedule(t testing.TB, id, announced string, rt ResetType, effective string, basis *ScheduleBasis, conf float64) UpcomingReset {
	eff := at(t, effective)
	return UpcomingReset{EffectiveAt: eff, EffectiveUntil: eff, PostID: id, ResetType: rt, Confidence: conf, ScheduleBasis: basis}
}

func manual(t testing.TB, id, completedAt, until, rep string, posts []string, types ...ResetType) ManualCompletion {
	return ManualCompletion{
		ID: id, CompletedAt: at(t, completedAt), VisibleUntil: at(t, until),
		RepresentativePostID: rep, SchedulePostIDs: posts, ScheduleTypes: types,
	}
}

func decide(t testing.TB, previous *ResetTodayState, current ResetTodayState, now string) []Alert {
	return ResetTodayAlerts(previous, current, at(t, now), time.UTC)
}

const noon = "2026-08-23T12:00:00Z"

func TestFirstLoadStaysSilent(t *testing.T) {
	cur := ResetTodayState{Events: []Event{completed(t, "100", ResetBanked, "2026-08-23T10:00:00Z")}}
	if got := decide(t, nil, cur, noon); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestNewOccurrencesUseIdentityAndStableOrder(t *testing.T) {
	existing := completed(t, "050", ResetGlobal, "2026-08-23T01:00:00Z")
	previous := &ResetTodayState{Events: []Event{existing}}
	current := ResetTodayState{Events: []Event{
		completed(t, "300", ResetBanked, "2026-08-23T04:00:00Z"),
		existing,
		completed(t, "200", ResetGlobal, "2026-08-23T04:00:00Z"),
		completed(t, "100", ResetGlobal, "2026-08-23T03:00:00Z"),
		completed(t, "tomorrow", ResetGlobal, "2026-08-24T03:00:00Z"),
		completed(t, "future", ResetGlobal, "2026-08-23T13:00:00Z"),
	}}
	got := decide(t, previous, current, noon)
	day := at(t, "2026-08-23T00:00:00Z").Unix()
	var ids, names []string
	for _, a := range got {
		ids = append(ids, a.ID)
		names = append(names, a.Name)
	}
	wantIDs := []string{
		fmt.Sprintf("rate-limit-reset:detected:100:%d", day),
		fmt.Sprintf("rate-limit-reset:detected:200:%d", day),
		fmt.Sprintf("rate-limit-reset:detected:300:%d", day),
	}
	if !reflect.DeepEqual(ids, wantIDs) || !reflect.DeepEqual(names, []string{"100", "200", "300"}) {
		t.Fatalf("ids %v names %v", ids, names)
	}
	if *got[0].ResetType != ResetGlobal || *got[2].ResetType != ResetBanked {
		t.Fatalf("types: %v %v", *got[0].ResetType, *got[2].ResetType)
	}
}

func TestTypeCorrectionDoesNotRedetect(t *testing.T) {
	prev := &ResetTodayState{Events: []Event{completed(t, "100", ResetGlobal, "2026-08-23T10:00:00Z")}}
	cur := ResetTodayState{Events: []Event{completed(t, "100", ResetBanked, "2026-08-23T10:00:00Z")}}
	if got := decide(t, prev, cur, noon); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestManualCompletionUsesManualIdentityAndMergedType(t *testing.T) {
	c := manual(t, "manual:mixed-1", "2026-08-23T04:00:00Z", "2026-08-24T04:00:00Z", "900", []string{"900", "901"}, ResetBanked, ResetGlobal)
	got := decide(t, &ResetTodayState{}, ResetTodayState{ManualCompletions: []ManualCompletion{c}}, noon)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	a := got[0]
	day := at(t, "2026-08-23T00:00:00Z").Unix()
	if a.ID != fmt.Sprintf("rate-limit-reset:detected:at:%d:%d", c.CompletedAt.Unix(), day) ||
		a.Name != "900" || *a.ResetType != ResetGlobalAndBanked || !a.Date.Equal(c.CompletedAt) {
		t.Fatalf("alert = %+v", a)
	}
}

func TestReclusteredManualCompletionDoesNotRedetect(t *testing.T) {
	until := "2026-09-02T04:00:00Z"
	prev := &ResetTodayState{ManualCompletions: []ManualCompletion{
		manual(t, "manual:cl_aaa", "2026-08-23T04:00:00Z", until, "900", []string{"900", "901"}, ResetGlobal, ResetGlobal),
	}}
	cur := ResetTodayState{ManualCompletions: []ManualCompletion{
		manual(t, "manual:cl_bbb", "2026-08-23T04:00:00Z", until, "902", []string{"901", "902"}, ResetGlobal, ResetGlobal),
	}}
	if got := decide(t, prev, cur, noon); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestDisjointManualCompletionStillAlerts(t *testing.T) {
	until := "2026-09-02T04:00:00Z"
	first := manual(t, "manual:cl_first", "2026-08-23T04:00:00Z", until, "900", []string{"900"}, ResetGlobal)
	second := manual(t, "manual:cl_second", "2026-08-23T08:00:00Z", until, "901", []string{"901"}, ResetGlobal)
	got := decide(t, &ResetTodayState{ManualCompletions: []ManualCompletion{first}},
		ResetTodayState{ManualCompletions: []ManualCompletion{first, second}}, noon)
	day := at(t, "2026-08-23T00:00:00Z").Unix()
	if len(got) != 1 || got[0].ID != fmt.Sprintf("rate-limit-reset:detected:at:%d:%d", second.CompletedAt.Unix(), day) || got[0].Name != "901" {
		t.Fatalf("got %+v", got)
	}
}

func TestCompletedEventThenMatchingManualStaysSeen(t *testing.T) {
	prev := &ResetTodayState{Events: []Event{completed(t, "900", ResetGlobal, "2026-08-23T04:00:00Z")}}
	cur := ResetTodayState{ManualCompletions: []ManualCompletion{
		manual(t, "manual:cl_later", "2026-08-23T04:00:00Z", "2026-09-02T04:00:00Z", "900", []string{"900"}, ResetGlobal),
	}}
	if got := decide(t, prev, cur, noon); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestExpiredManualCompletionIsNotAnOccurrence(t *testing.T) {
	c := manual(t, "manual:old", "2026-08-23T04:00:00Z", "2026-08-23T11:00:00Z", "900", []string{"900"}, ResetGlobal)
	if got := decide(t, &ResetTodayState{}, ResetTodayState{ManualCompletions: []ManualCompletion{c}}, noon); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestUpcomingAlertKeepsIdentityAcrossTypeChange(t *testing.T) {
	a := ResetTodayState{Upcoming: ptr(schedule(t, "500", "", ResetGlobal, "2026-08-23T12:25:00Z", nil, 1))}
	b := ResetTodayState{Upcoming: ptr(schedule(t, "500", "", ResetBanked, "2026-08-23T12:25:00Z", nil, 1))}
	ga := decide(t, &a, a, noon)
	gb := decide(t, &a, b, noon)
	eff := at(t, "2026-08-23T12:25:00Z").Unix()
	if len(ga) != 1 || len(gb) != 1 || ga[0].ID != gb[0].ID || ga[0].ID != fmt.Sprintf("rate-limit-reset:upcoming:500:30:%d", eff) {
		t.Fatalf("ga %+v gb %+v", ga, gb)
	}
	if *ga[0].ResetType != ResetGlobal || *gb[0].ResetType != ResetBanked || ga[0].ScheduleBasis != nil || *ga[0].ConfidencePercent != 100 {
		t.Fatalf("ga %+v gb %+v", ga[0], gb[0])
	}
}

func TestUpcomingThresholds(t *testing.T) {
	for _, tc := range []struct {
		effective string
		want      int
	}{
		{"2026-08-23T12:00:01Z", 30},
		{"2026-08-23T12:30:00Z", 30},
		{"2026-08-23T12:30:01Z", 60},
		{"2026-08-23T13:00:00Z", 60},
		{"2026-08-23T13:00:01Z", 0},
		{"2026-08-23T12:00:00Z", -1},
		{"2026-08-23T11:00:00Z", -1},
	} {
		s := ResetTodayState{Upcoming: ptr(schedule(t, "p", "", ResetGlobal, tc.effective, nil, 0.5))}
		got := decide(t, &s, s, noon)
		switch {
		case tc.want <= 0:
			if len(got) != 0 {
				t.Errorf("%s: got %+v", tc.effective, got)
			}
		case len(got) != 1 || *got[0].Threshold != tc.want:
			t.Errorf("%s: got %+v want threshold %d", tc.effective, got, tc.want)
		}
	}
}

func TestUpcomingRangeCarriesEndDate(t *testing.T) {
	up := schedule(t, "r", "", ResetGlobal, "2026-08-23T12:20:00Z", nil, 0.9)
	up.IsRange = true
	up.EffectiveUntil = at(t, "2026-08-23T23:59:00Z")
	s := ResetTodayState{Upcoming: &up}
	got := decide(t, &s, s, noon)
	if len(got) != 1 || got[0].EndDate == nil || !got[0].EndDate.Equal(up.EffectiveUntil) {
		t.Fatalf("got %+v", got)
	}
	up.IsRange = false
	if got := decide(t, &s, s, noon); got[0].EndDate != nil {
		t.Fatalf("a point schedule has no end date: %+v", got[0])
	}
}

func TestUnavailableStaysSilent(t *testing.T) {
	cur := ResetTodayState{
		Unavailable: true,
		Events:      []Event{completed(t, "completed", ResetGlobal, noon)},
		Upcoming:    ptr(schedule(t, "scheduled", "", ResetGlobal, "2026-08-23T12:30:00Z", nil, 1)),
	}
	if got := decide(t, &ResetTodayState{}, cur, noon); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestDetectedAlertFollowsLocalDay(t *testing.T) {
	now := at(t, "2026-08-01T09:25:19Z")
	cur := ResetTodayState{Events: []Event{completed(t, "x", ResetGlobal, "2026-08-01T03:32:37Z")}}
	la, _ := time.LoadLocation("America/Los_Angeles")
	cn, _ := time.LoadLocation("Asia/Shanghai")
	if la == nil || cn == nil {
		t.Skip("no tz database")
	}
	if got := ResetTodayAlerts(&ResetTodayState{}, cur, now, la); len(got) != 0 {
		t.Fatalf("Los Angeles: %+v", got)
	}
	got := ResetTodayAlerts(&ResetTodayState{}, cur, now, cn)
	if len(got) != 1 || got[0].Kind != KindRateLimitResetDetected {
		t.Fatalf("Shanghai: %+v", got)
	}
	if want := fmt.Sprintf("rate-limit-reset:detected:x:%d", at(t, "2026-07-31T16:00:00Z").Unix()); got[0].ID != want {
		t.Fatalf("id %s, want %s (start of the Shanghai day)", got[0].ID, want)
	}
}

func TestQuotaAlertsUseHighestThreshold(t *testing.T) {
	reset := at(t, "2026-08-23T18:00:00Z")
	got := QuotaAlerts(QuotaInput{
		Primary:   Window{82, &reset},
		Secondary: &Window{100, ptr(reset.Add(24 * time.Hour))},
		Additional: []NamedWindow{
			{"GPT-5.3-Codex-Spark", Window{96, nil}},
			{"Other", Window{79, &reset}},
		},
	}, legacyQuotaThresholds)
	var ids []string
	for _, a := range got {
		ids = append(ids, a.ID)
	}
	want := []string{
		fmt.Sprintf("quota:5-hour:80:%d", reset.Unix()),
		fmt.Sprintf("quota:Weekly:100:%d", reset.Add(24*time.Hour).Unix()),
		"quota:GPT-5.3-Codex-Spark:95:0",
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v", ids)
	}
	if got[2].Date != nil || *got[2].Threshold != 95 {
		t.Fatalf("no reset time means no date: %+v", got[2])
	}
}

func TestResetCreditAlertsOnlyExpiringAvailable(t *testing.T) {
	now := at(t, noon)
	day := 24 * time.Hour
	credit := func(id *string, status string, expires time.Duration, hasExpiry bool) ResetCredit {
		c := ResetCredit{ID: id, Status: status, RemainingSeconds: expires.Seconds()}
		if hasExpiry {
			c.ExpiresAt = ptr(now.Add(expires))
		}
		return c
	}
	got := ResetCreditAlerts([]ResetCredit{
		credit(ptr("c1"), "available", 3*day, true),
		credit(nil, "available", day, true),
		credit(ptr("c3"), "available", 8*day, true),
		credit(ptr("c4"), "available", 0, false),
		credit(ptr("c5"), "used", day, true),
		credit(ptr("c6"), "available", 7*day, true),
	})
	var ids []string
	for _, a := range got {
		ids = append(ids, a.ID)
	}
	want := []string{
		fmt.Sprintf("reset-credit:c1:%d", now.Add(3*day).Unix()),
		fmt.Sprintf("reset-credit:%d:%d", now.Add(day).Unix(), now.Add(day).Unix()),
		fmt.Sprintf("reset-credit:c6:%d", now.Add(7*day).Unix()),
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestMergeResetTypes(t *testing.T) {
	if MergeResetTypes(nil) != "" || MergeResetTypes([]ResetType{ResetBanked}) != ResetBanked ||
		MergeResetTypes([]ResetType{ResetGlobal, ResetBanked}) != ResetGlobalAndBanked ||
		MergeResetTypes([]ResetType{ResetGlobalAndBanked}) != ResetGlobalAndBanked {
		t.Fatal("unexpected merge")
	}
	if (ManualCompletion{}).ResetType() != ResetGlobal {
		t.Fatal("a completion without schedules is global")
	}
}

func TestDisplayedPercent(t *testing.T) {
	for in, want := range map[float64]int{0.876: 88, 0.72: 72, 1: 100, 0.005: 1, 0.004: 0, 0.995: 100} {
		if got := DisplayedPercent(in); got != want {
			t.Errorf("DisplayedPercent(%v) = %d, want %d", in, got, want)
		}
	}
}

// Store.

func TestStoreOnlyReturnsUnseen(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "sub", "alerts.json")}
	reset := time.Unix(2_000, 0)
	expiry := time.Unix(1_000, 0).Add(2 * 24 * time.Hour)
	alerts := append(
		QuotaAlerts(QuotaInput{Primary: Window{96, &reset}}, legacyQuotaThresholds),
		ResetCreditAlerts([]ResetCredit{{ID: ptr("credit-1"), Status: "available", ExpiresAt: &expiry, RemainingSeconds: 2 * 86400}})...)
	first, err := store.Unseen(alerts)
	if err != nil || len(first) != 2 {
		t.Fatalf("first = %+v %v", first, err)
	}
	second, err := store.Unseen(alerts)
	if err != nil || len(second) != 0 {
		t.Fatalf("second = %+v %v", second, err)
	}
	if info, err := os.Stat(store.Path); err != nil || info.IsDir() {
		t.Fatal("state file must exist")
	}
}

func TestStoreUnreadableStateMeansNothingSeen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.json")
	if err := os.WriteFile(path, []byte(`{"not":"an array"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Store{Path: path}.Unseen([]Alert{{ID: "a"}})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v %v", got, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != `["a"]` {
		t.Fatalf("file = %s", data)
	}
}

func TestStoreNoNewAlertsLeavesFileAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.json")
	store := Store{Path: path}
	if got, err := store.Unseen(nil); err != nil || len(got) != 0 {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("nothing to remember must not create the file")
	}
}

func TestStoreDuplicatesInOneBatchAreBothReturned(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "alerts.json")}
	got, _ := store.Unseen([]Alert{{ID: "a"}, {ID: "a"}})
	if len(got) != 2 {
		t.Fatalf("Swift filters against the stored set only: %+v", got)
	}
}

func TestDefaultPath(t *testing.T) {
	got := DefaultPath(filepath.Join("home", "u"))
	if filepath.Base(got) != "alerts.json" || filepath.Base(filepath.Dir(got)) != ".quosparr" {
		t.Fatalf("path = %s", got)
	}
}

// Compatibility with the Swift implementation: the fixtures were produced by
// the real RunwayAlertStore / RunwayAlertDecider.

func TestStateFileMatchesSwiftBytes(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "alerts.json")}
	var small []Alert
	for i := 0; i < 5; i++ {
		small = append(small, Alert{ID: fmt.Sprintf("quota:5-hour:%d:17830%d", 80+i, i)})
	}
	small = append(small, Alert{ID: "reset-credit:a/b:é:1"})
	if _, err := store.Unseen(small); err != nil {
		t.Fatal(err)
	}
	compare(t, store.Path, "swift_alert_state_small.json")

	var big []Alert
	for i := 0; i < 250; i++ {
		big = append(big, Alert{ID: fmt.Sprintf("quota:Weekly:100:%d", 1_780_000_000+i*7)})
	}
	unseen, err := store.Unseen(big)
	if err != nil || len(unseen) != 250 {
		t.Fatalf("unseen = %d %v", len(unseen), err)
	}
	compare(t, store.Path, "swift_alert_state_capped.json")

	// Only the lexicographically last 200 IDs are remembered, so the rest
	// alert again.
	again, err := store.Unseen(big)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(again))
	for _, a := range again {
		ids = append(ids, a.ID)
	}
	got, err := swiftjson.Marshal(ids, false)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/swift_alert_cap_reappear.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reappearing IDs differ\n got: %s\nwant: %s", got, want)
	}
	// 250 + the "reset-credit" ID from before sort to 251; the 51 smallest go.
	if len(again) != 51 {
		t.Fatalf("len = %d", len(again))
	}
}

func compare(t *testing.T, path, fixture string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs from Swift\n got: %s\nwant: %s", fixture, got, want)
	}
}

func TestDecisionsMatchSwift(t *testing.T) {
	now := at(t, noon)
	decisions := map[string][]Alert{}

	reset := at(t, "2026-08-23T18:00:00Z")
	decisions["quota"] = QuotaAlerts(QuotaInput{
		Primary:   Window{82, &reset},
		Secondary: &Window{100, ptr(reset.Add(24 * time.Hour))},
		Additional: []NamedWindow{
			{"GPT-5.3-Codex-Spark", Window{96, nil}},
			{"Other", Window{79, &reset}},
		},
	}, legacyQuotaThresholds)
	day := 24 * time.Hour
	exp := func(d time.Duration) *time.Time { return ptr(now.Add(d)) }
	decisions["resetCredit"] = ResetCreditAlerts([]ResetCredit{
		{ID: ptr("c1"), Status: "available", ExpiresAt: exp(3 * day), RemainingSeconds: (3 * day).Seconds()},
		{Status: "available", ExpiresAt: exp(day), RemainingSeconds: day.Seconds()},
		{ID: ptr("c3"), Status: "available", ExpiresAt: exp(8 * day), RemainingSeconds: (8 * day).Seconds()},
		{ID: ptr("c4"), Status: "available"},
		{ID: ptr("c5"), Status: "used", ExpiresAt: exp(day), RemainingSeconds: day.Seconds()},
		{ID: ptr("c6"), Status: "available", ExpiresAt: exp(7 * day), RemainingSeconds: (7 * day).Seconds()},
	})

	decisions["firstLoad"] = decide(t, nil,
		ResetTodayState{Events: []Event{completed(t, "100", ResetBanked, "2026-08-23T10:00:00Z")}}, noon)
	existing := completed(t, "050", ResetGlobal, "2026-08-23T01:00:00Z")
	decisions["newOccurrences"] = decide(t, &ResetTodayState{Events: []Event{existing}}, ResetTodayState{Events: []Event{
		completed(t, "300", ResetBanked, "2026-08-23T04:00:00Z"),
		existing,
		completed(t, "200", ResetGlobal, "2026-08-23T04:00:00Z"),
		completed(t, "100", ResetGlobal, "2026-08-23T03:00:00Z"),
		completed(t, "tomorrow", ResetGlobal, "2026-08-24T03:00:00Z"),
	}}, noon)
	decisions["manualMixed"] = decide(t, &ResetTodayState{}, ResetTodayState{ManualCompletions: []ManualCompletion{
		manual(t, "manual:mixed-1", "2026-08-23T04:00:00Z", "2026-08-24T04:00:00Z", "900", []string{"900", "901"}, ResetBanked, ResetGlobal),
	}}, noon)
	decisions["completedThenManual"] = decide(t,
		&ResetTodayState{Events: []Event{completed(t, "900", ResetGlobal, "2026-08-23T04:00:00Z")}},
		ResetTodayState{ManualCompletions: []ManualCompletion{
			manual(t, "manual:cl_later", "2026-08-23T04:00:00Z", "2026-09-02T04:00:00Z", "900", []string{"900"}, ResetGlobal),
		}}, noon)

	explicit, contextual := BasisExplicit, BasisContextualInference
	up := func(s UpcomingReset) *ResetTodayState { return &ResetTodayState{Upcoming: &s} }
	// Two schedules at the same time: merged type, minimum confidence, and the
	// basis of the first one because they are not all contextual.
	merged := schedule(t, "500", "", ResetGlobalAndBanked, "2026-08-23T12:25:00Z", &explicit, 0.72)
	decisions["upcomingMerged"] = decide(t, up(merged), *up(merged), noon)
	inferred := schedule(t, "preview", "", ResetGlobal, "2026-08-23T12:25:00Z", &contextual, 0.876)
	decisions["upcomingInferred"] = decide(t, up(inferred), *up(inferred), noon)
	sixty := schedule(t, "600", "", ResetBanked, "2026-08-23T12:45:30Z", &explicit, 0.5)
	decisions["upcomingSixty"] = decide(t, up(sixty), *up(sixty), noon)
	far := schedule(t, "700", "", ResetGlobal, "2026-08-23T14:00:00Z", nil, 1)
	decisions["upcomingFar"] = decide(t, up(far), *up(far), noon)
	expired := schedule(t, "800", "", ResetGlobal, "2026-08-23T11:00:00Z", nil, 1)
	decisions["upcomingExpired"] = decide(t, up(expired), *up(expired), noon)

	got, err := swiftjson.Marshal(decisions, false)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/swift_decisions.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("decisions differ from Swift\n got: %s\nwant: %s", got, want)
	}
	if strings.Contains(string(got), "null") {
		t.Fatal("nil optionals must be omitted")
	}
}
