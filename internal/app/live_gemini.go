package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

// geminiKeychain opens the keychain item the Antigravity clients share.
// Tests replace it so they never read the real login.
var geminiKeychain = gemini.SystemKeychain

// newGeminiModule keeps the accounts under home's Quosparr data and reads
// the official login from the keychain and home's mirror files.
func newGeminiModule(home, version string) *gemini.Module {
	st := gemini.NewStore(platform.RunwayDir(home), home)
	st.Official.Keychain = geminiKeychain()
	mod := gemini.NewModule(st)
	mod.AppVersion = version
	return mod
}

type geminiBridge struct {
	pricingSnapshot   func() pricing.Snapshot
	pricingGeneration uint64
	mod               *gemini.Module
	installed         func() bool
	mu                sync.Mutex
	onChange          func()
	state             runway.GeminiState
	refreshes         int
	lc                l10n.Localizer
	prefs             store.Preferences
	local             *gemini.SessionUsageSummary
	localRange        store.CostRange
	localErr          error
	speed             *speed.Report
	speedErr          error
	generation        uint64
	refreshMu         sync.Mutex
	scanMu            sync.Mutex
	sessions          gemini.SessionCache
}

func newGeminiBridge(mod *gemini.Module, home string, prefs store.Preferences, langs []string) *geminiBridge {
	return &geminiBridge{
		mod: mod, prefs: prefs.Clone(), lc: l10n.ForPreference(string(prefs.Language), langs),
		installed: func() bool { return gemini.ClientInstalled(home) },
	}
}

func (g *geminiBridge) State(time.Time) runway.GeminiState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.state
	st.Widget = geminiWidget(st)
	return st
}

// Refresh follows the official login and reads the current account's quota.
// The module spaces rate-limited reads itself, so force changes nothing.
func (g *geminiBridge) Refresh(ctx context.Context, _ bool) error {
	g.changeRefreshing(1)
	defer g.changeRefreshing(-1)
	g.refreshMu.Lock()
	defer g.refreshMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if g.mod != nil && g.mod.Store != nil {
		g.mod.Refresh(ctx, gemini.RefreshTarget{Kind: gemini.RefreshCurrent})
	}
	g.reload(true)
	return ctx.Err()
}

