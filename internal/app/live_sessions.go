package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/export/swiftjson"
	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/sessions"
	"github.com/licoy/quosparr/internal/store"
	"github.com/licoy/quosparr/internal/usage"
)

var errSessionsUnavailable = errors.New("session management service unavailable")

func installSessionActions(rt *liveApp, actions *SessionActions) {
	actions.Query = rt.queryManagedSessions
	actions.Detail = sessions.ReadManagedDetail
	actions.Delete = rt.deleteManagedSessions
	actions.Reveal = func(path string) error {
		mygo.Shell.ShowItemInFolder(path)
		return nil
	}
}

func (rt *liveApp) queryManagedSessions(ctx context.Context, provider store.Provider) ([]sessions.ManagedSession, error) {
	switch provider {
	case store.ProviderCodex:
		home := rt.codexFolder()
		if home == "" {
			return nil, errSessionsUnavailable
		}
		return (sessions.Scanner{CodexHome: home, Pricer: managedSessionPricer}).ListManaged(ctx)
	case store.ProviderClaude:
		if rt.claudeMod == nil || rt.claudeMod.Store == nil {
			return nil, errSessionsUnavailable
		}
		var cache *claude.SessionCache
		if rt.claudeB != nil {
			cache = &rt.claudeB.sessions
		}
		return (claude.SessionScanner{
			ConfigDir: rt.claudeMod.Store.ConfigDir(), Home: rt.claudeMod.Store.HomeDir(),
			Location: time.Local, Cache: cache,
		}).ListManaged(ctx)
	case store.ProviderGrok:
		if rt.grokMod == nil || rt.grokMod.Store == nil {
			return nil, errSessionsUnavailable
		}
		return (grok.SessionScanner{GrokHome: rt.grokMod.Store.OfficialHome(), Location: time.Local}).ListManaged(ctx)
	case store.ProviderGemini:
		scanner, ok := rt.geminiB.sessionScanner()
		if !ok {
			return nil, errSessionsUnavailable
		}
		return scanner.ListManaged(ctx)
	default:
		return nil, errSessionsUnavailable
	}
}

func managedSessionPricer(model string, totals sessions.Totals) (swiftjson.Decimal, bool) {
	cost, ok := usage.CostForTotals(model, usage.Totals{
		TotalTokens: int64(totals.TotalTokens), UncachedInputTokens: int64(totals.UncachedInputTokens),
		CachedInputTokens: int64(totals.CachedInputTokens), OutputTokens: int64(totals.OutputTokens),
		Turns: int64(totals.Turns), Threads: int64(totals.Threads),
	})
	if !ok {
		return swiftjson.Decimal{}, false
	}
	value, err := swiftjson.ParseDecimal(cost.String())
	return value, err == nil
}

// sessionTrash moves a session file to the system Trash; tests replace it.
var sessionTrash = mygo.Shell.TrashItem

// geminiRunningClients lists the running Antigravity clients; tests replace
// it so they never inspect the real processes.
var geminiRunningClients = gemini.RunningClients

// deleteManagedSessions sends Antigravity conversations through the Gemini
// scanner, which also edits the summaries databases, and every other
// provider through the shared deletion.
func (rt *liveApp) deleteManagedSessions(ctx context.Context, selected []sessions.ManagedSession) ([]sessions.DeleteResult, error) {
	var others, antigravity []sessions.ManagedSession
	for _, item := range selected {
		if item.Provider == sessions.ProviderGemini {
			antigravity = append(antigravity, item)
		} else {
			others = append(others, item)
		}
	}
	var results []sessions.DeleteResult
	var errs []error
	if len(others) > 0 {
		r, err := sessions.DeleteManaged(ctx, others, rt.sessionTrustedRoots(), sessionTrash)
		results, errs = append(results, r...), append(errs, err)
	}
	if len(antigravity) > 0 {
		r, err := rt.deleteGeminiSessions(ctx, antigravity)
		results, errs = append(results, r...), append(errs, err)
	}
	err := errors.Join(errs...)
	removed := map[store.Provider]bool{}
	for _, result := range results {
		if result.Removed > 0 {
			removed[store.Provider(result.Session.Provider)] = true
		}
	}
	for provider := range removed {
		rt.refreshDeletedSessions(provider)
	}
	return results, err
}

func (rt *liveApp) deleteGeminiSessions(ctx context.Context, selected []sessions.ManagedSession) ([]sessions.DeleteResult, error) {
	scanner, ok := rt.geminiB.sessionScanner()
	if !ok {
		results := make([]sessions.DeleteResult, len(selected))
		for i, item := range selected {
			results[i] = sessions.DeleteResult{Session: item, Total: len(item.Files), Err: errSessionsUnavailable}
		}
		return results, errSessionsUnavailable
	}
	// <Quosparr data>/backups, beside the account library, like every backup.
	scanner.BackupDir = filepath.Join(filepath.Dir(rt.geminiB.mod.Store.Root()), "backups")
	scanner.ClientRunning = geminiClientRunning(ctx, scanner)
	return scanner.DeleteManaged(ctx, selected, sessionTrash, time.Now())
}

// geminiClientRunning checks the processes once per deletion: the App owns
// ~/.gemini/antigravity and agy ~/.gemini/antigravity-cli. A failed check,
// or a root of neither, counts as running so nothing is touched.
func geminiClientRunning(ctx context.Context, scanner gemini.SessionScanner) func(root string) bool {
	var once sync.Once
	var running map[string]bool
	var err error
	return func(root string) bool {
		once.Do(func() { running, err = geminiRunningClients(ctx) })
		client := scanner.SessionClientOf(root)
		return err != nil || client == "" || running[client]
	}
}

func (rt *liveApp) sessionTrustedRoots() map[string][]string {
	roots := map[string][]string{}
	if home := rt.codexFolder(); home != "" {
		roots[sessions.ProviderCodex] = []string{filepath.Join(home, "sessions"), filepath.Join(home, "archived_sessions")}
	}
	if rt.claudeMod != nil && rt.claudeMod.Store != nil {
		roots[sessions.ProviderClaude] = claude.ProjectsDirs(rt.claudeMod.Store.ConfigDir(), rt.claudeMod.Store.HomeDir())
	}
	if rt.grokMod != nil && rt.grokMod.Store != nil {
		roots[sessions.ProviderGrok] = []string{filepath.Join(rt.grokMod.Store.OfficialHome(), "sessions")}
	}
	return roots
}

func (rt *liveApp) refreshDeletedSessions(provider store.Provider) {
	if rt.svc == nil {
		return
	}
	if localProvider(provider) {
		rt.svc.RefreshProviderLocal(provider)
		return
	}
	if provider != store.ProviderCodex {
		return
	}
	rt.svc.RefreshRecentSessions(true)
	rt.svc.RefreshSpeed(true)
	rt.svc.RefreshCost(usage.RefreshForce)
	rt.svc.RefreshTokenHeatmap(usage.RefreshForce)
}
