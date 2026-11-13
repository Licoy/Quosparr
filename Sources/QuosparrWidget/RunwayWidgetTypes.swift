import Foundation

// Value types the snapshot shares with the main app; their raw values are
// part of widget snapshot schema 1.

public enum RunwayProvider: String, Codable, CaseIterable, Sendable {
    case codex
    case grok
    case claude
    case gemini
}

public enum ResolvedLanguage: String, CaseIterable, Codable, Sendable, Equatable {
    case english
    case simplifiedChinese
    case traditionalChinese
    case korean
    case japanese
    case russian
    case french
}

public enum RateLimitResetType: String, Codable, Sendable, Equatable {
    case global
    case banked
    case globalAndBanked = "global_and_banked"
}

public enum RateLimitResetTodayConfidenceBand: String, Codable, Sendable, Equatable {
    case ok
    case warn
}

public enum RateLimitResetTodayVerdictReason: String, Codable, Sendable, Equatable {
    case completed
    case upcoming
    case grace
    case expiredUnconfirmed
    case none
    case unavailable
}

public enum RateLimitResetScheduleBasis: String, Codable, Sendable, Equatable {
    case explicit
    case contextualInference = "contextual_inference"
}

extension ResolvedLanguage {
    var locale: Locale {
        switch self {
        case .english: Locale(identifier: "en_US")
        case .simplifiedChinese: Locale(identifier: "zh_Hans_CN")
        case .traditionalChinese: Locale(identifier: "zh_Hant_TW")
        case .korean: Locale(identifier: "ko_KR")
        case .japanese: Locale(identifier: "ja_JP")
        case .russian: Locale(identifier: "ru_RU")
        case .french: Locale(identifier: "fr_FR")
        }
    }
}

extension RateLimitResetType {
    func localizedName(l10n: L10n) -> String {
        switch self {
        case .global: l10n.text(.rateLimitResetTypeGlobal)
        case .banked: l10n.text(.rateLimitResetTypeBanked)
        case .globalAndBanked: l10n.text(.rateLimitResetTypeGlobalAndBanked)
        }
    }
}

extension RateLimitResetTodayVerdictReason {
    func questionText(l10n: L10n) -> String {
        switch self {
        case .upcoming: l10n.text(.rateLimitResetQuestionUpcoming)
        case .grace: l10n.text(.rateLimitResetQuestionScheduledGrace)
        case .unavailable: l10n.text(.rateLimitResetQuestionUnavailable)
        case .completed, .expiredUnconfirmed, .none: l10n.text(.rateLimitResetQuestionCurrent)
        }
    }
}
