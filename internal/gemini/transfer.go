package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/licoy/quosparr/internal/codex"
)

// ProviderGemini marks Gemini transfer packs.
const ProviderGemini codex.Provider = "gemini"

// Exporter builds portable Gemini packs from the managed library. Packs use
// the shared transfer format with provider "gemini"; each credential is the
// account's managed document.
type Exporter struct {
	Store      *Store
	AppVersion string
}

// Export gathers the credentials of ids into a pack. Unknown accounts and
// missing credentials are reported in Failures and skipped.
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
			ID: meta.ID, Alias: meta.Alias, DisplayName: meta.ResolvedDisplayName(), Email: meta.Email(), Credential: credential,
		})
	}
	if len(entries) == 0 {
		return codex.ExportResult{Failures: failures, FailureErrors: failureErrors}, codex.ErrNoAccountsExported
	}
	pack := codex.NewTransferPack(ProviderGemini, now, e.AppVersion, entries)
	return codex.ExportResult{Pack: pack, ExportedCount: len(entries), Failures: failures, FailureErrors: failureErrors}, nil
}

// Write exports ids and writes the pack to path (atomic, owner only).
func (e Exporter) Write(ids []string, path string, now time.Time) (codex.ExportResult, error) {
	result, err := e.Export(ids, now)
	if err != nil {
		return result, err
	}
	return result, codex.WriteTransferPack(path, result.Pack)
}

// ExportAccounts builds an in-memory pack of the selected accounts.
func (m *Module) ExportAccounts(ids []string) (codex.ExportResult, error) {
	return Exporter{Store: m.Store, AppVersion: m.AppVersion}.Export(ids, m.now())
}

// ImportCandidate is a previewed credential, ready to be committed.
type ImportCandidate struct {
	Preview codex.PreviewItem
	Alias   string
	// Identity is known when the preview could name the account offline.
	Identity       Identity
	CredentialData []byte
}

// ID is the preview id used to select the candidate.
func (c ImportCandidate) ID() string { return c.Preview.ID }

