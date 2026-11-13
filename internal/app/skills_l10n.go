package app

const (
	xSkillsTitle             textKey = "skillsTitle"
	xSkillsModuleDescription textKey = "skillsModuleDescription"
	xSkillsManage            textKey = "skillsManage"
	xSkillsSummary           textKey = "skillsSummary"
	xSkillsIssueCount        textKey = "skillsIssueCount"
	xSkillsEmpty             textKey = "skillsEmpty"
	xSkillsSearch            textKey = "skillsSearch"
	xSkillsNoMatch           textKey = "skillsNoMatch"
	xSkillsInfo              textKey = "skillsInfo"
	xSkillsScanFailed        textKey = "skillsScanFailed"

	xSkillsScopeSystem  textKey = "skillsScopeSystem"
	xSkillsScopeBundled textKey = "skillsScopeBundled"
	xSkillsScopeAdmin   textKey = "skillsScopeAdmin"
	xSkillsScopePlugin  textKey = "skillsScopePlugin"
	xSkillsScopeSynced  textKey = "skillsScopeSynced"

	xSkillsStateOff         textKey = "skillsStateOff"
	xSkillsStateOverridden  textKey = "skillsStateOverridden"
	xSkillsStateNotLoaded   textKey = "skillsStateNotLoaded"
	xSkillsReasonConfigPath textKey = "skillsReasonConfigPath"
	xSkillsReasonConfigName textKey = "skillsReasonConfigName"
	xSkillsReasonBundledOff textKey = "skillsReasonBundledOff"
	xSkillsReasonOverride   textKey = "skillsReasonOverride"
	xSkillsReasonDeny       textKey = "skillsReasonDeny"
	xSkillsReasonDisabled   textKey = "skillsReasonDisabled"
	xSkillsReasonIgnored    textKey = "skillsReasonIgnored"
	xSkillsReasonCompatOff  textKey = "skillsReasonCompatOff"
	xSkillsReasonPluginOff  textKey = "skillsReasonPluginOff"
	xSkillsReasonShadowed   textKey = "skillsReasonShadowed"
	xSkillsReasonNested     textKey = "skillsReasonNested"
	xSkillsReasonTooDeep    textKey = "skillsReasonTooDeep"
	xSkillsReasonNoDesc     textKey = "skillsReasonMissingDescription"
	xSkillsReasonBadFront   textKey = "skillsReasonInvalidFrontmatter"
	xSkillsReasonHidden     textKey = "skillsReasonHiddenDir"

	xSkillsEnabled          textKey = "skillsEnabled"
	xSkillsToggleCodex      textKey = "skillsToggleHintCodex"
	xSkillsToggleClaude     textKey = "skillsToggleHintClaude"
	xSkillsToggleGrok       textKey = "skillsToggleHintGrok"
	xSkillsToggleFailed     textKey = "skillsToggleFailed"
	xSkillsBlockPlugin      textKey = "skillsToggleBlockedPlugin"
	xSkillsBlockReadOnly    textKey = "skillsToggleBlockedReadOnly"
	xSkillsBlockDeny        textKey = "skillsToggleBlockedDeny"
	xSkillsBlockIgnored     textKey = "skillsToggleBlockedIgnored"
	xSkillsBlockOther       textKey = "skillsToggleBlockedOther"
	xSkillsReadOnlySystem   textKey = "skillsReadOnlySystem"
	xSkillsReadOnlyBundled  textKey = "skillsReadOnlyBundled"
	xSkillsReadOnlyAdmin    textKey = "skillsReadOnlyAdmin"
	xSkillsReadOnlySynced   textKey = "skillsReadOnlySynced"
	xSkillsReadOnlyPluginCX textKey = "skillsReadOnlyPluginCodex"
	xSkillsReadOnlyPluginCL textKey = "skillsReadOnlyPluginClaude"
	xSkillsReadOnlyPluginGK textKey = "skillsReadOnlyPluginGrok"
	xSkillsReadOnlyPluginGM textKey = "skillsReadOnlyPluginGemini"
	xSkillsBlockNoToggle    textKey = "skillsToggleBlockedNoToggle"

	xSkillsLocation    textKey = "skillsLocation"
	xSkillsSource      textKey = "skillsSource"
	xSkillsInvoke      textKey = "skillsInvokeAs"
	xSkillsModified    textKey = "skillsModified"
	xSkillsSharedWith  textKey = "skillsSharedWith"
	xSkillsLinkTarget  textKey = "skillsLinkTarget"
	xSkillsOverridden  textKey = "skillsOverriddenBy"
	xSkillsPlugin      textKey = "skillsPlugin"
	xSkillsState       textKey = "skillsState"
	xSkillsStateOn     textKey = "skillsStateOn"
	xSkillsOpenFolder  textKey = "skillsOpenFolder"
	xSkillsReveal      textKey = "skillsReveal"
	xSkillsShowContent textKey = "skillsShowContent"
	xSkillsHideContent textKey = "skillsHideContent"
	xSkillsTruncated   textKey = "skillsContentTruncated"
	xSkillsNoContent   textKey = "skillsNoContent"
	xSkillsReadFailed  textKey = "skillsReadFailed"
	xSkillsOpenFailed  textKey = "skillsOpenFailed"

	xSkillsDelete        textKey = "skillsDelete"
	xSkillsDeleteTitle   textKey = "skillsDeleteTitle"
	xSkillsDeleteMessage textKey = "skillsDeleteMessage"
	xSkillsDeleteLink    textKey = "skillsDeleteLink"
	xSkillsDeleteCovers  textKey = "skillsDeleteCovers"
	xSkillsDeleteShared  textKey = "skillsDeleteShared"
	xSkillsDeleteRestart textKey = "skillsDeleteRestart"
	xSkillsDeleteFailed  textKey = "skillsDeleteFailed"
	xSkillsChanged       textKey = "skillsChanged"

	xSkillsIssuesTitle     textKey = "skillsIssuesTitle"
	xSkillsIssueBrokenLink textKey = "skillsIssueBrokenLink"
	xSkillsIssueNested     textKey = "skillsIssueNested"
	xSkillsIssueNoDesc     textKey = "skillsIssueMissingDescription"
	xSkillsIssueBadFront   textKey = "skillsIssueInvalidFrontmatter"
	xSkillsIssueLongName   textKey = "skillsIssueNameTooLong"
	xSkillsIssueDuplicate  textKey = "skillsIssueDuplicateName"
	xSkillsIssueTruncated  textKey = "skillsIssueTruncated"
	xSkillsRemoveLink      textKey = "skillsRemoveLink"
	xSkillsRemoveLinkTitle textKey = "skillsRemoveLinkTitle"
	xSkillsRemoveLinkMsg   textKey = "skillsRemoveLinkMessage"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xSkillsTitle:             {"Skills", "Skills", "Skills", "Skills", "Skills", "Навыки", "Skills"},
		xSkillsModuleDescription: {"The global skills this CLI loads, with their state, folders and removal", "该 CLI 加载的全局 Skill，可查看状态、打开目录或删除", "該 CLI 載入的全域 Skill，可查看狀態、打開目錄或刪除", "이 CLI가 불러오는 전역 Skill의 상태 확인, 폴더 열기, 삭제", "このCLIが読み込むグローバルSkillの状態確認、フォルダを開く、削除", "Глобальные навыки этого CLI: состояние, папки и удаление", "Les skills globaux chargés par ce CLI : état, dossiers et suppression"},
		xSkillsManage:            {"Manage skills", "管理 Skills", "管理 Skills", "Skills 관리", "Skillsを管理", "Управление навыками", "Gérer les skills"},
		xSkillsSummary:           {"Installed %d · Off %d", "已安装 %d · 已停用 %d", "已安裝 %d · 已停用 %d", "설치됨 %d · 꺼짐 %d", "インストール済み %d · オフ %d", "Установлено %d · Отключено %d", "Installés %d · Désactivés %d"},
		xSkillsIssueCount:        {"Issues found: %d", "发现问题：%d", "發現問題：%d", "발견된 문제: %d", "検出された問題：%d", "Найдено проблем: %d", "Problèmes détectés : %d"},
		xSkillsEmpty:             {"No global skills found", "未找到全局 Skill", "未找到全域 Skill", "전역 Skill이 없습니다", "グローバルSkillが見つかりません", "Глобальные навыки не найдены", "Aucun skill global trouvé"},
		xSkillsSearch:            {"Search skills", "搜索 Skill", "搜尋 Skill", "Skill 검색", "Skillを検索", "Поиск навыков", "Rechercher un skill"},
		xSkillsNoMatch:           {"No matching skills", "没有匹配的 Skill", "沒有符合的 Skill", "일치하는 Skill이 없습니다", "一致するSkillはありません", "Совпадений нет", "Aucun skill correspondant"},
		xSkillsInfo:              {"Lists the global skill folders this CLI reads, without project skills. Deleting moves the folder to the Quosparr skills trash, where Settings > Skills can restore it; switching a skill on or off edits the CLI's own settings file after a backup in ~/.quosparr/backups.", "列出该 CLI 读取的全局 Skill 目录，不含项目级 Skill。删除会把目录移到 Quosparr 的 Skill 回收站，可在“设置 > Skill 管理”中恢复；启用或停用会先备份到 ~/.quosparr/backups，再修改该 CLI 自己的配置文件。", "列出該 CLI 讀取的全域 Skill 目錄，不含專案級 Skill。刪除會把目錄移到 Quosparr 的 Skill 回收站，可在「設定 > Skill 管理」中復原；啟用或停用會先備份到 ~/.quosparr/backups，再修改該 CLI 自己的設定檔。", "이 CLI가 읽는 전역 Skill 폴더를 보여 주며 프로젝트 Skill은 제외합니다. 삭제하면 폴더가 Quosparr Skill 휴지통으로 이동해 설정 > Skill 관리에서 복원할 수 있고, 켜기·끄기는 ~/.quosparr/backups에 백업한 뒤 CLI 자체 설정 파일을 수정합니다.", "このCLIが読むグローバルSkillフォルダを一覧表示します（プロジェクトSkillは含みません）。削除するとフォルダはQuosparrのSkillゴミ箱へ移動し、設定 > Skill管理から復元できます。オン・オフは ~/.quosparr/backups にバックアップしてからCLI自身の設定ファイルを編集します。", "Показывает глобальные папки навыков этого CLI, без навыков проектов. Удаление перемещает папку в корзину навыков Quosparr, откуда её можно восстановить в «Настройки > Навыки»; включение и отключение меняют файл настроек самого CLI после резервной копии в ~/.quosparr/backups.", "Liste les dossiers de skills globaux lus par ce CLI, sans les skills de projet. Supprimer déplace le dossier dans la corbeille de skills de Quosparr, d'où Réglages > Skills peut le restaurer ; activer ou désactiver modifie le fichier de réglages du CLI après une sauvegarde dans ~/.quosparr/backups."},
		xSkillsScanFailed:        {"Could not read the skill folders", "无法读取 Skill 目录", "無法讀取 Skill 目錄", "Skill 폴더를 읽을 수 없습니다", "Skillフォルダを読み込めません", "Не удалось прочитать папки навыков", "Impossible de lire les dossiers de skills"},

		xSkillsScopeSystem:  {"System (installed by Codex)", "系统（Codex 内置）", "系統（Codex 內建）", "시스템 (Codex 기본 제공)", "システム（Codex内蔵）", "Системные (из Codex)", "Système (installés par Codex)"},
		xSkillsScopeBundled: {"Bundled with Grok", "Grok 内置", "Grok 內建", "Grok 기본 제공", "Grok同梱", "Встроенные в Grok", "Fournis avec Grok"},
		xSkillsScopeAdmin:   {"Managed by the administrator", "管理员托管", "管理員託管", "관리자 관리", "管理者による管理", "Управляются администратором", "Gérés par l'administrateur"},
		xSkillsScopePlugin:  {"Plugins", "插件", "外掛", "플러그인", "プラグイン", "Плагины", "Plugins"},
		xSkillsScopeSynced:  {"Synced from claude.ai", "从 claude.ai 同步", "從 claude.ai 同步", "claude.ai에서 동기화", "claude.aiから同期", "Синхронизированы с claude.ai", "Synchronisés depuis claude.ai"},

		xSkillsStateOff:         {"Off", "已停用", "已停用", "꺼짐", "オフ", "Отключён", "Désactivé"},
		xSkillsStateOverridden:  {"Overridden", "被覆盖", "被覆蓋", "덮어씀", "上書き", "Перекрыт", "Remplacé"},
		xSkillsStateNotLoaded:   {"Not loaded", "未加载", "未載入", "불러오지 않음", "読み込まれない", "Не загружается", "Non chargé"},
		xSkillsReasonConfigPath: {"Turned off by [[skills.config]] path in config.toml", "被 config.toml 中的 [[skills.config]] path 停用", "被 config.toml 中的 [[skills.config]] path 停用", "config.toml의 [[skills.config]] path로 꺼짐", "config.toml の [[skills.config]] path でオフ", "Отключён записью [[skills.config]] path в config.toml", "Désactivé par [[skills.config]] path dans config.toml"},
		xSkillsReasonConfigName: {"Turned off by [[skills.config]] name in config.toml", "被 config.toml 中的 [[skills.config]] name 停用", "被 config.toml 中的 [[skills.config]] name 停用", "config.toml의 [[skills.config]] name으로 꺼짐", "config.toml の [[skills.config]] name でオフ", "Отключён записью [[skills.config]] name в config.toml", "Désactivé par [[skills.config]] name dans config.toml"},
		xSkillsReasonBundledOff: {"Built-in skills are turned off in the CLI settings", "CLI 配置中关闭了内置 Skill", "CLI 設定中關閉了內建 Skill", "CLI 설정에서 기본 제공 Skill이 꺼져 있습니다", "CLI設定で内蔵Skillがオフになっています", "Встроенные навыки отключены в настройках CLI", "Les skills intégrés sont désactivés dans les réglages du CLI"},
		xSkillsReasonOverride:   {"Turned off by skillOverrides in settings.json", "被 settings.json 中的 skillOverrides 停用", "被 settings.json 中的 skillOverrides 停用", "settings.json의 skillOverrides로 꺼짐", "settings.json の skillOverrides でオフ", "Отключён параметром skillOverrides в settings.json", "Désactivé par skillOverrides dans settings.json"},
		xSkillsReasonDeny:       {"Denied by a Skill(…) rule in permissions.deny of settings.json", "被 settings.json 中 permissions.deny 的 Skill(…) 规则禁止", "被 settings.json 中 permissions.deny 的 Skill(…) 規則禁止", "settings.json permissions.deny의 Skill(…) 규칙으로 차단됨", "settings.json の permissions.deny にある Skill(…) ルールで拒否", "Запрещён правилом Skill(…) в permissions.deny файла settings.json", "Refusé par une règle Skill(…) de permissions.deny dans settings.json"},
		xSkillsReasonDisabled:   {"Listed in [skills].disabled of config.toml", "已列入 config.toml 的 [skills].disabled", "已列入 config.toml 的 [skills].disabled", "config.toml의 [skills].disabled에 있음", "config.toml の [skills].disabled に記載", "Указан в [skills].disabled файла config.toml", "Présent dans [skills].disabled de config.toml"},
		xSkillsReasonIgnored:    {"Hidden by [skills].ignore of config.toml", "被 config.toml 的 [skills].ignore 隐藏", "被 config.toml 的 [skills].ignore 隱藏", "config.toml의 [skills].ignore로 숨겨짐", "config.toml の [skills].ignore で非表示", "Скрыт параметром [skills].ignore в config.toml", "Masqué par [skills].ignore de config.toml"},
		xSkillsReasonCompatOff:  {"Grok does not read this folder ([compat] skills = false)", "Grok 未读取此目录（[compat] skills = false）", "Grok 未讀取此目錄（[compat] skills = false）", "Grok이 이 폴더를 읽지 않음 ([compat] skills = false)", "Grokはこのフォルダを読みません（[compat] skills = false）", "Grok не читает эту папку ([compat] skills = false)", "Grok ne lit pas ce dossier ([compat] skills = false)"},
		xSkillsReasonPluginOff:  {"Its plugin is turned off", "所属插件未启用", "所屬外掛未啟用", "소속 플러그인이 꺼져 있습니다", "所属プラグインがオフです", "Его плагин отключён", "Son plugin est désactivé"},
		xSkillsReasonShadowed:   {"A skill with the same name and a higher priority is used instead", "存在优先级更高的同名 Skill，实际使用的是它", "存在優先順序更高的同名 Skill，實際使用的是它", "우선순위가 더 높은 같은 이름의 Skill이 대신 사용됩니다", "優先度の高い同名のSkillが代わりに使われます", "Вместо него используется одноимённый навык с более высоким приоритетом", "Un skill du même nom, plus prioritaire, est utilisé à la place"},
		xSkillsReasonNested:     {"Claude Code only loads skills/<name>/SKILL.md; this one is nested deeper", "Claude Code 只加载 skills/<name>/SKILL.md，此 Skill 嵌套更深", "Claude Code 只載入 skills/<name>/SKILL.md，此 Skill 巢狀更深", "Claude Code는 skills/<name>/SKILL.md만 불러오며 이 Skill은 더 깊이 중첩되어 있습니다", "Claude Codeは skills/<name>/SKILL.md のみを読み込みます。このSkillはさらに深い階層にあります", "Claude Code загружает только skills/<name>/SKILL.md, а этот навык вложен глубже", "Claude Code ne charge que skills/<name>/SKILL.md ; celui-ci est plus profond"},
		xSkillsReasonTooDeep:    {"Deeper than the CLI scans", "超出 CLI 的扫描深度", "超出 CLI 的掃描深度", "CLI 검색 깊이를 넘었습니다", "CLIのスキャン深度を超えています", "Глубже, чем просматривает CLI", "Plus profond que ce que le CLI parcourt"},
		xSkillsReasonNoDesc:     {"Skipped by the CLI because it has no description", "缺少 description，CLI 会跳过", "缺少 description，CLI 會略過", "description이 없어 CLI가 건너뜁니다", "description がないためCLIはスキップします", "Пропускается CLI: нет description", "Ignoré par le CLI faute de description"},
		xSkillsReasonBadFront:   {"The frontmatter of SKILL.md cannot be read", "SKILL.md 的 frontmatter 无法解析", "SKILL.md 的 frontmatter 無法解析", "SKILL.md의 frontmatter를 읽을 수 없습니다", "SKILL.md のフロントマターを読み取れません", "Не удаётся прочитать frontmatter в SKILL.md", "Le frontmatter de SKILL.md est illisible"},
		xSkillsReasonHidden:     {"Inside a hidden folder, which the CLI skips", "位于 CLI 会跳过的隐藏目录中", "位於 CLI 會略過的隱藏目錄中", "CLI가 건너뛰는 숨김 폴더 안에 있습니다", "CLIがスキップする隠しフォルダ内にあります", "Находится в скрытой папке, которую CLI пропускает", "Dans un dossier caché, ignoré par le CLI"},

		xSkillsEnabled:          {"Enabled", "启用", "啟用", "사용", "有効", "Включён", "Activé"},
		xSkillsToggleCodex:      {"Saved to [[skills.config]] in config.toml after a backup; restart Codex to apply", "备份后写入 config.toml 的 [[skills.config]]，重启 Codex 后生效", "備份後寫入 config.toml 的 [[skills.config]]，重新啟動 Codex 後生效", "백업 후 config.toml의 [[skills.config]]에 저장되며 Codex를 다시 시작하면 적용됩니다", "バックアップ後に config.toml の [[skills.config]] へ保存し、Codexの再起動で反映されます", "Сохраняется в [[skills.config]] файла config.toml после резервной копии; перезапустите Codex", "Enregistré dans [[skills.config]] de config.toml après sauvegarde ; redémarrez Codex pour l'appliquer"},
		xSkillsToggleClaude:     {"Saved to skillOverrides in settings.json after a backup; new Claude Code sessions use it", "备份后写入 settings.json 的 skillOverrides，新的 Claude Code 会话生效", "備份後寫入 settings.json 的 skillOverrides，新的 Claude Code 工作階段生效", "백업 후 settings.json의 skillOverrides에 저장되며 새 Claude Code 세션부터 적용됩니다", "バックアップ後に settings.json の skillOverrides へ保存し、新しいClaude Codeセッションから反映されます", "Сохраняется в skillOverrides файла settings.json после резервной копии; действует в новых сеансах Claude Code", "Enregistré dans skillOverrides de settings.json après sauvegarde ; s'applique aux nouvelles sessions Claude Code"},
		xSkillsToggleGrok:       {"Saved to [skills].disabled in config.toml after a backup; it applies to every skill with this name; restart Grok to apply", "备份后写入 config.toml 的 [skills].disabled，对所有同名 Skill 生效，重启 Grok 后生效", "備份後寫入 config.toml 的 [skills].disabled，對所有同名 Skill 生效，重新啟動 Grok 後生效", "백업 후 config.toml의 [skills].disabled에 저장되며 같은 이름의 모든 Skill에 적용됩니다. Grok을 다시 시작하면 적용됩니다", "バックアップ後に config.toml の [skills].disabled へ保存し、同名のすべてのSkillに適用されます。Grokの再起動で反映されます", "Сохраняется в [skills].disabled файла config.toml после резервной копии и действует на все одноимённые навыки; перезапустите Grok", "Enregistré dans [skills].disabled de config.toml après sauvegarde ; s'applique à tous les skills de ce nom ; redémarrez Grok"},
		xSkillsToggleFailed:     {"Could not save: %s", "保存失败：%s", "儲存失敗：%s", "저장하지 못했습니다: %s", "保存できませんでした：%s", "Не удалось сохранить: %s", "Enregistrement impossible : %s"},
		xSkillsBlockPlugin:      {"Part of a plugin; turn the plugin on or off in the CLI", "属于插件，请在 CLI 中启停该插件", "屬於外掛，請在 CLI 中啟停該外掛", "플러그인의 일부이므로 CLI에서 플러그인을 켜거나 끄세요", "プラグインの一部です。CLIでプラグインをオン・オフしてください", "Входит в плагин; включайте и отключайте плагин в CLI", "Fait partie d'un plugin ; activez ou désactivez le plugin dans le CLI"},
		xSkillsBlockReadOnly:    {"This location cannot be switched here", "此位置不支持在这里启停", "此位置不支援在這裡啟停", "이 위치는 여기서 켜거나 끌 수 없습니다", "この場所はここではオン・オフできません", "Это расположение нельзя переключить здесь", "Cet emplacement ne peut pas être modifié ici"},
		xSkillsBlockDeny:        {"Remove the Skill(…) rule from permissions.deny in settings.json to turn it on", "需先从 settings.json 的 permissions.deny 中移除 Skill(…) 规则才能启用", "需先從 settings.json 的 permissions.deny 中移除 Skill(…) 規則才能啟用", "켜려면 settings.json permissions.deny에서 Skill(…) 규칙을 제거하세요", "オンにするには settings.json の permissions.deny から Skill(…) ルールを削除してください", "Чтобы включить, удалите правило Skill(…) из permissions.deny в settings.json", "Pour l'activer, retirez la règle Skill(…) de permissions.deny dans settings.json"},
		xSkillsBlockIgnored:     {"Remove its path from [skills].ignore in config.toml to show it again", "需从 config.toml 的 [skills].ignore 中移除该路径才能恢复", "需從 config.toml 的 [skills].ignore 中移除該路徑才能恢復", "다시 보이게 하려면 config.toml [skills].ignore에서 경로를 제거하세요", "再表示するには config.toml の [skills].ignore からパスを削除してください", "Чтобы вернуть, удалите путь из [skills].ignore в config.toml", "Pour le réafficher, retirez son chemin de [skills].ignore dans config.toml"},
		xSkillsBlockOther:       {"The CLI does not load it, so there is nothing to switch", "CLI 不会加载它，无需启停", "CLI 不會載入它，無需啟停", "CLI가 불러오지 않으므로 켜고 끌 대상이 없습니다", "CLIが読み込まないため、オン・オフの対象ではありません", "CLI его не загружает, переключать нечего", "Le CLI ne le charge pas ; rien à activer"},
		xSkillsReadOnlySystem:   {"Installed and restored by Codex; it cannot be deleted", "由 Codex 安装并自动恢复，不能删除", "由 Codex 安裝並自動恢復，不能刪除", "Codex가 설치하고 복원하므로 삭제할 수 없습니다", "Codexがインストールし自動で復元するため削除できません", "Устанавливается и восстанавливается Codex; удалить нельзя", "Installé et restauré par Codex ; suppression impossible"},
		xSkillsReadOnlyBundled:  {"Ships with Grok; turn it off instead of deleting it", "随 Grok 提供，可停用但不能删除", "隨 Grok 提供，可停用但不能刪除", "Grok에 포함되어 있어 삭제 대신 끌 수 있습니다", "Grokに同梱されています。削除せずオフにしてください", "Поставляется с Grok; отключите вместо удаления", "Fourni avec Grok ; désactivez-le au lieu de le supprimer"},
		xSkillsReadOnlyAdmin:    {"Managed by the administrator; it cannot be deleted here", "由管理员托管，不能在这里删除", "由管理員託管，不能在這裡刪除", "관리자가 관리하므로 여기서 삭제할 수 없습니다", "管理者が管理しているため、ここでは削除できません", "Управляется администратором; удалить здесь нельзя", "Géré par l'administrateur ; suppression impossible ici"},
		xSkillsReadOnlySynced:   {"Synced from claude.ai and overwritten on the next sync; it cannot be deleted here", "从 claude.ai 同步，下次同步会被覆盖，不能在这里删除", "從 claude.ai 同步，下次同步會被覆蓋，不能在這裡刪除", "claude.ai에서 동기화되며 다음 동기화 때 덮어써지므로 여기서 삭제할 수 없습니다", "claude.aiから同期され次回の同期で上書きされるため、ここでは削除できません", "Синхронизируется с claude.ai и перезаписывается; удалить здесь нельзя", "Synchronisé depuis claude.ai et écrasé à la prochaine synchronisation ; suppression impossible ici"},
		xSkillsReadOnlyPluginCX: {"Installed by a Codex plugin; remove the plugin with `codex plugin`", "由 Codex 插件安装，请用 `codex plugin` 移除插件", "由 Codex 外掛安裝，請用 `codex plugin` 移除外掛", "Codex 플러그인으로 설치됨. `codex plugin`으로 플러그인을 제거하세요", "Codexプラグインでインストールされています。`codex plugin` でプラグインを削除してください", "Установлен плагином Codex; удалите плагин через `codex plugin`", "Installé par un plugin Codex ; retirez le plugin avec `codex plugin`"},
		xSkillsReadOnlyPluginCL: {"Installed by a Claude Code plugin; remove it with `claude plugin uninstall %s`", "由 Claude Code 插件安装，请用 `claude plugin uninstall %s` 卸载", "由 Claude Code 外掛安裝，請用 `claude plugin uninstall %s` 解除安裝", "Claude Code 플러그인으로 설치됨. `claude plugin uninstall %s`로 제거하세요", "Claude Codeプラグインでインストールされています。`claude plugin uninstall %s` で削除してください", "Установлен плагином Claude Code; удалите командой `claude plugin uninstall %s`", "Installé par un plugin Claude Code ; retirez-le avec `claude plugin uninstall %s`"},
		xSkillsReadOnlyPluginGM: {"Installed by an Antigravity plugin; manage it in Antigravity",
			"由 Antigravity 插件安装，请在 Antigravity 中管理",
			"由 Antigravity 外掛安裝，請在 Antigravity 中管理",
			"Antigravity 플러그인으로 설치됨. Antigravity에서 관리하세요",
			"Antigravity プラグインでインストールされています。Antigravity で管理してください",
			"Установлен плагином Antigravity; управляйте им в Antigravity",
			"Installé par un plugin Antigravity ; gérez-le dans Antigravity"},
		xSkillsBlockNoToggle: {"Antigravity cannot turn a single skill on or off; move or delete the skill folder instead",
			"Antigravity 不支持单独开关 Skill，可移动或删除该 Skill 文件夹",
			"Antigravity 不支援單獨開關 Skill，可移動或刪除該 Skill 資料夾",
			"Antigravity는 Skill을 개별로 켜고 끌 수 없습니다. Skill 폴더를 옮기거나 삭제하세요",
			"Antigravity は Skill を個別にオン・オフできません。Skill フォルダを移動または削除してください",
			"Antigravity не умеет включать и отключать отдельный навык; переместите или удалите его папку",
			"Antigravity ne peut pas activer ou désactiver un skill seul ; déplacez ou supprimez plutôt son dossier"},
		xSkillsReadOnlyPluginGK: {"Installed by a Grok plugin; manage it in Grok", "由 Grok 插件安装，请在 Grok 中管理", "由 Grok 外掛安裝，請在 Grok 中管理", "Grok 플러그인으로 설치됨. Grok에서 관리하세요", "Grokプラグインでインストールされています。Grokで管理してください", "Установлен плагином Grok; управляйте им в Grok", "Installé par un plugin Grok ; gérez-le dans Grok"},

		xSkillsLocation:    {"Location", "位置", "位置", "위치", "場所", "Расположение", "Emplacement"},
		xSkillsSource:      {"Source", "来源", "來源", "출처", "ソース", "Источник", "Source"},
		xSkillsInvoke:      {"Invoke as", "调用名", "呼叫名稱", "호출 이름", "呼び出し名", "Вызов", "Appel"},
		xSkillsModified:    {"Modified", "修改时间", "修改時間", "수정 시각", "更新日時", "Изменён", "Modifié"},
		xSkillsSharedWith:  {"Also read by", "同时被以下 CLI 读取", "同時被以下 CLI 讀取", "함께 읽는 CLI", "ほかに読み込むCLI", "Также читают", "Lu aussi par"},
		xSkillsLinkTarget:  {"Link target", "链接目标", "連結目標", "링크 대상", "リンク先", "Цель ссылки", "Cible du lien"},
		xSkillsOverridden:  {"Overridden by", "被以下覆盖", "被以下覆蓋", "덮어쓴 Skill", "上書き元", "Перекрыт", "Remplacé par"},
		xSkillsPlugin:      {"Plugin", "插件", "外掛", "플러그인", "プラグイン", "Плагин", "Plugin"},
		xSkillsState:       {"State", "状态", "狀態", "상태", "状態", "Состояние", "État"},
		xSkillsStateOn:     {"On", "已启用", "已啟用", "켜짐", "オン", "Включён", "Activé"},
		xSkillsOpenFolder:  {"Open folder", "打开所在目录", "打開所在目錄", "폴더 열기", "フォルダを開く", "Открыть папку", "Ouvrir le dossier"},
		xSkillsReveal:      {"Reveal SKILL.md", "在文件夹中显示 SKILL.md", "在檔案夾中顯示 SKILL.md", "폴더에서 SKILL.md 보기", "フォルダで SKILL.md を表示", "Показать SKILL.md в папке", "Afficher SKILL.md dans le dossier"},
		xSkillsShowContent: {"Show SKILL.md content", "显示 SKILL.md 内容", "顯示 SKILL.md 內容", "SKILL.md 내용 보기", "SKILL.md の内容を表示", "Показать содержимое SKILL.md", "Afficher le contenu de SKILL.md"},
		xSkillsHideContent: {"Hide SKILL.md content", "收起 SKILL.md 内容", "收合 SKILL.md 內容", "SKILL.md 내용 숨기기", "SKILL.md の内容を隠す", "Скрыть содержимое SKILL.md", "Masquer le contenu de SKILL.md"},
		xSkillsTruncated:   {"Only the first 256 KB is shown", "仅显示前 256 KB", "僅顯示前 256 KB", "처음 256KB만 표시합니다", "先頭の256 KBのみ表示しています", "Показаны только первые 256 КБ", "Seuls les 256 premiers Ko sont affichés"},
		xSkillsNoContent:   {"SKILL.md has no content after the frontmatter", "SKILL.md 在 frontmatter 之后没有内容", "SKILL.md 在 frontmatter 之後沒有內容", "SKILL.md에 frontmatter 이후 내용이 없습니다", "SKILL.md にはフロントマター以降の内容がありません", "В SKILL.md нет текста после frontmatter", "SKILL.md n'a pas de contenu après le frontmatter"},
		xSkillsReadFailed:  {"Could not read SKILL.md: %s", "无法读取 SKILL.md：%s", "無法讀取 SKILL.md：%s", "SKILL.md를 읽을 수 없습니다: %s", "SKILL.md を読み込めません：%s", "Не удалось прочитать SKILL.md: %s", "Impossible de lire SKILL.md : %s"},
		xSkillsOpenFailed:  {"Could not open the folder: %s", "无法打开目录：%s", "無法打開目錄：%s", "폴더를 열 수 없습니다: %s", "フォルダを開けません：%s", "Не удалось открыть папку: %s", "Impossible d'ouvrir le dossier : %s"},

		xSkillsDelete:        {"Move to Trash", "移到回收站", "移到資源回收筒", "휴지통으로 이동", "ゴミ箱に移動", "В корзину", "Mettre à la corbeille"},
		xSkillsDeleteTitle:   {"Move this skill to the Trash?", "将此 Skill 移到回收站？", "將此 Skill 移到資源回收筒？", "이 Skill을 휴지통으로 이동할까요?", "このSkillをゴミ箱に移動しますか？", "Переместить этот навык в корзину?", "Mettre ce skill à la corbeille ?"},
		xSkillsDeleteMessage: {"%s goes to the Quosparr skills trash, where it can be restored.", "%s 将移到 Quosparr 的 Skill 回收站，可从中恢复。", "%s 將移到 Quosparr 的 Skill 回收站，可從中復原。", "%s이(가) Quosparr Skill 휴지통으로 이동하며 그곳에서 복원할 수 있습니다.", "%s はQuosparrのSkillゴミ箱に移動し、そこから復元できます。", "%s будет перемещён в корзину навыков Quosparr, откуда его можно восстановить.", "%s ira dans la corbeille de skills de Quosparr, d'où il pourra être restauré."},
		xSkillsDeleteLink:    {"Only the symbolic link is removed; the folder it points to stays.", "只移除符号链接本身，指向的目录保留。", "只移除符號連結本身，指向的目錄保留。", "심볼릭 링크만 제거되며 가리키는 폴더는 남습니다.", "シンボリックリンクだけを削除し、リンク先のフォルダは残ります。", "Удаляется только символическая ссылка; папка, на которую она указывает, остаётся.", "Seul le lien symbolique est retiré ; le dossier visé reste."},
		xSkillsDeleteCovers:  {"These skills go with it: %s", "以下 Skill 会一并移除：%s", "以下 Skill 會一併移除：%s", "다음 Skill도 함께 제거됩니다: %s", "次のSkillも一緒に削除されます：%s", "Вместе с ним будут удалены: %s", "Ces skills partent avec lui : %s"},
		xSkillsDeleteShared:  {"%s also reads this folder and will lose it too.", "%s 也读取此目录，同样会失去它。", "%s 也讀取此目錄，同樣會失去它。", "%s도 이 폴더를 읽으므로 함께 사라집니다.", "%s もこのフォルダを読み込んでいるため、同じく使えなくなります。", "%s тоже читает эту папку и тоже его лишится.", "%s lit aussi ce dossier et le perdra également."},
		xSkillsDeleteRestart: {"A CLI that is already running may keep using it until it restarts.", "已在运行的 CLI 可能在重启前仍会使用它。", "已在執行的 CLI 可能在重新啟動前仍會使用它。", "이미 실행 중인 CLI는 다시 시작할 때까지 계속 사용할 수 있습니다.", "実行中のCLIは再起動するまで使い続ける場合があります。", "Уже запущенный CLI может использовать его до перезапуска.", "Un CLI déjà lancé peut continuer à l'utiliser jusqu'à son redémarrage."},
		xSkillsDeleteFailed:  {"Could not move to the Trash: %s", "移到回收站失败：%s", "移到資源回收筒失敗：%s", "휴지통으로 이동하지 못했습니다: %s", "ゴミ箱に移動できませんでした：%s", "Не удалось переместить в корзину: %s", "Impossible de mettre à la corbeille : %s"},
		xSkillsChanged:       {"The skill changed on disk; the list was refreshed", "Skill 已在磁盘上变化，列表已刷新", "Skill 已在磁碟上變化，清單已重新整理", "디스크의 Skill이 바뀌어 목록을 새로 고쳤습니다", "ディスク上のSkillが変更されたため、一覧を更新しました", "Навык изменился на диске; список обновлён", "Le skill a changé sur le disque ; la liste a été actualisée"},

		xSkillsIssuesTitle:     {"Diagnostics", "诊断", "診斷", "진단", "診断", "Диагностика", "Diagnostic"},
		xSkillsIssueBrokenLink: {"Broken link: the target no longer exists", "失效链接：目标已不存在", "失效連結：目標已不存在", "끊어진 링크: 대상이 없습니다", "リンク切れ：リンク先がありません", "Битая ссылка: цели больше нет", "Lien cassé : la cible n'existe plus"},
		xSkillsIssueNested:     {"Not loaded: nested deeper than the CLI reads", "未加载：嵌套层级超出 CLI 的读取范围", "未載入：巢狀層級超出 CLI 的讀取範圍", "불러오지 않음: CLI가 읽는 범위보다 깊습니다", "読み込まれない：CLIが読む階層より深い", "Не загружается: вложен глубже, чем читает CLI", "Non chargé : plus profond que ce que lit le CLI"},
		xSkillsIssueNoDesc:     {"No description in the frontmatter", "frontmatter 缺少 description", "frontmatter 缺少 description", "frontmatter에 description이 없습니다", "フロントマターに description がありません", "В frontmatter нет description", "Pas de description dans le frontmatter"},
		xSkillsIssueBadFront:   {"The frontmatter cannot be read", "frontmatter 无法解析", "frontmatter 無法解析", "frontmatter를 읽을 수 없습니다", "フロントマターを読み取れません", "frontmatter не читается", "Le frontmatter est illisible"},
		xSkillsIssueLongName:   {"The name is longer than the CLI accepts", "名称超出 CLI 允许的长度", "名稱超出 CLI 允許的長度", "이름이 CLI 허용 길이보다 깁니다", "名前がCLIの許容長を超えています", "Имя длиннее, чем допускает CLI", "Le nom dépasse la longueur acceptée par le CLI"},
		xSkillsIssueDuplicate:  {"Several skills share this name", "多个 Skill 使用了相同名称", "多個 Skill 使用了相同名稱", "여러 Skill이 같은 이름을 사용합니다", "複数のSkillが同じ名前を使っています", "Несколько навыков с одним именем", "Plusieurs skills portent ce nom"},
		xSkillsIssueTruncated:  {"Too many folders; only part of this location was scanned", "目录过多，此位置只扫描了一部分", "目錄過多，此位置只掃描了一部分", "폴더가 너무 많아 이 위치의 일부만 검색했습니다", "フォルダが多すぎるため、この場所の一部のみスキャンしました", "Слишком много папок; просмотрена только часть", "Trop de dossiers ; seule une partie a été parcourue"},
		xSkillsRemoveLink:      {"Remove link", "移除链接", "移除連結", "링크 제거", "リンクを削除", "Удалить ссылку", "Retirer le lien"},
		xSkillsRemoveLinkTitle: {"Remove this broken link?", "移除此失效链接？", "移除此失效連結？", "끊어진 링크를 제거할까요?", "このリンク切れを削除しますか？", "Удалить битую ссылку?", "Retirer ce lien cassé ?"},
		xSkillsRemoveLinkMsg:   {"The link %s goes to the Quosparr skills trash.", "链接 %s 将移到 Quosparr 的 Skill 回收站。", "連結 %s 將移到 Quosparr 的 Skill 回收站。", "링크 %s이(가) Quosparr Skill 휴지통으로 이동합니다.", "リンク %s はQuosparrのSkillゴミ箱に移動します。", "Ссылка %s будет перемещена в корзину навыков Quosparr.", "Le lien %s ira dans la corbeille de skills de Quosparr."},
	} {
		extraTexts[key] = texts
	}
}
