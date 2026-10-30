package l10n

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestKeyCount(t *testing.T) {
	// Pinned so that adding or dropping a key is a deliberate change.
	if got := len(allKeys); got != 637 {
		t.Errorf("len(allKeys) = %d, want 637", got)
	}
	seen := map[Key]bool{}
	for _, k := range allKeys {
		if seen[k] {
			t.Errorf("duplicate key %q", k)
		}
		seen[k] = true
	}
}

func TestEveryKeyHasEveryLanguage(t *testing.T) {
	for _, lang := range Languages() {
		table, ok := tables[lang]
		if !ok {
			t.Fatalf("no table for %s", lang)
		}
		if missing := MissingTranslations(lang); len(missing) > 0 {
			t.Errorf("%s: %d missing/blank translations, first %q", lang, len(missing), missing[0])
		}
		for _, k := range allKeys {
			if _, ok := table[k]; !ok {
				t.Errorf("%s: key %q has no entry", lang, k)
			}
		}
		if len(table) != len(allKeys) {
			t.Errorf("%s: %d entries, want %d", lang, len(table), len(allKeys))
		}
	}
	if len(tables) != len(Languages()) {
		t.Errorf("tables has %d languages, want %d", len(tables), len(Languages()))
	}
}

// placeholders maps argument index to a conversion class so that argument
// order may differ between languages (positional specifiers) but types may not.
func placeholders(format string) map[int]string {
	out := map[int]string{}
	next := 0
	for _, seg := range parseFormat(format) {
		sp := seg.spec
		// Only @ and integer conversions occur in real format strings. Other
		// letters can follow a literal "% " in prose (French "95 % ou").
		if sp == nil || strings.IndexByte("@dDiuU", sp.conv) < 0 {
			continue
		}
		idx := next
		if sp.pos > 0 {
			idx = sp.pos - 1
		} else {
			next++
		}
		class := string(sp.conv)
		switch sp.conv {
		case '@', 's', 'S':
			class = "object"
		case 'd', 'i', 'D', 'u', 'U':
			class = "integer"
		}
		out[idx] = class
	}
	return out
}

var namedPlaceholder = regexp.MustCompile(`\{[A-Za-z]+\}`)

func named(s string) []string {
	m := namedPlaceholder.FindAllString(s, -1)
	sort.Strings(m)
	return m
}

func TestPlaceholdersConsistentAcrossLanguages(t *testing.T) {
	formatted := 0
	for _, k := range allKeys {
		en := tables[English][k]
		wantArgs, wantNamed := placeholders(en), named(en)
		if len(wantArgs) > 0 {
			formatted++
		}
		for _, lang := range Languages() {
			s := tables[lang][k]
			if got := placeholders(s); !reflect.DeepEqual(got, wantArgs) {
				t.Errorf("%s/%s: format arguments %v, English has %v\n  %q", lang, k, got, wantArgs, s)
			}
			if got := named(s); !reflect.DeepEqual(got, wantNamed) {
				t.Errorf("%s/%s: named placeholders %v, English has %v", lang, k, got, wantNamed)
			}
		}
	}
	if formatted != 40 {
		t.Errorf("formatted keys = %d, want 40 (update if Swift strings change)", formatted)
	}
}

func TestTextFallbackMatchesSwift(t *testing.T) {
	// Present entries win, including in other languages.
	if got := New(SimplifiedChinese).Text(KeyAuto); got == "Auto" || got == "" {
		t.Errorf("zh-Hans auto = %q", got)
	}
	// Unknown language and unknown key fall back like Swift: English, then
	// the raw key.
	if got := New("klingon").Text(KeyAuto); got != "Auto" {
		t.Errorf("unknown language = %q, want English text", got)
	}
	if got := New(French).Text("noSuchKey"); got != "noSuchKey" {
		t.Errorf("unknown key = %q, want the key itself", got)
	}
}

func TestTextfFormatsTranslation(t *testing.T) {
	l := New(English)
	if got := l.Textf(KeyAccountsImportSucceeded, 3); got != "Imported 3 account(s)" {
		t.Errorf("got %q", got)
	}
	if got := l.Textf(KeyAccountsImportProgress, 2, 5); got != "Importing 2 / 5" {
		t.Errorf("got %q", got)
	}
	if got := l.Textf(KeyRateLimitResetTodayPercentPrefix, "75"); got != "≥75%" {
		t.Errorf("got %q", got)
	}
}

func TestResetTypeName(t *testing.T) {
	l := New(English)
	cases := map[string]Key{
		"global":            KeyRateLimitResetTypeGlobal,
		"banked":            KeyRateLimitResetTypeBanked,
		"global_and_banked": KeyRateLimitResetTypeGlobalAndBanked,
	}
	for raw, key := range cases {
		if got := l.ResetTypeName(raw); got != l.Text(key) || got == "" {
			t.Errorf("ResetTypeName(%q) = %q", raw, got)
		}
	}
	if got := l.ResetTypeName("other"); got != "other" {
		t.Errorf("unknown reset type = %q", got)
	}
}

func TestSampleTranslationsAreByteExact(t *testing.T) {
	cases := []struct {
		lang Language
		key  Key
		want string
	}{
		{English, KeyNetworkProxy, "Network"},
		{TraditionalChinese, KeyNetworkProxy, "網路"},
		{French, KeyNetworkProxyUsername, "Nom d’utilisateur"},
		{English, KeyNetworkProxyTesting, "Testing…"},
		{SimplifiedChinese, KeyQuotaAlertsDescription, "Codex 已用配额达到设定阈值时提醒"},
		{Russian, KeyAuto, tables[Russian][KeyAuto]},
	}
	for _, c := range cases {
		if got, _ := Lookup(c.lang, c.key); got != c.want || strings.TrimSpace(got) == "" {
			t.Errorf("%s/%s = %q, want %q", c.lang, c.key, got, c.want)
		}
	}
}
