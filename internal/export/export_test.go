package export

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
)

func ptr[T any](v T) *T { return &v }

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func equalBytes(t testing.TB, name string, got []byte) {
	t.Helper()
	if want := fixture(t, name); !bytes.Equal(got, want) {
		t.Fatalf("%s differs from the Swift output\n got: %s\nwant: %s", name, got, want)
	}
}

func utc(y int, m time.Month, d, h, mi, s int) time.Time {
	return time.Date(y, m, d, h, mi, s, 0, time.UTC)
}

// Status.

func statusFixtureInputs() (time.Time, QuotaInput, CostSummary, sessions.Summary) {
	now := utc(2026, 6, 30, 10, 0, 0)
	totals := sessions.Totals{TotalTokens: 1000, UncachedInputTokens: 700, CachedInputTokens: 200, OutputTokens: 100, Turns: 3, Threads: 1}
	quota := QuotaInput{
		Plan:      ptr("pro"),
		Primary:   Window{25, ptr(now.Add(time.Hour))},
		Secondary: &Window{60, nil},
		Additional: []NamedWindow{
			{"GPT-5.3-Codex-Spark", Window{3, &now}},
		},
		CreditsBalance: ptr(12.5),
		UpdatedAt:      now,
	}
	cost := CostSummary{
		Source: "localSessions", Confidence: "priced",
		Window:       Interval{Start: now.Add(-time.Hour), Duration: 3600},
		EstimatedUSD: ptr(swiftjson.MustDecimal("1.25")),
		Totals:       totals,
		DailyRows: []CostDailyRow{
			{Date: "2026-06-29", Totals: totals, EstimatedUSD: ptr(swiftjson.MustDecimal("0.216048")), RawCredits: 3379.6,
				CreditsReported: ptr(true), TotalsReported: ptr(true)},
			{Date: "2026-06-30", RawCredits: 0},
		},
		ModelRows:   []CostBreakdownRow{{Name: "gpt-5.5", Totals: totals, EstimatedUSD: ptr(swiftjson.MustDecimal("1.25"))}},
		ProjectRows: []CostBreakdownRow{{Name: "quosparr", Totals: totals, EstimatedUSD: ptr(swiftjson.MustDecimal("1.25"))}},
		RawCredits:  3379.6, Warnings: []string{"analytics-token-parts-missing"},
		PricingVersion: "test/1", CalculatedAt: now,
	}
	sum := sessions.Summary{Items: []sessions.Item{
		{ID: "s1", Title: `Fix "status" / <json>`, ProjectName: "quosparr", Cwd: ptr("/Users/me/dev/quosparr"),
			UpdatedAt: now, State: sessions.StateNeedsAttention, Totals: totals, EstimatedUSD: ptr(swiftjson.MustDecimal("1.25"))},
		{ID: "s2", Title: "Untitled", ProjectName: "Unknown project",
			UpdatedAt: now.Add(-60500 * time.Millisecond), State: sessions.StateFailed},
	}}
	return now, quota, cost, sum
}

func TestStatusFileMatchesSwift(t *testing.T) {
	now, quota, cost, sum := statusFixtureInputs()
	q := NewStatusQuota(quota)
	dir := t.TempDir()
	exporter := StatusExporter{Path: filepath.Join(dir, "nested", "status.json")}

	cases := []struct {
		name string
		snap StatusSnapshot
	}{
		{"swift_status_full.json", StatusSnapshot{GeneratedAt: now, Quota: &q, Cost: &cost, Sessions: &sum}},
		{"swift_status_empty.json", StatusSnapshot{GeneratedAt: now}},
		{"swift_status_minimal.json", StatusSnapshot{GeneratedAt: now,
			Quota: ptr(NewStatusQuota(QuotaInput{Primary: Window{UsedPercent: 100}, UpdatedAt: now})), Sessions: &sessions.Summary{}}},
	}
	for _, c := range cases {
		if err := exporter.Save(c.snap); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(exporter.Path)
		if err != nil {
			t.Fatal(err)
		}
		equalBytes(t, c.name, got)
	}
}

