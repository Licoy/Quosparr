package grok

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/netx"
	"github.com/licoy/quosparr/internal/platform"
)

// xAI / Grok OAuth device-code login (RFC 8628). The official `grok login`
// needs a terminal and a loopback callback, which a tray app cannot provide,
// so the app requests a verification URL, opens it in the browser, polls for
// tokens and writes a Grok-compatible auth.json into an isolated home.
const (
	// OAuthIssuer is the OpenID issuer of xAI accounts.
	OAuthIssuer = "https://auth.x.ai"
	// OAuthDiscoveryURL is the OpenID discovery document.
	OAuthDiscoveryURL = OAuthIssuer + "/.well-known/openid-configuration"
	// OAuthClientID is the public Grok CLI client id.
	OAuthClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	// OAuthScope is the scope requested by the device flow.
	OAuthScope = "openid profile email offline_access grok-cli:access api:access"

	oauthDeviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"
	// OAuthDefaultPollInterval is the minimum polling interval and the step
	// added when the server asks to slow down.
	OAuthDefaultPollInterval = 5 * time.Second
	// OAuthMaxPollDuration caps how long a login waits for the user.
	OAuthMaxPollDuration = 30 * time.Minute

	oauthRequestTimeout  = 30 * time.Second
	oauthMaxResponseSize = 1 << 20
	oauthDefaultExpiry   = 30 * time.Minute
)

// OAuthErrorKind classifies a login failure.
type OAuthErrorKind int

const (
	OAuthDiscoveryFailed OAuthErrorKind = iota
	OAuthInvalidEndpoint
	OAuthDeviceCodeFailed
	OAuthAuthorizationPending
	OAuthSlowDown
	OAuthExpired
	OAuthDenied
	OAuthTokenFailed
	OAuthMissingIdentity
	OAuthWriteFailed
	OAuthBrowserOpenFailed
)

// OAuthError is a login failure. Message never carries credential text.
type OAuthError struct {
	Kind    OAuthErrorKind
	Message string
}

func (e *OAuthError) Error() string {
	switch e.Kind {
	case OAuthDiscoveryFailed:
		return "Grok OAuth discovery failed: " + e.Message
	case OAuthInvalidEndpoint:
		return "Grok OAuth endpoint invalid: " + e.Message
	case OAuthDeviceCodeFailed:
		return "Grok device authorization failed: " + e.Message
	case OAuthAuthorizationPending:
		return "Grok device authorization is still pending."
	case OAuthSlowDown:
		return "Grok device authorization polling must slow down."
	case OAuthExpired:
		return "Grok device code expired."
	case OAuthDenied:
		return "Grok device authorization was denied."
	case OAuthTokenFailed:
		return "Grok token exchange failed: " + e.Message
	case OAuthMissingIdentity:
		return "Grok OAuth response did not include a user identity."
	case OAuthWriteFailed:
		return "Unable to write Grok credentials: " + e.Message
	default:
		return "Unable to open the browser: " + e.Message
	}
}

// Is lets errors.Is match on the kind alone: errors.Is(err, &OAuthError{Kind: OAuthDenied}).
func (e *OAuthError) Is(target error) bool {
	t, ok := target.(*OAuthError)
	return ok && t.Kind == e.Kind && t.Message == ""
}

// DeviceCode is a started device authorization.
type DeviceCode struct {
	DeviceCode              string
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
	TokenEndpoint           string
}

// BrowserURL is the URL to open: the complete one when the server sent it.
func (d DeviceCode) BrowserURL() string {
	if d.VerificationURIComplete != "" {
		return d.VerificationURIComplete
	}
	return d.VerificationURI
}

// TokenBundle is the outcome of a device authorization. ExpiresIn is in
// seconds; zero means the server did not say.
type TokenBundle struct {
	AccessToken   string
	RefreshToken  string
	IDToken       string
	TokenType     string
	ExpiresIn     int
	Email         string
	Subject       string
	TokenEndpoint string
}

// OAuthLogin runs the device-code login. Every field is optional.
type OAuthLogin struct {
	// HTTP nil resolves the current netx context for each request.
	HTTP *http.Client
	// DiscoveryURL defaults to OAuthDiscoveryURL.
	DiscoveryURL string
	// OpenURL opens the verification page; nil means the system browser. A
	// failure is ignored: the user can still visit the URL by hand.
	OpenURL func(url string) error
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Sleep waits between polls; nil sleeps for real. Tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error
	// OnDeviceCode hears about the started authorization (to show the code).
	OnDeviceCode func(DeviceCode)
}

