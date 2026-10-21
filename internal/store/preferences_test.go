package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func mustDecode(t *testing.T, doc string) Preferences {
	t.Helper()
	p, err := DecodePreferences([]byte(doc))
	if err != nil {
		t.Fatalf("DecodePreferences(%s): %v", doc, err)
	}
	return p
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(b)
}

// sameJSON compares two JSON documents by value, ignoring key order and spacing.
func sameJSON(t *testing.T, got, want []byte) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("got is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("want is not JSON: %v\n%s", err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("JSON differs:\n got %s\nwant %s", got, want)
	}
}

// allNonDefault returns the same settings the Swift fixture swift_all.json was
// produced from: every one of the 34 fields differs from its default.
func allNonDefault() Preferences {
	return Preferences{
		SelectedProvider:                          ProviderGrok,
		Language:                                  LanguageJapanese,
		Appearance:                                AppearanceDark,
		MainPanelBackgroundStyle:                  PanelBackgroundOpaque,
		StatusBarDisplayStyle:                     StatusBarStyleRings,
		StatusBarMetersDetailStyle:                MetersDetailBoth,
		StatusBarBatteryScope:                     BatteryScopeBoth,
		StatusBarBatteryDetailStyle:               BatteryDetailRemainingPercent,
		StatusBarProviderScope:                    ProviderScopeBoth,
		RefreshIntervalSeconds:                    900,
		WidgetRefreshIntervalSeconds:              600,
		APICostSummaryRange:                       CostRangeThisMonth,
		MainPanelHeight:                           700.5,
		MainPanelModuleOrder:                      []Module{ModuleSessionRepair, ModuleRecentSessions, ModuleQuota, ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleCacheAnalysis, ModuleAPICost, ModuleSkills},
		ShowsQuotaSummary:                         false,
		ShowsResetCreditsSummary:                  false,
		ShowsQuotaEstimateSummary:                 false,
		QuotaEstimateWindowMode:                   WindowModeRollingWeek,
		ShowsCostSummary:                          false,
		ShowsRecentSessions:                       true,
		ShowsSessionRepairSummary:                 false,
		ShowsRateLimitResetToday:                  false,
		ShowsModelSpecificQuotaUsage:              true,
		ShowsTokenUsageHeatmap:                    false,
		TokenUsageChartStyle:                      ChartStyleBar,
		RateLimitResetTodayRefreshIntervalSeconds: 3600,
		AutomaticallyChecksForUpdates:             false,
		LaunchAtLoginEnabled:                      false,
		LaunchAtLoginInitialized:                  true,
		QuotaAlertsEnabled:                        true,
		QuotaAlertThresholds:                      DefaultQuotaAlertThresholds(),
		ResetCreditAlertsEnabled:                  true,
		RateLimitResetTodayAlertsEnabled:          false,
		ExportsStatusJSON:                         true,
		AccountPrivacyModeEnabled:                 true,
		// The frozen Swift fixture knows no speed analysis switch; it reads as on.
		ShowsSpeedAnalysis:  true,
		ShowsCacheAnalysis:  true,
		ShowsCreditsBalance: true,
		ShowsSkills:         true,
	}
}

func TestThirtyFourFieldsAllDifferFromDefaults(t *testing.T) {
	d, a := reflect.ValueOf(DefaultPreferences()), reflect.ValueOf(allNonDefault())
	typ := d.Type()
	count := 0
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Tag.Get("json") == "" || typ.Field(i).Name == "CursorMonitoringEnabled" || typ.Field(i).Name == "ThemeColor" || typ.Field(i).Name == "FontFamily" || typ.Field(i).Name == "QuotaResetTimeFormat" || typ.Field(i).Name == "ProxyTestURL" || typ.Field(i).Name == "ShowsSpeedAnalysis" || typ.Field(i).Name == "ShowsCacheAnalysis" || typ.Field(i).Name == "ShowsCreditsBalance" || typ.Field(i).Name == "ShowsSkills" || typ.Field(i).Name == "StatusBarProviders" || typ.Field(i).Name == "FloatingWindow" || typ.Field(i).Name == "PanelProviders" || typ.Field(i).Name == "DisableMainPanelAnimation" || typ.Field(i).Name == "QuotaAlertThresholds" {
			continue // the unexported extra map, and the keys only Go writes
		}
		count++
		if reflect.DeepEqual(d.Field(i).Interface(), a.Field(i).Interface()) {
			t.Errorf("field %s is the same in allNonDefault and the defaults", typ.Field(i).Name)
		}
	}
	if count != 34 {
		t.Errorf("Preferences has %d stored fields, want 34", count)
	}
}

