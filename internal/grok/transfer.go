package grok

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/platform"
)

// Failure strings that the UI maps to localized text (shared with Codex).
const (
	FailureNoCredentials   = codex.FailureNoCredentials
	FailureNothingSelected = codex.FailureNothingSelected
)

// EncodePack renders a Grok transfer pack the way the macOS app writes it:
// pretty JSON, localized key order, slashes unescaped. Credentials are
// embedded as JSON objects.
func EncodePack(pack codex.TransferPack) ([]byte, error) {
	if pack.Format != codex.TransferFormat {
		return nil, &codex.UnsupportedFormatError{Detail: pack.Format}
	}
	if pack.Version != codex.TransferVersion {
		return nil, &codex.UnsupportedVersionError{Version: pack.Version}
	}
	root := map[string]any{
		"format":     pack.Format,
		"version":    acctInt(pack.Version),
		"provider":   string(pack.Provider),
		"exportedAt": acctFormatMillis(pack.ExportedAt),
	}
	if pack.AppVersion != "" {
		root["appVersion"] = pack.AppVersion
	}
	accounts := make([]any, 0, len(pack.Accounts))
	for _, entry := range pack.Accounts {
		credential, ok := acctJSONContainer(entry.Credential)
		if !ok {
			return nil, ErrInvalidCredential
		}
		o := map[string]any{"credential": credential}
		for key, value := range map[string]string{
			"id": entry.ID, "alias": entry.Alias, "displayName": entry.DisplayName, "email": entry.Email,
		} {
			if value != "" {
				o[key] = value
			}
		}
		accounts = append(accounts, o)
	}
	root["accounts"] = accounts
	return acctMarshalJSON(root, acctSerialization, true)
}

