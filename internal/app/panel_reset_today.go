package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/resettoday/runwayfmt"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/store"
)

// resetCard is the verdict card with everything it needs at one instant.
type resetCard struct {
	l      l10n.Localizer
	snap   *resettoday.Snapshot
	now    time.Time
	loc    *time.Location
	verdct resettoday.VerdictPresentation
	busy   bool
	// site is the home page of the monitor the card reads.
	site string
}

// resetTodaySection is the reset-status section of a provider's panel;
// false for providers without a monitor.
func resetTodaySection(p store.Provider) (runway.Section, bool) {
	switch p {
	case store.ProviderCodex:
		return runway.SectionResetToday, true
	case store.ProviderGemini:
		return runway.SectionGeminiResetToday, true
	}
	return "", false
}

func newResetCard(l l10n.Localizer, state runway.ResetTodayState, busy bool) resetCard {
	rc := resetCard{l: l, now: time.Now(), loc: time.Local, busy: busy, site: resettoday.SiteURL}
	if s, ok := state.Detail.(*resettoday.Snapshot); ok && s != nil {
		rc.snap = s
		rc.verdct = s.VerdictPresentation(rc.now, rc.loc)
	}
	return rc
}

// panelResetToday is the "Did Codex reset today?" section; the Gemini panel
// shows the Antigravity monitor with the Gemini copy.
func panelResetToday(c *ui.Context, l l10n.Localizer, snap runway.Snapshot, acts PanelActions, first *bool, st *panelState) {
	sec, state := runway.SectionResetToday, snap.ResetToday
	if snap.Provider == store.ProviderGemini {
		sec, state = runway.SectionGeminiResetToday, snap.GeminiResetToday
		l = l.WithVariant(l10n.VariantGemini)
	}
	busy := snap.Refresh.Has(sec)
	rc := newResetCard(l, state, busy)
	if snap.Provider == store.ProviderGemini {
		rc.site = resettoday.AntigravityFeed.SiteURL
	}
	if !st.reaction.initialized && st.startReaction != nil {
		st.reaction.initialized = true
		if acts.QueryReaction != nil {
			st.startReaction(acts, false)
		}
	}
	sectionBlock(c, first, func() {
		drawSectionHead(c, l, st, sectionSpec{
			title: rc.sectionTitle(), refreshing: busy, refresh: sectionRefresh(acts, sec),
			info: &sectionInfo{id: "reset-today", help: l.Text(l10n.KeyRateLimitResetTodaySourceTitle), body: resetSourceBody(c, l, st, rc.site)},
		})
		cardColumn(c, surfaceRaisedKind, radiusCard).Padding(12).Children(func() {
			rc.hero(c, acts, st)
			rc.sections(c)
		})
	})
}

func (rc resetCard) sectionTitle() string {
	if rc.snap != nil {
		return rc.verdct.QuestionText(rc.l)
	}
	if rc.busy {
		return rc.l.Text(l10n.KeyRateLimitResetToday)
	}
	return rc.l.Text(l10n.KeyRateLimitResetQuestionUnavailable)
}

func resetSourceBody(c *ui.Context, l l10n.Localizer, st *panelState, site string) func() {
	t := c.Theme()
	return func() {
		ui.Text(c, l.Text(l10n.KeyRateLimitResetTodaySourceTitle)).FontSize(fsTitle3).FontWeight(600)
		ui.Text(c, l.Text(l10n.KeyRateLimitResetTodaySourceInfo)).FontSize(fsCallout).TextColor(inkSecondary(t))
		ui.Text(c, l.Text(l10n.KeyRateLimitResetTodayLocalDayHint)).FontSize(fsCallout).TextColor(inkSecondary(t))
		link := ui.ButtonBase(c).Label(l.Text(l10n.KeyRateLimitResetTodaySource)).Tooltip(l.Text(l10n.KeyRateLimitResetTodayOpenSource)).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		accent := brandInk(t, t.Accent)
		if link.Pressed() {
			accent = brandInk(t, t.AccentPressed)
		} else if link.Hovered() {
			accent = brandInk(t, t.AccentHover)
		}
		link.Children(func() {
			ui.Text(c, l.Text(l10n.KeyRateLimitResetTodaySource)).FontSize(fsCallout).FontWeight(500).TextColor(accent).Underline().SingleLine()
			ui.Icon(c, icArrowUpRight).Size(11, 11).TextColor(accent)
		})
		if link.Clicked() {
			st.info = ""
			c.OpenURL(site)
		}
		ui.Row(c).Children(func() {
			ui.Spacer(c).Grow(1)
			if PillButton(c, nil, l.Text(l10n.KeyOk), false).Clicked() {
				st.info = ""
			}
		})
	}
}

