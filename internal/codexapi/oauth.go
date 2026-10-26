package codexapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenAI / Codex OAuth (PKCE) constants. The UI opens AuthURL and the callback
// server delivers the redirect.
const (
	ClientID              = "app_EMoamEEZ73f0CkXaXp7hrann"
	AuthEndpoint          = "https://auth.openai.com/oauth/authorize"
	TokenEndpoint         = "https://auth.openai.com/oauth/token"
	Scopes                = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	Originator            = "codex_vscode"
	PreferredCallbackPort = 1455
	CallbackPath          = "/auth/callback"

	loginSessionLifetime = 5 * time.Minute
	exchangeTimeout      = 25 * time.Second
)

// LoginSession is one OAuth login attempt. CodeVerifier is a secret of the
// attempt; keep the session in memory only.
type LoginSession struct {
	LoginID      string
	AuthURL      string
	RedirectURI  string
	CodeVerifier string
	State        string
	Port         uint16
	ExpiresAt    time.Time
}

// StartLoginSession creates the PKCE verifier, state and authorization URL for
// a callback server on port (PreferredCallbackPort if 0). now and lifetime
// default to the current time and five minutes when zero.
func StartLoginSession(port uint16, now time.Time, lifetime time.Duration) (LoginSession, error) {
	if port == 0 {
		port = PreferredCallbackPort
	}
	if now.IsZero() {
		now = time.Now()
	}
	if lifetime <= 0 {
		lifetime = loginSessionLifetime
	}
	verifier, err := randomBase64URL(32)
	if err != nil {
		return LoginSession{}, err
	}
	state, err := randomBase64URL(16)
	if err != nil {
		return LoginSession{}, err
	}
	loginID, err := newUUID()
	if err != nil {
		return LoginSession{}, err
	}
	redirect := fmt.Sprintf("http://localhost:%d%s", port, CallbackPath)
	query := [][2]string{
		{"response_type", "code"},
		{"client_id", ClientID},
		{"redirect_uri", redirect},
		{"scope", Scopes},
		{"code_challenge", codeChallenge(verifier)},
		{"code_challenge_method", "S256"},
		{"state", state},
		{"id_token_add_organizations", "true"},
		{"codex_cli_simplified_flow", "true"},
		{"originator", Originator},
	}
	parts := make([]string, len(query))
	for i, kv := range query {
		parts[i] = kv[0] + "=" + strings.ReplaceAll(url.QueryEscape(kv[1]), "+", "%20")
	}
	return LoginSession{
		LoginID:      loginID,
		AuthURL:      AuthEndpoint + "?" + strings.Join(parts, "&"),
		RedirectURI:  redirect,
		CodeVerifier: verifier,
		State:        state,
		Port:         port,
		ExpiresAt:    now.Add(lifetime),
	}, nil
}

// OAuthError is a sign-in failure reported by the authorization server or
// detected in the redirect. It satisfies errors.Is(err, ErrAuthenticationRequired).
type OAuthError struct{ Description string }

func (e *OAuthError) Error() string { return e.Description }

// Is makes every OAuthError an authentication-required failure.
func (e *OAuthError) Is(target error) bool { return target == ErrAuthenticationRequired }

// AuthorizationCode extracts the code from the redirect URL after checking
// the error and state parameters.
func AuthorizationCode(callbackURL, expectedState string) (string, error) {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("invalid callback URL")
	}
	query := parsed.Query()
	if e := query.Get("error"); query.Has("error") {
		return "", &OAuthError{Description: e}
	}
	state := query.Get("state")
	if !query.Has("state") || subtle.ConstantTimeCompare([]byte(state), []byte(expectedState)) != 1 {
		return "", &OAuthError{Description: "OAuth state mismatch"}
	}
	code := query.Get("code")
	if code == "" {
		return "", &OAuthError{Description: "OAuth code missing"}
	}
	return code, nil
}

// OAuthLogin exchanges authorization codes.
type OAuthLogin struct {
	// HTTP nil resolves the current netx context for each request.
	HTTP *http.Client
	// TokenURL defaults to TokenEndpoint.
	TokenURL string
}

// OAuthTokens is the result of a code exchange: the tokens plus the identity
// the Swift app stores with a new ChatGPT credential.
type OAuthTokens struct {
	Tokens TokenSet
	// AccountID and PlanType come from the ID token claims, falling back to
	// the access token's.
	AccountID string
	PlanType  string
	// AuthMode is always "chatgpt".
	AuthMode string
	// LastRefresh is the exchange time in last_refresh format.
	LastRefresh string
}

// ExchangeCode trades the authorization code for tokens. Any non-2xx answer or
// unusable token set is ErrAuthenticationRequired.
func (l OAuthLogin) ExchangeCode(ctx context.Context, code string, session LoginSession, now time.Time) (OAuthTokens, error) {
	tokenURL := l.TokenURL
	if tokenURL == "" {
		tokenURL = TokenEndpoint
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {code},
		"redirect_uri":  {session.RedirectURI},
		"code_verifier": {session.CodeVerifier},
	}
	status, body, err := postForm(ctx, l.HTTP, tokenURL, form, exchangeTimeout)
	if err != nil {
		return OAuthTokens{}, err
	}
	if status < 200 || status > 299 {
		return OAuthTokens{}, errors.Join(ErrAuthenticationRequired, &HTTPError{StatusCode: status})
	}
	tokens, err := decodeTokenResponse(body)
	if err != nil {
		return OAuthTokens{}, err
	}
	if tokens.RefreshToken == "" && tokens.AccessToken == "" {
		return OAuthTokens{}, ErrAuthenticationRequired
	}
	id, _ := DecodeClaims(tokens.IDToken)
	access, _ := DecodeClaims(tokens.AccessToken)
	if now.IsZero() {
		now = time.Now()
	}
	return OAuthTokens{
		Tokens:      tokens,
		AccountID:   firstNonEmpty(id.AccountID, access.AccountID),
		PlanType:    firstNonEmpty(id.PlanType, access.PlanType),
		AuthMode:    "chatgpt",
		LastRefresh: FormatTimestamp(now),
	}, nil
}

func randomBase64URL(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// newUUID returns an upper-case random (version 4) UUID like Swift's
// UUID().uuidString.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.New("random source unavailable")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])), nil
}
