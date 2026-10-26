package app

const (
	xAPIAdd              textKey = "apiAccountAdd"
	xAPIEdit             textKey = "apiAccountEdit"
	xAPIBaseURL          textKey = "apiAccountBaseURL"
	xAPIBaseURLOptional  textKey = "apiAccountBaseURLOptional"
	xAPIKey              textKey = "apiAccountKey"
	xAPIKeyKeep          textKey = "apiAccountKeyKeep"
	xAPIModel            textKey = "apiAccountModel"
	xAPIModelOpus        textKey = "apiAccountModelOpus"
	xAPIModelSonnet      textKey = "apiAccountModelSonnet"
	xAPIModelHaiku       textKey = "apiAccountModelHaiku"
	xAPIModelFable       textKey = "apiAccountModelFable"
	xAPIContext1M        textKey = "apiAccountContext1M"
	xAPIContext1MHint    textKey = "apiAccountContext1MHint"
	xAPIModelMapping     textKey = "apiAccountModelMapping"
	xAPIFetchModels      textKey = "apiAccountFetchModels"
	xAPIModelsFound      textKey = "apiAccountModelsFound"
	xAPIModelsEmpty      textKey = "apiAccountModelsEmpty"
	xAPIModelsFailed     textKey = "apiAccountModelsFailed"
	xAPIHintCodex        textKey = "apiAccountHintCodex"
	xAPIHintClaude       textKey = "apiAccountHintClaude"
	xAPIAdded            textKey = "apiAccountAdded"
	xAPISaved            textKey = "apiAccountSaved"
	xAPIBalanceNone      textKey = "apiAccountBalanceNone"
	xAPIBalanceFailed    textKey = "apiAccountBalanceFailed"
	xAPIKeyRejected      textKey = "apiAccountKeyRejected"
	xAPIKeyRequired      textKey = "apiAccountKeyRequired"
	xAPIKeyInvalid       textKey = "apiAccountKeyInvalid"
	xAPIBaseURLInvalid   textKey = "apiAccountBaseURLInvalid"
	xAPIDuplicate        textKey = "apiAccountDuplicate"
	xAPIKeyMissing       textKey = "apiAccountKeyMissing"
	xAPIUnlimited        textKey = "apiAccountUnlimited"
	xAPIBalanceUsed      textKey = "apiAccountBalanceUsed"
	xAPIClaudeSwitchNote textKey = "apiAccountClaudeSwitchNote"
	xAPIApplyTitle       textKey = "apiAccountApplyTitle"
	xAPIApplyMessage     textKey = "apiAccountApplyMessage"
	xAPISaveOnly         textKey = "apiAccountSaveOnly"
	xAPISaveAndApply     textKey = "apiAccountSaveAndApply"
	xAPISavedApplied     textKey = "apiAccountSavedApplied"
	xAPISavedNotApplied  textKey = "apiAccountSavedNotApplied"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xAPIAdd:              {"Add API account", "添加 API 账号", "新增 API 帳號", "API 계정 추가", "APIアカウントを追加", "Добавить API-аккаунт", "Ajouter un compte API"},
		xAPIEdit:             {"Edit API account", "编辑 API 账号", "編輯 API 帳號", "API 계정 편집", "APIアカウントを編集", "Изменить API-аккаунт", "Modifier le compte API"},
		xAPIBaseURL:          {"Base URL", "Base URL", "Base URL", "Base URL", "Base URL", "Base URL", "URL de base"},
		xAPIBaseURLOptional:  {"Base URL (empty uses the official OpenAI API)", "Base URL（留空使用 OpenAI 官方 API）", "Base URL（留空使用 OpenAI 官方 API）", "Base URL (비우면 OpenAI 공식 API 사용)", "Base URL（空欄でOpenAI公式APIを使用）", "Base URL (пусто — официальный API OpenAI)", "URL de base (vide : API OpenAI officielle)"},
		xAPIKey:              {"API key", "API Key", "API Key", "API 키", "APIキー", "API-ключ", "Clé API"},
		xAPIKeyKeep:          {"API key (empty keeps the saved key)", "API Key（留空保留已保存的 Key）", "API Key（留空保留已儲存的 Key）", "API 키 (비우면 저장된 키 유지)", "APIキー（空欄で保存済みのキーを維持）", "API-ключ (пусто — оставить сохранённый)", "Clé API (vide : conserver la clé enregistrée)"},
		xAPIModel:            {"Model (optional)", "模型（可选）", "模型（選填）", "모델 (선택)", "モデル（任意）", "Модель (необязательно)", "Modèle (facultatif)"},
		xAPIModelOpus:        {"Opus model", "Opus 模型", "Opus 模型", "Opus 모델", "Opusモデル", "Модель Opus", "Modèle Opus"},
		xAPIModelSonnet:      {"Sonnet model", "Sonnet 模型", "Sonnet 模型", "Sonnet 모델", "Sonnetモデル", "Модель Sonnet", "Modèle Sonnet"},
		xAPIModelHaiku:       {"Haiku model", "Haiku 模型", "Haiku 模型", "Haiku 모델", "Haikuモデル", "Модель Haiku", "Modèle Haiku"},
		xAPIModelFable:       {"Fable model", "Fable 模型", "Fable 模型", "Fable 모델", "Fableモデル", "Модель Fable", "Modèle Fable"},
		xAPIModelMapping:     {"Model mapping (optional; empty leaves it to Claude Code)", "模型映射（可选，留空由 Claude Code 决定）", "模型映射（選填，留空由 Claude Code 決定）", "모델 매핑 (선택, 비우면 Claude Code가 결정)", "モデルマッピング（任意、空欄はClaude Codeに任せる）", "Сопоставление моделей (необязательно; пусто — на усмотрение Claude Code)", "Correspondance des modèles (facultatif ; vide : laissé à Claude Code)"},
		xAPIFetchModels:      {"Fetch models", "获取模型列表", "取得模型清單", "모델 목록 가져오기", "モデル一覧を取得", "Получить модели", "Récupérer les modèles"},
		xAPIModelsFound:      {"%d models found; pick one in a model field", "已获取 %d 个模型，可在模型输入框中选择", "已取得 %d 個模型，可在模型輸入框中選擇", "모델 %d개를 찾았습니다; 모델 입력란에서 선택하세요", "%d 件のモデルを取得しました、モデル欄で選択できます", "Найдено моделей: %d; выберите в поле модели", "%d modèles trouvés ; choisissez-en un dans un champ de modèle"},
		xAPIModelsEmpty:      {"The site returned no models", "站点没有返回任何模型", "站點沒有傳回任何模型", "사이트가 모델을 반환하지 않았습니다", "サイトからモデルが返されませんでした", "Сайт не вернул ни одной модели", "Le site n’a renvoyé aucun modèle"},
		xAPIModelsFailed:     {"Could not fetch the model list", "获取模型列表失败", "取得模型清單失敗", "모델 목록을 가져올 수 없습니다", "モデル一覧を取得できません", "Не удалось получить список моделей", "Impossible de récupérer la liste des modèles"},
		xAPIHintCodex:        {"Switching to this account writes the key to auth.json and the provider to config.toml; both files are backed up first.", "切换到此账号时会把 Key 写入 auth.json、把服务商写入 config.toml，写入前均会备份。", "切換到此帳號時會把 Key 寫入 auth.json、把服務商寫入 config.toml，寫入前皆會備份。", "이 계정으로 전환하면 키를 auth.json에, 제공자를 config.toml에 기록하며 두 파일 모두 먼저 백업합니다.", "このアカウントに切り替えると、キーを auth.json に、プロバイダーを config.toml に書き込みます。書き込み前に両方をバックアップします。", "При переключении ключ записывается в auth.json, а провайдер — в config.toml; оба файла предварительно копируются.", "Basculer vers ce compte écrit la clé dans auth.json et le fournisseur dans config.toml ; les deux fichiers sont sauvegardés avant."},
		xAPIHintClaude:       {"Switching to this account writes ANTHROPIC_BASE_URL and ANTHROPIC_AUTH_TOKEN to settings.json (backed up first); the subscription login is kept.", "切换到此账号时会把 ANTHROPIC_BASE_URL 和 ANTHROPIC_AUTH_TOKEN 写入 settings.json（写入前备份），订阅登录保持不变。", "切換到此帳號時會把 ANTHROPIC_BASE_URL 與 ANTHROPIC_AUTH_TOKEN 寫入 settings.json（寫入前備份），訂閱登入保持不變。", "이 계정으로 전환하면 ANTHROPIC_BASE_URL과 ANTHROPIC_AUTH_TOKEN을 settings.json에 기록하며(먼저 백업), 구독 로그인은 유지됩니다.", "このアカウントに切り替えると ANTHROPIC_BASE_URL と ANTHROPIC_AUTH_TOKEN を settings.json に書き込みます（事前にバックアップ）。サブスクリプションのログインはそのまま残ります。", "При переключении ANTHROPIC_BASE_URL и ANTHROPIC_AUTH_TOKEN записываются в settings.json (с резервной копией); вход по подписке сохраняется.", "Basculer vers ce compte écrit ANTHROPIC_BASE_URL et ANTHROPIC_AUTH_TOKEN dans settings.json (sauvegardé avant) ; la connexion par abonnement est conservée."},
		xAPIAdded:            {"API account added", "已添加 API 账号", "已新增 API 帳號", "API 계정을 추가했습니다", "APIアカウントを追加しました", "API-аккаунт добавлен", "Compte API ajouté"},
		xAPISaved:            {"API account saved", "已保存 API 账号", "已儲存 API 帳號", "API 계정을 저장했습니다", "APIアカウントを保存しました", "API-аккаунт сохранён", "Compte API enregistré"},
		xAPIBalanceNone:      {"This site does not report a balance", "该站点未提供余额查询", "該站點未提供餘額查詢", "이 사이트는 잔액을 제공하지 않습니다", "このサイトは残高を提供していません", "Сайт не сообщает баланс", "Ce site n’indique pas de solde"},
		xAPIBalanceFailed:    {"Could not read the balance", "余额查询失败", "餘額查詢失敗", "잔액을 읽을 수 없습니다", "残高を取得できません", "Не удалось получить баланс", "Impossible de lire le solde"},
		xAPIKeyRejected:      {"The site rejected the API key", "站点拒绝了该 API Key", "站點拒絕了該 API Key", "사이트가 API 키를 거부했습니다", "サイトがAPIキーを拒否しました", "Сайт отклонил API-ключ", "Le site a refusé la clé API"},
		xAPIKeyRequired:      {"Enter the API key", "请输入 API Key", "請輸入 API Key", "API 키를 입력하세요", "APIキーを入力してください", "Введите API-ключ", "Saisissez la clé API"},
		xAPIKeyInvalid:       {"The API key contains spaces or invalid characters", "API Key 含有空格或无效字符", "API Key 含有空格或無效字元", "API 키에 공백이나 잘못된 문자가 있습니다", "APIキーに空白または無効な文字が含まれています", "API-ключ содержит пробелы или недопустимые символы", "La clé API contient des espaces ou des caractères invalides"},
		xAPIBaseURLInvalid:   {"Enter a valid http(s) Base URL", "请输入有效的 http(s) Base URL", "請輸入有效的 http(s) Base URL", "올바른 http(s) Base URL을 입력하세요", "有効な http(s) の Base URL を入力してください", "Введите корректный Base URL http(s)", "Saisissez une URL de base http(s) valide"},
		xAPIDuplicate:        {"An API account with this Base URL and key already exists", "已存在相同 Base URL 和 Key 的 API 账号", "已存在相同 Base URL 與 Key 的 API 帳號", "같은 Base URL과 키를 가진 API 계정이 이미 있습니다", "同じ Base URL とキーのAPIアカウントが既にあります", "API-аккаунт с таким Base URL и ключом уже есть", "Un compte API avec cette URL de base et cette clé existe déjà"},
		xAPIKeyMissing:       {"The saved API key is missing; edit the account and enter it again", "已保存的 API Key 丢失，请编辑账号重新填写", "已儲存的 API Key 遺失，請編輯帳號重新填寫", "저장된 API 키가 없습니다; 계정을 편집해 다시 입력하세요", "保存済みのAPIキーが見つかりません、アカウントを編集して再入力してください", "Сохранённый API-ключ отсутствует; измените аккаунт и введите его снова", "La clé API enregistrée est introuvable ; modifiez le compte et saisissez-la à nouveau"},
		xAPIUnlimited:        {"Unlimited", "无限额度", "無限額度", "무제한", "無制限", "Без ограничений", "Illimité"},
		xAPIBalanceUsed:      {"Used %s", "已用 %s", "已用 %s", "사용 %s", "使用済み %s", "Использовано %s", "Utilisé %s"},
		xAPIClaudeSwitchNote: {"Claude Code will use this API site; the subscription login stays in place and comes back when you switch to it.", "Claude Code 将改用此 API 站点；订阅登录保持不变，切回订阅账号时恢复。", "Claude Code 將改用此 API 站點；訂閱登入保持不變，切回訂閱帳號時恢復。", "Claude Code가 이 API 사이트를 사용합니다; 구독 로그인은 그대로 유지되며 구독 계정으로 전환하면 돌아옵니다.", "Claude Code はこのAPIサイトを使います。サブスクリプションのログインはそのまま残り、切り替えると戻ります。", "Claude Code будет использовать этот API-сайт; вход по подписке сохраняется и вернётся при переключении на него.", "Claude Code utilisera ce site API ; la connexion par abonnement reste en place et revient quand vous y basculez."},
		xAPIApplyTitle:       {"Update the config files now?", "立即更新配置文件？", "立即更新設定檔？", "지금 설정 파일을 업데이트할까요?", "設定ファイルを今すぐ更新しますか？", "Обновить файлы настроек сейчас?", "Mettre à jour les fichiers de configuration maintenant ?"},
		xAPIApplyMessage:     {"This account is in use by %s. Saving and applying rewrites its config files now (backed up first); saving only keeps them until the next switch. Restart %s afterwards for the change to take effect.", "此账号正被 %s 使用。保存并应用会立即改写其配置文件（写入前备份）；仅保存则保持配置文件不变，直到下次切换。应用后需重新启动 %s 才会生效。", "此帳號正被 %s 使用。儲存並套用會立即改寫其設定檔（寫入前備份）；僅儲存則保持設定檔不變，直到下次切換。套用後需重新啟動 %s 才會生效。", "이 계정은 %s에서 사용 중입니다. 저장 후 적용하면 설정 파일을 바로 다시 씁니다(먼저 백업). 저장만 하면 다음 전환까지 그대로 둡니다. 적용 후 %s를 다시 시작해야 반영됩니다.", "このアカウントは %s で使用中です。保存して適用すると設定ファイルをすぐに書き換えます（事前にバックアップ）。保存のみの場合は次の切り替えまで変更しません。適用後、%s を再起動すると反映されます。", "Этот аккаунт используется в %s. «Сохранить и применить» сразу перезапишет файлы настроек (с резервной копией); «Только сохранить» оставит их до следующего переключения. После применения перезапустите %s.", "Ce compte est utilisé par %s. Enregistrer et appliquer réécrit ses fichiers de configuration maintenant (sauvegardés avant) ; enregistrer seulement les laisse jusqu’au prochain basculement. Redémarrez ensuite %s pour que le changement prenne effet."},
		xAPISaveOnly:         {"Save only", "仅保存", "僅儲存", "저장만", "保存のみ", "Только сохранить", "Enregistrer seulement"},
		xAPISaveAndApply:     {"Save and apply", "保存并应用", "儲存並套用", "저장 후 적용", "保存して適用", "Сохранить и применить", "Enregistrer et appliquer"},
		xAPISavedApplied:     {"API account saved and the config files updated. Restart %s to use the new settings.", "已保存 API 账号并更新配置文件，请重新启动 %s 以使用新设置。", "已儲存 API 帳號並更新設定檔，請重新啟動 %s 以使用新設定。", "API 계정을 저장하고 설정 파일을 업데이트했습니다. 새 설정을 사용하려면 %s를 다시 시작하세요.", "APIアカウントを保存し、設定ファイルを更新しました。新しい設定を使うには %s を再起動してください。", "API-аккаунт сохранён, файлы настроек обновлены. Перезапустите %s, чтобы применить новые настройки.", "Compte API enregistré et fichiers de configuration mis à jour. Redémarrez %s pour utiliser les nouveaux réglages."},
		xAPISavedNotApplied:  {"API account saved. The config files are unchanged and take the new settings at the next switch to this account.", "已保存 API 账号。配置文件未改动，下次切换到此账号时才会写入新设置。", "已儲存 API 帳號。設定檔未改動，下次切換到此帳號時才會寫入新設定。", "API 계정을 저장했습니다. 설정 파일은 바뀌지 않았으며 다음에 이 계정으로 전환할 때 새 설정이 기록됩니다.", "APIアカウントを保存しました。設定ファイルは変更されず、次にこのアカウントへ切り替えたときに新しい設定が書き込まれます。", "API-аккаунт сохранён. Файлы настроек не изменены; новые настройки будут записаны при следующем переключении на этот аккаунт.", "Compte API enregistré. Les fichiers de configuration sont inchangés et recevront les nouveaux réglages au prochain basculement vers ce compte."},
		xAPIContext1M:        {"Use 1M context", "启用 1M 上下文", "啟用 1M 上下文", "1M 컨텍스트 사용", "1Mコンテキストを使用", "Использовать контекст 1M", "Utiliser le contexte 1M"},
		xAPIContext1MHint: {
			"Adds [1m] to non-empty default, Opus and Sonnet models; the model and relay must support 1M, Haiku and Fable are unchanged; turning this off restores your entries and keeps any manually entered [1m] suffix",
			"为已填写的默认、Opus 和 Sonnet 模型追加 [1m]，模型与中转站均须支持 1M，Haiku 和 Fable 不变；关闭后恢复所填模型，手动填写的 [1m] 后缀仍会保留",
			"為已填寫的預設、Opus 與 Sonnet 模型加上 [1m]，模型與中轉站均須支援 1M，Haiku 與 Fable 不變；關閉後還原所填模型，手動填寫的 [1m] 後綴仍會保留",
			"입력된 기본·Opus·Sonnet 모델에 [1m]을 추가합니다; 모델과 중계 서버가 1M을 지원해야 하며 Haiku와 Fable은 그대로입니다; 끄면 입력값으로 복원하고 직접 입력한 [1m] 접미사는 유지합니다",
			"入力済みの既定・Opus・Sonnetモデルに [1m] を追加します、モデルと中継サイトの1M対応が必要です、HaikuとFableは変更しません、オフにすると入力したモデルに戻り、手入力の [1m] は残ります",
			"Добавляет [1m] к заполненным моделям по умолчанию, Opus и Sonnet; модель и прокси должны поддерживать 1M, Haiku и Fable не меняются; отключение возвращает введённые значения, сохраняя вручную добавленный [1m]",
			"Ajoute [1m] aux modèles par défaut, Opus et Sonnet renseignés ; le modèle et le relais doivent prendre en charge 1M, Haiku et Fable restent inchangés ; désactiver rétablit vos saisies en conservant tout suffixe [1m] ajouté manuellement",
		},
	} {
		extraTexts[key] = texts
	}
}
