package estimate

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 2026-06-29T12:00:00Z, the instant the Swift tests use.
var now = time.Unix(1_782_710_400, 0).UTC()

func ptr[T any](v T) *T { return &v }

func win(used int, minutes int, resets time.Time) Window {
	return NewWindow(used, ptr(minutes), &resets)
}

func weeklyQuota(used int) Quota {
	secondary := win(used, 10_080, now.Add(3*24*time.Hour))
	return Quota{Primary: win(10, 300, now.Add(time.Hour)), Secondary: &secondary}
}

// day mirrors the Swift test helper: Credits drive the token count.
func day(date string, credits float64) DailyRow {
	return DailyRow{
		Date: date, RawCredits: credits, TotalTokens: int(credits * 1000), Turns: 2,
		CreditsReported: true, TotalsReported: true,
	}
}

func TestPicksSecondaryWeeklyWindow(t *testing.T) {
	secondary := win(40, 10_080, now.Add(3*24*time.Hour))
	q := Quota{Primary: win(20, 300, now.Add(time.Hour)), Secondary: &secondary}
	w := WeeklyWindow(q)
	if w == nil || w.UsedPercent != 40 || !IsWeekly(*w) {
		t.Fatalf("weekly = %+v", w)
	}
}

func TestPicksPrimaryWhenWeekly(t *testing.T) {
	secondary := win(5, 300, now.Add(time.Hour))
	q := Quota{Primary: win(22, 10_080, now.Add(2*24*time.Hour)), Secondary: &secondary}
	w := WeeklyWindow(q)
	if w == nil || *w.WindowMinutes != 10_080 {
		t.Fatalf("weekly = %+v", w)
	}
}