func TestDefaultsMatchSwiftExceptAutomaticPanelHeight(t *testing.T) {
	d := DefaultPreferences()
	if got := mustDecode(t, `{}`); !reflect.DeepEqual(got, d) {
		t.Errorf("decoding {} = %+v, want defaults", got)
	}
	enc, err := EncodePreferences(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"mainPanelHeight":0,`) {
		t.Errorf("automatic mainPanelHeight should encode as zero: %s", enc)
	}
	if got := mustDecode(t, string(enc)); !reflect.DeepEqual(got, d) {
		t.Errorf("default round trip = %+v, want %+v", got, d)
	}
	// Keep the historical fixture frozen; only the new height default differs.
	d.MainPanelHeight = 584
	legacy, err := EncodePreferences(d)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, legacy, fixture(t, "swift_default.json"))
}

func TestSwiftProducedFixtures(t *testing.T) {
	legacyDefault := DefaultPreferences()
	legacyDefault.MainPanelHeight = 584
	if got := mustDecode(t, string(fixture(t, "swift_default.json"))); !reflect.DeepEqual(got, legacyDefault) {
		t.Errorf("swift_default.json decoded to %+v", got)
	}
	got := mustDecode(t, string(fixture(t, "swift_all.json")))
	if want := allNonDefault(); !reflect.DeepEqual(got, want) {
		t.Errorf("swift_all.json decoded to\n%+v\nwant\n%+v", got, want)
	}
	enc, err := EncodePreferences(allNonDefault())
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, enc, fixture(t, "swift_all.json"))
	if !strings.Contains(string(enc), `"mainPanelHeight":700.5,`) {
		t.Errorf("fractional height lost: %s", enc)
	}
}

func TestOldPreferencesMissingKeysUseDefaults(t *testing.T) {
	// The document from the Swift test oldPreferencesDefaultProvider.
	p := mustDecode(t, `{"language":"english","appearance":"dark","refreshIntervalSeconds":300}`)
	want := DefaultPreferences()
	want.Language, want.Appearance = LanguageEnglish, AppearanceDark
	if !reflect.DeepEqual(p, want) {
		t.Errorf("got %+v, want %+v", p, want)
	}
}

func TestDisableMainPanelAnimationPreference(t *testing.T) {
	for name, doc := range map[string]string{
		"missing": `{}`,
		"null":    `{"disableMainPanelAnimation":null}`,
		"false":   `{"disableMainPanelAnimation":false}`,
	} {
		if got := mustDecode(t, doc).DisableMainPanelAnimation; got {
			t.Errorf("%s decoded to true", name)
		}
	}
	p := mustDecode(t, `{"disableMainPanelAnimation":true}`)
	if !p.DisableMainPanelAnimation {
		t.Fatal("explicit true was not decoded")
	}
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"disableMainPanelAnimation":true`) {
		t.Fatalf("true was not encoded: %s", enc)
	}
	p.DisableMainPanelAnimation = false
	enc, err = EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "disableMainPanelAnimation") {
		t.Fatalf("false was not omitted: %s", enc)
	}
	for _, bad := range []string{`0`, `"true"`, `[]`, `{}`} {
		if _, err := DecodePreferences([]byte(`{"disableMainPanelAnimation":` + bad + `}`)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("value %s: err = %v, want ErrCorrupt", bad, err)
		}
	}
}

func TestBoolMissingVersusExplicit(t *testing.T) {
	bools := map[string]func(*Preferences) *bool{
		"showsQuotaSummary":                func(p *Preferences) *bool { return &p.ShowsQuotaSummary },
		"showsResetCreditsSummary":         func(p *Preferences) *bool { return &p.ShowsResetCreditsSummary },
		"showsQuotaEstimateSummary":        func(p *Preferences) *bool { return &p.ShowsQuotaEstimateSummary },
		"showsCostSummary":                 func(p *Preferences) *bool { return &p.ShowsCostSummary },
		"showsRecentSessions":              func(p *Preferences) *bool { return &p.ShowsRecentSessions },
		"showsSessionRepairSummary":        func(p *Preferences) *bool { return &p.ShowsSessionRepairSummary },
		"showsRateLimitResetToday":         func(p *Preferences) *bool { return &p.ShowsRateLimitResetToday },
		"showsModelSpecificQuotaUsage":     func(p *Preferences) *bool { return &p.ShowsModelSpecificQuotaUsage },
		"showsTokenUsageHeatmap":           func(p *Preferences) *bool { return &p.ShowsTokenUsageHeatmap },
		"automaticallyChecksForUpdates":    func(p *Preferences) *bool { return &p.AutomaticallyChecksForUpdates },
		"launchAtLoginEnabled":             func(p *Preferences) *bool { return &p.LaunchAtLoginEnabled },
		"launchAtLoginInitialized":         func(p *Preferences) *bool { return &p.LaunchAtLoginInitialized },
		"quotaAlertsEnabled":               func(p *Preferences) *bool { return &p.QuotaAlertsEnabled },
		"resetCreditAlertsEnabled":         func(p *Preferences) *bool { return &p.ResetCreditAlertsEnabled },
		"rateLimitResetTodayAlertsEnabled": func(p *Preferences) *bool { return &p.RateLimitResetTodayAlertsEnabled },
		"exportsStatusJSON":                func(p *Preferences) *bool { return &p.ExportsStatusJSON },
		"accountPrivacyModeEnabled":        func(p *Preferences) *bool { return &p.AccountPrivacyModeEnabled },
		"showsSpeedAnalysis":               func(p *Preferences) *bool { return &p.ShowsSpeedAnalysis },
		"showsCacheAnalysis":               func(p *Preferences) *bool { return &p.ShowsCacheAnalysis },
	}
	if len(bools) != 19 {
		t.Fatalf("expected 19 bool fields, have %d", len(bools))
	}
	defaults := DefaultPreferences()
	for key, field := range bools {
		def := *field(&defaults)
		for _, tc := range []struct {
			name, doc string
			want      bool
		}{
			{"missing", `{}`, def},
			{"null", `{"` + key + `":null}`, def},
			{"explicit true", `{"` + key + `":true}`, true},
			{"explicit false", `{"` + key + `":false}`, false},
		} {
			p := mustDecode(t, tc.doc)
			if got := *field(&p); got != tc.want {
				t.Errorf("%s %s: got %v, want %v", key, tc.name, got, tc.want)
			}
		}
		for _, bad := range []string{`0`, `1`, `"true"`, `[]`, `{}`} {
			if _, err := DecodePreferences([]byte(`{"` + key + `":` + bad + `}`)); !errors.Is(err, ErrCorrupt) {
				t.Errorf("%s=%s: err = %v, want ErrCorrupt", key, bad, err)
			}
		}
	}
}

func checkEnum[T ~string](t *testing.T, key string, values []T, valid func(T) bool, get func(Preferences) T) {
	t.Helper()
	for _, v := range values {
		if !valid(v) {
			t.Errorf("%s: %q should be valid", key, v)
		}
		p := mustDecode(t, `{"`+key+`":"`+string(v)+`"}`)
		if got := get(p); got != v {
			t.Errorf("%s: decoded %q, want %q", key, got, v)
		}
	}
	for _, bad := range []string{`"klingon"`, `""`, `"Codex"`, `1`, `true`, `[]`} {
		if _, err := DecodePreferences([]byte(`{"` + key + `":` + bad + `}`)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("%s=%s: err = %v, want ErrCorrupt", key, bad, err)
		}
	}
	if valid(T("klingon")) {
		t.Errorf("%s: klingon must not be valid", key)
	}
	if p := mustDecode(t, `{"`+key+`":null}`); get(p) != get(DefaultPreferences()) {
		t.Errorf("%s: null should give the default", key)
	}
}

