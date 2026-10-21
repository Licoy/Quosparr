package runway

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// ---- section bookkeeping ----------------------------------------------

// enter marks sections as refreshing; false when one already is (Swift
// withRefresh skips the operation then).
func (s *Service) enter(secs ...Section) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range secs {
		if s.busy[x] {
			return false
		}
	}
	for _, x := range secs {
		s.busy[x] = true
	}
	s.publishLocked()
	return true
}

func (s *Service) leave(secs ...Section) {
	s.mu.Lock()
	for _, x := range secs {
		delete(s.busy, x)
	}
	s.publishLocked()
	s.mu.Unlock()
}

// IsRefreshing reports whether a section (or a full refresh) is running.
func (s *Service) IsRefreshing(sec Section) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allBusy || s.busy[sec]
}

// ---- errors and auth state --------------------------------------------

// stopped reports whether an operation was cancelled, as opposed to failed.
func stopped(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled)
}

// errText is the user-facing text of err. Proxy errors are localized; others
// use their own message, which never carries credentials.
func (s *Service) errTextLocked(err error) string {
	if key := CredentialModeErrorKey(err); key != "" {
		return s.lc.Text(key)
	}
	if key := netx.L10nKey(err); key != "" {
		return s.lc.Text(key)
	}
	return err.Error()
}

// humanizeLocked turns an authentication failure into the text shown to the
// user (Swift humanizeAuthError).
func (s *Service) humanizeLocked(err error) string {
	lc := s.lc
	if key := CredentialModeErrorKey(err); key != "" {
		return lc.Text(key)
	}
	var load *AuthLoadError
	if errors.As(err, &load) {
		if load.Kind == AuthExpired {
			return lc.Text(l10n.KeyAuthExpired)
		}
		return lc.Text(l10n.KeyAuthFileInvalid)
	}
	if IsAuthFailure(err) {
		if s.src.OfficialUsability != nil {
			if u, ok := s.src.OfficialUsability(s.now()); ok {
				switch u {
				case codex.InvalidTokens:
					return lc.Text(l10n.KeyAuthFileInvalid)
				case codex.ExpiredAccessWithoutRefresh:
					return lc.Text(l10n.KeyAuthExpired)
				}
			}
		}
		return lc.Text(l10n.KeyAuthExpired)
	}
	return s.errTextLocked(err)
}

// handleAuthFailureLocked clears the account state when err means "sign in
// again". It returns false (and changes nothing) for other errors.
func (s *Service) handleAuthFailureLocked(err error) bool {
	if !IsAuthFailure(err) {
		return false
	}
	msg := s.humanizeLocked(err)
	s.clearAccountScopedLocked(nil)
	s.loggedOut = true
	s.lastErr = msg
	s.exportStatusLocked()
	return true
}

// current reports whether (auth, gen) is still the account being shown.
func (s *Service) current(auth codex.Auth, gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isCurrentLocked(auth, gen)
}

// apply runs fn and publishes when (auth, gen) is still current; stale
// results are dropped.
func (s *Service) apply(auth codex.Auth, gen uint64, fn func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.isCurrentLocked(auth, gen) {
		return false
	}
	fn()
	s.publishLocked()
	return true
}

// fail records the failure of an operation that was bound to (auth, gen).
// auth nil means the failure happened before a credential was loaded. Stale
// failures are dropped; authentication failures sign the account out.
func (s *Service) fail(auth *codex.Auth, gen uint64, err error, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.epoch {
		return
	}
	if auth != nil && !s.isCurrentLocked(*auth, gen) {
		return
	}
	if !s.handleAuthFailureLocked(err) && fn != nil {
		fn()
	}
	s.publishLocked()
}

// loadValidAuth gets a usable credential and records it. It returns the
// generation the credential belongs to; callers tag every result with it.
func (s *Service) loadValidAuth(ctx context.Context, preferCached bool) (codex.Auth, uint64, error) {
	s.mu.Lock()
	start := s.epoch
	var cached *codex.Auth
	if s.auth != nil {
		c := *s.auth
		cached = &c
	}
	s.mu.Unlock()

	auth, err := s.src.LoadValidAuth(ctx, preferCached, cached)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if start != s.epoch || ctx.Err() != nil {
			return codex.Auth{}, start, context.Canceled
		}
		var load *AuthLoadError
		if errors.As(err, &load) {
			msg := s.humanizeLocked(err)
			s.clearAccountScopedLocked(nil)
			s.lastErr = msg
			s.exportStatusLocked()
		} else {
			s.handleAuthFailureLocked(err)
		}
		s.publishLocked()
		return codex.Auth{}, start, err
	}

	s.mu.Lock()
	if start != s.epoch || ctx.Err() != nil {
		s.mu.Unlock()
		return codex.Auth{}, start, context.Canceled
	}
	prevKey, nextKey := "", codex.MatchKey(auth)
	if s.auth != nil {
		prevKey = codex.MatchKey(*s.auth)
	}
	plan := ""
	if prevKey != "" && prevKey == nextKey && s.quota != nil {
		plan = s.quota.Plan
	}
	if prevKey != "" && prevKey != nextKey {
		s.clearAccountScopedLocked(&auth)
	} else {
		a := auth
		s.auth = &a
	}
	gen := s.epoch
	s.mu.Unlock()

	// Mirror the credential into the account library before any account
	// quota result can arrive; never writes the official auth.json.
	if s.src.RecordAuth != nil {
		_ = s.src.RecordAuth(auth, plan, s.now())
	}
	s.mu.Lock()
	if gen == s.epoch {
		s.reloadIndexLocked()
		s.publishLocked()
	}
	s.mu.Unlock()
	return auth, gen, nil
}

