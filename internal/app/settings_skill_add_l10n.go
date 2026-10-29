package app

const (
	xSkillAdd                 textKey = "skillAddAdd"
	xSkillAddStepSource       textKey = "skillAddStepSource"
	xSkillAddStepPick         textKey = "skillAddStepPick"
	xSkillAddStepReview       textKey = "skillAddStepReview"
	xSkillAddReading          textKey = "skillAddReading"
	xSkillAddDownloading      textKey = "skillAddDownloading"
	xSkillAddDownloadingSize  textKey = "skillAddDownloadingSize"
	xSkillAddSelected         textKey = "skillAddSelected"
	xSkillAddChecking         textKey = "skillAddChecking"
	xSkillAddSubmit           textKey = "skillAddSubmit"
	xSkillAddRetry            textKey = "skillAddRetry"
	xSkillAddFolder           textKey = "skillAddFolder"
	xSkillAddZip              textKey = "skillAddZip"
	xSkillAddRepo             textKey = "skillAddRepo"
	xSkillAddRepoHint         textKey = "skillAddRepoHint"
	xSkillAddFolderPath       textKey = "skillAddFolderPath"
	xSkillAddFolderHint       textKey = "skillAddFolderHint"
	xSkillAddZipPath          textKey = "skillAddZipPath"
	xSkillAddZipHint          textKey = "skillAddZipHint"
	xSkillAddPathHint         textKey = "skillAddPathHint"
	xSkillAddChoose           textKey = "skillAddChoose"
	xSkillAddChooseFolder     textKey = "skillAddChooseFolder"
	xSkillAddChooseZip        textKey = "skillAddChooseZip"
	xSkillAddMemoryOnly       textKey = "skillAddMemoryOnly"
	xSkillAddScanHint         textKey = "skillAddScanHint"
	xSkillAddGitHubHint       textKey = "skillAddGitHubHint"
	xSkillAddFound            textKey = "skillAddFound"
	xSkillAddTruncated        textKey = "skillAddTruncated"
	xSkillAddRejected         textKey = "skillAddRejected"
	xSkillAddSelectAll        textKey = "skillAddSelectAll"
	xSkillAddPickOne          textKey = "skillAddPickOne"
	xSkillAddPlugin           textKey = "skillAddPlugin"
	xSkillAddExec             textKey = "skillAddExec"
	xSkillAddExecHint         textKey = "skillAddExecHint"
	xSkillAddFiles            textKey = "skillAddFiles"
	xSkillAddShowDetails      textKey = "skillAddShowDetails"
	xSkillAddHideDetails      textKey = "skillAddHideDetails"
	xSkillAddSkippedLinks     textKey = "skillAddSkippedLinks"
	xSkillAddBodyCut          textKey = "skillAddBodyCut"
	xSkillAddProblemFront     textKey = "skillAddProblemFront"
	xSkillAddProblemDesc      textKey = "skillAddProblemDesc"
	xSkillAddProblemName      textKey = "skillAddProblemName"
	xSkillAddProblemPath      textKey = "skillAddProblemPath"
	xSkillAddTargets          textKey = "skillAddTargets"
	xSkillAddTargetsHint      textKey = "skillAddTargetsHint"
	xSkillAddShared           textKey = "skillAddShared"
	xSkillAddReaders          textKey = "skillAddReaders"
	xSkillAddSharedOverlap    textKey = "skillAddSharedOverlap"
	xSkillAddReloadHint       textKey = "skillAddReloadHint"
	xSkillAddTargetDone       textKey = "skillAddTargetDone"
	xSkillAddTargetBlocked    textKey = "skillAddTargetBlocked"
	xSkillAddCounts           textKey = "skillAddCounts"
	xSkillAddWillAdd          textKey = "skillAddWillAdd"
	xSkillAddExists           textKey = "skillAddExists"
	xSkillAddInstalled        textKey = "skillAddInstalled"
	xSkillAddInvalid          textKey = "skillAddInvalid"
	xSkillAddClash            textKey = "skillAddClash"
	xSkillAddWriting          textKey = "skillAddWriting"
	xSkillAddResultsDone      textKey = "skillAddResultsDone"
	xSkillAddResultsFailed    textKey = "skillAddResultsFailed"
	xSkillAddNothing          textKey = "skillAddNothing"
	xSkillAddRecordFailed     textKey = "skillAddRecordFailed"
	xSkillAddFailed           textKey = "skillAddFailed"
	xSkillAddErrSource        textKey = "skillAddErrSource"
	xSkillAddErrCancelled     textKey = "skillAddErrCancelled"
	xSkillAddErrInvalidSource textKey = "skillAddErrInvalidSource"
	xSkillAddErrNotFound      textKey = "skillAddErrNotFound"
	xSkillAddErrRateLimited   textKey = "skillAddErrRateLimited"
	xSkillAddErrNetwork       textKey = "skillAddErrNetwork"
	xSkillAddErrHTTP          textKey = "skillAddErrHTTP"
	xSkillAddErrTooLarge      textKey = "skillAddErrTooLarge"
	xSkillAddErrTooManyFiles  textKey = "skillAddErrTooManyFiles"
	xSkillAddErrInvalidPath   textKey = "skillAddErrInvalidPath"
	xSkillAddErrArchive       textKey = "skillAddErrArchive"
	xSkillAddErrCase          textKey = "skillAddErrCase"
	xSkillAddErrNoSkills      textKey = "skillAddErrNoSkills"
	xSkillAddErrExists        textKey = "skillAddErrExists"
	xSkillAddErrUnsafeRoot    textKey = "skillAddErrUnsafeRoot"
	xSkillAddErrChanged       textKey = "skillAddErrChanged"
	xSkillAddErrWrite         textKey = "skillAddErrWrite"
	xSkillAddErrNotUpdatable  textKey = "skillAddErrNotUpdatable"
	xSkillAddErrModified      textKey = "skillAddErrModified"
	xSkillAddErrMissing       textKey = "skillAddErrMissing"
	xSkillsInstQuosparr       textKey = "skillsInstQuosparr"
	xSkillsCommit             textKey = "skillsCommit"
	xSkillsImportedFrom       textKey = "skillsImportedFrom"
	xSkillUpdateCheck         textKey = "skillUpdateCheck"
	xSkillUpdateChecking      textKey = "skillUpdateChecking"
	xSkillUpdateCurrent       textKey = "skillUpdateCurrent"
	xSkillUpdateAvailable     textKey = "skillUpdateAvailable"
	xSkillUpdate              textKey = "skillUpdateUpdate"
	xSkillUpdateForce         textKey = "skillUpdateForce"
	xSkillUpdateTitle         textKey = "skillUpdateTitle"
	xSkillUpdateMessage       textKey = "skillUpdateMessage"
	xSkillUpdateForceMessage  textKey = "skillUpdateForceMessage"
	xSkillUpdated             textKey = "skillUpdateUpdated"
	xSkillUpdatedSystem       textKey = "skillUpdateUpdatedSystem"
	xSkillUpdateUnchanged     textKey = "skillUpdateUnchanged"
	xSkillUpdateFailed        textKey = "skillUpdateFailed"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xSkillAdd:                 {"Add Skill", "添加 Skill", "新增 Skill", "Skill 추가", "Skill を追加", "Добавить навык", "Ajouter un skill"},
		xSkillAddStepSource:       {"Choose source", "选择来源", "選擇來源", "소스 선택", "取得元を選択", "Источник", "Choisir la source"},
		xSkillAddStepPick:         {"Pick skills", "选择 Skill", "選擇 Skill", "Skill 선택", "Skill を選択", "Выбор навыков", "Choisir les skills"},
		xSkillAddStepReview:       {"Confirm install", "确认安装", "確認安裝", "설치 확인", "インストールの確認", "Подтверждение", "Confirmer l’installation"},
		xSkillAddReading:          {"Reading source…", "正在读取来源…", "正在讀取來源…", "소스를 읽는 중…", "取得元を読み込み中…", "Чтение источника…", "Lecture de la source…"},
		xSkillAddDownloading:      {"Downloading the repository… %d%%", "正在下载仓库… %d%%", "正在下載儲存庫… %d%%", "저장소 다운로드 중… %d%%", "リポジトリをダウンロード中… %d%%", "Загрузка репозитория… %d%%", "Téléchargement du dépôt… %d %%"},
		xSkillAddDownloadingSize:  {"Downloading the repository… %s", "正在下载仓库… %s", "正在下載儲存庫… %s", "저장소 다운로드 중… %s", "リポジトリをダウンロード中… %s", "Загрузка репозитория… %s", "Téléchargement du dépôt… %s"},
		xSkillAddSelected:         {"Selected: %d of %d", "已选 %d / %d", "已選 %d / %d", "%d / %d 선택", "%d / %d 件を選択", "Выбрано: %d из %d", "Sélection : %d sur %d"},
		xSkillAddChecking:         {"Checking skills folders…", "正在检查安装位置…", "正在檢查安裝位置…", "Skill 폴더 확인 중…", "インストール先を確認中…", "Проверка папок навыков…", "Vérification des dossiers de skills…"},
		xSkillAddSubmit:           {"Install (%d)", "安装 %d 个", "安裝 %d 個", "%d개 설치", "%d 件をインストール", "Установить (%d)", "Installer (%d)"},
		xSkillAddRetry:            {"Retry failed folders", "重试失败的位置", "重試失敗的位置", "실패한 폴더 재시도", "失敗した場所を再試行", "Повторить для неудачных", "Réessayer les dossiers en échec"},
		xSkillAddFolder:           {"Folder", "本地文件夹", "本機資料夾", "로컬 폴더", "ローカルフォルダー", "Папка", "Dossier local"},
		xSkillAddZip:              {"ZIP file", "ZIP 文件", "ZIP 檔案", "ZIP 파일", "ZIP ファイル", "ZIP-файл", "Fichier ZIP"},
		xSkillAddRepo:             {"GitHub repository", "GitHub 仓库", "GitHub 儲存庫", "GitHub 저장소", "GitHub リポジトリ", "Репозиторий GitHub", "Dépôt GitHub"},
		xSkillAddRepoHint:         {"owner/repo, or a link to a repository, branch or folder on GitHub", "填写 owner/repo，或 GitHub 上仓库、分支、文件夹的链接", "填寫 owner/repo，或 GitHub 上儲存庫、分支、資料夾的連結", "owner/repo 또는 GitHub 저장소·브랜치·폴더 링크", "owner/repo、または GitHub のリポジトリ・ブランチ・フォルダーのリンク", "owner/repo или ссылка на репозиторий, ветку или папку GitHub", "owner/repo, ou le lien d’un dépôt, d’une branche ou d’un dossier GitHub"},
		xSkillAddFolderPath:       {"Folder", "文件夹", "資料夾", "폴더", "フォルダー", "Папка", "Dossier"},
		xSkillAddFolderHint:       {"One skill folder, or a folder holding several", "单个 Skill 文件夹，或包含多个 Skill 的文件夹", "單一 Skill 資料夾，或包含多個 Skill 的資料夾", "Skill 폴더 하나 또는 여러 Skill이 든 폴더", "1 つの Skill フォルダー、または複数を含むフォルダー", "Папка одного навыка или папка с несколькими", "Le dossier d’un skill, ou un dossier qui en contient plusieurs"},
		xSkillAddZipPath:          {"ZIP file", "ZIP 文件", "ZIP 檔案", "ZIP 파일", "ZIP ファイル", "ZIP-файл", "Fichier ZIP"},
		xSkillAddZipHint:          {"A .zip or .skill archive", "支持 .zip 或 .skill 压缩包", "支援 .zip 或 .skill 壓縮檔", ".zip 또는 .skill 압축 파일", ".zip または .skill アーカイブ", "Архив .zip или .skill", "Archive .zip ou .skill"},
		xSkillAddPathHint:         {"Type a path or choose one", "输入路径或点击选择", "輸入路徑或點擊選擇", "경로를 입력하거나 선택", "パスを入力するか選択", "Введите путь или выберите", "Saisissez un chemin ou choisissez-en un"},
		xSkillAddChoose:           {"Choose…", "选择…", "選擇…", "선택…", "選択…", "Выбрать…", "Choisir…"},
		xSkillAddChooseFolder:     {"Choose a skills folder", "选择 Skill 文件夹", "選擇 Skill 資料夾", "Skill 폴더 선택", "Skill フォルダーを選択", "Выберите папку навыков", "Choisir un dossier de skills"},
		xSkillAddChooseZip:        {"Choose a ZIP file", "选择 ZIP 文件", "選擇 ZIP 檔案", "ZIP 파일 선택", "ZIP ファイルを選択", "Выберите ZIP-файл", "Choisir un fichier ZIP"},
		xSkillAddMemoryOnly:       {"The source is read into memory for a preview; nothing is written before you confirm, and no script in it is run", "来源只读入内存供预览，确认前不写入任何文件，也不会执行其中的脚本", "來源只讀入記憶體供預覽，確認前不寫入任何檔案，也不會執行其中的指令碼", "소스는 미리보기용으로 메모리에만 읽으며, 확인 전에는 아무것도 쓰지 않고 스크립트도 실행하지 않습니다", "取得元はプレビューのためメモリーにのみ読み込み、確認するまで何も書き込まず、スクリプトも実行しません", "Источник читается в память для просмотра; до подтверждения ничего не записывается и скрипты не запускаются", "La source est lue en mémoire pour l’aperçu ; rien n’est écrit avant confirmation et aucun script n’est exécuté"},
		xSkillAddScanHint:         {"SKILL.md files are searched for in nested folders; version control, dependencies and system files are left out", "会在多级目录中查找 SKILL.md，并跳过版本控制、依赖目录和系统文件", "會在多層目錄中尋找 SKILL.md，並略過版本控制、相依目錄與系統檔案", "여러 단계 폴더에서 SKILL.md를 찾고 버전 관리, 의존성, 시스템 파일은 제외합니다", "階層フォルダーから SKILL.md を探し、バージョン管理・依存関係・システムファイルは除外します", "SKILL.md ищется во вложенных папках; системы контроля версий, зависимости и системные файлы пропускаются", "Les SKILL.md sont cherchés dans les sous-dossiers ; gestion de versions, dépendances et fichiers système sont ignorés"},
		xSkillAddGitHubHint:       {"Public repositories only, fetched through the app’s proxy settings", "仅支持公开仓库，按应用代理设置访问 GitHub", "僅支援公開儲存庫，依應用程式代理設定存取 GitHub", "공개 저장소만 지원하며 앱 프록시 설정으로 GitHub에 접속합니다", "公開リポジトリのみ対応、アプリのプロキシ設定で GitHub に接続します", "Только публичные репозитории, через прокси приложения", "Dépôts publics uniquement, via les réglages de proxy de l’app"},
		xSkillAddFound:            {"Skills found: %d", "发现 %d 个 Skill", "發現 %d 個 Skill", "Skill %d개 발견", "Skill %d 件を検出", "Найдено навыков: %d", "Skills trouvés : %d"},
		xSkillAddTruncated:        {"Only the first %d skills are listed", "只列出前 %d 个 Skill", "只列出前 %d 個 Skill", "처음 %d개 Skill만 표시합니다", "最初の %d 件の Skill のみ表示しています", "Показаны только первые %d навыков", "Seuls les %d premiers skills sont listés"},
		xSkillAddRejected:         {"Unsafe or conflicting paths left out: %d", "已忽略 %d 个不安全或冲突的路径", "已略過 %d 個不安全或衝突的路徑", "안전하지 않거나 충돌하는 경로 %d개 제외", "安全でない・競合するパス %d 件を除外しました", "Пропущено небезопасных или конфликтующих путей: %d", "Chemins dangereux ou en conflit ignorés : %d"},
		xSkillAddSelectAll:        {"Select all", "全选", "全選", "모두 선택", "すべて選択", "Выбрать все", "Tout sélectionner"},
		xSkillAddPickOne:          {"Select %s", "选择 %s", "選擇 %s", "%s 선택", "%s を選択", "Выбрать %s", "Sélectionner %s"},
		xSkillAddPlugin:           {"Plugin %s", "插件 %s", "外掛 %s", "플러그인 %s", "プラグイン %s", "Плагин %s", "Plugin %s"},
		xSkillAddExec:             {"Executables: %d", "含 %d 个可执行文件", "含 %d 個可執行檔", "실행 파일 %d개", "実行ファイル %d 件", "Исполняемых: %d", "Exécutables : %d"},
		xSkillAddExecHint:         {"Executable files are installed as they are and run only when a CLI uses the skill; check them before installing", "可执行文件会原样安装，只在 CLI 使用该 Skill 时运行，安装前请先检查", "可執行檔會原樣安裝，只在 CLI 使用該 Skill 時執行，安裝前請先檢查", "실행 파일은 그대로 설치되며 CLI가 Skill을 사용할 때만 실행됩니다. 설치 전에 확인하세요", "実行ファイルはそのままインストールされ、CLI が Skill を使うときだけ実行されます。インストール前に確認してください", "Исполняемые файлы устанавливаются как есть и запускаются только при использовании навыка; проверьте их", "Les exécutables sont installés tels quels et ne s’exécutent que lorsqu’une CLI utilise le skill ; vérifiez-les"},
		xSkillAddFiles:            {"Files: %d · %s", "%d 个文件 · %s", "%d 個檔案 · %s", "파일 %d개 · %s", "ファイル %d 件 · %s", "Файлов: %d · %s", "Fichiers : %d · %s"},
		xSkillAddShowDetails:      {"Show files and SKILL.md of %s", "展开 %s 的文件与 SKILL.md", "展開 %s 的檔案與 SKILL.md", "%s의 파일과 SKILL.md 보기", "%s のファイルと SKILL.md を表示", "Показать файлы и SKILL.md навыка %s", "Afficher les fichiers et le SKILL.md de %s"},
		xSkillAddHideDetails:      {"Hide files and SKILL.md of %s", "收起 %s 的文件与 SKILL.md", "收合 %s 的檔案與 SKILL.md", "%s의 파일과 SKILL.md 숨기기", "%s のファイルと SKILL.md を隠す", "Скрыть файлы и SKILL.md навыка %s", "Masquer les fichiers et le SKILL.md de %s"},
		xSkillAddSkippedLinks:     {"Not installed: links that leave the skill folder", "不会安装：指向 Skill 文件夹之外的链接", "不會安裝：指向 Skill 資料夾之外的連結", "설치하지 않음: Skill 폴더 밖을 가리키는 링크", "インストールしないもの：Skill フォルダーの外を指すリンク", "Не устанавливаются: ссылки за пределы папки навыка", "Non installés : liens pointant hors du dossier du skill"},
		xSkillAddBodyCut:          {"Only the beginning is shown", "仅显示开头部分", "僅顯示開頭部分", "앞부분만 표시합니다", "冒頭のみ表示しています", "Показано только начало", "Seul le début est affiché"},
		xSkillAddProblemFront:     {"Unreadable frontmatter", "frontmatter 无法解析", "frontmatter 無法解析", "frontmatter를 읽을 수 없음", "frontmatter を解析できません", "Не читается frontmatter", "Frontmatter illisible"},
		xSkillAddProblemDesc:      {"No description", "缺少描述", "缺少描述", "설명 없음", "説明がありません", "Нет описания", "Sans description"},
		xSkillAddProblemName:      {"Unusable name", "名称不可用", "名稱無法使用", "사용할 수 없는 이름", "使用できない名前", "Недопустимое имя", "Nom inutilisable"},
		xSkillAddProblemPath:      {"Unsafe or conflicting file paths", "含不安全或冲突的文件路径", "含不安全或衝突的檔案路徑", "안전하지 않거나 충돌하는 파일 경로", "安全でない・競合するファイルパスを含みます", "Небезопасные или конфликтующие пути", "Chemins de fichiers dangereux ou en conflit"},
		xSkillAddTargets:          {"Install to", "安装位置", "安裝位置", "설치 위치", "インストール先", "Куда установить", "Installer dans"},
		xSkillAddTargetsHint:      {"Each folder is checked before anything is written; existing folders are never overwritten", "写入前会逐个检查，已存在的文件夹不会被覆盖", "寫入前會逐一檢查，已存在的資料夾不會被覆寫", "쓰기 전에 각 폴더를 확인하며 기존 폴더는 덮어쓰지 않습니다", "書き込む前にそれぞれ確認し、既存のフォルダーは上書きしません", "Перед записью каждая папка проверяется; существующие не перезаписываются", "Chaque dossier est vérifié avant écriture ; les dossiers existants ne sont jamais écrasés"},
		xSkillAddShared:           {"Shared folder", "共享目录", "共用目錄", "공유 폴더", "共有フォルダー", "Общая папка", "Dossier partagé"},
		xSkillAddReaders:          {"Read by %s", "由 %s 读取", "由 %s 讀取", "%s에서 읽음", "%s が読み込みます", "Читают: %s", "Lu par %s"},
		xSkillAddSharedOverlap:    {"Codex and Grok also read the shared folder; installing to both gives them the same skill twice", "Codex 与 Grok 也会读取共享目录，同时安装到两处会出现重名 Skill", "Codex 與 Grok 也會讀取共用目錄，同時安裝到兩處會出現重名 Skill", "Codex와 Grok은 공유 폴더도 읽으므로 두 곳에 설치하면 같은 Skill이 중복됩니다", "Codex と Grok は共有フォルダーも読み込むため、両方に入れると同名の Skill が重複します", "Codex и Grok читают и общую папку; установка в оба места даст дубликаты", "Codex et Grok lisent aussi le dossier partagé ; installer aux deux endroits crée des doublons"},
		xSkillAddReloadHint:       {"CLIs load new skills in new sessions; restart a running CLI to see them", "CLI 会在新会话中加载新增的 Skill，已运行的 CLI 需重启后可见", "CLI 會在新工作階段載入新增的 Skill，已執行的 CLI 需重新啟動後可見", "CLI는 새 세션에서 새 Skill을 불러오며, 실행 중인 CLI는 다시 시작해야 보입니다", "CLI は新しいセッションで Skill を読み込みます。実行中の CLI は再起動してください", "Новые навыки загружаются в новых сеансах; перезапустите запущенную CLI", "Les CLI chargent les nouveaux skills dans de nouvelles sessions ; redémarrez une CLI en cours"},
		xSkillAddTargetDone:       {"Installed", "已安装", "已安裝", "설치됨", "インストール済み", "Установлено", "Installé"},
		xSkillAddTargetBlocked:    {"Cannot install", "无法安装", "無法安裝", "설치할 수 없음", "インストールできません", "Нельзя установить", "Installation impossible"},
		xSkillAddCounts:           {"%d to add · %d skipped", "新增 %d · 跳过 %d", "新增 %d · 略過 %d", "추가 %d · 건너뜀 %d", "追加 %d · スキップ %d", "добавить: %d · пропустить: %d", "À ajouter : %d · ignorés : %d"},
		xSkillAddWillAdd:          {"Will add", "将添加", "將新增", "추가 예정", "追加します", "Будет добавлен", "Sera ajouté"},
		xSkillAddExists:           {"Folder exists, not overwritten", "文件夹已存在，不覆盖", "資料夾已存在，不覆寫", "폴더가 있어 덮어쓰지 않음", "フォルダーが既にあるため上書きしません", "Папка есть, не перезаписывается", "Le dossier existe, non écrasé"},
		xSkillAddInstalled:        {"Already added from this source; update it from its page", "已从同一来源添加，可在详情页更新", "已從同一來源新增，可在詳細頁更新", "같은 소스에서 이미 추가됨, 상세 페이지에서 업데이트", "同じ取得元から追加済み、詳細ページで更新できます", "Уже добавлен из этого источника; обновляйте на его странице", "Déjà ajouté depuis cette source ; mettez-le à jour depuis sa page"},
		xSkillAddInvalid:          {"Name not allowed here", "该名称在此不可用", "此名稱在此無法使用", "여기서 사용할 수 없는 이름", "ここでは使用できない名前です", "Имя здесь недопустимо", "Nom non autorisé ici"},
		xSkillAddClash:            {"Same name as a loaded skill in %s: %s", "与 %s 中已加载的同名 Skill 冲突：%s", "與 %s 中已載入的同名 Skill 衝突：%s", "%s에 로드된 같은 이름의 Skill과 충돌: %s", "%s で読み込まれている同名の Skill と競合：%s", "Совпадает с загруженным навыком в %s: %s", "Même nom qu’un skill chargé dans %s : %s"},
		xSkillAddWriting:          {"Installing skills…", "正在安装 Skill…", "正在安裝 Skill…", "Skill 설치 중…", "Skill をインストール中…", "Установка навыков…", "Installation des skills…"},
		xSkillAddResultsDone:      {"Skills installed", "Skill 已安装", "Skill 已安裝", "Skill을 설치했습니다", "Skill をインストールしました", "Навыки установлены", "Skills installés"},
		xSkillAddResultsFailed:    {"Some skills could not be installed", "部分 Skill 未能安装", "部分 Skill 未能安裝", "일부 Skill을 설치하지 못했습니다", "一部の Skill をインストールできませんでした", "Не удалось установить некоторые навыки", "Certains skills n’ont pas pu être installés"},
		xSkillAddNothing:          {"Nothing new to add here", "此处没有需要新增的 Skill", "此處沒有需要新增的 Skill", "여기에 새로 추가할 항목이 없습니다", "ここに追加するものはありません", "Здесь нечего добавлять", "Rien de nouveau à ajouter ici"},
		xSkillAddRecordFailed:     {"Installed, but the source could not be recorded, so these skills cannot be updated", "已安装，但未能记录来源，这些 Skill 将无法更新", "已安裝，但未能記錄來源，這些 Skill 將無法更新", "설치했지만 소스를 기록하지 못해 업데이트할 수 없습니다", "インストールしましたが取得元を記録できず、更新できません", "Установлено, но источник не записан — обновление невозможно", "Installé, mais la source n’a pas pu être enregistrée ; mise à jour impossible"},
		xSkillAddFailed:           {"Could not add the skills", "添加 Skill 失败", "新增 Skill 失敗", "Skill을 추가하지 못했습니다", "Skill を追加できませんでした", "Не удалось добавить навыки", "Impossible d’ajouter les skills"},
		xSkillAddErrSource:        {"Enter a repository, folder or ZIP file", "请填写仓库、文件夹或 ZIP 文件", "請填寫儲存庫、資料夾或 ZIP 檔案", "저장소, 폴더 또는 ZIP 파일을 입력하세요", "リポジトリ、フォルダー、ZIP ファイルを指定してください", "Укажите репозиторий, папку или ZIP-файл", "Indiquez un dépôt, un dossier ou un fichier ZIP"},
		xSkillAddErrCancelled:     {"Cancelled", "已取消", "已取消", "취소됨", "キャンセルしました", "Отменено", "Annulé"},
		xSkillAddErrInvalidSource: {"Not a GitHub repository link, folder or ZIP file", "不是有效的 GitHub 仓库链接、文件夹或 ZIP 文件", "不是有效的 GitHub 儲存庫連結、資料夾或 ZIP 檔案", "올바른 GitHub 저장소 링크, 폴더 또는 ZIP 파일이 아닙니다", "有効な GitHub リポジトリのリンク、フォルダー、ZIP ファイルではありません", "Это не ссылка на репозиторий GitHub, папка или ZIP-файл", "Ce n’est ni un lien de dépôt GitHub, ni un dossier, ni un fichier ZIP"},
		xSkillAddErrNotFound:      {"Not found on GitHub, or the repository is private", "GitHub 上找不到，或仓库为私有", "GitHub 上找不到，或儲存庫為私人", "GitHub에서 찾을 수 없거나 비공개 저장소입니다", "GitHub に見つからないか、非公開リポジトリです", "Не найдено на GitHub или репозиторий закрыт", "Introuvable sur GitHub, ou dépôt privé"},
		xSkillAddErrRateLimited:   {"GitHub’s request limit was reached; try again later", "已达到 GitHub 请求上限，请稍后再试", "已達 GitHub 請求上限，請稍後再試", "GitHub 요청 한도에 도달했습니다. 나중에 다시 시도하세요", "GitHub のリクエスト上限に達しました。後でもう一度お試しください", "Достигнут лимит запросов GitHub; повторите позже", "Limite de requêtes GitHub atteinte ; réessayez plus tard"},
		xSkillAddErrNetwork:       {"Could not reach GitHub", "无法连接 GitHub", "無法連線 GitHub", "GitHub에 연결할 수 없습니다", "GitHub に接続できません", "Не удалось подключиться к GitHub", "Impossible de joindre GitHub"},
		xSkillAddErrHTTP:          {"GitHub returned an error", "GitHub 返回错误", "GitHub 回傳錯誤", "GitHub 오류", "GitHub がエラーを返しました", "GitHub вернул ошибку", "GitHub a renvoyé une erreur"},
		xSkillAddErrTooLarge:      {"The source exceeds the size limit", "来源超过大小限制", "來源超過大小限制", "소스가 크기 제한을 초과합니다", "取得元がサイズ上限を超えています", "Источник превышает лимит размера", "La source dépasse la taille maximale"},
		xSkillAddErrTooManyFiles:  {"The source has too many files", "来源中的文件过多", "來源中的檔案過多", "소스의 파일이 너무 많습니다", "取得元のファイルが多すぎます", "В источнике слишком много файлов", "La source contient trop de fichiers"},
		xSkillAddErrInvalidPath:   {"Unsafe path in the source", "来源中有不安全的路径", "來源中有不安全的路徑", "소스에 안전하지 않은 경로가 있습니다", "取得元に安全でないパスがあります", "Небезопасный путь в источнике", "Chemin dangereux dans la source"},
		xSkillAddErrArchive:       {"The archive cannot be read", "无法读取压缩包", "無法讀取壓縮檔", "압축 파일을 읽을 수 없습니다", "アーカイブを読み込めません", "Не удалось прочитать архив", "Impossible de lire l’archive"},
		xSkillAddErrCase:          {"Two paths differ only in letter case", "存在仅大小写不同的路径", "存在僅大小寫不同的路徑", "대소문자만 다른 경로가 있습니다", "大文字と小文字だけが異なるパスがあります", "Пути различаются только регистром", "Deux chemins ne diffèrent que par la casse"},
		xSkillAddErrNoSkills:      {"No SKILL.md found", "未找到 SKILL.md", "找不到 SKILL.md", "SKILL.md를 찾지 못했습니다", "SKILL.md が見つかりません", "SKILL.md не найден", "Aucun SKILL.md trouvé"},
		xSkillAddErrExists:        {"A folder with that name already exists", "同名文件夹已存在", "同名資料夾已存在", "같은 이름의 폴더가 이미 있습니다", "同名のフォルダーが既にあります", "Папка с таким именем уже есть", "Un dossier de ce nom existe déjà"},
		xSkillAddErrUnsafeRoot:    {"The skills folder is a link or a file", "Skill 目录是链接或文件", "Skill 目錄是連結或檔案", "Skill 폴더가 링크 또는 파일입니다", "Skill フォルダーがリンクまたはファイルです", "Папка навыков — ссылка или файл", "Le dossier des skills est un lien ou un fichier"},
		xSkillAddErrChanged:       {"The skills folder changed; check again", "Skill 目录已变化，请重新检查", "Skill 目錄已變更，請重新檢查", "Skill 폴더가 바뀌었습니다. 다시 확인하세요", "Skill フォルダーが変更されました。再確認してください", "Папка навыков изменилась; проверьте снова", "Le dossier des skills a changé ; vérifiez à nouveau"},
		xSkillAddErrWrite:         {"Could not write the files", "写入文件失败", "寫入檔案失敗", "파일을 쓰지 못했습니다", "ファイルを書き込めませんでした", "Не удалось записать файлы", "Impossible d’écrire les fichiers"},
		xSkillAddErrNotUpdatable:  {"Not a skill added from GitHub by Quosparr", "不是由 Quosparr 从 GitHub 添加的 Skill", "不是由 Quosparr 從 GitHub 新增的 Skill", "Quosparr가 GitHub에서 추가한 Skill이 아닙니다", "Quosparr で GitHub から追加した Skill ではありません", "Навык добавлен не через Quosparr из GitHub", "Ce skill n’a pas été ajouté depuis GitHub par Quosparr"},
		xSkillAddErrModified:      {"Changed locally since it was added", "添加后在本地被修改过", "新增後在本機被修改過", "추가 후 로컬에서 수정됨", "追加後にローカルで変更されています", "Изменён локально после добавления", "Modifié localement depuis son ajout"},
		xSkillAddErrMissing:       {"The skill is no longer in its repository", "仓库中已找不到该 Skill", "儲存庫中已找不到該 Skill", "저장소에 더 이상 이 Skill이 없습니다", "リポジトリにこの Skill がなくなっています", "Навыка больше нет в репозитории", "Le skill n’est plus dans son dépôt"},
		xSkillsInstQuosparr:       {"Quosparr (%s)", "Quosparr 添加（%s）", "Quosparr 新增（%s）", "Quosparr 추가 (%s)", "Quosparr で追加（%s）", "Quosparr (%s)", "Quosparr (%s)"},
		xSkillsCommit:             {"Commit", "提交", "提交", "커밋", "コミット", "Коммит", "Commit"},
		xSkillsImportedFrom:       {"Added from", "添加自", "新增自", "추가한 위치", "追加元", "Добавлен из", "Ajouté depuis"},
		xSkillUpdateCheck:         {"Check for updates", "检查更新", "檢查更新", "업데이트 확인", "更新を確認", "Проверить обновления", "Rechercher des mises à jour"},
		xSkillUpdateChecking:      {"Checking GitHub…", "正在检查 GitHub…", "正在檢查 GitHub…", "GitHub 확인 중…", "GitHub を確認中…", "Проверка GitHub…", "Vérification de GitHub…"},
		xSkillUpdateCurrent:       {"Up to date", "已是最新", "已是最新", "최신 상태", "最新です", "Актуальная версия", "À jour"},
		xSkillUpdateAvailable:     {"New version %s", "有新版本 %s", "有新版本 %s", "새 버전 %s", "新しいバージョン %s", "Новая версия %s", "Nouvelle version %s"},
		xSkillUpdate:              {"Update", "更新", "更新", "업데이트", "更新", "Обновить", "Mettre à jour"},
		xSkillUpdateForce:         {"Update anyway", "仍然更新", "仍然更新", "그래도 업데이트", "それでも更新", "Всё равно обновить", "Mettre à jour quand même"},
		xSkillUpdateTitle:         {"Update this skill?", "更新此 Skill？", "更新此 Skill？", "이 Skill을 업데이트할까요?", "この Skill を更新しますか？", "Обновить навык?", "Mettre à jour ce skill ?"},
		xSkillUpdateMessage:       {"The latest version from GitHub replaces %s; the current one moves to the skills trash, where it can be restored.", "将用 GitHub 上的最新版本替换 %s，当前版本移入 Skill 回收站，可随时恢复。", "將以 GitHub 上的最新版本取代 %s，目前版本移入 Skill 回收站，可隨時復原。", "GitHub의 최신 버전으로 %s을(를) 바꾸며, 현재 버전은 Skill 휴지통으로 옮겨져 복원할 수 있습니다.", "GitHub の最新版で %s を置き換えます。現在の版は Skill のゴミ箱に移動し、復元できます。", "Последняя версия с GitHub заменит %s; текущая переместится в корзину навыков, откуда её можно восстановить.", "La dernière version de GitHub remplace %s ; l’actuelle va dans la corbeille des skills, d’où elle peut être restaurée."},
		xSkillUpdateForceMessage:  {"This copy was changed after it was added; your changes go to the skills trash with it.", "此副本在添加后被修改过，你的修改会随旧版本一起移入 Skill 回收站。", "此副本在新增後被修改過，你的修改會隨舊版本一起移入 Skill 回收站。", "이 사본은 추가 후 수정되었으며, 수정 내용은 이전 버전과 함께 Skill 휴지통으로 옮겨집니다.", "このコピーは追加後に変更されています。変更内容は旧版と一緒に Skill のゴミ箱に移動します。", "Эта копия изменена после добавления; изменения уйдут в корзину навыков вместе с ней.", "Cette copie a été modifiée après son ajout ; vos changements vont avec elle dans la corbeille des skills."},
		xSkillUpdated:             {"Updated; the previous version is in the skills trash", "已更新，旧版本已移入 Skill 回收站", "已更新，舊版本已移入 Skill 回收站", "업데이트했습니다. 이전 버전은 Skill 휴지통에 있습니다", "更新しました。旧版は Skill のゴミ箱にあります", "Обновлено; прежняя версия в корзине навыков", "Mis à jour ; l’ancienne version est dans la corbeille des skills"},
		xSkillUpdatedSystem:       {"Updated; the previous version is in the system Trash", "已更新，旧版本已移到系统回收站", "已更新，舊版本已移到系統資源回收筒", "업데이트했습니다. 이전 버전은 시스템 휴지통에 있습니다", "更新しました。旧版はシステムのゴミ箱にあります", "Обновлено; прежняя версия в системной корзине", "Mis à jour ; l’ancienne version est dans la corbeille du système"},
		xSkillUpdateUnchanged:     {"Already the latest version", "已是最新版本", "已是最新版本", "이미 최신 버전입니다", "既に最新版です", "Уже последняя версия", "Déjà la dernière version"},
		xSkillUpdateFailed:        {"Update failed: %s", "更新失败：%s", "更新失敗：%s", "업데이트 실패: %s", "更新できませんでした：%s", "Не удалось обновить: %s", "Échec de la mise à jour : %s"},
	} {
		extraTexts[key] = texts
	}
}