func TestEveryEnumValue(t *testing.T) {
	checkEnum(t, "selectedProvider", Providers(), Provider.Valid, func(p Preferences) Provider { return p.SelectedProvider })
	checkEnum(t, "language", Languages(), Language.Valid, func(p Preferences) Language { return p.Language })
	checkEnum(t, "appearance", Appearances(), Appearance.Valid, func(p Preferences) Appearance { return p.Appearance })
	checkEnum(t, "mainPanelBackgroundStyle", PanelBackgrounds(), PanelBackground.Valid, func(p Preferences) PanelBackground { return p.MainPanelBackgroundStyle })
	checkEnum(t, "statusBarDisplayStyle", StatusBarStyles(), StatusBarStyle.Valid, func(p Preferences) StatusBarStyle { return p.StatusBarDisplayStyle })
	checkEnum(t, "statusBarMetersDetailStyle", MetersDetails(), MetersDetail.Valid, func(p Preferences) MetersDetail { return p.StatusBarMetersDetailStyle })
	checkEnum(t, "statusBarBatteryScope", BatteryScopes(), BatteryScope.Valid, func(p Preferences) BatteryScope { return p.StatusBarBatteryScope })
	checkEnum(t, "statusBarBatteryDetailStyle", BatteryDetails(), BatteryDetail.Valid, func(p Preferences) BatteryDetail { return p.StatusBarBatteryDetailStyle })
	// custom with no list falls back to selected, so it is not a round-trip value here.
	checkEnum(t, "statusBarProviderScope", []StatusBarProviderScope{ProviderScopeSelected, ProviderScopeBoth}, StatusBarProviderScope.Valid, func(p Preferences) StatusBarProviderScope { return p.StatusBarProviderScope })
	checkEnum(t, "apiCostSummaryRange", CostRanges(), CostRange.Valid, func(p Preferences) CostRange { return p.APICostSummaryRange })
	checkEnum(t, "tokenUsageChartStyle", ChartStyles(), ChartStyle.Valid, func(p Preferences) ChartStyle { return p.TokenUsageChartStyle })
}

func TestEnumRawValuesMatchSwift(t *testing.T) {
	strs := func(vs ...string) []string { return vs }
	conv := func(vs any) []string {
		rv := reflect.ValueOf(vs)
		out := make([]string, rv.Len())
		for i := range out {
			out[i] = rv.Index(i).String()
		}
		return out
	}
	for name, tc := range map[string]struct {
		got  []string
		want []string
	}{
		"provider":      {conv(CLIProviders()), strs("codex", "claude", "grok")},
		"language":      {conv(Languages()), strs("system", "english", "simplifiedChinese", "traditionalChinese", "korean", "japanese", "russian", "french")},
		"appearance":    {conv(Appearances()), strs("system", "light", "dark")},
		"background":    {conv(PanelBackgrounds()), strs("translucent", "opaque")},
		"statusBar":     {conv(StatusBarStyles()), strs("meters", "countdown", "battery", "rings", "text")},
		"metersDetail":  {conv(MetersDetails()), strs("remainingPercent", "resetTime", "both")},
		"batteryScope":  {conv(BatteryScopes()), strs("fiveHour", "weekly", "both")},
		"batteryDetail": {conv(BatteryDetails()), strs("countdown", "remainingPercent")},
		"providerScope": {conv(StatusBarProviderScopes()), strs("selected", "both", "custom")},
		"costRange":     {conv(CostRanges()), strs("today", "current", "previous", "thisMonth")},
		"chart":         {conv(ChartStyles()), strs("heatmap", "line", "bar")},
		"modules":       {conv(Modules()), strs("quota", "tokenUsage", "speedAnalysis", "rateLimitResetToday", "quotaEstimate", "resetCredits", "cacheAnalysis", "apiCost", "sessionRepair", "recentSessions", "skills")},
		"windowMode":    {conv(WindowModes()), strs("auto", "rollingWeek")},
		"proxyMode":     {conv(ProxyModes()), strs("system", "http", "socks5")},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s: %v, want %v", name, tc.got, tc.want)
		}
	}
}

func TestStatusBarProviderSelection(t *testing.T) {
	both := mustDecode(t, `{"statusBarProviderScope":"both"}`)
	if both.StatusBarProviderScope != ProviderScopeBoth || !slices.Equal(both.StatusBarSelection(), []Provider{ProviderCodex, ProviderGrok}) {
		t.Fatalf("both = %+v selection %v", both.StatusBarProviderScope, both.StatusBarSelection())
	}
	enc, err := EncodePreferences(both)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "statusBarProviders") || !strings.Contains(string(enc), `"statusBarProviderScope":"both"`) {
		t.Fatalf("legacy both must stay both without a provider list: %s", enc)
	}
	custom := mustDecode(t, `{"statusBarProviderScope":"custom","statusBarProviders":["claude","codex","nope","codex"]}`)
	if custom.StatusBarProviderScope != ProviderScopeCustom || !slices.Equal(custom.StatusBarProviders, []Provider{ProviderCodex, ProviderClaude}) {
		t.Fatalf("custom = %v %v", custom.StatusBarProviderScope, custom.StatusBarProviders)
	}
	enc, err = EncodePreferences(custom)
	if err != nil || !strings.Contains(string(enc), `"statusBarProviders":["codex","claude"]`) {
		t.Fatalf("encode custom: %v %s", err, enc)
	}
	if back := mustDecode(t, string(enc)); !slices.Equal(back.StatusBarProviders, custom.StatusBarProviders) {
		t.Fatalf("round trip %v", back.StatusBarProviders)
	}
	empty := mustDecode(t, `{"statusBarProviderScope":"custom","statusBarProviders":["nope"]}`)
	if empty.StatusBarProviderScope != ProviderScopeSelected || empty.StatusBarProviders != nil {
		t.Fatalf("empty custom = %v %v", empty.StatusBarProviderScope, empty.StatusBarProviders)
	}
	if _, err := DecodePreferences([]byte(`{"statusBarProviders":"codex"}`)); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("bad list: %v", err)
	}
}

