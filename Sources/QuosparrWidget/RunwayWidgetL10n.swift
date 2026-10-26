import Foundation

// The strings the widget shows, copied from the main app's tables in
// internal/l10n; keep the seven languages in sync when one changes.

struct L10n: Sendable {
    var language: ResolvedLanguage

    init(language: ResolvedLanguage) {
        self.language = language
    }

    /// The first supported language of the system, as the app resolves
    /// the "system" preference.
    static var system: L10n {
        L10n(language: resolve(preferredLanguages: Locale.preferredLanguages))
    }

    static func resolve(preferredLanguages: [String]) -> ResolvedLanguage {
        for identifier in preferredLanguages {
            if let resolved = language(forLocale: identifier) {
                return resolved
            }
        }
        return .english
    }

    static func language(forLocale identifier: String) -> ResolvedLanguage? {
        let parts = identifier.replacingOccurrences(of: "_", with: "-").lowercased()
            .split(separator: "-").map(String.init)
        guard let language = parts.first, !language.isEmpty else { return nil }
        if language == "zh" {
            if parts.contains("hant") {
                return .traditionalChinese
            }
            if parts.contains("hans") {
                return .simplifiedChinese
            }
            if parts.contains(where: { ["tw", "hk", "mo"].contains($0) }) {
                return .traditionalChinese
            }
            return .simplifiedChinese
        }
        switch language {
        case "ko": return .korean
        case "ja": return .japanese
        case "ru": return .russian
        case "fr": return .french
        case "en": return .english
        default: return nil
        }
    }

    func text(_ key: L10nKey) -> String {
        Self.table(for: language)[key] ?? Self.en[key] ?? key.rawValue
    }

    static func table(for language: ResolvedLanguage) -> [L10nKey: String] {
        switch language {
        case .english: en
        case .simplifiedChinese: zhHans
        case .traditionalChinese: zhHant
        case .korean: ko
        case .japanese: ja
        case .russian: ru
        case .french: fr
        }
    }
}

enum L10nKey: String, CaseIterable, Sendable {
    case available
    case grokCLIUnavailable
    case notLoggedIn
    case rateLimitResetQuestionCurrent
    case rateLimitResetQuestionScheduledGrace
    case rateLimitResetQuestionUnavailable
    case rateLimitResetQuestionUpcoming
    case rateLimitResetTodayLastCheck
    case rateLimitResetTodayNextScheduled
    case rateLimitResetTodayNo
    case rateLimitResetTodayPercentExact
    case rateLimitResetTodayPercentPrefix
    case rateLimitResetTodayYes
    case rateLimitResetTypeBanked
    case rateLimitResetTypeGlobal
    case rateLimitResetTypeGlobalAndBanked
    case rateLimitResetUnavailable
    case statusUnknown
    case tokenUsageHeatmapUnavailable
    case tokens
    case widgetAllDevices
    case widgetBalance
    case widgetCost
    case widgetPartialCost
    case widgetDataCorrupt
    case widgetDataOld
    case widgetNoData
    case widgetOverviewTitle
    case widgetProviderBoth
    case widgetProviderClaude
    case widgetProviderCodex
    case widgetProviderCodexClaude
    case widgetProviderGrok
    case widgetProviderGrokClaude
    case widgetRemaining
    case widgetResetCredits
    case widgetResetScheduleAwaitingConfirmation
    case widgetResetScheduleExplicit
    case widgetResetScheduleGraceAccessibility
    case widgetResetScheduleInferred
    case widgetResetScheduleInferredAccessibility
    case widgetResetScheduleInferredGraceAccessibility
    case widgetResetTodayRefreshRequired
    case widgetResetTodayTitle
    case widgetThisMac
    case widgetTokenTrendTitle
    case widgetUpdated
    case widgetVersionUnsupported
}

