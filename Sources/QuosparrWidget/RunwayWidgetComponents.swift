import SwiftUI
import WidgetKit

extension View {
    @ViewBuilder
    func runwayWidgetPreviewUnredacted(_ isPlaceholder: Bool) -> some View {
        if isPlaceholder {
            unredacted()
        } else {
            self
        }
    }
}

enum RunwayWidgetPalette {
    static let token = Color(red: 0.18, green: 0.62, blue: 0.76)
    static let track = Color.primary.opacity(0.10)

    static func quota(remaining: Int) -> Color {
        if remaining >= 50 { return .green }
        if remaining >= 20 { return .yellow }
        return .red
    }
}

struct RunwayWidgetHeader: View {
    var title: String
    var trailing: String?

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "gauge.with.dots.needle.bottom.50percent")
                .foregroundStyle(RunwayWidgetPalette.token)
            Text(title)
                .font(.headline)
                .lineLimit(1)
            Spacer(minLength: 4)
            if let trailing {
                Text(trailing)
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(.secondary)
            }
        }
    }
}

struct RunwayQuotaRow: View {
    var quota: RunwayWidgetQuota
    var showsCountdown: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text(quota.title)
                    .font(.caption.weight(.medium))
                    .lineLimit(1)
                Spacer(minLength: 4)
                if showsCountdown, let reset = quota.resetsAt, reset > Date() {
                    Text(reset, style: .timer)
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
                Text(quota.valueText ?? "\(quota.remainingPercent)%")
                    .font(.caption.weight(.semibold).monospacedDigit())
            }
            GeometryReader { proxy in
                ZStack(alignment: .leading) {
                    Capsule().fill(RunwayWidgetPalette.track)
                    Capsule()
                        .fill(RunwayWidgetPalette.quota(remaining: quota.remainingPercent))
                        .frame(width: proxy.size.width * CGFloat(quota.remainingPercent) / 100)
                }
            }
            .frame(height: 5)
        }
    }
}

struct RunwayProviderLabel: View {
    var provider: RunwayProvider
    var plan: String?
    var l10n: L10n = .system