// syncOfficial aligns the account list with the official auth.json.
func (s *Service) syncOfficial() {
	if s.src.SyncOfficial == nil {
		return
	}
	idx, err := s.src.SyncOfficial(s.now())
	s.mu.Lock()
	if err == nil || idx.Version != 0 {
		s.index, s.indexLoaded = idx, true
	}
	if err != nil {
		s.accountMessage = s.errTextLocked(err)
	}
	s.publishLocked()
	s.mu.Unlock()
}

// CredentialModeErrorKey shares the safe credential-mode message with account UI actions.
func CredentialModeErrorKey(err error) l10n.Key {
	switch {
	case errors.Is(err, codex.ErrCredentialStoreUnsupported):
		return l10n.KeyCodexCredentialStoreUnsupported
	case errors.Is(err, codex.ErrCredentialConfigInvalid):
		return l10n.KeyCodexCredentialConfigInvalid
	}
	return ""
}

// ---- full refresh -------------------------------------------------------

// Refresh starts a full refresh and returns a channel closed when it is done:
// every provider's quota, plus the local data of the provider the open panel
// shows. A refresh already running is joined; single sections refreshing on
// their own never hold it back. RefreshForce is a manual refresh.
func (s *Service) Refresh(policy usage.RefreshPolicy) <-chan struct{} {
	return s.beginFullRefreshWidgets(policy, false)
}

func (s *Service) beginFullRefresh(policy usage.RefreshPolicy) <-chan struct{} {
	return s.beginFullRefreshWidgets(policy, false)
}

func closedChan() <-chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}

func (s *Service) providerWantedLocked(p store.Provider) bool {
	if s.prefs.SelectedProvider == p {
		return true
	}
	if slices.Contains(s.prefs.StatusBarSelection(), p) {
		return true
	}
	return s.prefs.FloatingWindow.Enabled && slices.Contains(s.prefs.FloatingSelection(), p)
}

// showsProviderLocked reports whether the panel, the status item or a widget
// shows the provider in full. Only a shown provider reports its errors.
func (s *Service) showsProviderLocked(p store.Provider) bool {
	return s.providerWantedLocked(p) || (p != store.ProviderCursor && s.widgetNeedsLocked())
}

// quotaWantedLocked reports whether the provider's quota refreshes: it is
// shown, or it has a tab in the panel's provider strip. A provider the panel
// hides and nothing else shows is not fetched.
func (s *Service) quotaWantedLocked(p store.Provider) bool {
	if p == store.ProviderCursor && !s.prefs.CursorMonitoringEnabled {
		return false
	}
	return s.showsProviderLocked(p) || slices.Contains(s.prefs.PanelSelection(), p)
}

func (s *Service) widgetNeedsLocked() bool {
	r := s.widgetReqs
	return r.Has(export.RequireProviderQuota) || r.Has(export.RequireTokenTrend) || r.Has(export.RequireCost)
}

// refreshGrok refreshes the Grok provider in the background.
func (s *Service) refreshGrok() {
	if s.opts.Grok == nil {
		return
	}
	g := s.opts.Grok
	s.spawn(func() {
		_ = g.Refresh(s.root)
		s.publish()
	})
}

func (s *Service) refreshClaude(force bool) {
	if s.opts.Claude == nil {
		return
	}
	c := s.opts.Claude
	s.spawn(func() {
		_ = c.Refresh(s.root, force)
		s.publish()
	})
}

func (s *Service) refreshGrokLocal() {
	if s.opts.Grok == nil {
		return
	}
	g := s.opts.Grok
	s.spawn(func() {
		_ = g.RefreshLocal(s.root)
		s.publish()
	})
}

func (s *Service) refreshClaudeLocal() {
	if s.opts.Claude == nil {
		return
	}
	c := s.opts.Claude
	s.spawn(func() {
		_ = c.RefreshLocal(s.root)
		s.publish()
	})
}

// ---- local data ----------------------------------------------------------------

// SetPanelOpen tells the service whether the main panel shows. Opening it
// loads the local data of the panel's provider; quotas keep their schedule.
func (s *Service) SetPanelOpen(open bool) {
	s.mu.Lock()
	opened := open && !s.panelOpen
	s.panelOpen = open
	if !open {
		s.localPending = false
	}
	s.mu.Unlock()
	if opened {
		s.refreshPanelLocal()
	}
}

