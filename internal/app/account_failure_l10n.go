package app

// Fixed translations only; error messages and external fields never enter this table.
func init() {
	for key, texts := range map[textKey][7]string{
		textKey("accountFailure_unknown"):             {"Unrecognized cause [unknown]", "原因未识别 [unknown]", "原因未識別 [unknown]", "원인을 식별하지 못했습니다 [unknown]", "原因を特定できません [unknown]", "Причина не определена [unknown]", "Cause non identifiée [unknown]"},
		textKey("accountFailure_permission"):          {"Permission denied", "没有访问权限", "沒有存取權限", "접근 권한이 없습니다", "アクセス権限がありません", "Доступ запрещён", "Accès refusé"},
		textKey("accountFailure_read_only"):           {"Read-only storage", "存储为只读", "儲存為唯讀", "읽기 전용 저장소", "ストレージは読み取り専用です", "Хранилище доступно только для чтения", "Stockage en lecture seule"},
		textKey("accountFailure_disk_full"):           {"Storage is full", "存储空间已满", "儲存空間已滿", "저장 공간이 가득 찼습니다", "空き容量がありません", "Недостаточно места", "Stockage plein"},
		textKey("accountFailure_not_found"):           {"Required file or item was not found", "所需文件或项目不存在", "所需檔案或項目不存在", "필요한 파일이나 항목이 없습니다", "必要なファイルまたは項目がありません", "Нужный файл или элемент не найден", "Fichier ou élément requis introuvable"},
		textKey("accountFailure_exists"):              {"Destination already exists", "目标已存在", "目標已存在", "대상이 이미 존재합니다", "保存先が既に存在します", "Объект уже существует", "La destination existe déjà"},
		textKey("accountFailure_invalid_data"):        {"Invalid data format", "数据格式无效", "資料格式無效", "데이터 형식이 잘못되었습니다", "データ形式が無効です", "Неверный формат данных", "Format de données invalide"},
		textKey("accountFailure_unsupported_version"): {"Unsupported data version", "数据版本不受支持", "資料版本不受支援", "지원하지 않는 데이터 버전", "未対応のデータバージョンです", "Версия данных не поддерживается", "Version des données non prise en charge"},
		textKey("accountFailure_not_private"):         {"Private file permissions could not be verified", "无法确认文件仅当前用户可访问", "無法確認檔案僅目前使用者可存取", "파일의 개인 접근 권한을 확인할 수 없습니다", "ファイルのプライベート権限を確認できません", "Не удалось проверить приватность файла", "Impossible de vérifier les permissions privées du fichier"},
		textKey("accountFailure_command"):             {"System command failed", "系统命令执行失败", "系統命令執行失敗", "시스템 명령 실패", "システムコマンドに失敗しました", "Системная команда завершилась ошибкой", "Échec de la commande système"},
		textKey("accountFailure_network"):             {"Network request failed", "网络请求失败", "網路請求失敗", "네트워크 요청 실패", "ネットワークリクエストに失敗しました", "Ошибка сетевого запроса", "Échec de la requête réseau"},
		textKey("accountFailure_dns"):                 {"DNS lookup failed", "DNS 解析失败", "DNS 解析失敗", "DNS 조회 실패", "DNS検索に失敗しました", "Ошибка DNS", "Échec de résolution DNS"},
		textKey("accountFailure_connect"):             {"Connection could not be established", "无法建立连接", "無法建立連線", "연결할 수 없습니다", "接続を確立できません", "Не удалось установить соединение", "Connexion impossible"},
		textKey("accountFailure_TLS"):                 {"TLS certificate or handshake failed", "TLS 证书或握手失败", "TLS 憑證或交握失敗", "TLS 인증서 또는 핸드셰이크 실패", "TLS証明書またはハンドシェイクに失敗しました", "Ошибка сертификата или согласования TLS", "Échec du certificat ou de la négociation TLS"},
		textKey("accountFailure_http"):                {"Server returned an error", "服务器返回错误", "伺服器傳回錯誤", "서버가 오류를 반환했습니다", "サーバーがエラーを返しました", "Сервер вернул ошибку", "Le serveur a renvoyé une erreur"},
		textKey("accountFailure_authentication"):      {"Authentication was rejected or expired", "登录凭据被拒绝或已过期", "登入憑證遭拒或已過期", "인증이 거부되었거나 만료되었습니다", "認証が拒否されたか期限切れです", "Авторизация отклонена или истекла", "Authentification refusée ou expirée"},
		textKey("accountFailure_rate_limited"):        {"Too many requests", "请求过于频繁", "請求過於頻繁", "요청이 너무 많습니다", "リクエストが多すぎます", "Слишком много запросов", "Trop de requêtes"},
		textKey("accountFailure_rollback_failed"):     {"Previous state could not be fully restored", "未能完整恢复之前的状态", "未能完整還原先前狀態", "이전 상태를 완전히 복원하지 못했습니다", "以前の状態を完全に復元できません", "Не удалось полностью восстановить прежнее состояние", "L’état précédent n’a pas pu être entièrement restauré"},
		textKey("accountFailure_changed"):             {"Credentials or configuration changed during the operation", "操作期间凭据或配置已变化", "操作期間憑證或設定已變更", "작업 중 인증 정보나 설정이 변경되었습니다", "処理中に認証情報または設定が変わりました", "Учётные данные или настройки изменились во время операции", "Les identifiants ou la configuration ont changé pendant l’opération"},
		textKey("accountFailure_payload_large"):       {"Credential exceeds the system storage limit", "凭据超过系统存储上限", "憑證超過系統儲存上限", "인증 정보가 시스템 저장 한도를 초과합니다", "認証情報がシステムの保存上限を超えています", "Учётные данные превышают лимит системы", "Les identifiants dépassent la limite de stockage système"},
		textKey("accountFailure_verify"):              {"Stored credential did not match the written value", "凭据写入后读回校验不一致", "憑證寫入後讀回驗證不一致", "저장된 인증 정보가 기록한 값과 다릅니다", "保存後の認証情報が書き込み値と一致しません", "Проверка записанных учётных данных не пройдена", "La vérification des identifiants enregistrés a échoué"},
		textKey("accountFailure_read_official"):       {"Read official login", "读取官方登录", "讀取官方登入", "공식 로그인 읽기", "公式ログインの読み取り", "Чтение официального входа", "Lecture de la connexion officielle"},
		textKey("accountFailure_read_keychain"):       {"Read Keychain", "读取钥匙串", "讀取鑰匙圈", "키체인 읽기", "キーチェーンの読み取り", "Чтение связки ключей", "Lecture du trousseau"},
		textKey("accountFailure_write_keychain"):      {"Write Keychain", "写入钥匙串", "寫入鑰匙圈", "키체인 쓰기", "キーチェーンへの書き込み", "Запись в связку ключей", "Écriture du trousseau"},
		textKey("accountFailure_rollback_keychain"):   {"Restore Keychain", "恢复钥匙串", "還原鑰匙圈", "키체인 복원", "キーチェーンの復元", "Восстановление связки ключей", "Restauration du trousseau"},
		textKey("accountFailure_read_index"):          {"Read account index", "读取账号索引", "讀取帳號索引", "계정 인덱스 읽기", "アカウント索引の読み取り", "Чтение индекса аккаунтов", "Lecture de l’index des comptes"},
		textKey("accountFailure_write_index"):         {"Save account index", "保存账号索引", "儲存帳號索引", "계정 인덱스 저장", "アカウント索引の保存", "Сохранение индекса аккаунтов", "Enregistrement de l’index des comptes"},
		textKey("accountFailure_read_credential"):     {"Read stored credential", "读取托管凭据", "讀取託管憑證", "저장된 인증 정보 읽기", "保存済み認証情報の読み取り", "Чтение сохранённых учётных данных", "Lecture des identifiants enregistrés"},
		textKey("accountFailure_write_credential"):    {"Save stored credential", "保存托管凭据", "儲存託管憑證", "인증 정보 저장", "認証情報の保存", "Сохранение учётных данных", "Enregistrement des identifiants"},
		textKey("accountFailure_prepare_directory"):   {"Prepare account directory", "准备账号目录", "準備帳號目錄", "계정 디렉터리 준비", "アカウントディレクトリの準備", "Подготовка каталога аккаунтов", "Préparation du dossier des comptes"},
		textKey("accountFailure_read_identity"):       {"Read account identity", "读取账号身份", "讀取帳號身分", "계정 식별 정보 읽기", "アカウント情報の読み取り", "Чтение личности аккаунта", "Lecture de l’identité du compte"},
		textKey("accountFailure_read_settings"):       {"Read configuration", "读取配置", "讀取設定", "설정 읽기", "設定の読み取り", "Чтение настроек", "Lecture de la configuration"},
		textKey("accountFailure_oauth_exchange"):      {"Exchange login authorization", "交换登录授权", "交換登入授權", "로그인 인증 교환", "ログイン認証の交換", "Обмен авторизации входа", "Échange de l’autorisation de connexion"},
		textKey("accountFailure_oauth_identity"):      {"Verify login identity", "核实登录身份", "核實登入身分", "로그인 신원 확인", "ログイン情報の確認", "Проверка личности входа", "Vérification de l’identité de connexion"},
		textKey("accountFailure_timeout"):             {"Operation timed out", "操作超时", "操作逾時", "작업 시간 초과", "処理がタイムアウトしました", "Время операции истекло", "Délai de l’opération dépassé"},
		textKey("accountFailure_canceled"):            {"Operation canceled", "操作已取消", "操作已取消", "작업 취소됨", "処理がキャンセルされました", "Операция отменена", "Opération annulée"},
	} {
		extraTexts[key] = texts
	}
}

