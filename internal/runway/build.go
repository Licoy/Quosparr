package runway

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
)

// buildLocked renders the state into an immutable Snapshot. It only reads
// the state; every slice it hands out is freshly allocated or never mutated.
func (s *Service) buildLocked(now time.Time) Snapshot {
	lc := s.lc
	snap := Snapshot{
		GeneratedAt: now,
		Language:    lc.Language(),
		Provider:    s.prefs.SelectedProvider,
		Accounts:    s.accountsViewLocked(now),
		Codex:       s.codexViewLocked(now),
		Refresh:     s.refreshViewLocked(),
	}
	if s.opts.Grok != nil {
		snap.Grok = s.opts.Grok.State(now)
	} else {
		snap.Grok = GrokState{Availability: GrokCLIUnavailable}
	}
	if s.opts.Claude != nil {
		snap.Claude = s.opts.Claude.State(now)
	}
	if s.opts.Cursor != nil {
		snap.Cursor = s.opts.Cursor.State(now)
	}
	if s.opts.ResetToday != nil {
		snap.ResetToday = s.opts.ResetToday.State(now)
	}
	in := StatusInputs{
		Prefs: s.prefs, Provider: s.prefs.SelectedProvider,
		CodexText: snap.Codex.StatusText, CodexMeters: snap.Codex.Meters,
		Grok: snap.Grok, Claude: snap.Claude, Cursor: snap.Cursor, Now: now,
	}
	snap.StatusBar = BuildStatusBar(lc, in)
	if s.prefs.FloatingWindow.Enabled {
		snap.Floating = BuildFloating(lc, in)
	}
	return snap
}

func (s *Service) accountsViewLocked(now time.Time) AccountsView {
	v := AccountsView{
		Sidebar:   s.index.OrderedForSidebar(),
		ActiveID:  s.index.ActiveAccountID,
		Switching: s.switching,
		Message:   s.accountMessage,
	}
	for id := range s.refreshingIDs {
		v.RefreshingIDs = append(v.RefreshingIDs, id)
	}
	sort.Strings(v.RefreshingIDs)
	if s.auth != nil {
		plan := s.planHint
		if s.quota != nil {
			plan = s.quota.Plan
		}
		v.Display = codex.DisplayForAuth(*s.auth, plan, now)
	}
	return v
}

func (s *Service) refreshViewLocked() RefreshView {
	v := RefreshView{All: s.allBusy, AccountQuotas: s.accountsBusy, CostProgress: s.progress}
	for sec, on := range s.busy {
		if on {
			v.Sections = append(v.Sections, sec)
		}
	}
	sort.Slice(v.Sections, func(i, j int) bool { return v.Sections[i] < v.Sections[j] })
	return v
}

// currentAccountIDLocked is the managed id of the shown credential.
func (s *Service) currentAccountIDLocked() string {
	if s.auth == nil {
		return ""
	}
	for _, a := range s.index.Accounts {
		if codex.AccountMatches(a, *s.auth) {
			return a.ID
		}
	}
	return s.index.ActiveAccountID
}

func (s *Service) codexViewLocked(now time.Time) CodexView {
	lc := s.lc
	v := CodexView{
		AccountID:      s.currentAccountIDLocked(),
		Generation:     s.epoch,
		Authenticated:  s.auth != nil,
		Heatmap:        s.heat,
		DisplayedRange: s.dispRange,
		CostScanNote:   s.costNote,
		LastError:      s.lastErr,
		EstimateError:  s.estErr,
		Estimate:       s.est,
		Cost:           s.cost,
		DisplayedCost:  s.disp,
		Quota:          s.quota,
		ResetCredits:   s.credits,
	}
	if s.keyModeLocked() {
		s.keyViewLocked(&v)
	} else {
		s.quotaViewLocked(&v, now)
		s.creditsViewLocked(&v)
	}
	s.costViewLocked(&v, now)
	s.sessionsViewLocked(&v)
	_ = lc
	return v
}

func (s *Service) quotaViewLocked(v *CodexView, now time.Time) {
	lc := s.lc
	q := s.quota
	switch {
	case q != nil:
		v.StatusText = MenuBarText(lc, *q, now)
		v.Meters = QuotaMeters(lc, *q, s.prefs.ShowsModelSpecificQuotaUsage)
	case s.quotaErr != "" || s.statusFail:
		v.StatusText = lc.Text(l10n.KeyStatusError)
	default:
		v.StatusText = lc.Text(l10n.KeyStatusLogin)
	}
	switch {
	case s.quotaErr != "":
		v.QuotaText = lc.Text(l10n.KeyStatusError)
		v.QuotaLines = []DetailLine{{lc.Text(l10n.KeyError), s.quotaErr}}
	case q != nil:
		v.QuotaText, v.QuotaLines = s.quotaText(*q)
	case s.loggedOut:
		v.QuotaText = lc.Text(l10n.KeyStatusLogin)
	default:
		v.QuotaText = lc.Text(l10n.KeyNotLoaded)
	}
}