extension L10n {
    static let en: [L10nKey: String] = [
        .available: "Available",
        .grokCLIUnavailable: "Grok CLI is not installed",
        .notLoggedIn: "Not logged in",
        .rateLimitResetQuestionCurrent: "Any new Codex reset today?",
        .rateLimitResetQuestionScheduledGrace: "Did Codex reset as scheduled?",
        .rateLimitResetQuestionUnavailable: "What's the Codex reset status?",
        .rateLimitResetQuestionUpcoming: "Will there be a Codex reset next?",
        .rateLimitResetTodayLastCheck: "Last check",
        .rateLimitResetTodayNextScheduled: "Expected next reset",
        .rateLimitResetTodayNo: "No",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "Yes",
        .rateLimitResetTypeBanked: "Reset bank",
        .rateLimitResetTypeGlobal: "Global reset",
        .rateLimitResetTypeGlobalAndBanked: "Global reset + reset bank",
        .rateLimitResetUnavailable: "Unavailable",
        .statusUnknown: "unknown",
        .tokenUsageHeatmapUnavailable: "Token usage is temporarily unavailable",
        .tokens: "Tokens",
        .widgetAllDevices: "All devices",
        .widgetBalance: "Balance",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "API equivalent",
        .widgetPartialCost: "Partial cost",
        .widgetDataCorrupt: "Widget data is damaged. Refresh Quosparr.",
        .widgetDataOld: "Data is old",
        .widgetNoData: "Open Quosparr to load widget data.",
        .widgetOverviewTitle: "Quota Overview",
        .widgetRemaining: "Remaining",
        .widgetResetCredits: "Resets",
        .widgetResetScheduleAwaitingConfirmation: "Awaiting confirmation",
        .widgetResetScheduleExplicit: "Explicit schedule",
        .widgetResetScheduleGraceAccessibility: "The scheduled time passed without X or operator confirmation; the reset may have happened, check your account to confirm",
        .widgetResetScheduleInferred: "Inferred preview",
        .widgetResetScheduleInferredAccessibility: "Inferred reset preview; the source did not explicitly promise a reset",
        .widgetResetScheduleInferredGraceAccessibility: "The inferred time passed without X or operator confirmation; the source did not explicitly promise a reset, check your account to confirm",
        .widgetResetTodayRefreshRequired: "Open Quosparr to refresh reset status",
        .widgetResetTodayTitle: "Codex Reset Updates",
        .widgetThisMac: "This Mac",
        .widgetTokenTrendTitle: "Token Trend",
        .widgetUpdated: "Updated",
        .widgetVersionUnsupported: "Update Quosparr to read this widget data.",
    ]

    static let zhHans: [L10nKey: String] = [
        .available: "可用",
        .grokCLIUnavailable: "未安装 Grok CLI",
        .notLoggedIn: "未登录",
        .rateLimitResetQuestionCurrent: "今天有新的 Codex 重置吗？",
        .rateLimitResetQuestionScheduledGrace: "Codex 已按计划重置了吗？",
        .rateLimitResetQuestionUnavailable: "Codex 重置状态如何？",
        .rateLimitResetQuestionUpcoming: "接下来会有 Codex 重置吗？",
        .rateLimitResetTodayLastCheck: "最近检查",
        .rateLimitResetTodayNextScheduled: "预计下次重置",
        .rateLimitResetTodayNo: "否",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "是",
        .rateLimitResetTypeBanked: "重置卡",
        .rateLimitResetTypeGlobal: "全局重置",
        .rateLimitResetTypeGlobalAndBanked: "全局重置 + 重置卡",
        .rateLimitResetUnavailable: "暂不可用",
        .statusUnknown: "未知",
        .tokenUsageHeatmapUnavailable: "Token 用量暂时不可用",
        .tokens: "Tokens",
        .widgetAllDevices: "所有设备",
        .widgetBalance: "余额",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "API 等价成本",
        .widgetPartialCost: "部分成本",
        .widgetDataCorrupt: "组件数据已损坏，请刷新 Quosparr。",
        .widgetDataOld: "数据已过期",
        .widgetNoData: "打开 Quosparr 以加载组件数据。",
        .widgetOverviewTitle: "额度总览",
        .widgetRemaining: "剩余额度",
        .widgetResetCredits: "重置次数",
        .widgetResetScheduleAwaitingConfirmation: "待确认",
        .widgetResetScheduleExplicit: "明确排期",
        .widgetResetScheduleGraceAccessibility: "计划时间已过，但尚无 X 或运营确认，可能已发生重置，请查看账户确认",
        .widgetResetScheduleInferred: "推断预告",
        .widgetResetScheduleInferredAccessibility: "推断性重置预告，原文未明确承诺重置",
        .widgetResetScheduleInferredGraceAccessibility: "推断时间已过，但尚无 X 或运营确认，原文未明确承诺重置，请查看账户确认",
        .widgetResetTodayRefreshRequired: "打开 Quosparr 以刷新重置状态",
        .widgetResetTodayTitle: "Codex 重置动态",
        .widgetThisMac: "本机",
        .widgetTokenTrendTitle: "Token 趋势",
        .widgetUpdated: "更新于",
        .widgetVersionUnsupported: "请更新 Quosparr 以读取组件数据。",
    ]