// refreshPanelLocal refreshes the local data of the panel's provider: cost,
// token chart, sessions, speed and, for Codex, the quota estimate. It never
// fetches a quota, and data still fresh within the refresh interval is kept.
// While a full refresh runs it waits for that one to end.
func (s *Service) refreshPanelLocal() {
	s.mu.Lock()
	if !s.panelOpen {
		s.mu.Unlock()
		return
	}
	if s.allBusy {
		s.localPending = true
		s.mu.Unlock()
		return
	}
	p := s.prefs
	estimateDue := p.ShowsQuotaEstimateSummary && (s.est == nil || !s.now().Before(s.estAt.Add(s.refreshInterval())))
	s.mu.Unlock()
	switch p.SelectedProvider {
	case store.ProviderCursor:
		return
	case store.ProviderGrok:
		s.refreshGrokLocal()
	case store.ProviderClaude:
		s.refreshClaudeLocal()
	default:
		if p.ShowsSessionRepairSummary {
			s.RefreshSessionReport(false)
		}
		if p.ShowsRecentSessions {
			s.RefreshRecentSessions(false)
		}
		if p.ShowsSpeedAnalysis {
			s.RefreshSpeed(false)
		}
		if estimateDue {
			s.RefreshQuotaEstimate()
		}
		if p.ShowsCostSummary {
			s.RefreshCost(usage.RefreshIfChanged)
		}
		if p.ShowsTokenUsageHeatmap {
			s.RefreshTokenHeatmap(usage.RefreshIfChanged)
		}
	}
}

// RefreshProviderLocal is the manual refresh of a Grok or Claude section
// built from the local sessions; it leaves the quota to its schedule.
func (s *Service) RefreshProviderLocal(p store.Provider) {
	switch p {
	case store.ProviderGrok:
		s.refreshGrokLocal()
	case store.ProviderClaude:
		s.refreshClaudeLocal()
	}
}

// panelLocalLocked reports whether the open panel shows p's local data.
func (s *Service) panelLocalLocked(p store.Provider) bool {
	return s.panelOpen && s.prefs.SelectedProvider == p
}

// widgetLocalLocked reports whether a widget write needs the local cost or
// token chart of the providers it shows.
func (s *Service) widgetLocalLocked() bool {
	return s.widgetReqs.Has(export.RequireCost) || s.widgetReqs.Has(export.RequireTokenTrend)
}

func (s *Service) beginFullRefreshWidgets(policy usage.RefreshPolicy, widgets bool) <-chan struct{} {
	return s.startFull(policy, widgets)
}

func (s *Service) fullRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allBusy
}

// startFull launches the full refresh goroutine (or joins the running one).
func (s *Service) startFull(policy usage.RefreshPolicy, widgets bool) <-chan struct{} {
	s.mu.Lock()
	if s.allBusy {
		d := s.fullDone
		s.mu.Unlock()
		if d == nil {
			return closedChan()
		}
		return d
	}
	ctx, cancel := context.WithTimeout(s.root, s.opts.FullRefreshTimeout)
	s.fullID++
	id := s.fullID
	done := make(chan struct{})
	s.allBusy, s.fullCancel, s.fullDone = true, cancel, done
	s.schedule.RefreshStarted()
	if widgets {
		s.widgetPending = true
		s.widgetSchedule.RefreshStarted()
	}
	s.publishLocked()
	s.mu.Unlock()

	ok := s.spawn(func() {
		defer close(done)
		s.mu.Lock()
		showsGrok := s.showsProviderLocked(store.ProviderGrok)
		showsClaude := s.showsProviderLocked(store.ProviderClaude)
		wantsGrok := s.quotaWantedLocked(store.ProviderGrok)
		wantsClaude := s.quotaWantedLocked(store.ProviderClaude)
		wantsCursor := s.quotaWantedLocked(store.ProviderCursor)
		// Local data is the open panel's provider only, or every provider
		// when this refresh carries a widget write that needs it.
		widgetLocal := widgets && s.widgetLocalLocked()
		local := localScope{
			panel:      s.panelLocalLocked(store.ProviderCodex),
			widgetCost: widgets && s.widgetReqs.Has(export.RequireCost),
			widgetHeat: widgets && s.widgetReqs.Has(export.RequireTokenTrend),
		}
		grokLocal := widgetLocal || s.panelLocalLocked(store.ProviderGrok)
		claudeLocal := widgetLocal || s.panelLocalLocked(store.ProviderClaude)
		s.mu.Unlock()
		report := func(err error, shown bool) {
			if err != nil && ctx.Err() == nil && shown {
				s.mu.Lock()
				s.lastErr = s.errTextLocked(err)
				s.publishLocked()
				s.mu.Unlock()
			}
		}
		if g := s.opts.Grok; g != nil {
			if wantsGrok {
				report(g.Refresh(ctx), showsGrok)
			}
			if grokLocal {
				report(g.RefreshLocal(ctx), showsGrok)
			}
		}
		if c := s.opts.Claude; c != nil {
			if wantsClaude {
				report(c.Refresh(ctx, policy == usage.RefreshForce), showsClaude)
			}
			if claudeLocal {
				report(c.RefreshLocal(ctx), showsClaude)
			}
		}
		// Only Cursor's own manual refresh may skip its minimum interval.
		if c := s.opts.Cursor; c != nil && wantsCursor {
			_ = c.Refresh(ctx, false)
		}
		s.refreshNow(ctx, policy, local)
		cancel()
		s.finishFull(id)
	})
	if !ok {
		cancel()
		s.finishFull(id)
		close(done)
	}
	return done
}