func (l OAuthLogin) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l OAuthLogin) sleep(ctx context.Context, d time.Duration) error {
	if l.Sleep != nil {
		return l.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l OAuthLogin) discoveryURL() string {
	if l.DiscoveryURL != "" {
		return l.DiscoveryURL
	}
	return OAuthDiscoveryURL
}

// Login runs the device flow and writes auth.json into homeDir (directory
// 0700, file 0600, atomic).
func (l OAuthLogin) Login(ctx context.Context, homeDir string) error {
	device, err := l.StartDeviceFlow(ctx)
	if err != nil {
		return err
	}
	if l.OnDeviceCode != nil {
		l.OnDeviceCode(device)
	}
	open := l.OpenURL
	if open == nil {
		open = OpenInDefaultBrowser
	}
	// Best effort: the device code stays valid, so a failure does not abort.
	_ = open(device.BrowserURL())
	tokens, err := l.WaitForAuthorization(ctx, device)
	if err != nil {
		return err
	}
	data, err := MakeAuthDocumentData(tokens, l.now())
	if err != nil {
		return err
	}
	return writeAuthJSON(data, homeDir)
}

// StartDeviceFlow discovers the endpoints and requests a device code.
func (l OAuthLogin) StartDeviceFlow(ctx context.Context) (DeviceCode, error) {
	deviceEndpoint, tokenEndpoint, err := l.discover(ctx)
	if err != nil {
		return DeviceCode{}, err
	}
	return l.requestDeviceCode(ctx, deviceEndpoint, tokenEndpoint)
}

// WaitForAuthorization polls the token endpoint until the user approves, the
// code expires or ctx ends.
func (l OAuthLogin) WaitForAuthorization(ctx context.Context, device DeviceCode) (TokenBundle, error) {
	interval := max(device.Interval, OAuthDefaultPollInterval)
	started := l.now()
	deadline := started.Add(min(max(time.Second, device.ExpiresIn), OAuthMaxPollDuration))
	for first := true; ; first = false {
		if err := ctx.Err(); err != nil {
			return TokenBundle{}, err
		}
		if !first {
			if l.now().After(deadline) {
				return TokenBundle{}, &OAuthError{Kind: OAuthExpired}
			}
			if err := l.sleep(ctx, interval); err != nil {
				return TokenBundle{}, err
			}
		}
		tokens, err := l.exchangeDeviceCode(ctx, device.DeviceCode, device.TokenEndpoint)
		switch {
		case err == nil:
			return tokens, nil
		case errors.Is(err, &OAuthError{Kind: OAuthAuthorizationPending}):
		case errors.Is(err, &OAuthError{Kind: OAuthSlowDown}):
			interval += OAuthDefaultPollInterval
		default:
			return TokenBundle{}, err
		}
	}
}

func (l OAuthLogin) discover(ctx context.Context) (deviceEndpoint, tokenEndpoint string, err error) {
	status, body, err := oauthDo(ctx, l.HTTP, http.MethodGet, l.discoveryURL(), nil, oauthRequestTimeout)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", "", ctxErr
		}
		return "", "", &OAuthError{Kind: OAuthDiscoveryFailed, Message: shortError(err)}
	}
	if status < 200 || status > 299 {
		return "", "", &OAuthError{Kind: OAuthDiscoveryFailed, Message: fmt.Sprintf("HTTP %d", status)}
	}
	object, err := acctParseObject(body)
	if err != nil {
		return "", "", &OAuthError{Kind: OAuthDiscoveryFailed, Message: "invalid JSON"}
	}
	deviceEndpoint, err = validateOAuthEndpoint(oauthString(object["device_authorization_endpoint"]), "device_authorization_endpoint")
	if err != nil {
		return "", "", err
	}
	tokenEndpoint, err = validateOAuthEndpoint(oauthString(object["token_endpoint"]), "token_endpoint")
	if err != nil {
		return "", "", err
	}
	return deviceEndpoint, tokenEndpoint, nil
}