    static let zhHant: [L10nKey: String] = [
        .available: "可用",
        .grokCLIUnavailable: "未安裝 Grok CLI",
        .notLoggedIn: "未登入",
        .rateLimitResetQuestionCurrent: "今天有新的 Codex 重設嗎？",
        .rateLimitResetQuestionScheduledGrace: "Codex 已按計劃重設了嗎？",
        .rateLimitResetQuestionUnavailable: "Codex 重設狀態如何？",
        .rateLimitResetQuestionUpcoming: "接下來會有 Codex 重設嗎？",
        .rateLimitResetTodayLastCheck: "最近檢查",
        .rateLimitResetTodayNextScheduled: "預計下次重置",
        .rateLimitResetTodayNo: "否",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "是",
        .rateLimitResetTypeBanked: "重置卡",
        .rateLimitResetTypeGlobal: "全域重置",
        .rateLimitResetTypeGlobalAndBanked: "全域重置 + 重置卡",
        .rateLimitResetUnavailable: "暫不可用",
        .statusUnknown: "未知",
        .tokenUsageHeatmapUnavailable: "Token 用量暫時不可用",
        .tokens: "Tokens",
        .widgetAllDevices: "所有裝置",
        .widgetBalance: "餘額",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "API 等价成本",
        .widgetPartialCost: "部分成本",
        .widgetDataCorrupt: "小工具資料已損壞，請重新整理 Quosparr。",
        .widgetDataOld: "資料已過期",
        .widgetNoData: "開啟 Quosparr 以載入小工具資料。",
        .widgetOverviewTitle: "額度總覽",
        .widgetRemaining: "剩餘額度",
        .widgetResetCredits: "重置次數",
        .widgetResetScheduleAwaitingConfirmation: "待確認",
        .widgetResetScheduleExplicit: "明確排期",
        .widgetResetScheduleGraceAccessibility: "計劃時間已過，但尚無 X 或營運確認，可能已發生重設，請查看帳戶確認",
        .widgetResetScheduleInferred: "推斷預告",
        .widgetResetScheduleInferredAccessibility: "推斷性重設預告，原文未明確承諾重設",
        .widgetResetScheduleInferredGraceAccessibility: "推斷時間已過，但尚無 X 或營運確認，原文未明確承諾重設，請查看帳戶確認",
        .widgetResetTodayRefreshRequired: "開啟 Quosparr 以重新整理重設狀態",
        .widgetResetTodayTitle: "Codex 重設動態",
        .widgetThisMac: "本機",
        .widgetTokenTrendTitle: "Token 趨勢",
        .widgetUpdated: "更新于",
        .widgetVersionUnsupported: "請更新 Quosparr 以讀取小工具資料。",
    ]

