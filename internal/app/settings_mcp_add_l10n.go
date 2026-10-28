package app

const (
	xMCPCleanupWarning       textKey = "mcpAddCleanupWarning"
	xMCPNeedsReview          textKey = "mcpAddNeedsReview"
	xMCPJSONPosition         textKey = "mcpAddJSONPosition"
	xMCPErrorStageTools      textKey = "mcpAddErrorStageTools"
	xMCPErrorStageInitialize textKey = "mcpAddErrorStageInitialize"
	xMCPErrorStageDiscover   textKey = "mcpAddErrorStageDiscover"
	xMCPErrorStageConnect    textKey = "mcpAddErrorStageConnect"
	xMCPErrorStorage         textKey = "mcpAddErrorStorage"
	xMCPErrorChanged         textKey = "mcpAddErrorChanged"
	xMCPErrorProcess         textKey = "mcpAddErrorProcess"
	xMCPErrorCancelled       textKey = "mcpAddErrorCancelled"
	xMCPErrorTimeout         textKey = "mcpAddErrorTimeout"
	xMCPErrorRPC             textKey = "mcpAddErrorRPC"
	xMCPErrorProtocol        textKey = "mcpAddErrorProtocol"
	xMCPErrorResponse        textKey = "mcpAddErrorResponse"
	xMCPErrorHTTP            textKey = "mcpAddErrorHTTP"
	xMCPErrorNetwork         textKey = "mcpAddErrorNetwork"
	xMCPErrorEnv             textKey = "mcpAddErrorEnv"
	xMCPErrorInvalid         textKey = "mcpAddErrorInvalid"
	xMCPErrorUnsupported     textKey = "mcpAddErrorUnsupported"
	xMCPErrorConflict        textKey = "mcpAddErrorConflict"
	xMCPErrorDuplicate       textKey = "mcpAddErrorDuplicate"
	xMCPErrorSize            textKey = "mcpAddErrorSize"
	xMCPErrorSyntax          textKey = "mcpAddErrorSyntax"
	xMCPTransportChange      textKey = "mcpAddTransportChange"
	xMCPAdd                  textKey = "mcpAddAdd"
	xMCPFill                 textKey = "mcpAddFill"
	xMCPTools                textKey = "mcpAddTools"
	xMCPTargets              textKey = "mcpAddTargets"
	xMCPReview               textKey = "mcpAddReview"
	xMCPResults              textKey = "mcpAddResults"
	xMCPMemoryOnly           textKey = "mcpAddMemoryOnly"
	xMCPJSON                 textKey = "mcpAddJSON"
	xMCPReplace              textKey = "mcpAddReplace"
	xMCPParse                textKey = "mcpAddParse"
	xMCPSelectDraft          textKey = "mcpAddSelectDraft"
	xMCPUnnamed              textKey = "mcpAddUnnamed"
	xMCPNewDraft             textKey = "mcpAddNewDraft"
	xMCPName                 textKey = "mcpAddName"
	xMCPShowSecrets          textKey = "mcpAddShowSecrets"
	xMCPArgs                 textKey = "mcpAddArgs"
	xMCPAddArgument          textKey = "mcpAddAddArgument"
	xMCPEnv                  textKey = "mcpAddEnv"
	xMCPCwd                  textKey = "mcpAddCwd"
	xMCPHeaders              textKey = "mcpAddHeaders"
	xMCPEnvHeaders           textKey = "mcpAddEnvHeaders"
	xMCPBearerEnv            textKey = "mcpAddBearerEnv"
	xMCPKey                  textKey = "mcpAddKey"
	xMCPValue                textKey = "mcpAddValue"
	xMCPAddPair              textKey = "mcpAddAddPair"
	xMCPNext                 textKey = "mcpAddNext"
	xMCPPreviewInfo          textKey = "mcpAddPreviewInfo"
	xMCPFetch                textKey = "mcpAddFetch"
	xMCPStartFetch           textKey = "mcpAddStartFetch"
	xMCPExecuteWarning       textKey = "mcpAddExecuteWarning"
	xMCPNotFetched           textKey = "mcpAddNotFetched"
	xMCPUnverified           textKey = "mcpAddUnverified"
	xMCPUnverifiedStatus     textKey = "mcpAddUnverifiedStatus"
	xMCPPreviewSuccess       textKey = "mcpAddPreviewSuccess"
	xMCPIncomplete           textKey = "mcpAddIncomplete"
	xMCPZeroTools            textKey = "mcpAddZeroTools"
	xMCPToolSearch           textKey = "mcpAddToolSearch"
	xMCPOAuthHint            textKey = "mcpAddOAuthHint"
	xMCPWillAdd              textKey = "mcpAddWillAdd"
	xMCPAlreadyExists        textKey = "mcpAddAlreadyExists"
	xMCPReloadHint           textKey = "mcpAddReloadHint"
	xMCPSubmit               textKey = "mcpAddSubmit"
	xMCPWriting              textKey = "mcpAddWriting"
	xMCPAdded                textKey = "mcpAddAdded"
	xMCPRetry                textKey = "mcpAddRetry"
	xMCPDraftError           textKey = "mcpAddDraftError"
	xMCPProbeError           textKey = "mcpAddProbeError"
	xMCPAddFailed            textKey = "mcpAddAddFailed"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xMCPCleanupWarning:       {"The temporary server session could not be closed", "未能关闭服务端临时会话", "未能關閉伺服器端暫時工作階段", "임시 서버 세션을 닫지 못했습니다", "一時サーバーセッションを終了できませんでした", "Временный сеанс сервера не закрыт", "La session temporaire du serveur n’a pas pu être fermée"},
		xMCPNeedsReview:          {"Saved but verification failed — review required", "已写入但复核失败，需要人工复核", "已寫入但複核失敗，需要人工複核", "저장했으나 검증 실패 — 확인 필요", "保存済みですが検証に失敗、確認が必要です", "Записано, но проверка не пройдена — требуется проверка", "Enregistré mais vérification échouée — contrôle requis"},
		xMCPJSONPosition:         {"line %d, column %d", "第 %d 行，第 %d 列", "第 %d 行，第 %d 欄", "%d행, %d열", "%d 行、%d 列", "строка %d, столбец %d", "ligne %d, colonne %d"},
		xMCPErrorStageTools:      {"Listing tools", "获取工具阶段", "取得工具階段", "도구 목록 단계", "ツール一覧の取得", "Список инструментов", "Liste des outils"},
		xMCPErrorStageInitialize: {"Initializing", "初始化阶段", "初始化階段", "초기화 단계", "初期化", "Инициализация", "Initialisation"},
		xMCPErrorStageDiscover:   {"Discovering server", "发现服务阶段", "探索服務階段", "서버 검색 단계", "サーバー検出", "Обнаружение сервера", "Découverte du serveur"},
		xMCPErrorStageConnect:    {"Connecting", "连接阶段", "連線階段", "연결 단계", "接続", "Подключение", "Connexion"},
		xMCPErrorStorage:         {"Could not safely read, back up, save, or verify configuration", "无法安全读取、备份、保存或复核配置", "無法安全讀取、備份、儲存或複核設定", "구성을 안전하게 읽기, 백업, 저장 또는 확인할 수 없음", "設定の読み取り、バックアップ、保存または検証に失敗しました", "Не удалось безопасно прочитать, сохранить или проверить настройки", "Impossible de lire, sauvegarder, enregistrer ou vérifier la configuration"},
		xMCPErrorChanged:         {"Configuration changed; preflight again before retrying", "配置已变化，请重新预检后重试", "設定已變更，請重新預檢後重試", "구성이 변경됨; 다시 사전 확인하세요", "設定が変更されました、再確認してください", "Конфигурация изменилась; повторите проверку", "Configuration modifiée ; revérifiez avant de réessayer"},
		xMCPErrorProcess:         {"Program could not start or exited unexpectedly", "程序无法启动或意外退出", "程式無法啟動或意外結束", "프로그램을 시작하지 못했거나 예기치 않게 종료됨", "プログラムを起動できないか予期せず終了しました", "Не удалось запустить программу или она завершилась", "Impossible de démarrer le programme ou arrêt inattendu"},
		xMCPErrorCancelled:       {"Cancelled", "已取消", "已取消", "취소됨", "キャンセル済み", "Отменено", "Annulé"},
		xMCPErrorTimeout:         {"Connection timed out", "连接超时", "連線逾時", "연결 시간 초과", "接続がタイムアウトしました", "Время подключения истекло", "Délai de connexion dépassé"},
		xMCPErrorRPC:             {"Server rejected the MCP request", "服务拒绝 MCP 请求", "服務拒絕 MCP 請求", "서버가 MCP 요청을 거부함", "サーバーが MCP 要求を拒否しました", "Сервер отклонил запрос MCP", "Le serveur a rejeté la requête MCP"},
		xMCPErrorProtocol:        {"Unsupported MCP protocol or capabilities", "MCP 协议或能力不受支持", "MCP 通訊協定或能力不受支援", "지원하지 않는 MCP 프로토콜 또는 기능", "MCP プロトコルまたは機能に未対応です", "Протокол или возможности MCP не поддерживаются", "Protocole ou capacités MCP non pris en charge"},
		xMCPErrorResponse:        {"Invalid server response", "服务响应无效", "服務回應無效", "잘못된 서버 응답", "サーバー応答が無効です", "Некорректный ответ сервера", "Réponse du serveur invalide"},
		xMCPErrorHTTP:            {"Server returned an HTTP error", "服务返回 HTTP 错误", "服務回傳 HTTP 錯誤", "서버 HTTP 오류", "サーバーが HTTP エラーを返しました", "Ошибка HTTP сервера", "Erreur HTTP du serveur"},
		xMCPErrorNetwork:         {"Connection failed", "连接失败", "連線失敗", "연결 실패", "接続に失敗しました", "Ошибка подключения", "Échec de connexion"},
		xMCPErrorEnv:             {"Required environment variable is unavailable", "所需环境变量不可用", "所需環境變數無法使用", "필수 환경 변수를 사용할 수 없음", "必要な環境変数がありません", "Переменная окружения недоступна", "Variable d’environnement indisponible"},
		xMCPErrorInvalid:         {"Missing or invalid field", "字段缺失或无效", "欄位缺少或無效", "필드 누락 또는 잘못된 값", "項目が未入力または無効です", "Поле отсутствует или недопустимо", "Champ absent ou invalide"},
		xMCPErrorUnsupported:     {"Unsupported field, format, or transport", "不支持该字段、格式或传输方式", "不支援此欄位、格式或傳輸方式", "지원하지 않는 필드, 형식 또는 전송 방식", "未対応の項目、形式または接続方式です", "Неподдерживаемое поле, формат или транспорт", "Champ, format ou transport non pris en charge"},
		xMCPErrorConflict:        {"Conflicting values or an existing server with different settings", "字段冲突，或同名服务配置不同", "欄位衝突，或同名服務設定不同", "값 충돌 또는 기존 서버 설정 불일치", "設定が競合するか同名サーバーの設定が異なります", "Конфликт значений или настроек одноимённого сервера", "Valeurs en conflit ou serveur existant différent"},
		xMCPErrorDuplicate:       {"Duplicate names or keys", "存在重复名称或键", "存在重複名稱或鍵", "이름 또는 키 중복", "名前またはキーが重複しています", "Повтор имени или ключа", "Noms ou clés en double"},
		xMCPErrorSize:            {"Input or response exceeds the size limit", "输入或响应超过大小限制", "輸入或回應超過大小限制", "입력 또는 응답 크기 제한 초과", "入力または応答の上限を超えています", "Превышен лимит размера", "Taille maximale dépassée"},
		xMCPErrorSyntax:          {"Malformed JSON", "JSON 语法无效", "JSON 語法無效", "잘못된 JSON 문법", "JSON 構文が無効です", "Некорректный JSON", "Syntaxe JSON invalide"},
		xMCPTransportChange:      {"Switch transport and clear incompatible connection fields? This removes their values from this draft", "切换传输方式并清除不兼容的连接字段？这会删除草稿中的对应值", "切換傳輸方式並清除不相容的連線欄位？這會刪除草稿中的對應值", "전송 방식을 바꾸고 호환되지 않는 연결 값을 삭제할까요?", "接続方式を切り替え、互換性のない接続項目を削除しますか？", "Сменить транспорт и удалить несовместимые параметры подключения?", "Changer le transport et effacer les paramètres incompatibles ?"},
		xMCPAdd:                  {"Add MCP servers", "添加 MCP 服务", "新增 MCP 服務", "MCP 서버 추가", "MCP サーバーを追加", "Добавить серверы MCP", "Ajouter des serveurs MCP"},
		xMCPFill:                 {"Connection settings", "填写连接配置", "填寫連線設定", "연결 설정", "接続設定", "Параметры подключения", "Paramètres de connexion"},
		xMCPTools:                {"Preview tools", "预览工具", "預覽工具", "도구 미리 보기", "ツールのプレビュー", "Просмотр инструментов", "Aperçu des outils"},
		xMCPTargets:              {"Select providers", "选择服务商", "選擇服務商", "공급자 선택", "プロバイダーを選択", "Выбрать провайдеров", "Choisir les fournisseurs"},
		xMCPReview:               {"Review changes", "确认改动", "確認變更", "변경 검토", "変更を確認", "Проверить изменения", "Vérifier les modifications"},
		xMCPResults:              {"Results", "添加结果", "新增結果", "결과", "結果", "Результаты", "Résultats"},
		xMCPMemoryOnly:           {"Drafts stay in memory and are cleared when you leave", "草稿仅保存在内存，离开页面会清除", "草稿僅保存在記憶體，離開頁面會清除", "초안은 메모리에만 저장되며 페이지를 나가면 삭제됩니다", "下書きはメモリに保持され、ページを離れると消去されます", "Черновики хранятся в памяти и удаляются при выходе", "Les brouillons restent en mémoire et sont effacés en quittant cette page"},
		xMCPJSON:                 {"Paste JSON", "粘贴 JSON", "貼上 JSON", "JSON 붙여넣기", "JSON を貼り付け", "Вставить JSON", "Coller du JSON"},
		xMCPReplace:              {"Replace drafts after parsing (otherwise append)", "解析后替换草稿，否则追加", "解析後取代草稿，否則附加", "분석 후 초안 교체 (해제 시 추가)", "解析後に下書きを置換（未選択なら追加）", "Заменить черновики после разбора (иначе добавить)", "Remplacer les brouillons après analyse (sinon ajouter)"},
		xMCPParse:                {"Parse JSON", "解析 JSON", "解析 JSON", "JSON 분석", "JSON を解析", "Разобрать JSON", "Analyser le JSON"},
		xMCPSelectDraft:          {"Include this server", "添加此服务", "新增此服務", "이 서버 포함", "このサーバーを含める", "Включить сервер", "Inclure ce serveur"},
		xMCPUnnamed:              {"Unnamed server — enter a name", "未命名服务，请补填名称", "未命名服務，請補填名稱", "이름 없는 서버 — 이름 입력", "名前未設定 — 名前を入力", "Сервер без имени — укажите имя", "Serveur sans nom — saisir un nom"},
		xMCPNewDraft:             {"Add another server manually", "手动添加另一个服务", "手動新增另一個服務", "다른 서버 직접 추가", "別のサーバーを手動追加", "Добавить сервер вручную", "Ajouter un autre serveur manuellement"},
		xMCPName:                 {"Server name", "服务名称", "服務名稱", "서버 이름", "サーバー名", "Имя сервера", "Nom du serveur"},
		xMCPShowSecrets:          {"Show secret values", "显示秘密值", "顯示秘密值", "비밀 값 표시", "秘密の値を表示", "Показать секретные значения", "Afficher les valeurs secrètes"},
		xMCPArgs:                 {"Arguments (one item per argument)", "命令参数，每项一个参数", "命令參數，每項一個參數", "인수 (항목당 하나)", "引数（項目ごとに1つ）", "Аргументы (по одному)", "Arguments (un par élément)"},
		xMCPAddArgument:          {"Add argument", "添加参数", "新增參數", "인수 추가", "引数を追加", "Добавить аргумент", "Ajouter un argument"},
		xMCPEnv:                  {"Environment variables", "环境变量", "環境變數", "환경 변수", "環境変数", "Переменные окружения", "Variables d’environnement"},
		xMCPCwd:                  {"Advanced: working directory (optional)", "高级：工作目录，可选", "進階：工作目錄，選填", "고급: 작업 디렉터리 (선택)", "詳細：作業ディレクトリ（任意）", "Дополнительно: рабочий каталог", "Avancé : répertoire de travail (facultatif)"},
		xMCPHeaders:              {"HTTP headers", "HTTP 请求头", "HTTP 請求標頭", "HTTP 헤더", "HTTP ヘッダー", "Заголовки HTTP", "En-têtes HTTP"},
		xMCPEnvHeaders:           {"Header values from environment variable names", "从指定环境变量读取请求头", "從指定環境變數讀取請求標頭", "환경 변수 이름에서 헤더 값 읽기", "環境変数名からヘッダー値を読み取る", "Заголовки из переменных окружения", "Valeurs des en-têtes depuis les variables d’environnement"},
		xMCPBearerEnv:            {"Bearer token environment variable name (optional)", "Bearer token 环境变量名，可选", "Bearer token 環境變數名稱，選填", "Bearer 토큰 환경 변수 이름 (선택)", "Bearer トークンの環境変数名（任意）", "Имя переменной токена Bearer", "Nom de variable du jeton Bearer (facultatif)"},
		xMCPKey:                  {"Name", "键名", "鍵名", "이름", "名前", "Имя", "Nom"},
		xMCPValue:                {"Value", "值", "值", "값", "値", "Значение", "Valeur"},
		xMCPAddPair:              {"Add entry", "添加一项", "新增一項", "항목 추가", "項目を追加", "Добавить запись", "Ajouter une entrée"},
		xMCPNext:                 {"Continue", "下一步", "下一步", "계속", "次へ", "Продолжить", "Continuer"},
		xMCPPreviewInfo:          {"Only fetches tool descriptions; never invokes tools, and only connects after your click", "仅获取工具说明，不调用工具，仅在点击后连接", "僅取得工具說明，不呼叫工具，僅在點擊後連線", "클릭한 뒤에만 연결하며 도구 설명만 가져오고 실행하지 않습니다", "クリック後に接続してツール説明のみを取得し、ツールは実行しません", "Подключение только по нажатию; получение описаний без вызова инструментов", "Connexion après votre clic uniquement ; récupère les descriptions sans appeler les outils"},
		xMCPFetch:                {"Connect and fetch tools", "连接并获取工具", "連線並取得工具", "연결 및 도구 가져오기", "接続してツールを取得", "Подключить и получить инструменты", "Connecter et récupérer les outils"},
		xMCPStartFetch:           {"Start program and fetch tools", "启动并获取工具", "啟動並取得工具", "프로그램 실행 및 도구 가져오기", "プログラムを起動してツールを取得", "Запустить и получить инструменты", "Démarrer le programme et récupérer les outils"},
		xMCPExecuteWarning:       {"This starts the configured program with its arguments and environment; npx/uvx may download software", "点击将使用所填参数与环境启动该程序，npx / uvx 可能下载软件", "點擊將使用所填參數與環境啟動該程式，npx / uvx 可能下載軟體", "입력한 인수와 환경으로 프로그램을 실행하며 npx/uvx는 소프트웨어를 다운로드할 수 있습니다", "指定した引数と環境でプログラムを起動します、npx/uvx はソフトウェアをダウンロードする場合があります", "Запустит программу с указанными аргументами и окружением; npx/uvx могут скачать ПО", "Démarre le programme avec ses arguments et son environnement ; npx/uvx peuvent télécharger des logiciels"},
		xMCPNotFetched:           {"Tools not fetched", "尚未获取工具", "尚未取得工具", "도구를 가져오지 않음", "ツール未取得", "Инструменты не получены", "Outils non récupérés"},
		xMCPUnverified:           {"Add unverified configurations", "添加未经验证的配置", "新增未經驗證的設定", "검증되지 않은 구성 추가", "未検証の設定を追加", "Добавить непроверенные настройки", "Ajouter les configurations non vérifiées"},
		xMCPUnverifiedStatus:     {"Unverified — tool compatibility has not been confirmed", "未经验证，尚未确认工具兼容性", "未經驗證，尚未確認工具相容性", "미검증 — 도구 호환성이 확인되지 않음", "未検証 — ツールの互換性は未確認", "Не проверено — совместимость не подтверждена", "Non vérifié — compatibilité des outils non confirmée"},
		xMCPPreviewSuccess:       {"Tools fetched", "已获取工具", "已取得工具", "도구를 가져옴", "ツール取得済み", "Инструменты получены", "Outils récupérés"},
		xMCPIncomplete:           {"Incomplete tool list; adding requires unverified confirmation", "工具列表不完整，添加前须确认未经验证", "工具清單不完整，新增前須確認未經驗證", "도구 목록이 불완전합니다; 미검증 확인이 필요합니다", "ツール一覧が不完全です、未検証の確認が必要です", "Список неполный; требуется согласие на непроверенные настройки", "Liste incomplète ; confirmation non vérifiée requise"},
		xMCPZeroTools:            {"Connected successfully; server reports no tools", "连接成功，服务未提供工具", "連線成功，服務未提供工具", "연결 성공; 서버가 도구를 제공하지 않음", "接続成功、サーバーにツールはありません", "Подключено; сервер не предоставляет инструменты", "Connexion réussie ; aucun outil fourni"},
		xMCPToolSearch:           {"Search tools", "搜索工具", "搜尋工具", "도구 검색", "ツールを検索", "Поиск инструментов", "Rechercher des outils"},
		xMCPOAuthHint:            {"OAuth preview is not supported; save a credential-free configuration and authenticate in the target CLI", "暂不支持 OAuth 预览，可保存不含凭据的配置后在目标 CLI 认证", "暫不支援 OAuth 預覽，可儲存不含憑據的設定後在目標 CLI 認證", "OAuth 미리 보기는 지원되지 않습니다; 자격 증명 없이 저장 후 CLI에서 인증하세요", "OAuth プレビューは未対応です、認証情報なしで保存後に CLI で認証してください", "Просмотр OAuth не поддерживается; сохраните без учётных данных и войдите в CLI", "Aperçu OAuth non pris en charge ; enregistrez sans identifiants et authentifiez-vous dans le CLI"},
		xMCPWillAdd:              {"Will add", "将添加", "將新增", "추가 예정", "追加予定", "Будет добавлено", "Sera ajouté"},
		xMCPAlreadyExists:        {"Already exists — skip", "已存在，跳过", "已存在，略過", "이미 존재 — 건너뛰기", "既存 — スキップ", "Уже существует — пропуск", "Existe déjà — ignorer"},
		xMCPReloadHint:           {"Reload or restart the target CLI to use the saved configuration", "保存后请重新加载或重启目标 CLI", "儲存後請重新載入或重新啟動目標 CLI", "저장한 구성을 사용하려면 CLI를 다시 로드하거나 재시작하세요", "保存した設定を使うには CLI を再読み込みまたは再起動してください", "Перезагрузите или перезапустите CLI после сохранения", "Rechargez ou redémarrez le CLI après enregistrement"},
		xMCPSubmit:               {"Add to %d providers", "添加到 %d 个服务商", "新增至 %d 個服務商", "공급자 %d개에 추가", "%d プロバイダーに追加", "Добавить провайдерам: %d", "Ajouter à %d fournisseurs"},
		xMCPWriting:              {"Saving; leaving this page will not cancel confirmed writes", "正在保存，离开页面不会取消已确认的写入", "正在儲存，離開頁面不會取消已確認的寫入", "저장 중; 페이지를 나가도 확인된 쓰기는 취소되지 않습니다", "保存中、ページを離れても確定した書き込みは続行されます", "Сохранение; выход не отменит подтверждённую запись", "Enregistrement ; quitter ne l’annulera pas"},
		xMCPAdded:                {"%d added · %d already existed", "已添加 %d 个，已存在 %d 个", "已新增 %d 個，已存在 %d 個", "%d개 추가 · %d개 기존", "%d 件追加 · %d 件既存", "Добавлено: %d · уже было: %d", "%d ajoutés · %d existants"},
		xMCPRetry:                {"Preflight failed providers again", "重新预检失败的服务商", "重新預檢失敗的服務商", "실패한 공급자 다시 사전 확인", "失敗したプロバイダーを再確認", "Повторить проверку неудачных провайдеров", "Revérifier les fournisseurs en échec"},
		xMCPDraftError:           {"Invalid configuration", "配置无效", "設定無效", "잘못된 구성", "設定が無効です", "Недопустимая конфигурация", "Configuration invalide"},
		xMCPProbeError:           {"Could not fetch tools", "获取工具失败", "取得工具失敗", "도구 가져오기 실패", "ツール取得に失敗", "Не удалось получить инструменты", "Impossible de récupérer les outils"},
		xMCPAddFailed:            {"Operation failed; inspect the target configuration before retrying", "操作失败，请复核目标配置后重试", "操作失敗，請複核目標設定後重試", "작업 실패; 재시도 전에 대상 구성을 확인하세요", "操作に失敗、対象設定を確認して再試行してください", "Ошибка операции; проверьте настройки перед повтором", "Échec ; vérifiez la configuration cible avant de réessayer"},
	} {
		extraTexts[key] = texts
	}
}
