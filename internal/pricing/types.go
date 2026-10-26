// Package pricing stores public model prices, independently of usage logs.
package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/licoy/quosparr/internal/usage/decimal"
	"sort"
)

const DefaultSourceURL = "https://models.dev/api.json?type=all"
const CacheFileName = "model-prices-v1.json"

// Rate preserves absent prices; a present zero is a valid free rate.
type Rate struct {
	Input      *decimal.Decimal `json:"input,omitempty"`
	Output     *decimal.Decimal `json:"output,omitempty"`
	CacheRead  *decimal.Decimal `json:"cache_read,omitempty"`
	CacheWrite *decimal.Decimal `json:"cache_write,omitempty"`
}

// Tier retains the source threshold, whose comparison operator v1 omits.
type Tier struct {
	Size int64  `json:"size"`
	Type string `json:"type,omitempty"`
	Rate Rate   `json:"rate"`
}
type Model struct {
	ProviderID   string `json:"providerID"`
	ProviderName string `json:"providerName,omitempty"`
	SourceURL    string `json:"sourceURL,omitempty"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Rate         Rate   `json:"rate"`
	Tiers        []Tier `json:"tiers,omitempty"`
	LegacyLong   *Rate  `json:"legacyLong,omitempty"`
}

// Snapshot is immutable; all exported accessors return independent copies.
type Snapshot struct {
	models           map[string]Model
	version          string
	providerVersions map[string]string
	revision         string
}

func (s Snapshot) Version() string { return s.version }

// Revision includes display metadata and can key a directory browser cache
func (s Snapshot) Revision() string     { return s.revision }
func key(provider, model string) string { return provider + "\x00" + model }
func (s Snapshot) Lookup(provider, model string) (Model, bool) {
	m, ok := s.models[key(provider, model)]
	return cloneModel(m), ok
}
func (s Snapshot) Models() []Model {
	out := make([]Model, 0, len(s.models))
	for _, m := range s.models {
		out = append(out, cloneModel(m))
	}
	sort.Slice(out, func(i, j int) bool { return key(out[i].ProviderID, out[i].ID) < key(out[j].ProviderID, out[j].ID) })
	return out
}
func copyDecimal(d *decimal.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v := *d
	return &v
}
func cloneRate(r Rate) Rate {
	return Rate{copyDecimal(r.Input), copyDecimal(r.Output), copyDecimal(r.CacheRead), copyDecimal(r.CacheWrite)}
}
func cloneModel(m Model) Model {
	m.Rate = cloneRate(m.Rate)
	if m.LegacyLong != nil {
		r := cloneRate(*m.LegacyLong)
		m.LegacyLong = &r
	}
	m.Tiers = append([]Tier(nil), m.Tiers...)
	for i := range m.Tiers {
		m.Tiers[i].Rate = cloneRate(m.Tiers[i].Rate)
	}
	return m
}

// NewSnapshot validates an explicit public catalog, primarily for bundled data and tests.
func NewSnapshot(models []Model) (Snapshot, error) {
	s := Snapshot{models: map[string]Model{}, providerVersions: map[string]string{}}
	for _, m := range models {
		if err := validateModel(m); err != nil {
			return Snapshot{}, err
		}
		k := key(m.ProviderID, m.ID)
		if _, exists := s.models[k]; exists {
			return Snapshot{}, ErrInvalidCatalog
		}
		s.models[k] = cloneModel(m)
	}

	all := s.Models()
	revision, err := priceHash(all)
	if err != nil {
		return Snapshot{}, err
	}
	s.revision = revision
	canonical := make([]Model, 0, len(all))
	byProvider := map[string][]Model{}
	for _, m := range all {
		m.Name = ""
		m.ProviderName = ""
		m.SourceURL = ""
		canonical = append(canonical, m)
		byProvider[m.ProviderID] = append(byProvider[m.ProviderID], m)
	}

	s.version, err = priceHash(canonical)
	if err != nil {
		return Snapshot{}, err
	}
	for provider, models := range byProvider {
		version, err := priceHash(models)
		if err != nil {
			return Snapshot{}, err
		}
		s.providerVersions[provider] = version
	}
	return s, nil
}
func priceHash(models []Model) (string, error) {
	data, err := json.Marshal(models)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "models-dev-" + hex.EncodeToString(sum[:]), nil
}

// ProviderVersion is an O(1) lookup of this provider's effective-price hash.
func (s Snapshot) ProviderVersion(provider string) string { return s.providerVersions[provider] }

// Rule describes a bundled scenario for catalog presentation; provider adapters execute rules.
type Rule struct {
	Kind         string
	Rate         Rate
	CacheWrite5m *decimal.Decimal
	CacheWrite1h *decimal.Decimal
	Threshold    int64
	Inclusive    bool
	Multiplier   *decimal.Decimal
}