    static let ko: [L10nKey: String] = [
        .available: "사용 가능",
        .grokCLIUnavailable: "Grok CLI가 설치되어 있지 않습니다",
        .notLoggedIn: "로그인되지 않음",
        .rateLimitResetQuestionCurrent: "오늘 새로운 Codex 초기화가 있었나요?",
        .rateLimitResetQuestionScheduledGrace: "Codex가 예정대로 초기화되었나요?",
        .rateLimitResetQuestionUnavailable: "Codex 초기화 상태는 어떤가요?",
        .rateLimitResetQuestionUpcoming: "다음 Codex 초기화가 있나요?",
        .rateLimitResetTodayLastCheck: "마지막 확인",
        .rateLimitResetTodayNextScheduled: "다음 예상 리셋",
        .rateLimitResetTodayNo: "아니요",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "예",
        .rateLimitResetTypeBanked: "리셋 뱅크",
        .rateLimitResetTypeGlobal: "전체 리셋",
        .rateLimitResetTypeGlobalAndBanked: "전체 리셋 + 리셋 뱅크",
        .rateLimitResetUnavailable: "일시적으로 사용할 수 없음",
        .statusUnknown: "알 수 없음",
        .tokenUsageHeatmapUnavailable: "토큰 사용량을 일시적으로 사용할 수 없습니다",
        .tokens: "Tokens",
        .widgetAllDevices: "모든 기기",
        .widgetBalance: "잔액",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "API 환산",
        .widgetPartialCost: "일부 비용",
        .widgetDataCorrupt: "위젯 데이터가 손상되었습니다. Quosparr를 새로고침하세요.",
        .widgetDataOld: "데이터가 오래됨",
        .widgetNoData: "위젯 데이터를 불러오려면 Quosparr를 여세요.",
        .widgetOverviewTitle: "할당량 개요",
        .widgetRemaining: "남음",
        .widgetResetCredits: "리셋",
        .widgetResetScheduleAwaitingConfirmation: "확인 대기",
        .widgetResetScheduleExplicit: "명시적 일정",
        .widgetResetScheduleGraceAccessibility: "예정 시각이 지났지만 X 또는 운영 확인이 없습니다. 이미 초기화됐을 수 있으니 계정을 확인하세요",
        .widgetResetScheduleInferred: "추론 예고",
        .widgetResetScheduleInferredAccessibility: "추론한 초기화 예고이며 원문은 초기화를 명시적으로 약속하지 않았습니다",
        .widgetResetScheduleInferredGraceAccessibility: "추론 시각이 지났지만 X 또는 운영 확인이 없습니다. 원문은 초기화를 명시적으로 약속하지 않았으니 계정을 확인하세요",
        .widgetResetTodayRefreshRequired: "Quosparr를 열어 초기화 상태 새로고침",
        .widgetResetTodayTitle: "Codex 초기화 업데이트",
        .widgetThisMac: "이 Mac",
        .widgetTokenTrendTitle: "토큰 추세",
        .widgetUpdated: "업데이트됨",
        .widgetVersionUnsupported: "이 위젯 데이터를 읽으려면 Quosparr를 업데이트하세요.",
    ]