func TestStatusFileHasNoSecretsAndIsPrivateEnoughToShare(t *testing.T) {
	now, quota, _, _ := statusFixtureInputs()
	data, err := StatusSnapshot{GeneratedAt: now, Quota: ptr(NewStatusQuota(quota))}.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"token", "Token", "auth", "secret", "apiKey", "email"} {
		if strings.Contains(string(data), word) {
			t.Fatalf("status.json mentions %q: %s", word, data)
		}
	}
	path := filepath.Join(t.TempDir(), "status.json")
	if err := (StatusExporter{Path: path}).Save(StatusSnapshot{GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
			t.Fatalf("mode = %v", info.Mode().Perm())
		}
	}
	if got := DefaultStatusPath("/h"); filepath.Base(got) != "status.json" || filepath.Base(filepath.Dir(got)) != ".quosparr" {
		t.Fatalf("path = %s", got)
	}
}

// Widget snapshot.

func widgetFixtureSnapshot() Snapshot {
	date0 := time.Unix(1_800_000_000, 0).UTC()
	timeline := []ResetTodayEntry{
		{EffectiveAt: date0, Reason: ReasonUpcoming, State: ResetYes, ResetType: ptr(alerts.ResetBanked),
			NextScheduledAt: ptr(date0.Add(time.Hour)), NextScheduledResetType: ptr(alerts.ResetBanked),
			ScheduleBasis: ptr(alerts.BasisExplicit), ConfidencePercent: ptr(92), ConfidenceBand: ptr(BandOK)},
		{EffectiveAt: date0.Add(time.Hour), Reason: ReasonGrace, State: ResetYes, ResetType: ptr(alerts.ResetBanked),
			ScheduleBasis: ptr(alerts.BasisExplicit), ConfidencePercent: ptr(92), ConfidenceBand: ptr(BandOK)},
	}
	codex := NewProviderSnapshot(ProviderSnapshot{
		Provider: ProviderCodex, Availability: Available, Plan: ptr("Plus / Team"),
		UpdatedAt: ptr(date0.Add(900 * time.Millisecond)),
		Quota: []WidgetQuota{
			NewWidgetQuota("Model", ptr(60), QuotaModelSpecific, 150, -5, nil),
			NewWidgetQuota("Weekly", ptr(10_080), QuotaStandard, 40, 60, ptr(date0.Add(24*time.Hour))),
			NewWidgetQuota("Unknown", nil, QuotaStandard, 25, 75, nil),
			NewWidgetQuota("5 hours", ptr(300), QuotaStandard, 25, 75, ptr(date0.Add(3600400*time.Millisecond))),
		},
		BalanceUSD: ptr(swiftjson.MustDecimal("12.5")), APIEquivalentCostUSD: ptr(swiftjson.MustDecimal("4.25")),
		TokenSource:  TokensAllDevices,
		DailyTokens:  []DailyTokens{NewDailyTokens("2026-08-06", 99), NewDailyTokens("2026-08-05", -3)},
		ResetCredits: ptr(NewResetCredits(2, -1)),
	})
	grok := NewProviderSnapshot(ProviderSnapshot{Provider: ProviderGrok, Availability: NotLoggedIn, TokenSource: TokensThisMac})
	reset := &ResetTodaySnapshot{
		State: ResetUnknown, ResetType: ptr(alerts.ResetGlobalAndBanked), NextScheduledAt: ptr(date0.Add(time.Hour)),
		NextScheduledResetType: ptr(alerts.ResetBanked), FetchedAt: date0, ConfidencePercent: ptr(92),
		ConfidenceBand: ptr(BandOK), Timeline: timeline,
	}
	return NewSnapshot(date0, l10n.SimplifiedChinese, []ProviderSnapshot{grok, codex}, reset)
}

func TestWidgetSnapshotMatchesSwiftBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "widget-dir", SnapshotFileName)
	store := SnapshotStore{Path: path}
	if err := store.Save(widgetFixtureSnapshot()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	equalBytes(t, "swift_widget_full.json", got)

	empty := NewSnapshot(time.Unix(1_800_000_000, 0).UTC(), l10n.English, nil, nil)
	if err := store.Save(empty); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	equalBytes(t, "swift_widget_empty.json", got)
}

func TestWidgetSnapshotPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	dir := filepath.Join(t.TempDir(), "new")
	path := filepath.Join(dir, SnapshotFileName)
	if err := (SnapshotStore{Path: path}).Save(widgetFixtureSnapshot()); err != nil {
		t.Fatal(err)
	}
	fileInfo, _ := os.Stat(path)
	dirInfo, _ := os.Stat(dir)
	wantFile := strings.TrimSpace(string(fixture(t, "swift_widget_mode.txt")))
	if wantFile != "600" || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v (Swift %s)", fileInfo.Mode().Perm(), wantFile)
	}
	if want := strings.TrimSpace(string(fixture(t, "swift_widget_dir_mode.txt"))); want != "700" {
		t.Fatalf("Swift created the snapshot directory with mode %s", want)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v", dirInfo.Mode().Perm())
	}
}

func TestWidgetSnapshotLoadRoundTripAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), SnapshotFileName)
	store := SnapshotStore{Path: path}
	if _, err := store.Load(); !errors.Is(err, ErrMissing) {
		t.Fatalf("err = %v", err)
	}
	want := widgetFixtureSnapshot()
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	// Dates are written with whole seconds.
	again, _ := got.Encode()
	original, _ := want.Encode()
	if !bytes.Equal(again, original) {
		t.Fatalf("round trip changed the snapshot\n got: %s\nwant: %s", again, original)
	}
	if p := got.ProviderOf(ProviderCodex); p == nil || p.Quota[0].Title != "5 hours" {
		t.Fatalf("provider = %+v", p)
	}
	if got.ProviderOf("nobody") != nil {
		t.Fatal("unknown provider")
	}

	bad := strings.Replace(string(original), `"schemaVersion":1`, `"schemaVersion":2`, 1)
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	var unsupported UnsupportedSchemaError
	if _, err := store.Load(); !errors.As(err, &unsupported) || unsupported.Version != 2 {
		t.Fatalf("err = %v", err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("garbage must not load")
	}
}

