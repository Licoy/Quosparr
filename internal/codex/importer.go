package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Failure strings that the UI maps to localized text.
const (
	FailureNoCredentials   = "no_credentials"
	FailureNothingSelected = "nothing_selected"
)

// ImportBatchResult is the outcome of importing several credentials.
type ImportBatchResult struct {
	Succeeded []ManagedAccount
	// Counts reflect successful imports, including repeats within one batch.
	CreatedCount int
	UpdatedCount int
	// Failures are display lines of the form "<label>: <reason>".
	Failures      []string
	FailureErrors []error `json:"-"`
}

// ImportConflict says what committing a previewed item would do.
type ImportConflict struct {
	// WillUpdate is true when the item matches an existing account.
	WillUpdate          bool
	ExistingDisplayName string
}

// PreviewItem is one row of the import preview. It holds no secrets.
type PreviewItem struct {
	ID          string
	DisplayName string
	Detail      string
	Conflict    ImportConflict
}

// ImportCandidate is a previewed credential, ready to be committed.
type ImportCandidate struct {
	Preview PreviewItem
	Alias   string
	// Auth is the full credential when known without a refresh.
	Auth *Auth
	// RefreshToken is set for a refresh-only import, exchanged on commit.
	RefreshToken string
	EmailHint    string
}

// ID is the preview id used to select the candidate.
func (c ImportCandidate) ID() string { return c.Preview.ID }

// ImportPreview is what a set of files or a pack would import.
type ImportPreview struct {
	Items         []ImportCandidate
	Failures      []string
	FailureErrors []error `json:"-"`
	// SourceIsPack is true when a transfer pack was among the inputs.
	SourceIsPack bool
	// RoutedProvider is not ProviderCodex when a pack for another provider
	// was opened here, so the UI can offer to route it.
	RoutedProvider Provider
}

// Importer turns local auth, pasted text, files, API keys and packs into
// managed accounts. It never writes the official auth.json.
type Importer struct {
	Store *Store
	// Refresh exchanges the refresh token of an incomplete or expired
	// credential for fresh tokens (see Auth.MergeRefreshResponse). When nil,
	// such credentials fail with ErrRefreshUnavailable.
	Refresh func(ctx context.Context, auth *Auth) error
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (im *Importer) now() time.Time {
	if im.Now != nil {
		return im.Now()
	}
	return time.Now()
}

// ImportOfficial adds the login in ~/.codex/auth.json to the library.
func (im *Importer) ImportOfficial(makeActive bool) (ManagedAccount, error) {
	result, err := im.ImportOfficialWithResult(makeActive)
	return result.Account, err
}

// ImportOfficialWithResult also reports whether a managed account was added.
func (im *Importer) ImportOfficialWithResult(makeActive bool) (UpsertResult, error) {
	return im.Store.ImportOfficialAuthWithResult(makeActive, im.now())
}

// ImportAPIKey adds an OpenAI API key account.
func (im *Importer) ImportAPIKey(apiKey string, makeActive bool) (ManagedAccount, error) {
	key := trimSpace(apiKey)
	if key == "" {
		return ManagedAccount{}, ErrInvalidCredential
	}
	return im.Store.Upsert(APIKeyAuth(key), UpsertOptions{MakeActive: makeActive, Now: im.now()})
}

// ImportPastedText imports every credential found in pasted text: auth.json,
// a browser session JSON, a list of either, or bare refresh tokens.
func (im *Importer) ImportPastedText(ctx context.Context, text string, makeActiveFirst bool) ImportBatchResult {
	candidates := parseCandidates(text)
	if len(candidates) == 0 {
		return ImportBatchResult{Failures: []string{FailureNoCredentials}, FailureErrors: []error{ErrInvalidCredential}}
	}
	return im.importCandidates(ctx, candidates, makeActiveFirst)
}

// ImportFiles imports every credential found in the given files.
func (im *Importer) ImportFiles(ctx context.Context, paths []string, makeActiveFirst bool) ImportBatchResult {
	var candidates []importCandidate
	var failures []string
	var failureErrors []error
	for _, path := range paths {
		name := filepath.Base(path)
		text, err := readTextFile(path)
		if err != nil {
			failures = append(failures, name+": "+err.Error())
			failureErrors = append(failureErrors, err)
			continue
		}
		parsed := parseCandidates(text)
		if len(parsed) == 0 {
			failures = append(failures, name+": no credentials found")
			failureErrors = append(failureErrors, ErrInvalidCredential)
			continue
		}
		candidates = append(candidates, parsed...)
	}
	if len(candidates) == 0 && len(failures) == 0 {
		failures = append(failures, FailureNoCredentials)
		failureErrors = append(failureErrors, ErrInvalidCredential)
	}
	batch := im.importCandidates(ctx, candidates, makeActiveFirst)
	batch.Failures = append(failures, batch.Failures...)
	batch.FailureErrors = append(failureErrors, batch.FailureErrors...)
	return batch
}

func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", nil
	}
	return string(data), nil
}

