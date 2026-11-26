package codex

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/BurntSushi/toml"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/tomlstmt"
)

// ManagedProviderID is the model_providers key (and model_provider value) that
// Quosparr owns in config.toml.
const ManagedProviderID = "quosparr"

const (
	defaultProviderName = "Quosparr"
	configBackupKeep    = 10
)

var (
	// ErrProviderConfigUnsupported means config.toml defines the managed
	// provider in a form that cannot be edited line by line, or uses syntax
	// the editor does not understand. The file is left untouched.
	ErrProviderConfigUnsupported = errors.New("codex: config.toml defines the quosparr provider in a form that cannot be edited safely; adjust it manually")
	// ErrProviderConfigInvalid means config.toml does not parse, or the edit
	// failed its safety check. Parser errors can echo values, so they are not
	// wrapped.
	ErrProviderConfigInvalid = errors.New("codex: config.toml cannot be parsed or edited safely")
	// ErrProviderBaseURLRequired is returned when no base URL is given.
	ErrProviderBaseURLRequired = errors.New("codex: provider base URL is required")
)

// ProviderConfig is what Quosparr writes for a third-party relay. The API key
// is not part of config.toml: Codex reads OPENAI_API_KEY from auth.json.
type ProviderConfig struct {
	Name    string
	BaseURL string
	Model   string
}

// ApplyManagedProvider sets model_provider (and model when cfg.Model is not
// empty) before the first table header and writes the
// [model_providers.quosparr] table. Existing top-level lines and the owned
// table are replaced in place; every other byte, including comments, order and
// the line-ending style, is kept.
func ApplyManagedProvider(data []byte, cfg ProviderConfig) ([]byte, error) {
	cfg.Name, cfg.BaseURL, cfg.Model = strings.TrimSpace(cfg.Name), strings.TrimSpace(cfg.BaseURL), strings.TrimSpace(cfg.Model)
	if cfg.Name == "" {
		cfg.Name = defaultProviderName
	}
	if cfg.BaseURL == "" {
		return nil, ErrProviderBaseURLRequired
	}
	if !utf8.ValidString(cfg.Name + cfg.BaseURL + cfg.Model) {
		return nil, ErrProviderConfigInvalid
	}

	if _, err := apiquota.NormalizeBaseURL(cfg.BaseURL); err != nil {
		return nil, ErrRelayBaseURL
	}
	before, err := decodeConfig(data)
	if err != nil {
		return nil, err
	}
	withKeys, err := setTopLevelKeys(data, cfg)
	if err != nil {
		return nil, err
	}
	out, err := setManagedTable(withKeys, cfg)
	if err != nil {
		return nil, err
	}
	after, err := decodeConfig(out)
	if err != nil || !appliedAsExpected(before, after, cfg) {
		return nil, ErrProviderConfigInvalid
	}
	return out, nil
}

// RemoveManagedProvider undoes ApplyManagedProvider: it drops model_provider
// when it is "quosparr", the [model_providers.quosparr] table, and the
// top-level model when it equals appliedModel (non-empty). When nothing
// matched it returns data unchanged and changed=false.
func RemoveManagedProvider(data []byte, appliedModel string) (out []byte, changed bool, err error) {
	return removeProvider(data, appliedModel, "")
}

// removeProvider is RemoveManagedProvider that also drops a model_provider
// selecting another provider whose base_url is relayBase (non-empty): the
// relay an imported account found already configured. That provider's table
// stays.
func removeProvider(data []byte, appliedModel, relayBase string) (out []byte, changed bool, err error) {
	before, err := decodeConfig(data)
	if err != nil {
		return nil, false, err
	}
	items, layout, err := lexProviderLayout(data)
	if err != nil {
		return nil, false, err
	}
	dropProvider := layout.provider >= 0 && (before["model_provider"] == ManagedProviderID || selectsRelay(before, relayBase))
	removeModel := appliedModel != "" && layout.model >= 0 && before["model"] == appliedModel
	var edits []textEdit
	if dropProvider {
		edits = append(edits, deleteItems(items, layout.provider, layout.provider+1))
	}
	if removeModel {
		edits = append(edits, deleteItems(items, layout.model, layout.model+1))
	}
	if layout.tableStart >= 0 {
		edits = append(edits, deleteTable(items, layout))
	}
	if len(edits) == 0 {
		return data, false, nil
	}
	if layout.touchesMultilineString(items, dropProvider, removeModel) {
		return nil, false, ErrProviderConfigUnsupported
	}
	out = applyTextEdits(data, edits)
	after, err := decodeConfig(out)
	if err != nil || !removedAsExpected(before, after, dropProvider, removeModel) {
		return nil, false, ErrProviderConfigInvalid
	}
	return out, true, nil
}

