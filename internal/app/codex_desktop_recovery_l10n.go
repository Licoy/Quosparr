package app

import "github.com/licoy/quosparr/internal/l10n"

const (
	xProjectNothingRecoverable textKey = "codexProjectNothingRecoverable"
	xProjectRecover            textKey = "codexProjectRecover"
	xProjectRecoveryHint       textKey = "codexProjectRecoveryHint"
	xProjectSelected           textKey = "codexProjectSelected"
	xProjectSnapshot           textKey = "codexProjectSnapshot"
	xProjectCount              textKey = "codexProjectCount"
	xProjectSummary            textKey = "codexProjectSummary"
	xProjectPreview            textKey = "codexProjectPreview"
	xProjectConfirm            textKey = "codexProjectConfirm"
	xProjectRecovered          textKey = "codexProjectRecovered"
	xProjectNoSource           textKey = "codexProjectNoSource"
	xProjectPartial            textKey = "codexProjectPartial"
	xProjectConflict           textKey = "codexProjectConflict"
	xProjectChanged            textKey = "codexProjectChanged"
	xProjectFormat             textKey = "codexProjectFormat"
	xProjectSkipRemote         textKey = "codexProjectSkipRemote"
	xProjectSkipThread         textKey = "codexProjectSkipThread"
	xProjectSkipMapping        textKey = "codexProjectSkipMapping"
	xProjectSkipUnknown        textKey = "codexProjectSkipUnknown"
	xProjectRestartFailed      textKey = "codexProjectRestartFailed"
	xProjectBusy               textKey = "codexProjectBusy"
	xProjectStopFailed         textKey = "codexProjectStopFailed"
	xProjectTarget             textKey = "codexProjectTarget"
	xProjectCurrentAccount     textKey = "codexProjectCurrentAccount"
	xProjectRecheck            textKey = "codexProjectRecheck"
	xProjectDatabase           textKey = "codexProjectDatabase"
	xProjectIO                 textKey = "codexProjectIO"
	xProjectReopen             textKey = "codexProjectReopen"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xProjectNothingRecoverable: {"No projects or chats can be recovered from this source", "此来源没有可恢复的项目或聊天", "此來源沒有可還原的專案或對話", "이 원본에서 복원할 수 있는 프로젝트나 대화가 없습니다", "このソースから復元できるプロジェクトや会話がありません", "Из этого источника нельзя восстановить проекты или чаты", "Aucun projet ni discussion ne peut être récupéré depuis cette source"},
		xProjectRecover:            {"Restore projects from a local account", "从本机账号恢复项目", "從本機帳號還原專案", "로컬 계정에서 프로젝트 복원", "ローカルアカウントからプロジェクトを復元", "Восстановить проекты из локального аккаунта", "Restaurer les projets d’un compte local"},
		xProjectRecoveryHint:       {"Choose a saved source, preview the changes, then confirm recovery into the currently signed-in account; the ChatGPT desktop app will quit and reopen as needed", "请选择保存的来源，预览后确认恢复到当前登录账号，操作会按需退出并重新打开 ChatGPT 桌面端", "請選擇儲存的來源，預覽後確認還原至目前登入帳號，操作會視需要結束並重新開啟 ChatGPT 桌面版", "저장된 원본을 선택하고 미리 본 뒤 현재 로그인 계정으로 복원하세요; 필요하면 ChatGPT Desktop를 종료하고 재시작합니다", "保存済みのソースを選び、確認後に現在ログイン中のアカウントへ復元します、必要に応じて ChatGPT Desktop を停止・再起動します", "Выберите сохранённый источник и проверьте изменения перед восстановлением в текущий аккаунт; ChatGPT Desktop будет остановлен и перезапущен при необходимости", "Choisissez une source enregistrée, vérifiez puis confirmez la restauration dans le compte connecté actuel ; ChatGPT Desktop sera arrêté et redémarré si nécessaire"},
		xProjectSelected:           {"Selected source", "已选择此来源", "已選擇此來源", "선택한 원본", "選択中のソース", "Выбранный источник", "Source sélectionnée"},
		xProjectSnapshot:           {"Local snapshot %@", "本机快照 %@", "本機快照 %@", "로컬 스냅샷 %@", "ローカルスナップショット %@", "Локальный снимок %@", "Instantané local %@"},
		xProjectCount:              {"%@ projects", "%@ 个项目", "%@ 個專案", "프로젝트 %@개", "%@ 件のプロジェクト", "Проектов: %@", "%@ projets"},
		xProjectSummary:            {"Projects: %@ · Recoverable chats: %@ · Skipped entries: %@", "项目：%@ · 可恢复聊天：%@ · 跳过条目：%@", "專案：%@ · 可還原對話：%@ · 略過項目：%@", "프로젝트: %@ · 복원 가능한 대화: %@ · 건너뜀: %@", "プロジェクト: %@ · 復元可能な会話: %@ · スキップ: %@", "Проекты: %@ · Восстанавливаемые чаты: %@ · Пропущено: %@", "Projets : %@ · Discussions récupérables : %@ · Éléments ignorés : %@"},
		xProjectPreview:            {"Preview recovery", "预览恢复", "預覽還原", "복원 미리 보기", "復元をプレビュー", "Предпросмотр восстановления", "Aperçu de la restauration"},
		xProjectConfirm:            {"Confirm recovery", "确认恢复", "確認還原", "복원 확인", "復元を確認", "Подтвердить восстановление", "Confirmer la restauration"},
		xProjectRecovered:          {"Projects restored to the current account", "已恢复项目到当前账号", "已還原專案至目前帳號", "현재 계정으로 프로젝트를 복원했습니다", "現在のアカウントにプロジェクトを復元しました", "Проекты восстановлены в текущем аккаунте", "Projets restaurés dans le compte actuel"},
		xProjectNoSource:           {"No saved local projects are available", "没有可用的本机项目来源", "沒有可用的本機專案來源", "저장된 로컬 프로젝트가 없습니다", "保存済みのローカルプロジェクトがありません", "Нет сохранённых локальных проектов", "Aucun projet local enregistré disponible"},
		xProjectPartial:            {"Only part of the changes was written; review local data or undo the last sharing on the accounts page", "仅写入部分修改，请核对本机数据，可在账号页撤销上次共享", "僅寫入部分修改，請核對本機資料，可在帳號頁撤銷上次共用", "일부 변경만 기록되었습니다; 로컬 데이터를 확인하거나 계정 페이지에서 마지막 공유를 취소하세요", "一部のみ書き込まれました、データを確認するかアカウント画面で前回の共有を取り消してください", "Изменения записаны частично; проверьте данные или отмените последний перенос на странице аккаунтов", "Modifications partiellement écrites ; vérifiez les données ou annulez le dernier partage sur la page des comptes"},
		xProjectConflict:           {"Recovery conflicts with existing data; nothing was replaced", "恢复内容与已有数据冲突，未替换数据", "還原內容與現有資料衝突，未取代資料", "기존 데이터와 충돌하여 복원하지 않았습니다", "既存データと競合するため置換しませんでした", "Конфликт с существующими данными; замена не выполнена", "Conflit avec les données existantes ; aucun remplacement effectué"},
		xProjectChanged:            {"The current account or local data changed; reopen recovery and preview again", "当前账号或本机数据已变化，请重新打开恢复并预览", "目前帳號或本機資料已變更，請重新開啟還原並預覽", "현재 계정이나 로컬 데이터가 변경되었습니다; 다시 열어 미리 보세요", "現在のアカウントまたはデータが変更されました、復元を開き直して再確認してください", "Текущий аккаунт или данные изменились; откройте восстановление и проверьте заново", "Le compte actuel ou les données ont changé ; rouvrez et vérifiez la restauration"},
		xProjectFormat:             {"This local data format is not supported", "不支持此本机数据格式", "不支援此本機資料格式", "지원하지 않는 로컬 데이터 형식입니다", "このローカルデータ形式には対応していません", "Формат локальных данных не поддерживается", "Format des données locales non pris en charge"},
		xProjectSkipRemote:         {"Cloud or remote content cannot be restored locally", "云端或远程内容不能从本机恢复", "雲端或遠端內容無法從本機還原", "클라우드 또는 원격 콘텐츠는 로컬에서 복원할 수 없습니다", "クラウドまたはリモートの内容はローカルから復元できません", "Облачные и удалённые данные нельзя восстановить локально", "Le contenu distant ou cloud ne peut pas être restauré localement"},
		xProjectSkipThread:         {"Some chats have no local conversation records", "部分聊天缺少本机会话记录", "部分對話缺少本機會話記錄", "일부 대화에 로컬 기록이 없습니다", "一部の会話にローカル記録がありません", "Для некоторых чатов нет локальных записей", "Certaines discussions n’ont pas d’historique local"},
		xProjectSkipMapping:        {"Some entries lack a matching local project or folder", "部分条目缺少匹配的本机项目或文件夹", "部分項目缺少相符的本機專案或資料夾", "일부 항목에 일치하는 로컬 프로젝트나 폴더가 없습니다", "一部の項目に一致するローカルプロジェクトやフォルダがありません", "Для некоторых записей нет соответствующего локального проекта или папки", "Certains éléments n’ont pas de projet ou dossier local correspondant"},
		xProjectSkipUnknown:        {"Some entries could not be validated for recovery", "部分条目未通过恢复校验", "部分項目未通過還原驗證", "일부 항목을 복원 대상으로 확인하지 못했습니다", "一部の項目の復元を検証できませんでした", "Некоторые записи не прошли проверку восстановления", "Certains éléments n’ont pas pu être validés pour la restauration"},
		xProjectRestartFailed:      {"Projects were restored, but ChatGPT Desktop did not restart; start it manually", "项目已恢复，但 ChatGPT Desktop 未能重新启动，请手动启动", "專案已還原，但 ChatGPT Desktop 未能重新啟動，請手動啟動", "프로젝트는 복원되었지만 ChatGPT Desktop 재시작에 실패했습니다; 수동으로 시작하세요", "プロジェクトは復元済みですが ChatGPT Desktop を再起動できません、手動で起動してください", "Проекты восстановлены, но ChatGPT Desktop не перезапустился; запустите его вручную", "Projets restaurés, mais ChatGPT Desktop n’a pas redémarré ; lancez-le manuellement"},
		xProjectBusy:               {"Another account operation is running; try again when it finishes", "其他账号操作正在进行，请稍后重试", "其他帳號操作正在進行，請稍後重試", "다른 계정 작업이 진행 중입니다; 완료 후 다시 시도하세요", "別のアカウント操作が実行中です、完了後に再試行してください", "Выполняется другая операция с аккаунтом; повторите позже", "Une autre opération est en cours ; réessayez après sa fin"},
		xProjectStopFailed:         {"ChatGPT Desktop could not be stopped safely; recovery was not started", "未能安全停止 ChatGPT Desktop，尚未开始恢复", "未能安全停止 ChatGPT Desktop，尚未開始還原", "ChatGPT Desktop를 안전하게 종료하지 못하여 복원하지 않았습니다", "ChatGPT Desktop を安全に停止できず復元を開始しませんでした", "Не удалось безопасно остановить ChatGPT Desktop; восстановление не начато", "Impossible d’arrêter ChatGPT Desktop correctement ; restauration non commencée"},
		xProjectTarget:             {"Recovery target: %@", "恢复目标：%@", "還原目標：%@", "복원 대상: %@", "復元先: %@", "Цель восстановления: %@", "Compte cible : %@"},
		xProjectCurrentAccount:     {"Current OAuth account", "当前登录账号", "目前登入帳號", "현재 로그인 계정", "現在ログイン中のアカウント", "Текущий аккаунт", "Compte connecté actuel"},
		xProjectRecheck:            {"Check again", "重新检查", "重新檢查", "다시 확인", "再確認", "Проверить снова", "Vérifier à nouveau"},
		xProjectDatabase:           {"The local chat database could not be read or updated", "无法读取或更新本机聊天数据库", "無法讀取或更新本機對話資料庫", "로컬 대화 데이터베이스를 읽거나 갱신할 수 없습니다", "ローカル会話データベースを読み取り・更新できません", "Не удалось прочитать или обновить локальную базу чатов", "Impossible de lire ou mettre à jour la base locale des discussions"},
		xProjectIO:                 {"Local project files could not be read or written", "无法读取或写入本机项目文件", "無法讀取或寫入本機專案檔案", "로컬 프로젝트 파일을 읽거나 쓸 수 없습니다", "ローカルプロジェクトファイルを読み書きできません", "Не удалось прочитать или записать локальные файлы проектов", "Impossible de lire ou écrire les fichiers locaux des projets"},
		xProjectReopen:             {"The desktop app has not resumed running; open it manually", "桌面端尚未恢复运行，请手动打开", "桌面版尚未恢復執行，請手動開啟", "데스크톱 앱이 다시 실행되지 않았습니다; 수동으로 여세요", "デスクトップアプリが再開していません、手動で開いてください", "Приложение не возобновило работу; откройте его вручную", "L’application n’a pas repris ; ouvrez-la manuellement"},
	} {
		extraTexts[key] = texts
	}
}

func desktopSkipReason(l l10n.Localizer, reason string) string {
	switch reason {
	case "cloud_project", "remote_project", "remote_thread":
		return tx(l, xProjectSkipRemote)
	case "missing_thread":
		return tx(l, xProjectSkipThread)
	case "missing_project", "missing_project_mapping", "project_roots_mismatch", "projectless_thread", "unmapped_thread":
		return tx(l, xProjectSkipMapping)
	default:
		return tx(l, xProjectSkipUnknown)
	}
}