func (im *Importer) importCandidates(ctx context.Context, candidates []importCandidate, makeActiveFirst bool) ImportBatchResult {
	var result ImportBatchResult
	for _, c := range candidates {
		auth, err := im.materialize(ctx, c.auth, c.refreshToken)
		if err == nil {
			var acc UpsertResult
			acc, err = im.commit(auth, UpsertOptions{MakeActive: makeActiveFirst && len(result.Succeeded) == 0}, c.emailHint)
			if err == nil {
				result.addSuccess(acc)
				continue
			}
		}
		result.Failures = append(result.Failures, c.label+": "+err.Error())
		result.FailureErrors = append(result.FailureErrors, err)
	}
	return result
}

// commit stores auth and fills in an email the token did not carry.
func (im *Importer) commit(auth Auth, opt UpsertOptions, emailHint string) (UpsertResult, error) {
	opt.Now = im.now()
	result, err := im.Store.UpsertWithResult(auth, opt)
	if err != nil {
		return UpsertResult{}, err
	}
	acc := result.Account
	if acc.Email == "" && emailHint != "" {
		acc.Email = emailHint
		if acc.DisplayName == "" || acc.DisplayName == acc.ID {
			acc.DisplayName = emailHint
		}
		if err := im.Store.UpdateMetadata(acc); err != nil {
			return UpsertResult{}, err
		}
	}
	result.Account = acc
	return result, nil
}

// materialize turns a candidate into a complete credential, refreshing it
// when the access token is missing or expired.
func (im *Importer) materialize(ctx context.Context, auth *Auth, refreshToken string) (Auth, error) {
	if auth == nil {
		if refreshToken == "" {
			return Auth{}, ErrInvalidCredential
		}
		fresh := Auth{AuthMode: "chatgpt", Tokens: Tokens{RefreshToken: refreshToken}}
		return im.refreshed(ctx, fresh)
	}
	a := *auth
	if a.IsAPIKey() {
		return a, nil
	}
	expired := IsTokenExpired(a.Tokens.AccessToken, im.now())
	switch {
	case a.CanRefreshOAuth() && (a.Tokens.AccessToken == "" || expired):
		return im.refreshed(ctx, a)
	case a.Tokens.AccessToken == "":
		return Auth{}, ErrInvalidCredential
	case expired:
		return Auth{}, ErrReauthRequired
	}
	return a, nil
}

func (im *Importer) refreshed(ctx context.Context, auth Auth) (Auth, error) {
	if im.Refresh == nil {
		return Auth{}, ErrRefreshUnavailable
	}
	if err := im.Refresh(ctx, &auth); err != nil {
		return Auth{}, err
	}
	return auth, nil
}

// --- preview ------------------------------------------------------------

// PreviewFiles parses files into a selectable preview without writing the
// account library.
func (im *Importer) PreviewFiles(paths []string) ImportPreview {
	preview := ImportPreview{RoutedProvider: ProviderCodex}
	for _, path := range paths {
		im.previewFile(path, &preview)
	}
	if len(preview.Items) == 0 && len(preview.Failures) == 0 {
		preview.Failures = append(preview.Failures, FailureNoCredentials)
		preview.FailureErrors = append(preview.FailureErrors, ErrInvalidCredential)
	}
	preview.Items = dedupeCandidates(preview.Items)
	return preview
}