// ReadManagedProvider reports whether model_provider is "quosparr" and the
// base_url of the [model_providers.quosparr] table (empty when absent).
func ReadManagedProvider(data []byte) (active bool, baseURL string, err error) {
	config, err := decodeConfig(data)
	if err != nil {
		return false, "", err
	}
	active = config["model_provider"] == ManagedProviderID
	if table := managedTable(config); table != nil {
		baseURL, _ = table["base_url"].(string)
	}
	return active, baseURL, nil
}

// selectedProviderBaseURL is the base_url of the provider model_provider
// selects, "" for OpenAI itself or a provider without one.
func selectedProviderBaseURL(config map[string]any) string {
	id, _ := config["model_provider"].(string)
	if id == "" || id == "openai" {
		return ""
	}
	providers, _ := config["model_providers"].(map[string]any)
	table, _ := providers[id].(map[string]any)
	base, _ := table["base_url"].(string)
	return strings.TrimSpace(base)
}

// selectsRelay reports whether model_provider selects a provider other than
// the managed one whose base URL is the same site as relayBase.
func selectsRelay(config map[string]any, relayBase string) bool {
	if relayBase == "" || config["model_provider"] == ManagedProviderID {
		return false
	}
	p, err := parseRelayProfile(RelayProfile{BaseURL: selectedProviderBaseURL(config)}, apiquota.NormalizeStoredBaseURL)
	return err == nil && p.BaseURL != "" && p.BaseURL == relayBase
}

func decodeConfig(data []byte) (map[string]any, error) {
	config := map[string]any{}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, ErrProviderConfigInvalid
	}
	return config, nil
}

func managedTable(config map[string]any) map[string]any {
	providers, _ := config["model_providers"].(map[string]any)
	table, _ := providers[ManagedProviderID].(map[string]any)
	return table
}

// withoutManaged copies config minus the keys this editor owns.
func withoutManaged(config map[string]any) map[string]any {
	out := make(map[string]any, len(config))
	for k, v := range config {
		out[k] = v
	}
	delete(out, "model_provider")
	delete(out, "model")
	if providers, ok := config["model_providers"].(map[string]any); ok {
		rest := make(map[string]any, len(providers))
		for k, v := range providers {
			if k != ManagedProviderID {
				rest[k] = v
			}
		}
		if len(rest) == 0 {
			delete(out, "model_providers")
		} else {
			out["model_providers"] = rest
		}
	}
	return out
}

func appliedAsExpected(before, after map[string]any, cfg ProviderConfig) bool {
	table := managedTable(after)
	if after["model_provider"] != ManagedProviderID || table == nil ||
		table["name"] != cfg.Name || table["base_url"] != cfg.BaseURL ||
		table["wire_api"] != "responses" || table["requires_openai_auth"] != true {
		return false
	}
	wantModel, hasModel := before["model"]
	if cfg.Model != "" {
		wantModel, hasModel = cfg.Model, true
	}
	gotModel, gotHas := after["model"]
	if gotHas != hasModel || (hasModel && !reflect.DeepEqual(gotModel, wantModel)) {
		return false
	}
	return reflect.DeepEqual(withoutManaged(before), withoutManaged(after))
}

func removedAsExpected(before, after map[string]any, removedProvider, removedModel bool) bool {
	if managedTable(after) != nil {
		return false
	}
	if !keyOutcomeOK(before, after, "model_provider", removedProvider) ||
		!keyOutcomeOK(before, after, "model", removedModel) {
		return false
	}
	return reflect.DeepEqual(withoutManaged(before), withoutManaged(after))
}

// keyOutcomeOK checks a top-level key is gone when removed, otherwise unchanged.
func keyOutcomeOK(before, after map[string]any, key string, removed bool) bool {
	got, has := after[key]
	if removed {
		return !has
	}
	want, hadBefore := before[key]
	return has == hadBefore && reflect.DeepEqual(got, want)
}

// --- text editing -----------------------------------------------------------

type textEdit struct {
	start, end int
	text       string
}

