package runway

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

type durationUnit int

const (
	unitDay durationUnit = iota
	unitHour
	unitMinute
	unitSecond
)

// DurationLocalized formats seconds as a compact localized duration
// (Swift DurationFormatter.localized). Seconds are rounded up.
func DurationLocalized(seconds float64, lang l10n.Language, includeSeconds bool) string {
	value := int(math.Ceil(seconds))
	if math.IsNaN(seconds) || value < 0 {
		value = 0
	}
	return durationParts(value, lang, includeSeconds)
}

func durationParts(value int, lang l10n.Language, includeSeconds bool) string {
	days := value / 86_400
	hours := value % 86_400 / 3_600
	minutes := value % 3_600 / 60
	secs := value % 60
	var parts []string
	if days > 0 {
		parts = append(parts, unitText(lang, days, unitDay))
		if hours > 0 {
			parts = append(parts, unitText(lang, hours, unitHour))
		}
		return joinUnits(lang, parts)
	}
	if hours > 0 {
		parts = append(parts, unitText(lang, hours, unitHour))
	}
	if minutes > 0 || hours > 0 {
		parts = append(parts, unitText(lang, minutes, unitMinute))
	}
	if includeSeconds || len(parts) == 0 {
		parts = append(parts, unitText(lang, secs, unitSecond))
	}
	return joinUnits(lang, parts)
}

// ShortDuration is DurationLocalized with short units, for the menu bar:
// the two largest units, a zero second one left out. Seconds are rounded up.
func ShortDuration(seconds float64, lang l10n.Language) string {
	value := int(math.Ceil(seconds))
	if math.IsNaN(seconds) || value < 0 {
		value = 0
	}
	amounts := [...]int{value / 86_400, value % 86_400 / 3_600, value % 3_600 / 60, value % 60}
	units := byLang(lang,
		[4]string{"d", "h", "m", "s"},
		[4]string{"天", "小时", "分", "秒"},
		[4]string{"天", "小時", "分", "秒"},
		[4]string{"일", "시간", "분", "초"},
		[4]string{"日", "時間", "分", "秒"},
		[4]string{"д", "ч", "мин", "с"},
		[4]string{"j", "h", "min", "s"})
	first := durationUnit(0)
	for first < unitSecond && amounts[first] == 0 {
		first++
	}
	parts := []string{strconv.Itoa(amounts[first]) + units[first]}
	if first < unitMinute && amounts[first+1] > 0 {
		parts = append(parts, strconv.Itoa(amounts[first+1])+units[first+1])
	}
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese:
		return strings.Join(parts, "")
	}
	return strings.Join(parts, " ")
}

func joinUnits(lang l10n.Language, parts []string) string {
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese, l10n.Korean:
		return strings.Join(parts, "")
	}
	return strings.Join(parts, " ")
}

func unitText(lang l10n.Language, v int, u durationUnit) string {
	n := strconv.Itoa(v)
	pick := func(day, hour, minute, second string) string {
		return n + [...]string{day, hour, minute, second}[u]
	}
	switch lang {
	case l10n.SimplifiedChinese:
		return pick("天", "小时", "分钟", "秒")
	case l10n.TraditionalChinese:
		return pick("天", "小時", "分鐘", "秒")
	case l10n.Japanese:
		return pick("日", "時間", "分", "秒")
	case l10n.Korean:
		return pick("일", "시간", "분", "초")
	case l10n.Russian:
		return russianUnit(v, u)
	case l10n.French:
		w := [...][2]string{{"jour", "jours"}, {"heure", "heures"}, {"minute", "minutes"}, {"seconde", "secondes"}}[u]
		if v <= 1 {
			return n + " " + w[0]
		}
		return n + " " + w[1]
	}
	w := [...][2]string{{"day", "days"}, {"hour", "hours"}, {"minute", "minutes"}, {"second", "seconds"}}[u]
	if v == 1 {
		return n + " " + w[0]
	}
	return n + " " + w[1]
}