// finishFull ends a full refresh: reschedules, writes the widget snapshot
// when it is due and tells the UI.
func (s *Service) finishFull(id uint64) {
	s.mu.Lock()
	if s.fullID != id {
		s.mu.Unlock()
		return
	}
	s.allBusy, s.fullCancel, s.fullDone = false, nil, nil
	localPending := s.localPending
	s.localPending = false
	now := s.now()
	s.schedule.RefreshCompleted(now, s.refreshInterval())
	hasWidgets := s.widgetReqs != 0 && s.opts.WidgetSink != nil
	writeWidgets := hasWidgets && (s.widgetPending || s.widgetSchedule.IsDue(now))
	s.widgetPending = false
	if writeWidgets {
		s.widgetSchedule.RefreshCompleted(now, s.widgetInterval())
	}
	snap := s.buildLocked(now)
	s.publishSnapLocked(snap)
	s.mu.Unlock()
	if writeWidgets {
		s.writeWidget(snap, now)
	}
	if localPending {
		s.refreshPanelLocal()
	}
	s.emit(Event{Kind: EventFullRefreshDone})
}

// cancelAccountScoped cancels every account-scoped request and joins full
// and heatmap work; standalone/detail work may return late and stays tracked
// until Close, while epochs prevent it from touching the new selection.
func (s *Service) cancelAccountScoped() {
	s.mu.Lock()
	fc, fd := s.fullCancel, s.fullDone
	hc, hd := s.heatCancel, s.heatDone
	for _, task := range s.standalone {
		task.cancel()
	}
	for _, f := range s.detailFlight {
		f.cancel()
	}
	s.mu.Unlock()
	if fc != nil {
		fc()
	}
	if hc != nil {
		hc()
	}
	if fd != nil {
		<-fd
	}
	if hd != nil {
		<-hd
	}
}

// localScope is the Codex local data a full refresh owes: everything the
// panel shows while it is open on Codex, and the cost or token chart a widget
// write needs.
type localScope struct {
	panel      bool
	widgetCost bool
	widgetHeat bool
}

// refreshNow is the Codex part of a full refresh (Swift refreshNow): the
// accounts, quota and reset credits, then the local data in scope.
func (s *Service) refreshNow(ctx context.Context, policy usage.RefreshPolicy, local localScope) {
	s.syncOfficial()
	prefs := s.Preferences()

	var sessions sync.WaitGroup
	defer sessions.Wait()
	if local.panel && prefs.ShowsSessionRepairSummary {
		sessions.Add(1)
		go func() { defer sessions.Done(); s.loadSessionReport(ctx) }()
	}
	if local.panel && prefs.ShowsRecentSessions {
		sessions.Add(1)
		go func() { defer sessions.Done(); s.loadRecentSessions(ctx) }()
	}
	if local.panel && prefs.ShowsSpeedAnalysis {
		sessions.Add(1)
		go func() { defer sessions.Done(); s.loadSpeed(ctx) }()
	}

	start := s.Epoch()
	auth, gen, err := s.loadValidAuth(ctx, false)
	// Account polling starts once the current credential is recorded, and
	// also when loading it failed.
	// The current relay account is probed by this refresh itself.
	skip := ""
	if err == nil && auth.IsAPIKey() {
		skip = s.currentRelayID()
	}
	s.spawn(func() { s.refreshAccountsInline(s.root, skip) })

	var remote error
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		if start == s.epoch {
			s.statusFail = true
			s.publishLocked()
		}
		s.mu.Unlock()
		remote = err
		gen = start
	} else if auth.IsAPIKey() {
		s.refreshKeyMode(ctx, auth, gen, policy, local)
	} else {
		remote = s.refreshLegs(ctx, auth, gen, policy, prefs, local)
	}
	sessions.Wait()
	if ctx.Err() != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.epoch {
		return
	}
	if remote != nil {
		s.handleAuthFailureLocked(remote)
		s.exportStatusLocked()
		s.lastErr = s.humanizeLocked(remote)
	} else {
		if s.auth != nil {
			s.lastErr = ""
		}
		s.exportStatusLocked()
	}
	s.publishLocked()
}

