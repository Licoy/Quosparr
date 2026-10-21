package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/platform"
)

const (
	// KindAPI marks an API relay account in claude-index.json. An empty kind
	// is a subscription (OAuth) account.
	KindAPI = "api"

	apiIDPrefix = "claude-api-"
	apiKeyField = "apiKey"
)

// API account errors. None of them includes the key or the base URL.
var (
	// ErrNotAPIAccount means an API-only operation was asked for an OAuth account.
	ErrNotAPIAccount = errors.New("claude: not an API account")
	// ErrAPIAccount means an OAuth-only operation was asked for an API account.
	ErrAPIAccount = errors.New("claude: operation does not apply to an API account")
	// ErrAPIKeyRequired means the API key is empty.
	ErrAPIKeyRequired = errors.New("claude: API key is required")
	// ErrAPIKeyInvalid means the API key holds whitespace or control characters.
	ErrAPIKeyInvalid = errors.New("claude: API key has invalid characters")
	// ErrAPIKeyMissing means the managed key file is absent or unreadable.
	ErrAPIKeyMissing = errors.New("claude: API key is missing from the account library")
	// ErrAPIBaseURL means the base URL is empty or not an http(s) URL.
	ErrAPIBaseURL = errors.New("claude: API base URL is invalid")
	// ErrAPIDuplicate means another API account already has this base URL and key.
	ErrAPIDuplicate = errors.New("claude: an API account with this base URL and key already exists")
	// ErrAPIUnauthorized means the relay rejected the key during a balance read.
	ErrAPIUnauthorized = errors.New("claude: API key rejected")
	// ErrAPIUnavailable means the balance read failed for another reason.
	ErrAPIUnavailable = errors.New("claude: API balance read failed")
)

// APIProfile is the non-secret half of an API relay account. The key lives in
// the managed copy only. KeyHint is four characters of a hash of the key, so
// two accounts on one relay can be told apart without exposing any key text.
// The model fields are optional; empty means "leave to Claude Code".
type APIProfile struct {
	BaseURL     string `json:"baseURL"`
	KeyHint     string `json:"keyHint,omitempty"`
	Model       string `json:"model,omitempty"`
	OpusModel   string `json:"opusModel,omitempty"`
	SonnetModel string `json:"sonnetModel,omitempty"`
	HaikuModel  string `json:"haikuModel,omitempty"`
	FableModel  string `json:"fableModel,omitempty"`
	Context1M   bool   `json:"context1M,omitempty"`
}

// Env is the settings.json form of the profile with key as the auth token.
func (p APIProfile) Env(key string) APIEnv {
	env := APIEnv{
		BaseURL: p.BaseURL, AuthToken: key, Model: p.Model,
		OpusModel: p.OpusModel, SonnetModel: p.SonnetModel,
		HaikuModel: p.HaikuModel, FableModel: p.FableModel,
	}
	if p.Context1M {
		for _, model := range []*string{&env.Model, &env.OpusModel, &env.SonnetModel} {
			*model = strings.TrimSpace(*model)
			if *model != "" && !strings.HasSuffix(strings.ToLower(*model), "[1m]") {
				*model += "[1m]"
			}
		}
	}
	return env
}

// Host is the host (and port) of the base URL, "" when it is not parseable.
func (p APIProfile) Host() string {
	u, err := url.Parse(p.BaseURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// withModels returns p with the model fields and context choice of src, trimmed. BaseURL and
// KeyHint are kept.
func (p APIProfile) withModels(src APIProfile) APIProfile {
	p.Model = strings.TrimSpace(src.Model)
	p.OpusModel = strings.TrimSpace(src.OpusModel)
	p.SonnetModel = strings.TrimSpace(src.SonnetModel)
	p.HaikuModel = strings.TrimSpace(src.HaikuModel)
	p.FableModel = strings.TrimSpace(src.FableModel)
	p.Context1M = src.Context1M
	return p
}

// IsAPI reports whether the row is an API relay account.
func (a ManagedAccount) IsAPI() bool { return a.Kind == KindAPI }

// apiLabel is "<host> ···<hint>", the name of an API account without an alias.
func (a ManagedAccount) apiLabel() string {
	if a.API == nil {
		return ""
	}
	host := a.API.Host()
	switch {
	case host != "" && a.API.KeyHint != "":
		return host + " ···" + a.API.KeyHint
	case host != "":
		return host
	}
	return ""
}

// ApplyAPIQuota caches a balance reading and clears the error.
func (a ManagedAccount) ApplyAPIQuota(q apiquota.Quota, at time.Time) ManagedAccount {
	if !q.FetchedAt.IsZero() {
		at = q.FetchedAt
	}
	a.APIQuota = &q
	a.LastQuotaAt = &at
	a.RequiresReauth = false
	a.LastError = ""
	return a
}

// NormalizeAPIBaseURL returns the base URL written to ANTHROPIC_BASE_URL.
func NormalizeAPIBaseURL(raw string) (string, error) {
	urls, err := apiquota.NormalizeBaseURL(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrAPIBaseURL, err)
	}
	return urls.AnthropicBase, nil
}

// normalizeAPIKey trims the key and rejects empty or multi-token text.
func normalizeAPIKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", ErrAPIKeyRequired
	}
	if strings.IndexFunc(key, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", ErrAPIKeyInvalid
	}
	return key, nil
}

