package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/licoy/quosparr/internal/platform"
)

// Transfer pack identity. Packs carry the same two constants on every
// platform, so a pack written by one build is readable by any other.
const (
	TransferFormat        = "quosparr-accounts"
	LegacyTransferFormat  = "codex-runway-accounts"
	TransferVersion       = 1
	transferFileMode      = 0o600
	transferInvalidJSON   = "invalid JSON"
	transferMissingFormat = "missing"
)

// Provider names the service a pack's credentials belong to.
type Provider string

// Providers a pack can carry. Only Codex packs are imported by this package.
const (
	ProviderCodex  Provider = "codex"
	ProviderGrok   Provider = "grok"
	ProviderClaude Provider = "claude"
)

func (p Provider) valid() bool {
	return p == ProviderCodex || p == ProviderGrok || p == ProviderClaude
}

// Transfer errors. None of them carries credential text.
var (
	// ErrEmptySelection: an export was requested with no account ids.
	ErrEmptySelection = errors.New("codex: no accounts selected for export")
	// ErrNoAccountsExported: every selected account failed to load.
	ErrNoAccountsExported = errors.New("codex: none of the selected accounts could be exported")
)

// UnsupportedFormatError: the file is not a pack of this format.
type UnsupportedFormatError struct{ Detail string }

func (e *UnsupportedFormatError) Error() string {
	return "codex: unsupported account pack format: " + e.Detail
}

// UnsupportedVersionError: the pack version is not one this build reads.
type UnsupportedVersionError struct{ Version int }

func (e *UnsupportedVersionError) Error() string {
	return fmt.Sprintf("codex: unsupported account pack version %d", e.Version)
}

// ProviderMismatchError: a pack for another provider was given to the Codex
// importer.
type ProviderMismatchError struct{ Expected, Actual Provider }

func (e *ProviderMismatchError) Error() string {
	return fmt.Sprintf("codex: account pack is for %q, expected %q", e.Actual, e.Expected)
}

// TransferEntry is one account of a pack. Empty strings are absent values.
type TransferEntry struct {
	ID          string
	Alias       string
	DisplayName string
	Email       string
	// Credential is the provider's auth.json as raw JSON.
	Credential []byte
}

// TransferPack is the portable multi-account export for moving accounts to
// another machine. It holds secrets and must be treated like auth.json.
type TransferPack struct {
	Format     string
	Version    int
	Provider   Provider
	ExportedAt time.Time
	AppVersion string
	Accounts   []TransferEntry
}

// NewTransferPack returns a current-version pack.
func NewTransferPack(p Provider, exportedAt time.Time, appVersion string, accounts []TransferEntry) TransferPack {
	return TransferPack{
		Format: TransferFormat, Version: TransferVersion, Provider: p,
		ExportedAt: exportedAt, AppVersion: appVersion, Accounts: accounts,
	}
}

// EncodeTransferPack renders pack as pretty JSON with sorted keys and
// unescaped slashes, the layout the macOS app writes.
func EncodeTransferPack(pack TransferPack) ([]byte, error) {
	if pack.Format != TransferFormat {
		return nil, &UnsupportedFormatError{Detail: pack.Format}
	}
	if pack.Version != TransferVersion {
		return nil, &UnsupportedVersionError{Version: pack.Version}
	}
	root := object{
		"format":     mustValue(pack.Format),
		"version":    mustValue(pack.Version),
		"provider":   mustValue(string(pack.Provider)),
		"exportedAt": mustValue(FormatTime(pack.ExportedAt)),
	}
	if pack.AppVersion != "" {
		root["appVersion"] = mustValue(pack.AppVersion)
	}
	accounts := make([]object, 0, len(pack.Accounts))
	for _, entry := range pack.Accounts {
		o, err := entry.object()
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, o)
	}
	root["accounts"] = mustValue(accounts)
	raw, err := encodeValue(root)
	if err != nil {
		return nil, err
	}
	return formatJSON(raw, formatOptions{pretty: true})
}

func (e TransferEntry) object() (object, error) {
	if !isJSONContainer(e.Credential) {
		return nil, ErrInvalidCredential
	}
	o := object{}
	for key, value := range map[string]string{
		"id": e.ID, "alias": e.Alias, "displayName": e.DisplayName, "email": e.Email,
	} {
		if value != "" {
			o[key] = mustValue(value)
		}
	}
	o["credential"] = json.RawMessage(e.Credential)
	return o, nil
}

// isJSONContainer reports whether data is valid JSON whose top level is an
// object or array.
func isJSONContainer(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return false
	}
	return json.Valid(trimmed)
}

