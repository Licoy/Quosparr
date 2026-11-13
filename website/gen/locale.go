package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// Locale is one published translation. English is the root page and the
// x-default alternate.
type Locale struct {
	Code    string // URL segment and locales/<code>.json
	Lang    string // BCP 47 tag for lang and hreflang
	OG      string // og:locale
	Name    string // native name in the language menu
	Readme  string
	Upgrade string
	T       *Strings
}

func (l Locale) Path() string {
	if l.Code == "en" {
		return "/"
	}
	return "/" + l.Code + "/"
}

var localeTable = []Locale{
	{Code: "en", Lang: "en", OG: "en_US", Name: "English", Readme: "README.md", Upgrade: "docs/upgrade-from-codexrunway.md"},
	{Code: "zh-hans", Lang: "zh-Hans", OG: "zh_CN", Name: "简体中文", Readme: "README_ZH.md", Upgrade: "docs/upgrade-from-codexrunway_ZH.md"},
	{Code: "zh-hant", Lang: "zh-Hant", OG: "zh_TW", Name: "繁體中文", Readme: "README_ZH_HANT.md", Upgrade: "docs/upgrade-from-codexrunway_ZH_HANT.md"},
	{Code: "ko", Lang: "ko", OG: "ko_KR", Name: "한국어", Readme: "README_KO.md", Upgrade: "docs/upgrade-from-codexrunway_KO.md"},
	{Code: "ja", Lang: "ja", OG: "ja_JP", Name: "日本語", Readme: "README_JA.md", Upgrade: "docs/upgrade-from-codexrunway_JA.md"},
	{Code: "ru", Lang: "ru", OG: "ru_RU", Name: "Русский", Readme: "README_RU.md", Upgrade: "docs/upgrade-from-codexrunway_RU.md"},
	{Code: "fr", Lang: "fr", OG: "fr_FR", Name: "Français", Readme: "README_FR.md", Upgrade: "docs/upgrade-from-codexrunway_FR.md"},
}

type Item struct {
	Title string
	Text  string
}

// Strings mirrors locales/*.json. Every field is required in every language,
// and lists must have the same length as English.
type Strings struct {
	Meta struct {
		Title       string
		Description string
	}
	Nav struct {
		Features    string
		Screenshots string
		Privacy     string
		Download    string
	}
	UI struct {
		Skip        string
		Language    string
		ThemeSystem string
		ThemeLight  string
		ThemeDark   string
	}
	Hero struct {
		Etymology   string // HTML
		Tagline     string
		Description string
		Download    string
		DownloadFor string // {os} is replaced in the browser
		Source      string
		License     string
	}
	Features struct {
		Title       string
		Lead        string
		Codex       []string
		Claude      []string
		Gemini      []string
		Grok        []string
		CommonTitle string
		Common      []Item
	}
	Shots struct {
		Title    string
		Note     string
		Main     string
		Accounts string
		MenuBar  string
		Floating string
		Pricing  string
		Sessions string
		Skills   string
		MCP      string
	}
	Privacy struct {
		Title string
		Lead  string
		Items []Item
	}
	Download struct {
		Title       string
		Lead        string
		Latest      string
		Released    string
		Recommended string
		MacReq      string
		WinReq      string
		WinPortable string
		WinSetup    string
		WinNote     string
		PortNote    string // HTML
		LinuxReq    string
		LinuxNote   string // HTML
		AllReleases string
		Checksums   string
		Upgrade     string
	}
	// Panel labels mirror the app's own translations in internal/l10n and
	// internal/app/extra_l10n.go; {n} and {date} are filled in by buildPanel.
	Panel struct {
		Caption       string
		Providers     string
		Privacy       string
		Updates       string
		Refresh       string
		Quota         string
		FiveHour      string
		Weekly        string
		Left          string
		Used          string
		BurnRate      string
		ExhaustsIn    string
		Projected     string
		NextReset     string
		ResetSwitch   string
		ValidUntil    string
		TokenUsage    string
		Daily         string
		WeeklyChart   string
		Total         string
		Official      string
		Speed         string
		TTFT          string
		Basis         string
		Details       string
		APICost       string
		APIEquivalent string
		ExtraUsage    string
		GrokIncluded  string
		GrokWeekly    string
		Settings      string
		Quit          string
	}
	Footer struct {
		Source     string
		Releases   string
		Issues     string
		License    string
		Disclaimer string
	}
}

func loadLocales(dir string) ([]Locale, error) {
	locales := make([]Locale, len(localeTable))
	copy(locales, localeTable)
	for i := range locales {
		t, err := loadStrings(filepath.Join(dir, locales[i].Code+".json"))
		if err != nil {
			return nil, err
		}
		if err := checkComplete(reflect.ValueOf(*t), locales[i].Code); err != nil {
			return nil, err
		}
		if i > 0 {
			if err := checkShape(reflect.ValueOf(*locales[0].T), reflect.ValueOf(*t), locales[i].Code); err != nil {
				return nil, err
			}
		}
		locales[i].T = t
	}
	return locales, nil
}

func loadStrings(path string) (*Strings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var t Strings
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &t, nil
}

// checkComplete rejects empty strings and empty lists anywhere in v.
func checkComplete(v reflect.Value, path string) error {
	switch v.Kind() {
	case reflect.String:
		if strings.TrimSpace(v.String()) == "" {
			return fmt.Errorf("%s: missing translation", path)
		}
	case reflect.Slice:
		if v.Len() == 0 {
			return fmt.Errorf("%s: empty list", path)
		}
		for i := 0; i < v.Len(); i++ {
			if err := checkComplete(v.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := checkComplete(v.Field(i), path+"."+v.Type().Field(i).Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkShape requires every list in got to match the English length, since
// the page pairs list items with fixed icons.
func checkShape(want, got reflect.Value, path string) error {
	switch want.Kind() {
	case reflect.Slice:
		if want.Len() != got.Len() {
			return fmt.Errorf("%s: %d items, English has %d", path, got.Len(), want.Len())
		}
	case reflect.Struct:
		for i := 0; i < want.NumField(); i++ {
			if err := checkShape(want.Field(i), got.Field(i), path+"."+want.Type().Field(i).Name); err != nil {
				return err
			}
		}
	}
	return nil
}
