import AppIntents
import Foundation

/// macOS 27 resolves widget `AppEnum` parameters to nil and keeps the
/// default, so a saved "both" renders as Codex. These ids stay plain
/// strings, which the system still passes through.
@available(macOS 14.0, *)
struct RunwayProviderOptions: DynamicOptionsProvider, Sendable {
    func results() async throws -> IntentItemCollection<String> {
        let l10n = L10n.system
        return IntentItemCollection(sections: [
            IntentItemSection(items: [
                IntentItem("codex", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderCodex))),
                IntentItem("grok", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderGrok))),
                IntentItem("claude", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderClaude))),
                IntentItem("gemini", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderGemini))),
                IntentItem("both", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderBoth))),
                IntentItem("codex-claude", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderCodexClaude))),
                IntentItem("grok-claude", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderGrokClaude))),
                IntentItem("codex-gemini", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderCodexGemini))),
                IntentItem("claude-gemini", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderClaudeGemini))),
                IntentItem("gemini-grok", title: LocalizedStringResource(stringLiteral: l10n.text(.widgetProviderGeminiGrok))),
            ]),
        ])
    }

    func defaultResult() async -> String? { "codex" }
}

@available(macOS 14.0, *)
struct RunwayMetricOptions: DynamicOptionsProvider, Sendable {
    func results() async throws -> IntentItemCollection<String> {
        IntentItemCollection(sections: [
            IntentItemSection(items: [
                IntentItem("remainingQuota", title: "Remaining quota"),
                IntentItem("apiEquivalentCost", title: "API equivalent cost"),
                IntentItem("tokenCount", title: "Token count"),
                IntentItem("balance", title: "Balance"),
            ]),
        ])
    }

    func defaultResult() async -> String? { "remainingQuota" }
}

@available(macOS 14.0, *)
struct RunwayProviderSelectionIntent: AppIntent, WidgetConfigurationIntent {
    static let title: LocalizedStringResource = "Provider"
    static let description = IntentDescription("Choose which provider the widget displays.")

    @Parameter(title: "Provider", optionsProvider: RunwayProviderOptions())
    var provider: String
}

@available(macOS 14.0, *)
struct RunwayMetricSelectionIntent: AppIntent, WidgetConfigurationIntent {
    static let title: LocalizedStringResource = "Key Metric"
    static let description = IntentDescription("Choose a provider and metric.")

    @Parameter(title: "Provider", optionsProvider: RunwayProviderOptions())
    var provider: String

    @Parameter(title: "Metric", optionsProvider: RunwayMetricOptions())
    var metric: String
}
