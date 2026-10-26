package app

func init() {
	for key, texts := range map[textKey][7]string{
		xDesktopBind:            {"Bind Claude Desktop", "绑定 Claude Desktop", "綁定 Claude Desktop", "Claude Desktop 연결", "Claude Desktop を紐付け", "Привязать Claude Desktop", "Associer Claude Desktop"},
		xDesktopOpen:            {"Open bound Desktop account", "打开已绑定 Desktop 账号", "開啟已綁定 Desktop 帳號", "연결된 Desktop 계정 열기", "紐付け済み Desktop アカウントを開く", "Открыть привязанный аккаунт Desktop", "Ouvrir le compte Desktop associé"},
		xDesktopSwitch:          {"Also switch Claude Desktop", "同时切换 Claude Desktop", "同時切換 Claude Desktop", "Claude Desktop도 전환", "Claude Desktop も切り替える", "Также переключить Claude Desktop", "Changer aussi Claude Desktop"},
		xDesktopWarning:         {"Save your Desktop tasks first; this quits Desktop and saves/restores its complete local configuration, including sign-in, settings and local state, at its official data path; Quosparr does not verify the signed-in identity automatically", "请先保存 Desktop 任务，操作会退出 Desktop，并在官方数据路径保存或恢复完整本地配置，包括登录、设置及本地状态，Quosparr 不会自动验证登录身份", "請先儲存 Desktop 工作，操作會結束 Desktop，並在官方資料路徑儲存或還原完整本機設定，包括登入、設定及本機狀態，Quosparr 不會自動驗證登入身分", "Desktop 작업을 먼저 저장하세요; Desktop을 종료하고 공식 데이터 경로에서 로그인, 설정, 로컬 상태를 포함한 전체 구성을 저장하거나 복원합니다; Quosparr는 로그인 신원을 자동 확인하지 않습니다", "Desktop の作業を保存してください、Desktop を終了し公式データパスでログイン・設定・ローカル状態を含む構成全体を保存または復元します、ログインの本人確認は自動では行いません", "Сначала сохраните задачи Desktop; приложение завершится, а его полная локальная конфигурация со входом, настройками и состоянием будет сохранена или восстановлена по штатному пути; Quosparr не проверяет личность автоматически", "Enregistrez vos tâches Desktop ; l’application sera fermée et sa configuration locale complète, connexion, réglages et état local, sera enregistrée ou restaurée au chemin officiel ; Quosparr ne vérifie pas automatiquement l’identité"},
		xDesktopConsent:         {"I agree to quit Desktop and save/restore its complete local configuration", "我同意退出 Desktop 并保存或恢复完整本地配置", "我同意結束 Desktop 並儲存或還原完整本機設定", "Desktop 종료 및 전체 로컬 구성 저장/복원에 동의합니다", "Desktop を終了しローカル構成全体を保存・復元することに同意します", "Согласен завершить Desktop и сохранить/восстановить всю локальную конфигурацию", "J’accepte de fermer Desktop et d’enregistrer/restaurer sa configuration locale complète"},
		xDesktopUnbound:         {"Desktop is not bound; bind this subscription before enabling linked switching", "Desktop 尚未绑定，请先绑定此订阅账号后启用联动", "Desktop 尚未綁定，請先綁定此訂閱帳號再啟用連動", "Desktop 미연결; 연동 전환 전에 이 구독 계정을 연결하세요", "Desktop は未紐付けです、連動する前にこのサブスクリプションを紐付けてください", "Desktop не привязан; сначала привяжите этот аккаунт подписки", "Desktop non associé ; associez cet abonnement avant d’activer le changement lié"},
		xDesktopBound:           {"Bound by your manual confirmation; sign-in may change in Desktop", "已由你手动确认绑定，Desktop 内的登录仍可能变化", "已由你手動確認綁定，Desktop 內的登入仍可能變更", "수동 확인으로 연결됨; Desktop 로그인이 바뀔 수 있습니다", "手動確認で紐付け済みです、Desktop 内のログインは変更される場合があります", "Привязано по вашему подтверждению; вход в Desktop может измениться", "Association confirmée manuellement ; la connexion Desktop peut changer"},
		xDesktopPending:         {"Sign in to the account and organization shown above in Desktop, then return here; finishing saves this profile and restores the previous Desktop configuration", "请在 Desktop 登录上方账号及组织后返回，完成绑定会保存该配置档并恢复之前的 Desktop 配置", "請在 Desktop 登入上方帳號及組織後返回，完成綁定會儲存此設定檔並還原先前的 Desktop 設定", "Desktop에서 위 계정과 조직에 로그인한 뒤 돌아오세요; 완료하면 이 프로필을 저장하고 이전 Desktop 구성을 복원합니다", "Desktop で上記アカウントと組織にログインして戻ってください、完了時にこのプロファイルを保存し以前の Desktop 構成を復元します", "Войдите в указанный аккаунт и организацию в Desktop и вернитесь; завершение сохранит профиль и восстановит предыдущую конфигурацию Desktop", "Connectez-vous au compte et à l’organisation ci-dessus dans Desktop puis revenez ; la validation enregistre ce profil et restaure la configuration précédente"},
		xDesktopConfirmIdentity: {"I have signed in to this account and organization", "我已登录此账号及组织", "我已登入此帳號及組織", "이 계정과 조직에 로그인했습니다", "このアカウントと組織にログインしました", "Я вошёл в этот аккаунт и организацию", "Je me suis connecté à ce compte et à cette organisation"},
		xDesktopFinish:          {"Confirm binding and restore previous Desktop", "确认绑定并恢复之前的 Desktop", "確認綁定並還原先前的 Desktop", "연결 확인 및 이전 Desktop 복원", "紐付けを確認し以前の Desktop を復元", "Подтвердить и восстановить предыдущий Desktop", "Confirmer et restaurer le Desktop précédent"},
		xDesktopPath:            {"Desktop application path (blank: discover automatically)", "Desktop 应用路径，留空自动查找", "Desktop 應用程式路徑，留空自動尋找", "Desktop 앱 경로 (비워두면 자동 검색)", "Desktop アプリのパス（空欄で自動検出）", "Путь к Desktop (пусто: найти автоматически)", "Chemin de Desktop (vide : recherche automatique)"},
		xDesktopChoose:          {"Choose Desktop application", "选择 Desktop 应用", "選擇 Desktop 應用程式", "Desktop 앱 선택", "Desktop アプリを選択", "Выбрать приложение Desktop", "Choisir l’application Desktop"},
		xDesktopStart:           {"Start manual binding", "开始手动绑定", "開始手動綁定", "수동 연결 시작", "手動紐付けを開始", "Начать ручную привязку", "Commencer l’association manuelle"},
		xDesktopFailed:          {"Desktop operation failed; review the separate CLI and Desktop states below, then retry recovery if offered", "Desktop 操作失败，请核对下方分别列出的 CLI 与 Desktop 状态，如有恢复入口请重试恢复", "Desktop 操作失敗，請核對下方分別列出的 CLI 與 Desktop 狀態，如有還原入口請重試還原", "Desktop 작업 실패; 아래 CLI와 Desktop 상태를 각각 확인하고 복원 옵션이 있으면 다시 시도하세요", "Desktop の操作に失敗しました、以下の CLI と Desktop の状態を個別に確認し復元を再試行してください", "Операция Desktop не выполнена; проверьте отдельные состояния CLI и Desktop ниже и повторите восстановление, если доступно", "Échec de l’opération Desktop ; vérifiez séparément les états CLI et Desktop ci-dessous et réessayez la restauration si proposée"},
		xDesktopStarted:         {"Desktop profile restored and launch requested; confirm the account in Desktop", "Desktop 配置档已恢复并已请求启动，请在 Desktop 内核对账号", "Desktop 設定檔已還原並已要求啟動，請在 Desktop 內核對帳號", "Desktop 프로필 복원 및 시작 요청 완료; Desktop에서 계정을 확인하세요", "Desktop プロファイルを復元し起動を要求しました、Desktop 内でアカウントを確認してください", "Профиль Desktop восстановлен и запуск запрошен; проверьте аккаунт в Desktop", "Profil Desktop restauré et lancement demandé ; vérifiez le compte dans Desktop"},
		xDesktopNotStarted:      {"Desktop launch was not confirmed", "尚未确认 Desktop 启动", "尚未確認 Desktop 啟動", "Desktop 시작이 확인되지 않았습니다", "Desktop の起動は未確認です", "Запуск Desktop не подтверждён", "Lancement de Desktop non confirmé"},
		xDesktopCLIChanged:      {"CLI: account switched", "CLI：账号已切换", "CLI：帳號已切換", "CLI: 계정 전환됨", "CLI：アカウント切り替え済み", "CLI: аккаунт переключён", "CLI : compte changé"},
		xDesktopCLIUnchanged:    {"CLI: previous account retained or restored", "CLI：原账号已保留或恢复", "CLI：原帳號已保留或還原", "CLI: 이전 계정 유지 또는 복원됨", "CLI：以前のアカウントを保持または復元", "CLI: предыдущий аккаунт сохранён или восстановлен", "CLI : compte précédent conservé ou restauré"},
		xDesktopRestored:        {"Previous Desktop configuration restored", "之前的 Desktop 配置已恢复", "先前的 Desktop 設定已還原", "이전 Desktop 구성 복원됨", "以前の Desktop 構成を復元しました", "Предыдущая конфигурация Desktop восстановлена", "Configuration Desktop précédente restaurée"},
		xDesktopRecovery:        {"Desktop recovery is required; do not replace or remove profile directories manually", "Desktop 需要恢复，请勿手动替换或移除配置档目录", "Desktop 需要還原，請勿手動替換或移除設定檔目錄", "Desktop 복원이 필요합니다; 프로필 디렉터리를 수동으로 교체하거나 제거하지 마세요", "Desktop の復元が必要です、プロファイルのディレクトリを手動で置換・削除しないでください", "Требуется восстановление Desktop; не заменяйте и не удаляйте каталоги профилей вручную", "Restauration de Desktop nécessaire ; ne remplacez ni ne supprimez manuellement les dossiers de profils"},
		xDesktopRecover:         {"Restore previous Desktop configuration", "恢复上次 Desktop 配置", "還原上次 Desktop 設定", "이전 Desktop 구성 복원", "前回の Desktop 構成を復元", "Восстановить предыдущий Desktop", "Restaurer la configuration Desktop précédente"},
		xDesktopCLIRecovery:     {"CLI: verify the active account and switch manually if needed", "CLI：请核对当前账号，必要时手动切换", "CLI：請核對目前帳號，必要時手動切換", "CLI: 현재 계정을 확인하고 필요하면 수동 전환하세요", "CLI：現在のアカウントを確認し必要に応じて手動で切り替えてください", "CLI: проверьте активный аккаунт и при необходимости переключите вручную", "CLI : vérifiez le compte actif et changez-le manuellement si nécessaire"},
	} {
		extraTexts[key] = texts
	}
}