// apiAccountID is "claude-api-" plus 16 hex characters of a SHA-256 over the
// base URL and the key. The id is fixed when the account is created and does
// not change when the profile is edited later.
func apiAccountID(baseURL, key string) string {
	sum := sha256.Sum256([]byte("quosparr-claude-api|" + baseURL + "|" + key))
	return apiIDPrefix + hex.EncodeToString(sum[:8])
}

// apiKeyHint is the last four hex characters of a SHA-256 over the key.
func apiKeyHint(key string) string {
	sum := sha256.Sum256([]byte("quosparr-claude-api-key|" + key))
	h := hex.EncodeToString(sum[:])
	return h[len(h)-4:]
}

// --- managed copy -----------------------------------------------------------

type apiKeyDocument struct {
	APIKey string `json:"apiKey"`
}

// parseAPIKeyDocument reads the managed {"apiKey": "..."} file. The error
// never includes file text.
func parseAPIKeyDocument(data []byte) (string, error) {
	var doc apiKeyDocument
	if json.Unmarshal(data, &doc) != nil {
		return "", ErrAPIKeyMissing
	}
	key := strings.TrimSpace(doc.APIKey)
	if key == "" {
		return "", ErrAPIKeyMissing
	}
	return key, nil
}

func marshalAPIKeyDocument(key string) ([]byte, error) {
	return marshalRawObject(map[string]json.RawMessage{apiKeyField: rawString(key)})
}

func (s *Store) saveAPIKey(id, key string) error {
	data, err := marshalAPIKeyDocument(key)
	if err != nil {
		return err
	}
	if err := platform.EnsurePrivateDirectory(s.AccountDir(id)); err != nil {
		return err
	}
	return platform.WriteFileAtomic(s.CredentialPath(id), data, storeFilePerm)
}

// LoadAPIKey returns the managed API key of id. It is for internal use: the
// caller must not log it.
func (s *Store) LoadAPIKey(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadAPIKey(id)
}

func (s *Store) loadAPIKey(id string) (string, error) {
	data, err := os.ReadFile(s.CredentialPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return "", &AccountNotFoundError{ID: id}
	}
	if err != nil {
		return "", ErrAPIKeyMissing
	}
	return parseAPIKeyDocument(data)
}

// PutAPI stores an API account row and, when key is not empty, its managed
// copy. A row that is not in the index yet gets the next sort index and a
// creation time; an existing row is replaced as given. The index never holds
// the key. If the index cannot be written, a replaced key is restored.
func (s *Store) PutAPI(acc ManagedAccount, key string, now time.Time) (ManagedAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !acc.IsAPI() || acc.API == nil {
		return ManagedAccount{}, ErrNotAPIAccount
	}
	idx, err := s.loadIndex()
	if err != nil {
		return ManagedAccount{}, err
	}
	_, found := idx.Account(acc.ID)
	if !found {
		acc.SortIndex = nextSortIndex(idx)
		if acc.CreatedAt.IsZero() {
			acc.CreatedAt = now
		}
	}
	var previousKey string
	hadKey := false
	if key != "" {
		previousKey, err = s.loadAPIKey(acc.ID)
		hadKey = err == nil
		if err := s.saveAPIKey(acc.ID, key); err != nil {
			return ManagedAccount{}, err
		}
	}
	idx.Upsert(acc)
	if err := s.saveIndex(idx); err != nil {
		switch {
		case !found:
			_ = os.RemoveAll(s.AccountDir(acc.ID))
		case key != "" && hadKey:
			_ = s.saveAPIKey(acc.ID, previousKey)
		}
		return ManagedAccount{}, err
	}
	return acc, nil
}