func applyTextEdits(data []byte, edits []textEdit) []byte {
	shared := make([]tomlstmt.Edit, len(edits))
	for i, edit := range edits {
		shared[i] = tomlstmt.Edit{Start: edit.start, End: edit.end, Text: edit.text}
	}
	return tomlstmt.ApplyEdits(data, shared)
}

func detectEOL(data []byte) string { return tomlstmt.DetectEOL(data) }

func hasNewline(data []byte, it tomlItem) bool {
	return it.end > it.start && data[it.end-1] == '\n'
}

func tomlQuote(s string) string { return tomlstmt.Quote(s) }

func setTopLevelKeys(data []byte, cfg ProviderConfig) ([]byte, error) {
	items, layout, err := lexProviderLayout(data)
	if err != nil {
		return nil, err
	}
	eol := detectEOL(data)
	providerLine := "model_provider = " + tomlQuote(ManagedProviderID)
	modelLine := "model = " + tomlQuote(cfg.Model)
	needModelLine := cfg.Model != "" && layout.model < 0
	if layout.model >= 0 && items[layout.model].mlString || layout.provider >= 0 && items[layout.provider].mlString {
		return nil, ErrProviderConfigUnsupported
	}
	var edits []textEdit
	if layout.provider >= 0 {
		text := providerLine
		if needModelLine {
			text += eol + modelLine
		}
		edits = append(edits, replaceItem(data, items[layout.provider], text, eol))
	} else {
		block := []string{providerLine}
		if needModelLine {
			block = append(block, modelLine)
		}
		edits = append(edits, insertBlock(data, items, layout, block, eol))
	}
	if cfg.Model != "" && layout.model >= 0 {
		edits = append(edits, replaceItem(data, items[layout.model], modelLine, eol))
	}
	return applyTextEdits(data, edits), nil
}

func replaceItem(data []byte, it tomlItem, text, eol string) textEdit {
	if hasNewline(data, it) {
		text += eol
	}
	return textEdit{start: it.start, end: it.end, text: text}
}

// insertBlock puts new top-level lines after the last existing top-level
// key/value, or at the start of the file when there is none, so a comment
// above the first table stays attached to it.
func insertBlock(data []byte, items []tomlItem, layout providerLayout, lines []string, eol string) textEdit {
	text := strings.Join(lines, eol) + eol
	if layout.lastTopKV < 0 {
		return textEdit{text: text}
	}
	last := items[layout.lastTopKV]
	if !hasNewline(data, last) {
		text = eol + text
	}
	return textEdit{start: last.end, end: last.end, text: text}
}

func managedTableText(cfg ProviderConfig, eol string) string {
	lines := []string{
		"[model_providers." + ManagedProviderID + "]",
		"name = " + tomlQuote(cfg.Name),
		"base_url = " + tomlQuote(cfg.BaseURL),
		`wire_api = "responses"`,
		"requires_openai_auth = true",
	}
	return strings.Join(lines, eol) + eol
}

func setManagedTable(data []byte, cfg ProviderConfig) ([]byte, error) {
	items, layout, err := lexProviderLayout(data)
	if err != nil {
		return nil, err
	}
	eol := detectEOL(data)
	text := managedTableText(cfg, eol)
	if layout.tableStart >= 0 {
		if layout.touchesMultilineString(items, false, false) {
			return nil, ErrProviderConfigUnsupported
		}
		edit := textEdit{start: items[layout.tableStart].start, end: items[layout.tableEnd-1].end, text: text}
		return applyTextEdits(data, []textEdit{edit}), nil
	}
	out := append([]byte(nil), data...)
	if len(out) > 0 {
		if out[len(out)-1] != '\n' {
			out = append(out, eol...)
		}
		if !endsWithBlankLine(out) {
			out = append(out, eol...)
		}
	}
	return append(out, text...), nil
}

func endsWithBlankLine(data []byte) bool { return tomlstmt.EndsWithBlankLine(data) }

func deleteItems(items []tomlItem, from, to int) textEdit {
	return textEdit{start: items[from].start, end: items[to-1].end}
}

// deleteTable removes the owned table. The single blank line that
// ApplyManagedProvider adds before an appended table goes with it.
func deleteTable(items []tomlItem, layout providerLayout) textEdit {
	edit := deleteItems(items, layout.tableStart, layout.tableEnd)
	if layout.tableStart > 0 && items[layout.tableStart-1].kind == tomlBlank &&
		(layout.tableEnd == len(items) || items[layout.tableEnd].kind == tomlBlank) {
		edit.start = items[layout.tableStart-1].start
	}
	return edit
}

