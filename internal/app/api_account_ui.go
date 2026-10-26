package app

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/runway"
)

// Editor modes of the API account form.
const (
	modeClaudeAPIAdd  = "claude-api"
	modeClaudeAPIEdit = "claude-api-edit"
	modeCodexAPIAdd   = "codex-api"
	modeCodexAPIEdit  = "codex-api-edit"
)

const apiModelsTimeout = 20 * time.Second

// apiDraft is the API account form. The key is held only while the form is
// open and is never shown back: editing starts with it empty.
type apiDraft struct {
	baseURL, key                      string
	model, opus, sonnet, haiku, fable string
	context1M, initialContext1M       bool
	models                            []string
	modelsMessage                     string
	modelsFailed, fetching            bool
	// inUse: the edited account is the one the config files point at;
	// outdated: they spell its settings differently, so applying is worth
	// offering even without a change; initial is its profile when the form
	// opened. askApply shows the choice between saving only and also
	// rewriting those files.
	inUse, outdated, askApply bool
	initial                   [6]string
}

func (d *apiDraft) profileFields() [6]string {
	return [6]string{strings.TrimSpace(d.baseURL), strings.TrimSpace(d.model), strings.TrimSpace(d.opus),
		strings.TrimSpace(d.sonnet), strings.TrimSpace(d.haiku), strings.TrimSpace(d.fable)}
}

// apiNeedsApplyChoice reports whether saving the form would change what the
// config files of the account in use say: a new key, site or model, or a
// base URL they still spell the old way.
func apiNeedsApplyChoice(f *accountForm) bool {
	d := &f.api
	return isAPIEditMode(f.mode) && d.inUse && (d.outdated || strings.TrimSpace(d.key) != "" || d.profileFields() != d.initial || d.context1M != d.initialContext1M)
}

func isAPIMode(mode string) bool {
	switch mode {
	case modeClaudeAPIAdd, modeClaudeAPIEdit, modeCodexAPIAdd, modeCodexAPIEdit:
		return true
	}
	return false
}

func isAPIEditMode(mode string) bool {
	return mode == modeClaudeAPIEdit || mode == modeCodexAPIEdit
}

func apiEditorTitle(l l10n.Localizer, mode string) string {
	if isAPIEditMode(mode) {
		return tx(l, xAPIEdit)
	}
	return tx(l, xAPIAdd)
}

// apiEditorIncomplete disables the confirmation while a new account lacks
// its key, or a new Claude account its base URL.
func apiEditorIncomplete(f *accountForm) bool {
	d := &f.api
	switch f.mode {
	case modeClaudeAPIAdd:
		return strings.TrimSpace(d.key) == "" || strings.TrimSpace(d.baseURL) == ""
	case modeCodexAPIAdd:
		return strings.TrimSpace(d.key) == ""
	}
	return false
}

// openClaudeAPIEditor fills the form from a stored Claude API account.
func (f *accountForm) openClaudeAPIEditor(rt *liveApp, id string) {
	f.mode, f.editID, f.api = modeClaudeAPIEdit, id, apiDraft{}
	if rt == nil || rt.claudeMod == nil || rt.claudeMod.Store == nil {
		return
	}
	idx, err := rt.claudeMod.Store.LoadIndex()
	if err != nil {
		return
	}
	acc, ok := idx.Account(id)
	if !ok || acc.API == nil {
		return
	}
	p := acc.API
	f.alias = acc.Alias
	f.api = apiDraft{baseURL: p.BaseURL, model: p.Model, opus: p.OpusModel, sonnet: p.SonnetModel, haiku: p.HaikuModel, fable: p.FableModel, context1M: p.Context1M, initialContext1M: p.Context1M}
	f.api.inUse, f.api.outdated = rt.claudeMod.APIAccountState(id)
	f.api.initial = f.api.profileFields()
}

// openCodexAPIEditor fills the form from a stored Codex API account.
func (f *accountForm) openCodexAPIEditor(rt *liveApp, acc codex.ManagedAccount) {
	f.mode, f.editID, f.alias = modeCodexAPIEdit, acc.ID, acc.Alias
	f.api = apiDraft{baseURL: acc.APIBaseURL, model: acc.APIModel}
	f.api.inUse = rt != nil && rt.codex != nil && rt.codex.RelayInUse(acc.ID)
	f.api.initial = f.api.profileFields()
}

