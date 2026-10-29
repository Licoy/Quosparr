// Package grok manages the Grok accounts of Quosparr: parsing the Grok CLI
// credential file, the managed multi-account library, import and export, the
// OAuth login and token refresh. Files of this package that start with
// account, auth, store, importer, transfer, oauth or refresh are the account
// side; billing, CLI and session files belong to other parts of the package.
//
// File boundaries:
//
//   - ~/.quosparr/accounts/grok-index.json holds account metadata only,
//     never tokens, and keeps fields it does not know across rewrites.
//   - ~/.quosparr/accounts/<id>/auth.json holds one account's credentials
//     (directory 0700, file 0600, atomic writes).
//   - The official ~/.grok/auth.json is written only by an explicit account
//     switch (Module.MakeCurrent through installCurrentAccount), by the first
//     import that bootstraps a login, and by token refresh write-back of the
//     current account. A refresh of any other account writes only its
//     managed copy.
package grok

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/codex"
)

// AuthKind is how a managed Grok credential signs in.
type AuthKind string

const (
	// AuthKindOAuth is an OIDC login ("https://auth.x.ai::<client>").
	AuthKindOAuth AuthKind = "oauth"
	// AuthKindLegacySession is the old web login ("https://accounts.x.ai/sign-in").
	AuthKindLegacySession AuthKind = "legacySession"
)

// Scope keys of the Grok auth.json.
const (
	// OAuthScopePrefix starts the key of every OAuth credential.
	OAuthScopePrefix = "https://auth.x.ai::"
	// LegacyScope is the key of a legacy web-session credential.
	LegacyScope = "https://accounts.x.ai/sign-in"
	// APIKeyScope holds an API key; it is never managed.
	APIKeyScope = "xai::api_key"

	defaultIssuer = "https://auth.x.ai"
)

// CredentialIdentity is who a Grok credential belongs to, without secrets.
type CredentialIdentity struct {
	Kind            AuthKind
	Scope           string
	Issuer          string
	ClientID        string
	Email           string
	UserID          string
	PrincipalID     string
	PrincipalType   string
	TeamID          string
	TeamName        string
	ExpiresAt       *time.Time
	HasAccessToken  bool
	HasRefreshToken bool
}

// StableID is the account id derived from the identity: "grok-" plus a hash of
// issuer, user (or principal, or email) and team.
func (c CredentialIdentity) StableID() string {
	return "grok-" + codex.StableHash(c.stableIdentityKey())
}

// ResolvedDisplayName is the best human label of the identity.
func (c CredentialIdentity) ResolvedDisplayName() string {
	return acctFirstNonEmpty(c.Email, c.TeamName, c.UserID, c.PrincipalID, c.StableID())
}

func (c CredentialIdentity) stableIdentityKey() string {
	issuer := strings.ToLower(acctFirstNonEmpty(c.Issuer))
	if issuer == "" {
		issuer = defaultIssuer
	}
	team := strings.ToLower(acctFirstNonEmpty(c.TeamID))
	if team == "" {
		team = "personal"
	}
	if u := acctFirstNonEmpty(c.UserID); u != "" {
		return issuer + "|user|" + strings.ToLower(u) + "|" + team
	}
	if p := acctFirstNonEmpty(c.PrincipalID); p != "" {
		return issuer + "|principal|" + strings.ToLower(p) + "|" + team
	}
	return issuer + "|email|" + strings.ToLower(c.Email) + "|" + team
}

// acctFirstNonEmpty returns the first value that is not blank after trimming,
// trimmed; empty when there is none.
func acctFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := strings.TrimFunc(v, unicode.IsSpace); t != "" {
			return t
		}
	}
	return ""
}

// Auth document errors. None of them carries credential text.
var (
	// ErrAuthInvalidRoot: the data is not a JSON object (or cannot be rewritten as one).
	ErrAuthInvalidRoot = errors.New("grok: auth document is not a JSON object")
	// ErrAuthNoManagedCredential: no OAuth or legacy credential scope is present.
	ErrAuthNoManagedCredential = errors.New("grok: auth document has no managed credential")
	// ErrAuthInvalidManagedCredential: the managed scope lacks an identity or a usable field.
	ErrAuthInvalidManagedCredential = errors.New("grok: managed credential is invalid")
)

