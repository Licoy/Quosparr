package l10n

import "strings"

// Key identifies one localized string. Values keep the raw keys of the
// Swift releases; the typed constants live in tables.go.
type Key string

// Localizer looks up strings for one resolved language, optionally through
// the overrides of a product variant.
type Localizer struct {
	lang    Language
	variant Variant
}

// Variant names a product whose copy replaces some keys (see
// variantTables). The zero Variant uses the base tables only.
type Variant string

// VariantGemini is the copy of the Gemini reset-status feed.
const VariantGemini Variant = "gemini"

// WithVariant returns l reading the overrides of v first.
func (l Localizer) WithVariant(v Variant) Localizer {
	l.variant = v
	return l
}

// New returns a Localizer for lang. Unknown languages behave like English
// tables with missing entries (see Text).
func New(lang Language) Localizer { return Localizer{lang: lang} }

// ForPreference resolves a stored preference ("system" or a Language value)
// against the OS preferred languages and returns its Localizer.
func ForPreference(preference string, preferredLanguages []string) Localizer {
	return New(Resolve(preference, preferredLanguages))
}

// Language returns the resolved language.
func (l Localizer) Language() Language { return l.lang }

// Text returns the translation of key. Like the Swift implementation, a key
// absent from the language's table falls back to the English table, and a
// key absent from both yields the key's raw value. A present-but-empty
// translation is returned as is. A variant override wins over both.
func (l Localizer) Text(key Key) string {
	if row, ok := variantTables[l.variant][key]; ok {
		return row[languageIndex(l.lang)]
	}
	if s, ok := tables[l.lang][key]; ok {
		return s
	}
	if s, ok := tables[English][key]; ok {
		return s
	}
	return string(key)
}

// Textf formats the translation of key with Swift String(format:) semantics.
func (l Localizer) Textf(key Key, args ...any) string {
	return Format(l.Text(key), args...)
}

// ResetTypeName localizes a rate-limit reset type by its raw value
// ("global", "banked" or "global_and_banked"). Unknown values are returned
// unchanged.
func (l Localizer) ResetTypeName(raw string) string {
	switch raw {
	case "global":
		return l.Text(KeyRateLimitResetTypeGlobal)
	case "banked":
		return l.Text(KeyRateLimitResetTypeBanked)
	case "global_and_banked":
		return l.Text(KeyRateLimitResetTypeGlobalAndBanked)
	}
	return raw
}

// Lookup returns the translation stored for lang and key without any
// fallback.
func Lookup(lang Language, key Key) (string, bool) {
	s, ok := tables[lang][key]
	return s, ok
}

// Keys returns every key in declaration order.
func Keys() []Key { return append([]Key(nil), allKeys...) }

// MissingTranslations lists keys that lang does not translate (absent or
// blank), matching the Swift L10n.missingTranslations.
func MissingTranslations(lang Language) []Key {
	var out []Key
	for _, k := range allKeys {
		if s, ok := tables[lang][k]; !ok || isBlank(s) {
			out = append(out, k)
		}
	}
	return out
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }
