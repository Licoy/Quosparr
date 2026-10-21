package app

import (
	"context"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/alerts"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/estimate"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/resettoday"
	"github.com/licoy/quosparr/internal/resettoday/runwayfmt"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/sqliteruntime"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// BackgroundOptions is what StartBackground needs from main.
type BackgroundOptions struct {
	Home     string
	Settings *Settings
	Notifier *Notifier
	Version  string
	Host     *App
}

// StartBackground builds one coordinator and starts it. The returned func
// stops it. A missing SQLite library disables local cost scans only.
func StartBackground(ctx context.Context, opts BackgroundOptions) func() {
	rt := newRuntime(opts)
	setLive(rt)
	var unsubscribe func()
	if opts.Settings != nil {
		unsubscribe = opts.Settings.OnChange(func() { rt.pushPreferences(opts.Settings) })
	}
	if opts.Host != nil {
		opts.Host.SetPanelActions(rt.panelActions(opts.Settings))
	}
	rt.svc.Start(ctx)
	rt.widgets.refresh()
	return func() {
		if unsubscribe != nil {
			unsubscribe()
		}
		rt.stop()
	}
}

// LiveService is the running coordinator, or nil before it starts.
func LiveService() *runway.Service {
	liveMu.Lock()
	defer liveMu.Unlock()
	if liveRT == nil {
		return nil
	}
	return liveRT.svc
}

// UseProxy installs the save and test hooks. reapply runs after a saved
// proxy is installed (the updater, when it exists).
func UseProxy(s *Settings, reapply func()) {
	s.SetProxyHooks(ProxyHooks{
		Applied: func(cfg store.NetworkProxyConfiguration, creds *ProxyCredentials) {
			s.proxyErr = ApplySavedProxy(cfg, creds)
			if reapply != nil {
				reapply()
			}
		},
		Test: TestProxyDraft,
	})
}

// InstallStartupProxy makes the saved proxy current. A credential failure
// blocks the process network so nothing connects directly.
func InstallStartupProxy(s *Settings, creds *PlatformCredentials) {
	if s == nil {
		return
	}
	if err := s.ProxyError(); err != nil {
		netx.Block(err)
		return
	}
	var store netx.CredentialStore
	if creds != nil {
		store = creds.Netx()
	}
	if err := netx.ConfigureFrom(s.Proxy(), store, true); err != nil {
		netx.Block(err)
		s.proxyErr = err
	}
}

// ApplySavedProxy installs a proxy whose password is already in memory.
// It does not read the credential store again.
func ApplySavedProxy(cfg store.NetworkProxyConfiguration, creds *ProxyCredentials) error {
	nctx, err := netx.NewContext(cfg, netCredentials(creds))
	if err != nil {
		netx.Block(err)
		return err
	}
	netx.Configure(nctx)
	return nil
}

// TestProxyDraft fetches the test site through a temporary context. The
// draft is not installed.
func TestProxyDraft(ctx context.Context, cfg store.NetworkProxyConfiguration, creds *ProxyCredentials, site string) error {
	return probeProxy(ctx, cfg, netCredentials(creds), site)
}

func probeProxy(ctx context.Context, cfg store.NetworkProxyConfiguration, creds *netx.Credentials, rawURL string, opts ...netx.Option) error {
	nctx, err := netx.NewContext(cfg, creds, opts...)
	if err != nil {
		return err
	}
	defer nctx.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return netx.ErrConnectionFailed
	}
	resp, err := nctx.Client(netx.Standard).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	// Any answer from the site proves the route works, even a 404 from a
	// custom page; a proxy that cannot reach it answers 407 or 5xx.
	if resp.StatusCode == http.StatusProxyAuthRequired || resp.StatusCode >= 500 {
		return netx.ErrConnectionFailed
	}
	return nil
}

func netCredentials(c *ProxyCredentials) *netx.Credentials {
	if c == nil {
		return nil
	}
	return &netx.Credentials{Username: c.Username, Password: c.Password}
}

// OpenFolder reveals path in the file manager.
func OpenFolder(path string) { openFolder(path) }

type liveApp struct {
	preferenceWork livePreferenceWork
	svc            *runway.Service
	codex          *codex.Store
	grokMod        *grok.Module
	grok           *grokBridge
	claudeMod      *claude.Module
	claudeB        *claudeBridge
	cursorB        *cursorBridge
	codexCache     usage.CacheScanCache
	grokCache      grok.CacheScanCache
	reset          *resetBridge
	repo           *usage.Repository
	widgets        *widgetCoordinator
	dispatch       *snapshotDispatch
	widgetDispatch *snapshotDispatch
	alertDispatch  *alertDispatch
	version        string
	langs          []string
}

var (
	liveMu sync.Mutex
	liveRT *liveApp
)

func setLive(rt *liveApp) {
	liveMu.Lock()
	liveRT = rt
	liveMu.Unlock()
}

func currentRuntime() *liveApp {
	liveMu.Lock()
	defer liveMu.Unlock()
	return liveRT
}

