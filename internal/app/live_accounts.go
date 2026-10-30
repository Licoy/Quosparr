package app

import (
	"context"
	"errors"

	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

// installAccountActions wires the multi-account page to the runtime. Each
// action runs synchronously and returns the message to show; the panel
// calls them off the main thread. The confirmations of the Swift sheets are
// the panel's own popovers, so none of these asks again.
func installAccountActions(rt *liveApp, s *Settings, a *PanelActions) {
	loc := func() l10n.Localizer {
		if s != nil {
			return s.L()
		}
		return l10n.ForPreference(string(rt.svc.Preferences().Language), rt.langs)
	}
	a.Accounts = AccountActions{
		RefreshAll:   func() string { <-rt.svc.RefreshAllAccountQuotas(); return "" },
		Refresh:      func(id string) string { <-rt.svc.RefreshAccountQuota(id); return "" },
		Switch:       func(id string, restart bool) string { return rtSwitchCodex(loc(), id, restart) },
		CheckRuntime: func() string { return rt.checkCodexRuntime(loc()) },
		Delete:       func(id string) string { return rt.removeCodexAccount(loc(), id) },
	}
	installGrokAccountActions(rt, loc, &a.Accounts)
	installClaudeAccountActions(rt, loc, &a.Accounts)
}

func installClaudeAccountActions(rt *liveApp, loc func() l10n.Localizer, a *AccountActions) {
	if rt.claudeMod == nil {
		return
	}
	a.ClaudeRefreshAll = func() string { return rt.refreshClaudeAll(context.Background()) }
	a.ClaudeRefresh = func(id string) string { return rt.refreshClaude(context.Background(), loc(), id) }
	a.ClaudeSwitch = func(id string) string { return rt.switchClaude(loc(), id) }
	a.ClaudeSwitchDesktop = func(id string, desktop bool) string {
		return rt.switchClaudeWithDesktop(context.Background(), loc(), id, desktop)
	}
	a.ClaudeDelete = func(id string) string { return rt.removeClaude(loc(), id) }
}

func (rt *liveApp) removeCodexAccount(l l10n.Localizer, id string) string {
	if err := rt.codex.DeleteAccount(id); err != nil {
		return accountFailure(l, l.Text(l10n.KeyStatusError), err)
	}
	return rt.reloadCodexAfterSave(l)
}

func installGrokAccountActions(rt *liveApp, loc func() l10n.Localizer, a *AccountActions) {
	if rt.grokMod == nil {
		return
	}
	a.GrokRefreshAll = func() string { return rt.grokRefresh(loc(), grok.RefreshTarget{Kind: grok.RefreshAll}) }
	a.GrokRefresh = func(id string) string {
		return rt.grokRefresh(loc(), grok.RefreshTarget{Kind: grok.RefreshAccount, ID: id})
	}
	a.GrokSwitch = func(id string, force bool) (bool, string) { return rt.grokMakeCurrent(loc(), id, force) }
	a.GrokDelete = func(id string) string { return rt.grokRemove(loc(), id) }
	a.GrokAlias = func(id, alias string) string { return rt.setGrokAlias(loc(), id, alias) }
	a.GrokMove = rt.moveGrok
	a.GrokLogin = func(ctx context.Context) string { return rt.loginGrokContext(loc(), ctx) }
	a.GrokPaste = func(text string) (string, bool) { return rt.grokPasteText(loc(), text) }
	a.GrokImportOfficial = func() string { return rt.importGrokOfficial(loc()) }
}

func (rt *liveApp) grokRefresh(l l10n.Localizer, target grok.RefreshTarget) string {
	report := rt.grokMod.Refresh(context.Background(), target)
	rt.grok.reload(true)
	return grokRefreshMessage(l, report)
}

// grokMakeCurrent switches the Grok login. Running Grok processes block it
// until force is set; running reports that case.
func (rt *liveApp) grokMakeCurrent(l l10n.Localizer, id string, force bool) (running bool, message string) {
	_, err := rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandMakeCurrent, ID: id, AllowWhileRunning: force})
	rt.grok.reload(true)
	var blocked *grok.ProcessesRunningError
	switch {
	case errors.As(err, &blocked):
		return true, ""
	case err != nil:
		return false, accountFailure(l, l.Text(l10n.KeyGrokSwitchFailed), err)
	}
	return false, ""
}

func (rt *liveApp) grokRemove(l l10n.Localizer, id string) string {
	_, err := rt.grokMod.Apply(context.Background(), grok.Command{Kind: grok.CommandRemove, ID: id})
	rt.grok.reload(true)
	var current *grok.CurrentAccountCannotBeRemovedError
	if errors.As(err, &current) {
		return l.Text(l10n.KeyGrokCurrentAccountCannotDelete)
	}
	if err != nil {
		return accountFailure(l, l.Text(l10n.KeyStatusError), err)
	}
	return ""
}

// grokPasteText imports pasted credentials; the message is the success
// count or the reason nothing was imported.
func (rt *liveApp) grokPasteText(l l10n.Localizer, text string) (string, bool) {
	batch, _, err := rt.grokMod.ImportPastedText(context.Background(), text)
	rt.grok.reload(true)
	if err != nil {
		return accountBatchFailure(l, batch.SuccessCount(), batch.FailureCount()+1, append(batch.FailureErrors, err)), false
	}
	if message := accountBatchFailure(l, batch.SuccessCount(), batch.FailureCount(), batch.FailureErrors); message != "" {
		return message, false
	}
	if batch.SuccessCount() == 0 {
		return l.Text(l10n.KeyGrokAccountsImportNoCredentials), false
	}
	return l.Textf(l10n.KeyAccountsImportSucceeded, batch.SuccessCount()), true
}