// refreshLegs runs quota and reset credits in parallel, then the estimate and
// the cost scans, and returns the error to report (nil when all fine).
func (s *Service) refreshLegs(ctx context.Context, auth codex.Auth, gen uint64, policy usage.RefreshPolicy, prefs store.Preferences, local localScope) error {
	var (
		wg      sync.WaitGroup
		quota   codexapi.QuotaSnapshot
		qErr    error
		credErr error
	)
	wg.Add(2)
	go func() { defer wg.Done(); quota, qErr = s.fetchQuotaStep(ctx, auth, gen) }()
	go func() { defer wg.Done(); credErr = s.fetchCreditsStep(ctx, auth, gen) }()
	wg.Wait()

	var remote error
	if qErr == nil {
		if local.panel && prefs.ShowsQuotaEstimateSummary {
			s.loadEstimate(ctx, auth, gen, quota)
		}
		s.scanAfterQuota(ctx, auth, gen, quota, policy, local)
	} else if !stopped(ctx, qErr) {
		remote = qErr
	}
	if credErr != nil && !stopped(ctx, credErr) && (remote == nil || IsAuthFailure(credErr)) {
		remote = credErr
	}
	return remote
}

// scanAfterQuota runs the cost / heatmap scans a full refresh owes.
func (s *Service) scanAfterQuota(ctx context.Context, auth codex.Auth, gen uint64, quota codexapi.QuotaSnapshot, policy usage.RefreshPolicy, local localScope) {
	if !s.current(auth, gen) {
		return
	}
	now := s.now()
	s.mu.Lock()
	p := s.prefs
	needsCost := (local.panel && p.ShowsCostSummary || local.widgetCost) &&
		shouldRefreshCost(policy, s.lastCostDone, s.lastCostCycle, &quota, s.refreshInterval(), now)
	needsHeat := (local.panel && p.ShowsTokenUsageHeatmap || local.widgetHeat) &&
		shouldRefreshHeatmap(policy, s.lastHeatDone, s.refreshInterval(), now)
	s.mu.Unlock()
	if !needsCost && !needsHeat {
		return
	}
	var secs []Section
	if needsCost {
		secs = append(secs, SectionAPICost)
	}
	if needsHeat {
		secs = append(secs, SectionTokenHeatmap)
	}
	if !s.enter(secs...) {
		return
	}
	defer s.leave(secs...)
	s.scanCostAndHeatmap(ctx, quota, auth, gen, policy, needsCost, needsHeat)
	done := s.now()
	s.mu.Lock()
	if s.isCurrentLocked(auth, gen) {
		if needsCost {
			s.lastCostDone, s.lastCostCycle = done, cycleOf(quota)
		}
		if needsHeat {
			s.lastHeatDone = done
		}
	}
	s.mu.Unlock()
}

// ---- quota, credits, estimate ---------------------------------------------

func toAlertWindow(w codexapi.RateWindow) alerts.Window {
	out := alerts.Window{UsedPercent: w.UsedPercentExact}
	if !w.ResetsAt.IsZero() {
		t := w.ResetsAt
		out.ResetsAt = &t
	}
	return out
}

func quotaAlertInput(q codexapi.QuotaSnapshot) alerts.QuotaInput {
	in := alerts.QuotaInput{Primary: toAlertWindow(q.Primary)}
	if q.Secondary != nil {
		w := toAlertWindow(*q.Secondary)
		in.Secondary = &w
	}
	for _, a := range q.AdditionalWindows {
		in.Additional = append(in.Additional, alerts.NamedWindow{Name: a.Name, Window: toAlertWindow(a.Window)})
	}
	return in
}

func creditAlertInput(c codexapi.ResetCreditsSnapshot) []alerts.ResetCredit {
	out := make([]alerts.ResetCredit, 0, len(c.Credits))
	for _, cr := range c.Credits {
		ac := alerts.ResetCredit{Status: cr.Status, RemainingSeconds: cr.RemainingSeconds}
		if cr.ID != "" {
			id := cr.ID
			ac.ID = &id
		}
		if !cr.ExpiresAt.IsZero() {
			t := cr.ExpiresAt
			ac.ExpiresAt = &t
		}
		out = append(out, ac)
	}
	return out
}

// deliver dedupes candidate alerts against alerts.json and hands the new ones
// to OnEvent. Called without s.mu.
func (s *Service) deliver(candidates []alerts.Alert, enabled bool) {
	if !enabled || len(candidates) == 0 {
		return
	}
	st := s.alertStore()
	if st == nil {
		s.emit(Event{Kind: EventAlerts, Alerts: candidates})
		return
	}
	s.alertMu.Lock()
	unseen, err := st.Unseen(candidates)
	s.alertMu.Unlock()
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.publishLocked()
		s.mu.Unlock()
		return
	}
	if len(unseen) > 0 {
		s.emit(Event{Kind: EventAlerts, Alerts: unseen})
	}
}

// fetchQuotaStep is the quota leg of a full refresh.
func (s *Service) fetchQuotaStep(ctx context.Context, auth codex.Auth, gen uint64) (codexapi.QuotaSnapshot, error) {
	if !s.enter(SectionQuota) {
		return codexapi.QuotaSnapshot{}, context.Canceled
	}
	defer s.leave(SectionQuota)
	if !s.current(auth, gen) {
		return codexapi.QuotaSnapshot{}, context.Canceled
	}
	q, err := s.src.FetchQuota(ctx, apiAuth(auth))
	if err != nil {
		if stopped(ctx, err) {
			return q, context.Canceled
		}
		s.fail(&auth, gen, err, func() { s.quotaErr = s.errTextLocked(err) })
		return q, err
	}
	ok := s.apply(auth, gen, func() {
		s.quota, s.quotaErr, s.statusFail, s.loggedOut = &q, "", false, false
	})
	if !ok {
		return q, context.Canceled
	}
	s.deliverQuota(auth, gen, q)
	return q, nil
}

