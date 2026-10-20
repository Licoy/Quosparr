package app

const (
	xCacheTitle               textKey = "cacheAnalysis"
	xCacheModuleDescription   textKey = "cacheAnalysisDescription"
	xCacheHitRate             textKey = "cacheHitRate"
	xCacheTokenRatio          textKey = "cacheTokenRatio"
	xCacheLocalSource         textKey = "cacheLocalSource"
	xCacheRangeSource         textKey = "cacheRangeSource"
	xCacheSingleDaySource     textKey = "cacheSingleDaySource"
	xCacheMissingSamples      textKey = "cacheMissingSamples"
	xCacheNoData              textKey = "cacheNoData"
	xCacheByModel             textKey = "cacheByModel"
	xCacheBySession           textKey = "cacheBySession"
	xCacheModels              textKey = "cacheModels"
	xCacheSessions            textKey = "cacheSessions"
	xCacheCachedInput         textKey = "cacheCachedInput"
	xCacheAllInput            textKey = "cacheAllInput"
	xCacheSamples             textKey = "cacheSamples"
	xCacheName                textKey = "cacheName"
	xCacheCachedAll           textKey = "cacheCachedAll"
	xCacheUnknown             textKey = "cacheUnknown"
	xCacheViewDetails         textKey = "cacheViewDetails"
	xCacheCodexCycleMissing   textKey = "cacheCodexCycleMissing"
	xCacheClaudeCycleMissing  textKey = "cacheClaudeCycleMissing"
	xCacheProviderUnavailable textKey = "cacheProviderUnavailable"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xCacheTitle: {
			"Cache hit analysis", "缓存命中分析", "快取命中分析", "캐시 적중 분석", "キャッシュヒット分析", "Анализ попаданий в кэш", "Analyse des accès au cache",
		},
		xCacheModuleDescription: {
			"Cache-read share of input tokens in local sessions.", "本机会话输入 Token 的缓存读取占比。", "本機工作階段輸入 Token 的快取讀取占比。", "로컬 세션 입력 토큰 중 캐시 읽기 비율입니다.", "ローカルセッションの入力トークンに占めるキャッシュ読み取りの割合。", "Доля чтений из кэша во входных токенах локальных сеансов.", "Part des lectures du cache dans les jetons d’entrée des sessions locales.",
		},
		xCacheHitRate:             {"Hit rate", "命中率", "命中率", "적중률", "ヒット率", "Доля попаданий", "Taux d’accès"},
		xCacheTokenRatio:          {"Cached input %s / all input %s", "缓存读取 %s / 全部输入 %s", "快取讀取 %s / 全部輸入 %s", "캐시 입력 %s / 전체 입력 %s", "キャッシュ入力 %s / 全入力 %s", "Кэшированный ввод %s / весь ввод %s", "Entrée en cache %s / entrée totale %s"},
		xCacheLocalSource:         {"Local session logs", "本机会话日志", "本機工作階段日誌", "로컬 세션 로그", "ローカルセッションログ", "Локальные журналы сеансов", "Journaux de sessions locales"},
		xCacheRangeSource:         {"%s – %s · Local session logs", "%s – %s · 本机会话日志", "%s – %s · 本機工作階段日誌", "%s – %s · 로컬 세션 로그", "%s – %s · ローカルセッションログ", "%s – %s · Локальные журналы сеансов", "%s – %s · Journaux de sessions locales"},
		xCacheSingleDaySource:     {"%s · Local session logs", "%s · 本机会话日志", "%s · 本機工作階段日誌", "%s · 로컬 세션 로그", "%s · ローカルセッションログ", "%s · Локальные журналы сеансов", "%s · Journaux de sessions locales"},
		xCacheMissingSamples:      {"Excluded %d records without cache token data", "已排除 %d 条缺少缓存 Token 数据的记录", "已排除 %d 筆缺少快取 Token 資料的記錄", "캐시 토큰 데이터가 없는 기록 %d개를 제외했습니다", "キャッシュトークンデータがない記録 %d 件を除外しました", "Исключено записей без данных о токенах кэша: %d", "%d enregistrements sans données de jetons en cache ont été exclus"},
		xCacheNoData:              {"No cache data", "无缓存数据", "無快取資料", "캐시 데이터 없음", "キャッシュデータなし", "Нет данных кэша", "Aucune donnée de cache"},
		xCacheByModel:             {"By model", "按模型", "依模型", "모델별", "モデル別", "По моделям", "Par modèle"},
		xCacheBySession:           {"By session", "按会话", "依工作階段", "세션별", "セッション別", "По сеансам", "Par session"},
		xCacheModels:              {"Models", "模型", "模型", "모델", "モデル", "Модели", "Modèles"},
		xCacheSessions:            {"Sessions", "会话", "工作階段", "세션", "セッション", "Сеансы", "Sessions"},
		xCacheCachedInput:         {"Cached input", "缓存读取", "快取讀取", "캐시 입력", "キャッシュ入力", "Кэшированный ввод", "Entrée en cache"},
		xCacheAllInput:            {"All input", "全部输入", "全部輸入", "전체 입력", "全入力", "Весь ввод", "Entrée totale"},
		xCacheSamples:             {"Valid records", "有效记录", "有效記錄", "유효한 기록", "有効な記録", "Корректные записи", "Enregistrements valides"},
		xCacheName:                {"Name", "名称", "名稱", "이름", "名前", "Название", "Nom"},
		xCacheCachedAll:           {"Cached / input", "缓存 / 输入", "快取 / 輸入", "캐시 / 입력", "キャッシュ / 入力", "Кэш / ввод", "Cache / entrée"},
		xCacheUnknown:             {"Unknown", "未知", "未知", "알 수 없음", "不明", "Неизвестно", "Inconnu"},
		xCacheViewDetails:         {"View cache details", "查看缓存详情", "查看快取詳情", "캐시 세부 정보 보기", "キャッシュの詳細を表示", "Подробнее о кэше", "Voir les détails du cache"},
		xCacheCodexCycleMissing:   {"Codex subscription cycle unavailable; choose Today, This month, or Custom", "Codex 订阅周期暂不可用，请选择今日、本月或自定义", "Codex 訂閱週期暫不可用，請選擇今日、本月或自訂", "Codex 구독 주기를 사용할 수 없습니다. 오늘, 이번 달 또는 사용자 지정을 선택하세요", "Codex のサブスクリプション期間を取得できません。今日、今月、カスタムを選択してください", "Период подписки Codex недоступен; выберите сегодня, месяц или свой период", "Période d’abonnement Codex indisponible ; choisissez aujourd’hui, ce mois ou personnalisé"},
		xCacheClaudeCycleMissing:  {"Claude subscription cycle unavailable; choose Today, This month, or Custom", "Claude 订阅周期暂不可用，请选择今日、本月或自定义", "Claude 訂閱週期暫不可用，請選擇今日、本月或自訂", "Claude 구독 주기를 사용할 수 없습니다. 오늘, 이번 달 또는 사용자 지정을 선택하세요", "Claude のサブスクリプション期間を取得できません。今日、今月、カスタムを選択してください", "Период подписки Claude недоступен; выберите сегодня, месяц или свой период", "Période d’abonnement Claude indisponible ; choisissez aujourd’hui, ce mois ou personnalisé"},
		xCacheProviderUnavailable: {"Cache analysis is unavailable for this provider", "此供应商不支持缓存分析", "此供應商不支援快取分析", "이 공급자에서는 캐시 분석을 사용할 수 없습니다", "このプロバイダーではキャッシュ分析を利用できません", "Анализ кэша недоступен для этого поставщика", "L’analyse du cache n’est pas disponible pour ce fournisseur"},
	} {
		extraTexts[key] = texts
	}
}
