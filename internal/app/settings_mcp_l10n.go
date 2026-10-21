package app

const (
	xMCPSettingsTab          textKey = "mcpSettingsTab"
	xMCPScopeInfo            textKey = "mcpScopeInfo"
	xMCPSearch               textKey = "mcpSearch"
	xMCPSearchHint           textKey = "mcpSearchHint"
	xMCPLoadFailed           textKey = "mcpLoadFailed"
	xMCPRefreshFailed        textKey = "mcpRefreshFailed"
	xMCPEmpty                textKey = "mcpEmpty"
	xMCPNoMatch              textKey = "mcpNoMatch"
	xMCPSelectionCounts      textKey = "mcpSelectionCounts"
	xMCPSelectFiltered       textKey = "mcpSelectFiltered"
	xMCPClearSelection       textKey = "mcpClearSelection"
	xMCPDeleteSelected       textKey = "mcpDeleteSelected"
	xMCPDeleteOne            textKey = "mcpDeleteOne"
	xMCPSelectOne            textKey = "mcpSelectOne"
	xMCPOpenDetails          textKey = "mcpOpenDetails"
	xMCPDisabled             textKey = "mcpDisabled"
	xMCPProblem              textKey = "mcpProblem"
	xMCPMissingTransport     textKey = "mcpMissingTransport"
	xMCPMultipleTransports   textKey = "mcpMultipleTransports"
	xMCPInvalidTransport     textKey = "mcpInvalidTransport"
	xMCPMissingType          textKey = "mcpMissingType"
	xMCPTransportMismatch    textKey = "mcpTransportMismatch"
	xMCPUnknownProblem       textKey = "mcpUnknownProblem"
	xMCPReveal               textKey = "mcpReveal"
	xMCPBack                 textKey = "mcpBack"
	xMCPPath                 textKey = "mcpPath"
	xMCPTransport            textKey = "mcpTransport"
	xMCPCommand              textKey = "mcpCommand"
	xMCPURL                  textKey = "mcpURL"
	xMCPStatus               textKey = "mcpStatus"
	xMCPEnabled              textKey = "mcpConfigured"
	xMCPReadOnly             textKey = "mcpReadOnly"
	xMCPRedactedConfig       textKey = "mcpRedactedConfig"
	xMCPDeleteConfirmTitle   textKey = "mcpDeleteConfirmTitle"
	xMCPDeleteConfirmMessage textKey = "mcpDeleteConfirmMessage"
	xMCPDeleteFailed         textKey = "mcpDeleteFailed"
	xMCPDeletePartial        textKey = "mcpDeletePartial"
	xMCPDeleteSuccess        textKey = "mcpDeleteSuccess"
	xMCPEnableServer         textKey = "mcpEnableServer"
	xMCPToggle               textKey = "mcpToggle"
	xMCPToggleUnsupported    textKey = "mcpToggleUnsupported"
	xMCPToggleClaude         textKey = "mcpToggleClaude"
	xMCPToggleFailed         textKey = "mcpToggleFailed"
	xMCPEnabledNotice        textKey = "mcpEnabledNotice"
	xMCPDisabledNotice       textKey = "mcpDisabledNotice"
	xMCPApplyCodex           textKey = "mcpApplyCodex"
	xMCPApplyGrok            textKey = "mcpApplyGrok"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xMCPSettingsTab:          {"MCP", "MCP 管理", "MCP 管理", "MCP 관리", "MCP 管理", "MCP", "MCP"},
		xMCPScopeInfo:            {"Global native user configuration only; project, plugin, and enterprise-managed sources are not shown", "仅显示全局原生用户配置，不显示项目、插件与企业托管来源", "僅顯示全域原生使用者設定，不顯示專案、外掛與企業託管來源", "전역 네이티브 사용자 구성만 표시하며 프로젝트, 플러그인 및 기업 관리 소스는 표시하지 않습니다", "グローバルのネイティブユーザー設定のみ表示し、プロジェクト、プラグイン、企業管理のソースは表示しません", "Показаны только глобальные пользовательские настройки; источники проектов, плагинов и организаций скрыты", "Seule la configuration utilisateur native globale est affichée ; les sources de projet, de plug-in et d’entreprise sont masquées"},
		xMCPSearch:               {"Search MCP servers", "搜索 MCP 服务", "搜尋 MCP 服務", "MCP 서버 검색", "MCP サーバーを検索", "Поиск серверов MCP", "Rechercher des serveurs MCP"},
		xMCPSearchHint:           {"Name, transport, command, URL, or issue", "名称、传输方式、命令、URL 或问题", "名稱、傳輸方式、命令、URL 或問題", "이름, 전송 방식, 명령, URL 또는 문제", "名前、トランスポート、コマンド、URL、問題", "Имя, транспорт, команда, URL или проблема", "Nom, transport, commande, URL ou problème"},
		xMCPLoadFailed:           {"Could not load MCP configuration: %s", "无法加载 MCP 配置：%s", "無法載入 MCP 設定：%s", "MCP 구성을 불러올 수 없습니다: %s", "MCP 設定を読み込めません：%s", "Не удалось загрузить конфигурацию MCP: %s", "Impossible de charger la configuration MCP : %s"},
		xMCPRefreshFailed:        {"Could not refresh MCP configuration; showing the previous list: %s", "无法刷新 MCP 配置，继续显示之前的列表：%s", "無法重新整理 MCP 設定，繼續顯示先前的清單：%s", "MCP 구성을 새로 고칠 수 없어 이전 목록을 표시합니다: %s", "MCP 設定を更新できないため前の一覧を表示します：%s", "Не удалось обновить MCP; показан прежний список: %s", "Impossible d’actualiser MCP ; la liste précédente reste affichée : %s"},
		xMCPEmpty:                {"No global MCP servers found", "未找到全局 MCP 服务", "找不到全域 MCP 服務", "전역 MCP 서버가 없습니다", "グローバル MCP サーバーがありません", "Глобальные серверы MCP не найдены", "Aucun serveur MCP global"},
		xMCPNoMatch:              {"No MCP servers match this search", "没有 MCP 服务符合搜索条件", "沒有 MCP 服務符合搜尋條件", "검색과 일치하는 MCP 서버가 없습니다", "検索に一致する MCP サーバーがありません", "Нет серверов MCP по этому запросу", "Aucun serveur MCP ne correspond à la recherche"},
		xMCPSelectionCounts:      {"%d shown · %d selected", "显示 %d 个 · 已选择 %d 个", "顯示 %d 個 · 已選擇 %d 個", "%d개 표시 · %d개 선택", "%d 件表示 · %d 件選択", "Показано: %d · выбрано: %d", "%d affichés · %d sélectionnés"},
		xMCPSelectFiltered:       {"Select filtered (%d)", "全选筛选结果（%d）", "全選篩選結果（%d）", "필터 결과 선택 (%d)", "絞り込み結果を選択（%d）", "Выбрать результаты (%d)", "Sélectionner les résultats (%d)"},
		xMCPClearSelection:       {"Clear selection", "清除选择", "清除選擇", "선택 지우기", "選択を解除", "Снять выделение", "Effacer la sélection"},
		xMCPDeleteSelected:       {"Delete selected", "删除所选配置", "刪除所選設定", "선택한 구성 삭제", "選択した設定を削除", "Удалить выбранные", "Supprimer la sélection"},
		xMCPDeleteOne:            {"Delete configuration", "删除此配置", "刪除此設定", "이 구성 삭제", "この設定を削除", "Удалить конфигурацию", "Supprimer la configuration"},
		xMCPSelectOne:            {"Select %s", "选择 %s", "選擇 %s", "%s 선택", "%s を選択", "Выбрать %s", "Sélectionner %s"},
		xMCPOpenDetails:          {"Open details for %s", "打开 %s 的详情", "開啟 %s 的詳細資料", "%s 세부 정보 열기", "%s の詳細を開く", "Открыть сведения: %s", "Ouvrir les détails de %s"},
		xMCPDisabled:             {"Disabled in configuration", "配置中已禁用", "設定中已停用", "구성에서 비활성화됨", "設定で無効", "Отключено в конфигурации", "Désactivé dans la configuration"},
		xMCPProblem:              {"Needs attention", "需要处理", "需要處理", "확인 필요", "要確認", "Требует внимания", "À vérifier"},
		xMCPMissingTransport:     {"No command or URL is configured", "未配置命令或 URL", "未設定命令或 URL", "명령 또는 URL이 구성되지 않음", "コマンドまたは URL が未設定", "Команда или URL не настроены", "Aucune commande ni URL configurée"},
		xMCPMultipleTransports:   {"Both command and URL are configured", "同时配置了命令与 URL", "同時設定了命令與 URL", "명령과 URL이 모두 구성됨", "コマンドと URL の両方が設定済み", "Настроены и команда, и URL", "La commande et l’URL sont toutes deux configurées"},
		xMCPInvalidTransport:     {"The transport value is not supported", "传输方式不受支持", "傳輸方式不受支援", "지원되지 않는 전송 방식", "未対応のトランスポートです", "Тип транспорта не поддерживается", "Le transport n’est pas pris en charge"},
		xMCPMissingType:          {"The URL transport requires a type", "URL 传输缺少类型", "URL 傳輸缺少類型", "URL 전송에 유형이 필요함", "URL トランスポートに種類が必要です", "Для транспорта URL требуется тип", "Le transport URL nécessite un type"},
		xMCPTransportMismatch:    {"The transport type does not match the command or URL", "传输类型与命令或 URL 不匹配", "傳輸類型與命令或 URL 不相符", "전송 유형이 명령 또는 URL과 일치하지 않음", "トランスポート種類がコマンドまたは URL と一致しません", "Тип транспорта не соответствует команде или URL", "Le type de transport ne correspond pas à la commande ou à l’URL"},
		xMCPUnknownProblem:       {"The configuration needs attention", "配置需要处理", "設定需要處理", "구성을 확인해야 함", "設定の確認が必要", "Конфигурация требует внимания", "La configuration doit être vérifiée"},
		xMCPReveal:               {"Show configuration file", "显示配置文件", "顯示設定檔", "구성 파일 표시", "設定ファイルを表示", "Показать файл конфигурации", "Afficher le fichier de configuration"},
		xMCPBack:                 {"Back to MCP servers", "返回 MCP 服务列表", "返回 MCP 服務清單", "MCP 서버 목록으로 돌아가기", "MCP サーバー一覧に戻る", "Назад к серверам MCP", "Retour aux serveurs MCP"},
		xMCPPath:                 {"Configuration", "配置路径", "設定路徑", "구성 경로", "設定パス", "Конфигурация", "Configuration"},
		xMCPTransport:            {"Transport", "传输方式", "傳輸方式", "전송 방식", "トランスポート", "Транспорт", "Transport"},
		xMCPCommand:              {"Command", "命令", "命令", "명령", "コマンド", "Команда", "Commande"},
		xMCPURL:                  {"URL", "URL", "URL", "URL", "URL", "URL", "URL"},
		xMCPStatus:               {"Configuration state", "配置状态", "設定狀態", "구성 상태", "設定状態", "Состояние конфигурации", "État de la configuration"},
		xMCPEnabled:              {"Configured", "已配置", "已設定", "구성됨", "設定済み", "Настроено", "Configuré"},
		xMCPReadOnly:             {"Cannot be deleted here", "无法在此删除", "無法在此刪除", "여기에서 삭제할 수 없음", "ここでは削除不可", "Нельзя удалить здесь", "Suppression impossible ici"},
		xMCPRedactedConfig:       {"Redacted configuration", "脱敏配置", "脫敏設定", "민감 정보가 가려진 구성", "秘匿化された設定", "Конфигурация без секретов", "Configuration expurgée"},
		xMCPDeleteConfirmTitle:   {"Delete MCP configuration?", "删除 MCP 配置？", "刪除 MCP 設定？", "MCP 구성을 삭제할까요?", "MCP 設定を削除しますか？", "Удалить конфигурацию MCP?", "Supprimer la configuration MCP ?"},
		xMCPDeleteConfirmMessage: {"Provider: %s\nEntries: %d\n\nThis deletes the selected configuration entries after creating an automatic backup. Restart the matching CLI for the change to take effect.", "服务商：%s\n配置项：%d\n\n系统会先自动备份，再删除所选配置项；请重新启动对应 CLI 以使更改生效。", "服務商：%s\n設定項目：%d\n\n系統會先自動備份，再刪除所選設定項目；請重新啟動對應 CLI 以使變更生效。", "제공자: %s\n구성 항목: %d개\n\n자동 백업을 만든 뒤 선택한 구성 항목을 삭제합니다. 변경 사항을 적용하려면 해당 CLI를 다시 시작하세요.", "プロバイダー：%s\n設定項目：%d 件\n\n自動バックアップを作成してから選択した設定項目を削除します。反映するには対象の CLI を再起動してください。", "Поставщик: %s\nЗаписей: %d\n\nПосле автоматического резервного копирования выбранные записи будут удалены. Перезапустите соответствующий CLI.", "Fournisseur : %s\nEntrées : %d\n\nLes entrées sélectionnées seront supprimées après une sauvegarde automatique. Redémarrez le CLI concerné."},
		xMCPDeleteFailed:         {"Could not delete MCP configuration: %v", "无法删除 MCP 配置：%v", "無法刪除 MCP 設定：%v", "MCP 구성을 삭제할 수 없습니다: %v", "MCP 設定を削除できません：%v", "Не удалось удалить конфигурацию MCP: %v", "Impossible de supprimer la configuration MCP : %v"},
		xMCPDeletePartial:        {"Deleted %d; failed %d: %s", "已删除 %d 项，失败 %d 项：%s", "已刪除 %d 項，失敗 %d 項：%s", "%d개 삭제, %d개 실패: %s", "%d 件削除、%d 件失敗：%s", "Удалено: %d; ошибок: %d: %s", "%d supprimés ; %d échecs : %s"},
		xMCPDeleteSuccess:        {"Deleted %d MCP configuration entries; restart the CLI to apply the change", "已删除 %d 个 MCP 配置项，请重新启动 CLI 以应用更改", "已刪除 %d 個 MCP 設定項目，請重新啟動 CLI 以套用變更", "MCP 구성 항목 %d개를 삭제했습니다. 변경 사항을 적용하려면 CLI를 다시 시작하세요", "MCP 設定を %d 件削除しました。反映するには CLI を再起動してください", "Удалено записей MCP: %d; перезапустите CLI", "%d entrées MCP supprimées ; redémarrez le CLI"},
		xMCPEnableServer:         {"Enable %s", "启用 %s", "啟用 %s", "%s 활성화", "%s を有効化", "Включить %s", "Activer %s"},
		xMCPToggle:               {"Enable / disable", "启用 / 停用", "啟用 / 停用", "활성화 / 비활성화", "有効 / 無効", "Включение", "Activation"},
		xMCPToggleUnsupported:    {"This entry is written in a form that cannot be switched safely here; edit the configuration file", "此配置项的写法无法在此安全切换，请直接编辑配置文件", "此設定項目的寫法無法在此安全切換，請直接編輯設定檔", "이 항목은 여기에서 안전하게 전환할 수 없는 형식으로 작성되어 있습니다. 구성 파일을 직접 편집하세요", "この項目はここで安全に切り替えられない形式で書かれています。設定ファイルを直接編集してください", "Запись задана в форме, которую нельзя безопасно переключить здесь; измените файл конфигурации", "Cette entrée est écrite sous une forme qui ne peut pas être basculée ici en toute sécurité ; modifiez le fichier de configuration"},
		xMCPToggleClaude:         {"Claude Code records MCP on/off per project; switch it with /mcp in Claude Code", "Claude Code 按项目记录 MCP 启停，请在 Claude Code 中用 /mcp 切换", "Claude Code 依專案記錄 MCP 啟用狀態，請在 Claude Code 中以 /mcp 切換", "Claude Code는 MCP 활성화 상태를 프로젝트별로 기록합니다. Claude Code에서 /mcp로 전환하세요", "Claude Code は MCP の有効・無効をプロジェクトごとに記録します。Claude Code の /mcp で切り替えてください", "Claude Code хранит включение MCP для каждого проекта; переключайте через /mcp в Claude Code", "Claude Code enregistre l’activation MCP par projet ; basculez-la avec /mcp dans Claude Code"},
		xMCPToggleFailed:         {"Could not switch %s: %v", "无法切换 %s：%v", "無法切換 %s：%v", "%s을(를) 전환할 수 없습니다: %v", "%s を切り替えられません：%v", "Не удалось переключить %s: %v", "Impossible de basculer %s : %v"},
		xMCPEnabledNotice:        {"Enabled %s. %s", "已启用 %s。%s", "已啟用 %s。%s", "%s을(를) 활성화했습니다. %s", "%s を有効にしました。%s", "%s включён. %s", "%s activé. %s"},
		xMCPDisabledNotice:       {"Disabled %s. %s", "已停用 %s。%s", "已停用 %s。%s", "%s을(를) 비활성화했습니다. %s", "%s を無効にしました。%s", "%s отключён. %s", "%s désactivé. %s"},
		xMCPApplyCodex:           {"Saved to config.toml after a backup; restart Codex to apply", "已备份并写入 config.toml，重启 Codex 后生效", "已備份並寫入 config.toml，重新啟動 Codex 後生效", "백업 후 config.toml에 저장했습니다. Codex를 다시 시작하면 적용됩니다", "バックアップ後に config.toml へ保存しました。Codex の再起動で反映されます", "Сохранено в config.toml после резервной копии; перезапустите Codex", "Enregistré dans config.toml après sauvegarde ; redémarrez Codex pour l’appliquer"},
		xMCPApplyGrok:            {"Saved to config.toml after a backup; new Grok sessions use it, and an open session picks it up after pressing r in /mcps", "已备份并写入 config.toml，新的 Grok 会话直接生效，已打开的会话在 /mcps 中按 r 刷新", "已備份並寫入 config.toml，新的 Grok 工作階段直接生效，已開啟的工作階段在 /mcps 中按 r 重新整理", "백업 후 config.toml에 저장했습니다. 새 Grok 세션에 바로 적용되며, 열려 있는 세션은 /mcps에서 r을 눌러 새로 고치세요", "バックアップ後に config.toml へ保存しました。新しい Grok セッションに反映され、開いているセッションは /mcps で r を押すと更新されます", "Сохранено в config.toml после резервной копии; новые сеансы Grok применят это сразу, в открытом сеансе нажмите r в /mcps", "Enregistré dans config.toml après sauvegarde ; les nouvelles sessions Grok l’appliquent, et une session ouverte après avoir appuyé sur r dans /mcps"},
	} {
		extraTexts[key] = texts
	}
}