// fetchCreditsStep is the reset-credits leg of a full refresh; it returns the
// error to report.
func (s *Service) fetchCreditsStep(ctx context.Context, auth codex.Auth, gen uint64) error {
	if !s.enter(SectionResetCredits) {
		return nil
	}
	defer s.leave(SectionResetCredits)
	if !s.current(auth, gen) {
		return nil
	}
	c, err := s.src.FetchResetCredits(ctx, apiAuth(auth))
	if err != nil {
		if stopped(ctx, err) {
			return nil
		}
		s.fail(&auth, gen, err, func() { s.creditsErr = s.errTextLocked(err) })
		return err
	}
	var enabled bool
	s.apply(auth, gen, func() {
		s.credits, s.creditsErr = &c, ""
		enabled = s.prefs.ResetCreditAlertsEnabled
	})
	s.deliver(alerts.ResetCreditAlerts(creditAlertInput(c)), enabled)
	return nil
}

// loadEstimate fetches the analytics rows and computes the quota estimate.
func (s *Service) loadEstimate(ctx context.Context, auth codex.Auth, gen uint64, quota codexapi.QuotaSnapshot) {
	if auth.IsAPIKey() || !s.enter(SectionQuotaEstimate) {
		return
	}
	defer s.leaveAccount(gen, SectionQuotaEstimate)
	now := s.now()
	start, end, from, to := estimate.AnalyticsRange(now)
	daily, err := s.src.FetchDailyUsage(ctx, apiAuth(auth), start, end, usage.Window{Start: from, End: to}, now)
	if err != nil {
		if stopped(ctx, err) {
			return
		}
		s.fail(&auth, gen, err, func() { s.estErr = s.errTextLocked(err) })
		return
	}
	s.apply(auth, gen, func() {
		s.estDaily = &daily
		s.applyEstimateLocked(quota, daily, true)
		s.estErr, s.estAt = "", s.now()
	})
}

func estimateWindow(w codexapi.RateWindow) estimate.Window {
	var mins *int
	if w.WindowMinutes > 0 {
		m := w.WindowMinutes
		mins = &m
	}
	var reset *time.Time
	if !w.ResetsAt.IsZero() {
		t := w.ResetsAt
		reset = &t
	}
	out := estimate.NewWindow(w.UsedPercent, mins, reset)
	if w.UsedPercentExact != 0 {
		out.UsedPercentExact = w.UsedPercentExact
	}
	return out
}

func estimateDailyRows(rows []usage.DailyRow) []estimate.DailyRow {
	out := make([]estimate.DailyRow, 0, len(rows))
	for _, r := range rows {
		d := estimate.DailyRow{
			Date: r.Date, RawCredits: r.RawCredits,
			TotalTokens: int(r.Totals.TotalTokens), Turns: int(r.Totals.Turns),
			CreditsReported: r.CreditsReported != nil && *r.CreditsReported,
			TotalsReported:  r.TotalsReported != nil && *r.TotalsReported,
		}
		if r.EstimatedUSD != nil {
			f := decimalFloat(*r.EstimatedUSD)
			d.EstimatedUSD = &f
		}
		out = append(out, d)
	}
	return out
}

// applyEstimateLocked recomputes the estimate; with persist it also records
// the sample in the history file.
func (s *Service) applyEstimateLocked(q codexapi.QuotaSnapshot, daily usage.Summary, persist bool) {
	key := "unknown"
	if s.auth != nil {
		key = codex.MatchKey(*s.auth)
	}
	hs := s.historyStore()
	var history []estimate.HistorySample
	if !persist && s.est != nil {
		history = s.est.History
	} else if hs != nil {
		history = hs.Load(key)
	}
	eq := estimate.Quota{Primary: estimateWindow(q.Primary)}
	if q.Secondary != nil {
		w := estimateWindow(*q.Secondary)
		eq.Secondary = &w
	}
	snap := estimate.Make(eq, estimateDailyRows(daily.DailyRows),
		estimate.ParseWindowMode(string(s.prefs.QuotaEstimateWindowMode)), history, daily.CalculatedAt)
	if persist && hs != nil {
		if sample := estimate.SampleFrom(snap); sample != nil {
			if merged, err := hs.Upsert(key, *sample); err == nil {
				snap.History = merged
			}
		}
	}
	s.est = &snap
}

// ---- single-section refreshes ----------------------------------------------

