package app

const (
	xCodexImportResult      textKey = "codexImportResult"
	xCodexSavedReloadFailed textKey = "codexSavedReloadFailed"
	xOAuthPort              textKey = "oauthPortInUse"
	xOAuthTimeout           textKey = "oauthTimedOut"
	xOAuthBrowser           textKey = "oauthBrowserFailed"
	xOAuthCallback          textKey = "oauthInvalidCallback"
	xOAuthExchange          textKey = "oauthExchangeFailed"
	xOAuthStore             textKey = "oauthStoreFailed"
	xCodexRestartMac        textKey = "codexRestartMacOnly"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xCodexImportResult:      {"Added %d account(s), updated %d account(s)", "已新增 %d 个账号，已更新 %d 个账号", "已新增 %d 個帳號，已更新 %d 個帳號", "계정 %d개 추가, %d개 업데이트", "アカウントを%d件追加、%d件更新しました", "Добавлено аккаунтов: %d, обновлено: %d", "%d compte(s) ajouté(s), %d compte(s) mis à jour"},
		xCodexSavedReloadFailed: {"Changes were saved, but the account list could not be reloaded", "更改已保存，但账号列表未能重新加载", "變更已儲存，但帳號清單未能重新載入", "변경 사항은 저장되었지만 계정 목록을 다시 불러오지 못했습니다", "変更は保存されましたが、アカウント一覧を再読み込みできませんでした", "Изменения сохранены, но список аккаунтов не удалось обновить", "Les modifications ont été enregistrées, mais la liste des comptes n’a pas pu être rechargée"},
		xCodexRestartMac:        {"Restart is available on macOS", "重启功能仅支持 macOS", "重新啟動功能僅支援 macOS", "재시작은 macOS에서 사용할 수 있습니다", "再起動はmacOSで利用できます", "Перезапуск доступен в macOS", "Le redémarrage est disponible sur macOS"},
		xOAuthPort:              {"Could not listen on sign-in callback port 1455", "无法监听登录回调端口 1455", "無法監聽登入回呼連接埠 1455", "로그인 콜백 포트 1455에서 수신할 수 없습니다", "ログインコールバックポート1455で待ち受けできません", "Не удалось прослушивать порт обратного вызова входа 1455", "Impossible d’écouter le port de retour de connexion 1455"},
		xOAuthTimeout:           {"Sign-in timed out; start a new browser login", "登录已超时，请重新开始浏览器登录", "登入已逾時，請重新開始瀏覽器登入", "로그인 시간이 초과되었습니다; 브라우저 로그인을 다시 시작하세요", "ログインがタイムアウトしました、ブラウザログインをやり直してください", "Время входа истекло; начните вход заново", "La connexion a expiré ; recommencez dans le navigateur"},
		xOAuthBrowser:           {"Could not open the browser; check your default browser", "无法打开浏览器，请检查默认浏览器", "無法開啟瀏覽器，請檢查預設瀏覽器", "브라우저를 열 수 없습니다; 기본 브라우저를 확인하세요", "ブラウザを開けません、既定のブラウザを確認してください", "Не удалось открыть браузер; проверьте браузер по умолчанию", "Impossible d’ouvrir le navigateur ; vérifiez le navigateur par défaut"},
		xOAuthCallback:          {"Invalid sign-in callback; start a new login", "登录回调无效，请重新登录", "登入回呼無效，請重新登入", "로그인 콜백이 유효하지 않습니다; 다시 로그인하세요", "ログインコールバックが無効です、ログインをやり直してください", "Недействительный ответ входа; начните вход заново", "Retour de connexion invalide ; recommencez la connexion"},
		xOAuthExchange:          {"Login authorization exchange failed", "登录授权交换失败", "登入授權交換失敗", "로그인 인증 교환 실패", "ログイン認証の交換に失敗しました", "Ошибка обмена авторизации входа", "Échec de l’échange de l’autorisation de connexion"},
		xOAuthStore:             {"Browser authorization completed, but the account was not added", "浏览器授权已完成，但添加账号未完成", "瀏覽器授權已完成，但新增帳號未完成", "브라우저 인증은 완료되었지만 계정 추가가 완료되지 않았습니다", "ブラウザ認証は完了しましたが、アカウントの追加は完了していません", "Авторизация в браузере завершена, но аккаунт не добавлен", "Autorisation du navigateur terminée, mais le compte n’a pas été ajouté"},
	} {
		extraTexts[key] = texts
	}
}
