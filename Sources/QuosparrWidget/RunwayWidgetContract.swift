import Foundation

public enum RunwayWidgetKind: String, Codable, CaseIterable, Sendable {
    case overview = "com.quosparr.app.widget.overview"
    case tokenTrend = "com.quosparr.app.widget.token-trend"
    case metric = "com.quosparr.app.widget.metric"
    case resetToday = "com.quosparr.app.widget.reset-today"
}

public enum RunwayWidgetProviderScope: String, Codable, CaseIterable, Sendable {
    case codex
    case grok
    case claude
    case both
    case codexClaude = "codex-claude"
    case grokClaude = "grok-claude"

    public var providers: [RunwayProvider] {
        switch self {
        case .codex: [.codex]
        case .grok: [.grok]
        case .claude: [.claude]
        case .both: [.codex, .grok]
        case .codexClaude: [.codex, .claude]
        case .grokClaude: [.grok, .claude]
        }
    }
}

public enum RunwayWidgetMetricKind: String, Codable, CaseIterable, Sendable {
    case remainingQuota
    case apiEquivalentCost
    case tokenCount
    case balance
}

public enum RunwayWidgetStorageMode: String, Codable, Sendable {
    case appGroup = "app-group"
    case localDevelopment = "local"

    public static let infoKey = "RunwayWidgetStorageMode"
    public static let appGroupInfoKey = "RunwayAppGroupID"

    /// Development and release builds default to the local snapshot. App Group
    /// access is opt-in because `containerURL(forSecurityApplicationGroupIdentifier:)`
    /// prompts for `kTCCServiceSystemPolicyAppData`.
    public static func mode(fromInfoValue value: String?) -> Self {
        value.flatMap(Self.init(rawValue:)) ?? .localDevelopment
    }
}

public enum RunwayWidgetFamily: String, Codable, CaseIterable, Sendable {
    case small
    case medium
    case large
}

public enum RunwayWidgetLayoutPolicy {
    public static let refreshInterval: TimeInterval = 15 * 60

    public static func supports(_ family: RunwayWidgetFamily, for kind: RunwayWidgetKind) -> Bool {
        switch kind {
        case .overview: true
        case .tokenTrend: family == .medium || family == .large
        case .metric: family == .small
        case .resetToday: family == .small || family == .medium
        }
    }

    public static func trendDays(for family: RunwayWidgetFamily) -> Int {
        family == .large ? 30 : 14
    }

    public static func quotaLimit(for family: RunwayWidgetFamily) -> Int? {
        switch family {
        case .small: 1
        case .medium: 2
        case .large: nil
        }
    }
}

public enum RunwayWidgetSection: String, Codable, CaseIterable, Sendable {
    case overview
    case quota
    case tokens
    case cost
    case resetToday
}

public struct RunwayWidgetDeepLink: Equatable, Sendable {
    public static let scheme = "quosparr"
    public static let host = "widget"

    public var provider: RunwayWidgetProviderScope
    public var section: RunwayWidgetSection

    public init(provider: RunwayWidgetProviderScope, section: RunwayWidgetSection) {
        self.provider = provider
        self.section = section
    }

    public init?(url: URL) {
        guard url.scheme == Self.scheme, url.host == Self.host,
              url.path.isEmpty || url.path == "/",
              let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.queryItems?.count == 2,
              let providerValue = components.queryItems?.first(where: { $0.name == "provider" })?.value,
              let sectionValue = components.queryItems?.first(where: { $0.name == "section" })?.value,
              let provider = RunwayWidgetProviderScope(rawValue: providerValue),
              let section = RunwayWidgetSection(rawValue: sectionValue)
        else { return nil }
        self.init(provider: provider, section: section)
    }

    public var url: URL {
        var components = URLComponents()
        components.scheme = Self.scheme
        components.host = Self.host
        components.queryItems = [
            URLQueryItem(name: "provider", value: provider.rawValue),
            URLQueryItem(name: "section", value: section.rawValue),
        ]
        return components.url!
    }
}

public struct RunwayWidgetRequirements: OptionSet, Sendable, Equatable {
    public let rawValue: Int

    public init(rawValue: Int) {
        self.rawValue = rawValue
    }

    public static let providerQuota = Self(rawValue: 1 << 0)
    public static let tokenTrend = Self(rawValue: 1 << 1)
    public static let cost = Self(rawValue: 1 << 2)
    public static let resetToday = Self(rawValue: 1 << 3)
    public static let allWidgetData: Self = [
        .providerQuota,
        .tokenTrend,
        .cost,
        .resetToday,
    ]

    public static func make(kind: RunwayWidgetKind, family: RunwayWidgetFamily) -> Self {
        switch kind {
        case .overview:
            return family == .large ? [.providerQuota, .tokenTrend, .cost] : .providerQuota
        case .tokenTrend:
            return .tokenTrend
        case .metric:
            return [.providerQuota, .tokenTrend, .cost]
        case .resetToday:
            return .resetToday
        }
    }

    public static func make(activeKinds: some Sequence<String>) -> Self {
        activeKinds.reduce(into: Self()) { result, rawKind in
            guard let kind = RunwayWidgetKind(rawValue: rawKind) else { return }
            result.formUnion(make(kind: kind, family: .large))
        }
    }
}