// RefreshLocal rescans the local Antigravity conversations without touching
// the quota.
func (g *geminiBridge) RefreshLocal(ctx context.Context) error {
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

func (g *geminiBridge) SetOnChange(fn func()) {
	g.mu.Lock()
	g.onChange = fn
	g.mu.Unlock()
}

func (g *geminiBridge) changeRefreshing(delta int) {
	g.mu.Lock()
	g.refreshes += delta
	g.state.Refreshing = g.refreshes > 0
	fn := g.onChange
	g.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (g *geminiBridge) setPreferences(p store.Preferences, langs []string) {
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
	g.mu.Unlock()
	if languageChanged {
		g.reload(true)
	}
}

func (g *geminiBridge) reload(notify bool) {
	g.mu.Lock()
	lc, mod, installed := g.lc, g.mod, g.installed
	g.mu.Unlock()
	next := buildGeminiState(mod, lc, installed)
	g.mu.Lock()
	next.Refreshing = g.refreshes > 0
	g.applyLocalLocked(&next)
	g.state = next
	fn := g.onChange
	g.mu.Unlock()
	if notify && fn != nil {
		fn()
	}
}

func buildGeminiState(mod *gemini.Module, lc l10n.Localizer, installed func() bool) runway.GeminiState {
	st := runway.GeminiState{Availability: runway.GeminiNotLoggedIn}
	if mod == nil || mod.Store == nil {
		st.Availability = runway.GeminiNotInstalled
		st.LastError = geminiErrorText(lc, st.Availability)
		return st
	}
	idx, err := mod.Store.LoadIndex()
	if err != nil {
		st.Availability = runway.GeminiFailed
		st.LastError = accountFailure(lc, lc.Text(l10n.KeyGeminiRefreshFailed), err)
		return st
	}
	st.Accounts.ActiveID = idx.CurrentAccountID
	ordered := idx.OrderedAccounts()
	for _, account := range ordered {
		st.Accounts.Sidebar = append(st.Accounts.Sidebar, geminiSidebarAccount(lc, account))
	}
	st.AccountQuotas = geminiAccountQuotas(lc, ordered)
	acc, ok := idx.Account(idx.CurrentAccountID)
	if !ok {
		if len(ordered) == 0 && installed != nil && !installed() {
			st.Availability = runway.GeminiNotInstalled
		}
		st.LastError = geminiErrorText(lc, st.Availability)
		return st
	}
	st.Accounts.Display = codex.AccountDisplay{
		Authenticated: !acc.RequiresReauth, AccountID: acc.ID,
		DisplayName: acc.ResolvedDisplayName(), Email: acc.Email(),
	}
	var snap *gemini.QuotaSnapshot
	if len(acc.CachedQuota) > 0 {
		if parsed, perr := gemini.ParseQuotaSnapshot(acc.CachedQuota); perr == nil {
			snap = &parsed
		}
	}
	st.Availability = geminiAvailability(acc, snap)
	st.LastError = geminiErrorText(lc, st.Availability)
	if acc.LastQuotaAt != nil {
		st.UpdatedAt = *acc.LastQuotaAt
	}
	if snap == nil {
		return st
	}
	if !snap.UpdatedAt.IsZero() {
		st.UpdatedAt = snap.UpdatedAt
	}
	st.Plan, st.TierKind = geminiPlanName(lc, snap.Tier), string(snap.Tier.Kind)
	if snap.Validation != nil {
		st.ValidationURL = snap.Validation.URL
	}
	st.Meters = geminiMeters(lc, *snap, st.UpdatedAt)
	st.Details = geminiDetailLines(lc, *snap, st.UpdatedAt)
	st.Detail = snap
	return st
}

// geminiAvailability reads the stored error of the current account; a
// reading that is kept after a failure still shows.
func geminiAvailability(acc gemini.ManagedAccount, snap *gemini.QuotaSnapshot) runway.GeminiAvailability {
	if acc.RequiresReauth {
		return runway.GeminiReauthRequired
	}
	switch acc.LastError {
	case "":
		if snap != nil {
			if snap.Validation != nil {
				return runway.GeminiValidationRequired
			}
			return runway.GeminiReady
		}
		return runway.GeminiLoading
	case gemini.ErrorCodeAuthenticationNeeded:
		return runway.GeminiReauthRequired
	case gemini.ErrorCodeValidationRequired:
		return runway.GeminiValidationRequired
	case gemini.ErrorCodeSubscriptionRequired:
		return runway.GeminiSubscriptionRequired
	case gemini.ErrorCodeRateLimited:
		return runway.GeminiRateLimited
	case gemini.ErrorCodeNotSignedIn:
		return runway.GeminiNotLoggedIn
	case gemini.ErrorCodeNotInstalled:
		return runway.GeminiNotInstalled
	case gemini.ErrorCodeQuotaParseFailed:
		if snap == nil {
			return runway.GeminiQuotaParseFailed
		}
	}
	if snap != nil {
		return runway.GeminiReady
	}
	return runway.GeminiFailed
}

func geminiErrorText(lc l10n.Localizer, av runway.GeminiAvailability) string {
	switch av {
	case runway.GeminiNotInstalled:
		return lc.Text(l10n.KeyGeminiNotInstalled)
	case runway.GeminiNotLoggedIn:
		return lc.Text(l10n.KeyGeminiNotLoggedIn)
	case runway.GeminiReauthRequired:
		return lc.Text(l10n.KeyGeminiReauthRequired)
	case runway.GeminiValidationRequired:
		return lc.Text(l10n.KeyGeminiValidationRequired)
	case runway.GeminiSubscriptionRequired:
		return lc.Text(l10n.KeyGeminiSubscriptionRequired)
	case runway.GeminiRateLimited:
		return lc.Text(l10n.KeyGeminiRateLimited)
	case runway.GeminiQuotaParseFailed:
		return lc.Text(l10n.KeyGeminiQuotaParseFailed)
	case runway.GeminiFailed:
		return lc.Text(l10n.KeyGeminiRefreshFailed)
	}
	return ""
}

// geminiErrorCodeText is the account list text of a stored error code.
func geminiErrorCodeText(lc l10n.Localizer, code string) string {
	switch code {
	case gemini.ErrorCodeNotInstalled:
		return lc.Text(l10n.KeyGeminiNotInstalled)
	case gemini.ErrorCodeNotSignedIn:
		return lc.Text(l10n.KeyGeminiNotLoggedIn)
	case gemini.ErrorCodeAuthenticationNeeded:
		return lc.Text(l10n.KeyGeminiReauthRequired)
	case gemini.ErrorCodeValidationRequired:
		return lc.Text(l10n.KeyGeminiValidationRequired)
	case gemini.ErrorCodeSubscriptionRequired:
		return lc.Text(l10n.KeyGeminiSubscriptionRequired)
	case gemini.ErrorCodeRateLimited:
		return lc.Text(l10n.KeyGeminiRateLimited)
	case gemini.ErrorCodeQuotaParseFailed:
		return lc.Text(l10n.KeyGeminiQuotaParseFailed)
	case gemini.ErrorCodeTimeout:
		return lc.Text(l10n.KeyGeminiRefreshFailed) + ": " + accountReasonText(lc, "timeout")
	case gemini.ErrorCodeCancelled:
		return accountReasonText(lc, "canceled")
	}
	return lc.Text(l10n.KeyGeminiRefreshFailed)
}

func geminiSidebarAccount(lc l10n.Localizer, account gemini.ManagedAccount) codex.ManagedAccount {
	row := codex.ManagedAccount{
		ID: account.ID, SortIndex: account.SortIndex, DisplayName: account.ResolvedDisplayName(),
		Email: account.Email(), Alias: account.Alias,
		RequiresReauth: account.RequiresReauth, LastQuotaAt: account.LastQuotaAt,
	}
	if account.LastError != "" {
		row.LastError = geminiErrorCodeText(lc, account.LastError)
	}
	return row
}

func geminiAccountQuotas(lc l10n.Localizer, accounts []gemini.ManagedAccount) map[string]runway.GeminiAccountQuota {
	out := map[string]runway.GeminiAccountQuota{}
	for _, account := range accounts {
		q := runway.GeminiAccountQuota{TierKind: string(gemini.TierUnknown)}
		if account.LastQuotaAt != nil {
			q.UpdatedAt = *account.LastQuotaAt
		}
		if len(account.CachedQuota) > 0 {
			if snap, err := gemini.ParseQuotaSnapshot(account.CachedQuota); err == nil {
				if !snap.UpdatedAt.IsZero() {
					q.UpdatedAt = snap.UpdatedAt
				}
				q.Plan, q.TierKind = geminiPlanName(lc, snap.Tier), string(snap.Tier.Kind)
				q.Meters = geminiMeters(lc, snap, q.UpdatedAt)
				q.Details = geminiCreditLines(lc, snap.Credits)
				if m, ok := runway.PrimaryStandardMeter(q.Meters, []int{runway.FiveHourMinutes, runway.WeeklyMinutes}); ok {
					q.Title, q.UsedPercent, q.ResetsAt = m.Title, m.UsedPercent, m.ResetsAt
				}
			}
		}
		out[account.ID] = q
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func geminiWidget(st runway.GeminiState) *export.ProviderSnapshot {
	p := export.ProviderSnapshot{
		Provider: export.ProviderGemini, Availability: geminiWidgetAvailability(st.Availability),
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
			p.APIEquivalentCostPartial = st.Cost.UnpricedTokens > 0 || st.Cost.Confidence == usage.ConfidencePartiallyPriced
		}
	}
	out := export.NewProviderSnapshot(p)
	return &out
}

func geminiWidgetAvailability(av runway.GeminiAvailability) export.Availability {
	switch av {
	case runway.GeminiReady, runway.GeminiRateLimited:
		return export.Available
	case runway.GeminiNotLoggedIn, runway.GeminiReauthRequired, runway.GeminiValidationRequired, runway.GeminiSubscriptionRequired:
		return export.NotLoggedIn
	case runway.GeminiNotInstalled:
		return export.CLIUnavailable
	}
	return export.Unavailable
}

func (rt *liveApp) geminiReady() bool {
	return rt != nil && rt.geminiMod != nil && rt.geminiMod.Store != nil
}

func (rt *liveApp) geminiReload() {
	if rt.geminiB != nil {
		rt.geminiB.reload(true)
	}
}

func (rt *liveApp) importGeminiOfficial(l l10n.Localizer) string {
	if !rt.geminiReady() {
		return l.Text(l10n.KeyGeminiNotInstalled)
	}
	_, err := rt.geminiMod.Apply(context.Background(), gemini.Command{Kind: gemini.CommandImportOfficial})
	rt.geminiReload()
	if errors.Is(err, gemini.ErrOfficialCredentialMissing) {
		return l.Text(l10n.KeyGeminiNotLoggedIn)
	}
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyAccountsImportFailed), err)
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, 1)
}

