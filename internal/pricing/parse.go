package pricing

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

var ErrInvalidCatalog = errors.New("pricing: invalid public price catalog")

type sourceRate struct {
	Input      *decimal.Decimal `json:"input"`
	Output     *decimal.Decimal `json:"output"`
	CacheRead  *decimal.Decimal `json:"cache_read"`
	CacheWrite *decimal.Decimal `json:"cache_write"`
}
type sourceCost struct {
	Input      *decimal.Decimal `json:"input"`
	Output     *decimal.Decimal `json:"output"`
	CacheRead  *decimal.Decimal `json:"cache_read"`
	CacheWrite *decimal.Decimal `json:"cache_write"`
	LegacyLong *sourceCost      `json:"context_over_200k"`
	Tiers      []struct {
		sourceRate
		Tier struct {
			Type string `json:"type"`
			Size int64  `json:"size"`
		} `json:"tier"`
	} `json:"tiers"`
}

func (c sourceCost) rate() Rate { return Rate{c.Input, c.Output, c.CacheRead, c.CacheWrite} }
func validateRate(r Rate) error {
	for _, d := range []*decimal.Decimal{r.Input, r.Output, r.CacheRead, r.CacheWrite} {
		if d != nil && (d.Sign() < 0 || d.Overflow() || d.Cmp(decimal.FromInt(10_000)) > 0) {
			return ErrInvalidCatalog
		}
	}
	return nil
}

// Parse imports only public identity and price fields; executable metadata is ignored.
func Parse(data []byte) (Snapshot, error) {
	var providers map[string]struct {
		Name   string `json:"name"`
		Models map[string]struct {
			ID   string      `json:"id"`
			Name string      `json:"name"`
			Cost *sourceCost `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &providers); err != nil {
		return Snapshot{}, fmt.Errorf("%w: malformed JSON", ErrInvalidCatalog)
	}
	var models []Model
	for provider, p := range providers {
		if provider == "" {
			return Snapshot{}, ErrInvalidCatalog
		}
		for id, raw := range p.Models {
			if id == "" || raw.ID != "" && raw.ID != id {
				return Snapshot{}, ErrInvalidCatalog
			}
			m := Model{ProviderID: provider, ID: id, Name: raw.Name, ProviderName: p.Name, SourceURL: DefaultSourceURL}
			if raw.Cost != nil {
				m.Rate = raw.Cost.rate()
				if err := validateRate(m.Rate); err != nil {
					return Snapshot{}, err
				}
				if raw.Cost.LegacyLong != nil {
					r := raw.Cost.LegacyLong.rate()
					if err := validateRate(r); err != nil {
						return Snapshot{}, err
					}
					m.LegacyLong = &r
				}
				for _, t := range raw.Cost.Tiers {
					if t.Tier.Size < 0 {
						return Snapshot{}, ErrInvalidCatalog
					}
					r := Rate{t.Input, t.Output, t.CacheRead, t.CacheWrite}
					if err := validateRate(r); err != nil {
						return Snapshot{}, err
					}
					m.Tiers = append(m.Tiers, Tier{Size: t.Tier.Size, Type: t.Tier.Type, Rate: r})
				}
			}
			models = append(models, m)
		}
	}
	if !usableModels(models) {
		return Snapshot{}, ErrInvalidCatalog
	}
	return NewSnapshot(models)
}

func ratePresent(r Rate) bool {
	return r.Input != nil || r.Output != nil || r.CacheRead != nil || r.CacheWrite != nil
}

func usableModels(models []Model) bool {
	for _, m := range models {
		if ratePresent(m.Rate) {
			return true
		}
		if m.LegacyLong != nil && ratePresent(*m.LegacyLong) {
			return true
		}
		for _, tier := range m.Tiers {
			if ratePresent(tier.Rate) {
				return true
			}
		}
	}
	return false
}
