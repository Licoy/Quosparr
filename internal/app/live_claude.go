package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/pricing"
	"github.com/licoy/quosparr/internal/runway"
	"github.com/licoy/quosparr/internal/speed"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

type claudeBridge struct {
	pricingSnapshot   func() pricing.Snapshot
	pricingGeneration uint64
	mod               *claude.Module
	mu                sync.Mutex
	onChange          func()
	state             runway.ClaudeState
	refreshes         int
	lc                l10n.Localizer
	prefs             store.Preferences
	local             *claude.SessionUsageSummary
	localRange        store.CostRange
	localErr          error
	speed             *speed.Report
	speedErr          error
	generation        uint64
	gateway           bool
	refreshMu         sync.Mutex
	scanMu            sync.Mutex
	sessions          claude.SessionCache
}

func newClaudeBridge(mod *claude.Module, prefs store.Preferences, langs []string) *claudeBridge {
	b := &claudeBridge{mod: mod, prefs: prefs.Clone(), lc: l10n.ForPreference(string(prefs.Language), langs)}
	if mod != nil && mod.Store != nil {
		b.gateway = claudeGatewayConfigured(mod.Store.ConfigDir())
	}
	return b
}

func (g *claudeBridge) State(now time.Time) runway.ClaudeState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := g.state
	if snap, ok := st.Detail.(*claude.UsageSnapshot); ok && snap.ResetGrants != nil && snap.ResetGrants.Shown() {
		c := snap.ResetGrants.CodexSnapshot(now, st.UpdatedAt)
		sum := runway.SummarizeCredits(c)
		st.ResetCredits, st.ResetCreditSummary = &c, &sum
	}
	st.Widget = claudeWidget(st)
	return st
}

func (g *claudeBridge) Refresh(ctx context.Context, force bool) error {
	g.changeRefreshing(1)
	defer g.changeRefreshing(-1)
	g.refreshMu.Lock()
	defer g.refreshMu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if g.mod != nil && g.mod.Store != nil {
		// Follow the Claude Code login first: the first run imports it and an
		// external switch moves the current account.
		if id, err := g.mod.Sync(ctx); err == nil && id != "" {
			// An expired current account is reported, not refreshed.
			_ = g.mod.Refresh(ctx, id, force)
		}
	}
	g.reload(true)
	return ctx.Err()
}