func (rt *liveApp) geminiPasteText(l l10n.Localizer, text string) (string, bool) {
	if !rt.geminiReady() {
		return l.Text(l10n.KeyGeminiNotInstalled), false
	}
	batch, _, err := rt.geminiMod.ImportPastedText(context.Background(), text)
	rt.geminiReload()
	if err != nil {
		return accountBatchFailure(l, batch.SuccessCount(), batch.FailureCount()+1, append(batch.FailureErrors, err)), false
	}
	if message := accountBatchFailure(l, batch.SuccessCount(), batch.FailureCount(), batch.FailureErrors); message != "" {
		return message, false
	}
	if batch.SuccessCount() == 0 {
		return l.Text(l10n.KeyAccountsImportNoCredentials), false
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, batch.SuccessCount()), true
}

func (rt *liveApp) removeGemini(l l10n.Localizer, id string) string {
	if !rt.geminiReady() {
		return l.Text(l10n.KeyStatusError)
	}
	_, err := rt.geminiMod.Apply(context.Background(), gemini.Command{Kind: gemini.CommandRemove, ID: id})
	rt.geminiReload()
	var current *gemini.CurrentAccountCannotBeRemovedError
	if errors.As(err, &current) {
		return l.Text(l10n.KeyGeminiCurrentAccountCannotDelete)
	}
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyStatusError), err)
	}
	return ""
}