// DecodeTransferPack parses a pack. Credentials come back as compact JSON with
// sorted keys.
func DecodeTransferPack(data []byte) (TransferPack, error) {
	root, err := transferRoot(data)
	if err != nil {
		return TransferPack{}, err
	}
	format := stringField(root, "format")
	if format != TransferFormat && format != LegacyTransferFormat {
		if format == "" {
			format = transferMissingFormat
		}
		return TransferPack{}, &UnsupportedFormatError{Detail: format}
	}
	version := -1
	if raw, ok := root["version"]; ok {
		if n, err := parseIntLiteral(string(raw)); err == nil {
			version = n
		}
	}
	if version != TransferVersion {
		return TransferPack{}, &UnsupportedVersionError{Version: version}
	}
	provider := Provider(stringField(root, "provider"))
	if !provider.valid() {
		return TransferPack{}, &UnsupportedFormatError{Detail: "missing provider"}
	}
	exportedAt := time.Now()
	if t, ok := ParseTime(stringField(root, "exportedAt")); ok {
		exportedAt = t
	}
	entries, err := decodeTransferEntries(root["accounts"])
	if err != nil {
		return TransferPack{}, err
	}
	return TransferPack{
		Format: TransferFormat, Version: version, Provider: provider, ExportedAt: exportedAt,
		AppVersion: stringField(root, "appVersion"), Accounts: entries,
	}, nil
}

func transferRoot(data []byte) (object, error) {
	if !json.Valid(data) {
		return nil, errors.New(transferInvalidJSON)
	}
	root, err := parseObject(data)
	if err != nil {
		return nil, ErrInvalidCredential
	}
	return root, nil
}

func decodeTransferEntries(raw json.RawMessage) ([]TransferEntry, error) {
	var items []json.RawMessage
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) || json.Unmarshal(raw, &items) != nil {
		return nil, ErrInvalidCredential
	}
	entries := make([]TransferEntry, 0, len(items))
	for _, item := range items {
		o, err := parseObject(item)
		if err != nil {
			return nil, ErrInvalidCredential
		}
		credential, ok := o["credential"]
		if !ok || !isJSONContainer(credential) {
			return nil, ErrInvalidCredential
		}
		canonical, err := formatJSON(credential, formatOptions{})
		if err != nil {
			return nil, ErrInvalidCredential
		}
		entries = append(entries, TransferEntry{
			ID:          trimmedField(o, "id"),
			Alias:       trimmedField(o, "alias"),
			DisplayName: trimmedField(o, "displayName"),
			Email:       trimmedField(o, "email"),
			Credential:  canonical,
		})
	}
	return entries, nil
}

// stringField returns a string value as is; a missing or non-string value is "".
func stringField(o object, key string) string {
	var s string
	if raw, ok := o[key]; ok && json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func trimmedField(o object, key string) string {
	return strings.TrimFunc(stringField(o, key), unicode.IsSpace)
}

// DecodeTransferPackIfPack returns the decoded pack when data carries the
// pack format marker and decodes; otherwise ok is false and the caller should
// treat data as raw credentials.
func DecodeTransferPackIfPack(data []byte) (pack TransferPack, ok bool) {
	root, err := transferRoot(data)
	if err != nil || (stringField(root, "format") != TransferFormat && stringField(root, "format") != LegacyTransferFormat) {
		return TransferPack{}, false
	}
	pack, err = DecodeTransferPack(data)
	return pack, err == nil
}

// WriteTransferPack encodes pack and writes it to path atomically with owner
// only permissions. On failure no partial export is left behind.
func WriteTransferPack(path string, pack TransferPack) error {
	data, err := EncodeTransferPack(pack)
	if err != nil {
		return err
	}
	if err := platform.WriteFileAtomic(path, data, transferFileMode); err != nil {
		return err
	}
	if err := enforceOwnerOnly(path); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func enforceOwnerOnly(path string) error {
	if err := platform.ProtectOwnerOnly(path, transferFileMode); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return platform.VerifyOwnerOnly(path, transferFileMode)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != transferFileMode {
		return errors.New("codex: unable to enforce owner-only permissions on the account export")
	}
	return nil
}

// ExportResult is a built pack plus what could not be included.
type ExportResult struct {
	Pack          TransferPack
	ExportedCount int
	Failures      []string
	FailureErrors []error `json:"-"`
}

// Exporter builds portable packs from the managed account library.
type Exporter struct {
	Store      *Store
	AppVersion string
}

// Export gathers the credentials of ids into a Codex pack. Accounts that are
// unknown or lack a credential file are reported in Failures and skipped.
func (e Exporter) Export(ids []string, now time.Time) (ExportResult, error) {
	if len(ids) == 0 {
		return ExportResult{}, ErrEmptySelection
	}
	idx, err := e.Store.LoadIndex()
	if err != nil {
		return ExportResult{}, err
	}
	var entries []TransferEntry
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
		entries = append(entries, TransferEntry{
			ID: meta.ID, Alias: meta.Alias, DisplayName: meta.DisplayName,
			Email: meta.Email, Credential: credential,
		})
	}
	if len(entries) == 0 {
		return ExportResult{Failures: failures, FailureErrors: failureErrors}, ErrNoAccountsExported
	}
	pack := NewTransferPack(ProviderCodex, now, e.AppVersion, entries)
	return ExportResult{Pack: pack, ExportedCount: len(entries), Failures: failures, FailureErrors: failureErrors}, nil
}

// Write exports ids and writes the pack to path.
func (e Exporter) Write(ids []string, path string, now time.Time) (ExportResult, error) {
	result, err := e.Export(ids, now)
	if err != nil {
		return result, err
	}
	if err := WriteTransferPack(path, result.Pack); err != nil {
		return result, err
	}
	return result, nil
}