// AuthDocument is a parsed Grok auth.json. RawData is the file as read;
// the document never holds a second copy of the tokens.
type AuthDocument struct {
	RawData          []byte
	Identity         CredentialIdentity
	ManagedScopeKeys []string
}

// StableID is the account id of the document's identity.
func (d AuthDocument) StableID() string { return d.Identity.StableID() }

// RequiresReauthentication reports whether the credential can no longer be
// used: no access token, or an expired one with nothing to refresh it.
func (d AuthDocument) RequiresReauthentication(now time.Time) bool {
	if !d.Identity.HasAccessToken {
		return true
	}
	if d.Identity.ExpiresAt == nil {
		return false
	}
	return !d.Identity.ExpiresAt.After(now) && !d.Identity.HasRefreshToken
}

func isManagedScope(scope string) bool {
	return strings.HasPrefix(scope, OAuthScopePrefix) || scope == LegacyScope
}

// ParseAuthDocument parses auth.json content. The selected credential is the
// first OAuth scope in sorted order, else the first managed scope.
func ParseAuthDocument(data []byte) (AuthDocument, error) {
	root, err := authRoot(data)
	if err != nil {
		return AuthDocument{}, err
	}
	var managed []string
	for k := range root {
		if isManagedScope(k) {
			managed = append(managed, k)
		}
	}
	sort.Strings(managed)
	if len(managed) == 0 {
		return AuthDocument{}, ErrAuthNoManagedCredential
	}
	selected := managed[0]
	for _, k := range managed {
		if strings.HasPrefix(k, OAuthScopePrefix) {
			selected = k
			break
		}
	}
	object, ok := root[selected].(map[string]any)
	if !ok {
		return AuthDocument{}, ErrAuthInvalidManagedCredential
	}
	identity, err := parseAuthIdentity(selected, object)
	if err != nil {
		return AuthDocument{}, err
	}
	return AuthDocument{RawData: data, Identity: identity, ManagedScopeKeys: managed}, nil
}

func authRoot(data []byte) (map[string]any, error) {
	root, err := acctParseObject(data)
	if err != nil {
		return nil, ErrAuthInvalidRoot
	}
	return root, nil
}

func parseAuthIdentity(scope string, object map[string]any) (CredentialIdentity, error) {
	kind := AuthKindOAuth
	if scope == LegacyScope {
		kind = AuthKindLegacySession
	}
	email, userID, principalID := authString(object["email"]), authString(object["user_id"]), authString(object["principal_id"])
	if acctFirstNonEmpty(principalID, userID, email) == "" {
		return CredentialIdentity{}, ErrAuthInvalidManagedCredential
	}
	var expiresAt *time.Time
	if raw := authString(object["expires_at"]); raw != "" {
		parsed, ok := acctParseDate(raw)
		if !ok {
			return CredentialIdentity{}, ErrAuthInvalidManagedCredential
		}
		expiresAt = &parsed
	}
	return CredentialIdentity{
		Kind:            kind,
		Scope:           scope,
		Issuer:          authString(object["oidc_issuer"]),
		ClientID:        authString(object["oidc_client_id"]),
		Email:           email,
		UserID:          userID,
		PrincipalID:     principalID,
		PrincipalType:   authString(object["principal_type"]),
		TeamID:          authString(object["team_id"]),
		TeamName:        authString(object["team_name"]),
		ExpiresAt:       expiresAt,
		HasAccessToken:  authString(object["key"]) != "",
		HasRefreshToken: authString(object["refresh_token"]) != "",
	}, nil
}

// authString is the trimmed value of a JSON string; anything else is empty.
func authString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return acctFirstNonEmpty(s)
}

// AccessToken is the OAuth access token ("key") of the managed scope.
func (d AuthDocument) AccessToken() (string, error) { return d.credentialField("key") }

// RefreshToken is the refresh token of the managed scope.
func (d AuthDocument) RefreshToken() (string, error) { return d.credentialField("refresh_token") }

func (d AuthDocument) credentialField(name string) (string, error) {
	root, err := authRoot(d.RawData)
	if err != nil {
		return "", err
	}
	object, ok := root[d.Identity.Scope].(map[string]any)
	if !ok {
		return "", ErrAuthInvalidManagedCredential
	}
	value := authString(object[name])
	if value == "" {
		return "", ErrAuthInvalidManagedCredential
	}
	return value, nil
}

// AccessTokenFromData parses data and returns its access token.
func AccessTokenFromData(data []byte) (string, error) {
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return "", err
	}
	return doc.AccessToken()
}

// ApplyTokenRefresh merges a successful OAuth token response into the managed
// scope of an auth.json document and renders the result. Sibling scopes (an
// API key, a custom provider) and every other field are preserved. The access
// token is always replaced; the refresh token only when the server sent a new
// non-blank one; the expiry only for a positive expiresIn (0 means absent).
func ApplyTokenRefresh(data []byte, accessToken, refreshToken string, expiresIn int, now time.Time) ([]byte, error) {
	doc, err := ParseAuthDocument(data)
	if err != nil {
		return nil, err
	}
	root, err := authRoot(data)
	if err != nil {
		return nil, err
	}
	object, ok := root[doc.Identity.Scope].(map[string]any)
	if !ok {
		return nil, ErrAuthInvalidManagedCredential
	}
	access := acctFirstNonEmpty(accessToken)
	if access == "" {
		return nil, ErrAuthInvalidManagedCredential
	}
	object["key"] = access
	if refresh := acctFirstNonEmpty(refreshToken); refresh != "" {
		object["refresh_token"] = refresh
	}
	if expiresIn > 0 {
		object["expires_at"] = acctFormatSeconds(now.Add(time.Duration(expiresIn) * time.Second))
	}
	encoded, err := acctMarshalJSON(root, acctSerialization, true)
	if err != nil {
		return nil, err
	}
	if _, err := ParseAuthDocument(encoded); err != nil {
		if errors.Is(err, ErrAuthInvalidManagedCredential) || errors.Is(err, ErrAuthNoManagedCredential) {
			return nil, err
		}
		return nil, ErrAuthInvalidRoot
	}
	return encoded, nil
}

// ReplaceManagedScopes returns official with its managed scopes replaced by the
// one credential target selects. Non-managed scopes of official (API keys,
// custom providers) are kept; official may be nil.
func ReplaceManagedScopes(official, target []byte) ([]byte, error) {
	targetDoc, err := ParseAuthDocument(target)
	if err != nil {
		return nil, err
	}
	merged := map[string]any{}
	if official != nil {
		if merged, err = authRoot(official); err != nil {
			return nil, err
		}
	}
	targetRoot, err := authRoot(target)
	if err != nil {
		return nil, err
	}
	for scope := range merged {
		if isManagedScope(scope) {
			delete(merged, scope)
		}
	}
	credential, ok := targetRoot[targetDoc.Identity.Scope]
	if !ok {
		return nil, ErrAuthInvalidManagedCredential
	}
	merged[targetDoc.Identity.Scope] = credential
	return acctMarshalJSON(merged, acctSerialization, true)
}

// authHasAPIKeyScope reports whether data is a JSON object holding the API key scope.
func authHasAPIKeyScope(data []byte) bool {
	root, err := acctParseObject(data)
	if err != nil {
		return false
	}
	_, ok := root[APIKeyScope]
	return ok
}

// authCredentialJSON renders an already decoded JSON object for the importer.
// An encoding failure means a programming error: decoded trees only contain
// the JSON values the shared encoder supports.
func authCredentialJSON(root map[string]any) []byte {
	data, err := acctMarshalJSON(root, acctSerialization, true)
	if err != nil {
		panic(err)
	}
	return data
}
