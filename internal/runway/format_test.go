package runway

import (
	"strings"
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

func TestDurationLocalizedUsesReadableUnits(t *testing.T) {
	const secs = 3661
	cases := []struct {
		lang l10n.Language
		want string
	}{
		{l10n.SimplifiedChinese, "1小时1分钟1秒"},
		{l10n.English, "1 hour 1 minute 1 second"},
		{l10n.TraditionalChinese, "1小時1分鐘1秒"},
		{l10n.Japanese, "1時間1分1秒"},
		{l10n.French, "1 heure 1 minute 1 seconde"},
		{l10n.Russian, "1 час 1 минута 1 секунда"},
	}
	for _, c := range cases {
		if got := DurationLocalized(secs, c.lang, true); got != c.want {
			t.Errorf("%s: got %q want %q", c.lang, got, c.want)
		}
	}
	if got := DurationLocalized(secs, l10n.SimplifiedChinese, false); got != "1小时1分钟" {
		t.Errorf("no seconds: %q", got)
	}
	if got := DurationLocalized(305*3600+54*60, l10n.SimplifiedChinese, true); got != "12天17小时" {
		t.Errorf("days zh: %q", got)
	}
	if got := DurationLocalized(305*3600+54*60, l10n.English, true); got != "12 days 17 hours" {
		t.Errorf("days en: %q", got)
	}
	for _, lang := range []l10n.Language{l10n.Russian, l10n.French} {
		if strings.Contains(DurationLocalized(secs, lang, true), "hour") {
			t.Errorf("%s leaks English", lang)
		}
	}
	if DurationLocalized(secs, l10n.Japanese, true) == DurationLocalized(secs, l10n.English, true) {
		t.Error("japanese equals english")
	}
	if got := DurationLocalized(-5, l10n.English, true); got != "0 seconds" {
		t.Errorf("negative: %q", got)
	}
}

func TestRelativePast(t *testing.T) {
	base, now := time.Unix(1000, 0), time.Unix(1030, 0)
	if got := RelativePast(base, now, l10n.SimplifiedChinese, true); got != "30秒之前" {
		t.Errorf("zh: %q", got)
	}
	if got := RelativePast(base, now, l10n.English, true); got != "30 seconds ago" {
		t.Errorf("en: %q", got)
	}
	if got := RelativePast(base, now, l10n.English, false); got != "just now" {
		t.Errorf("collapsed: %q", got)
	}
}

func TestRelativePastSingleUnit(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cases := []struct {
		ago  time.Duration
		want string
	}{
		{30 * time.Second, "30 seconds"},
		{0, "1 second"},
		{5 * time.Minute, "5 minutes"},
		{3*time.Hour + 59*time.Minute, "3 hours"},
		{50 * time.Hour, "2 days"},
	}
	for _, c := range cases {
		got, ok := RelativePastSingleUnit(now.Add(-c.ago), now, l10n.English)
		if !ok || got != c.want {
			t.Errorf("%v: got %q,%v want %q", c.ago, got, ok, c.want)
		}
	}
	if _, ok := RelativePastSingleUnit(now.Add(time.Second), now, l10n.English); ok {
		t.Error("future should be rejected")
	}
}

func TestRemainingAndElapsedSpans(t *testing.T) {
	now := time.Unix(10_000, 0)
	if got := RemainingUntil(now.Add(20*time.Second), now, l10n.English, false); got != "under 1 minute" {
		t.Errorf("under a minute: %q", got)
	}
	if got := RemainingUntil(now.Add(90*time.Minute), now, l10n.English, false); got != "1 hour 30 minutes" {
		t.Errorf("remaining: %q", got)
	}
	if got := ElapsedSince(now.Add(time.Hour), now, l10n.English, true); got != "0 seconds" {
		t.Errorf("negative elapsed clamps: %q", got)
	}
}

func TestCompactNumberAndMoney(t *testing.T) {
	for in, want := range map[int64]string{999: "999", 1000: "1.00K", 12_000: "12.00K", 1_500_000: "1.50M", 0: "0"} {
		if got := CompactNumber(in); got != want {
			t.Errorf("%d: %q want %q", in, got, want)
		}
	}
	d, _ := decimal.Parse("1.23456")
	if got := Money(d); got != "$1.2346" {
		t.Errorf("money: %q", got)
	}
}

func TestResetShortLabel(t *testing.T) {
	loc := time.FixedZone("x", 8*3600)
	now := time.Date(2026, 9, 7, 10, 0, 0, 0, loc)
	if got := ResetShortLabel(time.Date(2026, 9, 7, 21, 5, 0, 0, loc), now, loc); got != "21:05" {
		t.Errorf("same day: %q", got)
	}
	if got := ResetShortLabel(time.Date(2026, 9, 9, 1, 0, 0, 0, loc), now, loc); got != "9/9" {
		t.Errorf("other day: %q", got)
	}
}

func TestResetExactTime(t *testing.T) {
	loc := time.FixedZone("x", 8*3600)
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, loc)
	at := time.Date(2026, 10, 20, 14, 30, 0, 0, loc)
	for lang, want := range map[l10n.Language]string{
		l10n.English:            "Oct 20 14:30",
		l10n.SimplifiedChinese:  "10月20日 14:30",
		l10n.TraditionalChinese: "10月20日 14:30",
		l10n.Japanese:           "10月20日 14:30",
		l10n.Korean:             "10월 20일 14:30",
		l10n.Russian:            "20 окт. 14:30",
		l10n.French:             "20 oct. 14:30",
	} {
		if got := ResetExactTime(at, now, lang, loc); got != want {
			t.Errorf("%v: %q, want %q", lang, got, want)
		}
	}
	if got := ResetExactTime(time.Date(2026, 10, 14, 21, 5, 0, 0, loc), now, l10n.English, loc); got != "21:05" {
		t.Errorf("same day: %q", got)
	}
	if got := ResetExactTime(time.Date(2027, 1, 2, 3, 4, 0, 0, loc), now, l10n.SimplifiedChinese, loc); got != "2027年1月2日 03:04" {
		t.Errorf("next year: %q", got)
	}
}