func newRuntime(opts BackgroundOptions) *liveApp {
	home := opts.Home
	langs := l10n.SystemPreferredLanguages()
	prefs := store.DefaultPreferences()
	if opts.Settings != nil {
		prefs = opts.Settings.Prefs()
	}
	codexStore := codex.NewStore(platform.CodexDir(home), platform.RunwayDir(home))
	// NewStore is the constructor; grok has no Open.
	gstore := grok.NewStore(grok.ResolveOfficialHome(os.Getenv("GROK_HOME"), home), platform.RunwayDir(home))
	mod := grok.NewModule(gstore)
	mod.AppVersion = opts.Version
	mod.Billing = (&grok.BillingClient{}).Fetch
	bridge := newGrokBridge(mod, prefs, langs)
	if _, err := mod.Load(context.Background()); err != nil {
		log.Println("grok accounts: load failed")
	}
	bridge.reload(false)
	cstore := claude.NewStore(claude.ResolveConfigDir(os.Getenv("CLAUDE_CONFIG_DIR"), home), platform.RunwayDir(home), home, os.Getenv("CLAUDE_CONFIG_DIR"), "")
	cmod := claude.NewModule(cstore)
	cmod.Usage = &claude.UsageClient{Version: claudeCLIVersion(cstore.ConfigDir())}
	cmod.Tokens = claude.RefreshClient{}
	cbridge := newClaudeBridge(cmod, prefs, langs)
	if _, err := cmod.Load(context.Background()); err != nil {
		log.Println("claude accounts: load failed")
	}
	cbridge.reload(false)
	cursorB := newCursorBridge(home, prefs, langs)
	reset := newResetBridge(opts.Settings, prefs, langs)
	repo := openUsage(home)
	widgetRequirements := initialWidgetRequirements(opts.Settings)
	widgets := newWidgetCoordinator(home, widgetRequirements != 0)
	var svc *runway.Service
	dispatch := &snapshotDispatch{post: mygo.RunOnMain, apply: func(snap runway.Snapshot) {
		if opts.Host != nil && LiveService() == svc {
			publishLive(opts.Host, snap)
		}
	}}
	widgetUI := &snapshotDispatch{post: func(fn func()) { fn() }, apply: func(snap runway.Snapshot) {
		if widgetRequirements != 0 {
			if err := widgets.publish(svc.WidgetSnapshot(time.Now())); err != nil && widgets.ctx.Err() == nil {
				svc.ReportWidgetError(err)
			}
		}
	}}
	alertUI := &alertDispatch{post: runOnMain, apply: func(e runway.Event) {
		if LiveService() == svc {
			deliverEvent(opts, e)
		}
	}}
	svc = runway.New(runway.Options{
		Sources: runway.NewLiveSources(runway.LiveConfig{
			Store: codexStore, CodexHome: platform.CodexDir(home),
			Quota: codexapi.QuotaClient{}, Refresher: codexapi.TokenRefresher{}, Repo: repo,
		}),
		Preferences:         prefs,
		AlertsPath:          alerts.DefaultPath(home),
		EstimateHistoryPath: estimate.DefaultHistoryPath(home),
		CostCachePath:       usage.DefaultSummaryCachePath(home),
		StatusPath:          export.DefaultStatusPath(home),
		Grok:                bridge,
		Claude:              cbridge,
		Cursor:              cursorB,
		ResetToday:          reset,
		WidgetRequirements:  widgetRequirements,
		WidgetSink:          widgets.publish,
		OnSnapshot:          func(snap runway.Snapshot) { dispatch.publish(snap); widgetUI.publish(snap) },
		OnEvent: func(e runway.Event) {
			if e.Kind == runway.EventFullRefreshDone {
				widgets.refresh()
			}
			alertUI.publish(e)
		},
		SystemLanguages: langs,
	})
	rt := &liveApp{
		svc: svc, codex: codexStore, grokMod: mod, grok: bridge, claudeMod: cmod, claudeB: cbridge, cursorB: cursorB, reset: reset,
		repo: repo, widgets: widgets, dispatch: dispatch, widgetDispatch: widgetUI, alertDispatch: alertUI, version: opts.Version, langs: langs,
	}
	widgets.onRequirements = svc.SetWidgetRequirements
	widgets.onError = svc.ReportWidgetError
	widgets.interval = func() time.Duration {
		return time.Duration(svc.Preferences().WidgetRefreshIntervalSeconds) * time.Second
	}
	return rt
}

func (rt *liveApp) stop() {
	setLive(nil)
	if rt.dispatch != nil {
		rt.dispatch.close()
	}
	if rt.widgetDispatch != nil {
		rt.widgetDispatch.close()
	}
	if rt.alertDispatch != nil {
		rt.alertDispatch.close()
	}
	if rt.widgets != nil {
		rt.widgets.close()
	}
	rt.closePreferences()
	rt.svc.Close()
	if rt.repo != nil {
		_ = rt.repo.Close()
	}
	setLive(nil)
}