func TestQuotaEstimateWindowModeIsTolerant(t *testing.T) {
	for doc, want := range map[string]WindowMode{
		`{"quotaEstimateWindowMode":"officialWeek"}`: WindowModeAuto, // legacy value
		`{"quotaEstimateWindowMode":"auto"}`:         WindowModeAuto,
		`{"quotaEstimateWindowMode":"rollingWeek"}`:  WindowModeRollingWeek,
		`{"quotaEstimateWindowMode":"future"}`:       WindowModeAuto,
		`{"quotaEstimateWindowMode":7}`:              WindowModeAuto,
		`{"quotaEstimateWindowMode":{"a":1}}`:        WindowModeAuto,
		`{"quotaEstimateWindowMode":null}`:           WindowModeAuto,
		`{}`:                                         WindowModeAuto,
	} {
		if got := mustDecode(t, doc).QuotaEstimateWindowMode; got != want {
			t.Errorf("%s: got %q, want %q", doc, got, want)
		}
	}
	// A re-encode writes the mapped value, never "officialWeek".
	enc, err := EncodePreferences(mustDecode(t, `{"quotaEstimateWindowMode":"officialWeek"}`))
	if err != nil || !strings.Contains(string(enc), `"quotaEstimateWindowMode":"auto"`) {
		t.Errorf("re-encode = %s, %v", enc, err)
	}
}

func TestIntFieldsDecodeLikeSwift(t *testing.T) {
	for doc, want := range map[string]int{
		`{"refreshIntervalSeconds":900}`:    900,
		`{"refreshIntervalSeconds":300.0}`:  300,
		`{"refreshIntervalSeconds":3e2}`:    300,
		`{"refreshIntervalSeconds":-5}`:     -5, // not clamped, as in Swift
		`{"refreshIntervalSeconds":0}`:      0,
		`{"refreshIntervalSeconds":null}`:   300,
		`{}`:                                300,
		`{"refreshIntervalSeconds":1.0e+3}`: 1000,
	} {
		if got := mustDecode(t, doc).RefreshIntervalSeconds; got != want {
			t.Errorf("%s: got %d, want %d", doc, got, want)
		}
	}
	for _, bad := range []string{`300.5`, `"300"`, `true`, `[]`, `1e999`, `9223372036854775808`, `1e19`} {
		if _, err := DecodePreferences([]byte(`{"refreshIntervalSeconds":` + bad + `}`)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("refreshIntervalSeconds=%s: err = %v, want ErrCorrupt", bad, err)
		}
	}
}

func TestClamping(t *testing.T) {
	for doc, want := range map[string]float64{
		`{"mainPanelHeight":0}`:     DefaultMainPanelHeight,
		`{"mainPanelHeight":null}`:  DefaultMainPanelHeight,
		`{"mainPanelHeight":100}`:   360,
		`{"mainPanelHeight":359}`:   360,
		`{"mainPanelHeight":360}`:   360,
		`{"mainPanelHeight":612.5}`: 612.5,
		`{"mainPanelHeight":900}`:   900,
		`{"mainPanelHeight":1200}`:  1200, // The UI now applies the screen-dependent maximum.
		`{"mainPanelHeight":-1}`:    360,
		`{}`:                        DefaultMainPanelHeight,
	} {
		if got := mustDecode(t, doc).MainPanelHeight; got != want {
			t.Errorf("%s: got %v, want %v", doc, got, want)
		}
	}
	for _, bad := range []string{`"700"`, `true`, `[]`, `1e999`} {
		if _, err := DecodePreferences([]byte(`{"mainPanelHeight":` + bad + `}`)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("mainPanelHeight=%s: err = %v, want ErrCorrupt", bad, err)
		}
	}
	for in, want := range map[int]int{-5: 60, 0: 60, 59: 60, 60: 60, 120: 120, 1800: 1800, 1801: 1800, 99999: 1800} {
		if got := ClampWidgetRefreshInterval(in); got != want {
			t.Errorf("widget clamp(%d) = %d, want %d", in, got, want)
		}
		if got := mustDecode(t, jsonInt("widgetRefreshIntervalSeconds", in)).WidgetRefreshIntervalSeconds; got != want {
			t.Errorf("decoded widget(%d) = %d, want %d", in, got, want)
		}
	}
	for in, want := range map[int]int{1: 300, 60: 300, 300: 300, 600: 600, 50_000: 21_600, 21_600: 21_600} {
		if got := ClampRateLimitResetTodayRefreshInterval(in); got != want {
			t.Errorf("reset-today clamp(%d) = %d, want %d", in, got, want)
		}
		if got := mustDecode(t, jsonInt("rateLimitResetTodayRefreshIntervalSeconds", in)).RateLimitResetTodayRefreshIntervalSeconds; got != want {
			t.Errorf("decoded reset-today(%d) = %d, want %d", in, got, want)
		}
	}
	if got := ClampMainPanelHeight(math.Inf(1)); got != DefaultMainPanelHeight {
		t.Errorf("infinite height = %v, want default", got)
	}
	if got := ClampMainPanelHeight(math.NaN()); got != DefaultMainPanelHeight {
		t.Errorf("NaN height = %v, want default", got)
	}
}

func TestRefreshOptions(t *testing.T) {
	if got, want := WidgetRefreshIntervalOptions(), []int{60, 300, 600, 900, 1800}; !slices.Equal(got, want) {
		t.Errorf("widget options = %v", got)
	}
	if got, want := RateLimitResetTodayRefreshIntervalOptions(), []int{300, 600, 900, 1800, 3600, 7200, 21600}; !slices.Equal(got, want) {
		t.Errorf("reset-today options = %v", got)
	}
	for _, o := range WidgetRefreshIntervalOptions() {
		if ClampWidgetRefreshInterval(o) != o {
			t.Errorf("widget option %d is not stable under clamping", o)
		}
	}
	for _, o := range RateLimitResetTodayRefreshIntervalOptions() {
		if ClampRateLimitResetTodayRefreshInterval(o) != o {
			t.Errorf("reset-today option %d is not stable under clamping", o)
		}
	}
}