// --- structure --------------------------------------------------------------

type providerLayout struct {
	provider, model int // item index of the top-level key, -1 when absent
	lastTopKV       int // last top-level key/value item, -1 when none
	tableStart      int // owned table header item, -1 when absent
	tableEnd        int // one past the last item that belongs to the table
}

func lexProviderLayout(data []byte) ([]tomlItem, providerLayout, error) {
	items, err := lexTOML(data)
	if err != nil {
		return nil, providerLayout{}, ErrProviderConfigUnsupported
	}
	layout, err := analyzeProvider(items)
	return items, layout, err
}

func underManaged(path []string) bool {
	return len(path) >= 2 && path[0] == "model_providers" && path[1] == ManagedProviderID
}

func isManagedTablePath(path []string) bool {
	return len(path) == 2 && underManaged(path)
}

func analyzeProvider(items []tomlItem) (providerLayout, error) {
	l := providerLayout{provider: -1, model: -1, lastTopKV: -1, tableStart: -1}
	var table []string
	inTable := false
	for i, it := range items {
		switch it.kind {
		case tomlHeader:
			inTable, table = true, it.path
			if !underManaged(it.path) {
				continue
			}
			if it.array || !isManagedTablePath(it.path) || l.tableStart >= 0 {
				return l, ErrProviderConfigUnsupported
			}
			l.tableStart = i
		case tomlKeyVal:
			full := append(append([]string(nil), table...), it.path...)
			if (underManaged(full) && !isManagedTablePath(table)) || (len(full) == 1 && full[0] == "model_providers") {
				return l, ErrProviderConfigUnsupported
			}
			if inTable {
				continue
			}
			l.lastTopKV = i
			switch {
			case len(it.path) == 1 && it.path[0] == "model_provider":
				l.provider = i
			case len(it.path) == 1 && it.path[0] == "model":
				l.model = i
			}
		}
	}
	l.tableEnd = tableEnd(items, l.tableStart)
	return l, nil
}

// tableEnd is the end of the owned table's statements: up to the next header,
// minus trailing blank and comment lines that belong to what follows.
func tableEnd(items []tomlItem, start int) int {
	if start < 0 {
		return -1
	}
	end := start + 1
	for end < len(items) && items[end].kind != tomlHeader {
		end++
	}
	for end > start+1 && (items[end-1].kind == tomlBlank || items[end-1].kind == tomlComment) {
		end--
	}
	return end
}

func (l providerLayout) touchesMultilineString(items []tomlItem, provider, model bool) bool {
	if provider && items[l.provider].mlString || model && items[l.model].mlString {
		return true
	}
	for i := l.tableStart; l.tableStart >= 0 && i < l.tableEnd; i++ {
		if items[i].mlString {
			return true
		}
	}
	return false
}

// --- files ------------------------------------------------------------------

// ConfigPath is config.toml next to the official auth.json.
func (s *Store) ConfigPath() string {
	return filepath.Join(filepath.Dir(s.officialAuth), "config.toml")
}

func (s *Store) configBackup() platform.BackupPolicy {
	return platform.BackupPolicy{
		Dir:    filepath.Join(s.runwayHome, "backups"),
		Prefix: "codex-config-",
		Ext:    ".toml",
		Keep:   configBackupKeep,
	}
}

// ApplyManagedProviderFile applies cfg to config.toml. The current file is
// backed up first; nothing is written when the content would not change.
func (s *Store) ApplyManagedProviderFile(cfg ProviderConfig, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.EditFileWithBackup(s.ConfigPath(), s.configBackup(), now, func(old []byte) ([]byte, error) {
		return ApplyManagedProvider(old, cfg)
	})
}

// RemoveManagedProviderFile removes what ApplyManagedProviderFile wrote.
func (s *Store) RemoveManagedProviderFile(appliedModel string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return platform.EditFileWithBackup(s.ConfigPath(), s.configBackup(), now, func(old []byte) ([]byte, error) {
		out, _, err := RemoveManagedProvider(old, appliedModel)
		return out, err
	})
}

// ReadManagedProviderFile reads config.toml; a missing file is inactive.
func (s *Store) ReadManagedProviderFile() (active bool, baseURL string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := readFileIfExists(s.ConfigPath())
	if err != nil {
		return false, "", err
	}
	return ReadManagedProvider(data)
}

func readFileIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}