func TestScheduledLabelAndCountdown(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 9, 7, 1, 5, 0, 0, loc)
	end := start.Add(2 * time.Hour)
	if got := ScheduledLabel(ScheduleWindow{StartAt: start, EndAt: start}, loc); got != "2026/9/7 01:05" {
		t.Errorf("single: %q", got)
	}
	if got := ScheduledLabel(ScheduleWindow{StartAt: start, EndAt: end}, loc); got != "2026/9/7 01:05~2026/9/7 03:05" {
		t.Errorf("range: %q", got)
	}
	now := start.Add(-time.Hour)
	if got := ScheduledCountdown(ScheduleWindow{StartAt: start, EndAt: end}, now, l10n.English); got != "1 hour 0 minutes 0 seconds~3 hours 0 minutes 0 seconds" {
		t.Errorf("countdown: %q", got)
	}
	if got := ScheduledCountdown(ScheduleWindow{StartAt: start, EndAt: end}, start.Add(time.Minute), l10n.English); !strings.HasPrefix(got, "now~") {
		t.Errorf("started: %q", got)
	}
}

func TestMonthTitle(t *testing.T) {
	if MonthTitle(3, l10n.English) != "Mar" || MonthTitle(3, l10n.SimplifiedChinese) != "3月" ||
		MonthTitle(3, l10n.Korean) != "3월" || MonthTitle(99, l10n.French) != "déc." {
		t.Error("month titles")
	}
}

func TestSubscriptionLocalDateTime(t *testing.T) {
	cases := []struct{ zone, ts, date, clock, offset string }{
		{"UTC", "2026-09-07T20:30:00Z", "2026/9/7", "20:30:00", "+00:00"},
		{"Asia/Singapore", "2026-09-07T20:30:00Z", "2026/9/8", "04:30:00", "+08:00"},
		{"Asia/Kolkata", "2026-09-07T20:30:00Z", "2026/9/8", "02:00:00", "+05:30"},
		{"America/Los_Angeles", "2026-09-07T20:30:00Z", "2026/9/7", "13:30:00", "-07:00"},
		{"America/Los_Angeles", "2026-03-08T09:59:59Z", "2026/3/8", "01:59:59", "-08:00"},
		{"America/Los_Angeles", "2026-03-08T10:00:00Z", "2026/3/8", "03:00:00", "-07:00"},
		{"America/Los_Angeles", "2026-11-01T08:30:00Z", "2026/11/1", "01:30:00", "-07:00"},
		{"America/Los_Angeles", "2026-11-01T09:30:00Z", "2026/11/1", "01:30:00", "-08:00"},
	}
	for _, c := range cases {
		loc, err := time.LoadLocation(c.zone)
		if err != nil {
			t.Skipf("tzdata unavailable: %v", err)
		}
		expiry, _ := time.Parse(time.RFC3339, c.ts)
		if got := SubscriptionExpiresOn(expiry, l10n.SimplifiedChinese, loc); got != c.date {
			t.Errorf("%s %s on: %q", c.zone, c.ts, got)
		}
		want := c.date + " " + c.clock + " (UTC" + c.offset + ", " + c.zone + ")"
		if got := SubscriptionExpiresAt(expiry, l10n.SimplifiedChinese, loc); got != want {
			t.Errorf("%s %s at: %q want %q", c.zone, c.ts, got, want)
		}
	}
}

