package claude

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"time"
)

var (
	ErrLoginPortInUse = errors.New("claude: login callback port unavailable")
	ErrLoginTimedOut  = errors.New("claude: login timed out")
	ErrLoginCallback  = errors.New("claude: invalid login callback")
	ErrLoginDenied    = errors.New("claude: login authorization denied")
	ErrLoginExchange  = errors.New("claude: login exchange failed")
	ErrLoginIdentity  = errors.New("claude: login identity could not be verified")
)

const loginScopes = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"

// LoginSession contains ephemeral secrets and must never be persisted or logged.
type LoginSession struct {
	AuthURL, RedirectURI, State, CodeVerifier string
	Port                                      uint16
	ExpiresAt                                 time.Time
}

func StartLoginSession(port uint16, now time.Time, lifetime time.Duration) (LoginSession, error) {
	if port == 0 {
		port = 54545
	}
	if lifetime <= 0 {
		lifetime = 5 * time.Minute
	}
	secret := make([]byte, 64)
	if _, err := rand.Read(secret); err != nil {
		return LoginSession{}, ErrLoginExchange
	}
	s := LoginSession{State: base64.RawURLEncoding.EncodeToString(secret[:32]), CodeVerifier: base64.RawURLEncoding.EncodeToString(secret[32:]), Port: port, ExpiresAt: now.Add(lifetime)}
	s.RedirectURI = fmt.Sprintf("http://localhost:%d/callback", port)
	challenge := sha256.Sum256([]byte(s.CodeVerifier))
	query := url.Values{"client_id": {OAuthClientID}, "response_type": {"code"}, "redirect_uri": {s.RedirectURI}, "scope": {loginScopes}, "state": {s.State}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "code": {"true"}}
	s.AuthURL = "https://claude.ai/oauth/authorize?" + query.Encode()
	return s, nil
}
