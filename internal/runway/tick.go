package runway

import (
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// Tick is the once-a-second heartbeat (the tick loop calls it; tests drive it
// with a fake clock). It refreshes countdown texts and starts the refreshes
// that are due: a passed quota reset, the refresh interval, the widget
// interval and the reset-today feed.
func (s *Service) Tick(now time.Time) {
	s.mu.Lock()
	snap := s.buildLocked(now)
	if key := tickKey(snap); key != s.lastTickKey {
		s.lastTickKey = key
		s.publishSnapLocked(snap)
	}
	hasWidgets := s.widgetReqs != 0 && s.opts.WidgetSink != nil
	refreshWidgets := hasWidgets && s.widgetSchedule.IsDue(now)
	// Section refreshes never hold the schedule back; only a full refresh
	// already running does.
	idle := !s.allBusy
	start := false
	if reset := s.nextDueResetLocked(s.lastReset, now); !reset.IsZero() && idle {
		s.lastReset = reset
		start = true
	} else if (s.schedule.IsDue(now) || refreshWidgets) && idle {
		start = true
	}
	resetTodayDue := false
	if s.opts.ResetToday != nil && s.resetTodayWantedLocked() {
		interval := time.Duration(s.prefs.RateLimitResetTodayRefreshIntervalSeconds) * time.Second
		last := s.opts.ResetToday.LastFetch()
		resetTodayDue = last.IsZero() || now.Sub(last) >= interval
	}
	s.mu.Unlock()

	if start {
		s.beginFullRefreshWidgets(usage.RefreshIfChanged, refreshWidgets)
	}
	if resetTodayDue {
		s.RefreshResetToday(false)
	}
}

// tickKey is everything a tick can change in a snapshot: the countdown
// texts. Only a change republishes, so an idle app stays quiet.
func tickKey(s Snapshot) string {
	var b strings.Builder
	b.WriteString(s.Codex.StatusText)
	b.WriteByte(0)
	b.WriteString(s.Codex.CostSubtitle)
	b.WriteByte(0)
	b.WriteString(s.StatusBar.Text)
	b.WriteByte(0)
	b.WriteString(s.StatusBar.Tooltip)
	for _, c := range s.StatusBar.TextCaptions {
		b.WriteByte(0)
		b.WriteString(c)
	}
	for _, it := range s.StatusBar.Items {
		b.WriteByte(0)
		b.WriteString(it.Ring + "|" + it.Countdown + "|" + it.Battery + "|" + it.MeterText)
	}
	b.WriteByte(0)
	b.WriteString(s.ResetToday.Text)
	return b.String()
}

// NextDueQuotaReset returns the earliest quota reset after `after` that passed
// at least a second ago, or the zero time. The Grok panel uses its first meter.
func (s *Service) NextDueQuotaReset(after, now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextDueResetLocked(after, now)
}

func (s *Service) nextDueResetLocked(after, now time.Time) time.Time {
	if s.prefs.SelectedProvider == store.ProviderCursor {
		if s.opts.Cursor == nil || !s.prefs.CursorMonitoringEnabled {
			return time.Time{}
		}
		var due time.Time
		for _, m := range s.opts.Cursor.State(now).Meters {
			r := m.ResetsAt
			if r.After(after) && now.Sub(r) >= time.Second && (due.IsZero() || r.Before(due)) {
				due = r
			}
		}
		return due
	}
	if s.prefs.SelectedProvider == store.ProviderGrok || s.prefs.SelectedProvider == store.ProviderClaude {
		var meters []Meter
		if s.prefs.SelectedProvider == store.ProviderGrok {
			if s.opts.Grok == nil {
				return time.Time{}
			}
			meters = s.opts.Grok.State(now).Meters
		} else {
			if s.opts.Claude == nil {
				return time.Time{}
			}
			meters = s.opts.Claude.State(now).Meters
		}
		if len(meters) == 0 {
			return time.Time{}
		}
		r := meters[0].ResetsAt
		for _, m := range meters {
			if m.WindowMinutes == FiveHourMinutes && m.HasReset() {
				r = m.ResetsAt
				break
			}
		}
		if r.IsZero() || !r.After(after) || now.Sub(r) < time.Second {
			return time.Time{}
		}
		return r
	}
	if s.quota == nil {
		return time.Time{}
	}
	return s.quota.NextDueReset(after, now)
}