func TestSubscriptionLanguageKeepsTimeZone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Skip(err)
	}
	expiry := time.Date(2026, 9, 7, 20, 30, 0, 0, time.UTC)
	for _, lang := range l10n.Languages() {
		got := SubscriptionExpiresAt(expiry, lang, loc)
		if !strings.HasSuffix(got, "04:30:00 (UTC+08:00, Asia/Singapore)") {
			t.Errorf("%s: %q", lang, got)
		}
	}
	if got := SubscriptionExpiresAt(expiry, l10n.English, loc); !strings.HasPrefix(got, "Sep 8, 2026 ") {
		t.Errorf("en: %q", got)
	}
	if got := SubscriptionExpiresAt(expiry, l10n.SimplifiedChinese, loc); !strings.HasPrefix(got, "2026/9/8 ") {
		t.Errorf("zh: %q", got)
	}
}

func TestSubscriptionExpiresAtItsInstant(t *testing.T) {
	expiry := time.Date(2026, 9, 7, 20, 30, 0, 0, time.UTC)
	if SubscriptionIsExpired(expiry, expiry.Add(-time.Second)) {
		t.Error("expired too early")
	}
	for _, d := range []time.Duration{0, time.Second, 4 * time.Hour} {
		if !SubscriptionIsExpired(expiry, expiry.Add(d)) {
			t.Errorf("not expired at +%v", d)
		}
	}
}

func TestResetCreditDateFormats(t *testing.T) {
	loc := time.FixedZone("x", 8*3600)
	at := time.Date(2026, 7, 12, 9, 50, 0, 0, loc)
	if got := ResetCreditExpiresAt(at, l10n.SimplifiedChinese, loc); got != "2026/7/12 09:50" {
		t.Errorf("zh: %q", got)
	}
	if got := ResetCreditExpiresAt(at, l10n.English, loc); strings.Contains(got, ",") {
		t.Errorf("en has comma: %q", got)
	}
}

func TestEndOfLocalDay(t *testing.T) {
	loc := time.FixedZone("x", 3600)
	got := EndOfLocalDay(time.Date(2026, 9, 7, 5, 0, 0, 0, loc), loc)
	if !got.Equal(time.Date(2026, 9, 7, 23, 59, 59, 0, loc)) {
		t.Errorf("end of day: %v", got)
	}
}

func TestShortDurationKeepsTwoUnits(t *testing.T) {
	cases := []struct {
		secs float64
		lang l10n.Language
		want string
	}{
		{3*3600 + 5*60, l10n.English, "3h 5m"},
		{3 * 3600, l10n.English, "3h"},
		{2*86400 + 4*3600 + 59*60, l10n.English, "2d 4h"},
		{45*60 + 10, l10n.English, "45m"},
		{30, l10n.English, "30s"},
		{-5, l10n.English, "0s"},
		{100 * 3600, l10n.SimplifiedChinese, "4天4小时"},
		{72 * 60, l10n.TraditionalChinese, "1小時12分"},
		{72 * 60, l10n.Japanese, "1時間12分"},
		{72 * 60, l10n.Korean, "1시간 12분"},
		{72 * 60, l10n.Russian, "1ч 12мин"},
		{30 * 3600, l10n.French, "1j 6h"},
	}
	for _, c := range cases {
		if got := ShortDuration(c.secs, c.lang); got != c.want {
			t.Errorf("%v %s = %q, want %q", c.secs, c.lang, got, c.want)
		}
	}
}