// RefreshLocal rescans the local Claude sessions without touching the usage.
func (g *claudeBridge) RefreshLocal(ctx context.Context) error {
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

func (g *claudeBridge) SetOnChange(fn func()) {
	g.mu.Lock()
	g.onChange = fn
	g.mu.Unlock()
}

func (g *claudeBridge) setPreferences(p store.Preferences, langs []string) {
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

func (g *claudeBridge) reload(notify bool) {
	g.mu.Lock()
	lc, mod, gateway := g.lc, g.mod, g.gateway
	g.mu.Unlock()
	next := buildClaudeState(mod, lc, gateway)
	g.mu.Lock()
	next.Refreshing = g.refreshes > 0
	g.relabelLocked(&next)
	applyClaudeLocal(&next, g.local)
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

func (g *claudeBridge) relabelLocked(st *runway.ClaudeState) {
	st.SpeedError = ""
	if st.API != nil {
		relabelClaudeAPI(g.lc, st)
	} else {
		st.LastError = claudeErrorText(g.lc, st.Availability)
		if snap, ok := st.Detail.(*claude.UsageSnapshot); ok {
			st.Meters = claudeMeters(g.lc, *snap, st.UpdatedAt)
			st.ExtraUsage = claudeExtraLine(snap.Extra)
		}
	}
	if g.localErr != nil {
		st.LastError = g.lc.Text(l10n.KeyStatusError) + ": " + g.localErr.Error()
	}
	if g.prefs.ShowsSpeedAnalysis && g.speedErr != nil {
		st.SpeedError = g.speedErr.Error()
	}
}

func buildClaudeState(mod *claude.Module, lc l10n.Localizer, gateway bool) runway.ClaudeState {
	st := runway.ClaudeState{Availability: runway.ClaudeNotLoggedIn, Gateway: gateway}
	if mod == nil || mod.Store == nil {
		st.LastError = lc.Text(l10n.KeyClaudeNotLoggedIn)
		return st
	}
	idx, err := mod.Store.LoadIndex()
	if err != nil {
		st.Availability = runway.ClaudeCredentialStoreUnavailable
		st.LastError = lc.Text(l10n.KeyClaudeCredentialUnavailable)
		return st
	}
	st.Accounts.ActiveID = idx.CurrentAccountID
	ordered := idx.OrderedAccounts()
	for _, account := range ordered {
		st.Accounts.Sidebar = append(st.Accounts.Sidebar, claudeSidebarAccount(lc, account))
	}
	st.AccountQuotas = claudeAccountQuotas(lc, ordered)
	if idx.CurrentAccountID == "" {
		if gateway {
			st.Availability = runway.ClaudeAPIKeyMode
			st.LastError = lc.Text(l10n.KeyClaudeAPIKeyMode)
		} else {
			st.LastError = lc.Text(l10n.KeyClaudeNotLoggedIn)
		}
		return st
	}
	acc, ok := idx.Account(idx.CurrentAccountID)
	if !ok {
		st.LastError = lc.Text(l10n.KeyClaudeNotLoggedIn)
		return st
	}
	if acc.IsAPI() {
		return buildClaudeAPIState(st, lc, acc)
	}
	st.Accounts.Display = codex.AccountDisplay{
		Authenticated: !acc.RequiresReauth, AccountID: acc.ID,
		DisplayName: acc.ResolvedDisplayName(), Email: acc.Identity.Email,
	}
	st.Plan = acc.Tier().DisplayName()
	var snap *claude.UsageSnapshot
	if len(acc.CachedQuota) > 0 {
		parsed, perr := claude.ParseUsageSnapshot(acc.CachedQuota)
		if perr != nil {
			st.Availability = runway.ClaudeUsageParseFailed
			st.LastError = lc.Text(l10n.KeyClaudeUsageParseFailed)
			return st
		}
		snap = &parsed
	}
	st.Availability = claudeAvailability(&acc, snap != nil, gateway)
	st.LastError = claudeErrorText(lc, st.Availability)
	if acc.LastQuotaAt != nil {
		st.UpdatedAt = *acc.LastQuotaAt
	}
	if snap == nil {
		return st
	}
	if !snap.FetchedAt.IsZero() {
		st.UpdatedAt = snap.FetchedAt
	}
	st.Meters = claudeMeters(lc, *snap, st.UpdatedAt)
	st.ExtraUsage = claudeExtraLine(snap.Extra)
	st.Detail = snap
	st.Widget = claudeWidget(st)
	return st
}

func claudeAvailability(acc *claude.ManagedAccount, hasSnap, gateway bool) runway.ClaudeAvailability {
	if !acc.Identity.HasAccessToken && gateway {
		return runway.ClaudeAPIKeyMode
	}
	switch acc.LastError {
	case claude.ErrorCodeReauth:
		return runway.ClaudeReauthRequired
	case claude.ErrorCodeTokenExpired:
		return runway.ClaudeTokenExpired
	case claude.ErrorCodeRateLimited:
		return runway.ClaudeRateLimited
	case claude.ErrorCodeUsageParse:
		if !hasSnap {
			return runway.ClaudeUsageParseFailed
		}
	}
	if acc.RequiresReauth {
		return runway.ClaudeReauthRequired
	}
	if hasSnap || acc.LastError == "" {
		if hasSnap {
			return runway.ClaudeReady
		}
		return runway.ClaudeLoading
	}
	return runway.ClaudeFailed
}

func claudeErrorText(lc l10n.Localizer, av runway.ClaudeAvailability) string {
	switch av {
	case runway.ClaudeNotLoggedIn:
		return lc.Text(l10n.KeyClaudeNotLoggedIn)
	case runway.ClaudeAPIKeyMode:
		return lc.Text(l10n.KeyClaudeAPIKeyMode)
	case runway.ClaudeTokenExpired:
		return lc.Text(l10n.KeyClaudeTokenExpired)
	case runway.ClaudeReauthRequired:
		return lc.Text(l10n.KeyClaudeReauthRequired)
	case runway.ClaudeRateLimited:
		return lc.Text(l10n.KeyClaudeRateLimited)
	case runway.ClaudeUsageParseFailed:
		return lc.Text(l10n.KeyClaudeUsageParseFailed)
	case runway.ClaudeCredentialStoreUnavailable, runway.ClaudeFailed:
		return lc.Text(l10n.KeyClaudeCredentialUnavailable)
	default:
		return ""
	}
}

func claudeMeters(lc l10n.Localizer, snap claude.UsageSnapshot, now time.Time) []runway.Meter {
	var meters []runway.Meter
	for _, w := range snap.Windows {
		if w.Null || w.UsedPercent == nil {
			continue
		}
		used := clampPercent(int(math.Round(*w.UsedPercent)))
		var title string
		minutes, source := 0, runway.SourceStandard
		switch w.Kind {
		case claude.WindowFiveHour:
			title, minutes = lc.Text(l10n.KeyFiveHourUsage), runway.FiveHourMinutes
		case claude.WindowSevenDay:
			title, minutes = lc.Text(l10n.KeyWeeklyUsage), runway.WeeklyMinutes
		case claude.WindowSevenDayModel:
			title, minutes, source = claude.ModelTitle(w.Model)+" · "+lc.Text(l10n.KeyWeeklyUsage), runway.WeeklyMinutes, runway.SourceModelSpecific
		default:
			// Windows of other products have no stable title to show.
			continue
		}
		window := runway.MeterWindow{UsedPercent: used, WindowMinutes: minutes}
		if w.ResetsAt != nil {
			window.ResetsAt = *w.ResetsAt
		}
		meters = append(meters, runway.NewMeter(title, window, now, runway.DefaultMarkers(), source))
	}
	return meters
}

func claudeExtraLine(extra *claude.ExtraUsage) string {
	if extra == nil {
		return ""
	}
	if extra.IsEnabled != nil && !*extra.IsEnabled {
		return ""
	}
	used, limit := "—", "—"
	if s := extra.FormatAmount(extra.UsedCredits); s != "" {
		used = s
	}
	if s := extra.FormatAmount(extra.MonthlyLimit); s != "" {
		limit = s
	}
	if used == "—" && limit == "—" && extra.Currency == "" {
		return ""
	}
	line := used + " / " + limit
	if extra.Currency != "" {
		line += " " + extra.Currency
	}
	return line
}

func claudeSidebarAccount(lc l10n.Localizer, account claude.ManagedAccount) codex.ManagedAccount {
	row := codex.ManagedAccount{
		ID: account.ID, SortIndex: account.SortIndex, DisplayName: account.ResolvedDisplayName(),
		Email: account.Identity.Email, Alias: account.Alias,
		RequiresReauth: account.RequiresReauth, LastQuotaAt: account.LastQuotaAt,
	}
	if account.LastError != "" {
		row.LastError = claudeErrorCodeText(lc, account.LastError)
	}
	if account.IsAPI() && account.API != nil {
		// The shared account rows read a relay from these codex fields.
		row.AuthMode, row.APIBaseURL, row.APIModel = codex.AuthModeAPIKey, account.API.BaseURL, account.API.Model
		row.APIQuota = account.APIQuota
	}
	return row
}

func claudeErrorCodeText(lc l10n.Localizer, code string) string {
	switch code {
	case claude.ErrorCodeReauth:
		return lc.Text(l10n.KeyClaudeReauthRequired)
	case claude.ErrorCodeTokenExpired:
		return lc.Text(l10n.KeyClaudeTokenExpired)
	case claude.ErrorCodeRateLimited:
		return lc.Text(l10n.KeyClaudeRateLimited)
	case claude.ErrorCodeUsageParse:
		return lc.Text(l10n.KeyClaudeUsageParseFailed)
	case claude.ErrorCodeAPIUnauthorized:
		return tx(lc, xAPIKeyRejected)
	case claude.ErrorCodeAPI:
		return tx(lc, xAPIBalanceFailed)
	default:
		return lc.Text(l10n.KeyStatusError)
	}
}

func claudeAccountQuotas(lc l10n.Localizer, accounts []claude.ManagedAccount) map[string]runway.ClaudeAccountQuota {
	out := map[string]runway.ClaudeAccountQuota{}
	for _, account := range accounts {
		if account.IsAPI() {
			if q, ok := claudeAPIAccountQuota(lc, account); ok {
				out[account.ID] = q
			}
			continue
		}
		if len(account.CachedQuota) == 0 {
			continue
		}
		snap, err := claude.ParseUsageSnapshot(account.CachedQuota)
		if err != nil {
			continue
		}
		q := runway.ClaudeAccountQuota{Plan: account.Tier().DisplayName(), ExtraUsage: claudeExtraLine(snap.Extra)}
		if account.LastQuotaAt != nil {
			q.UpdatedAt = *account.LastQuotaAt
		}
		if !snap.FetchedAt.IsZero() {
			q.UpdatedAt = snap.FetchedAt
		}
		q.Meters = claudeMeters(lc, snap, q.UpdatedAt)
		if len(q.Meters) > 0 {
			m := q.Meters[0]
			q.Title, q.UsedPercent, q.ResetsAt = m.Title, m.UsedPercent, m.ResetsAt
		}
		q.ResetCreditsUpdatedAt = q.UpdatedAt
		if snap.ResetGrants != nil && snap.ResetGrants.Eligible {
			for _, g := range snap.ResetGrants.Grants {
				if !g.Paused {
					one := claude.ResetGrantStatus{Eligible: true, Grants: []claude.ResetGrant{g}}
					count := one.CodexSnapshot(g.StartsAt, q.UpdatedAt).AvailableCount
					q.ResetGrants = append(q.ResetGrants, runway.AccountResetGrant{Count: count, StartsAt: g.StartsAt, EndsAt: g.EndsAt})
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

func claudeWidget(st runway.ClaudeState) *export.ProviderSnapshot {
	p := export.ProviderSnapshot{
		Provider: export.ProviderClaude, Availability: claudeWidgetAvailability(st.Availability),
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
	if st.API != nil && st.API.Quota != nil {
		applyClaudeAPIBalance(&p, *st.API.Quota)
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
	if st.ResetCreditSummary != nil {
		r := export.NewResetCredits(st.ResetCreditSummary.AvailableCount, st.ResetCreditSummary.ExpiringCount)
		p.ResetCredits = &r
	}
	out := export.NewProviderSnapshot(p)
	return &out
}

func claudeWidgetAvailability(av runway.ClaudeAvailability) export.Availability {
	switch av {
	case runway.ClaudeReady, runway.ClaudeRateLimited, runway.ClaudeTokenExpired:
		return export.Available
	case runway.ClaudeNotLoggedIn, runway.ClaudeReauthRequired, runway.ClaudeAPIKeyMode:
		return export.NotLoggedIn
	default:
		return export.Unavailable
	}
}

func claudeCLIVersion(configDir string) string {
	exe, err := claude.ClaudeExecutable()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "--version")
	platform.HideConsoleWindow(cmd)
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return claude.ParseCLIVersion(string(out))
}

// claudeGatewayConfigured reports whether settings.json names a gateway env
// key. Values are not decoded.
func claudeGatewayConfigured(configDir string) bool {
	data, err := os.ReadFile(filepath.Join(configDir, "settings.json"))
	if err != nil {
		return false
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(data, &root) != nil {
		return false
	}
	var env map[string]json.RawMessage
	if json.Unmarshal(root["env"], &env) != nil {
		return false
	}
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX"} {
		if _, ok := env[key]; ok {
			return true
		}
	}
	return false
}

func claudePasteIdentity(ctx context.Context, mod *claude.Module, data []byte) (claude.CredentialIdentity, error) {
	doc, err := claude.ParseAuthDocument(data)
	if err != nil {
		return claude.CredentialIdentity{}, err
	}
	if mod == nil || mod.Usage == nil || doc.OAuth().AccessToken == "" {
		return claude.CredentialIdentity{}, claude.ErrIdentityRequired
	}
	raw, _, err := mod.Usage.FetchProfile(ctx, doc.OAuth().AccessToken)
	if err != nil {
		return claude.CredentialIdentity{}, err
	}
	id, err := claude.IdentityFromProfile(raw)
	if err != nil || id.AccountUUID == "" {
		return claude.CredentialIdentity{}, claude.ErrIdentityRequired
	}
	return id, nil
}

func (rt *liveApp) importClaudeOfficial(l l10n.Localizer) string {
	if rt.claudeMod == nil {
		return l.Text(l10n.KeyClaudeNotLoggedIn)
	}
	if _, err := rt.claudeMod.Apply(context.Background(), claude.Command{Kind: claude.CommandImportOfficial}); err != nil {
		return claudePublicError(l, err)
	}
	rt.claudeB.reload(true)
	return ""
}

func (rt *liveApp) claudePasteText(ctx context.Context, l l10n.Localizer, text string) (string, bool) {
	if rt.claudeMod == nil {
		return l.Text(l10n.KeyClaudeNotLoggedIn), false
	}
	data, err := claude.ParsePasted(text)
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyAccountsImportFailed), err), false
	}
	id, err := claudePasteIdentity(ctx, rt.claudeMod, data)
	if err != nil {
		return claudePublicError(l, err), false
	}
	if _, err := rt.claudeMod.Apply(ctx, claude.Command{Kind: claude.CommandPaste, Text: text, Identity: id}); err != nil {
		return claudePublicError(l, err), false
	}
	rt.claudeB.reload(true)
	return "", true
}

func (rt *liveApp) importClaudeFile(ctx context.Context, l l10n.Localizer, path string) (string, bool) {
	data, err := claude.ReadCredentialFile(path)
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyAccountsImportFailed), err), false
	}
	return rt.claudePasteText(ctx, l, string(data))
}

func (rt *liveApp) switchClaude(l l10n.Localizer, id string) string {
	if rt.claudeMod == nil {
		return l.Text(l10n.KeyClaudeNotLoggedIn)
	}
	if err := rt.switchClaudeAccount(context.Background(), id); err != nil {
		return claudePublicError(l, err)
	}
	return ""
}

func (rt *liveApp) removeClaude(l l10n.Localizer, id string) string {
	if rt.claudeMod == nil {
		return l.Text(l10n.KeyStatusError)
	}
	if _, err := rt.claudeMod.Apply(context.Background(), claude.Command{Kind: claude.CommandRemove, ID: id}); err != nil {
		return claudePublicError(l, err)
	}
	rt.claudeB.reload(true)
	return ""
}

func (rt *liveApp) reorderClaude(ids []string) string {
	if rt.claudeMod == nil {
		return ""
	}
	if _, err := rt.claudeMod.Apply(context.Background(), claude.Command{Kind: claude.CommandReorder, IDs: ids}); err != nil {
		return accountError(rt, err)
	}
	rt.claudeB.reload(true)
	return ""
}

// refreshClaude polls one account. The current account is never token
// refreshed; an expired login reports the run-claude hint.
func (rt *liveApp) refreshClaude(ctx context.Context, l l10n.Localizer, id string) string {
	if rt.claudeMod == nil {
		return ""
	}
	err := rt.claudeMod.Refresh(ctx, id, true)
	rt.claudeB.reload(true)
	if err != nil {
		return claudePublicError(l, err)
	}
	return ""
}

func (rt *liveApp) refreshClaudeAll(ctx context.Context) string {
	if rt.claudeMod == nil || rt.claudeMod.Store == nil {
		return ""
	}
	idx, err := rt.claudeMod.Store.LoadIndex()
	if err != nil {
		return accountError(rt, err)
	}
	var failures []error
	for _, account := range idx.OrderedAccounts() {
		if err := rt.claudeMod.Refresh(ctx, account.ID, true); err != nil {
			failures = append(failures, err)
		}
	}
	l := l10n.New(rt.svc.Snapshot().Language)
	message := accountBatchFailure(l, len(idx.Accounts)-len(failures), len(failures), failures)
	if err := rt.claudeB.scanLocal(ctx); err != nil {
		if message != "" {
			message += "; "
		}
		message += accountFailure(l, l.Text(l10n.KeyStatusError), err)
	}
	rt.claudeB.reload(true)
	return message
}

func claudePublicError(l l10n.Localizer, err error) string {
	var current *claude.CurrentAccountCannotBeRemovedError
	switch {
	case errors.As(err, &current):
		return l.Text(l10n.KeyClaudeCurrentAccountCannotDelete)
	case errors.Is(err, claude.ErrNotLoggedIn):
		return l.Text(l10n.KeyClaudeNotLoggedIn)
	case errors.Is(err, claude.ErrTokenExpired):
		return l.Text(l10n.KeyClaudeTokenExpired)
	case errors.Is(err, claude.ErrInvalidGrant), errors.Is(err, claude.ErrIdentityRequired):
		return l.Text(l10n.KeyClaudeReauthRequired)
	default:
		return apiErrorText(l, err)
	}
}
