package app

import (
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/l10n"
)

const (
	xClaudeOAuthRollback      textKey = "claudeOAuthRollback"
	xClaudeOAuthManualInvalid textKey = "claudeOAuthManualInvalid"
	xClaudeCallbackReceived   textKey = "claudeCallbackReceived"
	xClaudeCallbackInvalid    textKey = "claudeCallbackInvalid"
	xClaudeCallbackRepeated   textKey = "claudeCallbackRepeated"
	xClaudeOAuthChanged       textKey = "claudeOAuthChanged"
	xClaudeOAuthPort          textKey = "claudeOAuthPort"
	xClaudeOAuthVerify        textKey = "claudeOAuthVerify"
	xClaudeOAuthSaving        textKey = "claudeOAuthSaving"
	xClaudeOAuthQuotaLoading  textKey = "claudeOAuthQuotaLoading"
	xClaudeOAuthAdded         textKey = "claudeOAuthAdded"
	xClaudeOAuthQuotaFailed   textKey = "claudeOAuthQuotaFailed"
	xClaudeOAuthExists        textKey = "claudeOAuthExists"
	xClaudeOAuthIdentity      textKey = "claudeOAuthIdentity"
	xClaudeOAuthDenied        textKey = "claudeOAuthDenied"
	xClaudeOAuthHint          textKey = "claudeOAuthHint"
	xClaudeOAuthManual        textKey = "claudeOAuthManual"
	xClaudeOAuthReopen        textKey = "claudeOAuthReopen"
	xClaudeOAuthSubmit        textKey = "claudeOAuthSubmit"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xClaudeOAuthRollback:      {"Account save failed, and restoring its account-library credentials also failed; import this account again", "账号保存失败，账号库凭据恢复也失败，请重新导入该账号", "帳號儲存失敗，帳號庫憑證還原也失敗，請重新匯入此帳號", "계정 저장에 실패했고 계정 라이브러리 자격 증명 복원도 실패했습니다; 이 계정을 다시 가져오세요", "アカウントの保存とアカウントライブラリの認証情報の復元に失敗しました、このアカウントを再度読み込んでください", "Не удалось сохранить аккаунт и восстановить его данные входа в библиотеке; импортируйте аккаунт заново", "L’enregistrement du compte et la restauration de ses identifiants dans la bibliothèque ont échoué ; importez à nouveau ce compte"},
		xClaudeOAuthManualInvalid: {"Invalid callback; check the full callback URL or code#state and try again", "回调无效，请检查完整回调 URL 或 code#state 后重试", "回呼無效，請檢查完整回呼 URL 或 code#state 後重試", "콜백이 유효하지 않습니다; 전체 콜백 URL 또는 code#state를 확인하고 다시 시도하세요", "コールバックが無効です、完全なコールバックURLまたはcode#stateを確認して再試行してください", "Недействительный ответ; проверьте полный URL ответа или code#state и повторите", "Retour invalide ; vérifiez l’URL de retour complète ou code#state et réessayez"},
		xClaudeCallbackReceived:   {"Authorization received; return to Quosparr to confirm the result", "已收到授权，请返回 Quosparr 查看处理结果", "已收到授權，請返回 Quosparr 查看處理結果", "인증을 받았습니다; Quosparr로 돌아가 결과를 확인하세요", "認証を受信しました、Quosparrに戻って結果を確認してください", "Авторизация получена; вернитесь в Quosparr для проверки результата", "Autorisation reçue ; retournez dans Quosparr pour vérifier le résultat"},
		xClaudeCallbackInvalid:    {"Invalid authorization callback; return to Quosparr", "授权回调无效，请返回 Quosparr", "授權回呼無效，請返回 Quosparr", "인증 콜백이 유효하지 않습니다; Quosparr로 돌아가세요", "認証コールバックが無効です、Quosparrに戻ってください", "Недействительный ответ авторизации; вернитесь в Quosparr", "Retour d’autorisation invalide ; retournez dans Quosparr"},
		xClaudeCallbackRepeated:   {"Authorization was already received; return to Quosparr to confirm the result", "已接收过授权，请返回 Quosparr 查看处理结果", "已接收過授權，請返回 Quosparr 查看處理結果", "인증이 이미 접수되었습니다; Quosparr로 돌아가 결과를 확인하세요", "認証は受信済みです、Quosparrに戻って結果を確認してください", "Авторизация уже получена; вернитесь в Quosparr для проверки результата", "Autorisation déjà reçue ; retournez dans Quosparr pour vérifier le résultat"},
		xClaudeOAuthPort:          {"Sign-in port 54545 is in use; close the other login and try again", "登录端口 54545 已被占用，请关闭其他登录后重试", "登入連接埠 54545 已被佔用，請關閉其他登入後重試", "로그인 포트 54545가 사용 중입니다; 다른 로그인을 닫고 다시 시도하세요", "ログインポート54545は使用中です、他のログインを閉じて再試行してください", "Порт входа 54545 занят; закройте другой вход и повторите", "Le port de connexion 54545 est occupé ; fermez l’autre connexion et réessayez"},
		xClaudeOAuthVerify:        {"Verifying authorization and account identity…", "正在验证授权与账号身份…", "正在驗證授權與帳號身分…", "인증 및 계정 신원 확인 중…", "認証とアカウント情報を確認中…", "Проверка авторизации и личности аккаунта…", "Vérification de l’autorisation et de l’identité du compte…"},
		xClaudeOAuthSaving:        {"Saving account…", "正在保存账号…", "正在儲存帳號…", "계정 저장 중…", "アカウントを保存中…", "Сохранение аккаунта…", "Enregistrement du compte…"},
		xClaudeOAuthQuotaLoading:  {"Account saved; reading usage…", "账号已保存，正在读取额度…", "帳號已儲存，正在讀取用量…", "계정 저장 완료; 사용량 읽는 중…", "アカウント保存済み、使用量を取得中…", "Аккаунт сохранён; загрузка использования…", "Compte enregistré ; lecture de l’utilisation…"},
		xClaudeOAuthAdded:         {"Account added; switch to it from the account list when ready", "账号已添加，可在账号列表中主动切换", "帳號已新增，可在帳號清單中主動切換", "계정이 추가되었습니다; 계정 목록에서 전환할 수 있습니다", "アカウントを追加しました、アカウント一覧から切り替えられます", "Аккаунт добавлен; переключиться на него можно в списке аккаунтов", "Compte ajouté ; vous pouvez l’activer depuis la liste des comptes"},
		xClaudeOAuthQuotaFailed:   {"Account added, but usage could not be read; switch to it from the account list or retry usage refresh", "账号已添加，但额度读取失败，可在账号列表切换或重试刷新额度", "帳號已新增，但用量讀取失敗，可在帳號清單切換或重試重新整理用量", "계정은 추가되었지만 사용량을 읽지 못했습니다; 목록에서 전환하거나 사용량 새로고침을 다시 시도하세요", "アカウントは追加されましたが使用量を取得できません、一覧で切り替えるか使用量の更新を再試行してください", "Аккаунт добавлен, но использование не загружено; переключитесь в списке или повторите обновление", "Compte ajouté, mais l’utilisation n’a pas pu être lue ; activez-le dans la liste ou réessayez l’actualisation"},
		xClaudeOAuthExists:        {"This subscription login is already saved on this computer; its existing credentials were kept", "本机已保存该订阅账号的登录，已保留现有凭据", "本機已儲存此訂閱帳號的登入，已保留現有憑證", "이 구독 계정의 로그인이 이 컴퓨터에 이미 저장되어 있습니다; 기존 자격 증명을 유지했습니다", "このサブスクリプションのログインはこの端末に保存済みです、既存の認証情報を保持しました", "Вход в этот аккаунт подписки уже сохранён на компьютере; существующие данные входа сохранены", "La connexion à cet abonnement est déjà enregistrée sur cet ordinateur ; les identifiants existants ont été conservés"},
		xClaudeOAuthChanged:       {"The local Claude Code login changed while adding the account; start a new login", "添加期间本机 Claude Code 登录发生变化，请重新开始登录", "新增期間本機 Claude Code 登入發生變更，請重新開始登入", "계정을 추가하는 동안 로컬 Claude Code 로그인이 변경되었습니다; 다시 로그인하세요", "追加中にこの端末のClaude Codeログインが変更されました、ログインをやり直してください", "Локальный вход Claude Code изменился при добавлении аккаунта; начните вход заново", "La connexion Claude Code locale a changé pendant l’ajout ; recommencez la connexion"},
		xClaudeOAuthIdentity:      {"Could not verify the Claude account identity; check your network and start a new login", "无法确认 Claude 账号身份，请检查网络后重新登录", "無法確認 Claude 帳號身分，請檢查網路後重新登入", "Claude 계정 신원을 확인하지 못했습니다; 네트워크를 확인하고 다시 로그인하세요", "Claudeアカウントを確認できません、ネットワークを確認して再ログインしてください", "Не удалось подтвердить аккаунт Claude; проверьте сеть и войдите заново", "Impossible de vérifier l’identité du compte Claude ; vérifiez le réseau et reconnectez-vous"},
		xClaudeOAuthDenied:        {"Authorization was declined; start a new login when ready", "授权已被拒绝，可重新开始登录", "授權已被拒絕，可重新開始登入", "인증이 거부되었습니다; 다시 로그인할 수 있습니다", "認証が拒否されました、ログインをやり直せます", "Авторизация отклонена; начните вход заново", "Autorisation refusée ; recommencez la connexion"},
		xClaudeOAuthHint:          {"Authorize in your default browser within 5 minutes; this only adds the account to Quosparr, without switching Claude Code", "请在 5 分钟内使用默认浏览器完成授权，仅添加到 Quosparr，不切换 Claude Code 当前账号", "請在 5 分鐘內使用預設瀏覽器完成授權，僅新增至 Quosparr，不切換 Claude Code 目前帳號", "5분 안에 기본 브라우저에서 인증하세요; Quosparr에만 추가하며 Claude Code 계정은 전환하지 않습니다", "5分以内に既定のブラウザで認証してください、Quosparrへの追加のみでClaude Codeのアカウントは切り替わりません", "Подтвердите вход в браузере за 5 минут; аккаунт добавится в Quosparr без переключения Claude Code", "Autorisez sous 5 minutes dans votre navigateur par défaut ; le compte sera ajouté à Quosparr sans changer le compte Claude Code"},
		xClaudeOAuthManual:        {"Paste the full callback URL or code#state", "粘贴完整回调 URL 或 code#state", "貼上完整回呼 URL 或 code#state", "전체 콜백 URL 또는 code#state 붙여넣기", "完全なコールバックURLまたはcode#stateを貼り付け", "Вставьте полный URL ответа или code#state", "Collez l’URL de retour complète ou code#state"},
		xClaudeOAuthReopen:        {"Reopen authorization page", "重新打开授权页", "重新開啟授權頁", "인증 페이지 다시 열기", "認証ページを再度開く", "Открыть страницу авторизации", "Rouvrir la page d’autorisation"},
		xClaudeOAuthSubmit:        {"Submit callback", "提交回调", "提交回呼", "콜백 제출", "コールバックを送信", "Отправить ответ", "Envoyer le retour"},
	} {
		extraTexts[key] = texts
	}
}

func claudeCallbackText(l l10n.Localizer) claude.LoginCallbackText {
	return claude.LoginCallbackText{Received: tx(l, xClaudeCallbackReceived), Invalid: tx(l, xClaudeCallbackInvalid), AlreadyReceived: tx(l, xClaudeCallbackRepeated)}
}