// RefreshQuota is the manual refresh of every provider's quota. It restarts
// the automatic interval, so the next scheduled refresh is a full interval
// away; a full refresh already running stands in for it.
func (s *Service) RefreshQuota() <-chan struct{} {
	s.mu.Lock()
	busy := s.allBusy
	if !busy {
		s.schedule.RefreshCompleted(s.now(), s.refreshInterval())
	}
	wantsGrok := s.quotaWantedLocked(store.ProviderGrok)
	wantsClaude := s.quotaWantedLocked(store.ProviderClaude)
	s.mu.Unlock()
	if busy {
		return closedChan()
	}
	if wantsGrok {
		s.refreshGrok()
	}
	if wantsClaude {
		s.refreshClaude(true)
	}
	s.RefreshCursor(false)
	return s.runAccountScoped(s.refreshQuotaNow)
}

func (s *Service) refreshQuotaNow(ctx context.Context) {
	busyEpoch := s.Epoch()
	if !s.enter(SectionQuota) {
		return
	}
	auth, gen, err := s.loadValidAuth(ctx, false)
	if err != nil {
		s.leaveAccount(busyEpoch, SectionQuota)
		return
	}
	if auth.IsAPIKey() {
		s.refreshKeyQuota(ctx, auth, gen)
		s.leaveAccount(busyEpoch, SectionQuota)
		return
	}
	q, err := s.src.FetchQuota(ctx, apiAuth(auth))
	if err != nil {
		if !stopped(ctx, err) {
			s.fail(&auth, gen, err, func() {
				s.quotaErr = s.errTextLocked(err)
				s.lastErr = s.quotaErr
			})
		}
		s.leaveAccount(busyEpoch, SectionQuota)
		return
	}
	ok := s.apply(auth, gen, func() {
		s.quota, s.quotaErr, s.statusFail, s.loggedOut, s.lastErr = &q, "", false, false, ""
		s.exportStatusLocked()
	})
	s.leaveAccount(busyEpoch, SectionQuota)
	if !ok {
		return
	}
	s.deliverQuota(auth, gen, q)
	s.mu.Lock()
	want := s.prefs.ShowsQuotaEstimateSummary && s.panelLocalLocked(store.ProviderCodex) && s.quota != nil && s.auth != nil
	s.mu.Unlock()
	if want {
		s.loadEstimate(ctx, auth, gen, q)
	}
}

// RefreshResetCredits refreshes the reset credits alone.
func (s *Service) RefreshResetCredits() <-chan struct{} {
	if s.fullRunning() {
		return closedChan()
	}
	return s.runAccountScoped(s.refreshCreditsNow)
}

func (s *Service) refreshCreditsNow(ctx context.Context) {
	busyEpoch := s.Epoch()
	if !s.enter(SectionResetCredits) {
		return
	}
	defer s.leaveAccount(busyEpoch, SectionResetCredits)
	auth, gen, err := s.loadValidAuth(ctx, true)
	if err != nil || auth.IsAPIKey() {
		return
	}
	c, err := s.src.FetchResetCredits(ctx, apiAuth(auth))
	if err != nil {
		if !stopped(ctx, err) {
			s.fail(&auth, gen, err, func() {
				s.creditsErr = s.errTextLocked(err)
				s.lastErr = s.creditsErr
			})
		}
		return
	}
	var enabled bool
	if s.apply(auth, gen, func() {
		s.credits, s.creditsErr, s.lastErr = &c, "", ""
		enabled = s.prefs.ResetCreditAlertsEnabled
		s.exportStatusLocked()
	}) {
		s.deliver(alerts.ResetCreditAlerts(creditAlertInput(c)), enabled)
	}
}

// RefreshQuotaEstimate refreshes the quota estimate alone.
func (s *Service) RefreshQuotaEstimate() <-chan struct{} {
	s.mu.Lock()
	skip := !s.prefs.ShowsQuotaEstimateSummary || s.allBusy || s.busy[SectionQuotaEstimate] || s.keyModeLocked()
	s.mu.Unlock()
	if skip {
		return closedChan()
	}
	return s.runAccountScoped(s.refreshEstimateNow)
}

func (s *Service) refreshEstimateNow(ctx context.Context) {
	start := s.Epoch()
	auth, gen, err := s.loadValidAuth(ctx, true)
	if err != nil {
		if !stopped(ctx, err) {
			s.fail(nil, start, err, func() { s.estErr = s.errTextLocked(err) })
		}
		return
	}
	if auth.IsAPIKey() {
		return
	}
	s.mu.Lock()
	var q *codexapi.QuotaSnapshot
	if s.quota != nil {
		c := *s.quota
		q = &c
	}
	s.mu.Unlock()
	if q == nil {
		fetched, err := s.src.FetchQuota(ctx, apiAuth(auth))
		if err != nil {
			if !stopped(ctx, err) {
				s.fail(&auth, gen, err, func() { s.estErr = s.errTextLocked(err) })
			}
			return
		}
		if !s.apply(auth, gen, func() { s.quota, s.quotaErr, s.statusFail = &fetched, "", false }) {
			return
		}
		q = &fetched
	}
	s.loadEstimate(ctx, auth, gen, *q)
}