func (l OAuthLogin) requestDeviceCode(ctx context.Context, deviceEndpoint, tokenEndpoint string) (DeviceCode, error) {
	form := "client_id=" + formEncode(OAuthClientID) + "&scope=" + formEncode(OAuthScope)
	status, body, err := oauthDo(ctx, l.HTTP, http.MethodPost, deviceEndpoint, strings.NewReader(form), oauthRequestTimeout)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return DeviceCode{}, ctxErr
		}
		return DeviceCode{}, &OAuthError{Kind: OAuthDeviceCodeFailed, Message: shortError(err)}
	}
	if status < 200 || status > 299 {
		text := truncateRunes(strings.TrimFunc(string(body), unicode.IsSpace), 200)
		return DeviceCode{}, &OAuthError{Kind: OAuthDeviceCodeFailed, Message: fmt.Sprintf("HTTP %d %s", status, text)}
	}
	object, err := acctParseObject(body)
	if err != nil {
		return DeviceCode{}, &OAuthError{Kind: OAuthDeviceCodeFailed, Message: "invalid JSON"}
	}
	deviceCode, userCode := oauthString(object["device_code"]), oauthString(object["user_code"])
	if deviceCode == "" || userCode == "" {
		return DeviceCode{}, &OAuthError{Kind: OAuthDeviceCodeFailed, Message: "missing device_code or user_code"}
	}
	verification := oauthString(object["verification_uri"])
	if verification == "" {
		verification = oauthString(object["verification_uri_complete"])
	}
	if !isWebURL(verification) {
		return DeviceCode{}, &OAuthError{Kind: OAuthDeviceCodeFailed, Message: "missing or invalid verification_uri"}
	}
	complete := oauthString(object["verification_uri_complete"])
	if _, err := url.Parse(complete); err != nil {
		complete = ""
	}
	expires, interval := oauthDefaultExpiry, OAuthDefaultPollInterval
	if n, ok := oauthNumber(object["expires_in"]); ok {
		expires = time.Duration(n) * time.Second
	}
	if n, ok := oauthNumber(object["interval"]); ok {
		interval = time.Duration(n) * time.Second
	}
	return DeviceCode{
		DeviceCode: deviceCode, UserCode: userCode, VerificationURI: verification,
		VerificationURIComplete: complete, ExpiresIn: expires, Interval: interval, TokenEndpoint: tokenEndpoint,
	}, nil
}

func (l OAuthLogin) exchangeDeviceCode(ctx context.Context, deviceCode, tokenEndpoint string) (TokenBundle, error) {
	form := "grant_type=" + formEncode(oauthDeviceGrantType) + "&device_code=" + formEncode(deviceCode) +
		"&client_id=" + formEncode(OAuthClientID)
	status, body, err := oauthDo(ctx, l.HTTP, http.MethodPost, tokenEndpoint, strings.NewReader(form), oauthRequestTimeout)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return TokenBundle{}, ctxErr
		}
		return TokenBundle{}, &OAuthError{Kind: OAuthTokenFailed, Message: shortError(err)}
	}
	object, err := acctParseObject(body)
	if err != nil {
		return TokenBundle{}, &OAuthError{Kind: OAuthTokenFailed, Message: "invalid JSON"}
	}
	if code := oauthString(object["error"]); code != "" {
		switch code {
		case "authorization_pending":
			return TokenBundle{}, &OAuthError{Kind: OAuthAuthorizationPending}
		case "slow_down":
			return TokenBundle{}, &OAuthError{Kind: OAuthSlowDown}
		case "expired_token":
			return TokenBundle{}, &OAuthError{Kind: OAuthExpired}
		case "access_denied":
			return TokenBundle{}, &OAuthError{Kind: OAuthDenied}
		}
		description := oauthString(object["error_description"])
		if description == "" {
			description = code
		}
		return TokenBundle{}, &OAuthError{Kind: OAuthTokenFailed, Message: description}
	}
	if status < 200 || status > 299 {
		return TokenBundle{}, &OAuthError{Kind: OAuthTokenFailed, Message: fmt.Sprintf("HTTP %d", status)}
	}
	access := oauthString(object["access_token"])
	if access == "" {
		return TokenBundle{}, &OAuthError{Kind: OAuthTokenFailed, Message: "missing access_token"}
	}
	idToken := oauthString(object["id_token"])
	claims := jwtClaims(idToken)
	bundle := TokenBundle{
		AccessToken:   access,
		RefreshToken:  oauthString(object["refresh_token"]),
		IDToken:       idToken,
		TokenType:     oauthString(object["token_type"]),
		Email:         firstOf(oauthString(object["email"]), oauthString(claims["email"])),
		Subject:       firstOf(oauthString(object["sub"]), oauthString(claims["sub"])),
		TokenEndpoint: tokenEndpoint,
	}
	if n, ok := oauthNumber(object["expires_in"]); ok {
		bundle.ExpiresIn = n
	}
	return bundle, nil
}

