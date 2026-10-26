package store

import (
	"encoding/json"
	"fmt"
	"maps"
)

const MinModelPricingInterval = 15 * 60
const MaxModelPricingInterval = 7 * 24 * 60 * 60

type ModelPricing struct {
	AutoSync            bool `json:"autoSync"`
	SyncIntervalSeconds int  `json:"syncIntervalSeconds"`
	extra               fields
}

func DefaultModelPricing() ModelPricing {
	return ModelPricing{AutoSync: true, SyncIntervalSeconds: 3600}
}
func (p ModelPricing) IsZero() bool {
	return p.AutoSync && p.SyncIntervalSeconds == 3600 && len(p.extra) == 0
}
func (p ModelPricing) Validate() error {
	if p.SyncIntervalSeconds < MinModelPricingInterval || p.SyncIntervalSeconds > MaxModelPricingInterval {
		return fmt.Errorf("modelPricing.syncIntervalSeconds must be between %d and %d", MinModelPricingInterval, MaxModelPricingInterval)
	}
	return nil
}
func (p ModelPricing) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	m := maps.Clone(p.extra)
	if m == nil {
		m = fields{}
	}
	m["autoSync"], _ = json.Marshal(p.AutoSync)
	m["syncIntervalSeconds"], _ = json.Marshal(p.SyncIntervalSeconds)
	return json.Marshal(m)
}
func readModelPricing(r *reader) ModelPricing {
	p := DefaultModelPricing()
	raw, ok := r.take("modelPricing")
	if !ok {
		return p
	}
	m, err := parseObject(raw)
	if err != nil {
		r.fail("modelPricing", "%v", err)
		return p
	}
	child := &reader{rest: m}
	p.AutoSync = child.bool("autoSync", p.AutoSync)
	p.SyncIntervalSeconds = child.int("syncIntervalSeconds", p.SyncIntervalSeconds)
	if child.err != nil {
		r.fail("modelPricing", "%v", child.err)
	} else if err := p.Validate(); err != nil {
		r.fail("modelPricing", "%v", err)
	}
	p.extra = child.rest
	return p
}