func TestFiveHourIsNotWeekly(t *testing.T) {
	q := Quota{Primary: win(50, 300, now.Add(time.Hour))}
	if WeeklyWindow(q) != nil || !IsFiveHour(q.Primary) {
		t.Fatal("a 5-hour window is not weekly")
	}
	s := Make(q, []DailyRow{day("2026-06-29", 12)}, ModeRollingWeek, nil, now)
	if s.CanExtrapolate || s.EstimatedCredits != nil || s.UsedCredits != 12 {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestAutoUsesOfficialCycleOfSelectedMeter(t *testing.T) {
	reset := time.Unix(1_783_315_200, 0) // 2026-07-06T12:00:00Z
	secondary := win(3, 10_080, reset)
	q := Quota{Primary: win(10, 300, now), Secondary: &secondary}
	s := Make(q, []DailyRow{
		day("2026-06-22", 50_000), day("2026-06-28", 20_000), day("2026-06-29", 3_379.6),
	}, ModeAuto, nil, now)
	if s.CycleStartDate != "2026-06-29" || s.UsedCredits != 3_379.6 || s.UsedPercent != 3 {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.EstimatedCredits == nil || math.Abs(*s.EstimatedCredits-3_379.6/0.03) > 0.01 {
		t.Fatalf("estimated = %v", s.EstimatedCredits)
	}
	if s.EstimatedUSD == nil || math.Abs(*s.EstimatedUSD-(3_379.6/0.03)*0.04) > 0.01 {
		t.Fatalf("usd = %v", s.EstimatedUSD)
	}
}

func TestRollingWeekStartsSixDaysBeforeLatestRow(t *testing.T) {
	s := Make(weeklyQuota(25), []DailyRow{
		day("2026-06-20", 9), day("2026-06-22", 10), day("2026-06-29", 15),
	}, ModeRollingWeek, nil, now)
	if s.CycleStartDate != "2026-06-23" || len(s.CurrentRows) != 1 || s.CurrentRows[0].Date != "2026-06-29" {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.UsedCredits != 15 || *s.EstimatedCredits != 60 || *s.EstimatedUSD != 2.4 {
		t.Fatalf("estimate = %v / %v", *s.EstimatedCredits, *s.EstimatedUSD)
	}
}

func TestAutoWeekUsesResetMinusWindowLength(t *testing.T) {
	reset := time.Unix(1_783_315_200, 0)
	secondary := win(50, 10_080, reset)
	q := Quota{Primary: win(10, 300, now), Secondary: &secondary}
	s := Make(q, []DailyRow{day("2026-06-28", 5), day("2026-06-29", 15), day("2026-07-01", 20)}, ModeAuto, nil, now)
	if s.CycleStartDate != "2026-06-29" || s.UsedCredits != 35 || *s.EstimatedCredits != 70 {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestZeroPercentAndZeroCreditsDoNotExtrapolate(t *testing.T) {
	zeroPercent := Make(weeklyQuota(0), []DailyRow{day("2026-06-29", 8)}, ModeRollingWeek, nil, now)
	if zeroPercent.CanExtrapolate || zeroPercent.EstimatedCredits != nil || zeroPercent.UsedCredits != 8 {
		t.Fatalf("zero percent: %+v", zeroPercent)
	}
	zeroCredits := Make(weeklyQuota(20), []DailyRow{day("2026-06-29", 0)}, ModeRollingWeek, nil, now)
	if zeroCredits.CanExtrapolate || zeroCredits.EstimatedCredits != nil {
		t.Fatalf("zero credits: %+v", zeroCredits)
	}
}

func TestZeroCreditsKeepTokenCost(t *testing.T) {
	row := DailyRow{
		Date: "2026-06-29", TotalTokens: 694_950_000, Turns: 1, EstimatedUSD: ptr(123.45),
		CreditsReported: true, TotalsReported: true,
	}
	s := Make(weeklyQuota(47), []DailyRow{row}, ModeRollingWeek, nil, now)
	if s.CurrentRows[0].USD == nil || *s.CurrentRows[0].USD != 123.45 || s.UsedCredits != 0 || s.EstimatedCredits != nil {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestUsesExactUsedPercent(t *testing.T) {
	secondary := NewWindow(12, ptr(10_080), ptr(now.Add(24*time.Hour)))
	secondary.UsedPercentExact = 12.5
	q := Quota{Primary: win(20, 300, now), Secondary: &secondary}
	s := Make(q, []DailyRow{day("2026-06-29", 25)}, ModeRollingWeek, nil, now)
	if s.UsedPercent != 12.5 || *s.EstimatedCredits != 200 {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestSameCycleHistoryIsNotACut(t *testing.T) {
	sample := HistorySample{"2026-06-23", 250, 40, 100, now.Add(-time.Hour)}
	s := Make(weeklyQuota(40), []DailyRow{day("2026-06-29", 80)}, ModeRollingWeek, []HistorySample{sample}, now)
	if s.CycleStartDate != "2026-06-23" || *s.EstimatedCredits != 200 || s.PreviousEstimate != nil || s.ChangeKind != nil {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestSignificantDecreaseIsMarked(t *testing.T) {
	previous := HistorySample{"2026-06-16", 200, 80, 160, now.Add(-7 * 24 * time.Hour)}
	s := Make(weeklyQuota(50), []DailyRow{day("2026-06-29", 80)}, ModeRollingWeek, []HistorySample{previous}, now)
	if *s.EstimatedCredits != 160 || s.ChangeKind == nil || *s.ChangeKind != ChangeDecreased || *s.ChangePercent != -20 {
		t.Fatalf("snapshot = %+v", s)
	}
	if s.PreviousEstimate == nil || s.PreviousEstimate.CycleStartDate != "2026-06-16" {
		t.Fatalf("previous = %+v", s.PreviousEstimate)
	}
}

func TestIncreaseAndJitter(t *testing.T) {
	previous := HistorySample{"2026-06-16", 200, 40, 80, now.Add(-7 * 24 * time.Hour)}
	similar := Make(weeklyQuota(50), []DailyRow{day("2026-06-29", 96)}, ModeRollingWeek, []HistorySample{previous}, now)
	if *similar.EstimatedCredits != 192 || *similar.ChangeKind != ChangeSimilar {
		t.Fatalf("similar: %+v", similar)
	}
	up := Make(weeklyQuota(50), []DailyRow{day("2026-06-29", 125)}, ModeRollingWeek, []HistorySample{previous}, now)
	if *up.EstimatedCredits != 250 || *up.ChangeKind != ChangeIncreased || *up.ChangePercent != 25 {
		t.Fatalf("increase: %+v", up)
	}
	zeroPrevious := HistorySample{"2026-06-16", 0, 40, 80, now}
	none := Make(weeklyQuota(50), []DailyRow{day("2026-06-29", 96)}, ModeRollingWeek, []HistorySample{zeroPrevious}, now)
	if none.ChangeKind != nil || none.ChangePercent != nil || none.PreviousEstimate == nil {
		t.Fatalf("a zero previous estimate has no percentage: %+v", none)
	}
}

func TestCompactTokens(t *testing.T) {
	for in, want := range map[int]string{12: "12", 1_500: "1.50K", 2_250_000: "2.25M", 999: "999", 1_000_000: "1.00M"} {
		if got := CompactTokens(in); got != want {
			t.Errorf("CompactTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParseWindowModeDefaultsToAuto(t *testing.T) {
	if ParseWindowMode("rollingWeek") != ModeRollingWeek || ParseWindowMode("") != ModeAuto || ParseWindowMode("x") != ModeAuto {
		t.Fatal("unexpected mode parsing")
	}
}

func TestAnalyticsRange(t *testing.T) {
	start, end, _, _ := AnalyticsRange(time.Date(2026, 6, 29, 23, 59, 0, 0, time.UTC))
	if start != "2026-05-30" || end != "2026-06-30" {
		t.Fatalf("range = %s..%s", start, end)
	}
}

// History store.

func TestHistoryStoreUpsertsAndCaps(t *testing.T) {
	store := HistoryStore{Path: filepath.Join(t.TempDir(), "quota-estimate-history.json")}
	const key = "oauth:user:abc|account:acct"
	for i := 0; i < 14; i++ {
		_, err := store.Upsert(key, HistorySample{
			CycleStartDate: "2026-05-" + twoDigits(i+1), EstimatedCredits: float64(100 + i), UsedPercent: 40, UsedCredits: 40,
			RecordedAt: now.Add(time.Duration(i) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	first := store.Load(key)
	if len(first) != 12 || first[0].CycleStartDate != "2026-05-03" || first[11].CycleStartDate != "2026-05-14" {
		t.Fatalf("first = %+v", first)
	}
	updated, err := store.Upsert(key, HistorySample{"2026-05-14", 50, 20, 10, now.Add(1000 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if updated[len(updated)-1].EstimatedCredits != 50 {
		t.Fatalf("updated = %+v", updated[len(updated)-1])
	}
	if len(store.Load("other")) != 0 {
		t.Fatal("another account must not see the history")
	}
}

func twoDigits(n int) string {
	return string([]byte{byte('0' + n/10), byte('0' + n%10)})
}

func TestOlderSampleOfSameCycleDoesNotReplaceNewer(t *testing.T) {
	newer := HistorySample{"2026-06-22", 200, 40, 80, now}
	older := HistorySample{"2026-06-22", 100, 20, 20, now.Add(-time.Hour)}
	got := Merging([]HistorySample{newer}, older)
	if len(got) != 1 || got[0].EstimatedCredits != 200 {
		t.Fatalf("got %+v", got)
	}
	tie := HistorySample{"2026-06-22", 300, 50, 150, now}
	if got := Merging([]HistorySample{newer}, tie); got[0].EstimatedCredits != 300 {
		t.Fatalf("on equal times the later sample wins: %+v", got)
	}
}

func TestCorruptOrIncompleteHistoryReadsAsEmptyAndIsReplaced(t *testing.T) {
	for name, content := range map[string]string{
		"garbage":       "{not json",
		"no accounts":   `{}`,
		"missing field": `{"accounts":{"k":[{"cycleStartDate":"2026-06-22","estimatedCredits":1,"usedPercent":1,"usedCredits":1}]}}`,
		"bad date":      `{"accounts":{"k":[{"cycleStartDate":"2026-06-22","estimatedCredits":1,"usedPercent":1,"usedCredits":1,"recordedAt":"yesterday"}]}}`,
	} {
		path := filepath.Join(t.TempDir(), "h.json")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		store := HistoryStore{Path: path}
		if got := store.Load("k"); len(got) != 0 {
			t.Fatalf("%s: got %+v", name, got)
		}
		if _, err := store.Upsert("k", HistorySample{"2026-06-22", 1, 1, 1, now}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := store.Load("k"); len(got) != 1 {
			t.Fatalf("%s: after upsert %+v", name, got)
		}
	}
	if got := (HistoryStore{Path: filepath.Join(t.TempDir(), "absent.json")}).Load("k"); len(got) != 0 {
		t.Fatalf("absent file: %+v", got)
	}
}

func TestDefaultHistoryPath(t *testing.T) {
	got := DefaultHistoryPath("/home/u")
	if filepath.Base(got) != HistoryFileName || filepath.Base(filepath.Dir(got)) != ".quosparr" {
		t.Fatalf("path = %s", got)
	}
}

// TestHistoryFileMatchesSwiftBytes rebuilds the history the Swift store wrote
// (see testdata/swift_history.json) and compares the file byte for byte.
func TestHistoryFileMatchesSwiftBytes(t *testing.T) {
	store := HistoryStore{Path: filepath.Join(t.TempDir(), "quota-estimate-history.json")}
	base := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	const key = "oauth:user:abc|account:acct"
	for i := 0; i < 14; i++ {
		_, err := store.Upsert(key, HistorySample{
			CycleStartDate:   "2026-05-" + twoDigits(i+1),
			EstimatedCredits: float64(100+i) + 0.5,
			UsedPercent:      40 + float64(i)/3,
			UsedCredits:      40.25 * float64(i+1),
			RecordedAt:       base.Add(time.Duration(i)*time.Minute + 750*time.Millisecond),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert(t, store, key, HistorySample{"2026-05-14", 50, 20, 10, base.Add(1000 * time.Second)})
	mustUpsert(t, store, "unknown", HistorySample{"2026-06-22", 112662.5, 3, 3379.6, base})
	mustUpsert(t, store, "id:api-é/中", HistorySample{"2026-06-22", 1e20, 0.00001, 0, base})
	got, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/swift_history.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("history file differs from Swift\n got: %s\nwant: %s", got, want)
	}
	// And the Swift file loads back through the Go store.
	swiftStore := HistoryStore{Path: "testdata/swift_history.json"}
	loaded := swiftStore.Load(key)
	if len(loaded) != 12 || loaded[11].EstimatedCredits != 50 {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func mustUpsert(t *testing.T, s HistoryStore, key string, sample HistorySample) {
	t.Helper()
	if _, err := s.Upsert(key, sample); err != nil {
		t.Fatal(err)
	}
}

// Availability (ported from QuotaEstimateAvailabilityTests, with the analytics
// decoding replaced by the DailyRow values it would produce).

var availNow = time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

func availQuota(percent int, weekly bool) Quota {
	q := Quota{Primary: win(10, 300, availNow.Add(time.Hour))}
	if weekly {
		w := win(percent, 10_080, availNow.Add(24*time.Hour))
		q.Secondary = &w
	}
	return q
}

func availEstimate(days []DailyRow, percent int, weekly bool) Snapshot {
	return Make(availQuota(percent, weekly), days, ModeRollingWeek, nil, availNow)
}

func used(date string, credits float64, reported bool, usd *float64, tokens int) DailyRow {
	return DailyRow{Date: date, RawCredits: credits, TotalTokens: tokens, Turns: 0, EstimatedUSD: usd, CreditsReported: reported, TotalsReported: true}
}

func TestMissingCreditsOnUsedDayPreventExtrapolation(t *testing.T) {
	s := availEstimate([]DailyRow{
		used("2026-09-01", 47, true, ptr(1.75), 1_000_000),
		used("2026-09-02", 0, false, ptr(3.5), 2_000_000),
	}, 47, true)
	if s.UnavailableReason == nil || *s.UnavailableReason != ReasonMissingCredits || s.CreditsComplete() ||
		s.UsedCredits != 47 || s.CanExtrapolate || s.EstimatedCredits != nil || SampleFrom(s) != nil {
		t.Fatalf("snapshot = %+v", s)
	}
	if usd := s.APIEquivalentUSD(); usd == nil || *usd != 5.25 {
		t.Fatalf("api usd = %v", usd)
	}
}

func TestExplicitZeroCreditsKeepTokenCost(t *testing.T) {
	s := availEstimate([]DailyRow{used("2026-09-02", 0, true, ptr(1214.1), 694_950_000)}, 47, true)
	if *s.UnavailableReason != ReasonZeroCreditsWithUsage || !s.CreditsComplete() || !s.CurrentRows[0].CreditsReported ||
		s.UsedCredits != 0 || *s.APIEquivalentUSD() <= 0 || SampleFrom(s) != nil {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestMissingDayIsUnknownNotUnused(t *testing.T) {
	unknown := DailyRow{Date: "2026-09-02"}
	usedDay := used("2026-09-01", 47, true, ptr(1.75), 1_000_000)
	for _, days := range [][]DailyRow{{unknown}, {usedDay, unknown}} {
		s := availEstimate(days, 47, true)
		if s.CurrentRows[len(s.CurrentRows)-1].TotalsReported || *s.UnavailableReason != ReasonMissingCredits ||
			s.CreditsComplete() || s.APIEquivalentUSD() != nil || s.EstimatedCredits != nil || SampleFrom(s) != nil {
			t.Fatalf("snapshot = %+v", s)
		}
	}
}

func TestPartialCostIsUnavailable(t *testing.T) {
	s := availEstimate([]DailyRow{
		used("2026-09-01", 20, true, ptr(1.75), 1_000_000),
		used("2026-09-02", 27, true, nil, 2_000_000),
	}, 47, true)
	if s.CurrentRows[0].USD == nil || s.CurrentRows[1].USD != nil || s.APIEquivalentUSD() != nil ||
		*s.EstimatedCredits != 100 || *s.EstimatedUSD != 4 || s.UnavailableReason != nil {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestTotalTokensAloneDoNotInventCost(t *testing.T) {
	s := availEstimate([]DailyRow{used("2026-09-02", 0, true, nil, 694_950_000)}, 47, true)
	if s.CurrentRows[0].Tokens != 694_950_000 || s.CurrentRows[0].USD != nil || s.APIEquivalentUSD() != nil ||
		*s.UnavailableReason != ReasonZeroCreditsWithUsage || s.EstimatedCredits != nil {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestPositiveCreditsKeepFormula(t *testing.T) {
	a, b := 1.75, 3.5
	s := availEstimate([]DailyRow{
		used("2026-09-01", 20, true, &a, 1_000_000),
		used("2026-09-02", 27, true, &b, 2_000_000),
	}, 47, true)
	if !s.CanExtrapolate || !s.CreditsComplete() || s.UnavailableReason != nil || *s.EstimatedCredits != 100 {
		t.Fatalf("snapshot = %+v", s)
	}
	if usd := s.APIEquivalentUSD(); usd == nil || *usd != a+b || *usd == s.UsedUSD {
		t.Fatalf("api usd = %v used usd = %v", usd, s.UsedUSD)
	}
	if through := s.StatsThroughDate(); through == nil || *through != "2026-09-02" || !s.CalculatedAt.Equal(availNow) {
		t.Fatalf("through = %v at = %v", through, s.CalculatedAt)
	}
	if sample := SampleFrom(s); sample == nil || sample.EstimatedCredits != 100 {
		t.Fatalf("sample = %+v", sample)
	}
}

func TestUnusedRowDoesNotBlockReportedUsage(t *testing.T) {
	first := used("2026-09-01", 47, true, ptr(1.75), 1_000_000)
	s := availEstimate([]DailyRow{first, used("2026-09-02", 0, false, nil, 0)}, 47, true)
	if !s.CreditsComplete() || *s.EstimatedCredits != 100 || *s.APIEquivalentUSD() != 1.75 {
		t.Fatalf("snapshot = %+v", s)
	}
}

func TestOtherUnavailableReasons(t *testing.T) {
	empty := availEstimate(nil, 0, true)
	if *empty.UnavailableReason != ReasonNoUsage || empty.StatsThroughDate() != nil || empty.APIEquivalentUSD() != nil {
		t.Fatalf("empty: %+v", empty)
	}
	idle := availEstimate([]DailyRow{used("2026-09-02", 0, true, nil, 0)}, 0, true)
	if *idle.UnavailableReason != ReasonNoUsage {
		t.Fatalf("idle: %+v", idle)
	}
	// A used window without a used day means the statistics lag behind.
	for name, days := range map[string][]DailyRow{"empty": nil, "idle": {used("2026-09-02", 0, true, nil, 0)}} {
		if s := availEstimate(days, 47, true); *s.UnavailableReason != ReasonStatsNotSynced || s.EstimatedCredits != nil {
			t.Fatalf("%s with used window: %+v", name, s)
		}
	}
	row := used("2026-09-02", 47, true, nil, 1000)
	if r := availEstimate([]DailyRow{row}, 0, true).UnavailableReason; r == nil || *r != ReasonZeroPercent {
		t.Fatalf("zero percent: %v", r)
	}
	if r := availEstimate([]DailyRow{row}, 47, false).UnavailableReason; r == nil || *r != ReasonUnavailableWindow {
		t.Fatalf("no weekly window: %v", r)
	}
}

func TestStatsBeforeCycleAreNotSyncedAndShowTheirDate(t *testing.T) {
	// The weekly window reset at 01:00 today; the statistics end yesterday.
	minutes := 10_080
	resets := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	q := Quota{Primary: Window{UsedPercent: 10, UsedPercentExact: 10, WindowMinutes: &minutes, ResetsAt: &resets}}
	s := Make(q, []DailyRow{used("2026-09-02", 50, true, nil, 1000)}, ModeAuto, nil, availNow)
	if s.CycleStartDate != "2026-09-03" || len(s.CurrentRows) != 0 || *s.UnavailableReason != ReasonStatsNotSynced {
		t.Fatalf("snapshot = %+v", s)
	}
	if through := s.StatsThroughDate(); through == nil || *through != "2026-09-02" {
		t.Fatalf("through = %v", through)
	}
}

func TestLegacyCreditsWithUnknownProvenanceCannotExtrapolate(t *testing.T) {
	row := DailyRow{Date: "2026-09-02", RawCredits: 47, TotalTokens: 1000, TotalsReported: false}
	s := Make(availQuota(47, true), []DailyRow{row}, ModeAuto, nil, availNow)
	if *s.UnavailableReason != ReasonMissingCredits || s.CreditsComplete() || SampleFrom(s) != nil {
		t.Fatalf("snapshot = %+v", s)
	}
}