// MakeAuthDocumentData renders the Grok auth.json for a finished login. The
// account needs a subject or an email; without a subject the user id is
// derived from the email.
func MakeAuthDocumentData(tokens TokenBundle, now time.Time) ([]byte, error) {
	email, subject := acctFirstNonEmpty(tokens.Email), acctFirstNonEmpty(tokens.Subject)
	if subject == "" && email == "" {
		return nil, &OAuthError{Kind: OAuthMissingIdentity}
	}
	userID := subject
	if userID == "" {
		userID = "email-" + codex.StableHash(strings.ToLower(email))
	}
	principalID := subject
	if principalID == "" {
		principalID = userID
	}
	credential := map[string]any{
		"auth_mode":      "oidc",
		"create_time":    acctFormatSeconds(now),
		"key":            tokens.AccessToken,
		"oidc_client_id": OAuthClientID,
		"oidc_issuer":    OAuthIssuer,
		"principal_id":   principalID,
		"principal_type": "User",
		"user_id":        userID,
	}
	if email != "" {
		credential["email"] = email
	}
	if tokens.RefreshToken != "" {
		credential["refresh_token"] = tokens.RefreshToken
	}
	if tokens.ExpiresIn > 0 {
		credential["expires_at"] = acctFormatSeconds(now.Add(time.Duration(tokens.ExpiresIn) * time.Second))
	}
	data, err := acctMarshalJSON(map[string]any{OAuthScopePrefix + OAuthClientID: credential}, acctSerialization, true)
	if err != nil {
		return nil, err
	}
	if _, err := ParseAuthDocument(data); err != nil {
		return nil, &OAuthError{Kind: OAuthWriteFailed, Message: "unable to encode Grok auth.json"}
	}
	return data, nil
}

func writeAuthJSON(data []byte, homeDir string) error {
	if err := platform.EnsurePrivateDirectory(homeDir); err != nil {
		return &OAuthError{Kind: OAuthWriteFailed, Message: "unable to protect the Grok home"}
	}
	if err := platform.WriteFileAtomic(homeDir+string(os.PathSeparator)+authFileName, data, storeFilePerm); err != nil {
		return &OAuthError{Kind: OAuthWriteFailed, Message: "unable to write auth.json"}
	}
	return nil
}

func validateOAuthEndpoint(raw, field string) (string, error) {
	if raw == "" {
		return "", &OAuthError{Kind: OAuthInvalidEndpoint, Message: field + " is empty"}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", &OAuthError{Kind: OAuthInvalidEndpoint, Message: field + " is empty"}
	}
	if strings.ToLower(u.Scheme) != "https" {
		return "", &OAuthError{Kind: OAuthInvalidEndpoint, Message: field + " must use https"}
	}
	host := strings.ToLower(u.Hostname())
	if host != "x.ai" && !strings.HasSuffix(host, ".x.ai") {
		return "", &OAuthError{Kind: OAuthInvalidEndpoint, Message: field + " host is not on x.ai"}
	}
	return raw, nil
}

func isWebURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || raw == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "https" || scheme == "http"
}

// jwtClaims reads the payload of a JWT without verifying it; the login only
// uses it as a hint for the account's email and subject.
func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil
	}
	object, err := acctParseObject(payload)
	if err != nil {
		return nil
	}
	return object
}

// oauthString is the trimmed value of a JSON string; anything else is empty.
func oauthString(v any) string { return authString(v) }

// oauthNumber reads an integer like the Swift app: a JSON number (fractions
// cut) or a string holding an integer.
func oauthNumber(v any) (int, bool) {
	switch t := v.(type) {
	case json.Number:
		if n, err := strconv.ParseInt(string(t), 10, 64); err == nil {
			return int(n), true
		}
		if f, err := strconv.ParseFloat(string(t), 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
			return int(f), true
		}
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n, true
		}
	}
	return 0, false
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// formEncode percent-encodes everything but unreserved characters, so a token
// holding "+", "&" or "=" cannot break the form body.
func formEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// shortError is an error message cut to 200 characters.
func shortError(err error) string { return truncateRunes(err.Error(), 200) }

// oauthDo sends one request and returns the status and (limited) body. The
// body never becomes part of an error here.
func oauthDo(
	ctx context.Context, client *http.Client, method, rawURL string, body io.Reader, timeout time.Duration,
) (int, []byte, error) {
	if client == nil {
		var err error
		if client, err = netx.ClientFor(ctx, netx.Standard); err != nil {
			return 0, nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := netx.DoAuthenticated(client, req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, oauthMaxResponseSize))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, data, nil
}
