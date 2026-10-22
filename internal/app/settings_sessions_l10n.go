package app

const (
	xSessionsSettingsTab          textKey = "sessionsSettingsTab"
	xSessionsSearch               textKey = "sessionsSearch"
	xSessionsSearchHint           textKey = "sessionsSearchHint"
	xSessionsStartDate            textKey = "sessionsStartDate"
	xSessionsEndDate              textKey = "sessionsEndDate"
	xSessionsChooseStart          textKey = "sessionsChooseStart"
	xSessionsChooseEnd            textKey = "sessionsChooseEnd"
	xSessionsClearDate            textKey = "sessionsClearDate"
	xSessionsClearDateNamed       textKey = "sessionsClearDateNamed"
	xSessionsUseDate              textKey = "sessionsUseDate"
	xSessionsProjectFilter        textKey = "sessionsProjectFilter"
	xSessionsAllProjects          textKey = "sessionsAllProjects"
	xSessionsApply                textKey = "sessionsApply"
	xSessionsQuickRange           textKey = "sessionsQuickRange"
	xSessionsLast7Days            textKey = "sessionsLast7Days"
	xSessionsLast30Days           textKey = "sessionsLast30Days"
	xSessionsAllDates             textKey = "sessionsAllDates"
	xSessionsFilterSubmitHint     textKey = "sessionsFilterSubmitHint"
	xSessionsInvalidDate          textKey = "sessionsInvalidDate"
	xSessionsReversedDate         textKey = "sessionsReversedDate"
	xSessionsLoadFailed           textKey = "sessionsLoadFailed"
	xSessionsRefreshFailed        textKey = "sessionsRefreshFailed"
	xSessionsEmpty                textKey = "sessionsEmpty"
	xSessionsNoMatch              textKey = "sessionsNoMatch"
	xSessionsShowing              textKey = "sessionsShowing"
	xSessionsTotal                textKey = "sessionsTotal"
	xSessionsSelectFiltered       textKey = "sessionsSelectFiltered"
	xSessionsClearSelection       textKey = "sessionsClearSelection"
	xSessionsSelected             textKey = "sessionsSelected"
	xSessionsDeleteSelected       textKey = "sessionsDeleteSelected"
	xSessionsDeleteShort          textKey = "sessionsDeleteShort"
	xSessionsDeleteOne            textKey = "sessionsDeleteOne"
	xSessionsShowMore             textKey = "sessionsShowMore"
	xSessionsSelectOne            textKey = "sessionsSelectOne"
	xSessionsOpenDetails          textKey = "sessionsOpenDetails"
	xSessionsArchived             textKey = "sessionsArchived"
	xSessionsNeedsAttention       textKey = "sessionsNeedsAttention"
	xSessionsReveal               textKey = "sessionsReveal"
	xSessionsBack                 textKey = "sessionsBack"
	xSessionsPreviewFailed        textKey = "sessionsPreviewFailed"
	xSessionsRetry                textKey = "sessionsRetry"
	xSessionsNoPreview            textKey = "sessionsNoPreview"
	xSessionsID                   textKey = "sessionsID"
	xSessionsProject              textKey = "sessionsProject"
	xSessionsModel                textKey = "sessionsModel"
	xSessionsTokenBreakdown       textKey = "sessionsTokenBreakdown"
	xSessionsTokenValues          textKey = "sessionsTokenValues"
	xSessionsEstimatedCost        textKey = "sessionsEstimatedCost"
	xSessionsCacheHitRate         textKey = "sessionsCacheHitRate"
	xSessionsUpdated              textKey = "sessionsUpdated"
	xSessionsFiles                textKey = "sessionsFiles"
	xSessionsPath                 textKey = "sessionsPath"
	xSessionsProblem              textKey = "sessionsProblem"
	xSessionsPreview              textKey = "sessionsPreview"
	xSessionsPreviewTruncated     textKey = "sessionsPreviewTruncated"
	xSessionsRoleUser             textKey = "sessionsRoleUser"
	xSessionsRoleAssistant        textKey = "sessionsRoleAssistant"
	xSessionsRoleSystem           textKey = "sessionsRoleSystem"
	xSessionsRoleTool             textKey = "sessionsRoleTool"
	xSessionsDeleteConfirmTitle   textKey = "sessionsDeleteConfirmTitle"
	xSessionsDeleteConfirmMessage textKey = "sessionsDeleteConfirmMessage"
	xSessionsDeleteFailed         textKey = "sessionsDeleteFailed"
	xSessionsDeletePartial        textKey = "sessionsDeletePartial"
	xSessionsDeleteSuccess        textKey = "sessionsDeleteSuccess"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xSessionsSettingsTab:          {"Sessions", "会话管理", "工作階段管理", "세션 관리", "セッション管理", "Сеансы", "Sessions"},
		xSessionsSearch:               {"Search sessions", "搜索会话", "搜尋工作階段", "세션 검색", "セッションを検索", "Поиск сеансов", "Rechercher des sessions"},
		xSessionsSearchHint:           {"Title, project, or ID", "标题、项目或 ID", "標題、專案或 ID", "제목, 프로젝트 또는 ID", "タイトル、プロジェクト、ID", "Название, проект или ID", "Titre, projet ou ID"},
		xSessionsStartDate:            {"Start date", "开始日期", "開始日期", "시작일", "開始日", "Дата начала", "Date de début"},
		xSessionsEndDate:              {"End date", "结束日期", "結束日期", "종료일", "終了日", "Дата окончания", "Date de fin"},
		xSessionsChooseStart:          {"Choose start", "选择开始日期", "選擇開始日期", "시작일 선택", "開始日を選択", "Выбрать начало", "Choisir le début"},
		xSessionsChooseEnd:            {"Choose end", "选择结束日期", "選擇結束日期", "종료일 선택", "終了日を選択", "Выбрать окончание", "Choisir la fin"},
		xSessionsClearDate:            {"Clear", "清除", "清除", "지우기", "クリア", "Очистить", "Effacer"},
		xSessionsClearDateNamed:       {"Clear %s", "清除%s", "清除%s", "%s 지우기", "%sをクリア", "Очистить: %s", "Effacer %s"},
		xSessionsUseDate:              {"Use date", "使用日期", "使用日期", "날짜 사용", "この日付を使用", "Использовать дату", "Utiliser la date"},
		xSessionsProjectFilter:        {"Project filter", "项目筛选", "專案篩選", "프로젝트 필터", "プロジェクト絞り込み", "Фильтр по проекту", "Filtre de projet"},
		xSessionsAllProjects:          {"All projects", "全部项目", "所有專案", "모든 프로젝트", "すべてのプロジェクト", "Все проекты", "Tous les projets"},
		xSessionsApply:                {"Apply", "应用筛选", "套用篩選", "필터 적용", "絞り込みを適用", "Применить", "Appliquer"},
		xSessionsQuickRange:           {"Date shortcuts", "日期快捷项", "日期捷徑", "날짜 바로가기", "日付ショートカット", "Быстрый период", "Périodes rapides"},
		xSessionsLast7Days:            {"7 days", "近 7 天", "近 7 天", "최근 7일", "過去7日", "7 дней", "7 jours"},
		xSessionsLast30Days:           {"30 days", "近 30 天", "近 30 天", "최근 30일", "過去30日", "30 дней", "30 jours"},
		xSessionsAllDates:             {"All", "全部日期", "所有日期", "전체", "すべて", "Все", "Toutes"},
		xSessionsFilterSubmitHint:     {"Search, project, and date choices change the list after Apply", "搜索、项目与日期选择需点击应用筛选", "搜尋、專案與日期選擇需按套用篩選", "검색, 프로젝트 및 날짜 선택은 필터 적용 후 반영됩니다", "検索、プロジェクト、日付の選択は適用後に反映されます", "Поиск, проект и выбранные даты применяются по кнопке", "La recherche, le projet et les dates choisies s’appliquent avec le bouton"},
		xSessionsInvalidDate:          {"Use YYYY-MM-DD for both dates", "日期格式应为 YYYY-MM-DD", "日期格式應為 YYYY-MM-DD", "날짜 형식은 YYYY-MM-DD여야 합니다", "日付は YYYY-MM-DD 形式で入力してください", "Используйте формат ГГГГ-ММ-ДД", "Utilisez le format AAAA-MM-JJ"},
		xSessionsReversedDate:         {"The end date must be on or after the start date", "结束日期不能早于开始日期", "結束日期不可早於開始日期", "종료일은 시작일보다 빠를 수 없습니다", "終了日は開始日以降にしてください", "Дата окончания не может быть раньше даты начала", "La date de fin doit suivre la date de début"},
		xSessionsLoadFailed:           {"Could not load sessions: %s", "无法加载会话：%s", "無法載入工作階段：%s", "세션을 불러올 수 없습니다: %s", "セッションを読み込めません：%s", "Не удалось загрузить сеансы: %s", "Impossible de charger les sessions : %s"},
		xSessionsRefreshFailed:        {"Could not refresh sessions; showing the previous list: %s", "无法刷新会话，继续显示之前的列表：%s", "無法重新整理工作階段，繼續顯示先前的清單：%s", "세션을 새로 고칠 수 없어 이전 목록을 표시합니다: %s", "セッションを更新できないため前の一覧を表示します：%s", "Не удалось обновить сеансы; показан прежний список: %s", "Impossible d’actualiser les sessions ; la liste précédente reste affichée : %s"},
		xSessionsEmpty:                {"No local sessions found", "未找到本机会话", "找不到本機工作階段", "로컬 세션이 없습니다", "ローカルセッションがありません", "Локальные сеансы не найдены", "Aucune session locale"},
		xSessionsNoMatch:              {"No sessions match the applied filter", "没有会话符合已应用的筛选", "沒有工作階段符合已套用的篩選", "적용한 필터와 일치하는 세션이 없습니다", "適用中の条件に一致するセッションがありません", "Нет сеансов по заданному фильтру", "Aucune session ne correspond au filtre"},
		xSessionsShowing:              {"Showing %d of %d", "显示 %d / %d", "顯示 %d / %d", "%d/%d 표시", "%d / %d 件を表示", "Показано %d из %d", "%d sur %d affichées"},
		xSessionsTotal:                {"%d sessions in this filter", "当前筛选共 %d 个会话", "目前篩選共 %d 個工作階段", "현재 필터에 세션 %d개", "現在の条件に %d 件", "В фильтре: %d", "%d sessions dans ce filtre"},
		xSessionsSelectFiltered:       {"Select all %d filtered sessions", "选择当前筛选的全部 %d 个会话", "選取目前篩選的全部 %d 個工作階段", "필터 결과 %d개 모두 선택", "絞り込み結果 %d 件をすべて選択", "Выбрать все отфильтрованные: %d", "Sélectionner les %d sessions filtrées"},
		xSessionsClearSelection:       {"Clear selection", "清空选择", "清除選取", "선택 해제", "選択を解除", "Снять выделение", "Effacer la sélection"},
		xSessionsSelected:             {"%d selected", "已选 %d 个", "已選 %d 個", "%d개 선택됨", "%d 件選択中", "Выбрано: %d", "%d sélectionnées"},
		xSessionsDeleteSelected:       {"Move selected to Trash", "将所选会话移到系统回收站", "將所選工作階段移到系統資源回收筒", "선택 항목을 시스템 휴지통으로 이동", "選択項目をシステムのゴミ箱へ移動", "Переместить выбранное в корзину", "Déplacer la sélection dans la corbeille"},
		xSessionsDeleteShort:          {"Delete selected", "删除已选", "刪除已選", "선택 항목 삭제", "選択項目を削除", "Удалить выбранное", "Supprimer la sélection"},
		xSessionsDeleteOne:            {"Move to Trash", "移到系统回收站", "移到系統資源回收筒", "시스템 휴지통으로 이동", "システムのゴミ箱へ移動", "Переместить в корзину", "Déplacer dans la corbeille"},
		xSessionsShowMore:             {"Show %d more", "再显示 %d 个", "再顯示 %d 個", "%d개 더 보기", "さらに %d 件表示", "Показать ещё %d", "Afficher %d de plus"},
		xSessionsSelectOne:            {"Select %s", "选择 %s", "選取 %s", "%s 선택", "%s を選択", "Выбрать %s", "Sélectionner %s"},
		xSessionsOpenDetails:          {"Open details for %s", "打开 %s 的详情", "開啟 %s 的詳細資料", "%s 상세 정보 열기", "%s の詳細を開く", "Открыть сведения: %s", "Ouvrir les détails de %s"},
		xSessionsArchived:             {"Archived", "已归档", "已封存", "보관됨", "アーカイブ済み", "Архив", "Archivée"},
		xSessionsNeedsAttention:       {"Needs attention", "需要检查", "需要檢查", "확인 필요", "要確認", "Требует внимания", "À vérifier"},
		xSessionsReveal:               {"Show in file manager", "在文件管理器中显示", "在檔案管理器中顯示", "파일 관리자에서 보기", "ファイルマネージャーで表示", "Показать в файловом менеджере", "Afficher dans le gestionnaire de fichiers"},
		xSessionsBack:                 {"Back to sessions", "返回会话列表", "返回工作階段清單", "세션 목록으로 돌아가기", "セッション一覧へ戻る", "Назад к сеансам", "Retour aux sessions"},
		xSessionsPreviewFailed:        {"Could not load the preview: %s", "无法加载对话预览：%s", "無法載入對話預覽：%s", "대화 미리보기를 불러올 수 없습니다: %s", "会話プレビューを読み込めません：%s", "Не удалось загрузить предпросмотр: %s", "Impossible de charger l’aperçu : %s"},
		xSessionsRetry:                {"Retry", "重试", "重試", "다시 시도", "再試行", "Повторить", "Réessayer"},
		xSessionsNoPreview:            {"No conversation preview is available", "没有可用的对话预览", "沒有可用的對話預覽", "사용 가능한 대화 미리보기가 없습니다", "会話プレビューはありません", "Предпросмотр диалога недоступен", "Aucun aperçu de conversation"},
		xSessionsID:                   {"Session ID", "会话 ID", "工作階段 ID", "세션 ID", "セッション ID", "ID сеанса", "ID de session"},
		xSessionsProject:              {"Project", "项目", "專案", "프로젝트", "プロジェクト", "Проект", "Projet"},
		xSessionsModel:                {"Model", "模型", "模型", "모델", "モデル", "Модель", "Modèle"},
		xSessionsTokenBreakdown:       {"Token usage", "Token 用量", "Token 用量", "토큰 사용량", "トークン使用量", "Токены", "Utilisation des jetons"},
		xSessionsTokenValues:          {"%s total · %s input · %s cached · %s output · %d turns", "总计 %s · 输入 %s · 缓存 %s · 输出 %s · %d 轮", "總計 %s · 輸入 %s · 快取 %s · 輸出 %s · %d 輪", "전체 %s · 입력 %s · 캐시 %s · 출력 %s · %d턴", "合計 %s · 入力 %s · キャッシュ %s · 出力 %s · %d ターン", "Всего %s · вход %s · кэш %s · выход %s · ходов %d", "Total %s · entrée %s · cache %s · sortie %s · %d tours"},
		xSessionsEstimatedCost:        {"API-equivalent cost", "API 等价成本", "API 等價成本", "API 환산 비용", "API 相当コスト", "Эквивалентная стоимость API", "Coût équivalent API"},
		xSessionsCacheHitRate:         {"Cache hit rate", "缓存命中率", "快取命中率", "캐시 적중률", "キャッシュヒット率", "Доля попаданий в кэш", "Taux d’accès au cache"},
		xSessionsUpdated:              {"Last activity", "最后活动时间", "最後活動時間", "마지막 활동", "最終アクティビティ", "Последняя активность", "Dernière activité"},
		xSessionsFiles:                {"Session files", "会话文件", "工作階段檔案", "세션 파일", "セッションファイル", "Файлы сеанса", "Fichiers de session"},
		xSessionsPath:                 {"Location", "位置", "位置", "위치", "場所", "Расположение", "Emplacement"},
		xSessionsProblem:              {"Scan issue", "扫描问题", "掃描問題", "스캔 문제", "スキャンの問題", "Проблема сканирования", "Problème d’analyse"},
		xSessionsPreview:              {"Conversation preview", "对话预览", "對話預覽", "대화 미리보기", "会話プレビュー", "Предпросмотр диалога", "Aperçu de la conversation"},
		xSessionsPreviewTruncated:     {"The preview is truncated; the session file is unchanged", "预览已截断，会话文件未被修改", "預覽已截斷，工作階段檔案未被修改", "미리보기가 잘렸으며 세션 파일은 변경되지 않았습니다", "プレビューは省略されていますが、セッションファイルは変更されていません", "Предпросмотр сокращён, файл сеанса не изменён", "L’aperçu est tronqué, le fichier de session reste inchangé"},
		xSessionsRoleUser:             {"User", "用户", "使用者", "사용자", "ユーザー", "Пользователь", "Utilisateur"},
		xSessionsRoleAssistant:        {"Assistant", "助手", "助理", "어시스턴트", "アシスタント", "Ассистент", "Assistant"},
		xSessionsRoleSystem:           {"System", "系统", "系統", "시스템", "システム", "Система", "Système"},
		xSessionsRoleTool:             {"Tool", "工具", "工具", "도구", "ツール", "Инструмент", "Outil"},
		xSessionsDeleteConfirmTitle:   {"Move sessions to Trash?", "将会话移到系统回收站？", "將工作階段移到系統資源回收筒？", "세션을 시스템 휴지통으로 이동할까요?", "セッションをシステムのゴミ箱へ移動しますか？", "Переместить сеансы в корзину?", "Déplacer les sessions dans la corbeille ?"},
		xSessionsDeleteConfirmMessage: {"Provider: %s\nSessions: %d\n\nClose or refresh the matching CLI first. The exact session files move to the system Trash and can be restored there. Local usage totals and history will change; Codex may leave index entries until Session Repair runs.", "平台：%s\n会话数量：%d\n\n请先关闭或刷新对应 CLI，准确归属的会话文件会移到系统回收站并可从那里恢复，本机用量与历史会随之变化；Codex 可能保留索引条目，之后可运行会话修复", "平台：%s\n工作階段數量：%d\n\n請先關閉或重新整理對應 CLI，明確屬於工作階段的檔案會移到系統資源回收筒並可從中復原，本機用量與歷史會隨之變更；Codex 可能保留索引項目，之後可執行工作階段修復", "제공자: %s\n세션: %d개\n\n먼저 해당 CLI를 닫거나 새로 고치세요. 해당 세션 파일은 시스템 휴지통으로 이동하며 복원할 수 있습니다. 로컬 사용량과 기록이 변경되고 Codex 인덱스 항목은 세션 복구 전까지 남을 수 있습니다", "プロバイダー：%s\nセッション：%d 件\n\n対象 CLI を閉じるか更新してください。対象のセッションファイルはシステムのゴミ箱へ移動し、復元できます。ローカル使用量と履歴が変わり、Codex の索引項目はセッション修復まで残る場合があります", "Поставщик: %s\nСеансов: %d\n\nСначала закройте или обновите соответствующий CLI. Файлы сеансов попадут в системную корзину, откуда их можно восстановить. Локальная статистика изменится; записи индекса Codex могут остаться до восстановления сеансов.", "Fournisseur : %s\nSessions : %d\n\nFermez ou actualisez d’abord le CLI correspondant. Les fichiers de session concernés iront dans la corbeille du système et pourront y être restaurés. Les statistiques locales changeront ; des entrées d’index Codex peuvent rester jusqu’à la réparation des sessions."},
		xSessionsDeleteFailed:         {"Could not move sessions to Trash: %s", "无法将会话移到系统回收站：%s", "無法將工作階段移到系統資源回收筒：%s", "세션을 시스템 휴지통으로 옮길 수 없습니다: %s", "セッションをシステムのゴミ箱へ移動できません：%s", "Не удалось переместить сеансы в корзину: %s", "Impossible de déplacer les sessions dans la corbeille : %s"},
		xSessionsDeletePartial:        {"Moved %d sessions; %d failed; files moved %d/%d: %s", "已移动 %d 个会话，%d 个失败，文件已回收 %d/%d：%s", "已移動 %d 個工作階段，%d 個失敗，檔案已回收 %d/%d：%s", "%d개 세션을 이동했고 %d개는 실패했습니다. 파일 %d/%d개 이동: %s", "%d 件を移動し、%d 件失敗、ファイル %d/%d 件を移動：%s", "Перемещено сеансов: %d; ошибок: %d; файлов %d/%d: %s", "%d sessions déplacées ; %d échecs ; fichiers %d/%d : %s"},
		xSessionsDeleteSuccess:        {"Moved %d sessions (%d files) to the system Trash", "已将 %d 个会话（%d 个文件）移到系统回收站", "已將 %d 個工作階段（%d 個檔案）移到系統資源回收筒", "%d개 세션(파일 %d개)을 시스템 휴지통으로 옮겼습니다", "%d 件（%d ファイル）をシステムのゴミ箱へ移動しました", "Перемещено в корзину: %d сеансов, %d файлов", "%d sessions (%d fichiers) déplacées dans la corbeille du système"},
	} {
		extraTexts[key] = texts
	}
}