const (
	xDesktopBind            textKey = "desktopBind"
	xDesktopOpen            textKey = "desktopOpen"
	xDesktopSwitch          textKey = "desktopSwitch"
	xDesktopWarning         textKey = "desktopWarning"
	xDesktopConsent         textKey = "desktopConsent"
	xDesktopUnbound         textKey = "desktopUnbound"
	xDesktopBound           textKey = "desktopBound"
	xDesktopPending         textKey = "desktopPending"
	xDesktopConfirmIdentity textKey = "desktopConfirmIdentity"
	xDesktopFinish          textKey = "desktopFinish"
	xDesktopPath            textKey = "desktopPath"
	xDesktopChoose          textKey = "desktopChoose"
	xDesktopStart           textKey = "desktopStart"
	xDesktopFailed          textKey = "desktopFailed"
	xDesktopStarted         textKey = "desktopStarted"
	xDesktopNotStarted      textKey = "desktopNotStarted"
	xDesktopCLIChanged      textKey = "desktopCLIChanged"
	xDesktopCLIUnchanged    textKey = "desktopCLIUnchanged"
	xDesktopRestored        textKey = "desktopRestored"
	xDesktopRecovery        textKey = "desktopRecovery"
	xDesktopRecover         textKey = "desktopRecover"
	xDesktopCLIRecovery     textKey = "desktopCLIRecovery"
)
