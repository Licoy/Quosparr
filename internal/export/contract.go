package export

import (
	"net/url"
	"strings"
	"time"
)

// Kind is a widget kind identifier, registered by the WidgetKit extension.
type Kind string

const (
	KindOverview   Kind = "com.quosparr.app.widget.overview"
	KindTokenTrend Kind = "com.quosparr.app.widget.token-trend"
	KindMetric     Kind = "com.quosparr.app.widget.metric"
	KindResetToday Kind = "com.quosparr.app.widget.reset-today"
)

// Kinds lists the widget kinds in Swift declaration order.
func Kinds() []Kind { return []Kind{KindOverview, KindTokenTrend, KindMetric, KindResetToday} }

// Provider is a quota provider.
type Provider string

const (
	ProviderCodex  Provider = "codex"
	ProviderGrok   Provider = "grok"
	ProviderClaude Provider = "claude"
)

// ProviderScope is which providers a widget shows.
type ProviderScope string

const (
	ScopeCodex       ProviderScope = "codex"
	ScopeGrok        ProviderScope = "grok"
	ScopeClaude      ProviderScope = "claude"
	ScopeBoth        ProviderScope = "both"
	ScopeCodexClaude ProviderScope = "codex-claude"
	ScopeGrokClaude  ProviderScope = "grok-claude"
)

// ProviderScopes lists the scopes in widget declaration order.
// both stays Codex + Grok so widgets already on screen keep their meaning.
func ProviderScopes() []ProviderScope {
	return []ProviderScope{ScopeCodex, ScopeGrok, ScopeClaude, ScopeBoth, ScopeCodexClaude, ScopeGrokClaude}
}

// Providers are the providers of a scope.
func (s ProviderScope) Providers() []Provider {
	switch s {
	case ScopeCodex:
		return []Provider{ProviderCodex}
	case ScopeGrok:
		return []Provider{ProviderGrok}
	case ScopeClaude:
		return []Provider{ProviderClaude}
	case ScopeBoth:
		return []Provider{ProviderCodex, ProviderGrok}
	case ScopeCodexClaude:
		return []Provider{ProviderCodex, ProviderClaude}
	case ScopeGrokClaude:
		return []Provider{ProviderGrok, ProviderClaude}
	}
	return nil
}

// Valid reports a known scope.
func (s ProviderScope) Valid() bool {
	switch s {
	case ScopeCodex, ScopeGrok, ScopeClaude, ScopeBoth, ScopeCodexClaude, ScopeGrokClaude:
		return true
	}
	return false
}

// MetricKind is the value a metric widget shows.
type MetricKind string

const (
	MetricRemainingQuota    MetricKind = "remainingQuota"
	MetricAPIEquivalentCost MetricKind = "apiEquivalentCost"
	MetricTokenCount        MetricKind = "tokenCount"
	MetricBalance           MetricKind = "balance"
)

// StorageMode says where the widget snapshot lives.
type StorageMode string

const (
	// StorageAppGroup is the macOS App Group container (opt-in: touching it
	// triggers a privacy prompt).
	StorageAppGroup StorageMode = "app-group"
	// StorageLocal is ~/.quosparr, the default for development and release builds.
	StorageLocal StorageMode = "local"

	// StorageModeInfoKey and AppGroupInfoKey are the Info.plist keys of the
	// Swift host that choose the mode and the group.
	StorageModeInfoKey = "RunwayWidgetStorageMode"
	AppGroupInfoKey    = "RunwayAppGroupID"
)

// StorageModeFromInfo decodes the Info.plist value; anything but "app-group"
// is local.
func StorageModeFromInfo(value string) StorageMode {
	if StorageMode(value) == StorageAppGroup {
		return StorageAppGroup
	}
	return StorageLocal
}

// Family is a widget size.
type Family string

const (
	FamilySmall  Family = "small"
	FamilyMedium Family = "medium"
	FamilyLarge  Family = "large"
)

// Families lists the sizes in Swift declaration order.
func Families() []Family { return []Family{FamilySmall, FamilyMedium, FamilyLarge} }

// RefreshInterval is how often the widget asks for a new timeline.
const RefreshInterval = 15 * time.Minute

// Supports reports whether a widget kind exists in a family.
func Supports(family Family, kind Kind) bool {
	switch kind {
	case KindOverview:
		return true
	case KindTokenTrend:
		return family == FamilyMedium || family == FamilyLarge
	case KindMetric:
		return family == FamilySmall
	case KindResetToday:
		return family == FamilySmall || family == FamilyMedium
	}
	return false
}

