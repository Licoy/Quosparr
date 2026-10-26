import AppKit
import SwiftUI
import WidgetKit

@available(macOS 14.0, *)
struct RunwayOverviewWidgetView: View {
    @Environment(\.widgetFamily) private var family
    var entry: RunwayWidgetEntry

    var body: some View {
        Group {
            switch entry.state {
            case .ready(let snapshot): content(snapshot)
            case .missing, .corrupt, .unsupportedVersion: RunwayWidgetUnavailableView(state: entry.state)
            }
        }
        .runwayWidgetPreviewUnredacted(entry.isPlaceholder)
        .containerBackground(for: .widget) { Color(nsColor: .windowBackgroundColor) }
        .widgetURL(RunwayWidgetDeepLink(provider: entry.provider, section: .overview).url)
    }

    private func content(_ snapshot: RunwayWidgetSnapshot) -> some View {
        let l10n = L10n(language: snapshot.language)
        let providers = entry.provider.providers.compactMap(snapshot.provider)
        return VStack(alignment: .leading, spacing: family == .systemSmall ? 8 : 10) {
            RunwayWidgetHeader(title: l10n.text(.widgetOverviewTitle), trailing: entry.provider.label(l10n: l10n))
            if providers.isEmpty {
                Text(l10n.text(.widgetNoData)).font(.caption).foregroundStyle(.secondary)
            } else {
                ForEach(Array(providers.enumerated()), id: \.element.provider) { index, provider in
                    if index > 0 { Divider() }
                    providerQuota(provider, l10n: l10n)
                }
            }
            Spacer(minLength: 0)
            RunwayWidgetFreshness(snapshot: snapshot, now: entry.date, l10n: l10n)
        }
    }

    @ViewBuilder
    private func providerQuota(_ provider: RunwayWidgetProviderSnapshot, l10n: L10n) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            RunwayProviderLabel(provider: provider.provider, plan: provider.plan, l10n: l10n)
            if provider.availability != .available {
                Text(runwayAvailabilityText(provider.availability, l10n: l10n))
                    .font(.caption).foregroundStyle(.secondary)
            } else if provider.quota.isEmpty {
                Text(l10n.text(.widgetNoData))
                    .font(.caption).foregroundStyle(.secondary)
            } else {
                let quota = RunwayWidgetLayoutPolicy.quotaLimit(for: family.runwayFamily)
                    .map { Array(provider.quota.prefix($0)) }
                    ?? provider.quota
                ForEach(Array(quota.enumerated()), id: \.offset) { _, meter in
                    RunwayQuotaRow(quota: meter, showsCountdown: family != .systemSmall)
                }
                if family == .systemLarge {
                    HStack(spacing: 14) {
                        extra(l10n.text(.widgetCost), runwayAPICost(provider, l10n: l10n))
                        extra(l10n.text(.tokens), runwayCompactNumber(provider.tokenTotal(limit: 30)))
                        extra(l10n.text(.widgetBalance), provider.balanceText ?? runwayMoney(provider.balanceUSD) ?? "—")
                        if let resets = provider.resetCredits {
                            extra(l10n.text(.widgetResetCredits), "\(resets.availableCount)")
                        }
                    }
                }
            }
        }
    }

    private func extra(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 1) {
            Text(title).font(.caption2).foregroundStyle(.secondary).lineLimit(1)
            Text(value).font(.caption.weight(.semibold).monospacedDigit()).lineLimit(1)
        }
    }

}

@available(macOS 14.0, *)
struct RunwayTokenTrendWidgetView: View {
    @Environment(\.widgetFamily) private var family
    var entry: RunwayWidgetEntry

    var body: some View {
        Group {
            switch entry.state {
            case .ready(let snapshot): content(snapshot)
            case .missing, .corrupt, .unsupportedVersion: RunwayWidgetUnavailableView(state: entry.state)
            }
        }
        .runwayWidgetPreviewUnredacted(entry.isPlaceholder)
        .containerBackground(for: .widget) { Color(nsColor: .windowBackgroundColor) }
        .widgetURL(RunwayWidgetDeepLink(provider: entry.provider, section: .tokens).url)
    }

