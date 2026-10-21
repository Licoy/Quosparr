package app

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

// claudeAPIPlan is the plan label of an API relay account.
const claudeAPIPlan = "API"

// ClaudeAPIInput is the form data of a Claude API relay account. APIKey is
// required when adding; when updating, empty keeps the stored key and an empty
// BaseURL keeps the stored base URL. Apply also rewrites settings.json when
// the account is in use.
type ClaudeAPIInput struct {
	Alias, BaseURL, APIKey                         string
	Model                                          string
	OpusModel, SonnetModel, HaikuModel, FableModel string
	Apply, Context1M                               bool
}

func (in ClaudeAPIInput) profile() claude.APIProfile {
	return claude.APIProfile{
		BaseURL: in.BaseURL, Model: in.Model, OpusModel: in.OpusModel,
		SonnetModel: in.SonnetModel, HaikuModel: in.HaikuModel, FableModel: in.FableModel,
		Context1M: in.Context1M,
	}
}

var errClaudeUnavailable = errors.New("claude: accounts are unavailable")

// addClaudeAPIAccount adds (or re-adds) an API relay account and reads its
// balance once. The balance read is best effort: its failure is recorded on
// the account and does not fail the add.
func (rt *liveApp) addClaudeAPIAccount(ctx context.Context, in ClaudeAPIInput) (claude.ManagedAccount, error) {
	if rt.claudeMod == nil {
		return claude.ManagedAccount{}, errClaudeUnavailable
	}
	acc, err := rt.claudeMod.AddAPIAccount(in.Alias, in.profile(), in.APIKey)
	if err != nil {
		return claude.ManagedAccount{}, err
	}
	_ = rt.claudeMod.Refresh(ctx, acc.ID, true)
	rt.claudeB.reload(true)
	return acc, nil
}

// updateClaudeAPIAccount edits an API account. With Apply, the settings.json
// entries of the account in use follow the edit.
func (rt *liveApp) updateClaudeAPIAccount(ctx context.Context, id string, in ClaudeAPIInput) error {
	if rt.claudeMod == nil {
		return errClaudeUnavailable
	}
	acc, err := rt.claudeMod.EditAPIAccount(id, claude.APIEdit{Alias: in.Alias, Profile: in.profile(), Key: in.APIKey, Apply: in.Apply})
	if err != nil {
		return err
	}
	if acc.APIQuota == nil {
		// A new key or base URL dropped the cached balance.
		_ = rt.claudeMod.Refresh(ctx, acc.ID, true)
	}
	rt.claudeB.reload(true)
	return nil
}

// switchClaudeAccount makes id the Claude Code account, subscription or API
// relay, and returns the error unlocalized.
func (rt *liveApp) switchClaudeAccount(ctx context.Context, id string) error {
	if rt.claudeMod == nil {
		return errClaudeUnavailable
	}
	_, err := rt.claudeMod.Apply(ctx, claude.Command{Kind: claude.CommandSwitch, ID: id, AllowWhileRunning: true})
	if err != nil {
		return err
	}
	rt.claudeB.reload(true)
	return nil
}

// claudeAPIModels lists the models of a relay. With accountID set, the stored
// key of that API account is used and apiKey and baseURL may be empty.
func (rt *liveApp) claudeAPIModels(ctx context.Context, accountID, baseURL, apiKey string) ([]string, error) {
	if rt.claudeMod == nil {
		return nil, errClaudeUnavailable
	}
	if accountID != "" && strings.TrimSpace(apiKey) == "" {
		return rt.claudeMod.ListAccountAPIModels(ctx, accountID)
	}
	return rt.claudeMod.ListAPIModels(ctx, baseURL, apiKey)
}

// --- state ------------------------------------------------------------------

// buildClaudeAPIState is the Claude view while an API relay account is the
// current one: no subscription windows, no gateway notice, the balance meter
// when the relay reports one.
func buildClaudeAPIState(st runway.ClaudeState, lc l10n.Localizer, acc claude.ManagedAccount) runway.ClaudeState {
	st.Gateway = false
	st.Accounts.Display = codex.AccountDisplay{
		Authenticated: true, AccountID: acc.ID, DisplayName: acc.ResolvedDisplayName(),
	}
	st.Plan = claudeAPIPlan
	view := &runway.ClaudeAPIView{ErrorCode: acc.LastError}
	if acc.API != nil {
		view.BaseURL = acc.API.BaseURL
	}
	if acc.APIQuota != nil {
		q := *acc.APIQuota
		view.Quota = &q
	}
	st.API = view
	if acc.LastQuotaAt != nil {
		st.UpdatedAt = *acc.LastQuotaAt
	}
	st.Availability = claudeAPIAvailability(acc)
	relabelClaudeAPI(lc, &st)
	st.Widget = claudeWidget(st)
	return st
}

// claudeAPIAvailability is Ready once a reading exists (a relay without a
// balance endpoint counts), Failed when only an error exists, else Loading.
func claudeAPIAvailability(acc claude.ManagedAccount) runway.ClaudeAvailability {
	switch {
	case acc.APIQuota != nil:
		return runway.ClaudeReady
	case acc.LastError != "":
		return runway.ClaudeFailed
	}
	return runway.ClaudeLoading
}

// relabelClaudeAPI rebuilds the localized parts of an API state.
func relabelClaudeAPI(lc l10n.Localizer, st *runway.ClaudeState) {
	st.Meters = nil
	st.ExtraUsage = ""
	st.LastError = ""
	if st.API == nil {
		return
	}
	if st.API.Quota != nil {
		if m, ok := runway.APIBalanceMeter(lc, *st.API.Quota); ok {
			st.Meters = []runway.Meter{m}
		}
	}
	if st.API.ErrorCode != "" {
		st.LastError = claudeErrorCodeText(lc, st.API.ErrorCode)
	}
}

// claudeAPIAccountQuota is the account-row line of an API account: the plan
// label, the balance detail and the balance meter's bar.
func claudeAPIAccountQuota(lc l10n.Localizer, account claude.ManagedAccount) (runway.ClaudeAccountQuota, bool) {
	if account.APIQuota == nil && account.LastQuotaAt == nil {
		return runway.ClaudeAccountQuota{}, false
	}
	q := runway.ClaudeAccountQuota{Plan: claudeAPIPlan}
	if account.LastQuotaAt != nil {
		q.UpdatedAt = *account.LastQuotaAt
	}
	if account.APIQuota == nil {
		return q, true
	}
	if !account.APIQuota.FetchedAt.IsZero() {
		q.UpdatedAt = account.APIQuota.FetchedAt
	}
	if m, ok := runway.APIBalanceMeter(lc, *account.APIQuota); ok {
		q.Title = m.Title
		q.UsedPercent = m.UsedPercent
		q.Balance = runway.APIBalanceDetail(*account.APIQuota)
	}
	return q, true
}

// applyClaudeAPIBalance adds the balance to the widget block: always the
// preformatted text in the site's unit, and the dollar value when the unit is
// USD and an amount is known.
func applyClaudeAPIBalance(p *export.ProviderSnapshot, q apiquota.Quota) {
	if text := runway.APIBalanceText(q); text != "" {
		p.BalanceText = &text
	}
	if q.Status != apiquota.StatusAvailable || q.Unlimited || q.Remaining == nil || !strings.EqualFold(q.Unit, apiquota.UnitUSD) {
		return
	}
	if d, err := swiftjson.ParseDecimal(strconv.FormatFloat(*q.Remaining, 'f', 2, 64)); err == nil {
		p.BalanceUSD = &d
	}
}