// WritePack encodes pack and writes it to path atomically with owner-only
// permissions. On failure no partial export is left behind.
func WritePack(path string, pack codex.TransferPack) error {
	data, err := EncodePack(pack)
	if err != nil {
		return err
	}
	if err := platform.WriteFileAtomic(path, data, storeFilePerm); err != nil {
		return err
	}
	if err := platform.ProtectOwnerOnly(path, storeFilePerm); err == nil && runtime.GOOS != "windows" {
		if !fileModeIs(path, storeFilePerm) {
			err = errors.New("grok: unable to enforce owner-only permissions on the account export")
			_ = os.Remove(path)
			return err
		}
	} else if err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

// decodePack reads a transfer pack, returning ok=false for any other file.
// Credentials are re-rendered like the Swift decoder does (compact, localized
// key order, normalized numbers).
func decodePack(data []byte) (codex.TransferPack, bool) {
	pack, ok := codex.DecodeTransferPackIfPack(data)
	if !ok {
		return codex.TransferPack{}, false
	}
	for i, entry := range pack.Accounts {
		value, ok := acctJSONContainer(entry.Credential)
		if !ok {
			return codex.TransferPack{}, false
		}
		encoded, err := acctMarshalJSON(value, acctSerialization, false)
		if err != nil {
			return codex.TransferPack{}, false
		}
		pack.Accounts[i].Credential = encoded
	}
	return pack, true
}

// Exporter builds portable Grok packs from the managed library.
type Exporter struct {
	Store      *Store
	AppVersion string
}

// Export gathers the credentials of ids into a pack. Accounts that are
// unknown or lack a credential file are reported in Failures and skipped.
func (e Exporter) Export(ids []string, now time.Time) (codex.ExportResult, error) {
	if len(ids) == 0 {
		return codex.ExportResult{}, codex.ErrEmptySelection
	}
	idx, err := e.Store.LoadIndex()
	if err != nil {
		return codex.ExportResult{}, err
	}
	var entries []codex.TransferEntry
	var failures []string
	var failureErrors []error
	for _, id := range ids {
		meta, ok := idx.Account(id)
		if !ok {
			failures = append(failures, id+": account not found")
			failureErrors = append(failureErrors, &AccountNotFoundError{ID: id})
			continue
		}
		credential, err := e.Store.LoadCredentialData(id)
		if err != nil {
			failures = append(failures, meta.ResolvedDisplayName()+": credential missing")
			failureErrors = append(failureErrors, err)
			continue
		}
		entries = append(entries, codex.TransferEntry{
			ID: meta.ID, Alias: meta.Alias, DisplayName: meta.ResolvedDisplayName(),
			Email: meta.Email(), Credential: credential,
		})
	}
	if len(entries) == 0 {
		return codex.ExportResult{Failures: failures, FailureErrors: failureErrors}, codex.ErrNoAccountsExported
	}
	pack := codex.NewTransferPack(codex.ProviderGrok, now, e.AppVersion, entries)
	return codex.ExportResult{Pack: pack, ExportedCount: len(entries), Failures: failures, FailureErrors: failureErrors}, nil
}

// Write exports ids and writes the pack to path.
func (e Exporter) Write(ids []string, path string, now time.Time) (codex.ExportResult, error) {
	result, err := e.Export(ids, now)
	if err != nil {
		return result, err
	}
	if err := WritePack(path, result.Pack); err != nil {
		return result, err
	}
	return result, nil
}

// ImportCandidate is a previewed credential, ready to be committed.
type ImportCandidate struct {
	Preview codex.PreviewItem
	Alias   string
	// CredentialData is the normalized auth.json ParseAuthDocument accepts.
	CredentialData []byte
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
	// RoutedProvider is not ProviderGrok when a pack for another provider was
	// opened here, so the UI can offer to route it.
	RoutedProvider codex.Provider
}

// PreviewItems are the rows to show, without credentials.
func (p ImportPreview) PreviewItems() []codex.PreviewItem {
	items := make([]codex.PreviewItem, len(p.Items))
	for i, c := range p.Items {
		items[i] = c.Preview
	}
	return items
}

// TransferImporter previews and commits Grok packs and credential files.
type TransferImporter struct {
	Store *Store
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (im TransferImporter) now() time.Time {
	if im.Now != nil {
		return im.Now()
	}
	return time.Now()
}

// PreviewFiles parses packs and raw credential files into a selectable
// preview without writing the library.
func (im TransferImporter) PreviewFiles(paths []string) ImportPreview {
	preview := ImportPreview{RoutedProvider: codex.ProviderGrok}
	for _, path := range paths {
		im.previewFile(path, &preview)
	}
	if len(preview.Items) == 0 && len(preview.Failures) == 0 {
		preview.Failures = append(preview.Failures, FailureNoCredentials)
		preview.FailureErrors = append(preview.FailureErrors, ErrInvalidCredential)
	}
	preview.Items = deduplicateCandidates(preview.Items)
	return preview
}

func (im TransferImporter) previewFile(path string, preview *ImportPreview) {
	name := filepath.Base(path)
	data, err := os.ReadFile(path)
	if err != nil {
		preview.Failures = append(preview.Failures, name+": unable to read the file")
		preview.FailureErrors = append(preview.FailureErrors, err)
		return
	}
	if pack, ok := decodePack(data); ok {
		preview.SourceIsPack = true
		if pack.Provider != codex.ProviderGrok {
			preview.RoutedProvider = pack.Provider
			preview.Failures = append(preview.Failures, fmt.Sprintf("%s: provider is %s", name, pack.Provider))
			preview.FailureErrors = append(preview.FailureErrors, &codex.ProviderMismatchError{Expected: codex.ProviderGrok, Actual: pack.Provider})
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
	payloads := ParsePayloads(text)
	if len(payloads) == 0 {
		preview.Failures = append(preview.Failures, name+": no credentials found")
		preview.FailureErrors = append(preview.FailureErrors, ErrInvalidCredential)
		return
	}
	for offset, payload := range payloads {
		doc, err := ParseAuthDocument(payload)
		if err != nil {
			continue
		}
		preview.Items = append(preview.Items, im.makeCandidate(doc, payload, "", "", "", fmt.Sprintf("%s[%d]", name, offset)))
	}
}

// PreviewPack previews a pack that is already decoded.
func (im TransferImporter) PreviewPack(pack codex.TransferPack) (ImportPreview, error) {
	if pack.Provider != codex.ProviderGrok {
		return ImportPreview{}, &codex.ProviderMismatchError{Expected: codex.ProviderGrok, Actual: pack.Provider}
	}
	items, failures, failureErrors := im.previewPackEntries(pack.Accounts, "pack")
	return ImportPreview{
		Items: deduplicateCandidates(items), Failures: failures, FailureErrors: failureErrors,
		SourceIsPack: true, RoutedProvider: codex.ProviderGrok,
	}, nil
}

func (im TransferImporter) previewPackEntries(entries []codex.TransferEntry, prefix string) ([]ImportCandidate, []string, []error) {
	var items []ImportCandidate
	var failures []string
	var failureErrors []error
	for offset, entry := range entries {
		label := fmt.Sprintf("%s[%d]", prefix, offset)
		doc, err := ParseAuthDocument(entry.Credential)
		if err != nil {
			failures = append(failures, label+": invalid credential")
			failureErrors = append(failureErrors, err)
			continue
		}
		items = append(items, im.makeCandidate(doc, entry.Credential, entry.Alias, entry.DisplayName, entry.Email, label))
	}
	return items, failures, failureErrors
}

func (im TransferImporter) makeCandidate(
	doc AuthDocument, credential []byte, alias, displayOverride, emailOverride, label string,
) ImportCandidate {
	idx, err := im.Store.LoadIndex()
	if err != nil {
		idx = NewAccountIndex()
	}
	conflict := codex.ImportConflict{}
	for _, existing := range idx.Accounts {
		if existing.Identity.StableID() == doc.Identity.StableID() || identitiesLooselyMatch(existing.Identity, doc.Identity) {
			conflict = codex.ImportConflict{WillUpdate: true, ExistingDisplayName: existing.ResolvedDisplayName()}
			break
		}
	}
	displayName := acctFirstNonEmpty(alias, displayOverride, doc.Identity.ResolvedDisplayName(), doc.StableID())
	return ImportCandidate{
		Preview: codex.PreviewItem{
			ID:          "grok-" + codex.StableHash(label+"|"+doc.StableID()),
			DisplayName: displayName,
			Detail:      acctFirstNonEmpty(emailOverride, doc.Identity.Email),
			Conflict:    conflict,
		},
		Alias:          alias,
		CredentialData: credential,
	}
}

func deduplicateCandidates(items []ImportCandidate) []ImportCandidate {
	seen := map[string]bool{}
	var out []ImportCandidate
	for _, item := range items {
		key := item.ID()
		if doc, err := ParseAuthDocument(item.CredentialData); err == nil {
			key = doc.StableID()
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

// identitiesLooselyMatch is the preview's matching: same stable id, or the
// same user, principal or email (case-insensitive) regardless of team.
func identitiesLooselyMatch(a, b CredentialIdentity) bool {
	if a.StableID() == b.StableID() {
		return true
	}
	for _, pair := range [][2]string{{a.UserID, b.UserID}, {a.PrincipalID, b.PrincipalID}, {a.Email, b.Email}} {
		l, r := acctFirstNonEmpty(pair[0]), acctFirstNonEmpty(pair[1])
		if l != "" && r != "" && strings.EqualFold(l, r) {
			return true
		}
	}
	return false
}

// ImportPreviewSelection commits the previewed candidates whose ids are in
// selectedIDs, in preview order. With makeCurrentFirst the first imported
// account becomes current when the library is empty and the official file
// holds no managed login. It writes the library only; installing a current
// account into the official file is the module's job.
func (im TransferImporter) ImportPreviewSelection(
	candidates []ImportCandidate, selectedIDs []string, makeCurrentFirst bool,
) (ImportBatchResult, error) {
	selected := map[string]bool{}
	for _, id := range selectedIDs {
		selected[id] = true
	}
	var chosen []ImportCandidate
	for _, c := range candidates {
		if selected[c.ID()] {
			chosen = append(chosen, c)
		}
	}
	if len(chosen) == 0 {
		return ImportBatchResult{Failures: []string{FailureNothingSelected}, FailureErrors: []error{codex.ErrEmptySelection}}, nil
	}
	before, err := im.Store.LoadIndex()
	if err != nil {
		return ImportBatchResult{}, err
	}
	status := im.Store.LoadOfficialCredentialStatus(im.now())
	canBootstrap := makeCurrentFirst && len(before.Accounts) == 0 && !status.HasManagedLogin()

	var result ImportBatchResult
	isFirst := true
	for _, c := range chosen {
		account, err := im.Store.UpsertCredentialData(c.CredentialData, UpsertOptions{MakeCurrent: isFirst && canBootstrap, Now: im.now()})
		if err != nil {
			result.Failures = append(result.Failures, c.Preview.DisplayName+": import failed")
			result.FailureErrors = append(result.FailureErrors, err)
			continue
		}
		if alias := strings.TrimFunc(c.Alias, unicode.IsSpace); alias != "" {
			account.Alias = alias
			if err := im.Store.UpdateMetadata(account); err != nil {
				result.Failures = append(result.Failures, c.Preview.DisplayName+": import failed")
				result.FailureErrors = append(result.FailureErrors, err)
				continue
			}
		}
		result.Succeeded = append(result.Succeeded, account)
		isFirst = false
	}
	return result, nil
}