    static let ja: [L10nKey: String] = [
        .available: "利用可能",
        .grokCLIUnavailable: "Grok CLI がインストールされていません",
        .notLoggedIn: "未サインイン",
        .rateLimitResetQuestionCurrent: "今日、新しい Codex リセットはあった？",
        .rateLimitResetQuestionScheduledGrace: "Codex は予定どおりリセットされた？",
        .rateLimitResetQuestionUnavailable: "Codex のリセット状況は？",
        .rateLimitResetQuestionUpcoming: "次に Codex リセットはある？",
        .rateLimitResetTodayLastCheck: "前回の確認",
        .rateLimitResetTodayNextScheduled: "次の予想リセット",
        .rateLimitResetTodayNo: "いいえ",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "はい",
        .rateLimitResetTypeBanked: "リセットバンク",
        .rateLimitResetTypeGlobal: "グローバルリセット",
        .rateLimitResetTypeGlobalAndBanked: "グローバルリセット + リセットバンク",
        .rateLimitResetUnavailable: "一時利用できません",
        .statusUnknown: "不明",
        .tokenUsageHeatmapUnavailable: "トークン使用量は一時的に利用できません",
        .tokens: "Tokens",
        .widgetAllDevices: "すべてのデバイス",
        .widgetBalance: "残高",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "API 換算",
        .widgetPartialCost: "一部の費用",
        .widgetDataCorrupt: "ウィジェットデータが壊れています。Quosparr を更新してください。",
        .widgetDataOld: "データが古いです",
        .widgetNoData: "ウィジェットデータを読み込むには Quosparr を開いてください。",
        .widgetOverviewTitle: "クォータ概要",
        .widgetRemaining: "残り",
        .widgetResetCredits: "リセット",
        .widgetResetScheduleAwaitingConfirmation: "確認待ち",
        .widgetResetScheduleExplicit: "明示的な予定",
        .widgetResetScheduleGraceAccessibility: "予定時刻を過ぎましたが X または運営の確認はありません。すでにリセットされた可能性があるため、アカウントを確認してください",
        .widgetResetScheduleInferred: "推定予告",
        .widgetResetScheduleInferredAccessibility: "推定によるリセット予告です。原文はリセットを明確に約束していません",
        .widgetResetScheduleInferredGraceAccessibility: "推定時刻を過ぎましたが X または運営の確認はありません。原文はリセットを明確に約束していないため、アカウントを確認してください",
        .widgetResetTodayRefreshRequired: "Quosparr を開いてリセット状況を更新",
        .widgetResetTodayTitle: "Codex リセット更新",
        .widgetThisMac: "この Mac",
        .widgetTokenTrendTitle: "トークンの推移",
        .widgetUpdated: "更新済み",
        .widgetVersionUnsupported: "このウィジェットデータを読むには Quosparr を更新してください。",
    ]

    static let ru: [L10nKey: String] = [
        .available: "Доступно",
        .grokCLIUnavailable: "Grok CLI не установлен",
        .notLoggedIn: "Не выполнен вход",
        .rateLimitResetQuestionCurrent: "Был ли сегодня новый сброс Codex?",
        .rateLimitResetQuestionScheduledGrace: "Сбросился ли Codex по расписанию?",
        .rateLimitResetQuestionUnavailable: "Каков статус сброса Codex?",
        .rateLimitResetQuestionUpcoming: "Будет ли следующий сброс Codex?",
        .rateLimitResetTodayLastCheck: "Последняя проверка",
        .rateLimitResetTodayNextScheduled: "Ожидаемый следующий сброс",
        .rateLimitResetTodayNo: "Нет",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "Да",
        .rateLimitResetTypeBanked: "Банк сбросов",
        .rateLimitResetTypeGlobal: "Глобальный сброс",
        .rateLimitResetTypeGlobalAndBanked: "Глобальный сброс + банк сбросов",
        .rateLimitResetUnavailable: "Недоступно",
        .statusUnknown: "неизвестно",
        .tokenUsageHeatmapUnavailable: "Использование токенов временно недоступно",
        .tokens: "Tokens",
        .widgetAllDevices: "Все устройства",
        .widgetBalance: "Баланс",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "Эквивалент API",
        .widgetPartialCost: "Частичная стоимость",
        .widgetDataCorrupt: "Данные виджета повреждены. Обновите Quosparr.",
        .widgetDataOld: "Данные устарели",
        .widgetNoData: "Откройте Quosparr, чтобы загрузить данные виджета.",
        .widgetOverviewTitle: "Обзор квоты",
        .widgetRemaining: "Осталось",
        .widgetResetCredits: "Сбросы",
        .widgetResetScheduleAwaitingConfirmation: "Ожидание подтверждения",
        .widgetResetScheduleExplicit: "Явное расписание",
        .widgetResetScheduleGraceAccessibility: "Запланированное время прошло без подтверждения в X или от операторов; сброс мог произойти, проверьте аккаунт",
        .widgetResetScheduleInferred: "Предполагаемый прогноз",
        .widgetResetScheduleInferredAccessibility: "Предполагаемый прогноз сброса; источник явно не обещает сброс",
        .widgetResetScheduleInferredGraceAccessibility: "Предполагаемое время прошло без подтверждения в X или от операторов; источник явно не обещает сброс, проверьте аккаунт",
        .widgetResetTodayRefreshRequired: "Откройте Quosparr, чтобы обновить статус сброса",
        .widgetResetTodayTitle: "Обновления сбросов Codex",
        .widgetThisMac: "Этот Mac",
        .widgetTokenTrendTitle: "Тренд токенов",
        .widgetUpdated: "Обновлено",
        .widgetVersionUnsupported: "Обновите Quosparr, чтобы читать эти данные виджета.",
    ]