func TestModuleOrderNormalization(t *testing.T) {
	d := Modules()
	for _, tc := range []struct {
		name string
		doc  string
		want []Module
	}{
		{"missing key", `{}`, d},
		{"empty array", `{"mainPanelModuleOrder":[]}`, d},
		{"null", `{"mainPanelModuleOrder":null}`, d},
		{"Swift test: duplicates, unknown, missing", `{"mainPanelModuleOrder":["apiCost","unknown-future-module","quota","apiCost"]}`,
			[]Module{ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleCacheAnalysis, ModuleAPICost, ModuleQuota, ModuleSessionRepair, ModuleRecentSessions, ModuleSkills}},
		{"user order kept, missing appended before neighbours", `{"mainPanelModuleOrder":["recentSessions","apiCost"]}`,
			[]Module{ModuleSessionRepair, ModuleRecentSessions, ModuleQuota, ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleCacheAnalysis, ModuleAPICost, ModuleSkills}},
		{"reversed is kept", `{"mainPanelModuleOrder":["recentSessions","sessionRepair","apiCost","resetCredits","quotaEstimate","rateLimitResetToday","tokenUsage","quota"]}`,
			[]Module{ModuleRecentSessions, ModuleSessionRepair, ModuleCacheAnalysis, ModuleAPICost, ModuleResetCredits, ModuleQuotaEstimate, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleTokenUsage, ModuleQuota, ModuleSkills}},
		{"explicit cache position is kept", `{"mainPanelModuleOrder":["apiCost","cacheAnalysis","quota"]}`,
			[]Module{ModuleAPICost, ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleCacheAnalysis, ModuleQuota, ModuleSessionRepair, ModuleRecentSessions, ModuleSkills}},
		{"new module inserted before its default neighbour (quotaEstimate before resetCredits)",
			`{"mainPanelModuleOrder":["quota","tokenUsage","rateLimitResetToday","resetCredits","apiCost","sessionRepair","recentSessions"]}`, d},
		{"all unknown", `{"mainPanelModuleOrder":["a","b"]}`, d},
		{"case sensitive", `{"mainPanelModuleOrder":["Quota"]}`, d},
	} {
		if got := mustDecode(t, tc.doc).MainPanelModuleOrder; !slices.Equal(got, tc.want) {
			t.Errorf("%s:\n got %v\nwant %v", tc.name, got, tc.want)
		}
	}
	for _, bad := range []string{`"quota"`, `{}`, `[1]`, `["quota",null]`, `[["quota"]]`, `true`} {
		if _, err := DecodePreferences([]byte(`{"mainPanelModuleOrder":` + bad + `}`)); !errors.Is(err, ErrCorrupt) {
			t.Errorf("mainPanelModuleOrder=%s: err = %v, want ErrCorrupt", bad, err)
		}
	}
}

func TestNormalizeModuleOrderProperties(t *testing.T) {
	inputs := [][]Module{
		nil, {}, {ModuleAPICost}, {ModuleAPICost, ModuleAPICost}, {"x", ModuleQuota, "y"},
		{ModuleRecentSessions, ModuleQuota, ModuleSessionRepair},
	}
	for _, in := range inputs {
		got := NormalizeModuleOrder(in)
		if len(got) != len(Modules()) {
			t.Errorf("%v -> %v: want every module exactly once", in, got)
		}
		seen := map[Module]bool{}
		for _, m := range got {
			if seen[m] || !m.Valid() {
				t.Errorf("%v -> %v: duplicate or unknown module %q", in, got, m)
			}
			seen[m] = true
		}
		if again := NormalizeModuleOrder(got); !slices.Equal(again, got) {
			t.Errorf("not idempotent: %v -> %v", got, again)
		}
	}
	in := []Module{ModuleAPICost, ModuleQuota}
	NormalizeModuleOrder(in)
	if !slices.Equal(in, []Module{ModuleAPICost, ModuleQuota}) {
		t.Errorf("input was modified: %v", in)
	}
}

func TestMoveAndResetModuleOrder(t *testing.T) {
	p := DefaultPreferences()
	p.MoveMainPanelModule(ModuleAPICost, -1)
	want := []Module{ModuleQuota, ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleAPICost, ModuleCacheAnalysis, ModuleSessionRepair, ModuleRecentSessions, ModuleSkills}
	if !slices.Equal(p.MainPanelModuleOrder, want) {
		t.Errorf("after move: %v", p.MainPanelModuleOrder)
	}
	p.MoveMainPanelModule(ModuleQuota, -1) // already first
	if p.MainPanelModuleOrder[0] != ModuleQuota {
		t.Errorf("moving the first module up changed the order: %v", p.MainPanelModuleOrder)
	}
	p.MoveMainPanelModule(ModuleRecentSessions, 1) // already last
	p.ResetMainPanelModuleOrder()
	if !slices.Equal(p.MainPanelModuleOrder, Modules()) {
		t.Errorf("reset: %v", p.MainPanelModuleOrder)
	}
}

func TestCorruptDocumentsAreErrors(t *testing.T) {
	for _, doc := range []string{
		``, `   `, `{`, `not json`, `null`, `[]`, `"x"`, `5`, `true`, `{"a":1}}`, `{"language":"klingon"}`,
		`{"language":1}`, `{"showsQuotaSummary":"yes"}`, `{"refreshIntervalSeconds":1.5}`,
		`{"mainPanelModuleOrder":"quota"}`, "\xff\xfe",
	} {
		p, err := DecodePreferences([]byte(doc))
		if !errors.Is(err, ErrCorrupt) {
			t.Errorf("%q: err = %v, want ErrCorrupt", doc, err)
		}
		if err != nil && !reflect.DeepEqual(p, Preferences{}) {
			t.Errorf("%q: an error must come with the zero value, not defaults", doc)
		}
	}
	if _, err := DecodePreferences([]byte(`{"language":"klingon"}`)); err == nil || !strings.Contains(err.Error(), "language") {
		t.Errorf("error should name the key: %v", err)
	}
}