    private func content(_ snapshot: RunwayWidgetSnapshot) -> some View {
        let l10n = L10n(language: snapshot.language)
        let limit = RunwayWidgetLayoutPolicy.trendDays(for: family.runwayFamily)
        let providers = entry.provider.providers.compactMap(snapshot.provider)
        return VStack(alignment: .leading, spacing: 9) {
            RunwayWidgetHeader(title: l10n.text(.widgetTokenTrendTitle), trailing: "\(limit)d")
            ForEach(Array(providers.enumerated()), id: \.element.provider) { index, provider in
                if index > 0 { Divider() }
                VStack(alignment: .leading, spacing: 4) {
                    HStack {
                        RunwayProviderLabel(provider: provider.provider, plan: nil, l10n: l10n)
                        Spacer()
                        Text(source(provider.tokenSource, l10n: l10n))
                            .font(.caption2).foregroundStyle(.secondary)
                        Text(runwayCompactNumber(provider.tokenTotal(limit: limit)))
                            .font(.caption.weight(.semibold).monospacedDigit())
                    }
                    let points = Array(provider.dailyTokens.suffix(limit))
                    if provider.availability != .available {
                        Text(runwayAvailabilityText(provider.availability, l10n: l10n))
                            .font(.caption).foregroundStyle(.secondary)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                    } else if points.isEmpty {
                        Text(l10n.text(.tokenUsageHeatmapUnavailable))
                            .font(.caption).foregroundStyle(.secondary)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
                    } else {
                        RunwayTokenBars(points: points)
                            .frame(minHeight: providers.count > 1 ? 32 : 52)
                    }
                }
            }
            Spacer(minLength: 0)
            RunwayWidgetFreshness(snapshot: snapshot, now: entry.date, l10n: l10n)
        }
    }

    private func source(_ source: RunwayWidgetTokenSource, l10n: L10n) -> String {
        source == .allDevices ? l10n.text(.widgetAllDevices) : l10n.text(.widgetThisMac)
    }
}

@available(macOS 14.0, *)
struct RunwayMetricWidgetView: View {
    var entry: RunwayWidgetEntry

    var body: some View {
        Group {
            switch entry.state {
            case .ready(let snapshot): content(snapshot)
            case .missing, .corrupt, .unsupportedVersion: RunwayWidgetUnavailableView(state: entry.state)
            }
        }
        .runwayWidgetPreviewUnredacted(entry.isPlaceholder)
        .containerBackground(for: .widget) { Color(nsColor: .windowBackgroundColor) }
        .widgetURL(RunwayWidgetDeepLink(provider: entry.provider, section: section).url)
    }

    private func content(_ snapshot: RunwayWidgetSnapshot) -> some View {
        let l10n = L10n(language: snapshot.language)
        let providers = entry.provider.providers.compactMap(snapshot.provider)
        return VStack(alignment: .leading, spacing: 9) {
            RunwayWidgetHeader(title: metricTitle(l10n), trailing: nil)
            ForEach(Array(providers.enumerated()), id: \.element.provider) { index, provider in
                if index > 0 { Divider() }
                HStack(alignment: .firstTextBaseline) {
                    Text(providerName(provider.provider, l10n: l10n))
                        .font(.caption.weight(.semibold))
                    Spacer()
                    VStack(alignment: .trailing, spacing: 1) {
                        Text(metricValue(provider, l10n: l10n))
                            .font(providers.count == 1 ? .title.weight(.bold) : .title3.weight(.bold))
                            .monospacedDigit()
                            .foregroundStyle(metricColor(provider))
                            .lineLimit(1)
                            .minimumScaleFactor(0.7)
                        if provider.availability != .available {
                            Text(runwayAvailabilityText(provider.availability, l10n: l10n))
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                    }
                }
            }
            Spacer(minLength: 0)
            RunwayWidgetFreshness(snapshot: snapshot, now: entry.date, l10n: l10n)
        }
    }

    private var section: RunwayWidgetSection {
        switch entry.metric {
        case .remainingQuota, .balance: .quota
        case .apiEquivalentCost: .cost
        case .tokenCount: .tokens
        }
    }

    private func metricTitle(_ l10n: L10n) -> String {
        switch entry.metric {
        case .remainingQuota: l10n.text(.widgetRemaining)
        case .apiEquivalentCost: l10n.text(.widgetCost)
        case .tokenCount: l10n.text(.tokens)
        case .balance: l10n.text(.widgetBalance)
        }
    }

    private func metricValue(_ provider: RunwayWidgetProviderSnapshot, l10n: L10n) -> String {
        guard provider.availability == .available else { return "—" }
        return switch entry.metric {
        case .remainingQuota: provider.primaryQuota.map { $0.valueText ?? "\($0.remainingPercent)%" } ?? "—"
        case .apiEquivalentCost: runwayAPICost(provider, l10n: l10n)
        case .tokenCount: runwayCompactNumber(provider.tokenTotal(limit: 30))
        case .balance: provider.balanceText ?? runwayMoney(provider.balanceUSD) ?? "—"
        }
    }

    private func metricColor(_ provider: RunwayWidgetProviderSnapshot) -> Color {
        guard entry.metric == .remainingQuota, let quota = provider.primaryQuota else {
            return entry.metric == .tokenCount ? RunwayWidgetPalette.token : .primary
        }
        return RunwayWidgetPalette.quota(remaining: quota.remainingPercent)
    }
}

@available(macOS 14.0, *)
struct RunwayResetTodayWidgetView: View {
    @Environment(\.widgetFamily) private var family
    var entry: RunwayWidgetEntry
    var familyOverride: WidgetFamily? = nil