const (
	xAccountBatchResult textKey = "accountBatchResult"
	xAccountSaveFailed  textKey = "accountSaveFailed"
)

func init() {
	extraTexts[xAccountBatchResult] = [7]string{"%d succeeded, %d failed", "%d 项成功，%d 项失败", "%d 項成功，%d 項失敗", "%d개 성공, %d개 실패", "%d件成功、%d件失敗", "Успешно: %d, ошибок: %d", "%d réussites, %d échecs"}
	extraTexts[xAccountSaveFailed] = [7]string{"Account save failed", "账号保存失败", "帳號儲存失敗", "계정 저장 실패", "アカウントの保存に失敗しました", "Не удалось сохранить аккаунт", "Échec de l’enregistrement du compte"}
}

func init() {
	extraTexts["accountFailure_refresh"] = [7]string{"Refresh credentials", "刷新凭据", "重新整理憑證", "인증 정보 새로 고침", "認証情報の更新", "Обновление учётных данных", "Actualisation des identifiants"}
	extraTexts["accountFailure_read_usage"] = [7]string{"Read subscription usage", "读取订阅用量", "讀取訂閱用量", "구독 사용량 읽기", "サブスクリプション使用量の読み取り", "Чтение использования подписки", "Lecture de l’utilisation de l’abonnement"}
}