    var body: some View {
        HStack(spacing: 5) {
            Text(providerName(provider, l10n: l10n))
                .font(.caption.weight(.bold))
            if let plan, !plan.isEmpty {
                Text(plan)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
    }
}

struct RunwayTokenBars: View {
    var points: [RunwayWidgetDailyTokens]

    var body: some View {
        GeometryReader { proxy in
            let maximum = max(1, points.map(\.tokens).max() ?? 1)
            HStack(alignment: .bottom, spacing: max(1, proxy.size.width / CGFloat(max(1, points.count)) * 0.22)) {
                ForEach(points, id: \.date) { point in
                    RoundedRectangle(cornerRadius: 1.5, style: .continuous)
                        .fill(RunwayWidgetPalette.token)
                        .frame(
                            maxWidth: .infinity,
                            minHeight: point.tokens == 0 ? 1 : 3,
                            maxHeight: max(1, proxy.size.height * CGFloat(point.tokens) / CGFloat(maximum)))
                }
            }
            .frame(maxHeight: .infinity, alignment: .bottom)
        }
    }
}

struct RunwayWidgetFreshness: View {
    var date: Date
    var isStale: Bool
    var now: Date
    var l10n: L10n

    init(snapshot: RunwayWidgetSnapshot, now: Date, l10n: L10n) {
        date = snapshot.generatedAt
        isStale = snapshot.isStale(at: now)
        self.now = now
        self.l10n = l10n
    }

    init(date: Date, isStale: Bool, now: Date, l10n: L10n) {
        self.date = date
        self.isStale = isStale
        self.now = now
        self.l10n = l10n
    }

    var body: some View {
        HStack(spacing: 4) {
            if isStale {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(.yellow)
                Text(l10n.text(.widgetDataOld))
            } else {
                Text(l10n.text(.widgetUpdated))
            }
            Text(date, style: .relative)
        }
        .font(.caption2)
        .foregroundStyle(.secondary)
        .lineLimit(1)
    }
}

func runwayAvailabilityText(_ state: RunwayWidgetAvailability, l10n: L10n) -> String {
    switch state {
    case .notLoggedIn: l10n.text(.notLoggedIn)
    case .cliUnavailable: l10n.text(.grokCLIUnavailable)
    case .available: l10n.text(.available)
    case .unavailable: l10n.text(.statusUnknown)
    }
}

struct RunwayWidgetUnavailableView: View {
    var state: RunwayWidgetLoadState

    private var l10n: L10n { .system }

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            Image(systemName: icon)
                .font(.title2)
                .foregroundStyle(RunwayWidgetPalette.token)
            Text("Quosparr")
                .font(.headline)
            Text(message)
                .font(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private var icon: String {
        switch state {
        case .corrupt: "exclamationmark.triangle"
        case .unsupportedVersion: "arrow.down.app"
        case .ready, .missing: "gauge.with.dots.needle.bottom.50percent"
        }
    }

    private var message: String {
        switch state {
        case .corrupt: l10n.text(.widgetDataCorrupt)
        case .unsupportedVersion: l10n.text(.widgetVersionUnsupported)
        case .ready, .missing: l10n.text(.widgetNoData)
        }
    }
}

extension RunwayWidgetProviderSnapshot {
    var primaryQuota: RunwayWidgetQuota? {
        quota.first(where: { $0.source == .standard && $0.windowMinutes == 300 })
            ?? quota.first(where: { $0.source == .standard })
            ?? quota.first
    }

    func tokenTotal(limit: Int) -> Int {
        dailyTokens.suffix(limit).reduce(0) { $0 + $1.tokens }
    }
}

func providerName(_ provider: RunwayProvider, l10n: L10n) -> String {
    switch provider {
    case .codex: l10n.text(.widgetProviderCodex)
    case .grok: l10n.text(.widgetProviderGrok)
    case .claude: l10n.text(.widgetProviderClaude)
    }
}

extension RunwayWidgetProviderScope {
    func label(l10n: L10n) -> String {
        switch self {
        case .codex: l10n.text(.widgetProviderCodex)
        case .grok: l10n.text(.widgetProviderGrok)
        case .claude: l10n.text(.widgetProviderClaude)
        case .both: l10n.text(.widgetProviderBoth)
        case .codexClaude: l10n.text(.widgetProviderCodexClaude)
        case .grokClaude: l10n.text(.widgetProviderGrokClaude)
        }
    }
}

extension WidgetFamily {
    var runwayFamily: RunwayWidgetFamily {
        switch self {
        case .systemSmall: .small
        case .systemLarge: .large
        default: .medium
        }
    }
}

func runwayCompactNumber(_ value: Int) -> String {
    let formatter = NumberFormatter()
    formatter.numberStyle = .decimal
    formatter.maximumFractionDigits = value >= 1_000_000 ? 1 : 0
    if value >= 1_000_000 {
        return "\(formatter.string(from: NSNumber(value: Double(value) / 1_000_000)) ?? "0")M"
    }
    if value >= 1_000 {
        return "\(formatter.string(from: NSNumber(value: Double(value) / 1_000)) ?? "0")K"
    }
    return formatter.string(from: NSNumber(value: value)) ?? "0"
}

func runwayMoney(_ value: Decimal?) -> String? {
    guard let value else { return nil }
    let formatter = NumberFormatter()
    formatter.numberStyle = .currency
    formatter.currencyCode = "USD"
    formatter.maximumFractionDigits = 2
    return formatter.string(from: value as NSDecimalNumber)
}

func runwayAPICost(_ provider: RunwayWidgetProviderSnapshot, l10n: L10n) -> String {
    guard let amount = runwayMoney(provider.apiEquivalentCostUSD) else { return "—" }
    return provider.apiEquivalentCostPartial == true ? amount + " · " + l10n.text(.widgetPartialCost) : amount
}