func (rt *liveApp) setGeminiAlias(l l10n.Localizer, id, alias string) string {
	if !rt.geminiReady() {
		return tx(l, xDetailUnavailable)
	}
	_, err := rt.geminiMod.Apply(context.Background(), gemini.Command{Kind: gemini.CommandSetAlias, ID: id, Alias: alias})
	rt.geminiReload()
	if err != nil {
		return accountFailure(l, tx(l, xAccountSaveFailed), err)
	}
	return ""
}

func (rt *liveApp) moveGemini(id string, dir int) string {
	if !rt.geminiReady() {
		return ""
	}
	idx, err := rt.geminiMod.Store.LoadIndex()
	if err != nil {
		return accountError(rt, err)
	}
	next := moveIDs(accountIDs(idx.OrderedAccounts(), func(a gemini.ManagedAccount) string { return a.ID }), id, dir)
	if next == nil {
		return ""
	}
	if _, err := rt.geminiMod.Apply(context.Background(), gemini.Command{Kind: gemini.CommandReorder, IDs: next}); err != nil {
		return accountError(rt, err)
	}
	rt.geminiReload()
	return ""
}

// geminiRefresh reads the quota of the target and reports the failures.
func (rt *liveApp) geminiRefresh(l l10n.Localizer, target gemini.RefreshTarget) string {
	if !rt.geminiReady() {
		return ""
	}
	report := rt.geminiMod.Refresh(context.Background(), target)
	rt.geminiReload()
	return geminiRefreshMessage(l, report)
}

func geminiRefreshMessage(l l10n.Localizer, report gemini.RefreshReport) string {
	var failures []error
	for _, outcome := range report.Outcomes {
		if outcome.Err != nil {
			failures = append(failures, outcome.Err)
		}
	}
	return accountBatchFailure(l, len(report.Outcomes)-len(failures), len(failures), failures)
}

