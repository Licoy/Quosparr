package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestQuotaAlertThresholdsLegacyDefaults(t *testing.T) {
	for _, doc := range []string{`{}`, `{"quotaAlertThresholds":null}`} {
		p := mustDecode(t, doc)
		if !slices.Equal(p.QuotaAlertThresholds, []int{80, 95, 100}) {
			t.Fatalf("%s: thresholds = %v", doc, p.QuotaAlertThresholds)
		}
		encoded, err := EncodePreferences(p)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["quotaAlertThresholds"]; ok {
			t.Fatalf("default thresholds must be omitted: %s", encoded)
		}
	}
}

func TestQuotaAlertThresholdsRoundTrip(t *testing.T) {
	p := mustDecode(t, `{"quotaAlertThresholds":[100,0,75,75],"quotaAlertsEnabled":true,"language":"english","future":{"nested":[1,true]}}`)
	if !slices.Equal(p.QuotaAlertThresholds, []int{0, 75, 100}) {
		t.Fatalf("thresholds = %v", p.QuotaAlertThresholds)
	}
	encoded, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(encoded)); !reflect.DeepEqual(got, p) {
		t.Fatalf("round trip changed preferences: %+v", got)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	sameJSON(t, fields["future"], []byte(`{"nested":[1,true]}`))
	if p.Language != LanguageEnglish || !p.QuotaAlertsEnabled {
		t.Fatal("unrelated preferences changed")
	}
}

func TestQuotaAlertThresholdsRejectInvalidStoredValues(t *testing.T) {
	for _, value := range []string{`[]`, `[-1]`, `[101]`, `[80.5]`, `[80.000000000000001]`, `["80"]`, `[null]`, `[true]`, `80`, `{}`, `"80"`, `true`} {
		t.Run(value, func(t *testing.T) {
			_, err := DecodePreferences([]byte(`{"quotaAlertThresholds":` + value + `}`))
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("error = %v, want ErrCorrupt", err)
			}
		})
	}
}

func TestQuotaAlertThresholdsNormalizationAndIsolation(t *testing.T) {
	in := []int{100, 0, 75, 75}
	normalized, err := NormalizeQuotaAlertThresholds(in)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(normalized, []int{0, 75, 100}) || !slices.Equal(in, []int{100, 0, 75, 75}) {
		t.Fatalf("normalized = %v, input = %v", normalized, in)
	}
	normalized[0] = 10
	if in[0] != 100 {
		t.Fatal("normalization aliases input")
	}
	p := DefaultPreferences()
	p.QuotaAlertThresholds = in
	clone := p.Clone()
	clone.QuotaAlertThresholds[0] = 20
	if in[0] != 100 {
		t.Fatal("clone aliases original thresholds")
	}
	p.Normalize()
	if !slices.Equal(p.QuotaAlertThresholds, []int{0, 75, 100}) || in[0] != 100 {
		t.Fatalf("Preferences.Normalize: thresholds = %v, input = %v", p.QuotaAlertThresholds, in)
	}
	first, second := DefaultQuotaAlertThresholds(), DefaultQuotaAlertThresholds()
	first[0] = 1
	if second[0] != 80 {
		t.Fatal("default thresholds share storage")
	}
}

func TestQuotaAlertThresholdsValidateAndEncode(t *testing.T) {
	for _, thresholds := range [][]int{nil, {}, {-1}, {101}} {
		if _, err := NormalizeQuotaAlertThresholds(thresholds); err == nil {
			t.Fatalf("normalization accepted %v", thresholds)
		}
		p := DefaultPreferences()
		p.QuotaAlertThresholds = thresholds
		if p.Validate() == nil {
			t.Fatalf("validation accepted %v", thresholds)
		}
		p.Normalize()
		_, err := EncodePreferences(p)
		if thresholds == nil {
			if err != nil || !slices.Equal(p.QuotaAlertThresholds, DefaultQuotaAlertThresholds()) {
				t.Fatalf("legacy nil normalization: %v, %v", p.QuotaAlertThresholds, err)
			}
		} else if err == nil {
			t.Fatalf("encoding accepted %v", thresholds)
		}
	}
	p := DefaultPreferences()
	p.QuotaAlertThresholds = []int{100, 80, 95, 80}
	encoded, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.QuotaAlertThresholds, []int{100, 80, 95, 80}) {
		t.Fatal("encoding mutated caller")
	}
	if got := mustDecode(t, string(encoded)); !slices.Equal(got.QuotaAlertThresholds, DefaultQuotaAlertThresholds()) {
		t.Fatalf("default-equivalent round trip: %v", got.QuotaAlertThresholds)
	}
}