func russianUnit(v int, u durationUnit) string {
	forms := [...][3]string{
		{"день", "дня", "дней"},
		{"час", "часа", "часов"},
		{"минута", "минуты", "минут"},
		{"секунда", "секунды", "секунд"},
	}[u]
	a := v
	if a < 0 {
		a = -a
	}
	m10, m100 := a%10, a%100
	w := forms[2]
	switch {
	case m10 == 1 && m100 != 11:
		w = forms[0]
	case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
		w = forms[1]
	}
	return strconv.Itoa(v) + " " + w
}

func byLang[T any](lang l10n.Language, en, zhHans, zhHant, ko, ja, ru, fr T) T {
	switch lang {
	case l10n.SimplifiedChinese:
		return zhHans
	case l10n.TraditionalChinese:
		return zhHant
	case l10n.Korean:
		return ko
	case l10n.Japanese:
		return ja
	case l10n.Russian:
		return ru
	case l10n.French:
		return fr
	}
	return en
}

// JustNowText is the "just now" copy (Swift DurationCopy.justNow).
func JustNowText(lang l10n.Language) string {
	return byLang(lang, "just now", "刚刚", "剛剛", "방금", "たった今", "только что", "à l'instant")
}

// UnderOneMinuteText is the "under 1 minute" copy.
func UnderOneMinuteText(lang l10n.Language) string {
	return byLang(lang, "under 1 minute", "不到1分钟", "不到1分鐘", "1분 미만", "1分未満", "меньше 1 минуты", "moins d'1 minute")
}

// NowText is the "now" copy.
func NowText(lang l10n.Language) string {
	return byLang(lang, "now", "现在", "現在", "지금", "現在", "сейчас", "maintenant")
}

func agoText(text string, lang l10n.Language) string {
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese:
		return text + "之前"
	case l10n.Korean:
		return text + " 전"
	case l10n.Japanese:
		return text + "前"
	case l10n.Russian:
		return text + " назад"
	case l10n.French:
		return "il y a " + text
	}
	return text + " ago"
}

// Money formats a cost as "$0.0000" (Swift DurationFormatter.money).
func Money(v decimal.Decimal) string {
	f, _ := strconv.ParseFloat(v.String(), 64)
	return "$" + fmt.Sprintf("%.4f", f)
}

// RelativePast formats how long ago date was. Without seconds it collapses
// the first minute to "just now" so timer-driven callers change once a minute.
func RelativePast(date, now time.Time, lang l10n.Language, includeSeconds bool) string {
	interval := now.Sub(date).Seconds()
	if !includeSeconds && interval < 60 {
		return JustNowText(lang)
	}
	return agoText(DurationLocalized(interval, lang, includeSeconds), lang)
}

// RelativePastSingleUnit is the single-unit past duration of the hosted
// reset-today copy; future or invalid intervals return ok=false.
func RelativePastSingleUnit(date, now time.Time, lang l10n.Language) (string, bool) {
	interval := now.Sub(date).Seconds()
	if math.IsNaN(interval) || math.IsInf(interval, 0) || interval < 0 {
		return "", false
	}
	secs := int(interval)
	if secs < 60 {
		return unitText(lang, max(secs, 1), unitSecond), true
	}
	if mins := secs / 60; mins < 60 {
		return unitText(lang, mins, unitMinute), true
	}
	hours := secs / 3600
	if hours < 24 {
		return unitText(lang, hours, unitHour), true
	}
	return unitText(lang, hours/24, unitDay), true
}

// RemainingUntil is the compact remaining duration until date.
func RemainingUntil(date, now time.Time, lang l10n.Language, includeSeconds bool) string {
	return spanText(math.Max(0, date.Sub(now).Seconds()), lang, includeSeconds)
}

// ElapsedSince is the compact elapsed duration since date.
func ElapsedSince(date, now time.Time, lang l10n.Language, includeSeconds bool) string {
	return spanText(math.Max(0, now.Sub(date).Seconds()), lang, includeSeconds)
}

func spanText(seconds float64, lang l10n.Language, includeSeconds bool) string {
	if !includeSeconds && seconds < 60 {
		return UnderOneMinuteText(lang)
	}
	return DurationLocalized(seconds, lang, includeSeconds)
}