// apiFieldsMaxHeight keeps the Claude form, with its model mapping, inside
// the settings window; the fields scroll past it.
const apiFieldsMaxHeight = 460

func apiAccountFields(c *ui.Context, u *settingsUI, f *accountForm) {
	l := u.L()
	hint := tx(l, xAPIHintCodex)
	if isClaudeAPIMode(f.mode) {
		hint = tx(l, xAPIHintClaude)
	}
	ui.Text(c, hint).FontSize(12).TextColor(c.Theme().TextMuted)
	ui.Scroll(c.Key("api-fields")).MaxHeight(apiFieldsMaxHeight).Gap(12).Padding(2).Children(func() {
		apiFieldList(c, u, f)
	})
}

func apiFieldList(c *ui.Context, u *settingsUI, f *accountForm) {
	l, d := u.L(), &f.api
	claudeForm := isClaudeAPIMode(f.mode)
	baseLabel := tx(l, xAPIBaseURLOptional)
	if claudeForm {
		baseLabel = tx(l, xAPIBaseURL)
	}
	ui.Field(c, l.Text(l10n.KeyAlias), func() {
		ui.TextInput(c.Key("api-alias"), &f.alias).Disabled(f.busy)
	})
	ui.Field(c, baseLabel, func() {
		ui.TextInput(c.Key("api-base"), &d.baseURL).Placeholder("https://").Disabled(f.busy)
	})
	ui.Field(c, tx(l, xAPIKey), func() {
		in := ui.TextInput(c.Key("api-key"), &d.key).Password().Disabled(f.busy)
		if isAPIEditMode(f.mode) {
			in.Placeholder(tx(l, xAPIKeyKeep))
		}
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if PillButton(c, nil, tx(l, xAPIFetchModels), f.busy || d.fetching || !apiCanFetchModels(f)).Clicked() {
			u.fetchAPIModels(f)
		}
		if d.fetching {
			ui.Spinner(c).Size(12, 12)
		}
		if d.modelsMessage != "" {
			ink := c.Theme().TextMuted
			if d.modelsFailed {
				ink = c.Theme().Danger
			}
			ui.Text(c, d.modelsMessage).FontSize(11).TextColor(ink).MaxLines(2).Shrink(1)
		}
	})
	apiModelInput(c, f, "api-model", &d.model, tx(l, xAPIModel))
	if !claudeForm {
		return
	}
	ui.Checkbox(c.Key("api-context-1m"), &d.context1M, tx(l, xAPIContext1M)).Disabled(f.busy)
	ui.Text(c, tx(l, xAPIContext1MHint)).FontSize(11).TextColor(c.Theme().TextMuted)
	ui.Text(c, tx(l, xAPIModelMapping)).FontSize(12).TextColor(c.Theme().TextMuted)
	apiModelPair(c, f, [2]string{"api-opus", "api-sonnet"}, [2]*string{&d.opus, &d.sonnet}, [2]string{tx(l, xAPIModelOpus), tx(l, xAPIModelSonnet)})
	apiModelPair(c, f, [2]string{"api-haiku", "api-fable"}, [2]*string{&d.haiku, &d.fable}, [2]string{tx(l, xAPIModelHaiku), tx(l, xAPIModelFable)})
}

func isClaudeAPIMode(mode string) bool {
	return mode == modeClaudeAPIAdd || mode == modeClaudeAPIEdit
}

// apiModelPair lays two model fields side by side.
func apiModelPair(c *ui.Context, f *accountForm, keys [2]string, values [2]*string, labels [2]string) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		for i := range keys {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				apiModelInput(c, f, keys[i], values[i], labels[i])
			})
		}
	})
}

// apiModelInput is a labelled free model field that suggests the fetched
// models.
func apiModelInput(c *ui.Context, f *accountForm, key string, value *string, label string) {
	ui.Field(c, label, func() {
		modelCombo(c.Key(key), value, f.api.models, f.busy)
	})
}

// modelComboMaxHeight bounds the list of fetched models below a model field.
const modelComboMaxHeight = 260

