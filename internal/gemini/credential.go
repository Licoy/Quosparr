package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/codex"
)

// Credential document keys.
const (
	authMethodConsumer = "consumer"
	tokenTypeBearer    = "Bearer"
)

// Identity is the Google account a credential belongs to. It never holds
// tokens.
type Identity struct {
	Subject string `json:"sub,omitempty"`
	Email   string `json:"email,omitempty"`
	Name    string `json:"name,omitempty"`
	Picture string `json:"picture,omitempty"`
}

// Known reports whether the identity names an account.
func (i Identity) Known() bool { return i.key() != "" }

func (i Identity) key() string {
	if s := trimSpace(i.Subject); s != "" {
		return "sub:" + s
	}
	if e := strings.ToLower(trimSpace(i.Email)); e != "" {
		return "email:" + e
	}
	return ""
}

// StableID is the account id: "gemini-" plus the hash of the subject, else
// of the case-folded email.
func (i Identity) StableID() string { return "gemini-" + codex.StableHash(i.key()) }

// ResolvedDisplayName is the best human label.
func (i Identity) ResolvedDisplayName() string {
	return firstNonEmpty(i.Email, i.Name, i.Subject)
}

// merge fills empty fields of i from other.
func (i Identity) merge(other Identity) Identity {
	i.Subject = firstNonEmpty(i.Subject, other.Subject)
	i.Email = firstNonEmpty(i.Email, other.Email)
	i.Name = firstNonEmpty(i.Name, other.Name)
	i.Picture = firstNonEmpty(i.Picture, other.Picture)
	return i
}

// identitiesMatch: same subject when both have one, else the same email.
func identitiesMatch(a, b Identity) bool {
	if l, r := trimSpace(a.Subject), trimSpace(b.Subject); l != "" && r != "" {
		return l == r
	}
	l, r := trimSpace(a.Email), trimSpace(b.Email)
	return l != "" && r != "" && strings.EqualFold(l, r)
}

// Credential is a parsed Antigravity credential document. Unknown fields of
// the root and of "token" are kept for Marshal.
type Credential struct {
	AccessToken  string
	TokenType    string
	RefreshToken string
	Expiry       time.Time
	AuthMethod   string
	IDToken      string

	root  map[string]json.RawMessage
	token map[string]json.RawMessage
}

// ParseCredential decodes a credential document. It needs a token object with
// an access or refresh token.
func ParseCredential(data []byte) (Credential, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(data), &root); err != nil || root == nil {
		return Credential{}, ErrInvalidCredential
	}
	var token map[string]json.RawMessage
	if err := json.Unmarshal(root["token"], &token); err != nil || token == nil {
		return Credential{}, ErrInvalidCredential
	}
	c := Credential{
		AccessToken:  rawString(token["access_token"]),
		TokenType:    rawString(token["token_type"]),
		RefreshToken: rawString(token["refresh_token"]),
		AuthMethod:   rawString(root["auth_method"]),
		IDToken:      rawString(root["id_token"]),
		root:         root,
		token:        token,
	}
	if raw, ok := token["expiry"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &c.Expiry); err != nil {
			return Credential{}, ErrInvalidCredential
		}
	}
	if c.AccessToken == "" && c.RefreshToken == "" {
		return Credential{}, ErrInvalidCredential
	}
	return c, nil
}

// NewCredential builds a document for tokens from a fresh login.
func NewCredential(access, refresh, idToken string, expiry time.Time) Credential {
	return Credential{
		AccessToken: access, TokenType: tokenTypeBearer, RefreshToken: refresh,
		Expiry: expiry, AuthMethod: authMethodConsumer, IDToken: idToken,
	}
}

// Marshal renders the document compactly, keeping unknown fields.
func (c Credential) Marshal() ([]byte, error) {
	token := cloneRaw(c.token)
	setRaw(token, "access_token", c.AccessToken)
	setRaw(token, "token_type", firstNonEmpty(c.TokenType, tokenTypeBearer))
	setRaw(token, "refresh_token", c.RefreshToken)
	if c.Expiry.IsZero() {
		delete(token, "expiry")
	} else if raw, err := json.Marshal(c.Expiry); err == nil {
		token["expiry"] = raw
	}
	root := cloneRaw(c.root)
	encoded, err := json.Marshal(token)
	if err != nil {
		return nil, err
	}
	root["token"] = encoded
	setRaw(root, "auth_method", firstNonEmpty(c.AuthMethod, authMethodConsumer))
	setRaw(root, "id_token", c.IDToken)
	return json.Marshal(root)
}

// WithRefreshedTokens applies a token-endpoint answer. Empty refresh or id
// tokens keep the previous ones; a positive expiresIn sets the expiry and
// a stored expires_in field.
func (c Credential) WithRefreshedTokens(access, refresh, idToken string, expiresIn int64, now time.Time) Credential {
	c.token = cloneRaw(c.token)
	c.AccessToken = access
	c.TokenType = tokenTypeBearer
	if trimSpace(refresh) != "" {
		c.RefreshToken = refresh
	}
	if trimSpace(idToken) != "" {
		c.IDToken = idToken
	}
	if expiresIn > 0 {
		c.Expiry = now.Add(time.Duration(expiresIn) * time.Second)
		if _, ok := c.token["expires_in"]; ok {
			c.token["expires_in"], _ = json.Marshal(expiresIn)
		}
	}
	return c
}

// NeedsRefresh reports whether the access token is missing or expires within
// skew of now. A document without an expiry is treated as expired.
func (c Credential) NeedsRefresh(now time.Time, skew time.Duration) bool {
	if c.AccessToken == "" || c.Expiry.IsZero() {
		return true
	}
	return c.Expiry.Sub(now) <= skew
}

// RequiresReauthentication: nothing can produce a valid access token.
func (c Credential) RequiresReauthentication(now time.Time) bool {
	if c.RefreshToken != "" {
		return false
	}
	return c.AccessToken == "" || (!c.Expiry.IsZero() && !c.Expiry.After(now))
}

// IDTokenIdentity reads sub, email, name and picture from the id_token
// payload. The token came from Google's token endpoint or the official
// store, so the signature is not checked; the claims only label the account.
func (c Credential) IDTokenIdentity() Identity {
	claims := jwtClaims(c.IDToken)
	return Identity{
		Subject: claimString(claims, "sub"),
		Email:   claimString(claims, "email"),
		Name:    claimString(claims, "name"),
		Picture: claimString(claims, "picture"),
	}
}

// sameLogin reports whether two documents carry the same refresh token.
func sameLogin(a, b Credential) bool {
	return a.RefreshToken != "" && a.RefreshToken == b.RefreshToken
}

func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return nil
	}
	return claims
}

func claimString(claims map[string]any, key string) string {
	s, _ := claims[key].(string)
	return trimSpace(s)
}

func rawString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

func setRaw(o map[string]json.RawMessage, key, value string) {
	if value == "" {
		delete(o, key)
		return
	}
	o[key], _ = json.Marshal(value)
}

func cloneRaw(o map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(o)+4)
	for k, v := range o {
		out[k] = v
	}
	return out
}

func trimSpace(s string) string { return strings.TrimFunc(s, unicode.IsSpace) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := trimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