func (im *Importer) previewFile(path string, preview *ImportPreview) {
	name := filepath.Base(path)
	data, err := os.ReadFile(path)
	if err != nil {
		preview.Failures = append(preview.Failures, name+": "+err.Error())
		preview.FailureErrors = append(preview.FailureErrors, err)
		return
	}
	if pack, ok := DecodeTransferPackIfPack(data); ok {
		preview.SourceIsPack = true
		if pack.Provider != ProviderCodex {
			preview.RoutedProvider = pack.Provider
			preview.Failures = append(preview.Failures, fmt.Sprintf("%s: provider is %s", name, pack.Provider))
			preview.FailureErrors = append(preview.FailureErrors, &ProviderMismatchError{Expected: ProviderCodex, Actual: pack.Provider})
			return
		}
		items, failures, failureErrors := im.previewPackEntries(pack.Accounts, name)
		preview.Items = append(preview.Items, items...)
		preview.Failures = append(preview.Failures, failures...)
		preview.FailureErrors = append(preview.FailureErrors, failureErrors...)
		return
	}
	text := ""
	if utf8.Valid(data) {
		text = string(data)
	}
	candidates := parseCandidates(text)
	if len(candidates) == 0 {
		preview.Failures = append(preview.Failures, name+": no credentials found")
		preview.FailureErrors = append(preview.FailureErrors, ErrInvalidCredential)
		return
	}
	preview.Items = append(preview.Items, im.previewRaw(candidates, name)...)
}

// PreviewPack previews a pack that is already decoded.
func (im *Importer) PreviewPack(pack TransferPack) (ImportPreview, error) {
	if pack.Provider != ProviderCodex {
		return ImportPreview{}, &ProviderMismatchError{Expected: ProviderCodex, Actual: pack.Provider}
	}
	items, failures, failureErrors := im.previewPackEntries(pack.Accounts, "pack")
	return ImportPreview{
		Items: dedupeCandidates(items), Failures: failures, FailureErrors: failureErrors,
		SourceIsPack: true, RoutedProvider: ProviderCodex,
	}, nil
}

func (im *Importer) currentIndex() AccountIndex {
	idx, err := im.Store.LoadIndex()
	if err != nil {
		return NewAccountIndex()
	}
	return idx
}

func conflictFor(idx AccountIndex, auth Auth) ImportConflict {
	for _, acc := range idx.Accounts {
		if AccountMatches(acc, auth) {
			return ImportConflict{WillUpdate: true, ExistingDisplayName: acc.ResolvedDisplayName()}
		}
	}
	return ImportConflict{}
}

func previewID(label, matchKey string) string {
	return "codex-" + StableHash(label+"|"+matchKey)
}

func (im *Importer) previewPackEntries(entries []TransferEntry, prefix string) ([]ImportCandidate, []string, []error) {
	idx := im.currentIndex()
	var items []ImportCandidate
	var failures []string
	var failureErrors []error
	for offset, entry := range entries {
		label := fmt.Sprintf("%s[%d]", prefix, offset)
		auth, err := DecodeAuth(entry.Credential)
		if err != nil {
			failures = append(failures, label+": invalid credential")
			failureErrors = append(failureErrors, err)
			continue
		}
		display := NewManagedAccount("", auth, 0, entry.Alias, "", "", im.now())
		name := firstNonEmpty(entry.Alias, entry.DisplayName, display.ResolvedDisplayName())
		if name == "" {
			name = display.ID
		}
		items = append(items, ImportCandidate{
			Preview: PreviewItem{
				ID:          previewID(label, MatchKey(auth)),
				DisplayName: name,
				Detail:      firstNonEmpty(entry.Email, display.Email),
				Conflict:    conflictFor(idx, auth),
			},
			Alias:     entry.Alias,
			Auth:      &auth,
			EmailHint: firstNonEmpty(entry.Email, display.Email),
		})
	}
	return items, failures, failureErrors
}