// modelCombo is a text input that takes any model name and lists options
// below it: all of them on a click or Down, those containing the text while
// typing. The theme's Autocomplete stays closed until something is typed,
// so a fetched list could not be browsed.
func modelCombo(c *ui.Context, value *string, options []string, disabled bool) {
	t := c.Theme()
	cb := ui.ComboboxBase(c, value)
	in := cb.Input.Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).Background(t.Surface).Disabled(disabled)
	if in.Focused() {
		in.Border(1, t.Accent)
	} else {
		in.Border(1, t.Border)
	}
	shown := matchingModels(options, *value, cb.Filtering())
	if len(shown) > 0 {
		cb.Popup(func(panel ui.Element) {
			panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).Radius(t.Radius+2).
				Background(t.Background).Border(1, t.Border).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
			ui.Scroll(c).MaxHeight(modelComboMaxHeight).Children(func() {
				for _, opt := range shown {
					item := cb.Item(opt).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
					if item.Highlighted() {
						item.Background(t.Accent).TextColor(t.AccentText)
					}
					item.Children(func() { ui.Text(c, opt).SingleLine().Ellipsis("…") })
				}
			})
		})
	}
	if v, ok := cb.Chosen(); ok {
		*value = v
	}
}

// matchingModels is every option while the list was opened by a click, or
// those containing the typed text, the ones starting with it first.
func matchingModels(options []string, text string, typed bool) []string {
	q := strings.ToLower(strings.TrimSpace(text))
	if !typed || q == "" {
		return options
	}
	var first, rest []string
	for _, o := range options {
		switch lo := strings.ToLower(o); {
		case strings.HasPrefix(lo, q):
			first = append(first, o)
		case strings.Contains(lo, q):
			rest = append(rest, o)
		}
	}
	return append(first, rest...)
}

// apiCanFetchModels needs a base URL and a key, or a stored account whose
// saved key can be used.
func apiCanFetchModels(f *accountForm) bool {
	d := &f.api
	if strings.TrimSpace(d.key) != "" {
		return strings.TrimSpace(d.baseURL) != "" || f.mode == modeCodexAPIAdd || f.mode == modeCodexAPIEdit
	}
	return isAPIEditMode(f.mode) && f.editID != ""
}

// fetchAPIModels asks the site for its models with the draft, or with the
// stored key of the edited account when no key was typed.
func (u *settingsUI) fetchAPIModels(f *accountForm) {
	rt := currentRuntime()
	if rt == nil || f.api.fetching {
		return
	}
	l, d := u.L(), &f.api
	mode, id, base, key := f.mode, f.editID, strings.TrimSpace(d.baseURL), strings.TrimSpace(d.key)
	d.fetching, d.modelsMessage, d.modelsFailed = true, "", false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), apiModelsTimeout)
		defer cancel()
		var (
			models []string
			err    error
		)
		if isClaudeAPIMode(mode) {
			models, err = rt.claudeAPIModels(ctx, apiEditID(mode, id), base, key)
		} else {
			models, err = rt.codexAPIModels(ctx, apiEditID(mode, id), base, key)
		}
		u.post(func() {
			if f.mode != mode || f.editID != id {
				return // the form was closed or reopened meanwhile
			}
			d.fetching = false
			switch {
			case err != nil:
				d.modelsMessage, d.modelsFailed = apiModelsErrorText(l, err), true
			case len(models) == 0:
				d.modelsMessage, d.modelsFailed = tx(l, xAPIModelsEmpty), true
			default:
				d.models, d.modelsMessage = models, tx(l, xAPIModelsFound, len(models))
			}
		})
	}()
}

func apiEditID(mode, id string) string {
	if isAPIEditMode(mode) {
		return id
	}
	return ""
}

// apiApply is what the user chose for the config files of the account in use.
type apiApply int

const (
	apiApplyNone     apiApply = iota // not in use, or nothing they hold changed
	apiApplySaveOnly                 // keep the files until the next switch
	apiApplyWrite                    // rewrite them now
)