func (s *Service) quotaText(q codexapi.QuotaSnapshot) (string, []DetailLine) {
	lc := s.lc
	unknown := lc.Text(l10n.KeyUnknown)
	plan := q.Plan
	if plan == "" {
		plan = unknown
	}
	title := WindowTitle(lc, q.Primary.WindowMinutes)
	weekly := lc.Text(l10n.KeyWeeklyUsage) + " n/a"
	if q.Secondary != nil {
		weekly = lc.Text(l10n.KeyWeeklyUsage) + " " + strconv.Itoa(q.Secondary.UsedPercent) + "%"
	}
	text := lc.Text(l10n.KeyPlan) + " " + plan + " · " + title + " " + strconv.Itoa(q.Primary.UsedPercent) + "% · " + weekly
	lines := []DetailLine{
		{lc.Text(l10n.KeyPlan), plan},
		{title, s.windowText(q.Primary, q.UpdatedAt)},
	}
	if q.Secondary != nil {
		lines = append(lines, DetailLine{lc.Text(l10n.KeyWeeklyUsage), s.windowText(*q.Secondary, q.UpdatedAt)})
	}
	if s.prefs.ShowsModelSpecificQuotaUsage {
		for _, e := range q.AdditionalWindows {
			lines = append(lines, DetailLine{e.Name, s.windowText(e.Window, q.UpdatedAt)})
		}
	}
	if q.CreditsBalance != nil {
		lines = append(lines, DetailLine{lc.Text(l10n.KeyCreditsBalance), fmt.Sprintf("%.2f", *q.CreditsBalance)})
	}
	return text, lines
}

func (s *Service) windowText(w codexapi.RateWindow, now time.Time) string {
	lc := s.lc
	reset := ""
	if !w.ResetsAt.IsZero() {
		reset = " · " + lc.Text(l10n.KeyResetsIn) + " " + DurationLocalized(w.ResetsAt.Sub(now).Seconds(), lc.Language(), true)
	}
	return strconv.Itoa(w.UsedPercent) + "% " + lc.Text(l10n.KeyUsed) + reset
}

func (s *Service) creditsViewLocked(v *CodexView) {
	lc := s.lc
	c := s.credits
	switch {
	case s.creditsErr != "":
		v.ResetCreditsText = lc.Text(l10n.KeyStatusError)
		v.ResetCreditLines = []DetailLine{{lc.Text(l10n.KeyError), s.creditsErr}}
	case c != nil:
		sum := SummarizeCredits(*c)
		v.ResetCreditSummary = &sum
		v.CreditDetails = CreditDetails(lc, *c)
		v.ResetCreditsText, v.ResetCreditLines = s.creditsText(*c, sum)
	case s.loggedOut:
		v.ResetCreditsText = lc.Text(l10n.KeyStatusLogin)
	default:
		v.ResetCreditsText = lc.Text(l10n.KeyNotLoaded)
	}
	if s.creditsErr != "" && c != nil {
		sum := SummarizeCredits(*c)
		v.ResetCreditSummary = &sum
		v.CreditDetails = CreditDetails(lc, *c)
	}
}

func (s *Service) creditsText(c codexapi.ResetCreditsSnapshot, sum ResetCreditSummary) (string, []DetailLine) {
	lc := s.lc
	lang := lc.Language()
	suffix := ""
	if !sum.NextExpiry.IsZero() {
		suffix = " · " + lc.Text(l10n.KeyLeft) + " " + DurationLocalized(sum.NextExpiry.Sub(c.UpdatedAt).Seconds(), lang, true)
	}
	text := strconv.Itoa(c.AvailableCount) + " " + lc.Text(l10n.KeyAvailable) + " / " + strconv.Itoa(len(c.Credits)) + " " + lc.Text(l10n.KeyTotal) + suffix
	lines := []DetailLine{
		{lc.Text(l10n.KeyAvailable), strconv.Itoa(c.AvailableCount)},
		{lc.Text(l10n.KeyTotal), strconv.Itoa(len(c.Credits))},
	}
	for i, cr := range c.Credits {
		if i >= 6 {
			break
		}
		expiry := lc.Text(l10n.KeyNoExpiry)
		if !cr.ExpiresAt.IsZero() {
			expiry = ResetCreditExpiresAt(cr.ExpiresAt, lang, s.opts.Location) + " · " +
				DurationLocalized(cr.ExpiresAt.Sub(c.UpdatedAt).Seconds(), lang, true) + " " + lc.Text(l10n.KeyLeft)
		}
		lines = append(lines, DetailLine{lc.Text(l10n.KeyCredit) + " " + strconv.Itoa(i+1), CreditStatusText(lc, cr.Status) + " · " + expiry})
	}
	return text, lines
}