func (rt *liveApp) pushPreferences(s *Settings) {
	rt.enqueuePreferences(s.Prefs())
}

func (rt *liveApp) panelActions(s *Settings) PanelActions {
	svc := rt.svc
	actions := PanelActions{
		Refresh:        func() { rt.requestRefresh(s, "") },
		RefreshSection: func(sec runway.Section) { rt.requestRefresh(s, sec) },
		SetCursorMonitoring: func(on bool) {
			if s != nil {
				s.SetCursorMonitoringEnabled(on)
			}
		},
		SwitchAccount: func(id string) {
			p := store.DefaultPreferences()
			p.SelectedProvider = svc.Snapshot().Provider
			l := l10n.ForPreference(string(p.Language), rt.langs)
			if s != nil {
				p, l = s.Prefs(), s.L()
			}
			accounts := svc.Snapshot().Accounts
			if p.SelectedProvider == store.ProviderGrok {
				accounts = svc.Snapshot().Grok.Accounts
			}
			name := id
			for _, account := range accounts.Sidebar {
				if account.ID == id {
					name = account.ResolvedDisplayName()
					break
				}
			}
			if p.AccountPrivacyModeEnabled {
				name = codex.MaskAccountText(name)
			}
			go func() {
				if s == nil {
					p = svc.Preferences()
					l = l10n.ForPreference(string(p.Language), rt.langs)
					if p.AccountPrivacyModeEnabled {
						name = codex.MaskAccountText(name)
					}
				}
				message := ""
				if p.SelectedProvider == store.ProviderGrok {
					message = rt.switchGrok(l, id, name)
				} else {
					message = rt.switchCodex(l, id, name)
				}
				if message != "" {
					runOnMain(func() { ShowDialog(l.Text(l10n.KeyAccountsSwitchFailed), message) })
				}
			}()
		},
		SelectProvider: func(p store.Provider) {
			if s != nil {
				s.SetSelectedProvider(p)
			} else {
				rt.queuePreferences(livePreferenceRequest{provider: &p})
			}
		},
		PanelShown: svc.SetPanelOpen,
	}
	installDetailActions(rt, &actions)
	installAccountActions(rt, s, &actions)
	return actions
}

func refreshSection(s *runway.Service, sec runway.Section) {
	if s.Preferences().SelectedProvider == store.ProviderCursor {
		if sec == runway.SectionQuota {
			s.RefreshCursor(true)
		}
		return
	}
	if p := s.Preferences().SelectedProvider; p == store.ProviderGrok || p == store.ProviderClaude {
		switch sec {
		case runway.SectionQuotaEstimate, runway.SectionSessionRepair:
			// A queued Codex-only section can outlive a provider switch.
			return
		case runway.SectionQuota, runway.SectionResetCredits:
			s.RefreshQuota()
			return
		case runway.SectionAPICost, runway.SectionTokenHeatmap, runway.SectionRecentSessions, runway.SectionSpeed:
			s.RefreshProviderLocal(p)
			return
		}
	}
	switch sec {
	case runway.SectionQuota:
		s.RefreshQuota()
	case runway.SectionResetCredits:
		s.RefreshResetCredits()
	case runway.SectionResetToday:
		s.RefreshResetToday(true)
	case runway.SectionQuotaEstimate:
		s.RefreshQuotaEstimate()
	case runway.SectionAPICost:
		s.RefreshCost(usage.RefreshForce)
	case runway.SectionTokenHeatmap:
		s.RefreshTokenHeatmap(usage.RefreshForce)
	case runway.SectionSessionRepair:
		s.RefreshSessionReport(true)
	case runway.SectionRecentSessions:
		s.RefreshRecentSessions(true)
	case runway.SectionSpeed:
		s.RefreshSpeed(true)
	}
}

func openUsage(home string) *usage.Repository {
	if err := sqliteruntime.Load(); err != nil {
		log.Println("usage index:", err)
		return nil
	}
	repo, err := usage.NewRepository(usage.Config{
		CodexHome:    platform.CodexDir(home),
		DatabasePath: usage.DefaultDatabasePath(home),
	})
	if err != nil {
		log.Println("usage index:", err)
		return nil
	}
	return repo
}

func publishLive(host *App, snap runway.Snapshot) {
	if host == nil {
		return
	}
	mygo.RunOnMain(func() {
		host.PublishSnapshot(snap)
		ApplyTray(host.tray, TrayFromStatus(snap.StatusBar))
	})
}

func deliverEvent(opts BackgroundOptions, e runway.Event) {
	if e.Kind != runway.EventAlerts || opts.Notifier == nil || len(e.Alerts) == 0 {
		return
	}
	runOnMain(func() {
		lc := l10n.ForPreference(string(l10n.English), nil)
		if opts.Settings != nil {
			lc = opts.Settings.L()
		}
		notes := make([]Notification, 0, len(e.Alerts))
		now := time.Now()
		for _, a := range e.Alerts {
			notes = append(notes, alertNotification(lc, a, now))
		}
		go opts.Notifier.Deliver(notes)
	})
}