// CompactNumber is the Swift RunwayModel.compactNumber form: "1.50M",
// "12.00K" or the plain integer below 1000.
func CompactNumber(value int64) string {
	switch {
	case value >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(value)/1_000_000)
	case value >= 1_000:
		return fmt.Sprintf("%.2fK", float64(value)/1_000)
	}
	return strconv.FormatInt(value, 10)
}

// ResetShortLabel is "HH:mm" when date falls on now's day in loc, else "M/d".
func ResetShortLabel(date, now time.Time, loc *time.Location) string {
	loc = locOrLocal(loc)
	d, n := date.In(loc), now.In(loc)
	if sameDay(d, n) {
		return d.Format("15:04")
	}
	return fmt.Sprintf("%d/%d", int(d.Month()), d.Day())
}

// ResetExactTime is the local time of a quota reset: "HH:mm" on now's day,
// else the short month and day before it ("Oct 20 14:30", "10月20日 14:30"),
// with the year when it is not now's.
func ResetExactTime(date, now time.Time, lang l10n.Language, loc *time.Location) string {
	loc = locOrLocal(loc)
	d, n := date.In(loc), now.In(loc)
	clock := d.Format("15:04")
	switch {
	case sameDay(d, n):
		return clock
	case d.Year() != n.Year():
		return monthDayYear(d, lang) + " " + clock
	}
	var day string
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese:
		day = fmt.Sprintf("%d月%d日", int(d.Month()), d.Day())
	case l10n.Korean:
		day = fmt.Sprintf("%d월 %d일", int(d.Month()), d.Day())
	case l10n.Russian, l10n.French:
		day = fmt.Sprintf("%d %s", d.Day(), MonthTitle(int(d.Month()), lang))
	default:
		day = fmt.Sprintf("%s %d", MonthTitle(int(d.Month()), lang), d.Day())
	}
	return day + " " + clock
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func locOrLocal(loc *time.Location) *time.Location {
	if loc == nil {
		return time.Local
	}
	return loc
}

// ScheduleWindow is the window of a scheduled rate-limit reset. It mirrors
// the Swift RateLimitResetScheduleWindow without importing the reset-today
// package.
type ScheduleWindow struct {
	StartAt time.Time
	EndAt   time.Time
}

// IsRange reports whether the window spans more than one instant.
func (w ScheduleWindow) IsRange() bool { return w.EndAt.After(w.StartAt) }

// ScheduledLabel is "yyyy/M/d HH:mm" or "start~end" for a range.
func ScheduledLabel(w ScheduleWindow, loc *time.Location) string {
	loc = locOrLocal(loc)
	full := func(t time.Time) string {
		t = t.In(loc)
		return fmt.Sprintf("%d/%d/%d %s", t.Year(), int(t.Month()), t.Day(), t.Format("15:04"))
	}
	if !w.IsRange() {
		return full(w.StartAt)
	}
	return full(w.StartAt) + "~" + full(w.EndAt)
}

// ScheduledCountdown is the live countdown to the window (start~end).
func ScheduledCountdown(w ScheduleWindow, now time.Time, lang l10n.Language) string {
	end := countdownDuration(w.EndAt, now, lang)
	if !w.IsRange() {
		return end
	}
	start := NowText(lang)
	if w.StartAt.After(now) {
		start = countdownDuration(w.StartAt, now, lang)
	}
	return start + "~" + end
}

func countdownDuration(until, now time.Time, lang l10n.Language) string {
	total := max(0, int(math.Floor(until.Sub(now).Seconds())))
	hours, minutes, secs := total/3600, total%3600/60, total%60
	var parts []string
	if hours > 0 {
		parts = append(parts, unitText(lang, hours, unitHour))
	}
	if minutes > 0 || hours > 0 {
		parts = append(parts, unitText(lang, minutes, unitMinute))
	}
	parts = append(parts, unitText(lang, secs, unitSecond))
	return joinUnits(lang, parts)
}