func (rt *liveApp) exportGemini(l l10n.Localizer, ids []string) (string, bool) {
	if len(ids) == 0 {
		return l.Text(l10n.KeyAccountsExportEmpty), false
	}
	if !rt.geminiReady() {
		return tx(l, xDetailUnavailable), false
	}
	path, pickErr := pickExport(l)
	if pickErr != nil {
		return accountFailure(l, l.Text(l10n.KeyAccountsExportFailed), pickErr), false
	}
	if path == "" {
		return "", false
	}
	res, err := rt.geminiMod.ExportAccounts(ids)
	if err != nil {
		if message := accountBatchFailure(l, 0, len(res.Failures), append(res.FailureErrors, err)); message != "" {
			return message, false
		}
		return accountFailure(l, l.Text(l10n.KeyAccountsExportFailed), err), false
	}
	if err := codex.WriteTransferPack(path, res.Pack); err != nil {
		return accountFailure(l, l.Text(l10n.KeyAccountsExportFailed), err), false
	}
	if message := accountBatchFailure(l, res.ExportedCount, len(res.Failures), res.FailureErrors); message != "" {
		return message, false
	}
	return l.Textf(l10n.KeyAccountsExportSucceeded, res.ExportedCount), true
}

// switchGemini makes id the Antigravity login. With restart, a running
// desktop app is quit first and reopened afterwards. Running clients block
// the switch until force is set; running reports that case. ok is false
// when nothing was switched.
func (rt *liveApp) switchGemini(l l10n.Localizer, id string, restart, force bool) (running bool, message string, ok bool) {
	if !rt.geminiReady() {
		return false, l.Text(l10n.KeyGeminiNotInstalled), false
	}
	var reopen func() error
	if restart && geminiRestartSupported {
		start, err := prepareGeminiAppRestart()
		if err != nil {
			if start != nil {
				_ = start()
			}
			return false, l.Text(l10n.KeyGeminiQuitFailed), false
		}
		reopen = start
	}
	_, err := rt.geminiMod.Apply(context.Background(), gemini.Command{Kind: gemini.CommandMakeCurrent, ID: id, AllowWhileRunning: force})
	rt.geminiReload()
	if err != nil && reopen != nil {
		// Nothing changed: give the user their app back.
		_ = reopen()
	}
	var blocked *gemini.ProcessesRunningError
	switch {
	case errors.As(err, &blocked):
		return true, "", false
	case err != nil:
		return false, accountFailure(l, l.Text(l10n.KeyAccountsSwitchFailed), err), false
	case reopen != nil && reopen() != nil:
		return false, l.Text(l10n.KeyGeminiReopenFailed), true
	case reopen != nil && force:
		return false, l.Text(l10n.KeyGeminiSwitchedRestartCLI), true
	case force:
		return false, l.Text(l10n.KeyGeminiSwitchedRestartClients), true
	}
	return false, "", true
}

// switchGeminiDialog switches and, when Antigravity or agy is running,
// asks before switching anyway.
func (rt *liveApp) switchGeminiDialog(l l10n.Localizer, id, name string, restart bool) string {
	message, _ := rt.switchGeminiConfirmed(l, id, name, restart)
	return message
}

// switchGeminiConfirmed is switchGeminiDialog with whether the action is
// over; declining the second question ends it without a switch.
func (rt *liveApp) switchGeminiConfirmed(l l10n.Localizer, id, name string, restart bool) (string, bool) {
	running, message, ok := rt.switchGemini(l, id, restart, false)
	if !running {
		return message, ok
	}
	confirmed, err := confirmDialog(l, l.Text(l10n.KeyGeminiRunningTitle), l.Text(l10n.KeyGeminiRunningMessage), name, l.Text(l10n.KeyAccountsForceCurrent))
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyStatusError), err), false
	}
	if !confirmed {
		return "", true
	}
	_, message, ok = rt.switchGemini(l, id, restart, true)
	return message, ok
}

func installGeminiAccountActions(rt *liveApp, loc func() l10n.Localizer, a *AccountActions) {
	if !rt.geminiReady() {
		return
	}
	a.GeminiRefreshAll = func() string { return rt.geminiRefresh(loc(), gemini.RefreshTarget{Kind: gemini.RefreshAll}) }
	a.GeminiRefresh = func(id string) string {
		return rt.geminiRefresh(loc(), gemini.RefreshTarget{Kind: gemini.RefreshAccount, ID: id})
	}
	a.GeminiSwitch = func(id string, restart, force bool) (bool, string) {
		running, message, _ := rt.switchGemini(loc(), id, restart, force)
		return running, message
	}
	a.GeminiDelete = func(id string) string { return rt.removeGemini(loc(), id) }
}