func TestKeysAreCaseSensitive(t *testing.T) {
	// Go's struct decoder would match "Language"; Swift ignores it as unknown.
	p := mustDecode(t, `{"Language":"english"}`)
	if p.Language != LanguageSystem {
		t.Errorf("Language matched case-insensitively: %q", p.Language)
	}
	if _, kept := p.extra["Language"]; !kept {
		t.Error("the mis-cased key should be kept as an unknown key")
	}
}

func TestUnknownKeysRoundTrip(t *testing.T) {
	doc := `{"language":"english","futureFlag":true,"futureObject":{"a":[1,2,{"b":null}],"s":"<x>&"},"futureNumber":1.50,"futureString":"é"}`
	p := mustDecode(t, doc)
	p.Appearance = AppearanceDark
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(enc, &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"futureFlag":   `true`,
		"futureObject": `{"a":[1,2,{"b":null}],"s":"<x>&"}`,
		"futureNumber": `1.50`,
		"futureString": `"é"`,
		"language":     `"english"`,
		"appearance":   `"dark"`,
	} {
		if string(got[key]) != want {
			t.Errorf("%s = %s, want %s", key, got[key], want)
		}
	}
	// Decoding the output again gives the same value.
	again := mustDecode(t, string(enc))
	if !reflect.DeepEqual(again, p) {
		t.Errorf("round trip changed the value:\n%+v\n%+v", again, p)
	}
	// An unknown key never overrides a known one.
	q := mustDecode(t, `{"language":"english"}`)
	q.extra = map[string]json.RawMessage{"language": json.RawMessage(`"french"`)}
	enc, _ = EncodePreferences(q)
	if !strings.Contains(string(enc), `"language":"english"`) {
		t.Errorf("extra key overrode a known key: %s", enc)
	}
}

func TestCloneIsDeep(t *testing.T) {
	p := mustDecode(t, `{"x":1}`)
	c := p.Clone()
	c.MainPanelModuleOrder[0] = ModuleRecentSessions
	c.extra["y"] = json.RawMessage(`2`)
	if p.MainPanelModuleOrder[0] != ModuleQuota || len(p.extra) != 1 {
		t.Errorf("Clone shares memory with the original: %+v", p)
	}
}

func TestEncodeNormalizesAndValidates(t *testing.T) {
	p := DefaultPreferences()
	p.MainPanelHeight = 100
	p.WidgetRefreshIntervalSeconds = 1
	p.RateLimitResetTodayRefreshIntervalSeconds = 1
	p.MainPanelModuleOrder = []Module{ModuleAPICost}
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	got := mustDecode(t, string(enc))
	if got.MainPanelHeight != 360 || got.WidgetRefreshIntervalSeconds != 60 || got.RateLimitResetTodayRefreshIntervalSeconds != 300 || len(got.MainPanelModuleOrder) != len(Modules()) {
		t.Errorf("not normalized: %+v", got)
	}
	if p.MainPanelHeight != 100 {
		t.Error("EncodePreferences modified its argument")
	}

	for name, mutate := range map[string]func(*Preferences){
		"provider":   func(p *Preferences) { p.SelectedProvider = "bogus" },
		"language":   func(p *Preferences) { p.Language = "" },
		"appearance": func(p *Preferences) { p.Appearance = "sepia" },
		"chart":      func(p *Preferences) { p.TokenUsageChartStyle = "pie" },
		"windowMode": func(p *Preferences) { p.QuotaEstimateWindowMode = "officialWeek" },
	} {
		bad := DefaultPreferences()
		mutate(&bad)
		if out, err := EncodePreferences(bad); err == nil {
			t.Errorf("%s: invalid enum encoded without error: %s", name, out)
		}
	}
	if _, err := EncodePreferences(Preferences{}); err == nil {
		t.Error("the zero Preferences has empty enums and must not encode")
	}
}

func TestEncodedFloatFormat(t *testing.T) {
	for height, want := range map[float64]string{584: `"mainPanelHeight":584,`, 700.5: `"mainPanelHeight":700.5,`, 900: `"mainPanelHeight":900,`, 1620: `"mainPanelHeight":1620,`} {
		p := DefaultPreferences()
		p.MainPanelHeight = height
		enc, err := EncodePreferences(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(enc), want) {
			t.Errorf("height %v: %s", height, enc)
		}
		if got := mustDecode(t, string(enc)).MainPanelHeight; got != height {
			t.Errorf("height round trip = %v, want %v", got, height)
		}
	}
}

func jsonInt(key string, v int) string {
	b, _ := json.Marshal(map[string]int{key: v})
	return string(b)
}

func TestThemeColorIsGoOnlyAndLenient(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "themeColor") {
		t.Errorf("the brand color is written: %s", enc)
	}
	for raw, want := range map[string]string{
		`"#007aff"`: "#007AFF",
		`"#B3582D"`: "#B3582D",
		`"blue"`:    "",
		`"#12345"`:  "",
		`"#GGGGGG"`: "",
		`12`:        "",
		`null`:      "",
	} {
		got := mustDecode(t, `{"themeColor":`+raw+`}`)
		if got.ThemeColor != want {
			t.Errorf("themeColor %s decoded to %q, want %q", raw, got.ThemeColor, want)
		}
	}
	p := DefaultPreferences()
	p.ThemeColor = "#62ba46"
	enc, err = EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(enc)); got.ThemeColor != "#62BA46" {
		t.Errorf("round trip = %q", got.ThemeColor)
	}
}

