package runway

import (
	"context"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// Defaults of Options.
const (
	// DefaultFullRefreshTimeout is the watchdog of a full refresh: a hung leg
	// is cancelled so it never pins the refresh pipeline.
	DefaultFullRefreshTimeout = 120 * time.Second
	DefaultTickInterval       = time.Second
	defaultRecentSessions     = 5
	// visibleSectionTTL is how long a session section stays fresh when the
	// panel only asks for a non-forced refresh.
	visibleSectionTTL = 120 * time.Second
)

// Options configures a Service.
type Options struct {
	Sources     Sources
	Preferences store.Preferences

	// File locations; an empty path turns the feature off.
	AlertsPath          string // alerts.json (which alerts were shown)
	EstimateHistoryPath string // quota-estimate-history.json
	CostCachePath       string // api-equivalent-cost.json
	StatusPath          string // status.json (written when ExportsStatusJSON)

	// Grok, Claude, Cursor and ResetToday are optional providers.
	Grok       GrokProvider
	Claude     ClaudeProvider
	Cursor     CursorProvider
	ResetToday ResetTodayProvider

	// WidgetSink receives the widget snapshot when the widget schedule is
	// due; nil disables widgets. WidgetRequirements is the data the active
	// widgets need (zero: no active widgets).
	WidgetSink         func(export.Snapshot) error
	WidgetRequirements export.Requirements

	// OnSnapshot receives every published snapshot, in order, from one
	// goroutine; intermediate snapshots may be skipped when the receiver is
	// slow (the latest always arrives). The UI layer marshals it onto its
	// own thread. OnEvent receives alerts and refresh completions.
	OnSnapshot func(Snapshot)
	OnEvent    func(Event)

	// Now replaces the clock, Location the calendar (cost ranges, labels).
	Now      func() time.Time
	Location *time.Location
	// SystemLanguages are the OS preferred languages, for Language "system".
	SystemLanguages []string

	FullRefreshTimeout time.Duration
	TickInterval       time.Duration
	RecentSessions     int
}

// Service is the Codex-side coordinator. All methods are safe from any
// goroutine. Data flows one way: sources -> Service state -> Snapshot.
type Service struct {
	opts Options
	src  Sources

	root   context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// lifeMu guards closed and tasks (background work started by spawn);
	// alertMu serializes alert dedupe.
	lifeMu  sync.Mutex
	closed  bool
	tasks   int
	alertMu sync.Mutex

	pubMu  sync.Mutex
	pubSig chan struct{}
	latest Snapshot

	mu    sync.Mutex
	seq   uint64
	prefs store.Preferences
	lc    l10n.Localizer

	// epoch is the account-state generation (Swift accountStateGeneration).
	// Every result carries the epoch and the account identity it was fetched
	// for; clearAccountScopedLocked bumps it, so late results are dropped.
	epoch        uint64
	standaloneID uint64
	standalone   map[uint64]accountTask
	auth         *codex.Auth

	quota      *codexapi.QuotaSnapshot
	quotaErr   string
	credits    *codexapi.ResetCreditsSnapshot
	creditsErr string
	// loggedOut marks the state after an authentication failure.
	loggedOut bool
	// statusFail marks a failed refresh before any quota was loaded.
	statusFail bool
	// planHint is the plan of the account just switched to, shown until a
	// quota arrives.
	planHint string

	estDaily    *usage.Summary
	est         *estimate.Snapshot
	estErr      string
	estAt       time.Time
	cost        *usage.Summary
	disp        *usage.Summary
	dispRange   store.CostRange
	costErrText string
	costNote    string
	fullWindow  *usage.Window

	lastCostDone    time.Time
	lastCostCycle   costCycle
	lastHeatDone    time.Time
	detail          detailCache
	detailFlight    map[string]*flight
	heat            HeatmapData
	progress        usage.Progress
	progressCount   int
	progressReports *usage.ProgressReporter

	report       *sessions.Report
	reportAt     time.Time
	sessErr      string
	repairDone   bool
	repairBackup string
	repairErr    string
	recent       []sessions.Item
	recentAt     time.Time
	recentErr    string
	recentSet    bool
	speed        *speed.Report
	speedAt      time.Time
	speedErr     string

	index          codex.AccountIndex
	indexLoaded    bool
	switching      bool
	accountMessage string
	refreshingIDs  map[string]bool
	accountsBusy   bool
	// probes are the relay accounts whose balance is being probed (by id).
	probes map[string]bool

	lastErr string

	busy       map[Section]bool
	allBusy    bool
	fullID     uint64
	fullCancel context.CancelFunc
	fullDone   chan struct{}
	heatID     uint64
	heatCancel context.CancelFunc
	heatDone   chan struct{}

	schedule       Schedule
	widgetSchedule Schedule
	widgetPending  bool
	widgetReqs     export.Requirements
	// panelOpen is set while the main panel shows: only then does the panel's
	// provider refresh its local data. localPending asks the full refresh
	// that was running when the panel opened to do it once it ends.
	panelOpen    bool
	localPending bool
	lastReset    time.Time
	lastTickKey  string
}

// flight is one shared detail cost query.
type flight struct {
	epoch  uint64
	cancel context.CancelFunc
	done   chan struct{}
	sum    usage.Summary
	err    error
}

// New builds a Service. Nothing runs until Start (or a Refresh call).
func New(o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.FullRefreshTimeout <= 0 {
		o.FullRefreshTimeout = DefaultFullRefreshTimeout
	}
	if o.TickInterval <= 0 {
		o.TickInterval = DefaultTickInterval
	}
	if o.RecentSessions <= 0 {
		o.RecentSessions = defaultRecentSessions
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		opts: o, src: o.Sources, root: ctx, cancel: cancel,
		pubSig:         make(chan struct{}, 1),
		prefs:          o.Preferences,
		busy:           map[Section]bool{},
		standalone:     map[uint64]accountTask{},
		refreshingIDs:  map[string]bool{},
		probes:         map[string]bool{},
		detailFlight:   map[string]*flight{},
		schedule:       NewSchedule(),
		widgetSchedule: NewSchedule(),
		widgetReqs:     o.WidgetRequirements,
		progress:       usage.ProgressIdle(),
	}
	s.lc = s.localizer()
	s.progressReports = usage.NewProgressReporter(100*time.Millisecond, s.setProgress)
	s.bootstrap()
	if o.Grok != nil {
		o.Grok.SetOnChange(s.publish)
	}
	if o.Claude != nil {
		o.Claude.SetOnChange(s.publish)
	}
	if o.Cursor != nil {
		o.Cursor.SetOnChange(s.publish)
	}
	if o.ResetToday != nil {
		o.ResetToday.SetOnChange(s.publish)
	}
	return s
}

func (s *Service) localizer() l10n.Localizer {
	return l10n.ForPreference(string(s.prefs.Language), s.opts.SystemLanguages)
}

func (s *Service) now() time.Time { return s.opts.Now() }

// Start launches the publisher and the 1 s tick, and begins the first full
// refresh. It returns immediately; Close stops everything.
func (s *Service) Start(ctx context.Context) {
	s.wg.Add(2)
	go func() { defer s.wg.Done(); s.publishLoop() }()
	go func() { defer s.wg.Done(); s.tickLoop(ctx) }()
	s.publish()
	s.beginFullRefresh(usage.RefreshIfChanged)
}

// Close cancels all work, waits for it and stops publishing.
func (s *Service) Close() {
	s.lifeMu.Lock()
	s.closed = true
	s.lifeMu.Unlock()
	s.cancel()
	s.wg.Wait()
}

// spawn runs fn in a tracked goroutine; false when the Service is closed.
func (s *Service) spawn(fn func()) bool {
	s.lifeMu.Lock()
	if s.closed {
		s.lifeMu.Unlock()
		return false
	}
	s.wg.Add(1)
	s.tasks++
	s.lifeMu.Unlock()
	go func() {
		defer func() {
			s.lifeMu.Lock()
			s.tasks--
			s.lifeMu.Unlock()
			s.wg.Done()
		}()
		fn()
	}()
	return true
}

// run starts fn in the background and returns a channel closed when it is done.
func (s *Service) run(fn func()) <-chan struct{} {
	done := make(chan struct{})
	if !s.spawn(func() { defer close(done); fn() }) {
		close(done)
	}
	return done
}

// WaitIdle blocks until no background work is running (or ctx ends). Tests
// and shutdown paths use it; the UI does not need it.
func (s *Service) WaitIdle(ctx context.Context) error {
	for {
		s.lifeMu.Lock()
		n := s.tasks
		s.lifeMu.Unlock()
		if n == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// Snapshot returns the latest published snapshot.
func (s *Service) Snapshot() Snapshot {
	s.pubMu.Lock()
	defer s.pubMu.Unlock()
	return s.latest
}

func (s *Service) tickLoop(ctx context.Context) {
	t := time.NewTicker(s.opts.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-s.root.Done():
			return
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(s.now())
		}
	}
}

// publishLoop delivers the latest snapshot to OnSnapshot, conflating bursts.
func (s *Service) publishLoop() {
	for {
		select {
		case <-s.root.Done():
			return
		case <-s.pubSig:
			if s.opts.OnSnapshot != nil {
				s.opts.OnSnapshot(s.Snapshot())
			}
		}
	}
}

// publish builds a snapshot of the current state and hands it out.
func (s *Service) publish() {
	s.mu.Lock()
	s.publishLocked()
	s.mu.Unlock()
}

func (s *Service) publishLocked() { s.publishSnapLocked(s.buildLocked(s.now())) }

func (s *Service) publishSnapLocked(snap Snapshot) {
	s.seq++
	snap.Seq = s.seq
	s.pubMu.Lock()
	s.latest = snap
	s.pubMu.Unlock()
	select {
	case s.pubSig <- struct{}{}:
	default:
	}
}

func (s *Service) emit(e Event) {
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}

func (s *Service) setProgress(p usage.Progress) {
	s.mu.Lock()
	s.progress = p
	s.publishLocked()
	s.mu.Unlock()
}

// bootstrap loads the account list, the cached cost and the official auth,
// as the Swift model's init does.
func (s *Service) bootstrap() {
	now := s.now()
	if cache := s.costCache(); cache != nil {
		if l := cache.Load(); l.State == usage.CacheLoaded {
			c := l.Summary
			s.cost = &c
			if s.prefs.APICostSummaryRange == store.CostRangeCurrent {
				s.disp, s.dispRange = &c, store.CostRangeCurrent
			}
		}
	}
	s.reloadIndexLocked()
	_ = now
}

func (s *Service) costCache() *usage.CacheStore {
	if s.opts.CostCachePath == "" {
		return nil
	}
	return &usage.CacheStore{Path: s.opts.CostCachePath}
}

// isCurrentLocked reports whether a result fetched for auth in epoch gen
// still belongs to the account being shown.
func (s *Service) isCurrentLocked(auth codex.Auth, gen uint64) bool {
	if gen != s.epoch || s.auth == nil {
		return false
	}
	return codex.MatchKey(auth) == codex.MatchKey(*s.auth)
}

// clearAccountScopedLocked drops everything bound to the previous account and
// starts a new generation; auth is the credential now in force (nil: none).
func (s *Service) clearAccountScopedLocked(auth *codex.Auth) {
	s.epoch++
	s.clearAccountBusyLocked()
	s.quota, s.quotaErr = nil, ""
	s.credits, s.creditsErr = nil, ""
	s.estDaily, s.est, s.estErr = nil, nil, ""
	s.cost, s.disp, s.costErrText, s.costNote = nil, nil, "", ""
	s.fullWindow = nil
	s.lastCostDone, s.lastCostCycle, s.lastHeatDone = time.Time{}, costCycle{}, time.Time{}
	s.detail.clear()
	for _, f := range s.detailFlight {
		f.cancel()
	}
	s.detailFlight = map[string]*flight{}
	s.heat = HeatmapData{}
	s.loggedOut, s.statusFail, s.planHint = false, false, ""
	s.auth = auth
	if s.src.Generations != nil {
		s.src.Generations.InvalidateAll()
	}
}

// Epoch returns the current account-state generation.
func (s *Service) Epoch() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.epoch
}

// SetPreferences applies changed preferences: relabels, re-arms the refresh
// schedules and fetches what a newly enabled section needs.
func (s *Service) SetPreferences(p store.Preferences) {
	s.mu.Lock()
	old := s.prefs
	hadGrok := s.quotaWantedLocked(store.ProviderGrok)
	hadClaude := s.quotaWantedLocked(store.ProviderClaude)
	hadCursor := s.quotaWantedLocked(store.ProviderCursor)
	s.prefs = p.Clone()
	s.lc = s.localizer()
	// A provider shown again fetches its quota now instead of at the next tick.
	grokBack := !hadGrok && s.quotaWantedLocked(store.ProviderGrok)
	claudeBack := !hadClaude && s.quotaWantedLocked(store.ProviderClaude)
	cursorBack := !hadCursor && s.quotaWantedLocked(store.ProviderCursor)
	now := s.now()
	s.refreshIntervalChangedLocked(now)
	if old.QuotaEstimateWindowMode != p.QuotaEstimateWindowMode && s.est != nil && s.quota != nil && s.estDaily != nil {
		s.applyEstimateLocked(*s.quota, *s.estDaily, false)
	}
	codexSelected := p.SelectedProvider == store.ProviderCodex
	wantEstimate := codexSelected && p.ShowsQuotaEstimateSummary && s.est == nil
	wantResetToday := codexSelected && p.ShowsRateLimitResetToday && s.opts.ResetToday != nil &&
		!s.opts.ResetToday.State(now).HasData
	rangeChanged := old.APICostSummaryRange != p.APICostSummaryRange
	costOn := p.ShowsCostSummary && !old.ShowsCostSummary
	heatOn := p.ShowsTokenUsageHeatmap && !old.ShowsTokenUsageHeatmap
	reportOn := codexSelected && p.ShowsSessionRepairSummary && !old.ShowsSessionRepairSummary
	recentOn := p.ShowsRecentSessions && !old.ShowsRecentSessions
	speedOn := p.ShowsSpeedAnalysis && !old.ShowsSpeedAnalysis
	// Quotas keep their schedule; a switch only loads the local data of the
	// provider the open panel now shows.
	switched := old.SelectedProvider != p.SelectedProvider
	if p.ExportsStatusJSON && !old.ExportsStatusJSON {
		s.exportStatusLocked()
	}
	s.publishLocked()
	s.mu.Unlock()
	if switched {
		s.refreshPanelLocal()
	}
	if grokBack {
		s.refreshGrok()
	}
	if claudeBack {
		s.refreshClaude(false)
	}
	if cursorBack {
		s.RefreshCursor(false)
	}
	if p.SelectedProvider == store.ProviderGrok && (rangeChanged && p.ShowsCostSummary || costOn || heatOn || recentOn || speedOn) {
		s.refreshGrokLocal()
	} else if p.SelectedProvider == store.ProviderClaude && (rangeChanged && p.ShowsCostSummary || costOn || heatOn || recentOn || speedOn) {
		s.refreshClaudeLocal()
	}
	if reportOn {
		s.RefreshSessionReport(true)
	}
	if codexSelected && recentOn {
		s.RefreshRecentSessions(true)
	}
	if codexSelected && speedOn {
		s.RefreshSpeed(true)
	}
	if wantEstimate {
		s.RefreshQuotaEstimate()
	}
	if wantResetToday {
		s.RefreshResetToday(true)
	}
	if codexSelected && (rangeChanged && p.ShowsCostSummary || costOn) {
		s.RefreshCost(usage.RefreshForce)
	}
	if codexSelected && heatOn {
		s.RefreshTokenHeatmap(usage.RefreshForce)
	}
}

// Preferences returns the preferences in force.
func (s *Service) Preferences() store.Preferences {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prefs.Clone()
}

// SelectProvider switches the panel. claude is accepted like codex and grok.
func (s *Service) SelectProvider(p store.Provider) {
	prefs := s.Preferences()
	if prefs.SelectedProvider == p {
		return
	}
	prefs.SelectedProvider = p
	s.SetPreferences(prefs)
}

// SetWidgetRequirements records which data the active widgets need.
func (s *Service) SetWidgetRequirements(r export.Requirements) {
	s.mu.Lock()
	if s.widgetReqs == r {
		s.mu.Unlock()
		return
	}
	s.widgetReqs = r
	s.refreshIntervalChangedLocked(s.now())
	s.mu.Unlock()
	if r != 0 {
		s.beginFullRefreshWidgets(usage.RefreshIfChanged, true)
	}
}

func (s *Service) refreshIntervalChangedLocked(now time.Time) {
	s.schedule.IntervalChanged(s.refreshInterval(), now)
	if s.widgetReqs == 0 {
		s.widgetSchedule = NewSchedule()
		s.widgetPending = false
		return
	}
	s.widgetSchedule.IntervalChanged(s.widgetInterval(), now)
}

func (s *Service) refreshInterval() time.Duration {
	return time.Duration(s.prefs.RefreshIntervalSeconds) * time.Second
}

func (s *Service) widgetInterval() time.Duration {
	return time.Duration(s.prefs.WidgetRefreshIntervalSeconds) * time.Second
}

// alertStore returns the alert dedupe store, nil when disabled.
func (s *Service) alertStore() *alerts.Store {
	if s.opts.AlertsPath == "" {
		return nil
	}
	return &alerts.Store{Path: s.opts.AlertsPath}
}

func (s *Service) historyStore() *estimate.HistoryStore {
	if s.opts.EstimateHistoryPath == "" {
		return nil
	}
	return &estimate.HistoryStore{Path: s.opts.EstimateHistoryPath}
}