// heroColor is the color of the verdict word.
func (rc resetCard) heroColor(t *ui.Theme) ui.Color {
	p := rc.verdct
	switch {
	case rc.snap == nil || p.Reason == export.ReasonUnavailable:
		return inkSecondary(t)
	case p.Band != nil:
		return bandColor(t, *p.Band)
	case p.IsCompleted && p.ResetType != nil && *p.ResetType == alerts.ResetBanked:
		return sys(t, sysBlue)
	case p.ShowsYes:
		return sys(t, sysGreen)
	}
	return sys(t, sysOrange)
}

func bandColor(t *ui.Theme, b export.ConfidenceBand) ui.Color {
	if b == export.BandOK {
		return sys(t, sysPurple)
	}
	return sys(t, sysYellow)
}

func resetTypeColor(t *ui.Theme, rt alerts.ResetType) ui.Color {
	switch rt {
	case alerts.ResetBanked:
		return sys(t, sysBlue)
	case alerts.ResetGlobalAndBanked:
		return sys(t, sysPurple)
	}
	return sys(t, sysGreen)
}

// hero is the verdict and the reaction button, with the explaining line.
func (rc resetCard) hero(c *ui.Context, acts PanelActions, st *panelState) {
	t := c.Theme()
	color := rc.heroColor(t)
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			rc.heroTitle(c, color)
			ui.Spacer(c).Grow(1)
			rc.reactionButton(c, acts, st)
		})
		rc.heroSubtitle(c)
		if st.reaction.task.Err != nil {
			message := tx(rc.l, xReactionFailed)
			if st.reaction.snapshot != nil && st.reaction.snapshot.IsExhausted() {
				message = rc.l.Text(l10n.KeyRateLimitResetTodayReactionLimitReached)
			}
			ui.Text(c, message).FontSize(fsCaption).TextColor(sys(t, sysRed))
		}
	})
}

func (rc resetCard) heroTitle(c *ui.Context, color ui.Color) {
	if rc.snap == nil {
		answer := rc.l.Text(l10n.KeyRateLimitResetUnavailable)
		if rc.busy {
			answer = "…"
		}
		ui.Text(c, answer).FontSize(28).FontWeight(600).TextColor(color).SingleLine()
		return
	}
	spans := []ui.Span{}
	if percent := rc.verdct.PercentText(rc.l); percent != "" {
		spans = append(spans, ui.Span{Text: percent, Size: 22, Weight: 600, Color: color}, ui.Span{Text: " ", Size: 22})
	}
	spans = append(spans, ui.Span{Text: rc.verdct.AnswerText(rc.l), Size: 28, Weight: 600, Color: color})
	ui.RichText(c, spans...).Label(rc.verdct.TitleText(rc.l))
}

func (rc resetCard) heroSubtitle(c *ui.Context) {
	t := c.Theme()
	hint := rc.l.Text(l10n.KeyRateLimitResetTodayUnavailableHint)
	if rc.snap == nil {
		if rc.busy {
			hint = rc.l.Text(l10n.KeyCalculating)
		}
		ui.Text(c, hint).FontSize(fsCaption).TextColor(inkSecondary(t))
		return
	}
	detail := rc.snap.VerdictDetail(rc.l, runwayfmt.Formatter{}, rc.now, rc.loc)
	if detail == nil {
		ui.Text(c, hint).FontSize(fsCaption).TextColor(inkSecondary(t))
		return
	}
	ui.RichText(c, rc.detailSpans(t, detail)...)
}