func TestPanelProvidersAreGoOnlyAndLenient(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "panelProviders") {
		t.Errorf("showing every provider is written: %s", enc)
	}
	for raw, want := range map[string][]Provider{
		`["grok","codex","codex"]`: {ProviderCodex, ProviderGrok},
		`["claude","gemini"]`:      {ProviderClaude},
		`["gemini"]`:               nil,
		`[]`:                       nil,
		`"codex"`:                  nil,
		`[1]`:                      nil,
		`null`:                     nil,
	} {
		if got := mustDecode(t, `{"panelProviders":`+raw+`}`); !slices.Equal(got.PanelProviders, want) {
			t.Errorf("panelProviders %s decoded to %v, want %v", raw, got.PanelProviders, want)
		}
	}
	p := DefaultPreferences()
	p.PanelProviders = []Provider{ProviderCodex}
	if enc, err = EncodePreferences(p); err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(enc)); !slices.Equal(got.PanelProviders, p.PanelProviders) {
		t.Errorf("round trip = %v", got.PanelProviders)
	}
}

func TestPanelSelectionKeepsSelectedProviderShown(t *testing.T) {
	p := DefaultPreferences()
	if got := p.PanelSelection(); !slices.Equal(got, Providers()) {
		t.Errorf("default selection = %v, want every provider", got)
	}
	p.SelectedProvider = ProviderGrok
	p.PanelProviders = []Provider{ProviderGrok, ProviderClaude}
	p.Normalize()
	if p.SelectedProvider != ProviderGrok {
		t.Errorf("a shown selection moved to %s", p.SelectedProvider)
	}
	if got := p.PanelSelection(); !slices.Equal(got, []Provider{ProviderClaude, ProviderGrok}) {
		t.Errorf("selection = %v, want Providers() order", got)
	}
	p.PanelProviders = []Provider{ProviderClaude}
	p.Normalize()
	if p.SelectedProvider != ProviderClaude {
		t.Errorf("a hidden selection stayed on %s, want claude", p.SelectedProvider)
	}
	got := mustDecode(t, `{"selectedProvider":"grok","panelProviders":["codex"]}`)
	if got.SelectedProvider != ProviderCodex {
		t.Errorf("decoded a hidden selection as %s, want codex", got.SelectedProvider)
	}
}

func TestFontFamilyIsGoOnlyAndLenient(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "fontFamily") {
		t.Errorf("the system font is written: %s", enc)
	}
	for raw, want := range map[string]string{
		`"PingFang SC"`:      "PingFang SC",
		`"  Menlo  "`:        "Menlo",
		`"Menlo, monospace"`: "",
		`"Bad\nName"`:        "",
		`12`:                 "",
		`null`:               "",
	} {
		if got := mustDecode(t, `{"fontFamily":`+raw+`}`); got.FontFamily != want {
			t.Errorf("fontFamily %s decoded to %q, want %q", raw, got.FontFamily, want)
		}
	}
	p := DefaultPreferences()
	p.FontFamily = "Source Han Serif SC"
	if enc, err = EncodePreferences(p); err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(enc)); got.FontFamily != p.FontFamily {
		t.Errorf("round trip = %q", got.FontFamily)
	}
	if got := NormalizeFontFamily(strings.Repeat("a", 257)); got != "" {
		t.Errorf("an overlong name normalized to %q", got)
	}
}

func TestQuotaResetTimeFormatIsGoOnlyAndLenient(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "quotaResetTimeFormat") {
		t.Errorf("the relative format is written: %s", enc)
	}
	for raw, want := range map[string]ResetTimeFormat{
		`"exact"`:    ResetTimeExact,
		`"relative"`: ResetTimeRelative,
		`"Exact"`:    ResetTimeRelative,
		`1`:          ResetTimeRelative,
		`null`:       ResetTimeRelative,
	} {
		if got := mustDecode(t, `{"quotaResetTimeFormat":`+raw+`}`); got.QuotaResetTimeFormat != want {
			t.Errorf("quotaResetTimeFormat %s decoded to %q, want %q", raw, got.QuotaResetTimeFormat, want)
		}
	}
	p := DefaultPreferences()
	p.QuotaResetTimeFormat = ResetTimeExact
	if enc, err = EncodePreferences(p); err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(enc)); got.QuotaResetTimeFormat != ResetTimeExact {
		t.Errorf("round trip = %q", got.QuotaResetTimeFormat)
	}
	p.QuotaResetTimeFormat = "bogus"
	if p.Normalize(); p.QuotaResetTimeFormat != ResetTimeRelative {
		t.Errorf("normalized bogus format = %q", p.QuotaResetTimeFormat)
	}
	if ResetTimeExact.Toggled() != ResetTimeRelative || ResetTimeRelative.Toggled() != ResetTimeExact {
		t.Error("Toggled does not flip the format")
	}
}

func TestProxyTestURLIsGoOnlyAndLenient(t *testing.T) {
	enc, err := EncodePreferences(DefaultPreferences())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "proxyTestURL") {
		t.Errorf("the default test site is written: %s", enc)
	}
	if got := DefaultPreferences().ProxyTestTarget(); got != DefaultProxyTestURL {
		t.Errorf("default target = %q", got)
	}
	for raw, want := range map[string]string{
		`"https://example.com/ping"`: "https://example.com/ping",
		`"https://github.com"`:       "",
		`"https://github.com/"`:      "",
		`"ftp://example.com"`:        "",
		`"https://u:p@example.com"`:  "",
		`1`:                          "",
		`null`:                       "",
	} {
		if got := mustDecode(t, `{"proxyTestURL":`+raw+`}`); got.ProxyTestURL != want {
			t.Errorf("proxyTestURL %s decoded to %q, want %q", raw, got.ProxyTestURL, want)
		}
	}
	p := DefaultPreferences()
	p.ProxyTestURL = "http://192.168.1.2:8080/health"
	if enc, err = EncodePreferences(p); err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(enc)); got.ProxyTestTarget() != p.ProxyTestURL {
		t.Errorf("round trip = %q", got.ProxyTestURL)
	}
}