    static let fr: [L10nKey: String] = [
        .available: "Disponible",
        .grokCLIUnavailable: "Le CLI Grok n’est pas installé",
        .notLoggedIn: "Non connecté",
        .rateLimitResetQuestionCurrent: "Une nouvelle réinitialisation Codex aujourd’hui ?",
        .rateLimitResetQuestionScheduledGrace: "Codex a-t-il été réinitialisé comme prévu ?",
        .rateLimitResetQuestionUnavailable: "Quel est l’état des réinitialisations Codex ?",
        .rateLimitResetQuestionUpcoming: "Une réinitialisation Codex est-elle prévue ensuite ?",
        .rateLimitResetTodayLastCheck: "Dernière vérification",
        .rateLimitResetTodayNextScheduled: "Prochaine réinit. prévue",
        .rateLimitResetTodayNo: "Non",
        .rateLimitResetTodayPercentExact: "%@%%",
        .rateLimitResetTodayPercentPrefix: "≥%@%%",
        .rateLimitResetTodayYes: "Oui",
        .rateLimitResetTypeBanked: "Banque de réinit.",
        .rateLimitResetTypeGlobal: "Réinit. globale",
        .rateLimitResetTypeGlobalAndBanked: "Réinit. globale + banque",
        .rateLimitResetUnavailable: "Indisponible",
        .statusUnknown: "inconnu",
        .tokenUsageHeatmapUnavailable: "L’usage des jetons est temporairement indisponible",
        .tokens: "Tokens",
        .widgetAllDevices: "Tous les appareils",
        .widgetBalance: "Solde",
        .widgetProviderBoth: "Codex + Grok",
        .widgetProviderClaude: "Claude",
        .widgetProviderCodex: "Codex",
        .widgetProviderCodexClaude: "Codex + Claude",
        .widgetProviderGrok: "Grok",
        .widgetProviderGrokClaude: "Grok + Claude",
        .widgetCost: "Équivalent API",
        .widgetPartialCost: "Coût partiel",
        .widgetDataCorrupt: "Les données du widget sont endommagées. Actualisez Quosparr.",
        .widgetDataOld: "Données anciennes",
        .widgetNoData: "Ouvrez Quosparr pour charger les données du widget.",
        .widgetOverviewTitle: "Aperçu des quotas",
        .widgetRemaining: "Restant",
        .widgetResetCredits: "Réinit.",
        .widgetResetScheduleAwaitingConfirmation: "En attente de confirmation",
        .widgetResetScheduleExplicit: "Horaire explicite",
        .widgetResetScheduleGraceAccessibility: "L’heure prévue est passée sans confirmation sur X ou de l’équipe ; la réinitialisation a peut-être eu lieu, vérifiez votre compte",
        .widgetResetScheduleInferred: "Aperçu déduit",
        .widgetResetScheduleInferredAccessibility: "Aperçu de réinitialisation déduit ; la source ne promet pas explicitement de réinitialisation",
        .widgetResetScheduleInferredGraceAccessibility: "L’heure déduite est passée sans confirmation sur X ou de l’équipe ; la source ne promet pas explicitement de réinitialisation, vérifiez votre compte",
        .widgetResetTodayRefreshRequired: "Ouvrez Quosparr pour actualiser l’état des réinitialisations",
        .widgetResetTodayTitle: "Actualités des réinitialisations Codex",
        .widgetThisMac: "Ce Mac",
        .widgetTokenTrendTitle: "Tendance des jetons",
        .widgetUpdated: "Mis à jour",
        .widgetVersionUnsupported: "Mettez à jour Quosparr pour lire ces données de widget.",
    ]
}