func alertNotification(l l10n.Localizer, a alerts.Alert, now time.Time) Notification {
	return Notification{ID: a.ID, Title: alertTitle(l, a, now), Body: alertBody(l, a, now)}
}

func alertTitle(l l10n.Localizer, a alerts.Alert, now time.Time) string {
	name := resetTypeName(l, a.ResetType)
	switch a.Kind {
	case alerts.KindQuota:
		return l.Text(l10n.KeyQuotaAlertTitle)
	case alerts.KindResetCredit:
		return l.Text(l10n.KeyResetCreditAlertTitle)
	case alerts.KindRateLimitResetDetected:
		if name == "" {
			return l.Text(l10n.KeyRateLimitResetDetectedAlertTitle)
		}
		return l.Textf(l10n.KeyRateLimitResetDetectedTypedAlertTitle, name)
	case alerts.KindRateLimitResetUpcoming:
		return upcomingTitle(l, a, now, name)
	default:
		return l.Text(l10n.KeyRateLimitResetToday)
	}
}

func upcomingTitle(l l10n.Localizer, a alerts.Alert, now time.Time, name string) string {
	passed := alertPassed(a, now)
	inferred := inferredAlert(a)
	ranged := a.EndDate != nil
	switch {
	case passed && name != "":
		return l.Textf(l10n.KeyRateLimitResetSchedulePassedTypedAlertTitle, name)
	case passed:
		return l.Text(l10n.KeyRateLimitResetSchedulePassedAlertTitle)
	case inferred && name != "":
		return l.Textf(l10n.KeyRateLimitResetPreviewTypedAlertTitle, name)
	case inferred:
		return l.Text(l10n.KeyRateLimitResetPreviewAlertTitle)
	case ranged && name != "":
		return l.Textf(l10n.KeyRateLimitResetUpcomingTypedRangeAlertTitle, name)
	case ranged:
		return l.Text(l10n.KeyRateLimitResetUpcomingRangeAlertTitle)
	case name != "":
		return l.Textf(l10n.KeyRateLimitResetUpcomingTypedAlertTitle, name)
	default:
		return l.Text(l10n.KeyRateLimitResetUpcomingAlertTitle)
	}
}

func alertBody(l l10n.Localizer, a alerts.Alert, now time.Time) string {
	switch a.Kind {
	case alerts.KindQuota:
		pct := "--"
		if a.Threshold != nil {
			pct = strconv.Itoa(*a.Threshold) + "%"
		}
		return l.Textf(l10n.KeyQuotaAlertBody, quotaWindowName(l, a.Name), pct)
	case alerts.KindResetCredit:
		return l.Text(l10n.KeyResetCreditAlertBody)
	case alerts.KindRateLimitResetDetected:
		return detectedBody(l, a.ResetType)
	default:
		return upcomingBody(l, a, now)
	}
}

func upcomingBody(l l10n.Localizer, a alerts.Alert, now time.Time) string {
	when := alertWhen(a)
	inferred := inferredAlert(a)
	if alertPassed(a, now) {
		if when == "" {
			when = "—"
		}
		key := l10n.KeyRateLimitResetSchedulePassedAlertBody
		if inferred {
			key = l10n.KeyRateLimitResetPreviewPassedAlertBody
		}
		return l.Textf(key, when)
	}
	if a.EndDate != nil {
		return rangeBody(l, a, when, inferred)
	}
	return pointBody(l, a, when, inferred)
}

func rangeBody(l l10n.Localizer, a alerts.Alert, when string, inferred bool) string {
	if when == "" {
		when = "—"
	}
	if a.ConfidencePercent != nil {
		key := l10n.KeyRateLimitResetUpcomingRangeDetailAlertBody
		if inferred {
			key = l10n.KeyRateLimitResetPreviewRangeDetailAlertBody
		}
		return l.Textf(key, when, strconv.Itoa(*a.ConfidencePercent)+"%")
	}
	if inferred {
		return l.Textf(l10n.KeyRateLimitResetPreviewLegacyAlertBody, when)
	}
	return l.Textf(l10n.KeyRateLimitResetUpcomingRangeAlertBody, when)
}

func pointBody(l l10n.Localizer, a alerts.Alert, when string, inferred bool) string {
	relative := l.Text(l10n.KeyRateLimitResetUpcomingAlertBody1h)
	if a.Threshold != nil && *a.Threshold <= 30 {
		relative = l.Text(l10n.KeyRateLimitResetUpcomingAlertBody30m)
	}
	if when == "" {
		if inferred {
			return l.Text(l10n.KeyRateLimitResetPreviewAlertBody)
		}
		return relative
	}
	if a.ConfidencePercent == nil {
		if inferred {
			return l.Textf(l10n.KeyRateLimitResetPreviewLegacyAlertBody, when)
		}
		return relative
	}
	conf := strconv.Itoa(*a.ConfidencePercent) + "%"
	if inferred {
		return l.Textf(l10n.KeyRateLimitResetPreviewDetailAlertBody, when, conf)
	}
	return l.Textf(l10n.KeyRateLimitResetUpcomingDetailAlertBody, when, conf, relative)
}

