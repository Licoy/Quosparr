package app

const (
	xDesktopLoading       textKey = "desktopLoading"
	xDesktopRecoveryTitle textKey = "desktopRecoveryTitle"
	xDesktopStopFailed    textKey = "desktopStopFailed"
	xDesktopLaunchFailed  textKey = "desktopLaunchFailed"
	xDesktopBusy          textKey = "desktopBusy"
	xDesktopUnsafe        textKey = "desktopUnsafe"
	xDesktopChanged       textKey = "desktopChanged"
	xDesktopCrossVolume   textKey = "desktopCrossVolume"
	xDesktopPersistence   textKey = "desktopPersistence"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xDesktopLoading:       {"Reading target account…", "正在读取目标账号…", "正在讀取目標帳號…", "대상 계정 읽는 중…", "対象アカウントを読み込み中…", "Чтение целевого аккаунта…", "Lecture du compte cible…"},
		xDesktopRecoveryTitle: {"Restore Claude Desktop", "恢复 Claude Desktop", "還原 Claude Desktop", "Claude Desktop 복원", "Claude Desktop を復元", "Восстановить Claude Desktop", "Restaurer Claude Desktop"},
		xDesktopStopFailed:    {"Could not confirm Desktop has exited; save your tasks and quit it normally", "无法确认 Desktop 已退出，请保存任务并正常退出", "無法確認 Desktop 已結束，請儲存工作並正常結束", "Desktop 종료를 확인할 수 없습니다; 작업 저장 후 정상 종료하세요", "Desktop の終了を確認できません、作業を保存して通常終了してください", "Не удалось подтвердить завершение Desktop; сохраните задачи и выйдите обычным способом", "Impossible de confirmer la fermeture de Desktop ; enregistrez vos tâches et quittez normalement"},
		xDesktopLaunchFailed:  {"Desktop could not be started; check its installation and the restore status", "Desktop 启动失败，请检查安装及恢复状态", "Desktop 啟動失敗，請檢查安裝及還原狀態", "Desktop 시작 실패; 설치 및 복원 상태를 확인하세요", "Desktop を起動できません、インストールと復元状態を確認してください", "Не удалось запустить Desktop; проверьте установку и состояние восстановления", "Impossible de lancer Desktop ; vérifiez son installation et l’état de restauration"},
		xDesktopBusy:          {"Another Desktop operation is running; wait for it to finish", "另一个 Desktop 操作正在进行，请等待完成", "另一個 Desktop 操作正在進行，請等待完成", "다른 Desktop 작업이 진행 중입니다; 완료될 때까지 기다리세요", "別の Desktop 操作の完了を待ってください", "Другая операция Desktop выполняется; дождитесь завершения", "Une autre opération Desktop est en cours ; attendez sa fin"},
		xDesktopUnsafe:        {"The application or data path is unsafe or unsupported; select the correct installation", "应用或数据路径不安全或不支持，请选择正确安装", "應用程式或資料路徑不安全或不支援，請選擇正確安裝", "앱 또는 데이터 경로가 안전하지 않거나 지원되지 않습니다; 올바른 설치를 선택하세요", "アプリまたはデータパスが安全でないか未対応です、正しいインストールを選択してください", "Путь приложения или данных небезопасен либо не поддерживается; выберите правильную установку", "Chemin d’application ou de données non sûr ou non pris en charge ; choisissez la bonne installation"},
		xDesktopChanged:       {"Desktop configuration changed outside Quosparr; no existing destination will be overwritten", "Desktop 配置已被外部改动，不会覆盖现有目标", "Desktop 設定已被外部變更，不會覆寫現有目標", "Desktop 구성이 외부에서 변경되었습니다; 기존 대상을 덮어쓰지 않습니다", "Desktop の構成が外部で変更されています、既存の保存先は上書きしません", "Конфигурация Desktop изменена извне; существующий каталог не будет перезаписан", "La configuration Desktop a changé hors de Quosparr ; la destination existante ne sera pas écrasée"},
		xDesktopCrossVolume:   {"Desktop data and Quosparr profiles must be on the same volume; cross-volume moves are unsupported", "Desktop 数据与 Quosparr 配置档必须在同一卷，不支持跨卷移动", "Desktop 資料與 Quosparr 設定檔必須在同一磁碟區，不支援跨磁碟區移動", "Desktop 데이터와 Quosparr 프로필은 같은 볼륨에 있어야 합니다; 다른 볼륨 이동은 지원하지 않습니다", "Desktop データと Quosparr プロファイルは同一ボリュームが必要です、別ボリュームへの移動は未対応です", "Данные Desktop и профили Quosparr должны находиться на одном томе; перенос между томами не поддерживается", "Les données Desktop et les profils Quosparr doivent être sur le même volume ; les déplacements entre volumes ne sont pas pris en charge"},
		xDesktopPersistence:   {"Could not save Desktop operation state; review recovery before trying again", "无法保存 Desktop 操作状态，请先检查恢复状态再重试", "無法儲存 Desktop 操作狀態，請先檢查還原狀態再重試", "Desktop 작업 상태를 저장할 수 없습니다; 복원 상태를 확인한 뒤 다시 시도하세요", "Desktop の操作状態を保存できません、復元状態を確認してから再試行してください", "Не удалось сохранить состояние операции Desktop; проверьте восстановление перед повтором", "Impossible d’enregistrer l’état de l’opération Desktop ; vérifiez la restauration avant de réessayer"},
	} {
		extraTexts[key] = texts
	}
}