// detailSpans colors the reset type, percentage and time of the sentence and
// underlines them, as the Swift detail text does.
func (rc resetCard) detailSpans(t *ui.Theme, d *resettoday.DetailPresentation) []ui.Span {
	secondary := inkSecondary(t)
	emphasis, typeColor := sys(t, sysGreen), sys(t, sysGreen)
	if rc.verdct.Band != nil {
		emphasis = bandColor(t, *rc.verdct.Band)
		typeColor = emphasis
	} else if d.ResetType != nil {
		typeColor = resetTypeColor(t, *d.ResetType)
	}
	marked := func(text string, color ui.Color) ui.Span {
		return ui.Span{Text: text, Size: fsCaption, Weight: 600, Color: color, Underline: true}
	}
	var spans []ui.Span
	for _, tok := range d.Tokens {
		switch tok.Kind {
		case resettoday.TokenResetType:
			spans = append(spans, marked(d.TypeLabel, typeColor))
		case resettoday.TokenPercent:
			spans = append(spans, marked(deref(d.PercentText), emphasis))
		case resettoday.TokenTime:
			spans = append(spans, marked(deref(d.TimeText), emphasis))
		default:
			spans = append(spans, ui.Span{Text: tok.Text, Size: fsCaption, Color: secondary})
		}
	}
	return spans
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// sections are the divided zones under the hero.
func (rc resetCard) sections(c *ui.Context) {
	t := c.Theme()
	divided := func(body func()) {
		ui.Column(c).Gap(8).Padding(8, 0, 0, 0).Children(func() {
			hairlineFaint(c)
			body()
		})
	}
	if rc.snap != nil && rc.verdct.Reason != export.ReasonUnavailable {
		if next := rc.snap.NextScheduledResetSummary(rc.now); next != nil {
			divided(func() { rc.nextScheduled(c, next) })
		}
	}
	if scope := rc.scopeText(); scope != "" {
		divided(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icFilterSliders).Size(11, 11).TextColor(inkSecondary(t))
				ui.Text(c, scope).FontSize(fsCaption).TextColor(inkSecondary(t)).MaxLines(2).Grow(1).Basis(0)
			})
		})
	}
	if rc.hasEvidence() {
		divided(func() { rc.evidence(c) })
	}
	if meta := rc.metaText(); meta != "" {
		divided(func() { ui.Text(c, meta).FontSize(fsCaption).TextColor(inkSecondary(t)) })
	}
	divided(func() { rc.websiteLink(c) })
}

func (rc resetCard) nextScheduled(c *ui.Context, next *resettoday.UpcomingSummary) {
	t := c.Theme()
	lang := rc.l.Language()
	win := runway.ScheduleWindow{StartAt: next.Window.StartAt, EndAt: next.Window.EndAt}
	label := runway.ScheduledLabel(win, rc.loc)
	remaining := runway.ScheduledCountdown(win, rc.now, lang)
	countdown := l10n.Format(rc.l.Text(l10n.KeyRateLimitResetTodayUntilReset), remaining)
	timeColor := bandColor(t, resettoday.ScheduleConfidenceBand(next.Event))
	if next.Confidence > 0 {
		timeColor = bandColor(t, resettoday.BandFor(resettoday.DisplayedPercent(next.Confidence)))
	}
	secondary := inkSecondary(t)
	ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icClock).Size(11, 11).TextColor(secondary).Margin(1, 0, 0, 0)
		ui.RichText(c,
			ui.Span{Text: rc.l.Text(l10n.KeyRateLimitResetTodayNextScheduled) + lang.ColonSeparator(), Size: fsCaption, Color: secondary},
			ui.Span{Text: resetTypeName(rc.l, &next.ResetType) + " · ", Size: fsCaption, Weight: 600, Color: resetTypeColor(t, next.ResetType)},
			ui.Span{Text: label, Size: fsCaption, Weight: 700, Color: timeColor},
			ui.Span{Text: lang.OpenParen() + countdown + lang.CloseParen(), Size: fsCaption, Color: secondary},
		).Grow(1).Basis(0)
	})
}