// TrendDays is how many days the token trend shows.
func TrendDays(family Family) int {
	if family == FamilyLarge {
		return 30
	}
	return 14
}

// QuotaLimit is how many quota rows fit; ok is false for no limit.
func QuotaLimit(family Family) (limit int, ok bool) {
	switch family {
	case FamilySmall:
		return 1, true
	case FamilyMedium:
		return 2, true
	}
	return 0, false
}

// Section is the part of the app a widget tap opens.
type Section string

const (
	SectionOverview   Section = "overview"
	SectionQuota      Section = "quota"
	SectionTokens     Section = "tokens"
	SectionCost       Section = "cost"
	SectionResetToday Section = "resetToday"
)

// Sections lists the sections in Swift declaration order.
func Sections() []Section {
	return []Section{SectionOverview, SectionQuota, SectionTokens, SectionCost, SectionResetToday}
}

// Valid reports a known section.
func (s Section) Valid() bool {
	for _, v := range Sections() {
		if s == v {
			return true
		}
	}
	return false
}

// Deep-link scheme and host: quosparr://widget?provider=...&section=...
const (
	DeepLinkScheme = "quosparr"
	DeepLinkHost   = "widget"
)

// DeepLink is the target of a widget tap.
type DeepLink struct {
	Provider ProviderScope
	Section  Section
}

// URL is quosparr://widget?provider=<scope>&section=<section>.
func (d DeepLink) URL() string {
	return DeepLinkScheme + "://" + DeepLinkHost + "?provider=" + url.QueryEscape(string(d.Provider)) +
		"&section=" + url.QueryEscape(string(d.Section))
}

// ParseDeepLink accepts exactly the links URL produces: lowercase scheme,
// host "widget", an empty or "/" path and exactly the two query items provider
// and section with known values. A fragment is ignored. The scheme is matched
// case-sensitively, as Foundation does.
func ParseDeepLink(raw string) (DeepLink, bool) {
	if i := strings.IndexByte(raw, '#'); i >= 0 {
		raw = raw[:i]
	}
	rest, ok := strings.CutPrefix(raw, DeepLinkScheme+"://")
	if !ok {
		return DeepLink{}, false
	}
	location, query, _ := strings.Cut(rest, "?")
	host, path, _ := strings.Cut(location, "/")
	if host != DeepLinkHost || path != "" {
		return DeepLink{}, false
	}
	items := strings.Split(query, "&")
	if len(items) != 2 {
		return DeepLink{}, false
	}
	values := map[string]string{}
	for _, item := range items {
		name, value, hasValue := strings.Cut(item, "=")
		if !hasValue {
			return DeepLink{}, false
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return DeepLink{}, false
		}
		if _, dup := values[name]; dup {
			return DeepLink{}, false
		}
		values[name] = decoded
	}
	link := DeepLink{Provider: ProviderScope(values["provider"]), Section: Section(values["section"])}
	if !link.Provider.Valid() || !link.Section.Valid() {
		return DeepLink{}, false
	}
	return link, true
}

// Requirements is the set of data a widget needs the app to collect.
type Requirements int

const (
	RequireProviderQuota Requirements = 1 << iota
	RequireTokenTrend
	RequireCost
	RequireResetToday

	// RequireAll is every kind of widget data.
	RequireAll = RequireProviderQuota | RequireTokenTrend | RequireCost | RequireResetToday
)

// Has reports whether every bit of r2 is set.
func (r Requirements) Has(r2 Requirements) bool { return r&r2 == r2 }

// RequirementsFor is the data one widget kind and family needs.
func RequirementsFor(kind Kind, family Family) Requirements {
	switch kind {
	case KindOverview:
		if family == FamilyLarge {
			return RequireProviderQuota | RequireTokenTrend | RequireCost
		}
		return RequireProviderQuota
	case KindTokenTrend:
		return RequireTokenTrend
	case KindMetric:
		return RequireProviderQuota | RequireTokenTrend | RequireCost
	case KindResetToday:
		return RequireResetToday
	}
	return 0
}

// RequirementsForActive is the union over the widgets currently installed,
// assuming the largest family of each; unknown kinds are ignored.
func RequirementsForActive(activeKinds []string) Requirements {
	var out Requirements
	for _, raw := range activeKinds {
		out |= RequirementsFor(Kind(raw), FamilyLarge)
	}
	return out
}