// submitAPIAccount adds or saves the API account of the form. apiApplyWrite
// also rewrites the config files of the account in use; the message then asks
// to restart the CLI, which reads them only when it starts.
func submitAPIAccount(l l10n.Localizer, mode, id, alias string, d apiDraft, apply apiApply) (string, bool) {
	rt := currentRuntime()
	if rt == nil {
		return tx(l, xDetailUnavailable), false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*apiModelsTimeout)
	defer cancel()
	var err error
	switch mode {
	case modeClaudeAPIAdd, modeClaudeAPIEdit:
		in := ClaudeAPIInput{
			Alias: alias, BaseURL: d.baseURL, APIKey: d.key, Model: d.model,
			OpusModel: d.opus, SonnetModel: d.sonnet, HaikuModel: d.haiku, FableModel: d.fable, Apply: apply == apiApplyWrite, Context1M: d.context1M,
		}
		if mode == modeClaudeAPIAdd {
			_, err = rt.addClaudeAPIAccount(ctx, in)
		} else {
			err = rt.updateClaudeAPIAccount(ctx, id, in)
		}
	default:
		in := CodexAPIInput{Alias: alias, BaseURL: d.baseURL, APIKey: d.key, Model: d.model, Apply: apply == apiApplyWrite}
		if mode == modeCodexAPIAdd {
			err = rt.addCodexAPIAccount(ctx, in)
		} else {
			err = rt.updateCodexAPIAccount(ctx, id, in)
		}
	}
	if err != nil {
		if text := apiErrorText(l, err); text != "" {
			return text, false
		}
		return l.Text(l10n.KeyStatusError), false
	}
	if isAPIEditMode(mode) {
		return apiSavedText(l, mode, apply), true
	}
	return tx(l, xAPIAdded), true
}

// apiSavedText is the notice after an edit; for the account in use it says
// whether the config files changed and, if so, to restart the CLI.
func apiSavedText(l l10n.Localizer, mode string, apply apiApply) string {
	switch apply {
	case apiApplyNone:
		return tx(l, xAPISaved)
	case apiApplySaveOnly:
		return tx(l, xAPISavedNotApplied)
	}
	product := "Codex"
	if isClaudeAPIMode(mode) {
		product = "Claude Code"
	}
	return tx(l, xAPISavedApplied, product)
}

// apiErrorText localizes API validation and classifies other failures safely.
func apiErrorText(l l10n.Localizer, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, claude.ErrAPIKeyRequired), errors.Is(err, codex.ErrRelayKeyRequired):
		return tx(l, xAPIKeyRequired)
	case errors.Is(err, claude.ErrAPIKeyInvalid), errors.Is(err, codex.ErrRelayKeyInvalid):
		return tx(l, xAPIKeyInvalid)
	case errors.Is(err, claude.ErrAPIBaseURL), errors.Is(err, codex.ErrRelayBaseURL),
		errors.Is(err, apiquota.ErrEmptyBaseURL), errors.Is(err, apiquota.ErrInvalidBaseURL):
		return tx(l, xAPIBaseURLInvalid)
	case errors.Is(err, claude.ErrAPIDuplicate), errors.Is(err, codex.ErrRelayDuplicate):
		return tx(l, xAPIDuplicate)
	case errors.Is(err, claude.ErrAPIKeyMissing):
		return tx(l, xAPIKeyMissing)
	case errors.Is(err, claude.ErrAPIUnauthorized), errors.Is(err, apiquota.ErrUnauthorized):
		return tx(l, xAPIKeyRejected)
	case errors.Is(err, claude.ErrAPIUnavailable):
		return accountFailure(l, tx(l, xAPIBalanceFailed), err)
	}
	if key := netx.L10nKey(err); key != "" {
		return accountFailure(l, l.Text(key), err)
	}
	return accountErrorReason(l, err)
}

func apiModelsErrorText(l l10n.Localizer, err error) string {
	if err == nil {
		return ""
	}
	return tx(l, xAPIModelsFailed) + "\n" + apiErrorText(l, err)
}

// apiBalanceSummary is the one-line balance of an API account row: the
// amount (with the total when known) and when it was read, or why there is
// none. "" before the first reading.
func apiBalanceSummary(l l10n.Localizer, q *apiquota.Quota) string {
	if q == nil {
		return ""
	}
	switch q.Status {
	case apiquota.StatusAvailable:
	case apiquota.StatusUnsupported:
		return tx(l, xAPIBalanceNone)
	case apiquota.StatusUnauthorized:
		return tx(l, xAPIKeyRejected)
	default:
		return tx(l, xAPIBalanceFailed)
	}
	text := apiBalanceValue(l, *q)
	if text == "" {
		return tx(l, xAPIBalanceNone)
	}
	return accountUpdatedSummary(l, l.Text(l10n.KeyWidgetBalance)+" "+text, q.FetchedAt)
}

