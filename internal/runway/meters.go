package runway

import (
	"math"
	"strconv"
	"time"

	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
)

// Health is the colour class of a meter.
type Health int

// Health values.
const (
	HealthGreen Health = iota
	HealthYellow
	HealthRed
)

// MeterSource tells a standard quota window from a model-specific one.
type MeterSource int

// MeterSource values.
const (
	SourceStandard MeterSource = iota
	SourceModelSpecific
)

// Window lengths the app gives a name to, in minutes.
const (
	FiveHourMinutes = 300
	WeeklyMinutes   = 10_080
	MonthlyMinutes  = 30 * 24 * 60
)

// MeterWindow is the part of a rate window a meter needs. Both providers
// build meters from it. A zero ResetsAt or WindowMinutes means unknown.
type MeterWindow struct {
	UsedPercent   int
	WindowMinutes int
	ResetsAt      time.Time
}

// Meter is one quota bar (Swift QuotaMeter).
type Meter struct {
	Title            string
	WindowMinutes    int
	Source           MeterSource
	RemainingPercent int
	UsedPercent      int
	// ResetsAt is zero when the window has no known reset.
	ResetsAt time.Time
	// ResetText is the English remaining time without seconds, "" without reset.
	ResetText      string
	Health         Health
	MarkerPercents []int
	Projection     *BurnProjection
	// ValueText replaces the percentage in captions ("$12.34"); set on the
	// balance meter of an API relay account.
	ValueText string
	// PreserveTitle keeps a provider-defined pool name in compact displays.
	PreserveTitle bool
	// Balance marks that meter: it has no window and keeps its own title.
	Balance bool
}

// HasReset reports whether the meter knows its reset time.
func (m Meter) HasReset() bool { return !m.ResetsAt.IsZero() }

// DefaultMarkers are the threshold ticks of a Codex quota bar.
func DefaultMarkers() []int { return []int{20, 50, 80} }

// NewMeter builds a meter from a window as seen at now.
func NewMeter(title string, w MeterWindow, now time.Time, markers []int, source MeterSource) Meter {
	used := min(100, max(0, w.UsedPercent))
	m := Meter{
		Title:            title,
		WindowMinutes:    w.WindowMinutes,
		Source:           source,
		UsedPercent:      used,
		RemainingPercent: max(0, 100-used),
		ResetsAt:         w.ResetsAt,
		Health:           HealthForUsedPercent(used),
		MarkerPercents:   append([]int(nil), markers...),
		Projection:       MakeBurnProjection(w, now),
	}
	if !w.ResetsAt.IsZero() {
		m.ResetText = DurationLocalized(w.ResetsAt.Sub(now).Seconds(), l10n.English, false)
	}
	return m
}

// HealthForUsedPercent maps remaining quota to a colour: >=50 green, >=20 yellow.
func HealthForUsedPercent(used int) Health {
	remaining := max(0, 100-used)
	switch {
	case remaining >= 50:
		return HealthGreen
	case remaining >= 20:
		return HealthYellow
	}
	return HealthRed
}

// BurnProjection extrapolates the current burn rate to the window reset.
type BurnProjection struct {
	// ExhaustsAt is zero when the quota lasts until the reset.
	ExhaustsAt                  time.Time
	ProjectedUsedPercentAtReset int
}

// MakeBurnProjection mirrors Swift QuotaBurnProjection.make; nil means no
// projection is possible (no reset, no window, nothing used yet).
func MakeBurnProjection(w MeterWindow, now time.Time) *BurnProjection {
	if w.ResetsAt.IsZero() || w.WindowMinutes <= 0 || !w.ResetsAt.After(now) {
		return nil
	}
	used := min(100, max(0, w.UsedPercent))
	if used <= 0 {
		return nil
	}
	if used >= 100 {
		return &BurnProjection{ExhaustsAt: now, ProjectedUsedPercentAtReset: 100}
	}
	duration := float64(w.WindowMinutes * 60)
	start := w.ResetsAt.Add(-time.Duration(duration * float64(time.Second)))
	elapsed := now.Sub(start).Seconds()
	if elapsed <= 0 {
		return nil
	}
	rate := float64(used) / elapsed
	projected := min(100, max(0, int(math.Round(rate*duration))))
	toFull := float64(100-used) / rate
	exhausts := now.Add(time.Duration(toFull * float64(time.Second)))
	p := &BurnProjection{ProjectedUsedPercentAtReset: projected}
	if exhausts.Before(w.ResetsAt) {
		p.ExhaustsAt = exhausts
	}
	return p
}

func fromRateWindow(w codexapi.RateWindow) MeterWindow {
	return MeterWindow{UsedPercent: w.UsedPercent, WindowMinutes: w.WindowMinutes, ResetsAt: w.ResetsAt}
}

// WindowTitle names a Codex window by its length.
func WindowTitle(lc l10n.Localizer, windowMinutes int) string {
	switch windowMinutes {
	case FiveHourMinutes:
		return lc.Text(l10n.KeyFiveHourUsage)
	case WeeklyMinutes:
		return lc.Text(l10n.KeyWeeklyUsage)
	}
	return lc.Text(l10n.KeyQuota)
}

// ShortWindowTitle is WindowTitle plus the monthly window, for the status bar.
func ShortWindowTitle(lc l10n.Localizer, windowMinutes int) string {
	if windowMinutes == MonthlyMinutes {
		return lc.Text(l10n.KeyHeatmapMonthly)
	}
	return WindowTitle(lc, windowMinutes)
}

// QuotaMeters builds the Codex meters of a quota reading. Model-specific
// windows are included only when showModelSpecific is set.
func QuotaMeters(lc l10n.Localizer, q codexapi.QuotaSnapshot, showModelSpecific bool) []Meter {
	now := q.UpdatedAt
	meters := []Meter{NewMeter(WindowTitle(lc, q.Primary.WindowMinutes), fromRateWindow(q.Primary), now, DefaultMarkers(), SourceStandard)}
	if q.Secondary != nil {
		meters = append(meters, NewMeter(lc.Text(l10n.KeyWeeklyUsage), fromRateWindow(*q.Secondary), now, DefaultMarkers(), SourceStandard))
	}
	if showModelSpecific {
		for _, extra := range q.AdditionalWindows {
			meters = append(meters, NewMeter(extra.Name, fromRateWindow(extra.Window), now, DefaultMarkers(), SourceModelSpecific))
		}
	}
	return meters
}

// MenuBarText is the Codex status text: the time to reset, "Wait <time>" when
// the window is used up, or just the percentage when no reset is known.
func MenuBarText(lc l10n.Localizer, q codexapi.QuotaSnapshot, now time.Time) string {
	used := q.Primary.UsedPercent
	wait := lc.Text(l10n.KeyStatusWait)
	if q.Primary.ResetsAt.IsZero() {
		if used >= 100 {
			return wait
		}
		return strconv.Itoa(used) + "%"
	}
	text := DurationLocalized(q.Primary.ResetsAt.Sub(now).Seconds(), lc.Language(), false)
	if used >= 100 {
		return wait + " " + text
	}
	return text
}