func detectedBody(l l10n.Localizer, t *alerts.ResetType) string {
	if t == nil {
		return l.Text(l10n.KeyRateLimitResetDetectedAlertBody)
	}
	switch *t {
	case alerts.ResetBanked:
		return l.Text(l10n.KeyRateLimitResetDetectedBankedAlertBody)
	case alerts.ResetGlobalAndBanked:
		return l.Text(l10n.KeyRateLimitResetDetectedGlobalAndBankedAlertBody)
	default:
		return l.Text(l10n.KeyRateLimitResetDetectedAlertBody)
	}
}

func quotaWindowName(l l10n.Localizer, name string) string {
	switch name {
	case "5-hour":
		return l.Text(l10n.KeyFiveHourUsage)
	case "Weekly":
		return l.Text(l10n.KeyWeeklyUsage)
	default:
		return name
	}
}

func resetTypeName(l l10n.Localizer, t *alerts.ResetType) string {
	if t == nil {
		return ""
	}
	switch *t {
	case alerts.ResetBanked:
		return l.Text(l10n.KeyRateLimitResetTypeBanked)
	case alerts.ResetGlobalAndBanked:
		return l.Text(l10n.KeyRateLimitResetTypeGlobalAndBanked)
	default:
		return l.Text(l10n.KeyRateLimitResetTypeGlobal)
	}
}

func inferredAlert(a alerts.Alert) bool {
	return a.ScheduleBasis != nil && *a.ScheduleBasis == alerts.BasisContextualInference
}

func alertPassed(a alerts.Alert, now time.Time) bool {
	if a.EndDate != nil {
		return !a.EndDate.Add(time.Minute).After(now)
	}
	return a.Date != nil && !a.Date.After(now)
}

func alertWhen(a alerts.Alert) string {
	if a.Date == nil {
		return ""
	}
	end := *a.Date
	if a.EndDate != nil {
		end = *a.EndDate
	}
	return runway.ScheduledLabel(runway.ScheduleWindow{StartAt: *a.Date, EndAt: end}, time.Local)
}

type grokBridge struct {
	mod        *grok.Module
	mu         sync.Mutex
	onChange   func()
	state      runway.GrokState
	refreshes  int
	lc         l10n.Localizer
	prefs      store.Preferences
	local      *grok.SessionUsageSummary
	localRange store.CostRange
	localErr   error
	speed      *speed.Report
	speedErr   error
	generation uint64
	refreshMu  sync.Mutex
	scanMu     sync.Mutex
}

func newGrokBridge(mod *grok.Module, prefs store.Preferences, langs []string) *grokBridge {
	return &grokBridge{mod: mod, prefs: prefs.Clone(), lc: l10n.ForPreference(string(prefs.Language), langs)}
}

func (g *grokBridge) State(now time.Time) runway.GrokState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.state
	if snap, ok := st.Detail.(*grok.BillingSnapshot); ok && snap.ResetCredits != nil {
		c := snap.ResetCredits.CodexSnapshot(now)
		sum := runway.SummarizeCredits(c)
		st.ResetCredits, st.ResetCreditSummary = &c, &sum
	}
	st.Widget = grokWidget(st)
	return st
}

func (g *grokBridge) Refresh(ctx context.Context) error {
	g.changeRefreshing(1)
	defer g.changeRefreshing(-1)
	g.refreshMu.Lock()
	defer g.refreshMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if g.mod != nil {
		g.mod.Refresh(ctx, grok.RefreshTarget{Kind: grok.RefreshCurrent})
	}
	g.reload(true)
	return ctx.Err()
}