    private var displayFamily: WidgetFamily { familyOverride ?? family }

    var body: some View {
        Group {
            switch entry.state {
            case .ready(let snapshot): content(snapshot)
            case .missing, .corrupt, .unsupportedVersion: RunwayWidgetUnavailableView(state: entry.state)
            }
        }
        .runwayWidgetPreviewUnredacted(entry.isPlaceholder)
        .containerBackground(for: .widget) { Color(nsColor: .windowBackgroundColor) }
        .widgetURL(RunwayWidgetDeepLink(provider: .codex, section: .resetToday).url)
    }

    private func content(_ snapshot: RunwayWidgetSnapshot) -> some View {
        let l10n = L10n(language: snapshot.language)
        return VStack(alignment: .leading, spacing: displayFamily == .systemSmall ? 5 : 8) {
            HStack(spacing: 6) {
                Image(systemName: "gauge.with.dots.needle.bottom.50percent")
                    .foregroundStyle(RunwayWidgetPalette.token)
                Text(l10n.text(.widgetResetTodayTitle))
                    .font(.headline)
                    .lineLimit(2)
                    .minimumScaleFactor(0.72)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 0)
            }
            if let reset = snapshot.resetToday {
                if let presentation = reset.presentation(at: entry.date) {
                    Text(presentation.reason.questionText(l10n: l10n))
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                        .minimumScaleFactor(0.75)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(stateText(presentation, l10n: l10n))
                        .font(.system(
                            size: displayFamily == .systemSmall ? 34 : 42,
                            weight: .bold,
                            design: .rounded))
                        .foregroundStyle(stateColor(presentation))
                    if let resetType = presentation.resetType {
                        Text(resetType.localizedName(l10n: l10n))
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(resetTypeColor(resetType))
                            .lineLimit(2)
                            .minimumScaleFactor(0.75)
                    }
                    if let hint = semanticHint(presentation, l10n: l10n) {
                        Text(hint)
                            .font(.caption2.weight(.medium))
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                            .minimumScaleFactor(0.75)
                            .accessibilityLabel(
                                Text(semanticAccessibility(presentation, l10n: l10n)))
                    }
                    if let next = presentation.nextScheduledAt {
                        HStack(spacing: 5) {
                            if let resetType = presentation.nextScheduledResetType {
                                Image(systemName: "clock.arrow.circlepath")
                                    .foregroundStyle(.secondary)
                                Text(resetType.localizedName(l10n: l10n))
                                    .fontWeight(.semibold)
                                    .foregroundStyle(resetTypeColor(resetType))
                            } else {
                                Text(l10n.text(.rateLimitResetTodayNextScheduled))
                            }
                            Spacer(minLength: 4)
                            Text(next, style: .timer).monospacedDigit()
                        }
                        .font(.caption)
                        .lineLimit(2)
                        .minimumScaleFactor(0.75)
                        .accessibilityElement(children: .ignore)
                        .accessibilityLabel(
                            Text(nextScheduleAccessibilityLabel(
                                presentation.nextScheduledResetType,
                                l10n: l10n)))
                        .accessibilityValue(Text(next, style: .timer))
                    }
                } else {
                    Text(l10n.text(.widgetResetTodayRefreshRequired))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if displayFamily == .systemMedium, let checked = reset.lastSuccessfulCheckAt {
                    HStack(spacing: 4) {
                        Text(l10n.text(.rateLimitResetTodayLastCheck))
                        Text(checked, style: .relative)
                    }
                    .font(.caption2).foregroundStyle(.secondary)
                }
            } else {
                Text(l10n.text(.widgetNoData)).font(.caption).foregroundStyle(.secondary)
            }
            Spacer(minLength: 0)
            if let reset = snapshot.resetToday {
                let freshnessDate = reset.lastSuccessfulCheckAt ?? reset.fetchedAt
                RunwayWidgetFreshness(
                    date: freshnessDate,
                    isStale: entry.date.timeIntervalSince(freshnessDate) > RunwayWidgetSnapshot.staleAfter,
                    now: entry.date,
                    l10n: l10n)
            } else {
                RunwayWidgetFreshness(snapshot: snapshot, now: entry.date, l10n: l10n)
            }
        }
        .environment(\.locale, snapshot.language.locale)
    }

    private func stateText(
        _ presentation: RunwayWidgetResetTodayTimelineEntry,
        l10n: L10n) -> String
    {
        if presentation.reason == .unavailable {
            return l10n.text(.rateLimitResetUnavailable)
        }
        if let percent = presentation.confidencePercent {
            let key: L10nKey = percent == 100
                ? .rateLimitResetTodayPercentExact
                : .rateLimitResetTodayPercentPrefix
            return String(format: l10n.text(key), "\(percent)") + l10n.text(.rateLimitResetTodayYes)
        }
        switch presentation.state {
        case .yes: return l10n.text(.rateLimitResetTodayYes)
        case .no: return l10n.text(.rateLimitResetTodayNo)
        case .unknown: return l10n.text(.rateLimitResetUnavailable)
        }
    }

    private func stateColor(_ presentation: RunwayWidgetResetTodayTimelineEntry) -> Color {
        if let band = presentation.confidenceBand {
            return band == .ok ? Color(nsColor: .systemPurple) : Color(nsColor: .systemYellow)
        }
        switch presentation.state {
        case .yes:
            return presentation.resetType == .banked ? Color(nsColor: .systemBlue) : .green
        case .no:
            return .primary
        case .unknown:
            return Color(nsColor: .secondaryLabelColor)
        }
    }

    private func semanticHint(
        _ presentation: RunwayWidgetResetTodayTimelineEntry,
        l10n: L10n) -> String?
    {
        if presentation.reason == .grace {
            let awaiting = l10n.text(.widgetResetScheduleAwaitingConfirmation)
            guard presentation.scheduleBasis == .contextualInference else { return awaiting }
            return "\(l10n.text(.widgetResetScheduleInferred)) · \(awaiting)"
        }
        guard presentation.reason == .upcoming || presentation.nextScheduledAt != nil else {
            return nil
        }
        return l10n.text(
            presentation.scheduleBasis == .contextualInference
                ? .widgetResetScheduleInferred
                : .widgetResetScheduleExplicit)
    }

    private func semanticAccessibility(
        _ presentation: RunwayWidgetResetTodayTimelineEntry,
        l10n: L10n) -> String
    {
        if presentation.reason == .grace {
            return l10n.text(
                presentation.scheduleBasis == .contextualInference
                    ? .widgetResetScheduleInferredGraceAccessibility
                    : .widgetResetScheduleGraceAccessibility)
        }
        if presentation.scheduleBasis == .contextualInference {
            return l10n.text(.widgetResetScheduleInferredAccessibility)
        }
        return l10n.text(.widgetResetScheduleExplicit)
    }

    private func resetTypeColor(_ resetType: RateLimitResetType) -> Color {
        switch resetType {
        case .global: Color(nsColor: .systemGreen)
        case .banked: Color(nsColor: .systemBlue)
        case .globalAndBanked: Color(nsColor: .systemPurple)
        }
    }

    private func nextScheduleAccessibilityLabel(
        _ resetType: RateLimitResetType?,
        l10n: L10n
    ) -> String {
        let label = l10n.text(.rateLimitResetTodayNextScheduled)
        guard let resetType else { return label }
        return "\(label) · \(resetType.localizedName(l10n: l10n))"
    }
}