func TestWidgetModelClamping(t *testing.T) {
	q := NewWidgetQuota("x", nil, QuotaStandard, 150, -5, nil)
	if q.UsedPercent != 100 || q.RemainingPercent != 0 {
		t.Fatalf("%+v", q)
	}
	if NewDailyTokens("d", -3).Tokens != 0 {
		t.Fatal("negative tokens")
	}
	if r := NewResetCredits(-1, -2); r.AvailableCount != 0 || r.ExpiringCount != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestSortedQuota(t *testing.T) {
	in := []WidgetQuota{
		{Title: "m1", Source: QuotaModelSpecific, WindowMinutes: ptr(10)},
		{Title: "week", Source: QuotaStandard, WindowMinutes: ptr(10_080)},
		{Title: "none", Source: QuotaStandard},
		{Title: "5h", Source: QuotaStandard, WindowMinutes: ptr(300)},
		{Title: "m0", Source: QuotaModelSpecific},
	}
	var titles []string
	for _, q := range SortedQuota(in) {
		titles = append(titles, q.Title)
	}
	if !reflect.DeepEqual(titles, []string{"5h", "week", "none", "m1", "m0"}) {
		t.Fatalf("titles = %v", titles)
	}
	if in[0].Title != "m1" {
		t.Fatal("the input must not be reordered")
	}
}

func TestSnapshotStalenessAndTimeline(t *testing.T) {
	s := widgetFixtureSnapshot()
	if s.IsStale(s.GeneratedAt.Add(time.Hour)) || !s.IsStale(s.GeneratedAt.Add(time.Hour+time.Second)) {
		t.Fatal("stale after exactly one hour")
	}
	r := s.ResetToday
	if got := r.Presentation(r.FetchedAt.Add(-time.Minute)); got == nil || got.Reason != ReasonUpcoming {
		t.Fatalf("before the first entry the first applies: %+v", got)
	}
	if got := r.Presentation(r.FetchedAt.Add(time.Hour)); got.Reason != ReasonGrace {
		t.Fatalf("at an entry's time it applies: %+v", got)
	}
	if got := r.TransitionDates(r.FetchedAt); len(got) != 1 || !got[0].Equal(r.FetchedAt.Add(time.Hour)) {
		t.Fatalf("transitions = %v", got)
	}
	if (ResetTodaySnapshot{}).Presentation(time.Now()) != nil {
		t.Fatal("legacy snapshots have no presentation")
	}
}

func TestLegacyTimelineIsOmittedAndEmptyIsWritten(t *testing.T) {
	s := NewSnapshot(time.Unix(0, 0), l10n.English, nil, &ResetTodaySnapshot{State: ResetNo, FetchedAt: time.Unix(0, 0)})
	data, _ := s.Encode()
	if strings.Contains(string(data), "timeline") {
		t.Fatalf("nil timeline must be omitted: %s", data)
	}
	s.ResetToday.Timeline = []ResetTodayEntry{}
	data, _ = s.Encode()
	if !strings.Contains(string(data), `"timeline":[]`) {
		t.Fatalf("empty timeline must be written: %s", data)
	}
}

func TestSnapshotPaths(t *testing.T) {
	home := filepath.Join("h", "u")
	if got := LocalSnapshotPath(home); got != filepath.Join(home, ".quosparr", "widget-snapshot.json") {
		t.Fatal(got)
	}
	got, err := appGroupSnapshotPath("darwin", home, "")
	if err != nil || got != filepath.Join(home, "Library", "Group Containers", "group.com.quosparr.app", "widget-snapshot.json") {
		t.Fatalf("%s %v", got, err)
	}
	if _, err := appGroupSnapshotPath("linux", home, ""); !errors.Is(err, ErrAppGroupUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if p, err := SnapshotPath(StorageLocal, "", home); err != nil || p != LocalSnapshotPath(home) {
		t.Fatalf("%s %v", p, err)
	}
	if StorageModeFromInfo("app-group") != StorageAppGroup || StorageModeFromInfo("") != StorageLocal || StorageModeFromInfo("x") != StorageLocal {
		t.Fatal("storage mode")
	}
}

// Reset-today timeline builder. The expectations are the timelines the Swift
// verdict code produced for the same scenarios (testdata/swift_widget_reset_cases.json).

type fakeSource struct {
	fetched     time.Time
	lastChecked *time.Time
	verdict     func(time.Time) Verdict
	next        func(time.Time) *NextReset
	transitions []time.Time
}

func (f fakeSource) Verdict(now time.Time) Verdict {
	if f.verdict == nil {
		return Verdict{Reason: ReasonNone}
	}
	return f.verdict(now)
}
func (f fakeSource) NextScheduled(now time.Time) *NextReset {
	if f.next == nil {
		return nil
	}
	return f.next(now)
}
func (f fakeSource) TransitionDates(now time.Time) []time.Time {
	var out []time.Time
	for _, d := range f.transitions {
		if d.After(now) {
			out = append(out, d)
		}
	}
	return out
}
func (f fakeSource) FetchedAt() time.Time              { return f.fetched }
func (f fakeSource) LastSuccessfulCheckAt() *time.Time { return f.lastChecked }

func TestResetTodayTimelineMatchesSwiftNone(t *testing.T) {
	now := utc(2026, 7, 28, 12, 0, 0)
	staleAt := utc(2026, 7, 29, 18, 0, 1) // fetched + 30h + 1s
	src := fakeSource{
		fetched: now, lastChecked: &now,
		verdict: func(at time.Time) Verdict {
			if !at.Before(staleAt) {
				return Verdict{Reason: ReasonUnavailable}
			}
			return Verdict{Reason: ReasonNone}
		},
		// A midnight, and the stale boundary: the midnight repeats the
		// presentation and is dropped.
		transitions: []time.Time{utc(2026, 7, 29, 0, 0, 0), staleAt},
	}
	got := BuildResetToday(src, now)
	cases := fixtureCases(t)
	assertCase(t, "none", got, cases)
}

func TestResetTodayTimelineMatchesSwiftCompletedWithFuture(t *testing.T) {
	now := utc(2026, 7, 28, 12, 0, 0)
	start := utc(2026, 7, 29, 13, 0, 0).Add(123 * time.Millisecond)
	staleAt := utc(2026, 7, 29, 18, 0, 1)
	global := alerts.ResetGlobal
	next := &NextReset{StartAt: start, PendingUntil: start, ResetType: global}
	src := fakeSource{
		fetched: now, lastChecked: &now,
		verdict: func(at time.Time) Verdict {
			switch {
			case !at.Before(staleAt):
				return Verdict{Reason: ReasonUnavailable}
			case !at.Before(utc(2026, 7, 29, 16, 0, 1)):
				return Verdict{Reason: ReasonExpiredUnconfirmed}
			case !at.Before(utc(2026, 7, 29, 13, 0, 1)):
				return Verdict{Reason: ReasonGrace, ShowsYes: true, ResetType: &global, IsScheduled: true, Percent: ptr(98), Band: ptr(BandOK)}
			case !at.Before(utc(2026, 7, 29, 0, 0, 0)):
				return Verdict{Reason: ReasonUpcoming, ShowsYes: true, ResetType: &global, IsScheduled: true, Percent: ptr(98), Band: ptr(BandOK)}
			}
			return Verdict{Reason: ReasonCompleted, ShowsYes: true, ResetType: &global}
		},
		next: func(at time.Time) *NextReset {
			// The schedule is open until it has started plus its wait.
			if !at.Before(utc(2026, 7, 29, 13, 0, 1)) {
				return nil
			}
			return next
		},
		transitions: []time.Time{
			utc(2026, 7, 29, 0, 0, 0), start, utc(2026, 7, 29, 16, 0, 1), staleAt,
		},
	}
	got := BuildResetToday(src, now)
	assertCase(t, "completedWithFuture", got, fixtureCases(t))
}

func fixtureCases(t *testing.T) map[string]string {
	t.Helper()
	// The cases file is a JSON object; re-encode each value to compare with
	// our own encoding.
	raw := fixture(t, "swift_widget_reset_cases.json")
	cases := map[string]string{}
	for _, name := range []string{"none", "completedWithFuture"} {
		cases[name] = jsonMember(t, raw, name)
	}
	return cases
}

func assertCase(t *testing.T, name string, got ResetTodaySnapshot, cases map[string]string) {
	t.Helper()
	data, err := swiftjson.Marshal(got, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != cases[name] {
		t.Fatalf("%s differs from Swift\n got: %s\nwant: %s", name, data, cases[name])
	}
}

func TestResetTodayTimelineAnchorsAtFetchTimeAndRoundsUp(t *testing.T) {
	fetched := utc(2026, 7, 28, 12, 0, 0)
	now := fetched.Add(10 * time.Minute)
	boundary := fetched.Add(20*time.Minute + 1*time.Nanosecond)
	src := fakeSource{
		fetched: fetched,
		verdict: func(at time.Time) Verdict {
			if at.Before(boundary) {
				return Verdict{Reason: ReasonNone}
			}
			return Verdict{Reason: ReasonExpiredUnconfirmed}
		},
		transitions: []time.Time{boundary},
	}
	got := BuildResetToday(src, now)
	if !got.Timeline[0].EffectiveAt.Equal(fetched) {
		t.Fatalf("first entry is stamped min(fetchedAt, now): %v", got.Timeline[0].EffectiveAt)
	}
	if len(got.Timeline) != 2 || !got.Timeline[1].EffectiveAt.Equal(fetched.Add(20*time.Minute+time.Second)) ||
		got.Timeline[1].Reason != ReasonExpiredUnconfirmed {
		t.Fatalf("timeline = %+v", got.Timeline)
	}
	// A now before the fetch time stamps the first entry with now.
	early := BuildResetToday(src, fetched.Add(-time.Hour))
	if !early.Timeline[0].EffectiveAt.Equal(fetched.Add(-time.Hour)) {
		t.Fatalf("early = %v", early.Timeline[0].EffectiveAt)
	}
}

func TestResetTodayEntryDetails(t *testing.T) {
	now := utc(2026, 7, 28, 12, 0, 0)
	contextual := alerts.BasisContextualInference
	explicit := alerts.BasisExplicit
	banked := alerts.ResetBanked
	nxt := &NextReset{StartAt: now.Add(time.Hour), PendingUntil: now.Add(4 * time.Hour), ResetType: banked, ScheduleBasis: &contextual}
	src := fakeSource{
		fetched: now,
		verdict: func(time.Time) Verdict {
			return Verdict{Reason: ReasonNone}
		},
		next: func(at time.Time) *NextReset { return nxt },
	}
	e := BuildResetToday(src, now).Timeline[0]
	if e.State != ResetNo || e.NextScheduledAt == nil || !e.NextScheduledAt.Equal(nxt.StartAt) ||
		*e.NextScheduledResetType != banked || *e.ScheduleBasis != contextual {
		t.Fatalf("not yet started: %+v", e)
	}
	// Once the schedule has started the timer waits for the end of the pending
	// period, and a scheduled verdict supplies its own basis.
	src.verdict = func(time.Time) Verdict {
		return Verdict{Reason: ReasonUpcoming, ShowsYes: true, IsScheduled: true, ScheduleBasis: &explicit}
	}
	late := BuildResetToday(src, nxt.StartAt.Add(time.Minute)).Timeline[0]
	if !late.NextScheduledAt.Equal(nxt.PendingUntil) || *late.ScheduleBasis != explicit || late.State != ResetYes {
		t.Fatalf("started: %+v", late)
	}
	// Unavailable never looks at the schedule.
	src.verdict = func(time.Time) Verdict { return Verdict{Reason: ReasonUnavailable, ShowsYes: true} }
	un := BuildResetToday(src, now).Timeline[0]
	if un.State != ResetUnknown || un.NextScheduledAt != nil || un.NextScheduledResetType != nil || un.ScheduleBasis != nil {
		t.Fatalf("unavailable: %+v", un)
	}
}

func TestCeilSecond(t *testing.T) {
	whole := utc(2026, 1, 1, 0, 0, 5)
	if !CeilSecond(whole).Equal(whole) || !CeilSecond(whole.Add(1)).Equal(whole.Add(time.Second)) ||
		!CeilSecond(whole.Add(999*time.Millisecond)).Equal(whole.Add(time.Second)) {
		t.Fatal("CeilSecond")
	}
}

// Contract.

func TestDeepLinksMatchSwift(t *testing.T) {
	// The frozen Swift fixture only lists the original three scopes.
	var lines []string
	for _, p := range []ProviderScope{ScopeCodex, ScopeGrok, ScopeBoth} {
		for _, s := range Sections() {
			lines = append(lines, DeepLink{p, s}.URL())
		}
	}
	equalBytes(t, "swift_deeplinks.txt", []byte(strings.Join(lines, "\n")+"\n"))
	for _, p := range ProviderScopes() {
		for _, s := range Sections() {
			link := DeepLink{p, s}
			if got, ok := ParseDeepLink(link.URL()); !ok || got != link {
				t.Fatalf("round trip of %s: %v %v", link.URL(), got, ok)
			}
		}
	}
}

func TestDeepLinkParsingMatchesSwift(t *testing.T) {
	for _, line := range strings.Split(strings.TrimSpace(string(fixture(t, "swift_deeplink_parse.txt"))), "\n") {
		raw, want, _ := strings.Cut(line, "\t")
		link, ok := ParseDeepLink(raw)
		got := "-"
		if ok {
			got = string(link.Provider) + "/" + string(link.Section)
		}
		if got != want {
			t.Errorf("ParseDeepLink(%q) = %s, Swift says %s", raw, got, want)
		}
	}
}

func TestLayoutPolicy(t *testing.T) {
	if RefreshInterval != 15*time.Minute {
		t.Fatal("refresh interval")
	}
	type tc struct {
		f Family
		k Kind
		w bool
	}
	for _, c := range []tc{
		{FamilySmall, KindOverview, true}, {FamilyLarge, KindOverview, true},
		{FamilySmall, KindTokenTrend, false}, {FamilyMedium, KindTokenTrend, true}, {FamilyLarge, KindTokenTrend, true},
		{FamilySmall, KindMetric, true}, {FamilyMedium, KindMetric, false},
		{FamilySmall, KindResetToday, true}, {FamilyMedium, KindResetToday, true}, {FamilyLarge, KindResetToday, false},
	} {
		if Supports(c.f, c.k) != c.w {
			t.Errorf("Supports(%s,%s) != %v", c.f, c.k, c.w)
		}
	}
	if TrendDays(FamilyLarge) != 30 || TrendDays(FamilySmall) != 14 || TrendDays(FamilyMedium) != 14 {
		t.Fatal("trend days")
	}
	if n, ok := QuotaLimit(FamilySmall); !ok || n != 1 {
		t.Fatal("small")
	}
	if n, ok := QuotaLimit(FamilyMedium); !ok || n != 2 {
		t.Fatal("medium")
	}
	if _, ok := QuotaLimit(FamilyLarge); ok {
		t.Fatal("large has no limit")
	}
	if !reflect.DeepEqual(ProviderScope("both").Providers(), []Provider{ProviderCodex, ProviderGrok}) ||
		!reflect.DeepEqual(ScopeClaude.Providers(), []Provider{ProviderClaude}) ||
		!reflect.DeepEqual(ScopeCodexClaude.Providers(), []Provider{ProviderCodex, ProviderClaude}) ||
		!reflect.DeepEqual(ScopeGrokClaude.Providers(), []Provider{ProviderGrok, ProviderClaude}) ||
		!reflect.DeepEqual(ScopeGemini.Providers(), []Provider{ProviderGemini}) ||
		!reflect.DeepEqual(ScopeCodexGemini.Providers(), []Provider{ProviderCodex, ProviderGemini}) ||
		!reflect.DeepEqual(ScopeClaudeGemini.Providers(), []Provider{ProviderClaude, ProviderGemini}) ||
		!reflect.DeepEqual(ScopeGeminiGrok.Providers(), []Provider{ProviderGemini, ProviderGrok}) ||
		ProviderScope("gemini-claude").Valid() || len(ProviderScopes()) != 10 {
		t.Fatal("providers")
	}
	if string(KindOverview) != "com.quosparr.app.widget.overview" || len(Kinds()) != 4 {
		t.Fatal("kinds")
	}
}

func TestRequirements(t *testing.T) {
	if RequirementsFor(KindOverview, FamilySmall) != RequireProviderQuota ||
		RequirementsFor(KindOverview, FamilyLarge) != RequireProviderQuota|RequireTokenTrend|RequireCost ||
		RequirementsFor(KindTokenTrend, FamilyMedium) != RequireTokenTrend ||
		RequirementsFor(KindMetric, FamilySmall) != RequireProviderQuota|RequireTokenTrend|RequireCost ||
		RequirementsFor(KindResetToday, FamilySmall) != RequireResetToday {
		t.Fatal("requirements")
	}
	if RequireProviderQuota != 1 || RequireTokenTrend != 2 || RequireCost != 4 || RequireResetToday != 8 || RequireAll != 15 {
		t.Fatal("bit values are part of the contract")
	}
	got := RequirementsForActive([]string{string(KindResetToday), string(KindTokenTrend), "unknown"})
	if got != RequireResetToday|RequireTokenTrend || !got.Has(RequireTokenTrend) || got.Has(RequireCost) {
		t.Fatalf("active = %d", got)
	}
	if RequirementsForActive(nil) != 0 {
		t.Fatal("nothing active needs nothing")
	}
}

// Reaction visitor.

func TestParseVisitorID(t *testing.T) {
	ok := map[string]string{
		"0ca75a14e041f05a5258f7924fa08914":     "0ca75a14e041f05a5258f7924fa08914",
		"0CA75A14E041F05A5258F7924FA08914":     "0ca75a14e041f05a5258f7924fa08914",
		"  abcdef0123456789abcdef0123456789  ": "abcdef0123456789abcdef0123456789",
		"\nabcdef0123456789abcdef0123456789\t": "abcdef0123456789abcdef0123456789",
	}
	for in, want := range ok {
		if got, valid := ParseVisitorID(in); !valid || got != want {
			t.Errorf("ParseVisitorID(%q) = %q %v", in, got, valid)
		}
	}
	for _, in := range []string{"", "not-a-visitor-id", "0ca75a14e041f05a5258f7924fa0891", "0ca75a14e041f05a5258f7924fa0891400", "0ca75a14e041f05a5258f7924fa0891g", "ａ"} {
		if _, valid := ParseVisitorID(in); valid {
			t.Errorf("ParseVisitorID(%q) must fail", in)
		}
	}
}

func TestVisitorIDFromSetCookie(t *testing.T) {
	const id = "0ca75a14e041f05a5258f7924fa08914"
	if got, ok := VisitorIDFromSetCookie("hr_react=" + id + "; HttpOnly; Max-Age=31536000; Path=/; SameSite=lax; Secure"); !ok || got != id {
		t.Fatalf("%q %v", got, ok)
	}
	for _, h := range []string{"", "other=" + id, "hr_react", "hr_react=short", "x=1; hr_react=" + id} {
		if _, ok := VisitorIDFromSetCookie(h); ok {
			t.Errorf("%q must not parse", h)
		}
	}
}

func TestReactionStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", ReactionFileName)
	store := ReactionStore{Path: path}
	if _, ok := store.LoadVisitorID(); ok {
		t.Fatal("empty store")
	}
	if err := store.SaveVisitorID("not-valid"); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.LoadVisitorID(); ok {
		t.Fatal("invalid ID stored")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("an invalid ID must not create the file")
	}
	const visitor = "abcdef0123456789abcdef0123456789"
	if err := store.SaveVisitorID(visitor); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.LoadVisitorID(); !ok || got != visitor {
		t.Fatalf("%q %v", got, ok)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v", info.Mode().Perm())
		}
	}
	if err := store.SaveVisitorID("FFFF0000aaaa1111bbbb2222cccc3333"); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.LoadVisitorID(); got != "ffff0000aaaa1111bbbb2222cccc3333" {
		t.Fatalf("got %q", got)
	}
}

func TestReactionStoreKeepsFileWhenIDUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), ReactionFileName)
	clock := utc(2026, 8, 23, 12, 0, 0)
	store := ReactionStore{Path: path, Now: func() time.Time { return clock }}
	const visitor = "0123456789abcdef0123456789abcdef"
	if err := store.SaveVisitorID(visitor); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	clock = clock.Add(time.Hour)
	if err := store.SaveVisitorID("  " + strings.ToUpper(visitor) + "\n"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Fatalf("unchanged ID rewrote the file: %s vs %s", first, second)
	}
}