func init() {
	extraTexts["accountFailure_rollback_credential"] = [7]string{"Restore saved credential", "恢复托管凭据", "還原託管憑證", "저장된 인증 정보 복원", "保存済み認証情報の復元", "Восстановление сохранённых учётных данных", "Restauration des identifiants enregistrés"}
	extraTexts["accountFailure_original_failure"] = [7]string{"Original failure", "原始失败", "原始失敗", "원래 오류", "元のエラー", "Первоначальная ошибка", "Erreur initiale"}
	extraTexts["accountFailure_recovery_failure"] = [7]string{"Recovery failure", "恢复失败", "還原失敗", "복구 오류", "復元エラー", "Ошибка восстановления", "Erreur de restauration"}
	extraTexts["accountFailure_oauth_discovery"] = [7]string{"Login service discovery failed", "登录服务发现失败", "登入服務探索失敗", "로그인 서비스 검색 실패", "ログインサービスの検出に失敗しました", "Не удалось обнаружить службу входа", "Échec de la découverte du service de connexion"}
	extraTexts["accountFailure_oauth_device"] = [7]string{"Device authorization request failed", "设备授权请求失败", "裝置授權請求失敗", "기기 인증 요청 실패", "デバイス認証リクエストに失敗しました", "Ошибка запроса авторизации устройства", "Échec de la demande d’autorisation de l’appareil"}
}