func (rc resetCard) scopeText() string {
	if rc.snap == nil || rc.verdct.EvidenceEvent == nil {
		return ""
	}
	return rc.snap.ScopeSummary(*rc.verdct.EvidenceEvent, rc.l)
}

func (rc resetCard) evidenceURL() string {
	if rc.snap == nil {
		return ""
	}
	return rc.snap.EvidenceURL(rc.now, rc.loc, rc.l.Language())
}

func (rc resetCard) evidenceLine() string {
	if rc.snap == nil {
		return ""
	}
	return rc.snap.EvidenceLine(rc.l, rc.now, rc.loc)
}

func (rc resetCard) hasEvidence() bool { return rc.evidenceURL() != "" || rc.evidenceLine() != "" }

func (rc resetCard) evidence(c *ui.Context) {
	t := c.Theme()
	line := rc.evidenceLine()
	if line == "" {
		line = rc.l.Text(l10n.KeyRateLimitResetTodayLatestEvidence)
	}
	url := rc.evidenceURL()
	row := ui.ButtonBase(c).Label(line).Disabled(url == "").Padding(3, 6).Gap(6).Radius(radiusRow).AlignItems(ui.Center).Margin(-3, -6)
	if url != "" {
		row.Tooltip(rc.l.Text(l10n.KeyShowDetails)).Cursor(ui.CursorPointer)
		if row.Hovered() {
			row.Background(hoverNeutral(t))
		}
	}
	row.Children(func() {
		ui.Icon(c, icChat).Size(11, 11).TextColor(inkSecondary(t))
		ui.Text(c, line).FontSize(fsCaption).TextColor(t.Text.Alpha(0.85)).MaxLines(2).Grow(1).Basis(0)
		if url != "" {
			ui.Icon(c, icArrowUpRight).Size(11, 11).TextColor(inkTertiary(t))
		}
	})
	if row.Clicked() && url != "" {
		c.OpenURL(url)
	}
}

// metaText is the "last check · last reset · confidence · fetched" line.
func (rc resetCard) metaText() string {
	if rc.snap == nil {
		if rc.busy {
			return rc.l.Text(l10n.KeyCalculating)
		}
		return rc.l.Text(l10n.KeyRateLimitResetTodayUnavailableHint)
	}
	lang := rc.l.Language()
	var parts []string
	if at := rc.snap.LastSuccessfulCheckAt; at != nil {
		parts = append(parts, rc.l.Text(l10n.KeyRateLimitResetTodayLastCheck)+" "+runway.RelativePast(*at, rc.now, lang, true))
	}
	if latest := rc.snap.LatestReset(rc.now); latest != nil {
		parts = append(parts, rc.l.Text(l10n.KeyLastReset)+" "+resetTypeName(rc.l, &latest.ResetType)+" · "+runway.RelativePast(latest.At, rc.now, lang, true))
	}
	if conf := rc.verdct.Confidence; conf != nil {
		parts = append(parts, rc.l.Text(l10n.KeyRateLimitResetTodayConfidence)+" "+l10n.Format("%d%%", int(*conf*100+0.5)))
	}
	parts = append(parts, rc.l.Text(l10n.KeyRateLimitResetTodayLastFetched)+" "+runway.RelativePast(rc.snap.FetchedAt, rc.now, lang, true))
	return strings.Join(parts, " · ")
}

