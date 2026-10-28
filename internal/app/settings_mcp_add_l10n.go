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
	xMCPAdd                  textKey = "mcpAddAdd"
	xMCPTargets              textKey = "mcpAddTargets"
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
	xMCPValue                textKey = "mcpAddValue"
	xMCPNext                 textKey = "mcpAddNext"
	xMCPPreviewInfo          textKey = "mcpAddPreviewInfo"
	xMCPFetch                textKey = "mcpAddFetch"
	xMCPStartFetch           textKey = "mcpAddStartFetch"
	xMCPExecuteWarning       textKey = "mcpAddExecuteWarning"
	xMCPNotFetched           textKey = "mcpAddNotFetched"
	xMCPUnverified           textKey = "mcpAddUnverified"
	xMCPUnverifiedStatus     textKey = "mcpAddUnverifiedStatus"
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
	xMCPProbeError           textKey = "mcpAddProbeError"
	xMCPAddFailed            textKey = "mcpAddAddFailed"
	xMCPStepConfigure        textKey = "mcpAddStepConfigure"
	xMCPStepTools            textKey = "mcpAddStepTools"
	xMCPStepReview           textKey = "mcpAddStepReview"
	xMCPPrevious             textKey = "mcpAddPrevious"
	xMCPChecking             textKey = "mcpAddChecking"
	xMCPDone                 textKey = "mcpAddDone"
	xMCPManual               textKey = "mcpAddManual"
	xMCPImported             textKey = "mcpAddImported"
	xMCPJSONHint             textKey = "mcpAddJSONHint"
	xMCPServers              textKey = "mcpAddServers"
	xMCPNeedsFix             textKey = "mcpAddNeedsFix"
	xMCPRemoveDraft          textKey = "mcpAddRemoveDraft"
	xMCPNameHint             textKey = "mcpAddNameHint"
	xMCPArgsHint             textKey = "mcpAddArgsHint"
	xMCPArgument             textKey = "mcpAddArgument"
	xMCPRemove               textKey = "mcpAddRemove"
	xMCPEnvHint              textKey = "mcpAddEnvHint"
	xMCPVariable             textKey = "mcpAddVariable"
	xMCPAddVariable          textKey = "mcpAddAddVariable"
	xMCPHeadersHint          textKey = "mcpAddHeadersHint"
	xMCPHeaderName           textKey = "mcpAddHeaderName"
	xMCPAddHeader            textKey = "mcpAddAddHeader"
	xMCPAdvanced             textKey = "mcpAddAdvanced"
	xMCPOptional             textKey = "mcpAddOptional"
	xMCPCwdHint              textKey = "mcpAddCwdHint"
	xMCPBearerHint           textKey = "mcpAddBearerHint"
	xMCPEnvHeadersHint       textKey = "mcpAddEnvHeadersHint"
	xMCPHideSecrets          textKey = "mcpAddHideSecrets"
	xMCPTransportStdio       textKey = "mcpAddTransportStdio"
	xMCPStdioHint            textKey = "mcpAddStdioHint"
	xMCPHTTPHint             textKey = "mcpAddHTTPHint"
	xMCPSSEHint              textKey = "mcpAddSSEHint"
	xMCPRefetch              textKey = "mcpAddRefetch"
	xMCPFetching             textKey = "mcpAddFetching"
	xMCPToolCount            textKey = "mcpAddToolCount"
	xMCPProtocol             textKey = "mcpAddProtocol"
	xMCPToolParams           textKey = "mcpAddToolParams"
	xMCPInputSchema          textKey = "mcpAddInputSchema"
	xMCPTargetsHint          textKey = "mcpAddTargetsHint"
	xMCPTargetDone           textKey = "mcpAddTargetDone"
	xMCPTargetBlocked        textKey = "mcpAddTargetBlocked"
	xMCPPlanCounts           textKey = "mcpAddPlanCounts"
	xMCPResultsDone          textKey = "mcpAddResultsDone"
	xMCPResultsFailed        textKey = "mcpAddResultsFailed"
	xMCPDisabledList         textKey = "mcpAddDisabledList"
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
		xMCPAdd:                  {"Add MCP servers", "添加 MCP 服务", "新增 MCP 服務", "MCP 서버 추가", "MCP サーバーを追加", "Добавить серверы MCP", "Ajouter des serveurs MCP"},
		xMCPTargets:              {"Add to", "添加到", "新增至", "추가 대상", "追加先", "Добавить в", "Ajouter à"},
		xMCPMemoryOnly:           {"Drafts stay in memory and are cleared when you leave", "草稿仅保存在内存，离开页面会清除", "草稿僅保存在記憶體，離開頁面會清除", "초안은 메모리에만 저장되며 페이지를 나가면 삭제됩니다", "下書きはメモリに保持され、ページを離れると消去されます", "Черновики хранятся в памяти и удаляются при выходе", "Les brouillons restent en mémoire et sont effacés en quittant cette page"},
		xMCPJSON:                 {"Paste JSON", "粘贴 JSON", "貼上 JSON", "JSON 붙여넣기", "JSON を貼り付け", "Вставить JSON", "Coller du JSON"},
		xMCPReplace:              {"Replace the servers in the form (%d); otherwise append", "替换表单中现有的 %d 个服务（不勾选则追加）", "取代表單中現有的 %d 個服務（未勾選則附加）", "양식의 서버 %d개 교체 (해제 시 추가)", "フォームの %d 件のサーバーを置換（未選択なら追加）", "Заменить серверы в форме (%d), иначе добавить", "Remplacer les serveurs du formulaire (%d), sinon ajouter"},
		xMCPParse:                {"Parse into form", "解析并填入", "解析並填入", "분석하여 채우기", "解析して入力", "Разобрать в форму", "Analyser et remplir"},
		xMCPSelectDraft:          {"Include %s", "添加 %s", "新增 %s", "%s 포함", "%s を含める", "Включить %s", "Inclure %s"},
		xMCPUnnamed:              {"Unnamed", "未命名", "未命名", "이름 없음", "名前なし", "Без имени", "Sans nom"},
		xMCPNewDraft:             {"Add server", "添加服务", "新增服務", "서버 추가", "サーバーを追加", "Добавить сервер", "Ajouter un serveur"},
		xMCPName:                 {"Server name", "服务名称", "服務名稱", "서버 이름", "サーバー名", "Имя сервера", "Nom du serveur"},
		xMCPShowSecrets:          {"Show values", "显示值", "顯示值", "값 표시", "値を表示", "Показать значения", "Afficher les valeurs"},
		xMCPArgs:                 {"Arguments", "参数", "參數", "인수", "引数", "Аргументы", "Arguments"},
		xMCPAddArgument:          {"Add argument", "添加参数", "新增參數", "인수 추가", "引数を追加", "Добавить аргумент", "Ajouter un argument"},
		xMCPEnv:                  {"Environment variables", "环境变量", "環境變數", "환경 변수", "環境変数", "Переменные окружения", "Variables d’environnement"},
		xMCPCwd:                  {"Working directory", "工作目录", "工作目錄", "작업 디렉터리", "作業ディレクトリ", "Рабочий каталог", "Répertoire de travail"},
		xMCPHeaders:              {"HTTP headers", "HTTP 请求头", "HTTP 請求標頭", "HTTP 헤더", "HTTP ヘッダー", "Заголовки HTTP", "En-têtes HTTP"},
		xMCPEnvHeaders:           {"Headers from environment variables", "从环境变量读取的请求头", "從環境變數讀取的標頭", "환경 변수에서 읽는 헤더", "環境変数から読むヘッダー", "Заголовки из переменных окружения", "En-têtes depuis des variables d’environnement"},
		xMCPBearerEnv:            {"Bearer token variable", "Bearer token 环境变量", "Bearer token 環境變數", "Bearer 토큰 변수", "Bearer トークン変数", "Переменная токена Bearer", "Variable du jeton Bearer"},
		xMCPValue:                {"Value", "值", "值", "값", "値", "Значение", "Valeur"},
		xMCPNext:                 {"Continue", "下一步", "下一步", "계속", "次へ", "Продолжить", "Continuer"},
		xMCPPreviewInfo:          {"Only fetches tool descriptions; never invokes tools, and only connects after your click", "仅获取工具说明，不调用工具，仅在点击后连接", "僅取得工具說明，不呼叫工具，僅在點擊後連線", "클릭한 뒤에만 연결하며 도구 설명만 가져오고 실행하지 않습니다", "クリック後に接続してツール説明のみを取得し、ツールは実行しません", "Подключение только по нажатию; получение описаний без вызова инструментов", "Connexion après votre clic uniquement ; récupère les descriptions sans appeler les outils"},
		xMCPFetch:                {"Fetch tools", "获取工具", "取得工具", "도구 가져오기", "ツールを取得", "Получить инструменты", "Récupérer les outils"},
		xMCPStartFetch:           {"Start and fetch tools", "启动并获取工具", "啟動並取得工具", "실행 후 도구 가져오기", "起動してツールを取得", "Запустить и получить", "Démarrer et récupérer"},
		xMCPExecuteWarning:       {"Fetching starts %s with the configured arguments and environment; npx / uvx may download software", "获取时会以所填参数和环境变量启动 %s，npx / uvx 可能下载软件", "取得時會以所填參數與環境變數啟動 %s，npx / uvx 可能下載軟體", "가져올 때 입력한 인수와 환경으로 %s을(를) 실행하며 npx/uvx는 소프트웨어를 다운로드할 수 있습니다", "取得時に設定した引数と環境で %s を起動します。npx / uvx はソフトウェアをダウンロードする場合があります", "При получении запустится %s с указанными аргументами и окружением; npx/uvx могут скачать ПО", "La récupération démarre %s avec les arguments et l’environnement saisis ; npx/uvx peuvent télécharger des logiciels"},
		xMCPNotFetched:           {"Tools not fetched", "尚未获取工具", "尚未取得工具", "도구를 가져오지 않음", "ツール未取得", "Инструменты не получены", "Outils non récupérés"},
		xMCPUnverified:           {"Add unverified configurations", "添加未经验证的配置", "新增未經驗證的設定", "검증되지 않은 구성 추가", "未検証の設定を追加", "Добавить непроверенные настройки", "Ajouter les configurations non vérifiées"},
		xMCPUnverifiedStatus:     {"Unverified — tool compatibility has not been confirmed", "未经验证，尚未确认工具兼容性", "未經驗證，尚未確認工具相容性", "미검증 — 도구 호환성이 확인되지 않음", "未検証 — ツールの互換性は未確認", "Не проверено — совместимость не подтверждена", "Non vérifié — compatibilité des outils non confirmée"},
		xMCPIncomplete:           {"Incomplete tool list; adding requires unverified confirmation", "工具列表不完整，添加前须确认未经验证", "工具清單不完整，新增前須確認未經驗證", "도구 목록이 불완전합니다; 미검증 확인이 필요합니다", "ツール一覧が不完全です、未検証の確認が必要です", "Список неполный; требуется согласие на непроверенные настройки", "Liste incomplète ; confirmation non vérifiée requise"},
		xMCPZeroTools:            {"Connected successfully; server reports no tools", "连接成功，服务未提供工具", "連線成功，服務未提供工具", "연결 성공; 서버가 도구를 제공하지 않음", "接続成功、サーバーにツールはありません", "Подключено; сервер не предоставляет инструменты", "Connexion réussie ; aucun outil fourni"},
		xMCPToolSearch:           {"Search tools", "搜索工具", "搜尋工具", "도구 검색", "ツールを検索", "Поиск инструментов", "Rechercher des outils"},
		xMCPOAuthHint:            {"OAuth preview is not supported; save a credential-free configuration and authenticate in the target CLI", "暂不支持 OAuth 预览，可保存不含凭据的配置后在目标 CLI 认证", "暫不支援 OAuth 預覽，可儲存不含憑據的設定後在目標 CLI 認證", "OAuth 미리 보기는 지원되지 않습니다; 자격 증명 없이 저장 후 CLI에서 인증하세요", "OAuth プレビューは未対応です、認証情報なしで保存後に CLI で認証してください", "Просмотр OAuth не поддерживается; сохраните без учётных данных и войдите в CLI", "Aperçu OAuth non pris en charge ; enregistrez sans identifiants et authentifiez-vous dans le CLI"},
		xMCPWillAdd:              {"Will add", "将添加", "將新增", "추가 예정", "追加予定", "Будет добавлено", "Sera ajouté"},
		xMCPAlreadyExists:        {"Already exists — skip", "已存在，跳过", "已存在，略過", "이미 존재 — 건너뛰기", "既存 — スキップ", "Уже существует — пропуск", "Existe déjà — ignorer"},
		xMCPReloadHint:           {"Reload or restart the target CLI to use the saved configuration", "保存后请重新加载或重启目标 CLI", "儲存後請重新載入或重新啟動目標 CLI", "저장한 구성을 사용하려면 CLI를 다시 로드하거나 재시작하세요", "保存した設定を使うには CLI を再読み込みまたは再起動してください", "Перезагрузите или перезапустите CLI после сохранения", "Rechargez ou redémarrez le CLI après enregistrement"},
		xMCPSubmit:               {"Add to selected (%d)", "添加到 %d 个服务商", "新增至 %d 個服務商", "공급자 %d개에 추가", "%d プロバイダーに追加", "Добавить провайдерам: %d", "Ajouter à la sélection (%d)"},
		xMCPWriting:              {"Saving; leaving this page will not cancel confirmed writes", "正在保存，离开页面不会取消已确认的写入", "正在儲存，離開頁面不會取消已確認的寫入", "저장 중; 페이지를 나가도 확인된 쓰기는 취소되지 않습니다", "保存中、ページを離れても確定した書き込みは続行されます", "Сохранение; выход не отменит подтверждённую запись", "Enregistrement ; quitter ne l’annulera pas"},
		xMCPAdded:                {"%d added · %d already existed", "已添加 %d 个，已存在 %d 个", "已新增 %d 個，已存在 %d 個", "%d개 추가 · %d개 기존", "%d 件追加 · %d 件既存", "Добавлено: %d · уже было: %d", "Ajoutés : %d · existants : %d"},
		xMCPRetry:                {"Retry failed providers", "重试失败的服务商", "重試失敗的服務商", "실패한 공급자 재시도", "失敗したプロバイダーを再試行", "Повторить для неудачных", "Réessayer les fournisseurs en échec"},
		xMCPProbeError:           {"Could not fetch tools", "获取工具失败", "取得工具失敗", "도구 가져오기 실패", "ツール取得に失敗", "Не удалось получить инструменты", "Impossible de récupérer les outils"},
		xMCPAddFailed:            {"Operation failed; inspect the target configuration before retrying", "操作失败，请复核目标配置后重试", "操作失敗，請複核目標設定後重試", "작업 실패; 재시도 전에 대상 구성을 확인하세요", "操作に失敗、対象設定を確認して再試行してください", "Ошибка операции; проверьте настройки перед повтором", "Échec ; vérifiez la configuration cible avant de réessayer"},
		xMCPStepConfigure:        {"Configure", "配置服务", "設定服務", "서버 구성", "サーバー設定", "Настройка", "Configuration"},
		xMCPStepTools:            {"Preview tools", "预览工具", "預覽工具", "도구 미리 보기", "ツールのプレビュー", "Просмотр инструментов", "Aperçu des outils"},
		xMCPStepReview:           {"Confirm", "确认添加", "確認新增", "추가 확인", "追加の確認", "Подтверждение", "Confirmation"},
		xMCPPrevious:             {"Previous", "上一步", "上一步", "이전", "前へ", "Предыдущий шаг", "Précédent"},
		xMCPChecking:             {"Checking target configurations…", "正在检查目标配置…", "正在檢查目標設定…", "대상 구성 확인 중…", "対象の設定を確認中…", "Проверка конфигураций…", "Vérification des configurations…"},
		xMCPDone:                 {"Done", "完成", "完成", "완료", "完了", "Готово", "Terminé"},
		xMCPManual:               {"Form", "手动填写", "手動填寫", "직접 입력", "手動入力", "Вручную", "Formulaire"},
		xMCPImported:             {"Servers imported: %d — review them below before continuing", "已导入 %d 个服务，可在下方检查和修改", "已匯入 %d 個服務，可在下方檢查與修改", "서버 %d개를 가져왔습니다 — 아래에서 확인하고 수정하세요", "%d 件のサーバーを取り込みました。下で確認・修正できます", "Импортировано серверов: %d — проверьте их ниже", "Serveurs importés : %d — vérifiez-les ci-dessous"},
		xMCPJSONHint:             {"Paste one server object, or a configuration with mcpServers / mcp_servers; parsed servers fill the form, where you can keep editing", "可粘贴单个服务对象，或包含 mcpServers / mcp_servers 的配置；解析后会填入表单，可继续修改", "可貼上單一服務物件，或包含 mcpServers / mcp_servers 的設定；解析後會填入表單，可繼續修改", "서버 객체 하나 또는 mcpServers / mcp_servers 구성을 붙여넣으세요. 분석된 서버는 양식에 채워지며 계속 수정할 수 있습니다", "サーバーオブジェクト1つ、または mcpServers / mcp_servers を含む設定を貼り付けます。解析結果はフォームに入り、引き続き編集できます", "Вставьте объект сервера или конфигурацию с mcpServers / mcp_servers; разобранные серверы попадут в форму для правки", "Collez un objet serveur, ou une configuration avec mcpServers / mcp_servers ; les serveurs analysés remplissent le formulaire, modifiable ensuite"},
		xMCPServers:              {"Servers to add · %d of %d selected", "待添加的服务 · 已选 %d / %d", "待新增的服務 · 已選 %d / %d", "추가할 서버 · %d/%d개 선택", "追加するサーバー · %d / %d 件選択", "Серверы для добавления · выбрано %d из %d", "Serveurs à ajouter · sélection %d / %d"},
		xMCPNeedsFix:             {"Needs fixing", "需修正", "需修正", "수정 필요", "要修正", "Нужно исправить", "À corriger"},
		xMCPRemoveDraft:          {"Remove %s", "移除 %s", "移除 %s", "%s 제거", "%s を削除", "Удалить %s", "Retirer %s"},
		xMCPNameHint:             {"Letters, digits, - and _ only; the CLI groups the tools under this name", "仅限字母、数字、- 和 _，CLI 会以此名称区分工具", "僅限字母、數字、- 與 _，CLI 會以此名稱區分工具", "영문자, 숫자, -, _만 사용; CLI가 이 이름으로 도구를 구분합니다", "英数字、- と _ のみ。CLI はこの名前でツールを区別します", "Только буквы, цифры, - и _; CLI различает инструменты по этому имени", "Lettres, chiffres, - et _ uniquement ; le CLI regroupe les outils sous ce nom"},
		xMCPArgsHint:             {"One argument per field, passed to the command in order", "每项一个参数，按顺序传给命令", "每項一個參數，依序傳給命令", "항목마다 인수 하나, 순서대로 명령에 전달", "1項目に1つの引数、順にコマンドへ渡します", "По одному аргументу в поле, передаются по порядку", "Un argument par champ, transmis dans l’ordre"},
		xMCPArgument:             {"Argument %d", "参数 %d", "參數 %d", "인수 %d", "引数 %d", "Аргумент %d", "Argument %d"},
		xMCPRemove:               {"Remove", "移除", "移除", "제거", "削除", "Удалить", "Retirer"},
		xMCPEnvHint:              {"Passed to the program when it starts; values are hidden by default", "启动程序时传入；值默认隐藏", "啟動程式時傳入；值預設隱藏", "프로그램 시작 시 전달되며 값은 기본적으로 숨겨집니다", "起動時にプログラムへ渡します。値は既定で非表示です", "Передаются программе при запуске; значения скрыты", "Transmises au programme au démarrage ; valeurs masquées par défaut"},
		xMCPVariable:             {"Variable", "变量名", "變數名稱", "변수 이름", "変数名", "Переменная", "Variable"},
		xMCPAddVariable:          {"Add variable", "添加变量", "新增變數", "변수 추가", "変数を追加", "Добавить переменную", "Ajouter une variable"},
		xMCPHeadersHint:          {"Sent with every request, such as Authorization; values are hidden by default", "每次请求都会附带，例如 Authorization；值默认隐藏", "每次請求都會附帶，例如 Authorization；值預設隱藏", "Authorization 등 모든 요청에 포함되며 값은 기본적으로 숨겨집니다", "Authorization など毎回の要求に付けます。値は既定で非表示です", "Отправляются с каждым запросом, например Authorization; значения скрыты", "Envoyés avec chaque requête, comme Authorization ; valeurs masquées par défaut"},
		xMCPHeaderName:           {"Header", "请求头名称", "標頭名稱", "헤더 이름", "ヘッダー名", "Заголовок", "En-tête"},
		xMCPAddHeader:            {"Add header", "添加请求头", "新增標頭", "헤더 추가", "ヘッダーを追加", "Добавить заголовок", "Ajouter un en-tête"},
		xMCPAdvanced:             {"Advanced", "高级选项", "進階選項", "고급", "詳細設定", "Дополнительно", "Options avancées"},
		xMCPOptional:             {"Optional", "可选", "選填", "선택 사항", "任意", "Необязательно", "Facultatif"},
		xMCPCwdHint:              {"Where the program starts; leave empty for the CLI's default", "程序启动时所在的目录，留空使用 CLI 默认值", "程式啟動時所在的目錄，留空使用 CLI 預設值", "프로그램이 시작되는 위치; 비워 두면 CLI 기본값", "プログラムの起動場所。空欄なら CLI の既定値", "Каталог запуска; пусто — по умолчанию CLI", "Dossier de démarrage ; vide pour la valeur par défaut du CLI"},
		xMCPBearerHint:           {"Name of an environment variable whose value is sent as Authorization: Bearer; the token itself is not saved", "填写环境变量名，其值作为 Authorization: Bearer 发送，配置中不保存令牌本身", "填寫環境變數名稱，其值作為 Authorization: Bearer 傳送，設定中不儲存權杖本身", "값이 Authorization: Bearer로 전송되는 환경 변수 이름; 토큰 자체는 저장하지 않습니다", "値を Authorization: Bearer として送る環境変数名。トークン自体は保存しません", "Имя переменной, значение которой отправляется как Authorization: Bearer; сам токен не сохраняется", "Nom d’une variable d’environnement envoyée comme Authorization: Bearer ; le jeton n’est pas enregistré"},
		xMCPEnvHeadersHint:       {"Each value is read from the named variable when the CLI connects; only the variable name is saved", "CLI 连接时从指定变量读取请求头的值，配置中只保存变量名", "CLI 連線時從指定變數讀取標頭的值，設定中只儲存變數名稱", "CLI가 연결할 때 지정한 변수에서 헤더 값을 읽으며 변수 이름만 저장합니다", "CLI の接続時に指定した変数から値を読みます。保存するのは変数名のみです", "Значение читается из переменной при подключении CLI; сохраняется только имя", "La valeur est lue dans la variable lors de la connexion du CLI ; seul son nom est enregistré"},
		xMCPHideSecrets:          {"Hide values", "隐藏值", "隱藏值", "값 숨기기", "値を隠す", "Скрыть значения", "Masquer les valeurs"},
		xMCPTransportStdio:       {"Local (stdio)", "本地 (stdio)", "本機 (stdio)", "로컬 (stdio)", "ローカル (stdio)", "Локально (stdio)", "Local (stdio)"},
		xMCPStdioHint:            {"Starts a local program and talks to it over standard input and output", "在本机启动程序，通过标准输入输出通信", "在本機啟動程式，透過標準輸入輸出通訊", "로컬 프로그램을 실행하고 표준 입출력으로 통신합니다", "ローカルでプログラムを起動し、標準入出力で通信します", "Запускает локальную программу и общается через стандартный ввод-вывод", "Démarre un programme local et communique par l’entrée et la sortie standard"},
		xMCPHTTPHint:             {"Connects to a remote server over Streamable HTTP", "通过 Streamable HTTP 连接远程服务", "透過 Streamable HTTP 連線遠端服務", "Streamable HTTP로 원격 서버에 연결합니다", "Streamable HTTP でリモートサーバーに接続します", "Подключение к удалённому серверу по Streamable HTTP", "Se connecte à un serveur distant en Streamable HTTP"},
		xMCPSSEHint:              {"Connects to an older server over Server-Sent Events; prefer HTTP for new servers", "通过 SSE 连接旧版服务，新服务建议使用 HTTP", "透過 SSE 連線舊版服務，新服務建議使用 HTTP", "SSE로 이전 방식 서버에 연결; 새 서버는 HTTP 권장", "SSE で旧方式のサーバーに接続します。新しいサーバーには HTTP を推奨", "Подключение к старому серверу по SSE; для новых лучше HTTP", "Se connecte à un ancien serveur en SSE ; préférez HTTP pour les nouveaux"},
		xMCPRefetch:              {"Fetch again", "重新获取", "重新取得", "다시 가져오기", "再取得", "Получить снова", "Récupérer à nouveau"},
		xMCPFetching:             {"Connecting…", "正在连接…", "正在連線…", "연결 중…", "接続中…", "Подключение…", "Connexion…"},
		xMCPToolCount:            {"Tools: %d", "%d 个工具", "%d 個工具", "도구 %d개", "ツール %d 件", "Инструментов: %d", "Outils : %d"},
		xMCPProtocol:             {"protocol %s", "协议 %s", "協定 %s", "프로토콜 %s", "プロトコル %s", "протокол %s", "protocole %s"},
		xMCPToolParams:           {"Parameters", "参数", "參數", "매개변수", "パラメーター", "Параметры", "Paramètres"},
		xMCPInputSchema:          {"Input schema", "输入结构", "輸入結構", "입력 스키마", "入力スキーマ", "Схема ввода", "Schéma d’entrée"},
		xMCPTargetsHint:          {"Choose the CLIs to receive these servers; each configuration is checked before anything is written", "选择要添加到的 CLI，写入前会先检查各自的配置", "選擇要新增到的 CLI，寫入前會先檢查各自的設定", "서버를 추가할 CLI를 선택하세요. 쓰기 전에 각 구성을 먼저 확인합니다", "追加先の CLI を選びます。書き込む前にそれぞれの設定を確認します", "Выберите CLI; перед записью их конфигурации будут проверены", "Choisissez les CLI cibles ; chaque configuration est vérifiée avant écriture"},
		xMCPTargetDone:           {"Added", "已添加", "已新增", "추가됨", "追加済み", "Добавлено", "Ajouté"},
		xMCPTargetBlocked:        {"Cannot add", "无法添加", "無法新增", "추가할 수 없음", "追加できません", "Нельзя добавить", "Ajout impossible"},
		xMCPPlanCounts:           {"%d to add · %d existing", "新增 %d · 已存在 %d", "新增 %d · 已存在 %d", "추가 %d · 기존 %d", "追加 %d · 既存 %d", "добавить: %d · есть: %d", "À ajouter : %d · existants : %d"},
		xMCPResultsDone:          {"Servers added", "已添加 MCP 服务", "已新增 MCP 服務", "MCP 서버를 추가했습니다", "MCP サーバーを追加しました", "Серверы MCP добавлены", "Serveurs MCP ajoutés"},
		xMCPResultsFailed:        {"Some providers could not be updated", "部分服务商未能添加", "部分服務商未能新增", "일부 공급자에 추가하지 못했습니다", "一部のプロバイダーに追加できませんでした", "Не удалось обновить некоторых провайдеров", "Certains fournisseurs n’ont pas pu être mis à jour"},
		xMCPDisabledList:         {"Grok disabled list", "Grok 停用名单", "Grok 停用名單", "Grok 비활성 목록", "Grok の無効化リスト", "Список отключённых Grok", "Liste des désactivés de Grok"},
	} {
		extraTexts[key] = texts
	}
}
