package store

import (
	"strings"
	"testing"
)

func TestModelPricingPreferences(t *testing.T) {
	for _, raw := range []string{`{}`, `{"modelPricing":null}`, `{"modelPricing":{}}`} {
		p, err := DecodePreferences([]byte(raw))
		if err != nil || p.ModelPricing.AutoSync != true || p.ModelPricing.SyncIntervalSeconds != 3600 {
			t.Fatalf("%s: %+v %v", raw, p.ModelPricing, err)
		}
	}
	for _, seconds := range []int{900, 3600, 604800} {
		p := DefaultPreferences()
		p.ModelPricing = ModelPricing{AutoSync: false, SyncIntervalSeconds: seconds}
		encoded, err := EncodePreferences(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodePreferences(encoded)
		if err != nil || got.ModelPricing.AutoSync || got.ModelPricing.SyncIntervalSeconds != seconds {
			t.Fatalf("roundtrip %s: %+v %v", encoded, got.ModelPricing, err)
		}
	}
	for _, raw := range []string{`{"modelPricing":{"syncIntervalSeconds":899}}`, `{"modelPricing":{"syncIntervalSeconds":604801}}`, `{"modelPricing":{"autoSync":1}}`, `{"modelPricing":[]}`} {
		if _, err := DecodePreferences([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	p := DefaultPreferences()
	raw, err := EncodePreferences(p)
	if err != nil || strings.Contains(string(raw), "modelPricing") {
		t.Fatalf("default encoded: %s %v", raw, err)
	}
	p, err = DecodePreferences([]byte(`{"modelPricing":{"autoSync":false,"future":{"a":1}}}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err = EncodePreferences(p)
	if err != nil || !strings.Contains(string(raw), `"future":{"a":1}`) {
		t.Fatalf("unknown field lost %s %v", raw, err)
	}
}