// RefreshLocal rescans the local Grok sessions without touching the quota.
func (g *grokBridge) RefreshLocal(ctx context.Context) error {
	g.changeRefreshing(1)
	defer g.changeRefreshing(-1)
	g.scanMu.Lock()
	defer g.scanMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	err := g.scanLocal(ctx)
	g.reload(true)
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (g *grokBridge) SetOnChange(fn func()) {
	g.mu.Lock()
	g.onChange = fn
	g.mu.Unlock()
}

func (g *grokBridge) setPreferences(p store.Preferences, langs []string) {
	g.mu.Lock()
	rangeChanged := g.prefs.APICostSummaryRange != p.APICostSummaryRange
	speedChanged := g.prefs.ShowsSpeedAnalysis != p.ShowsSpeedAnalysis
	if rangeChanged || g.prefs.ShowsRecentSessions != p.ShowsRecentSessions || speedChanged {
		g.generation++
	}
	if speedChanged && !p.ShowsSpeedAnalysis {
		g.speed = nil
		g.speedErr = nil
	}
	if rangeChanged {
		g.state.Cost = nil
	}
	languageChanged := g.prefs.Language != p.Language
	g.prefs = p.Clone()
	g.lc = l10n.ForPreference(string(p.Language), langs)
	if languageChanged {
		g.relabelLocked(&g.state)
	}
	g.mu.Unlock()
}

func (g *grokBridge) reload(notify bool) {
	g.mu.Lock()
	lc, mod := g.lc, g.mod
	g.mu.Unlock()
	next := buildGrokState(mod, lc)
	g.mu.Lock()
	next.Refreshing = g.refreshes > 0
	g.relabelLocked(&next)
	applyGrokLocal(&next, g.local)
	if g.prefs.ShowsSpeedAnalysis {
		next.Speed = g.speed
	}
	if g.localRange != g.prefs.APICostSummaryRange {
		next.Cost = nil
	}
	if g.localErr != nil {
		next.LastError = g.lc.Text(l10n.KeyStatusError) + ": " + g.localErr.Error()
	}
	g.state = next
	fn := g.onChange
	g.mu.Unlock()
	if notify && fn != nil {
		fn()
	}
}

// relabelLocked uses the latest preference even when a disk reload began
// before the language changed; preferences themselves never reread the index.
func (g *grokBridge) relabelLocked(st *runway.GrokState) {
	st.LastError = grokErrorText(g.lc, st.Availability)
	st.SpeedError = ""
	if snap, ok := st.Detail.(*grok.BillingSnapshot); ok {
		st.Meters = grokMeters(g.lc, *snap, st.UpdatedAt)
	}
	if g.localErr != nil {
		st.LastError = g.lc.Text(l10n.KeyStatusError) + ": " + g.localErr.Error()
	}
	if g.prefs.ShowsSpeedAnalysis && g.speedErr != nil {
		st.SpeedError = g.speedErr.Error()
	}
}

func buildGrokState(mod *grok.Module, lc l10n.Localizer) runway.GrokState {
	st := runway.GrokState{Availability: runway.GrokNotLoggedIn}
	if mod == nil || mod.Store == nil {
		st.LastError = lc.Text(l10n.KeyGrokNotLoggedIn)
		return st
	}
	idx, err := mod.Store.LoadIndex()
	if err != nil {
		st.Availability = runway.GrokFailed
		st.LastError = lc.Text(l10n.KeyStatusError)
		return st
	}
	st.Accounts.ActiveID = idx.CurrentAccountID
	ordered := idx.OrderedAccounts()
	for _, account := range ordered {
		st.Accounts.Sidebar = append(st.Accounts.Sidebar, grokSidebarAccount(lc, account))
	}
	st.AccountQuotas = grokAccountQuotas(lc, ordered)
	if idx.CurrentAccountID == "" {
		st.LastError = lc.Text(l10n.KeyGrokNotLoggedIn)
		return st
	}
	acc, ok := idx.Account(idx.CurrentAccountID)
	if !ok {
		st.LastError = lc.Text(l10n.KeyGrokNotLoggedIn)
		return st
	}
	st.Accounts.Display = codex.AccountDisplay{Authenticated: !acc.RequiresReauth, AccountID: acc.ID, DisplayName: acc.ResolvedDisplayName(), Email: acc.Email()}
	var snap *grok.BillingSnapshot
	if len(acc.CachedQuota) > 0 {
		parsed, perr := grok.ParseBillingSnapshot(acc.CachedQuota)
		if perr != nil {
			st.Availability = runway.GrokBillingParseFailed
			st.LastError = lc.Text(l10n.KeyGrokBillingParseFailed)
			return st
		}
		snap = &parsed
	}
	st.Availability = grokAvailability(&acc, snap != nil)
	st.LastError = grokErrorText(lc, st.Availability)
	if acc.LastQuotaAt != nil {
		st.UpdatedAt = *acc.LastQuotaAt
	}
	if snap == nil {
		return st
	}
	if snap.Plan != nil {
		st.Plan = *snap.Plan
	}
	if !snap.UpdatedAt.IsZero() {
		st.UpdatedAt = snap.UpdatedAt
	}
	st.Meters = grokMeters(lc, *snap, st.UpdatedAt)
	if snap.ResetCredits != nil {
		c := snap.ResetCredits.CodexSnapshot(time.Now())
		sum := runway.SummarizeCredits(c)
		st.ResetCredits, st.ResetCreditSummary = &c, &sum
	}
	st.Widget = grokWidget(st)
	st.Detail = snap
	return st
}

func grokAvailability(acc *grok.ManagedAccount, hasSnap bool) runway.GrokAvailability {
	if acc.RequiresReauth || acc.LastError == grok.ErrorCodeAuthenticationNeeded {
		return runway.GrokReauthRequired
	}
	switch acc.LastError {
	case grok.ErrorCodeCLIUnavailable:
		return runway.GrokCLIUnavailable
	case grok.ErrorCodeBillingParseFailed:
		if !hasSnap {
			return runway.GrokBillingParseFailed
		}
	}
	if hasSnap {
		return runway.GrokReady
	}
	if acc.LastError != "" {
		return runway.GrokFailed
	}
	return runway.GrokLoading
}

func grokErrorText(lc l10n.Localizer, av runway.GrokAvailability) string {
	switch av {
	case runway.GrokReauthRequired:
		return lc.Text(l10n.KeyGrokReauthenticationRequired)
	case runway.GrokCLIUnavailable:
		return lc.Text(l10n.KeyGrokCLIUnavailable)
	case runway.GrokCLITooOld:
		return lc.Text(l10n.KeyGrokCLITooOld)
	case runway.GrokBillingParseFailed:
		return lc.Text(l10n.KeyGrokBillingParseFailed)
	case runway.GrokNotLoggedIn:
		return lc.Text(l10n.KeyGrokNotLoggedIn)
	case runway.GrokFailed:
		return lc.Text(l10n.KeyStatusError)
	default:
		return ""
	}
}

func grokMeters(lc l10n.Localizer, snap grok.BillingSnapshot, now time.Time) []runway.Meter {
	window := meterWindow(snap, clampPercent(int(math.Round(snap.IncludedUsagePercent))))
	title := lc.Text(l10n.KeyGrokIncludedQuota)
	if snap.Period != nil {
		title = periodTitle(lc, snap.Period.Kind)
	}
	meters := []runway.Meter{runway.NewMeter(title, window, now, runway.DefaultMarkers(), runway.SourceStandard)}
	for _, product := range snap.ProductUsage {
		w := window
		w.UsedPercent = clampPercent(int(math.Round(product.UsagePercent)))
		meters = append(meters, runway.NewMeter(productTitle(lc, product.Product), w, now, nil, runway.SourceModelSpecific))
	}
	return meters
}

func meterWindow(snap grok.BillingSnapshot, used int) runway.MeterWindow {
	w := runway.MeterWindow{UsedPercent: used}
	if snap.Period == nil {
		return w
	}
	w.WindowMinutes = periodMinutes(snap.Period.Kind)
	if snap.Period.ResetsAt != nil {
		w.ResetsAt = *snap.Period.ResetsAt
	}
	return w
}

func periodTitle(lc l10n.Localizer, kind grok.BillingPeriodKind) string {
	switch kind {
	case grok.BillingWeekly:
		return lc.Text(l10n.KeyGrokBillingPeriodWeekly)
	case grok.BillingMonthly:
		return lc.Text(l10n.KeyGrokBillingPeriodMonthly)
	default:
		return lc.Text(l10n.KeyGrokIncludedQuota)
	}
}

func periodMinutes(kind grok.BillingPeriodKind) int {
	switch kind {
	case grok.BillingWeekly:
		return 7 * 24 * 60
	case grok.BillingMonthly:
		return 30 * 24 * 60
	default:
		return 0
	}
}

func productTitle(lc l10n.Localizer, product string) string {
	switch strings.ToLower(product) {
	case "grokbuild":
		return lc.Text(l10n.KeyGrokProductBuild)
	case "grokimagine":
		return lc.Text(l10n.KeyGrokProductImagine)
	case "grokchat":
		return lc.Text(l10n.KeyGrokProductChat)
	default:
		return product
	}
}

func clampPercent(n int) int { return max(0, min(100, n)) }

func grokWidget(st runway.GrokState) *export.ProviderSnapshot {
	p := export.ProviderSnapshot{
		Provider: export.ProviderGrok, Availability: widgetAvailability(st.Availability),
		TokenSource: export.TokensThisMac,
	}
	if st.Plan != "" {
		plan := st.Plan
		p.Plan = &plan
	}
	if !st.UpdatedAt.IsZero() {
		t := st.UpdatedAt
		p.UpdatedAt = &t
	}
	for _, m := range st.Meters {
		p.Quota = append(p.Quota, meterQuota(m))
	}
	for day, tokens := range st.Heatmap.Local {
		p.DailyTokens = append(p.DailyTokens, export.NewDailyTokens(day, int(tokens)))
	}
	if st.Cost != nil && st.Cost.EstimatedUSD != nil {
		d, err := swiftjson.ParseDecimal(st.Cost.EstimatedUSD.String())
		if err == nil {
			p.APIEquivalentCostUSD = &d
		}
	}
	if st.ResetCreditSummary != nil {
		r := export.NewResetCredits(st.ResetCreditSummary.AvailableCount, st.ResetCreditSummary.ExpiringCount)
		p.ResetCredits = &r
	}
	out := export.NewProviderSnapshot(p)
	return &out
}

func widgetAvailability(av runway.GrokAvailability) export.Availability {
	switch av {
	case runway.GrokReady:
		return export.Available
	case runway.GrokNotLoggedIn, runway.GrokReauthRequired:
		return export.NotLoggedIn
	case runway.GrokCLIUnavailable, runway.GrokCLITooOld:
		return export.CLIUnavailable
	default:
		return export.Unavailable
	}
}

func meterQuota(m runway.Meter) export.WidgetQuota {
	var mins *int
	if m.WindowMinutes > 0 {
		v := m.WindowMinutes
		mins = &v
	}
	src := export.QuotaStandard
	if m.Source == runway.SourceModelSpecific {
		src = export.QuotaModelSpecific
	}
	var reset *time.Time
	if !m.ResetsAt.IsZero() {
		t := m.ResetsAt
		reset = &t
	}
	q := export.NewWidgetQuota(m.Title, mins, src, m.UsedPercent, m.RemainingPercent, reset)
	if m.ValueText != "" {
		v := m.ValueText
		q.ValueText = &v
	}
	return q
}

type resetBridge struct {
	ref      *resettoday.Refresher
	mu       sync.Mutex
	onChange func()
	pending  []alerts.Alert
	lc       l10n.Localizer
	loc      *time.Location
	prefs    store.Preferences
}

func newResetBridge(s *Settings, prefs store.Preferences, langs []string) *resetBridge {
	b := &resetBridge{prefs: prefs.Clone(), lc: l10n.ForPreference(string(prefs.Language), langs), loc: time.Local}
	client := resettoday.Client{}
	b.ref = &resettoday.Refresher{
		Fetch: client.FetchStatus,
		Loc:   time.Local,
		Interval: func() time.Duration {
			b.mu.Lock()
			defer b.mu.Unlock()
			return time.Duration(b.prefs.RateLimitResetTodayRefreshIntervalSeconds) * time.Second
		},
		AlertsEnabled: func() bool {
			b.mu.Lock()
			defer b.mu.Unlock()
			return b.prefs.RateLimitResetTodayAlertsEnabled
		},
		Deliver:  b.stash,
		OnChange: func(resettoday.RefreshState) { b.fire() },
	}
	return b
}

func resetInterval(s *Settings) time.Duration {
	seconds := store.DefaultPreferences().RateLimitResetTodayRefreshIntervalSeconds
	if s != nil {
		seconds = s.Prefs().RateLimitResetTodayRefreshIntervalSeconds
	}
	if seconds <= 0 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second
}

func (b *resetBridge) State(now time.Time) runway.ResetTodayState {
	st := b.ref.State()
	if st.Snapshot == nil {
		return runway.ResetTodayState{}
	}
	b.mu.Lock()
	lc := b.lc
	b.mu.Unlock()
	src := resettoday.NewSource(st.Snapshot, b.loc)
	w := src.Widget(now)
	return runway.ResetTodayState{
		Text: b.verdictText(st.Snapshot, now), Lines: resetDetailLines(lc, st.Snapshot, now, b.loc),
		HasData: true, Widget: &w, Detail: st.Snapshot,
	}
}

func resetDetailLines(lc l10n.Localizer, snap *resettoday.Snapshot, now time.Time, loc *time.Location) []runway.DetailLine {
	var lines []runway.DetailLine
	if d := snap.VerdictDetail(lc, runwayfmt.Formatter{}, now, loc); d != nil && d.PlainText != "" {
		lines = append(lines, runway.DetailLine{Value: d.PlainText})
	}
	if line := snap.EvidenceLine(lc, now, loc); line != "" {
		lines = append(lines, runway.DetailLine{Value: line})
	}
	return lines
}

func (b *resetBridge) Refresh(ctx context.Context, force bool) error {
	if !b.ref.Refresh(ctx, force) {
		st := b.ref.State()
		if !st.Refreshing && st.Snapshot != nil && b.alertsOn() {
			b.stash(resettoday.Alerts(st.Snapshot, st.Snapshot, time.Now(), b.loc))
		}
	}
	return nil
}

func (b *resetBridge) LastFetch() time.Time { return b.ref.State().LastFetch }

func (b *resetBridge) PendingAlerts(time.Time) []alerts.Alert {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending = nil
	return out
}

func (b *resetBridge) SetOnChange(fn func()) {
	b.mu.Lock()
	b.onChange = fn
	b.mu.Unlock()
}

func (b *resetBridge) setPreferences(p store.Preferences, langs []string) {
	b.mu.Lock()
	b.prefs = p.Clone()
	b.lc = l10n.ForPreference(string(p.Language), langs)
	b.mu.Unlock()
}

func (b *resetBridge) stash(list []alerts.Alert) {
	if len(list) == 0 {
		return
	}
	b.mu.Lock()
	b.pending = append(b.pending, list...)
	b.mu.Unlock()
}

func (b *resetBridge) fire() {
	b.mu.Lock()
	fn := b.onChange
	b.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (b *resetBridge) alertsOn() bool {
	return b.ref.AlertsEnabled == nil || b.ref.AlertsEnabled()
}

func (b *resetBridge) verdictText(snap *resettoday.Snapshot, now time.Time) string {
	b.mu.Lock()
	lc := b.lc
	b.mu.Unlock()
	p := snap.VerdictPresentation(now, b.loc)
	if p.ShowsYes {
		return lc.Text(l10n.KeyRateLimitResetTodayYes)
	}
	if p.Reason == export.ReasonUnavailable {
		return lc.Text(l10n.KeyRateLimitResetTodayUnknown)
	}
	return lc.Text(l10n.KeyRateLimitResetTodayNo)
}