// ImportPreview is what a set of files would import.
type ImportPreview struct {
	Items         []ImportCandidate
	Failures      []string
	FailureErrors []error `json:"-"`
	SourceIsPack  bool
	// RoutedProvider is not ProviderGemini when a pack of another provider
	// was opened here.
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

// PreviewImportFiles previews packs and credential files without network
// access and without writing the library.
func (m *Module) PreviewImportFiles(ctx context.Context, paths []string) (ImportPreview, error) {
	if err := m.acquire(ctx); err != nil {
		return ImportPreview{}, err
	}
	defer m.release()
	preview := ImportPreview{RoutedProvider: ProviderGemini}
	for _, path := range paths {
		m.previewFile(path, &preview)
	}
	if len(preview.Items) == 0 && len(preview.Failures) == 0 {
		preview.Failures = append(preview.Failures, FailureNoCredentials)
		preview.FailureErrors = append(preview.FailureErrors, ErrInvalidCredential)
	}
	preview.Items = deduplicateCandidates(preview.Items)
	return preview, nil
}

type packEntry struct {
	Alias       string          `json:"alias"`
	DisplayName string          `json:"displayName"`
	Email       string          `json:"email"`
	Credential  json.RawMessage `json:"credential"`
}

type packFile struct {
	Format   string      `json:"format"`
	Version  int         `json:"version"`
	Provider string      `json:"provider"`
	Accounts []packEntry `json:"accounts"`
}

func (m *Module) previewFile(path string, preview *ImportPreview) {
	name := filepath.Base(path)
	data, err := os.ReadFile(path)
	if err != nil {
		preview.Failures = append(preview.Failures, name+": unable to read the file")
		preview.FailureErrors = append(preview.FailureErrors, err)
		return
	}
	var pack packFile
	if json.Unmarshal(data, &pack) == nil && (pack.Format == codex.TransferFormat || pack.Format == codex.LegacyTransferFormat) {
		preview.SourceIsPack = true
		if codex.Provider(pack.Provider) != ProviderGemini {
			preview.RoutedProvider = codex.Provider(pack.Provider)
			preview.Failures = append(preview.Failures, fmt.Sprintf("%s: provider is %s", name, pack.Provider))
			preview.FailureErrors = append(preview.FailureErrors, &codex.ProviderMismatchError{Expected: ProviderGemini, Actual: codex.Provider(pack.Provider)})
			return
		}
		if pack.Version != codex.TransferVersion {
			preview.Failures = append(preview.Failures, name+": unsupported pack version")
			preview.FailureErrors = append(preview.FailureErrors, &codex.UnsupportedVersionError{Version: pack.Version})
			return
		}
		for i, entry := range pack.Accounts {
			label := fmt.Sprintf("%s[%d]", name, i)
			if _, err := ParseCredential(entry.Credential); err != nil {
				preview.Failures = append(preview.Failures, label+": invalid credential")
				preview.FailureErrors = append(preview.FailureErrors, err)
				continue
			}
			preview.Items = append(preview.Items, m.makeCandidate(entry.Credential, trimSpace(entry.Alias), entry.DisplayName, entry.Email, label))
		}
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
	for i, payload := range payloads {
		preview.Items = append(preview.Items, m.makeCandidate(payload, "", "", "", fmt.Sprintf("%s[%d]", name, i)))
	}
}

// makeCandidate names the account offline: a managed copy with the same
// refresh token, else the id_token claims. Otherwise the identity is
// resolved when the candidate is committed.
func (m *Module) makeCandidate(credential []byte, alias, displayName, email, label string) ImportCandidate {
	cred, _ := ParseCredential(credential)
	identity := cred.IDTokenIdentity()
	if account, ok := m.Store.AccountForLogin(cred); ok {
		identity = account.Identity
	}
	conflict := codex.ImportConflict{}
	if idx, err := m.Store.LoadIndex(); err == nil && identity.Known() {
		if existing, ok := idx.matching(identity); ok {
			conflict = codex.ImportConflict{WillUpdate: true, ExistingDisplayName: existing.ResolvedDisplayName()}
		}
	}
	return ImportCandidate{
		Preview: codex.PreviewItem{
			ID:          "gemini-" + codex.StableHash(label+"|"+firstNonEmpty(cred.RefreshToken, cred.AccessToken)),
			DisplayName: firstNonEmpty(alias, displayName, identity.ResolvedDisplayName(), email, label),
			Detail:      firstNonEmpty(email, identity.Email),
			Conflict:    conflict,
		},
		Alias:          alias,
		Identity:       identity,
		CredentialData: credential,
	}
}

func deduplicateCandidates(items []ImportCandidate) []ImportCandidate {
	seen := map[string]bool{}
	var out []ImportCandidate
	for _, item := range items {
		key := item.ID()
		if cred, err := ParseCredential(item.CredentialData); err == nil {
			key = firstNonEmpty(cred.RefreshToken, cred.AccessToken)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

// ImportPreviewSelection commits the selected candidates in preview order.
// It writes the library only, never the official login.
func (m *Module) ImportPreviewSelection(
	ctx context.Context, candidates []ImportCandidate, selectedIDs []string,
) (ImportBatchResult, AccountState, error) {
	if err := m.acquire(ctx); err != nil {
		return ImportBatchResult{}, AccountState{}, err
	}
	defer m.release()
	selected := map[string]bool{}
	for _, id := range selectedIDs {
		selected[id] = true
	}
	var entries []importEntry
	for _, c := range candidates {
		if selected[c.ID()] {
			entries = append(entries, importEntry{label: c.Preview.DisplayName, data: c.CredentialData, alias: c.Alias, identity: c.Identity})
		}
	}
	if len(entries) == 0 {
		state, err := m.reconcileOfficialIdentity(ctx, false)
		return ImportBatchResult{Failures: []string{FailureNothingSelected}, FailureErrors: []error{codex.ErrEmptySelection}}, state, err
	}
	result, err := m.importEntries(ctx, entries, false)
	if err != nil {
		return result, AccountState{}, err
	}
	state, err := m.stateAfter(ctx, len(result.Succeeded) > 0)
	return result, state, err
}