// resetTodayWantedLocked reports whether the reset-today feed polls in the
// background: its module shows, its alerts are on or a widget shows it,
// whichever provider the panel shows.
func (s *Service) resetTodayWantedLocked() bool {
	return s.prefs.ShowsRateLimitResetToday || s.prefs.RateLimitResetTodayAlertsEnabled ||
		s.widgetReqs.Has(export.RequireResetToday)
}

// RefreshResetToday refreshes the reset-today provider (a no-op without one
// or when nothing shows or alerts it).
func (s *Service) RefreshResetToday(force bool) <-chan struct{} {
	rt := s.opts.ResetToday
	s.mu.Lock()
	skip := rt == nil || !s.resetTodayWantedLocked() || s.busy[SectionResetToday]
	s.mu.Unlock()
	if skip {
		return closedChan()
	}
	return s.run(func() {
		if !s.enter(SectionResetToday) {
			return
		}
		err := rt.Refresh(s.root, force)
		s.leave(SectionResetToday)
		if err != nil && s.root.Err() != nil {
			return
		}
		s.mu.Lock()
		enabled := s.prefs.RateLimitResetTodayAlertsEnabled
		s.mu.Unlock()
		s.deliver(rt.PendingAlerts(s.now()), enabled)
	})
}

// ---- sessions ---------------------------------------------------------------

func (s *Service) stale(at time.Time) bool {
	return at.IsZero() || s.now().Sub(at) >= visibleSectionTTL
}

// RefreshSessionReport re-reads the session index diagnosis; without force it
// is skipped while the last result is fresh.
func (s *Service) RefreshSessionReport(force bool) <-chan struct{} {
	s.mu.Lock()
	skip := s.allBusy || (!force && !s.stale(s.reportAt))
	s.mu.Unlock()
	if skip {
		return closedChan()
	}
	return s.run(func() { s.loadSessionReport(s.root) })
}

func (s *Service) loadSessionReport(ctx context.Context) {
	if s.src.DryRunSessions == nil || !s.enter(SectionSessionRepair) {
		return
	}
	defer s.leave(SectionSessionRepair)
	r, err := s.src.DryRunSessions(ctx)
	if stopped(ctx, err) && err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.sessErr = err.Error()
	} else {
		s.report, s.sessErr, s.reportAt = &r, "", s.now()
		s.repairDone, s.repairErr, s.repairBackup = false, "", ""
	}
	s.publishLocked()
}

// RepairSessions rebuilds session_index.jsonl (the repairer backs it up
// first) and reports the outcome in the snapshot.
func (s *Service) RepairSessions() <-chan struct{} {
	s.mu.Lock()
	skip := s.allBusy
	s.mu.Unlock()
	if skip || s.src.RepairSessions == nil {
		return closedChan()
	}
	return s.run(func() {
		if !s.enter(SectionSessionRepair) {
			return
		}
		defer s.leave(SectionSessionRepair)
		r, err := s.src.RepairSessions(s.root)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			s.repairDone, s.repairErr = false, err.Error()
		} else {
			s.report, s.sessErr, s.reportAt = &r, "", s.now()
			s.repairDone, s.repairErr, s.repairBackup = true, "", baseName(r.BackupPath)
		}
		s.publishLocked()
	})
}

// RefreshRecentSessions re-reads the recent sessions list.
func (s *Service) RefreshRecentSessions(force bool) <-chan struct{} {
	s.mu.Lock()
	skip := s.allBusy || (!force && !s.stale(s.recentAt))
	s.mu.Unlock()
	if skip {
		return closedChan()
	}
	return s.run(func() {
		s.loadRecentSessions(s.root)
		s.mu.Lock()
		s.exportStatusLocked()
		s.mu.Unlock()
	})
}

func (s *Service) loadRecentSessions(ctx context.Context) {
	if s.src.ScanRecentSessions == nil || !s.enter(SectionRecentSessions) {
		return
	}
	defer s.leave(SectionRecentSessions)
	sum, err := s.src.ScanRecentSessions(ctx, s.opts.RecentSessions)
	if stopped(ctx, err) && err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.recent, s.recentErr = nil, err.Error()
	} else {
		s.recent, s.recentErr, s.recentAt = sum.Items, "", s.now()
	}
	s.publishLocked()
}

// RefreshSpeed re-measures the model speed from the newest local sessions;
// without force it is skipped while the last result is fresh.
func (s *Service) RefreshSpeed(force bool) <-chan struct{} {
	s.mu.Lock()
	skip := s.allBusy || (!force && !s.stale(s.speedAt))
	s.mu.Unlock()
	if skip {
		return closedChan()
	}
	return s.run(func() { s.loadSpeed(s.root) })
}

func (s *Service) loadSpeed(ctx context.Context) {
	if s.src.ScanSpeed == nil || !s.enter(SectionSpeed) {
		return
	}
	defer s.leave(SectionSpeed)
	r, err := s.src.ScanSpeed(ctx, s.now())
	if stopped(ctx, err) && err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.speed, s.speedErr = nil, err.Error()
	} else {
		s.speed, s.speedErr, s.speedAt = &r, "", s.now()
	}
	s.publishLocked()
}