func TestParseProxyTestURL(t *testing.T) {
	for raw, want := range map[string]string{
		" example.com ":            "https://example.com",
		"http://example.com/a?b=c": "http://example.com/a?b=c",
		"HTTPS://Example.com":      "https://Example.com",
	} {
		if got, err := ParseProxyTestURL(raw); err != nil || got != want {
			t.Errorf("ParseProxyTestURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "   ", "https://", "file:///etc/hosts", "https://a b.com", "https://user@example.com", "mailto:a@b.c"} {
		if _, err := ParseProxyTestURL(raw); !errors.Is(err, ErrInvalidProxyTestURL) {
			t.Errorf("ParseProxyTestURL(%q) err = %v", raw, err)
		}
	}
}

func TestShowsSpeedAnalysisIsWrittenOnlyWhenOff(t *testing.T) {
	p := DefaultPreferences()
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "showsSpeedAnalysis") || strings.Contains(string(enc), "speedAnalysis") {
		t.Errorf("a default document carries the speed analysis: %s", enc)
	}
	p.ShowsSpeedAnalysis = false
	enc, err = EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"showsSpeedAnalysis":false`) {
		t.Errorf("off is not written: %s", enc)
	}
	if got := mustDecode(t, string(enc)); got.ShowsSpeedAnalysis {
		t.Error("off did not round trip")
	}
	if got := mustDecode(t, `{"showsSpeedAnalysis":true}`); !got.ShowsSpeedAnalysis {
		t.Error("explicit on read as off")
	}
	// An explicit true is the default and is not written back.
	back, err := EncodePreferences(mustDecode(t, `{"showsSpeedAnalysis":true,"futureKey":1}`))
	if err != nil || strings.Contains(string(back), "showsSpeedAnalysis") || !strings.Contains(string(back), `"futureKey":1`) {
		t.Errorf("re-encode = %s, %v", back, err)
	}
}

func TestShowsCacheAnalysisIsWrittenOnlyWhenOff(t *testing.T) {
	p := DefaultPreferences()
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "showsCacheAnalysis") || strings.Contains(string(enc), "cacheAnalysis") {
		t.Errorf("a default document carries cache analysis: %s", enc)
	}
	p.ShowsCacheAnalysis = false
	enc, err = EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"showsCacheAnalysis":false`) {
		t.Errorf("off is not written: %s", enc)
	}
	if got := mustDecode(t, string(enc)); got.ShowsCacheAnalysis {
		t.Error("off did not round trip")
	}
}

func TestSpeedModuleOrderWire(t *testing.T) {
	// The Swift order has no speedAnalysis: reading puts it after tokenUsage and
	// writing leaves it out again, so the document is the one that was read.
	const swiftOrder = `["sessionRepair","recentSessions","quota","tokenUsage","rateLimitResetToday","quotaEstimate","resetCredits","apiCost"]`
	p := mustDecode(t, `{"mainPanelModuleOrder":`+swiftOrder+`}`)
	want := []Module{ModuleSessionRepair, ModuleRecentSessions, ModuleQuota, ModuleTokenUsage, ModuleSpeedAnalysis, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleCacheAnalysis, ModuleAPICost, ModuleSkills}
	if !slices.Equal(p.MainPanelModuleOrder, want) {
		t.Fatalf("read order = %v", p.MainPanelModuleOrder)
	}
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"mainPanelModuleOrder":`+swiftOrder) {
		t.Errorf("written order changed: %s", enc)
	}
}

func TestSpeedModuleOrderMovedByTheUserRoundTrips(t *testing.T) {
	for _, order := range [][]Module{
		{ModuleSpeedAnalysis, ModuleQuota, ModuleTokenUsage, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleAPICost, ModuleSessionRepair, ModuleRecentSessions},
		{ModuleQuota, ModuleTokenUsage, ModuleRateLimitResetToday, ModuleSpeedAnalysis, ModuleQuotaEstimate, ModuleResetCredits, ModuleAPICost, ModuleSessionRepair, ModuleRecentSessions},
		{ModuleQuota, ModuleTokenUsage, ModuleRateLimitResetToday, ModuleQuotaEstimate, ModuleResetCredits, ModuleAPICost, ModuleSessionRepair, ModuleRecentSessions, ModuleSpeedAnalysis},
		{ModuleRecentSessions, ModuleSessionRepair, ModuleAPICost, ModuleResetCredits, ModuleQuotaEstimate, ModuleRateLimitResetToday, ModuleTokenUsage, ModuleQuota, ModuleSpeedAnalysis},
	} {
		p := DefaultPreferences()
		p.MainPanelModuleOrder = slices.Clone(order)
		enc, err := EncodePreferences(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(enc), `"speedAnalysis"`) {
			t.Errorf("order %v lost the moved module: %s", order, enc)
		}
		if got := mustDecode(t, string(enc)).MainPanelModuleOrder; !slices.Equal(got, NormalizeModuleOrder(order)) {
			t.Errorf("order %v round-tripped to %v", order, got)
		}
	}
}

func TestSpeedModuleOrderDefaultsAreOmittedWhereverTheyFall(t *testing.T) {
	// Moving a neighbour does not move the module in the written form: whatever
	// order reading would rebuild is not written.
	p := DefaultPreferences()
	p.MoveMainPanelModule(ModuleQuota, 1) // tokenUsage, quota, speedAnalysis, ...
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustDecode(t, string(enc)).MainPanelModuleOrder; !slices.Equal(got, p.MainPanelModuleOrder) {
		t.Errorf("order %v round-tripped to %v", p.MainPanelModuleOrder, got)
	}
}

func TestShowsCreditsBalanceGoOnlyAndDefaults(t *testing.T) {
	p := DefaultPreferences()
	if !p.ShowsCreditsBalance {
		t.Fatal("expected ShowsCreditsBalance to be true by default")
	}
	enc, err := EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(enc), "showsCreditsBalance") {
		t.Errorf("showsCreditsBalance must be omitted when true: %s", enc)
	}
	p.ShowsCreditsBalance = false
	enc, err = EncodePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(enc), `"showsCreditsBalance":false`) {
		t.Errorf("showsCreditsBalance must be written when false: %s", enc)
	}
	if got := mustDecode(t, string(enc)); got.ShowsCreditsBalance {
		t.Errorf("decoded ShowsCreditsBalance is %v, want false", got.ShowsCreditsBalance)
	}
}