func (im *Importer) previewRaw(candidates []importCandidate, prefix string) []ImportCandidate {
	idx := im.currentIndex()
	var items []ImportCandidate
	for offset, c := range candidates {
		label := fmt.Sprintf("%s[%d]", prefix, offset)
		switch {
		case c.auth != nil:
			auth := *c.auth
			display := NewManagedAccount("", auth, 0, "", "", "", im.now())
			name := firstNonEmpty(display.ResolvedDisplayName(), c.emailHint, c.label)
			if name == "" {
				name = c.label
			}
			items = append(items, ImportCandidate{
				Preview: PreviewItem{
					ID:          previewID(label, MatchKey(auth)),
					DisplayName: name,
					Detail:      firstNonEmpty(c.emailHint, display.Email),
					Conflict:    conflictFor(idx, auth),
				},
				Auth:      &auth,
				EmailHint: c.emailHint,
			})
		case c.refreshToken != "":
			name := firstNonEmpty(c.emailHint, c.label)
			if name == "" {
				name = "token"
			}
			items = append(items, ImportCandidate{
				Preview: PreviewItem{
					ID:          previewID(label, refreshMatchKey(c.refreshToken)),
					DisplayName: name,
					Detail:      c.emailHint,
				},
				RefreshToken: c.refreshToken,
				EmailHint:    c.emailHint,
			})
		}
	}
	return items
}

func refreshMatchKey(refresh string) string { return "rt:" + StableHash(refresh) }