var monthShort = map[l10n.Language][12]string{
	l10n.English: {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	l10n.Russian: {"янв.", "февр.", "март", "апр.", "май", "июнь", "июль", "авг.", "сент.", "окт.", "нояб.", "дек."},
	l10n.French:  {"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."},
}

// MonthTitle is the short month name (Swift TokenUsageDateFormatting.monthTitle).
func MonthTitle(month int, lang l10n.Language) string {
	month = min(12, max(1, month))
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese:
		return fmt.Sprintf("%d月", month)
	case l10n.Korean:
		return fmt.Sprintf("%d월", month)
	}
	names, ok := monthShort[lang]
	if !ok {
		names = monthShort[l10n.English]
	}
	return names[month-1]
}

func monthDayYear(t time.Time, lang l10n.Language) string {
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese:
		return fmt.Sprintf("%d年%d月%d日", t.Year(), int(t.Month()), t.Day())
	case l10n.Korean:
		return fmt.Sprintf("%d. %d. %d.", t.Year(), int(t.Month()), t.Day())
	case l10n.Russian, l10n.French:
		return fmt.Sprintf("%d %s %d", t.Day(), MonthTitle(int(t.Month()), lang), t.Year())
	}
	return fmt.Sprintf("%s %d, %d", MonthTitle(int(t.Month()), lang), t.Day(), t.Year())
}

// ResetCreditUpdatedAt is a medium date plus short time ("Oct 12, 2026 14:30").
func ResetCreditUpdatedAt(t time.Time, lang l10n.Language, loc *time.Location) string {
	t = t.In(locOrLocal(loc))
	return monthDayYear(t, lang) + " " + t.Format("15:04")
}

// ResetCreditExpiresAt is a short date plus short time ("10/12/26 14:30").
func ResetCreditExpiresAt(t time.Time, lang l10n.Language, loc *time.Location) string {
	t = t.In(locOrLocal(loc))
	var date string
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese:
		date = fmt.Sprintf("%d/%d/%d", t.Year(), int(t.Month()), t.Day())
	case l10n.Korean:
		date = fmt.Sprintf("%02d. %d. %d.", t.Year()%100, int(t.Month()), t.Day())
	case l10n.Russian:
		date = fmt.Sprintf("%02d.%02d.%d", t.Day(), int(t.Month()), t.Year())
	case l10n.French:
		date = fmt.Sprintf("%02d/%02d/%d", t.Day(), int(t.Month()), t.Year())
	default:
		date = fmt.Sprintf("%d/%d/%02d", int(t.Month()), t.Day(), t.Year()%100)
	}
	return date + " " + t.Format("15:04")
}

// SubscriptionExpiresOn is the stable calendar date of the account header.
func SubscriptionExpiresOn(t time.Time, lang l10n.Language, loc *time.Location) string {
	t = t.In(locOrLocal(loc))
	switch lang {
	case l10n.SimplifiedChinese, l10n.TraditionalChinese, l10n.Japanese, l10n.Korean:
		return fmt.Sprintf("%d/%d/%d", t.Year(), int(t.Month()), t.Day())
	case l10n.Russian, l10n.French:
		return fmt.Sprintf("%d %s %d", t.Day(), MonthTitle(int(t.Month()), lang), t.Year())
	}
	return fmt.Sprintf("%s %d, %d", MonthTitle(int(t.Month()), l10n.English), t.Day(), t.Year())
}

// SubscriptionExpiresAt is the full instant with its UTC offset and zone name.
func SubscriptionExpiresAt(t time.Time, lang l10n.Language, loc *time.Location) string {
	loc = locOrLocal(loc)
	local := t.In(loc)
	return fmt.Sprintf("%s %s (UTC%s, %s)",
		SubscriptionExpiresOn(t, lang, loc), local.Format("15:04:05"), local.Format("-07:00"), loc.String())
}

// SubscriptionIsExpired compares absolute instants.
func SubscriptionIsExpired(expiresAt, now time.Time) bool { return !now.Before(expiresAt) }

// EndOfLocalDay is the last second of date's local day.
func EndOfLocalDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(locOrLocal(loc)).Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, locOrLocal(loc)).Add(-time.Second)
}
