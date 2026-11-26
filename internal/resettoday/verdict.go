package resettoday

import (
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
)

// OKPercent is the percentage from which a scheduled answer is "ok".
const OKPercent = 80

// DisplayedPercent is the whole percentage shown for a 0..1 confidence.
func DisplayedPercent(confidence float64) int { return alerts.DisplayedPercent(confidence) }

// BandFor is the confidence band of a displayed percentage.
func BandFor(percent int) export.ConfidenceBand {
	if percent >= OKPercent {
		return export.BandOK
	}
	return export.BandWarn
}

// VerdictPresentation is the homepage-aligned hero: completed is "Yes";
// scheduled is a percentage plus "Yes".
type VerdictPresentation struct {
	ShowsYes       bool
	Percent        *int
	Band           *export.ConfidenceBand
	IsScheduled    bool
	IsCompleted    bool
	ResetType      *ResetType
	Reason         export.VerdictReason
	EvidenceEvent  *Event
	ScheduleWindow *ScheduleWindow
	ScheduleBasis  ScheduleBasis // "" when none
	Confidence     *float64
	CompletedAt    *time.Time
}

// VerdictPresentation is verdictPresentation(now:calendar:).
func (s *Snapshot) VerdictPresentation(now time.Time, loc *time.Location) VerdictPresentation {
	loc = locOrLocal(loc)
	c, resetType := s.combinedVerdictCandidate(now, loc)
	scheduled := c.reason == export.ReasonUpcoming || c.reason == export.ReasonGrace
	expired := c.reason == export.ReasonExpiredUnconfirmed
	p := VerdictPresentation{
		ShowsYes:    c.reason == export.ReasonCompleted || scheduled,
		IsScheduled: scheduled,
		IsCompleted: c.reason == export.ReasonCompleted,
		Reason:      c.reason,
		CompletedAt: c.completedAt,
	}
	if !expired {
		p.Confidence = c.confidence
		p.ResetType = resetType
		p.EvidenceEvent = c.event
		p.ScheduleWindow = c.window
		p.ScheduleBasis = c.scheduleBasis
		if p.ScheduleBasis == "" && c.event != nil {
			p.ScheduleBasis = c.event.ScheduleBasis
		}
	}
	if scheduled && p.Confidence != nil {
		percent := DisplayedPercent(*p.Confidence)
		band := BandFor(percent)
		p.Percent, p.Band = &percent, &band
	}
	return p
}

// ExportVerdict is the part of the presentation the widget stores.
func (p VerdictPresentation) ExportVerdict() export.Verdict {
	v := export.Verdict{
		Reason:      p.Reason,
		ShowsYes:    p.ShowsYes,
		ResetType:   p.ResetType,
		IsScheduled: p.IsScheduled,
		Percent:     p.Percent,
		Band:        p.Band,
	}
	if p.ScheduleBasis != "" {
		b := p.ScheduleBasis
		v.ScheduleBasis = &b
	}
	return v
}

// PrimaryEvidenceEvent is the event that best explains the current answer.
func (s *Snapshot) PrimaryEvidenceEvent(now time.Time, loc *time.Location) *Event {
	return s.VerdictPresentation(now, loc).EvidenceEvent
}

// ScheduleConfidenceBand is the band of an event's own confidence.
func ScheduleConfidenceBand(e Event) export.ConfidenceBand {
	return BandFor(DisplayedPercent(e.Confidence))
}

// QuestionKey is the question line for a verdict reason.
func QuestionKey(reason export.VerdictReason) l10n.Key {
	switch reason {
	case export.ReasonUpcoming:
		return l10n.KeyRateLimitResetQuestionUpcoming
	case export.ReasonGrace:
		return l10n.KeyRateLimitResetQuestionScheduledGrace
	case export.ReasonUnavailable:
		return l10n.KeyRateLimitResetQuestionUnavailable
	}
	return l10n.KeyRateLimitResetQuestionCurrent
}

// QuestionText localizes the question line.
func (p VerdictPresentation) QuestionText(l l10n.Localizer) string {
	return l.Text(QuestionKey(p.Reason))
}

// PercentText is "≥80%"-style text, "" when the verdict has no percentage.
func (p VerdictPresentation) PercentText(l l10n.Localizer) string {
	if p.Percent == nil {
		return ""
	}
	key := l10n.KeyRateLimitResetTodayPercentPrefix
	if *p.Percent == 100 {
		key = l10n.KeyRateLimitResetTodayPercentExact
	}
	return l.Textf(key, strconv.Itoa(*p.Percent))
}

// AnswerText is the yes/no/unavailable word.
func (p VerdictPresentation) AnswerText(l l10n.Localizer) string {
	switch {
	case p.Reason == export.ReasonUnavailable:
		return l.Text(l10n.KeyRateLimitResetUnavailable)
	case p.ShowsYes:
		return l.Text(l10n.KeyRateLimitResetTodayYes)
	}
	return l.Text(l10n.KeyRateLimitResetTodayNo)
}

// TitleText is the percentage followed by the answer.
func (p VerdictPresentation) TitleText(l l10n.Localizer) string {
	return p.PercentText(l) + p.AnswerText(l)
}