func (s *Service) costViewLocked(v *CodexView, now time.Time) {
	lc := s.lc
	switch {
	case s.disp != nil:
		d := DisplayCost(lc, *s.disp, s.dispRange, s.costNote, now)
		v.CostText, v.CostSubtitle, v.CostLines = d.Text, d.Subtitle, d.Lines
	case s.costErrText != "":
		d := ClearedCost(lc, s.costErrText)
		v.CostText, v.CostLines = d.Text, d.Lines
	default:
		v.CostText = lc.Text(l10n.KeyNotScanned)
	}
}

func (s *Service) sessionsViewLocked(v *CodexView) {
	lc := s.lc
	switch {
	case s.repairErr != "":
		v.SessionText = lc.Text(l10n.KeyRepairFailed) + ": " + s.repairErr
	case s.repairDone:
		backup := s.repairBackup
		if backup == "" {
			backup = lc.Text(l10n.KeyNoPreviousIndex)
		}
		planned := 0
		if s.report != nil {
			planned = s.report.PlannedEntries
		}
		v.SessionText = fmt.Sprintf("%s %d. %s: %s", lc.Text(l10n.KeyRebuilt), planned, lc.Text(l10n.KeyBackup), backup)
	case s.sessErr != "":
		v.SessionText = lc.Text(l10n.KeySessionScanFailed)
	case s.report != nil:
		r := *s.report
		v.SessionText = fmt.Sprintf("%d %s, %d %s, %d %s", len(r.MissingIndexIDs), lc.Text(l10n.KeyMissing),
			len(r.OrphanIndexIDs), lc.Text(l10n.KeyOrphan), len(r.DuplicateIndexIDs), lc.Text(l10n.KeyDuplicate))
	default:
		v.SessionText = lc.Text(l10n.KeyNotScanned)
	}
	if s.sessErr != "" {
		v.SessionLines = []DetailLine{{lc.Text(l10n.KeyError), s.sessErr}}
	}
	if s.report != nil {
		r := *s.report
		v.SessionReport = &r
		if s.sessErr == "" {
			v.SessionLines = []DetailLine{
				{lc.Text(l10n.KeyPlannedEntries), strconv.Itoa(r.PlannedEntries)},
				{lc.Text(l10n.KeyMissingFromIndex), strconv.Itoa(len(r.MissingIndexIDs))},
				{lc.Text(l10n.KeyOrphanIndexRows), strconv.Itoa(len(r.OrphanIndexIDs))},
				{lc.Text(l10n.KeyDuplicateIndexIDs), strconv.Itoa(len(r.DuplicateIndexIDs))},
				{lc.Text(l10n.KeyStaleTitles), strconv.Itoa(len(r.StaleTitleIDs))},
			}
			if r.BackupPath != "" {
				v.SessionLines = append(v.SessionLines, DetailLine{lc.Text(l10n.KeyBackup), baseName(r.BackupPath)})
			}
		}
	}
	v.Speed, v.SpeedError = s.speed, s.speedErr
	v.RecentSessions = s.recent
	if s.recentErr != "" {
		v.RecentSessionLines = []DetailLine{{lc.Text(l10n.KeyError), s.recentErr}}
		return
	}
	for i, it := range s.recent {
		if i >= defaultRecentSessions {
			break
		}
		amount := "--"
		if it.EstimatedUSD != nil {
			amount = moneyJSON(*it.EstimatedUSD)
		}
		v.RecentSessionLines = append(v.RecentSessionLines, DetailLine{it.ProjectName,
			sessionStateText(lc, it.State) + " · " + CompactNumber(int64(it.Totals.TotalTokens)) + " " + lc.Text(l10n.KeyTokens) + " · " + amount})
	}
}

func moneyJSON(d swiftjson.Decimal) string { return "$" + fmt.Sprintf("%.4f", d.Float64()) }

func sessionStateText(lc l10n.Localizer, st sessions.State) string {
	switch st {
	case sessions.StateNeedsAttention:
		return lc.Text(l10n.KeyNeedsAttention)
	case sessions.StateFailed:
		return lc.Text(l10n.KeyFailed)
	}
	return lc.Text(l10n.KeyRecent)
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}

var _ = store.ProviderCodex