func (rc resetCard) websiteLink(c *ui.Context) {
	t := c.Theme()
	title := rc.l.Text(l10n.KeyRateLimitResetTodayOpenWebsite)
	b := ui.ButtonBase(c).Label(title).Tooltip(rc.l.Text(l10n.KeyRateLimitResetTodayOpenSource)).Padding(3, 6).Gap(4).Radius(radiusRow).AlignItems(ui.Center).Margin(-3, -6).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(hoverNeutral(t))
	}
	link := brandInk(t, t.Accent)
	if b.Pressed() {
		link = brandInk(t, t.AccentPressed)
	} else if b.Hovered() {
		link = brandInk(t, t.AccentHover)
	}
	b.Children(func() {
		underlineIf(ui.Text(c, title).FontSize(fsCaption).TextColor(link).Grow(1).Basis(0), b.Hovered())
		ui.Icon(c, icArrowUpRight).Size(11, 11).TextColor(link)
	})
	if b.Clicked() {
		c.OpenURL(rc.site)
	}
}

// reactionButton is the capsule next to the verdict: the thanks / please
// label with the count, a spinner while the count is loading.
func (rc resetCard) reactionButton(c *ui.Context, acts PanelActions, st *panelState) {
	t := c.Theme()
	rs := &st.reaction
	snap := rs.snapshot
	loading := rs.task.Running && snap == nil
	if snap == nil || !snap.IsVisible() {
		if loading {
			ui.Row(c).Padding(4, 8).Radius(100).Background(surfaceRaised(t)).Border(1, hairlineColor(t)).Children(func() { ui.Spinner(c).Size(10, 10) })
		}
		return
	}
	yes := snap.Polarity != nil && *snap.Polarity == resettoday.PolarityYes
	key, emoji := l10n.KeyRateLimitResetTodayReactionPlease, "\U0001F64F"
	if yes {
		key, emoji = l10n.KeyRateLimitResetTodayReactionThank, "\U0001F389"
	}
	count := 0
	if snap.Count != nil {
		count = *snap.Count
	}
	exhausted := snap.IsExhausted()
	disabled := rs.task.Running || exhausted || acts.React == nil
	label := rc.l.Text(key)
	b := ui.ButtonBase(c).Label(label).Disabled(disabled).Padding(4, 8).Gap(4).AlignItems(ui.Center).Radius(100)
	hovered := b.Hovered() && !disabled
	fill, border, ink := surfaceRaised(t), hairlineColor(t), t.Text
	if yes {
		ink = sys(t, sysGreen)
		border = ink.Alpha(0.55)
		if hovered {
			fill, border = ink.Alpha(0.16), ink.Alpha(0.95)
		}
	} else if hovered {
		fill, border = hoverNeutral(t), t.Text.Alpha(0.35)
	}
	b.Background(fill).Border(1, border).Tooltip(rc.reactionHelp(exhausted, count, yes))
	if !disabled {
		b.Cursor(ui.CursorPointer)
	}
	b.Children(func() {
		if rs.task.Running {
			ui.Spinner(c).Size(10, 10)
		} else {
			ui.Text(c, emoji).FontSize(fsCaption)
		}
		ui.Text(c, label).FontSize(fsCaption).FontWeight(600).TextColor(ink).SingleLine()
		ui.Text(c, resettoday.FormatCount(count)).FontSize(fsCaption).FontWeight(600).FontFeatures("tnum").TextColor(ink).SingleLine()
	})
	if b.Clicked() && st.startReaction != nil {
		st.startReaction(acts, true)
	}
	reactionFloat(c, b, rs)
}

func (rc resetCard) reactionHelp(exhausted bool, count int, yes bool) string {
	if exhausted {
		return rc.l.Text(l10n.KeyRateLimitResetTodayReactionLimitReached)
	}
	key := l10n.KeyRateLimitResetTodayReactionPleaseAria
	if yes {
		key = l10n.KeyRateLimitResetTodayReactionThankAria
	}
	return l10n.Format(rc.l.Text(key), resettoday.FormatCount(count))
}
