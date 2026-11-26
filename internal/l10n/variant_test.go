package l10n

import (
	"reflect"
	"strings"
	"testing"
)

func TestVariantRowsAreComplete(t *testing.T) {
	for v, rows := range variantTables {
		for k, row := range rows {
			if _, ok := tables[English][k]; !ok {
				t.Errorf("%s/%s: not a base key", v, k)
			}
			want := placeholders(tables[English][k])
			for i, s := range row {
				if strings.TrimSpace(s) == "" {
					t.Errorf("%s/%s/%s: blank", v, k, Languages()[i])
				}
				if got := placeholders(s); !reflect.DeepEqual(got, want) {
					t.Errorf("%s/%s/%s: format arguments %v, base has %v", v, k, Languages()[i], got, want)
				}
			}
		}
	}
}

func TestGeminiVariantNamesGemini(t *testing.T) {
	for k, row := range variantTables[VariantGemini] {
		for i, s := range row {
			if strings.Contains(s, "Codex") {
				t.Errorf("%s/%s still names Codex: %q", k, Languages()[i], s)
			}
		}
	}
	zh := New(SimplifiedChinese)
	if got := zh.WithVariant(VariantGemini).Text(KeyRateLimitResetQuestionCurrent); got != "今天有新的 Gemini 重置吗？" {
		t.Errorf("gemini zh = %q", got)
	}
	if got := zh.Text(KeyRateLimitResetQuestionCurrent); got != "今天有新的 Codex 重置吗？" {
		t.Errorf("base zh = %q", got)
	}
	// Keys without an override read the base tables.
	if got, want := zh.WithVariant(VariantGemini).Text(KeyRateLimitResetTodayYes), zh.Text(KeyRateLimitResetTodayYes); got != want {
		t.Errorf("fallback = %q, want %q", got, want)
	}
	if got := New(Language("xx")).WithVariant(VariantGemini).Text(KeyRateLimitResetTodaySource); got != "Agy Reset" {
		t.Errorf("unknown language = %q", got)
	}
}