func TestReactionFileMatchesSwift(t *testing.T) {
	path := filepath.Join(t.TempDir(), ReactionFileName)
	store := ReactionStore{Path: path, Now: func() time.Time { return utc(2026, 8, 23, 12, 0, 0) }}
	if err := store.SaveVisitorID("  0123456789ABCDEF0123456789abcdef \n"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	equalBytes(t, "swift_reaction_visitor.json", got)
	if want := strings.TrimSpace(string(fixture(t, "swift_reaction_mode.txt"))); want != "600" {
		t.Fatalf("Swift mode %s", want)
	}
	// The Swift file loads.
	swiftStore := ReactionStore{Path: filepath.Join("testdata", "swift_reaction_visitor.json")}
	if id, ok := swiftStore.LoadVisitorID(); !ok || id != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("%q %v", id, ok)
	}
}

func TestReactionStoreRejectsDamagedFiles(t *testing.T) {
	for name, content := range map[string]string{
		"garbage":    "nope",
		"no date":    `{"visitorId":"0123456789abcdef0123456789abcdef"}`,
		"bad date":   `{"visitorId":"0123456789abcdef0123456789abcdef","updatedAt":"yesterday"}`,
		"no id":      `{"updatedAt":"2026-08-23T12:00:00Z"}`,
		"invalid id": `{"visitorId":"xyz","updatedAt":"2026-08-23T12:00:00Z"}`,
	} {
		path := filepath.Join(t.TempDir(), ReactionFileName)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if id, ok := (ReactionStore{Path: path}).LoadVisitorID(); ok {
			t.Errorf("%s: loaded %q", name, id)
		}
	}
	if got := DefaultReactionPath("/h"); filepath.Base(got) != "reaction-visitor.json" {
		t.Fatal(got)
	}
}

// jsonMember returns the raw text of a top-level member of a compact JSON
// object, scanning brackets and strings.
func jsonMember(t testing.TB, raw []byte, name string) string {
	t.Helper()
	key := []byte(`"` + name + `":`)
	i := bytes.Index(raw, key)
	if i < 0 {
		t.Fatalf("member %s not found", name)
	}
	start := i + len(key)
	depth, inString := 0, false
	for j := start; j < len(raw); j++ {
		c := raw[j]
		switch {
		case inString:
			if c == '\\' {
				j++
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				return string(raw[start : j+1])
			}
		}
	}
	t.Fatalf("member %s is unterminated", name)
	return ""
}