func init() {
	extraTexts["accountFailure_locked"] = [7]string{"Another process holds the login file lock", "其他进程持有登录文件锁", "其他處理程序持有登入檔案鎖", "다른 프로세스가 로그인 파일을 잠그고 있습니다", "別のプロセスがログインファイルをロックしています", "Файл входа заблокирован другим процессом", "Un autre processus verrouille le fichier de connexion"}
	extraTexts["accountFailure_identity_mismatch"] = [7]string{"Credential belongs to a different account", "凭据属于其他账号", "憑證屬於其他帳號", "인증 정보가 다른 계정에 속합니다", "認証情報が別のアカウントに属しています", "Учётные данные принадлежат другому аккаунту", "Les identifiants appartiennent à un autre compte"}
	extraTexts["accountFailure_processes_running"] = [7]string{"The CLI is still running; confirm the switch first", "CLI 仍在运行，请先确认切换", "CLI 仍在執行，請先確認切換", "CLI가 실행 중입니다; 먼저 전환을 확인하세요", "CLIが実行中です、先に切り替えを確認してください", "CLI ещё работает; сначала подтвердите переключение", "Le CLI est encore actif ; confirmez d’abord le changement"}
	extraTexts["accountFailure_provider_mismatch"] = [7]string{"The account archive is for a different provider", "账号归档属于其他供应商", "帳號封存屬於其他供應商", "계정 아카이브가 다른 공급자용입니다", "アカウントアーカイブは別のプロバイダー用です", "Архив аккаунтов предназначен для другого провайдера", "L’archive de comptes concerne un autre fournisseur"}
}

func init() {
	extraTexts["accountFailure_oauth_callback"] = [7]string{"Listen for login callback", "监听登录回调", "監聽登入回呼", "로그인 콜백 수신", "ログインコールバックの待ち受け", "Ожидание обратного вызова входа", "Écoute du retour de connexion"}
	extraTexts["accountFailure_address_in_use"] = [7]string{"Port is already in use", "端口已被占用", "連接埠已被佔用", "포트가 이미 사용 중입니다", "ポートが既に使用されています", "Порт уже используется", "Le port est déjà utilisé"}
}

const xAccountPreviewFailures textKey = "accountPreviewFailures"

func init() {
	extraTexts[xAccountPreviewFailures] = [7]string{"%d items could not be previewed", "%d 项无法预览", "%d 項無法預覽", "%d개 항목을 미리 볼 수 없습니다", "%d件をプレビューできません", "Не удалось просмотреть элементов: %d", "%d éléments n’ont pas pu être prévisualisés"}
}

func init() {
	extraTexts["accountFailure_invalid_index"] = [7]string{"Account index file format is invalid or corrupted", "账号索引文件格式无效或已损坏", "帳號索引檔案格式無效或已損毀", "계정 인덱스 파일 형식이 잘못되었거나 손상되었습니다", "アカウント索引ファイルの形式が無効か破損しています", "Файл индекса аккаунтов имеет неверный формат или повреждён", "Le format du fichier d’index des comptes est invalide ou corrompu"}
}