// apiBalanceValue is the amount shown for a reading: remaining (of total),
// "Unlimited", or what was used when only that is known.
func apiBalanceValue(l l10n.Localizer, q apiquota.Quota) string {
	switch {
	case q.Status != apiquota.StatusAvailable:
		return ""
	case q.Unlimited:
		return tx(l, xAPIUnlimited)
	case q.Remaining == nil && q.Used != nil:
		return tx(l, xAPIBalanceUsed, runway.APIBalanceAmount(q.Unit, *q.Used))
	}
	return runway.APIBalanceDetail(q)
}

// apiBalanceBlock is the balance part of an account card: a bar when the
// total is known, else the amount alone.
func apiBalanceBlock(c *ui.Context, l l10n.Localizer, q *apiquota.Quota, view accountQuotaOptions) {
	if q == nil {
		calloutDim(c, l.Text(l10n.KeyNotLoaded)).FontSize(fsCaption)
		return
	}
	value := apiBalanceValue(l, *q)
	if value == "" {
		calloutDim(c, apiBalanceSummary(l, q)).FontSize(fsCaption)
		return
	}
	title := l.Text(l10n.KeyWidgetBalance)
	f, ok := q.UsedFraction()
	if !ok {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(11).FontWeight(600).Grow(1).Basis(0)
			ui.Text(c, value).FontSize(fsCallout).FontWeight(600).FontFeatures("tnum").SingleLine()
		})
		return
	}
	used := min(100, max(0, int(f*100+0.5)))
	accountMeter(c, l, meterSpec{title: title, remaining: 100 - used, used: used, trailing: value, current: view.current}, view)
}

// apiTierBadge is the "API" plan badge of a relay account, the same for
// every provider.
func apiTierBadge(c *ui.Context, l l10n.Localizer) {
	tierBadgePadded(c, codexTierLook(c.Theme(), codex.TierAPI), l.Text(l10n.KeyPlanAPI), 1, 5)
}

// panelAPIBalance is the quota section of the main panel while an API relay
// account is current: the balance, a bar when the total is known, the site
// and when it was read, or why there is no balance. Privacy mode masks the
// site the way it masks the account name.
func panelAPIBalance(c *ui.Context, l l10n.Localizer, baseURL string, q *apiquota.Quota, failure string, refreshing, private bool) {
	t := c.Theme()
	value := ""
	if q != nil {
		value = apiBalanceValue(l, *q)
	}
	ui.Column(c).Gap(6).Children(func() {
		switch {
		case value != "":
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, l.Text(l10n.KeyWidgetBalance)).FontSize(fsCallout).FontWeight(600).Grow(1).Basis(0)
				ui.Text(c, value).FontSize(fsCallout).FontWeight(600).FontFeatures("tnum").SingleLine()
			})
			if f, ok := q.UsedFraction(); ok {
				used := min(100, max(0, int(f*100+0.5)))
				quotaBar(c, runway.Meter{UsedPercent: used, RemainingPercent: 100 - used, Health: runway.HealthForUsedPercent(used)})
			}
		case q != nil:
			calloutDim(c, apiBalanceSummary(l, q))
		case failure == "":
			sectionEmpty(c, l, refreshing)
		}
		if failure != "" {
			calloutDim(c, privateSiteText(private, failure, baseURL))
		}
		meta := accountPrivate(private, apiHost(baseURL))
		if q != nil && !q.FetchedAt.IsZero() {
			meta = accountUpdatedSummary(l, meta, q.FetchedAt)
		}
		if meta != "" {
			ui.Text(c, meta).FontSize(fsCaption).TextColor(inkSecondary(t)).SingleLine().Ellipsis("…")
		}
	})
}

// privateSiteText masks, in privacy mode, the emails in text and the host of
// the account's site, which network errors quote.
func privateSiteText(private bool, text, baseURL string) string {
	if !private || text == "" {
		return text
	}
	text = codex.MaskEmailsIn(text)
	if u, err := url.Parse(baseURL); err == nil && u.Hostname() != "" {
		text = strings.ReplaceAll(text, u.Hostname(), accountPrivate(true, u.Hostname()))
	}
	return text
}

// apiHost is the host of a base URL, the URL itself when it does not parse.
func apiHost(baseURL string) string {
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return baseURL
}
