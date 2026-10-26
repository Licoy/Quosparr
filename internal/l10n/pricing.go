package l10n

import "fmt"

func (l Localizer) PricingRule(key string, args ...any) string {
	return fmt.Sprintf(l.Pricing(key), args...)
}

// Pricing returns localized model-pricing settings copy.
func (l Localizer) Pricing(key string) string {
	v, ok := pricingStrings[key]
	if !ok {
		return key
	}
	i := 0
	for n, lang := range Languages() {
		if lang == l.Language() {
			i = n
			break
		}
	}
	return v[i]
}

var pricingStrings = map[string][7]string{
	"tab":                 {"Model prices", "模型价格", "模型價格", "모델 가격", "モデル価格", "Цены моделей", "Prix des modèles"},
	"auto":                {"Automatic sync", "自动同步", "自動同步", "자동 동기화", "自動同期", "Автосинхронизация", "Synchronisation automatique"},
	"interval":            {"Sync frequency", "同步频率", "同步頻率", "동기화 주기", "同期間隔", "Частота синхронизации", "Fréquence de synchronisation"},
	"custom":              {"Custom (minutes)", "自定义（分钟）", "自訂（分鐘）", "사용자 지정 (분)", "カスタム（分）", "Другое (минуты)", "Personnalisée (minutes)"},
	"refresh":             {"Sync now", "立即同步", "立即同步", "지금 동기화", "今すぐ同期", "Синхронизировать", "Synchroniser"},
	"search":              {"Search model name or ID", "搜索模型名称或 ID", "搜尋模型名稱或 ID", "모델 이름 또는 ID 검색", "モデル名または ID を検索", "Поиск по имени или ID", "Rechercher un nom ou ID"},
	"all":                 {"All", "全部", "全部", "전체", "すべて", "Все", "Tous"},
	"available":           {"Available", "可用", "可用", "사용 가능", "利用可能", "Доступные", "Disponibles"},
	"reference":           {"Reference only", "仅参考", "僅參考", "참고용", "参考のみ", "Справочные", "Référence"},
	"incomplete":          {"Incomplete", "不完整", "不完整", "불완전", "不完全", "Неполные", "Incomplets"},
	"builtin":             {"Built-in", "内置", "內建", "기본 제공", "内蔵", "Встроенные", "Intégrés"},
	"name":                {"Name", "名称", "名稱", "이름", "名前", "Название", "Nom"},
	"input":               {"Input", "输入", "輸入", "입력", "入力", "Вход", "Entrée"},
	"output":              {"Output", "输出", "輸出", "출력", "出力", "Выход", "Sortie"},
	"cacheRead":           {"Cache read", "缓存读取", "快取讀取", "캐시 읽기", "キャッシュ読取", "Чтение кеша", "Lecture du cache"},
	"cacheWrite":          {"Cache write", "缓存写入", "快取寫入", "캐시 쓰기", "キャッシュ書込", "Запись кеша", "Écriture du cache"},
	"unit":                {"USD per million tokens; token prices only, excluding image, video and other modality billing", "美元 / 百万 Token；仅列 Token 价格，不代表图像、视频等其他模态费用", "美元 / 百萬 Token；僅列 Token 價格，不代表圖像、影片等其他模態費用", "백만 토큰당 USD; 이미지, 동영상 등 다른 모달리티 요금 제외", "100万トークンあたりUSD；画像、動画など他のモダリティ料金は対象外", "USD за миллион токенов; без тарифов изображений, видео и других модальностей", "USD par million de tokens ; hors images, vidéos et autres modalités"},
	"empty":               {"No matching models", "没有匹配的模型", "沒有符合的模型", "일치하는 모델 없음", "一致するモデルなし", "Модели не найдены", "Aucun modèle correspondant"},
	"lastAttempt":         {"Last check", "上次检查", "上次檢查", "마지막 확인", "前回の確認", "Последняя проверка", "Dernière vérification"},
	"lastSuccess":         {"Last successful sync", "上次同步成功", "上次同步成功", "마지막 동기화 성공", "前回の同期成功", "Успешная синхронизация", "Dernière synchronisation réussie"},
	"lastChanged":         {"Last price change", "上次价格变化", "上次價格變更", "마지막 가격 변경", "前回の価格変更", "Изменение цен", "Dernier changement de prix"},
	"nextSync":            {"Next sync", "下次同步", "下次同步", "다음 동기화", "次回の同期", "Следующая синхронизация", "Prochaine synchronisation"},
	"version":             {"Version", "版本", "版本", "버전", "バージョン", "Версия", "Version"},
	"source":              {"Source", "数据来源", "資料來源", "출처", "データソース", "Источник", "Source"},
	"error":               {"Sync error", "同步错误", "同步錯誤", "동기화 오류", "同期エラー", "Ошибка синхронизации", "Erreur de synchronisation"},
	"invalid":             {"Enter a whole number of minutes between 15 and 10080", "请输入 15 至 10080 之间的整数分钟", "請輸入 15 至 10080 之間的整數分鐘", "15~10080 사이의 정수 분을 입력하세요", "15〜10080の整数（分）を入力してください", "Введите целое число минут от 15 до 10080", "Saisissez un nombre entier de minutes entre 15 et 10080"},
	"back":                {"Back to models", "返回模型列表", "返回模型清單", "모델 목록으로", "モデル一覧に戻る", "К списку моделей", "Retour aux modèles"},
	"previous":            {"Previous", "上一页", "上一頁", "이전", "前へ", "Назад", "Précédent"},
	"next":                {"Next", "下一页", "下一頁", "다음", "次へ", "Далее", "Suivant"},
	"missing":             {"— means missing; $0 means explicitly free", "— 表示缺价；$0 表示明确免费", "— 表示缺價；$0 表示明確免費", "— 는 가격 없음; $0 는 명시적 무료", "— は価格不明、$0 は明示的な無料", "— нет цены; $0 — явно бесплатно", "— prix absent ; $0 gratuit explicitement"},
	"applied":             {"Applied pricing", "实际采用价格", "實際採用價格", "적용 가격", "適用価格", "Применённые цены", "Tarifs appliqués"},
	"catalog":             {"Catalog reference pricing", "目录参考价格", "目錄參考價格", "카탈로그 참고 가격", "カタログ参考価格", "Справочные цены каталога", "Tarifs de référence du catalogue"},
	"ruleStandard":        {"Standard pricing", "标准价格", "標準價格", "표준 가격", "標準料金", "Стандартный тариф", "Tarif standard"},
	"ruleFastUnpriced":    {"Fast pricing unavailable; these tokens are unpriced", "缺少 Fast 档价格；这些 Token 未计价", "缺少 Fast 檔價格；這些 Token 未計價", "Fast 가격 없음; 이 토큰은 미산정", "Fast料金が不明なため、これらのトークンは未計上", "Тариф Fast отсутствует; эти токены не оценены", "Tarif Fast absent ; ces tokens ne sont pas chiffrés"},
	"ruleGeoUS":           {"US processing: standard prices × %s", "美国处理：标准价格 × %s", "美國處理：標準價格 × %s", "미국 처리: 표준 가격 × %s", "米国処理: 標準料金 × %s", "Обработка в США: стандартные цены × %s", "Traitement aux États-Unis : tarifs standard × %s"},
	"syncing":             {"Syncing…", "同步中…", "同步中…", "동기화 중…", "同期中…", "Синхронизация…", "Synchronisation…"},
	"everyMinutes":        {"Every %d minutes", "每 %d 分钟", "每 %d 分鐘", "%d분마다", "%d分ごと", "Каждые %d мин", "Toutes les %d min"},
	"everyHours":          {"Every %d hours", "每 %d 小时", "每 %d 小時", "%d시간마다", "%d時間ごと", "Каждые %d ч", "Toutes les %d h"},
	"partial":             {"Partial cost", "部分成本", "部分成本", "일부 비용", "一部の費用", "Частичная стоимость", "Coût partiel"},
	"unpriced":            {"Unpriced", "未定价", "未定價", "미산정", "未計上", "Без оценки", "Non chiffré"},
	"unknownModel":        {"Unknown model: %s", "未知模型：%s", "未知模型：%s", "알 수 없는 모델: %s", "不明なモデル: %s", "Неизвестная модель: %s", "Modèle inconnu : %s"},
	"missingPrice":        {"Required prices or billing rules missing: %s", "缺少必要费率或计费规则：%s", "缺少必要費率或計費規則：%s", "필수 가격 또는 과금 규칙 누락: %s", "必要な料金または課金ルールが不明: %s", "Нет нужных цен или правил тарификации: %s", "Tarifs ou règles nécessaires absents : %s"},
	"fastUnpriced":        {"Fast pricing unavailable: %s", "缺少 Fast 档价格：%s", "缺少 Fast 檔價格：%s", "Fast 가격 없음: %s", "Fast料金が不明: %s", "Нет тарифа Fast: %s", "Tarif Fast absent : %s"},
	"ambiguousTier":       {"Context tier boundary unclear: %s", "上下文阶梯边界不明：%s", "上下文階梯邊界不明：%s", "컨텍스트 구간 경계 불명확: %s", "コンテキスト段階の境界が不明: %s", "Неясная граница уровня контекста: %s", "Limite de palier de contexte incertaine : %s"},
	"unpricedCount":       {"Unpriced: %s tokens", "未定价：%s Token", "未定價：%s Token", "미산정: %s 토큰", "未計上: %sトークン", "Без оценки: %s токенов", "Non chiffrés : %s tokens"},
	"currentEstimate":     {"Historical usage is estimated again at current prices", "历史用量按当前价格重新估算", "歷史用量按目前價格重新估算", "과거 사용량을 현재 가격으로 다시 추정합니다", "過去の使用量を現在の料金で再推定", "История использования пересчитывается по текущим ценам", "Les usages historiques sont réestimés aux tarifs actuels"},
	"pricingIssues":       {"Unpriced usage details", "未定价明细", "未定價明細", "미산정 사용량 내역", "未計上の使用量", "Использование без оценки", "Détail des usages non chiffrés"},
	"scanIssues":          {"Scan notices", "扫描提示", "掃描提示", "스캔 알림", "スキャンの注意事項", "Замечания сканирования", "Remarques de lecture"},
	"scanMalformed":       {"Malformed log lines skipped: %s", "已跳过损坏日志行：%s", "已略過損壞日誌行：%s", "건너뛴 손상 로그 행: %s", "破損したログ行をスキップ: %s", "Пропущено повреждённых строк: %s", "Lignes de journal corrompues ignorées : %s"},
	"modelUnpriced":       {"%s — %s tokens unpriced", "%s — %s Token 未定价", "%s — %s Token 未定價", "%s — %s 토큰 미산정", "%s — %sトークン未計上", "%s — %s токенов без оценки", "%s — %s tokens non chiffrés"},
	"pricingSourceModel":  {"%s — %s", "%s — %s", "%s — %s", "%s — %s", "%s — %s", "%s — %s", "%s — %s"},
	"ruleLegacyLong":      {"Legacy long-context rates; the threshold is unverified and is not treated as an applicable rule", "历史长上下文费率；阈值未核实，不作为明确适用规则", "歷史長上下文費率；門檻未核實，不作為明確適用規則", "이전 긴 컨텍스트 요금; 임계값이 검증되지 않아 적용 규칙으로 취급하지 않음", "従来の長コンテキスト料金；閾値が未検証のため適用ルールとは扱いません", "Прежние тарифы длинного контекста; порог не проверен и не считается применимым правилом", "Anciens tarifs de long contexte ; seuil non vérifié, non traité comme règle applicable"},
	"catalogLimitations":  {"The catalog does not fully specify tier comparisons, cache TTL, Fast, regions or multipliers; usage is priced only when log and rule evidence is sufficient", "目录未完整说明阶梯比较符、缓存时长、Fast、地区或倍率；仅在日志与规则证据充分时计价", "目錄未完整說明階梯比較符、快取時長、Fast、地區或倍率；僅在日誌與規則證據充分時計價", "카탈로그에 구간 비교, 캐시 TTL, Fast, 지역 또는 배수가 완전히 명시되지 않음; 로그와 규칙 근거가 충분할 때만 계산", "カタログの段階比較、キャッシュTTL、Fast、地域、倍率は不完全なため、ログとルールの根拠が十分な場合のみ計上", "Каталог не полностью задаёт сравнения уровней, TTL кеша, Fast, регионы и множители; оценка выполняется только при достаточных данных логов и правил", "Le catalogue ne précise pas tous les seuils, TTL du cache, tarifs Fast, régions ou multiplicateurs ; calcul uniquement avec des preuves suffisantes dans les logs et règles"},
	"ruleCacheUnknownTTL": {"Cache write TTL unspecified", "缓存写入时长未说明", "快取寫入時長未說明", "캐시 쓰기 TTL 미지정", "キャッシュ書込TTLが不明", "TTL записи кеша не указан", "TTL d’écriture du cache non précisé"},
	"usingBuiltin":        {"Using built-in prices; not synced yet", "使用内置价格，尚未同步", "使用內建價格，尚未同步", "기본 가격 사용 중; 아직 동기화되지 않음", "内蔵料金を使用中、未同期", "Используются встроенные цены; синхронизации ещё не было", "Tarifs intégrés utilisés ; pas encore synchronisés"},
	"openSource":          {"Open pricing source", "打开价格来源", "開啟價格來源", "가격 출처 열기", "料金の出典を開く", "Открыть источник цен", "Ouvrir la source des tarifs"},
	"syncSettings":        {"Sync settings", "频率设置", "頻率設定", "동기화 설정", "同期設定", "Настройки синхронизации", "Réglages de synchronisation"},
	"shortUnit":           {"USD / million tokens", "美元 / 百万 Token", "美元 / 百萬 Token", "USD / 백만 토큰", "USD / 100万トークン", "USD / млн токенов", "USD / million de tokens"},
	"synced":              {"Prices synced", "价格已同步", "價格已同步", "가격 동기화됨", "料金を同期済み", "Цены синхронизированы", "Tarifs synchronisés"},
	"model":               {"Model", "模型", "模型", "모델", "モデル", "Модель", "Modèle"},
	"modelCount":          {"%d models", "%d 个模型", "%d 個模型", "모델 %d개", "%dモデル", "Моделей: %d", "%d modèles"},
	"globalSync":          {"Sync settings apply to all three providers", "同步设置适用于全部服务商", "同步設定適用於全部供應商", "동기화 설정은 세 공급자 모두에 적용됩니다", "同期設定は3つのプロバイダーすべてに適用", "Настройки синхронизации общие для трёх провайдеров", "Ces réglages s’appliquent aux trois fournisseurs"},
	"done":                {"Done", "完成", "完成", "완료", "完了", "Готово", "Terminé"},
	"tokenPrices":         {"Token prices", "Token 价格", "Token 價格", "토큰 가격", "トークン料金", "Цены токенов", "Tarifs des tokens"},
	"pricingOrigin":       {"Source and version", "来源与版本", "來源與版本", "출처 및 버전", "出典とバージョン", "Источник и версия", "Source et version"},
	"pricingRules":        {"Billing rules and notes", "计价规则与说明", "計價規則與說明", "과금 규칙 및 설명", "課金ルールと説明", "Правила расчёта и примечания", "Règles de calcul et remarques"},
	"ruleTTL":             {"Cache write (%s): %s", "缓存写入（%s）：%s", "快取寫入（%s）：%s", "캐시 쓰기 (%s): %s", "キャッシュ書込（%s）: %s", "Запись кеша (%s): %s", "Écriture du cache (%s) : %s"},
	"ruleTier":            {"Context %s: %s", "上下文 %s：%s", "上下文 %s：%s", "컨텍스트 %s: %s", "コンテキスト %s: %s", "Контекст %s: %s", "Contexte %s : %s"},
	"ruleFast":            {"Fast pricing: %s", "Fast 档价格：%s", "Fast 檔價格：%s", "Fast 가격: %s", "Fast料金: %s", "Тариф Fast: %s", "Tarif Fast : %s"},
	"ruleRegion":          {"Region %s: %s", "地区 %s：%s", "地區 %s：%s", "지역 %s: %s", "リージョン %s: %s", "Регион %s: %s", "Région %s : %s"},
	"ruleApplied":         {"Applied: %s; catalog reference: %s", "实际采用：%s；目录参考：%s", "實際採用：%s；目錄參考：%s", "적용: %s; 카탈로그 참고: %s", "適用: %s、カタログ参考: %s", "Применено: %s; каталог: %s", "Appliqué : %s ; référence du catalogue : %s"},
	"rulePreserved":       {"Built-in rules preserve context tiers, cache TTL, Fast and regional billing where the catalog does not describe them completely", "目录未完整描述上下文分档、缓存时长、Fast 或地区计费时保留内置规则", "目錄未完整描述上下文分檔、快取時長、Fast 或地區計費時保留內建規則", "카탈로그가 컨텍스트 구간, 캐시 TTL, Fast 또는 지역 요금을 완전히 설명하지 않으면 기본 규칙을 유지합니다", "カタログにコンテキスト段階、キャッシュTTL、Fast、地域料金の完全な情報がない場合は内蔵ルールを保持", "Встроенные правила сохраняют уровни контекста, TTL кеша, Fast и региональные тарифы, если каталог описывает их неполно", "Les règles intégrées conservent les paliers de contexte, TTL du cache, Fast et tarifs régionaux non décrits complètement dans le catalogue"},
}