// EvidenceKey picks the app-owned sentence for the evidence event; the feed
// text is never shown. ok is false when there is no evidence.
func (s *Snapshot) EvidenceKey(now time.Time, loc *time.Location) (key l10n.Key, ok bool) {
	p := s.VerdictPresentation(now, loc)
	if p.Reason == export.ReasonCompleted && (p.EvidenceEvent == nil || p.EvidenceEvent.Kind != KindResetCompleted) {
		return l10n.KeyRateLimitResetTodayEvidenceManualCompletion, true
	}
	if p.EvidenceEvent == nil {
		return "", false
	}
	return evidenceKey(*p.EvidenceEvent), true
}

func pickByType(t ResetType, global, banked, both l10n.Key) l10n.Key {
	switch t {
	case ResetBanked:
		return banked
	case ResetGlobalAndBanked:
		return both
	}
	return global
}

func evidenceKey(e Event) l10n.Key {
	switch e.Kind {
	case KindResetCompleted:
		if e.Source.IsOperator() {
			return l10n.KeyRateLimitResetTodayEvidenceOperatorCompletion
		}
		return pickByType(e.ResetType,
			l10n.KeyRateLimitResetTodayEvidenceResetCompleted,
			l10n.KeyRateLimitResetTodayEvidenceBankedCompleted,
			l10n.KeyRateLimitResetTodayEvidenceGlobalAndBankedCompleted)
	case KindResetScheduled:
		if e.Source.IsOperator() {
			return l10n.KeyRateLimitResetTodayEvidenceOperatorScheduled
		}
		if e.ScheduleBasis == BasisContextualInference {
			return pickByType(e.ResetType,
				l10n.KeyRateLimitResetTodayEvidenceResetPreview,
				l10n.KeyRateLimitResetTodayEvidenceBankedPreview,
				l10n.KeyRateLimitResetTodayEvidenceGlobalAndBankedPreview)
		}
		return pickByType(e.ResetType,
			l10n.KeyRateLimitResetTodayEvidenceResetScheduled,
			l10n.KeyRateLimitResetTodayEvidenceBankedScheduled,
			l10n.KeyRateLimitResetTodayEvidenceGlobalAndBankedScheduled)
	case KindBankedReset:
		return l10n.KeyRateLimitResetTodayEvidenceBankedReset
	case KindLimitIncrease:
		return l10n.KeyRateLimitResetTodayEvidenceLimitIncrease
	}
	return l10n.KeyRateLimitResetTodayEvidenceUncertain
}

// EvidenceLine is the localized evidence sentence, "" when there is none.
func (s *Snapshot) EvidenceLine(l l10n.Localizer, now time.Time, loc *time.Location) string {
	if key, ok := s.EvidenceKey(now, loc); ok {
		return l.Text(key)
	}
	return ""
}

// EvidenceURL is the hosted history page of the evidence event; "" when there
// is none. Chinese languages get the /zh/ page.
func (s *Snapshot) EvidenceURL(now time.Time, loc *time.Location, lang l10n.Language) string {
	e := s.PrimaryEvidenceEvent(now, loc)
	if e == nil {
		return ""
	}
	ms := e.AnnouncedAt.Unix()*1000 + int64((e.AnnouncedAt.Nanosecond()+500_000)/1_000_000)
	feed := s.Feed.orCodex()
	prefix := feed.SiteURL
	if lang == l10n.SimplifiedChinese || lang == l10n.TraditionalChinese {
		prefix += "zh/"
	}
	return prefix + "history/" + strconv.FormatInt(ms, 10) + feed.HistoryExt
}

// ScopeSummary is "Plans: ... · Windows: ...", "" when the scope only says
// unknown.
func (s *Snapshot) ScopeSummary(e Event, l l10n.Localizer) string {
	var parts []string
	if plans := scopeLabels(e.Scope.Plans, func(p string) string { return planLabel(p, l) }); len(plans) > 0 {
		parts = append(parts, l.Text(l10n.KeyRateLimitResetTodayPlans)+": "+strings.Join(plans, ", "))
	}
	if windows := scopeLabels(e.Scope.Windows, func(w string) string { return windowLabel(w, l) }); len(windows) > 0 {
		parts = append(parts, l.Text(l10n.KeyRateLimitResetTodayWindows)+": "+strings.Join(windows, ", "))
	}
	return strings.Join(parts, " · ")
}

func scopeLabels(values []string, label func(string) string) []string {
	var out []string
	for _, v := range values {
		if v == "unknown" {
			continue
		}
		if text := label(v); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func planLabel(plan string, l l10n.Localizer) string {
	keys := map[string]l10n.Key{
		"all": l10n.KeyRateLimitResetTodayPlanAll, "free": l10n.KeyRateLimitResetTodayPlanFree,
		"plus": l10n.KeyRateLimitResetTodayPlanPlus, "pro": l10n.KeyRateLimitResetTodayPlanPro,
		"team": l10n.KeyRateLimitResetTodayPlanTeam, "business": l10n.KeyRateLimitResetTodayPlanBusiness,
		"enterprise": l10n.KeyRateLimitResetTodayPlanEnterprise, "unknown": l10n.KeyRateLimitResetTodayPlanUnknown,
	}
	if k, ok := keys[plan]; ok {
		return l.Text(k)
	}
	return plan
}

func windowLabel(window string, l l10n.Localizer) string {
	keys := map[string]l10n.Key{
		"weekly": l10n.KeyRateLimitResetTodayWindowWeekly, "five_hour": l10n.KeyRateLimitResetTodayWindowFiveHour,
		"unknown": l10n.KeyRateLimitResetTodayWindowUnknown,
	}
	if k, ok := keys[window]; ok {
		return l.Text(k)
	}
	return window
}