// dedupeCandidates drops later items with the same identity, keeping order.
func dedupeCandidates(items []ImportCandidate) []ImportCandidate {
	seen := map[string]bool{}
	var out []ImportCandidate
	for _, item := range items {
		var key string
		switch {
		case item.Auth != nil:
			key = MatchKey(*item.Auth)
		case item.RefreshToken != "":
			key = refreshMatchKey(item.RefreshToken)
		default:
			key = item.ID()
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

// ImportPreviewSelection commits the previewed candidates whose ids are in
// selectedIDs, in preview order.
func (im *Importer) ImportPreviewSelection(
	ctx context.Context, candidates []ImportCandidate, selectedIDs []string, makeActiveFirst bool,
) ImportBatchResult {
	selected := map[string]bool{}
	for _, id := range selectedIDs {
		selected[id] = true
	}
	var result ImportBatchResult
	any := false
	for _, c := range candidates {
		if !selected[c.ID()] {
			continue
		}
		any = true
		if err := im.commitPreview(ctx, c, makeActiveFirst && len(result.Succeeded) == 0, &result); err != nil {
			result.Failures = append(result.Failures, c.Preview.DisplayName+": "+err.Error())
			result.FailureErrors = append(result.FailureErrors, err)
		}
	}
	if !any {
		return ImportBatchResult{Failures: []string{FailureNothingSelected}, FailureErrors: []error{ErrEmptySelection}}
	}
	return result
}

func (im *Importer) commitPreview(ctx context.Context, c ImportCandidate, active bool, result *ImportBatchResult) error {
	auth, err := im.materialize(ctx, c.Auth, c.RefreshToken)
	if err != nil {
		return err
	}
	opt := UpsertOptions{MakeActive: active}
	if c.Alias != "" {
		alias := c.Alias
		opt.Alias = &alias
	}
	acc, err := im.commit(auth, opt, c.EmailHint)
	if err != nil {
		return err
	}
	result.addSuccess(acc)
	return nil
}

func (result *ImportBatchResult) addSuccess(saved UpsertResult) {
	result.Succeeded = append(result.Succeeded, saved.Account)
	if saved.Created {
		result.CreatedCount++
	} else {
		result.UpdatedCount++
	}
}

// --- parsing pasted text ------------------------------------------------

// importCandidate is a credential found in text, before any network step.
type importCandidate struct {
	auth         *Auth
	refreshToken string
	emailHint    string
	label        string
}

func trimSpace(s string) string { return strings.TrimFunc(s, unicode.IsSpace) }

func isNewline(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// parseCandidates finds credentials in text: whole-text JSON (also when
// surrounded by notes), one JSON value or bare token per line, or a single
// bare token.
func parseCandidates(text string) []importCandidate {
	trimmed := trimSpace(text)
	if trimmed == "" {
		return nil
	}
	for _, payload := range jsonPayloadCandidates(trimmed) {
		if value, ok := decodeJSONContainer(payload); ok {
			if parsed := parseJSONValue(value, "json"); len(parsed) > 0 {
				return parsed
			}
		}
	}
	var lines []string
	for _, line := range strings.FieldsFunc(trimmed, isNewline) {
		if line = trimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > 1 {
		if found := parseLines(lines); len(found) > 0 {
			return found
		}
	}
	if looksLikeToken(trimmed) {
		return []importCandidate{{refreshToken: trimmed, label: "token"}}
	}
	return nil
}

func parseLines(lines []string) []importCandidate {
	var out []importCandidate
	for i, line := range lines {
		label := fmt.Sprintf("line %d", i+1)
		if value, ok := decodeJSONContainer(line); ok {
			out = append(out, parseJSONValue(value, label)...)
		} else if looksLikeToken(line) {
			out = append(out, importCandidate{refreshToken: line, label: label})
		}
	}
	return out
}

// decodeJSONContainer decodes s when it is a JSON object or array.
func decodeJSONContainer(s string) (any, bool) {
	if s == "" || (s[0] != '{' && s[0] != '[') || !json.Valid([]byte(s)) {
		return nil, false
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}

// looksLikeToken: refresh tokens are long opaque strings without spaces.
func looksLikeToken(s string) bool {
	return utf8.RuneCountInString(s) >= 20 && !strings.ContainsAny(s, " {[")
}

func parseJSONValue(value any, label string) []importCandidate {
	if list, ok := value.([]any); ok {
		var out []importCandidate
		for i, item := range list {
			out = append(out, parseJSONValue(item, fmt.Sprintf("%s[%d]", label, i))...)
		}
		return out
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	if auth, ok := parseAuthFile(obj); ok {
		return []importCandidate{{auth: &auth, label: label}}
	}
	if auth, ok := parseSessionLike(obj); ok {
		email := firstString(obj, "email")
		if email == "" {
			email = nestedString(obj["user"], "email")
		}
		return []importCandidate{{auth: &auth, emailHint: email, label: label}}
	}
	if refresh := firstNonEmpty(
		stringValue(obj["refresh_token"]), stringValue(obj["refreshToken"]),
		nestedRefreshToken(obj["token"]), nestedRefreshToken(obj["tokens"]),
	); refresh != "" {
		return []importCandidate{{refreshToken: refresh, emailHint: stringValue(obj["email"]), label: label}}
	}
	if key := firstNonEmpty(
		stringValue(obj["OPENAI_API_KEY"]), stringValue(obj["api_key"]), stringValue(obj["apiKey"]),
	); key != "" {
		auth := APIKeyAuth(key)
		return []importCandidate{{auth: &auth, label: label}}
	}
	return nil
}

// parseAuthFile reads a snake_case Codex auth.json object.
func parseAuthFile(obj map[string]any) (Auth, bool) {
	if !hasKey(obj, "tokens") && !hasKey(obj, "OPENAI_API_KEY") && !hasKey(obj, "auth_mode") {
		return Auth{}, false
	}
	raw, err := encodeValue(obj)
	if err != nil {
		return Auth{}, false
	}
	auth, err := DecodeAuth(raw)
	return auth, err == nil
}

func hasKey(obj map[string]any, key string) bool { _, ok := obj[key]; return ok }

// parseSessionLike reads a ChatGPT web session payload, typically
// {"user":{"email":...},"account":{"id":...,"planType":...},"accessToken":...}.
func parseSessionLike(obj map[string]any) (Auth, bool) {
	access := firstNonEmpty(
		firstString(obj, "accessToken", "access_token", "access_token_jwt"),
		nestedString(obj["user"], "accessToken", "access_token"),
		nestedString(obj["session"], "accessToken", "access_token"),
		nestedString(obj["data"], "accessToken", "access_token"),
	)
	refresh := firstNonEmpty(
		firstString(obj, "refreshToken", "refresh_token"),
		nestedString(obj["session"], "refreshToken", "refresh_token"),
		nestedString(obj["tokens"], "refreshToken", "refresh_token"),
		nestedString(obj["token"], "refreshToken", "refresh_token"),
	)
	idToken := firstNonEmpty(
		firstString(obj, "idToken", "id_token"),
		nestedString(obj["session"], "idToken", "id_token"),
	)
	if access == "" {
		if refresh == "" {
			return Auth{}, false
		}
		return Auth{AuthMode: "chatgpt", Tokens: Tokens{IDToken: idToken, RefreshToken: refresh}}, true
	}
	account, _ := obj["account"].(map[string]any)
	_, hasUser := obj["user"].(map[string]any)
	shaped := account != nil || hasUser || hasKey(obj, "expires") || hasKey(obj, "authProvider") ||
		hasKey(obj, "accessToken") || hasKey(obj, "access_token") || refresh != ""
	if !shaped {
		return Auth{}, false
	}
	accessClaims, idClaims := DecodeIdentityClaims(access), DecodeIdentityClaims(idToken)
	accountID := firstNonEmpty(
		firstString(obj, "account_id", "accountId", "chatgpt_account_id"),
		firstString(account, "id", "account_id", "accountId"),
		accessClaims.accountID(), idClaims.accountID(),
		accessClaims.orgID(), idClaims.orgID(),
	)
	plan := firstNonEmpty(
		firstString(obj, "plan_type", "planType", "chatgpt_plan_type"),
		firstString(account, "planType", "plan_type", "chatgpt_plan_type"),
	)
	return Auth{
		AuthMode: "chatgpt",
		Tokens: Tokens{
			IDToken: firstNonEmpty(idToken, access), AccessToken: access,
			RefreshToken: refresh, AccountID: accountID,
		},
		PlanType: plan, AuthFilePlanType: plan,
	}, true
}

func nestedRefreshToken(v any) string {
	return nestedString(v, "refresh_token", "refreshToken")
}

func nestedString(v any, keys ...string) string {
	obj, _ := v.(map[string]any)
	return firstString(obj, keys...)
}

// firstString returns the first non-blank string among keys of obj.
func firstString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := stringValue(obj[key]); s != "" {
			return s
		}
	}
	return ""
}

// stringValue is the trimmed string in v, or "" when v is not a string.
func stringValue(v any) string {
	s, _ := v.(string)
	return trimSpace(s)
}

// jsonPayloadCandidates is the text plus its first balanced {...} and [...]
// slices, without duplicates, so JSON surrounded by notes still parses.
func jsonPayloadCandidates(text string) []string {
	payloads := []string{text}
	for _, pair := range [][2]byte{{'{', '}'}, {'[', ']'}} {
		slice, ok := firstBalancedSlice(text, pair[0], pair[1])
		if ok && !containsString(payloads, slice) {
			payloads = append(payloads, slice)
		}
	}
	return payloads
}

// firstBalancedSlice returns the text from the first open to its matching
// close, honoring JSON string quoting. The delimiters are ASCII, so working
// on bytes is safe for UTF-8 text.
func firstBalancedSlice(text string, open, closing byte) (string, bool) {
	start := strings.IndexByte(text, open)
	if start < 0 {
		return "", false
	}
	depth, inString, escaped := 0, false, false
	for i := start; i < len(text); i++ {
		ch := text[i]
		switch {
		case inString:
			escaped, inString = nextStringState(ch, escaped)
		case ch == '"':
			inString = true
		case ch == open:
			depth++
		case ch == closing:
			if depth--; depth == 0 {
				return text[start : i+1], true
			}
		}
	}
	return "", false
}

// nextStringState advances the in-string scanner by one byte.
func nextStringState(ch byte, escaped bool) (newEscaped, stillInString bool) {
	switch {
	case escaped:
		return false, true
	case ch == '\\':
		return true, true
	case ch == '"':
		return false, false
	}
	return false, true
}
