package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/usage"
)

// openAIBaseURL is where a plain API key (no relay) lists its models.
const openAIBaseURL = "https://api.openai.com/v1"

var errCodexUnavailable = errors.New("codex: accounts are unavailable")

// CodexAPIInput is the form data of a Codex API account. An empty BaseURL is
// a plain OpenAI key. When editing, an empty APIKey keeps the stored key, and
// Apply also rewrites auth.json and config.toml if the account is in use.
type CodexAPIInput struct {
	Alias, BaseURL, APIKey, Model string
	Apply                         bool
}

// addCodexAPIAccount adds an API key account, pointed at a relay when a base
// URL is given, and reads its balance once (best effort).
func (rt *liveApp) addCodexAPIAccount(ctx context.Context, in CodexAPIInput) error {
	if rt.codex == nil {
		return errCodexUnavailable
	}
	acc, err := (&codex.Importer{Store: rt.codex}).AddRelayAPIKey(in.APIKey, codex.RelayProfile{BaseURL: in.BaseURL, Model: in.Model}, in.Alias, false)
	if err != nil {
		return err
	}
	rt.svc.ReloadAccounts()
	rt.awaitCodexBalance(ctx, acc)
	return nil
}

// updateCodexAPIAccount saves the alias, then the site, model and an optional
// new key; with Apply, the official files of the account in use follow and
// the service rereads the login.
func (rt *liveApp) updateCodexAPIAccount(ctx context.Context, id string, in CodexAPIInput) error {
	if rt.codex == nil {
		return errCodexUnavailable
	}
	idx, err := rt.codex.LoadIndex()
	if err != nil {
		return err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return &codex.AccountNotFoundError{ID: id}
	}
	if alias := strings.TrimSpace(in.Alias); alias != acc.Alias {
		acc.Alias = alias
		if err := rt.codex.UpdateMetadata(acc); err != nil {
			return err
		}
	}
	edit := codex.RelayEdit{Profile: codex.RelayProfile{BaseURL: in.BaseURL, Model: in.Model}, Key: in.APIKey, Apply: in.Apply}
	saved, err := rt.codex.EditRelayAccount(id, edit, time.Now())
	rt.svc.ReloadAccounts()
	if err != nil {
		return err
	}
	if in.Apply {
		rt.svc.InvalidateCodexRuntime()
		// The shown login changed under the service.
		rt.svc.Refresh(usage.RefreshForce)
	}
	if saved.APIQuota == nil {
		rt.awaitCodexBalance(ctx, saved)
	}
	return nil
}

// awaitCodexBalance probes a relay account's balance and waits for it, or
// for ctx.
func (rt *liveApp) awaitCodexBalance(ctx context.Context, acc codex.ManagedAccount) {
	if !acc.IsRelay() {
		return
	}
	select {
	case <-rt.svc.RefreshAccountQuota(acc.ID):
	case <-ctx.Done():
	}
}

// codexAPIModels lists the models of a site. With accountID set and no key
// typed, the stored key of that account is used; an empty base URL asks
// OpenAI itself.
func (rt *liveApp) codexAPIModels(ctx context.Context, accountID, baseURL, apiKey string) ([]string, error) {
	if rt.codex == nil {
		return nil, errCodexUnavailable
	}
	key := strings.TrimSpace(apiKey)
	if key == "" && accountID != "" {
		auth, err := rt.codex.LoadCredential(accountID)
		if err != nil || !auth.IsAPIKey() {
			return nil, codex.ErrRelayKeyRequired
		}
		key = auth.APIKey()
	}
	if key == "" {
		return nil, codex.ErrRelayKeyRequired
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = openAIBaseURL
	}
	client, err := netx.ClientFor(ctx, netx.WithoutCookies)
	if err != nil {
		return nil, err
	}
	return apiquota.FetchModels(ctx, client, baseURL, key, apiquota.ModelStyleOpenAI)
}
