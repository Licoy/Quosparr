package claude

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/licoy/quosparr/internal/apiquota"
)

// CommandKind selects Module.Apply.
type CommandKind int

const (
	// CommandImportOfficial copies the official login into the library.
	CommandImportOfficial CommandKind = iota
	// CommandSwitch installs an account as the official login.
	CommandSwitch
	// CommandRemove deletes a non-current account.
	CommandRemove
	// CommandSetAlias sets or clears the alias.
	CommandSetAlias
	// CommandReorder reorders accounts.
	CommandReorder
	// CommandPaste imports a pasted claudeAiOauth document.
	CommandPaste
	// CommandAddAPI adds an API relay account from API, APIKey and Alias, or
	// updates the one with the same base URL and key.
	CommandAddAPI
	// CommandUpdateAPI edits the API account ID: Alias and the model fields
	// of API are replaced; an empty API.BaseURL or APIKey keeps the stored one.
	CommandUpdateAPI
)

// Command is one account action.
type Command struct {
	Kind              CommandKind
	ID                string
	Alias             string
	IDs               []string
	Text              string
	Identity          CredentialIdentity
	AllowWhileRunning bool
	// APIKey and API describe an API relay account (CommandAddAPI,
	// CommandUpdateAPI). The key is written to the managed copy only.
	APIKey string
	API    *APIProfile
}

// AccountState is the library after an operation.
type AccountState struct {
	CurrentAccountID string
	Accounts         []ManagedAccount
}

// Module orchestrates the account library. The current account's token is
// never refreshed. Operations are serialized.
type Module struct {
	Store             *Store
	Usage             *UsageClient
	Tokens            RefreshClient
	Throttle          *UsageThrottle
	RunningProcessIDs func(ctx context.Context) ([]int, error)
	Now               func() time.Time
	Replacer          OAuthAccountReplacer
	// APIClient sends API relay balance and model-list requests. Nil uses the
	// proxied network client of Usage; it never falls back to a direct one.
	APIClient apiquota.Doer

	mu              sync.Mutex
	profileKey      string
	profileIdentity CredentialIdentity
}

// NewModule returns a module over store.
func NewModule(store *Store) *Module { return &Module{Store: store} }

func (m *Module) now() time.Time {
	if m != nil && m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Module) throttle() *UsageThrottle {
	if m.Throttle == nil {
		m.Throttle = &UsageThrottle{}
	}
	return m.Throttle
}

func (m *Module) configPath() string {
	if m.Replacer.Path != "" {
		return m.Replacer.Path
	}
	return GlobalConfigPath(m.Store.ConfigDir(), m.Store.HomeDir())
}

func (m *Module) replacer(path string) OAuthAccountReplacer {
	r := m.Replacer
	r.Path = path
	return r
}

// Load returns the account index.
func (m *Module) Load(ctx context.Context) (AccountState, error) {
	if err := ctx.Err(); err != nil {
		return AccountState{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state()
}

func (m *Module) state() (AccountState, error) {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return AccountState{}, err
	}
	return AccountState{CurrentAccountID: idx.CurrentAccountID, Accounts: idx.OrderedAccounts()}, nil
}

// Apply runs one command.
func (m *Module) Apply(ctx context.Context, cmd Command) (AccountState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return AccountState{}, err
	}
	var err error
	switch cmd.Kind {
	case CommandImportOfficial:
		err = m.importOfficial(ctx)
	case CommandSwitch:
		err = m.switchLocked(ctx, cmd.ID, cmd.AllowWhileRunning)
	case CommandRemove:
		err = m.Store.Remove(cmd.ID)
	case CommandSetAlias:
		err = m.setAlias(cmd.ID, cmd.Alias)
	case CommandReorder:
		err = m.Store.Reorder(cmd.IDs)
	case CommandPaste:
		_, err = m.importPasted(cmd.Text, cmd.Identity)
	case CommandAddAPI:
		_, err = m.addAPI(cmd.Alias, apiOrEmpty(cmd.API), cmd.APIKey)
	case CommandUpdateAPI:
		_, err = m.updateAPI(cmd.ID, cmd.Alias, apiOrEmpty(cmd.API), cmd.APIKey, true)
	default:
		err = errors.New("claude: unknown account command")
	}
	if err != nil {
		return AccountState{}, err
	}
	return m.state()
}

func apiOrEmpty(p *APIProfile) APIProfile {
	if p == nil {
		return APIProfile{}
	}
	return *p
}

// importOfficial stores the Claude Code login in the library. While an API
// account is the one in settings.json, the login is stored without becoming
// current.
func (m *Module) importOfficial(ctx context.Context) error {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	var login officialLogin
	if _, _, apiActive := m.liveAPIAccount(idx); apiActive {
		login, err = m.syncOAuth(ctx, idx, false)
	} else {
		login, err = m.syncOfficial(ctx)
	}
	switch {
	case err != nil:
		return err
	case login.Data == nil:
		return ErrNotLoggedIn
	case !login.Known:
		return ErrIdentityRequired
	}
	return nil
}

func (m *Module) importPasted(text string, identity CredentialIdentity) (ManagedAccount, error) {
	data, err := ParsePasted(text)
	if err != nil {
		return ManagedAccount{}, err
	}
	return m.Store.Upsert(data, identity, UpsertOptions{Now: m.now()})
}

func (m *Module) setAlias(id, alias string) error {
	idx, err := m.Store.LoadIndex()
	if err != nil {
		return err
	}
	acc, ok := idx.Account(id)
	if !ok {
		return &AccountNotFoundError{ID: id}
	}
	acc.Alias = alias
	return m.Store.UpdateMetadata(acc)
}

// Refresh reads usage. The current account is never token-refreshed. A
// non-current account with an expired access token is refreshed first, and
// both rotated tokens are stored. invalid_grant marks that account.
func (m *Module) Refresh(ctx context.Context, id string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.refreshAccount(ctx, id, force)
}

func (m *Module) refreshUsage(ctx context.Context, id string, force bool) {
	_ = m.refreshAccount(ctx, id, force)
}
