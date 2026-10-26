package app

import (
	"slices"

	"github.com/licoy/quosparr/internal/l10n"
)

// textKey names a string that the Swift tables do not have. These are the
// strings the Go interface needs on top of internal/l10n (which is generated
// from the Swift tables and must not be edited by hand). Each has all seven
// languages; TestExtraTextsAreComplete enforces it. When they move into the
// Swift tables they can be deleted here.
type textKey string

const (
	xAccountCount                 textKey = "accountCount"
	xAccountCountOne              textKey = "accountCountOne"
	xPrivacyDisplaySection        textKey = "privacyDisplaySection"
	xPrivacyDisplayDescription    textKey = "privacyDisplayDescription"
	xProvider                     textKey = "provider"
	xProviderDescription          textKey = "providerDescription"
	xPanelHeight                  textKey = "panelHeight"
	xPanelLoading                 textKey = "panelLoading"
	xPanelHeightHint              textKey = "panelHeightDescription"
	xDisablePanelAnimation        textKey = "disableMainPanelAnimation"
	xDisablePanelAnimationHint    textKey = "disableMainPanelAnimationDescription"
	xMacOnly                      textKey = "settingMacOnly"
	xWidgetMac14                  textKey = "settingWidgetMac14"
	xSettingsUnreadable           textKey = "settingsUnreadableTitle"
	xSettingsReadOnly             textKey = "settingsReadOnlyNote"
	xProxyBlocked                 textKey = "proxyBlockedNote"
	xSettingsSaveFailed           textKey = "settingsSaveFailed"
	xAccountsComingSoon           textKey = "accountsComingSoon"
	xTrayUnavailable              textKey = "trayUnavailable"
	xLinuxDesktopDependencies     textKey = "linuxDesktopDependencies"
	xLinuxReleaseUnreadable       textKey = "linuxReleaseUnreadable"
	xPanelPlaceholder             textKey = "panelPlaceholder"
	xNotAvailableYet              textKey = "notAvailableYet"
	xProxyLocalSaveWarning        textKey = "proxyPasswordNotStored"
	xThemeColor                   textKey = "themeColor"
	xColorBrown                   textKey = "themeColorBrown"
	xColorBlue                    textKey = "themeColorBlue"
	xColorPurple                  textKey = "themeColorPurple"
	xColorPink                    textKey = "themeColorPink"
	xColorRed                     textKey = "themeColorRed"
	xColorOrange                  textKey = "themeColorOrange"
	xColorYellow                  textKey = "themeColorYellow"
	xColorGreen                   textKey = "themeColorGreen"
	xColorGraphite                textKey = "themeColorGraphite"
	xColorCustom                  textKey = "themeColorCustom"
	xFloatingSection              textKey = "floatingWindowSection"
	xFloatingEnabled              textKey = "floatingWindow"
	xFloatingEnabledHint          textKey = "floatingWindowDescription"
	xFloatingStyle                textKey = "floatingWindowStyle"
	xFloatingStyleCapsule         textKey = "floatingWindowStyleCapsule"
	xFloatingStyleCard            textKey = "floatingWindowStyleCard"
	xFloatingStyleRings           textKey = "floatingWindowStyleRings"
	xFloatingStyleBattery         textKey = "floatingWindowStyleBattery"
	xFloatingProviders            textKey = "floatingWindowProviders"
	xFloatingOpacity              textKey = "floatingWindowOpacity"
	xFloatingLocked               textKey = "floatingWindowLocked"
	xFloatingLockedHint           textKey = "floatingWindowLockedDescription"
	xFloatingFullScreen           textKey = "floatingWindowOverFullScreen"
	xFloatingFullScreenHint       textKey = "floatingWindowOverFullScreenDescription"
	xFloatingHide                 textKey = "floatingWindowHide"
	xFloatingWayland              textKey = "floatingWindowWayland"
	xFloatingAppFilter            textKey = "floatingWindowAppFilter"
	xFloatingAppFilterHint        textKey = "floatingWindowAppFilterDescription"
	xFloatingAppFilterOff         textKey = "floatingWindowAppFilterOff"
	xFloatingAppFilterAllow       textKey = "floatingWindowAppFilterAllow"
	xFloatingAppFilterBlock       textKey = "floatingWindowAppFilterBlock"
	xFloatingAppFilterAllowHint   textKey = "floatingWindowAppFilterAllowDescription"
	xFloatingAppFilterBlockHint   textKey = "floatingWindowAppFilterBlockDescription"
	xFloatingAppFilterUnsupported textKey = "floatingWindowAppFilterUnsupported"
	xFloatingApps                 textKey = "floatingWindowApps"
	xFloatingAppsNone             textKey = "floatingWindowAppsNone"
	xFloatingAppsCount            textKey = "floatingWindowAppsCount"
	xFloatingChooseApps           textKey = "floatingWindowChooseApps"
	xFloatingAppsTitle            textKey = "floatingWindowAppsTitle"
	xFloatingAppsSearch           textKey = "floatingWindowAppsSearch"
	xFloatingAppsAddFile          textKey = "floatingWindowAppsAddFile"
	xFloatingAppsEmpty            textKey = "floatingWindowAppsEmpty"
	xFloatingAppsLoadFailed       textKey = "floatingWindowAppsLoadFailed"
	xFloatingAppsNotApp           textKey = "floatingWindowAppsNotApp"
	xFloatingOffList              textKey = "floatingWindowOffList"
	xFloatingOffListLower         textKey = "floatingWindowOffListLower"
	xFloatingOffListHide          textKey = "floatingWindowOffListHide"
	xFloatingOffListLowerHint     textKey = "floatingWindowOffListLowerDescription"
	xFloatingOffListHideHint      textKey = "floatingWindowOffListHideDescription"
	xDisplayFont                  textKey = "displayFont"
	xDisplayFontHint              textKey = "displayFontDescription"
	xDisplayFontDefault           textKey = "displayFontDefault"
	xPanelProviders               textKey = "mainPanelProviders"
	xPanelProvidersHint           textKey = "mainPanelProvidersDescription"
	xPanelProvidersAll            textKey = "mainPanelProvidersAll"
	xResetTimeFormat              textKey = "quotaResetTimeFormat"
	xResetTimeFormatHint          textKey = "quotaResetTimeFormatDescription"
	xShowCreditsBalance           textKey = "showCreditsBalance"
	xShowCreditsBalanceHint       textKey = "showCreditsBalanceDescription"
	xResetTimeRelative            textKey = "quotaResetTimeRelative"
	xResetTimeExact               textKey = "quotaResetTimeExact"
	xAccountResetsIn              textKey = "accountQuotaResetsIn"
	xAccountResetsAt              textKey = "accountQuotaResetsAt"
	xProxyTestURL                 textKey = "networkProxyTestURL"
	xProxyTestURLInvalid          textKey = "networkProxyTestURLInvalid"
	xSpeedTitle                   textKey = "speedAnalysis"
	xSpeedModuleDescription       textKey = "speedAnalysisDescription"
	xSpeedInfoTTFT                textKey = "speedAnalysisInfoTTFT"
	xSpeedInfoNoTTFT              textKey = "speedAnalysisInfoNoTTFT"
	xSpeedInfoOutput              textKey = "speedAnalysisInfoOutput"
	xSpeedInfoSource              textKey = "speedAnalysisInfoSource"
	xSpeedTTFT                    textKey = "speedAnalysisTTFT"
	xSpeedOutput                  textKey = "speedAnalysisOutput"
	xSpeedModel                   textKey = "speedAnalysisModel"
	xSpeedCalls                   textKey = "speedAnalysisCalls"
	xSpeedByModel                 textKey = "speedAnalysisByModel"
	xSpeedBasis                   textKey = "speedAnalysisBasis"
	xSpeedBasisOne                textKey = "speedAnalysisBasisOne"
	xSpeedLatest                  textKey = "speedAnalysisLatest"
	xSpeedEmpty                   textKey = "speedAnalysisEmpty"
	xSpeedRange                   textKey = "speedAnalysisRange"
	xSpeedTierFast                textKey = "speedAnalysisTierFast"
	xSpeedTierStandard            textKey = "speedAnalysisTierStandard"
	xSpeedInfoTier                textKey = "speedAnalysisInfoTier"
	xLegacyRunningTitle           textKey = "legacyRunningTitle"
	xLegacyRunningDetail          textKey = "legacyRunningDetail"
	xLegacyRunningManual          textKey = "legacyRunningManual"
	xLegacyQuitAndContinue        textKey = "legacyQuitAndContinue"
	xLegacyQuitFailed             textKey = "legacyQuitFailed"
)

// extraTexts is indexed by textKey, then by position in l10n.Languages():
// English, Simplified Chinese, Traditional Chinese, Korean, Japanese,
// Russian, French.
var extraTexts = map[textKey][7]string{
	xPanelLoading: {"Loading…", "加载中…", "載入中…", "불러오는 중…", "読み込み中…", "Загрузка…", "Chargement…"},
	xDisablePanelAnimation: {
		"Disable main panel animation", "关闭主弹窗动画效果", "關閉主彈窗動畫效果", "메인 패널 애니메이션 끄기", "メインポップアップのアニメーションをオフ", "Отключить анимацию главной панели", "Désactiver l’animation du panneau principal",
	},
	xDisablePanelAnimationHint: {
		"Turn this on to open the main panel faster; takes effect the next time it opens",
		"开启此项可更快打开主弹窗，下次打开时生效",
		"開啟此項可更快開啟主彈窗，下次開啟時生效",
		"켜면 메인 패널이 더 빠르게 열리며, 다음에 열 때 적용됩니다",
		"オンにするとメインポップアップをすばやく開けます。次回開くときに反映されます",
		"Включите, чтобы главная панель открывалась быстрее; изменение вступит в силу при следующем открытии",
		"Activez cette option pour ouvrir le panneau principal plus vite ; elle prendra effet à la prochaine ouverture",
	},
	xThemeColor: {
		"Theme color", "主题颜色", "主題顏色", "테마 색상", "テーマカラー", "Цвет темы", "Couleur du thème",
	},
	xColorBrown: {
		"Brown (default)", "棕色（默认）", "棕色（預設）", "갈색(기본)", "ブラウン（デフォルト）", "Коричневый (по умолчанию)", "Brun (par défaut)",
	},
	xColorBlue:     {"Blue", "蓝色", "藍色", "파란색", "ブルー", "Синий", "Bleu"},
	xColorPurple:   {"Purple", "紫色", "紫色", "보라색", "パープル", "Фиолетовый", "Violet"},
	xColorPink:     {"Pink", "粉色", "粉紅色", "분홍색", "ピンク", "Розовый", "Rose"},
	xColorRed:      {"Red", "红色", "紅色", "빨간색", "レッド", "Красный", "Rouge"},
	xColorOrange:   {"Orange", "橙色", "橙色", "주황색", "オレンジ", "Оранжевый", "Orange"},
	xColorYellow:   {"Yellow", "黄色", "黃色", "노란색", "イエロー", "Жёлтый", "Jaune"},
	xColorGreen:    {"Green", "绿色", "綠色", "초록색", "グリーン", "Зелёный", "Vert"},
	xColorGraphite: {"Graphite", "石墨色", "石墨色", "그래파이트", "グラファイト", "Графит", "Graphite"},
	xColorCustom:   {"Custom", "自定义", "自訂", "사용자 지정", "カスタム", "Свой цвет", "Personnalisée"},
	xFloatingSection: {
		"Floating window", "桌面悬浮窗", "桌面浮動視窗", "플로팅 창", "フローティングウインドウ", "Плавающее окно", "Fenêtre flottante",
	},
	xFloatingEnabled: {
		"Show floating window", "显示悬浮窗", "顯示浮動視窗", "플로팅 창 표시", "フローティングウインドウを表示", "Показывать плавающее окно", "Afficher la fenêtre flottante",
	},
	xFloatingEnabledHint: {
		"Keeps the remaining quota on top of other windows. Click to open the main panel, drag to move it, right-click for more",
		"置顶显示剩余额度；单击打开主面板，拖动可移动，右键查看更多选项",
		"置頂顯示剩餘額度；按一下開啟主面板，拖曳可移動，按右鍵查看更多選項",
		"남은 한도를 다른 창 위에 표시합니다. 클릭하면 메인 패널이 열리고, 드래그로 이동하며, 오른쪽 클릭으로 더 많은 옵션을 봅니다",
		"残りの使用枠を最前面に表示します。クリックでメインパネルを開き、ドラッグで移動、右クリックでその他の操作",
		"Показывает остаток лимита поверх других окон. Нажмите, чтобы открыть главную панель, перетащите, чтобы переместить, правый щелчок — другие действия",
		"Garde le quota restant au-dessus des autres fenêtres. Cliquez pour ouvrir le panneau principal, faites glisser pour la déplacer, clic droit pour plus d’options",
	},
	xFloatingStyle: {
		"Floating window style", "悬浮窗样式", "浮動視窗樣式", "플로팅 창 스타일", "フローティングウインドウのスタイル", "Стиль плавающего окна", "Style de la fenêtre flottante",
	},
	xFloatingStyleCapsule: {
		"Capsule", "胶囊", "膠囊", "캡슐", "カプセル", "Капсула", "Capsule",
	},
	xFloatingStyleCard: {
		"Card", "卡片", "卡片", "카드", "カード", "Карточка", "Carte",
	},
	xFloatingStyleRings: {
		"Rings", "圆环", "圓環", "링", "リング", "Кольца", "Anneaux",
	},
	xFloatingStyleBattery: {
		"Battery", "电池", "電池", "배터리", "バッテリー", "Батарея", "Batterie",
	},
	xFloatingProviders: {
		"Floating window providers", "悬浮窗服务商", "浮動視窗服務商", "플로팅 창 공급자", "フローティングウインドウのプロバイダー", "Провайдеры в плавающем окне", "Fournisseurs de la fenêtre flottante",
	},
	xFloatingOpacity: {
		"Opacity", "不透明度", "不透明度", "불투명도", "不透明度", "Непрозрачность", "Opacité",
	},
	xFloatingLocked: {
		"Lock position", "锁定位置", "鎖定位置", "위치 고정", "位置を固定", "Закрепить положение", "Verrouiller la position",
	},
	xFloatingLockedHint: {
		"Dragging no longer moves the floating window",
		"锁定后拖动不会移动悬浮窗",
		"鎖定後拖曳不會移動浮動視窗",
		"고정하면 드래그해도 플로팅 창이 움직이지 않습니다",
		"固定するとドラッグしてもフローティングウインドウは動きません",
		"Перетаскивание больше не перемещает плавающее окно",
		"Le glisser ne déplace plus la fenêtre flottante",
	},
	xFloatingFullScreen: {
		"Show over full-screen apps", "全屏应用上方也显示", "全螢幕 App 上方也顯示", "전체 화면 앱 위에도 표시", "フルスクリーンのアプリの上にも表示", "Показывать поверх полноэкранных приложений", "Afficher au-dessus des apps en plein écran",
	},
	xFloatingFullScreenHint: {
		"When off, the floating window steps aside while another app is full screen",
		"关闭时，其他应用全屏期间悬浮窗自动让开",
		"關閉時，其他 App 全螢幕期間浮動視窗會自動讓開",
		"끄면 다른 앱이 전체 화면일 때 플로팅 창이 비켜 줍니다",
		"オフのときは、ほかのアプリがフルスクリーンの間フローティングウインドウを隠します",
		"Если выключено, плавающее окно скрывается, пока другое приложение в полноэкранном режиме",
		"Désactivé, la fenêtre flottante s’efface tant qu’une autre app est en plein écran",
	},
	xFloatingHide: {
		"Hide floating window", "隐藏悬浮窗", "隱藏浮動視窗", "플로팅 창 숨기기", "フローティングウインドウを隠す", "Скрыть плавающее окно", "Masquer la fenêtre flottante",
	},
	xFloatingWayland: {
		"On Wayland, the desktop may not let apps stay on top or choose where their windows go",
		"Wayland 桌面可能不允许应用置顶或指定窗口位置",
		"Wayland 桌面可能不允許 App 置頂或指定視窗位置",
		"Wayland 데스크톱에서는 앱이 항상 위에 표시되거나 창 위치를 지정하지 못할 수 있습니다",
		"Wayland のデスクトップでは、アプリの最前面表示やウインドウ位置の指定ができない場合があります",
		"В Wayland рабочий стол может не позволять приложениям оставаться поверх других окон или выбирать их положение",
		"Sous Wayland, le bureau peut empêcher les apps de rester au premier plan ou de choisir la position de leurs fenêtres",
	},
	xFloatingAppFilter: {
		"Show by app in front", "按前台应用显示", "依前景 App 顯示", "앞에 있는 앱에 따라 표시", "前面のアプリに応じて表示", "Показ по активному приложению", "Afficher selon l’app au premier plan",
	},
	xFloatingAppFilterHint: {
		"Show or hide the floating window by the app in front",
		"根据前台应用显示或隐藏悬浮窗",
		"依前景 App 顯示或隱藏浮動視窗",
		"앞에 있는 앱에 따라 플로팅 창을 표시하거나 숨깁니다",
		"前面にあるアプリに応じてフローティングウインドウを表示または非表示にします",
		"Показывать или скрывать плавающее окно в зависимости от активного приложения",
		"Affiche ou masque la fenêtre flottante selon l’app au premier plan",
	},
	xFloatingAppFilterOff:   {"Off", "关闭", "關閉", "끔", "オフ", "Выкл.", "Désactivé"},
	xFloatingAppFilterAllow: {"Allow list", "白名单", "白名單", "허용 목록", "許可リスト", "Белый список", "Liste d’autorisation"},
	xFloatingAppFilterBlock: {"Block list", "黑名单", "黑名單", "차단 목록", "ブロックリスト", "Чёрный список", "Liste de blocage"},
	xFloatingAppFilterAllowHint: {
		"Stays on top only while a listed app is in front, and always on the desktop",
		"仅名单中的应用位于前台时置顶显示，桌面上始终显示",
		"僅名單中的 App 位於前景時置頂顯示，桌面上一律顯示",
		"목록에 있는 앱이 앞에 있을 때만 맨 위에 표시하며, 데스크톱에서는 항상 표시합니다",
		"リストのアプリが前面にあるときだけ最前面に表示し、デスクトップでは常に表示します",
		"Остаётся поверх окон, только пока активно приложение из списка, и всегда — на рабочем столе",
		"Reste au-dessus des fenêtres seulement quand une app de la liste est au premier plan, et toujours sur le bureau",
	},
	xFloatingAppFilterBlockHint: {
		"Hides while a listed app is in front, and always shows on the desktop",
		"名单中的应用位于前台时隐藏，桌面上始终显示",
		"名單中的 App 位於前景時隱藏，桌面上一律顯示",
		"목록에 있는 앱이 앞에 있으면 숨기며, 데스크톱에서는 항상 표시합니다",
		"リストのアプリが前面にあるときは隠し、デスクトップでは常に表示します",
		"Скрывается, пока активно приложение из списка, и всегда показывается на рабочем столе",
		"Se masque quand une app de la liste est au premier plan, et s’affiche toujours sur le bureau",
	},
	xFloatingAppFilterUnsupported: {
		"This desktop does not tell apps which app is in front (as on Wayland), so this is unavailable",
		"当前桌面（如 Wayland）不向应用提供前台应用信息，此功能不可用",
		"目前的桌面（如 Wayland）不向 App 提供前景 App 資訊，此功能無法使用",
		"이 데스크톱(Wayland 등)은 앞에 있는 앱을 알려 주지 않아 사용할 수 없습니다",
		"このデスクトップ（Wayland など）は前面のアプリを知らせないため、利用できません",
		"Этот рабочий стол (например, Wayland) не сообщает приложениям, какое приложение активно, поэтому функция недоступна",
		"Ce bureau (comme Wayland) n’indique pas aux apps laquelle est au premier plan, cette option n’est donc pas disponible",
	},
	xFloatingApps:     {"Apps", "应用名单", "App 名單", "앱 목록", "アプリのリスト", "Приложения", "Apps"},
	xFloatingAppsNone: {"No apps chosen", "尚未选择应用", "尚未選擇 App", "선택한 앱 없음", "アプリが選択されていません", "Приложения не выбраны", "Aucune app choisie"},
	xFloatingAppsCount: {
		"%@ apps", "已选 %@ 个应用", "已選 %@ 個 App", "앱 %@개", "%@ 個のアプリ", "Приложений: %@", "%@ apps",
	},
	xFloatingChooseApps: {"Choose Apps…", "选择应用…", "選擇 App…", "앱 선택…", "アプリを選択…", "Выбрать приложения…", "Choisir des apps…"},
	xFloatingAppsTitle:  {"Choose apps", "选择应用", "選擇 App", "앱 선택", "アプリを選択", "Выбор приложений", "Choisir des apps"},
	xFloatingAppsSearch: {"Search apps", "搜索应用", "搜尋 App", "앱 검색", "アプリを検索", "Поиск приложений", "Rechercher des apps"},
	xFloatingAppsAddFile: {
		"Add from File…", "从文件添加…", "從檔案加入…", "파일에서 추가…", "ファイルから追加…", "Добавить из файла…", "Ajouter depuis un fichier…",
	},
	xFloatingAppsEmpty: {"No matching apps", "没有匹配的应用", "沒有符合的 App", "일치하는 앱 없음", "一致するアプリはありません", "Нет подходящих приложений", "Aucune app correspondante"},
	xFloatingAppsLoadFailed: {
		"Couldn’t list the installed apps. You can still add them from files",
		"无法列出已安装的应用，仍可从文件添加",
		"無法列出已安裝的 App，仍可從檔案加入",
		"설치된 앱 목록을 불러오지 못했습니다. 파일에서 추가할 수는 있습니다",
		"インストール済みのアプリを一覧にできませんでした。ファイルからは追加できます",
		"Не удалось получить список установленных приложений. Их можно добавить из файлов",
		"Impossible de lister les apps installées. Vous pouvez toujours les ajouter depuis des fichiers",
	},
	xFloatingAppsNotApp: {
		"Some of the files are not apps and were skipped",
		"部分文件不是应用，已跳过",
		"部分檔案不是 App，已略過",
		"일부 파일은 앱이 아니어서 건너뛰었습니다",
		"アプリではないファイルがあったため、スキップしました",
		"Некоторые файлы не являются приложениями и были пропущены",
		"Certains fichiers ne sont pas des apps et ont été ignorés",
	},
	xFloatingOffList: {
		"When another app is in front", "名单外应用在前台时", "名單外 App 位於前景時", "목록에 없는 앱이 앞에 있을 때",
		"リスト外のアプリが前面にあるとき", "Когда активно другое приложение", "Quand une autre app est au premier plan",
	},
	xFloatingOffListLower: {
		"Move behind it", "退到其窗口之下", "退到其視窗之下", "그 앱 뒤로 이동", "そのアプリの背面に移動", "Уйти под его окно", "Passer derrière elle",
	},
	xFloatingOffListHide: {"Hide", "隐藏", "隱藏", "숨기기", "隠す", "Скрыть", "Masquer"},
	xFloatingOffListLowerHint: {
		"Stays on screen without staying on top, so switching back to a listed app does not make it reappear",
		"悬浮窗保持显示但不再置顶，切回名单应用时无需重新出现",
		"浮動視窗保持顯示但不再置頂，切回名單 App 時無須重新出現",
		"화면에 그대로 두되 맨 위에 고정하지 않아, 목록의 앱으로 돌아올 때 다시 나타나지 않습니다",
		"画面に表示したまま最前面を解除するため、リストのアプリに戻っても表示し直しません",
		"Остаётся на экране, но не поверх окон, поэтому при возврате к приложению из списка не появляется заново",
		"Reste à l’écran sans rester au-dessus des fenêtres : revenir à une app de la liste ne la fait pas réapparaître",
	},
	xFloatingOffListHideHint: {
		"Hides, and shows again when a listed app comes back in front",
		"隐藏悬浮窗，名单应用回到前台时重新显示",
		"隱藏浮動視窗，名單 App 回到前景時重新顯示",
		"플로팅 창을 숨기고, 목록의 앱이 다시 앞에 오면 표시합니다",
		"フローティングウインドウを隠し、リストのアプリが前面に戻ると再表示します",
		"Скрывает плавающее окно и показывает его снова, когда приложение из списка снова активно",
		"Masque la fenêtre flottante et l’affiche de nouveau quand une app de la liste revient au premier plan",
	},
	xDisplayFont: {
		"Display font", "显示字体", "顯示字體", "표시 글꼴", "表示フォント", "Шрифт интерфейса", "Police d’affichage",
	},
	xDisplayFontHint: {
		"Used by the main panel and settings; the menu bar keeps the system font",
		"用于主面板和设置窗口，菜单栏保持系统字体",
		"用於主面板和設定視窗，選單列保持系統字體",
		"메인 패널과 설정 창에 적용되며 메뉴 막대는 시스템 글꼴을 유지합니다",
		"メインパネルと設定ウインドウに適用され、メニューバーはシステムフォントのままです",
		"Для главной панели и настроек; в строке меню остаётся системный шрифт",
		"Pour le panneau principal et les réglages ; la barre des menus garde la police du système",
	},
	xDisplayFontDefault: {"Default", "默认", "預設", "기본값", "デフォルト", "По умолчанию", "Par défaut"},
	xResetTimeFormat: {
		"Next reset time", "下次重置时间格式", "下次重置時間格式", "다음 리셋 시간 형식", "次のリセット時刻の表示", "Время следующего сброса", "Heure de la prochaine réinit.",
	},
	xPanelProviders: {
		"Main panel providers", "主面板服务商", "主面板服務商", "메인 패널 공급자", "メインパネルのプロバイダー", "Провайдеры главной панели", "Fournisseurs du panneau principal",
	},
	xPanelProvidersHint: {
		"Hidden Grok and Claude stop refreshing quotas unless the status bar, floating window or a widget shows them.",
		"隐藏的 Grok、Claude 不再刷新额度，除非状态栏、悬浮窗或桌面组件仍在显示。",
		"隱藏的 Grok、Claude 不再重新整理額度，除非狀態列、浮動視窗或桌面小工具仍在顯示。",
		"숨긴 Grok, Claude는 상태 막대, 플로팅 창 또는 위젯에 표시되지 않으면 사용량을 새로 고치지 않습니다.",
		"非表示の Grok・Claude は、ステータスバー・フローティングウインドウ・ウィジェットに表示されない限り使用量を更新しません。",
		"Скрытые Grok и Claude не обновляют лимиты, если их не показывают строка состояния, плавающее окно или виджет.",
		"Grok et Claude masqués n’actualisent plus leurs quotas, sauf s’ils figurent dans la barre d’état, la fenêtre flottante ou un widget.",
	},
	xPanelProvidersAll: {"Show all", "显示全部", "顯示全部", "모두 표시", "すべて表示", "Показывать все", "Tout afficher"},
	xResetTimeFormatHint: {
		"Click the reset time in the main panel to switch.",
		"在主面板中点击重置时间也可切换。",
		"在主面板中點按重置時間也可切換。",
		"메인 패널에서 리셋 시간을 클릭해도 전환됩니다.",
		"メインパネルでリセット時刻をクリックしても切り替えられます。",
		"Нажмите на время сброса в главной панели, чтобы переключить.",
		"Cliquez sur l’heure de réinit. dans le panneau principal pour basculer.",
	},
	xShowCreditsBalance: {
		"Show Credits balance", "显示 Credits 余额", "顯示 Credits 餘額", "크레딧 잔액 표시", "クレジット残高を表示", "Показывать баланс кредитов", "Afficher le solde de crédits",
	},
	xShowCreditsBalanceHint: {
		"Show the ChatGPT credits balance at the end of the quota section when available.",
		"在配额末尾显示账户的 ChatGPT Credits 余额（可用时）。",
		"在配額末尾顯示帳戶的 ChatGPT Credits 餘額（可用時）。",
		"사용 가능한 경우 할당량 섹션 끝에 ChatGPT 크레딧 잔액을 표시합니다.",
		"利用可能な場合、クォータ欄の末尾に ChatGPT クレジット残高を表示します。",
		"Показывать баланс кредитов ChatGPT в конце раздела квот при наличии.",
		"Affiche le solde de crédits ChatGPT à la fin de la section des quotas le cas échéant.",
	},
	xResetTimeRelative: {"Relative time", "相对时间", "相對時間", "상대 시간", "相対時間", "Относительное время", "Temps relatif"},
	xResetTimeExact:    {"Exact time", "精准时间", "精準時間", "정확한 시간", "正確な時刻", "Точное время", "Heure exacte"},
	xAccountResetsIn:   {"Resets in %@", "%@后重置", "%@後重置", "%@ 후 리셋", "%@後にリセット", "Сброс через %@", "Réinit. dans %@"},
	xAccountResetsAt:   {"Resets %@", "%@ 重置", "%@ 重置", "%@ 리셋", "%@ にリセット", "Сброс %@", "Réinit. %@"},
	xProxyTestURL: {
		"Test site", "测试站点", "測試站點", "테스트 사이트", "テストサイト", "Тестовый сайт", "Site de test",
	},
	xProxyTestURLInvalid: {
		"Enter an http or https address for the test site.",
		"请输入 http 或 https 开头的测试站点地址",
		"請輸入 http 或 https 開頭的測試站點位址",
		"테스트 사이트에 http 또는 https 주소를 입력하세요.",
		"テストサイトには http または https のアドレスを入力してください。",
		"Введите адрес тестового сайта с http или https.",
		"Saisissez une adresse http ou https pour le site de test.",
	},
	xSpeedTitle: {
		"Speed analysis", "速率分析", "速率分析", "속도 분석", "速度分析", "Анализ скорости", "Analyse de vitesse",
	},
	xSpeedModuleDescription: {
		"First-token latency and output speed by model and reasoning effort, from recent local sessions.",
		"按模型与思考等级统计最近本机会话的首字延迟与输出速率。",
		"按模型與思考等級統計最近本機工作階段的首字延遲與輸出速率。",
		"최근 로컬 세션에서 모델과 추론 수준별 첫 토큰 지연과 출력 속도를 집계합니다.",
		"直近のローカル セッションから、モデルと推論レベルごとの最初のトークンまでの遅延と出力速度を集計します。",
		"Задержка до первого токена и скорость вывода по моделям и уровню рассуждения в недавних локальных сессиях.",
		"Latence du premier jeton et vitesse de sortie par modèle et niveau de raisonnement, d’après les sessions locales récentes.",
	},
	xSpeedInfoTTFT: {
		"TTFT (time to first token) is the wait from sending a request until the first streamed output arrives.",
		"首字延迟（TTFT）：从发出请求到收到第一段流式输出的等待时间。",
		"首字延遲（TTFT）：從送出請求到收到第一段串流輸出的等待時間。",
		"TTFT(첫 토큰 지연)는 요청을 보낸 뒤 첫 스트리밍 출력이 도착할 때까지의 대기 시간입니다.",
		"TTFT（最初のトークンまでの時間）は、リクエスト送信から最初のストリーミング出力が届くまでの待ち時間です。",
		"TTFT (время до первого токена) — ожидание от отправки запроса до первого потокового вывода.",
		"TTFT (délai avant le premier jeton) : attente entre l’envoi de la requête et l’arrivée de la première sortie en flux.",
	},
	xSpeedInfoNoTTFT: {
		"These logs have no first-token time, so TTFT is not shown and output speed is timed from the request, wait included.",
		"该服务商的日志没有首字时间，因此不显示首字延迟，输出速率从发出请求开始计时，包含等待时间。",
		"該服務商的記錄沒有首字時間，因此不顯示首字延遲，輸出速率從送出請求開始計時，包含等待時間。",
		"이 로그에는 첫 토큰 시각이 없어 TTFT를 표시하지 않으며, 출력 속도는 대기 시간을 포함해 요청 시점부터 측정합니다.",
		"このログには最初のトークンの時刻がないため TTFT は表示せず、出力速度は待ち時間を含めリクエスト送信時から計測します。",
		"В этих логах нет времени первого токена, поэтому TTFT не показывается, а скорость вывода считается от отправки запроса с учётом ожидания.",
		"Ces journaux n’indiquent pas l’heure du premier jeton : le TTFT n’est pas affiché et la vitesse de sortie est mesurée depuis la requête, attente comprise.",
	},
	xSpeedInfoOutput: {
		"Output speed is output tokens (reasoning included) divided by streaming time when first-token timing exists; otherwise it uses end-to-end request time, wait included.",
		"输出速率为输出 token（含思考）除以耗时；有首字计时时使用流式输出时间，否则使用包含等待的端到端请求时间。",
		"輸出速率為輸出 token（含思考）除以耗時；有首字計時時使用串流輸出時間，否則使用包含等待的端到端請求時間。",
		"출력 속도는 출력 토큰(추론 포함)을 소요 시간으로 나눈 값입니다. 첫 토큰 시간이 있으면 스트리밍 시간을, 없으면 대기를 포함한 요청의 전체 시간을 사용합니다.",
		"出力速度は出力トークン（推論を含む）を所要時間で割った値です。最初のトークン時刻がある場合はストリーミング時間を、ない場合は待ち時間を含むリクエスト全体の時間を使います。",
		"Скорость вывода — число выходных токенов с рассуждением, делённое на длительность. При наличии времени первого токена используется время потока, иначе — полное время запроса с ожиданием.",
		"La vitesse de sortie correspond aux jetons produits, raisonnement inclus, divisés par la durée. Avec l’heure du premier jeton, elle utilise la durée du flux ; sinon, la durée complète de la requête, attente comprise.",
	},
	xSpeedInfoSource: {
		"Measured from timestamps in the local CLI logs, so network latency and queueing are included. For Grok, TPS includes only turns with complete first-token timing for every model call and a unique completion match; reasoning effort is set per session.",
		"根据本机 CLI 日志中的时间戳测得，包含网络延迟与排队时间；Grok 的 TPS 仅统计每次模型调用都有完整首字计时且完成记录配对唯一的回合，思考等级以会话为单位。",
		"根據本機 CLI 記錄中的時間戳記測得，包含網路延遲與排隊時間；Grok 的 TPS 僅統計每次模型呼叫都有完整首字計時且完成記錄配對唯一的回合，思考等級以工作階段為單位。",
		"로컬 CLI 로그의 타임스탬프로 측정하므로 네트워크 지연과 대기 시간이 포함됩니다. Grok TPS는 모든 모델 호출의 첫 토큰 시간이 완전하고 완료 기록이 하나로 명확히 대응되는 턴만 집계하며, 추론 수준은 세션 단위입니다.",
		"ローカル CLI ログのタイムスタンプから計測するため、ネットワーク遅延や待機時間を含みます。Grok の TPS は、すべてのモデル呼び出しで最初のトークン時刻が揃い、完了記録との対応が一意なターンだけを集計し、推論レベルはセッション単位です。",
		"Измерено по меткам времени в локальных логах CLI, поэтому учитывает сетевые задержки и очередь. Для Grok TPS рассчитывается только для ходов с полным временем первого токена каждого вызова модели и однозначно сопоставленной записью завершения; уровень рассуждения задаётся на сессию.",
		"Mesuré d’après les horodatages des journaux CLI, avec la latence réseau et la file d’attente. Pour Grok, le TPS ne compte que les tours où chaque appel de modèle a un délai de premier jeton complet et une correspondance d’achèvement unique ; le niveau de raisonnement est défini par session.",
	},
	xSpeedTTFT:   {"TTFT", "首字", "首字", "첫 토큰", "TTFT", "TTFT", "TTFT"},
	xSpeedOutput: {"Output", "输出", "輸出", "출력", "出力", "Вывод", "Sortie"},
	xSpeedModel:  {"Model", "模型", "模型", "모델", "モデル", "Модель", "Modèle"},
	xSpeedCalls:  {"Calls", "次数", "次數", "횟수", "回数", "Вызовы", "Appels"},
	xSpeedByModel: {
		"By model and reasoning effort", "按模型与思考等级", "按模型與思考等級", "모델 및 추론 수준별", "モデルと推論レベル別",
		"По модели и уровню рассуждения", "Par modèle et niveau de raisonnement",
	},
	xSpeedBasis: {
		"Based on the latest %@ sessions", "基于最近 %@ 个会话", "基於最近 %@ 個工作階段", "최근 세션 %@개 기준",
		"直近 %@ 件のセッションに基づく", "По последним сессиям: %@", "Sur les %@ dernières sessions",
	},
	xSpeedBasisOne: {
		"Based on the latest session", "基于最近 1 个会话", "基於最近 1 個工作階段", "최근 세션 1개 기준",
		"直近 1 件のセッションに基づく", "По последней сессии", "Sur la dernière session",
	},
	xSpeedLatest: {"Latest: %@", "最新：%@", "最新：%@", "최신: %@", "最新: %@", "Последняя: %@", "Dernière : %@"},
	xSpeedEmpty: {
		"No speed data in recent sessions.", "最近的会话中暂无速率数据。", "最近的工作階段中暫無速率資料。",
		"최근 세션에 속도 데이터가 없습니다.", "直近のセッションに速度データがありません。",
		"В недавних сессиях нет данных о скорости.", "Aucune donnée de vitesse dans les sessions récentes.",
	},
	xSpeedRange: {
		"Samples from %@ to %@", "样本时间：%@ 至 %@", "樣本時間：%@ 至 %@", "표본 기간: %@ ~ %@",
		"サンプル期間: %@ ～ %@", "Период выборки: %@ — %@", "Échantillons du %@ au %@",
	},
	xSpeedTierFast:     {"Fast", "Fast", "Fast", "Fast", "Fast", "Fast", "Fast"},
	xSpeedTierStandard: {"Standard", "标准", "標準", "표준", "標準", "Стандарт", "Standard"},
	xSpeedInfoTier: {
		"Once a Fast call falls in the range, Fast and standard calls are listed separately; calls whose log does not record the tier stay unmarked. Speed also depends on load and time of day, so treat the comparison as a rough guide.",
		"统计范围内出现 Fast 调用时，Fast 与标准档分行列出；日志未记录档位的调用不加标记。速率还受负载与时段影响，对比仅供参考。",
		"統計範圍內出現 Fast 呼叫時，Fast 與標準檔分行列出；記錄未標示檔位的呼叫不加標記。速率也受負載與時段影響，比較僅供參考。",
		"범위 안에 Fast 호출이 있으면 Fast와 표준 호출을 따로 표시하며, 로그에 등급이 기록되지 않은 호출에는 표시를 붙이지 않습니다. 속도는 부하와 시간대의 영향도 받으므로 비교는 참고용입니다.",
		"範囲内に Fast の呼び出しがあると、Fast と標準を別々の行に表示し、ログに区分がない呼び出しには表示を付けません。速度は負荷や時間帯にも左右されるため、比較は目安です。",
		"Если в периоде есть вызовы Fast, они показываются отдельно от стандартных; вызовы без указания режима в логе не помечаются. Скорость зависит также от нагрузки и времени суток, поэтому сравнение приблизительное.",
		"Dès qu’un appel Fast figure dans la période, les appels Fast et standard sont listés séparément ; ceux dont le journal n’indique pas le niveau ne sont pas marqués. La vitesse dépend aussi de la charge et de l’heure : la comparaison reste indicative.",
	},
	xAccountCount: {
		"%@ accounts", "%@ 个账号", "%@ 個帳號", "계정 %@개", "%@ アカウント", "Аккаунтов: %@", "%@ comptes",
	},
	xAccountCountOne: {
		"%@ account", "%@ 个账号", "%@ 個帳號", "계정 %@개", "%@ アカウント", "%@ аккаунт", "%@ compte",
	},
	xPrivacyDisplaySection: {
		"Privacy display", "隐私显示", "隱私顯示", "개인정보 표시", "プライバシー表示", "Приватное отображение", "Affichage confidentiel",
	},
	xPrivacyDisplayDescription: {
		"Mask account names and emails in account lists and the main panel.",
		"隐藏账号列表和主面板中的账号名称与邮箱",
		"隱藏帳號清單與主面板中的帳號名稱和電子郵件",
		"계정 목록과 메인 패널에서 계정 이름과 이메일을 가립니다.",
		"アカウント一覧とメインパネルのアカウント名とメールアドレスを隠します。",
		"Скрывает имена и адреса почты в списках аккаунтов и на главной панели.",
		"Masque les noms et adresses e-mail dans les listes de comptes et le panneau principal.",
	},
	xProvider: {
		"Provider", "平台", "平台", "플랫폼", "プラットフォーム", "Платформа", "Plateforme",
	},
	xProviderDescription: {
		"The platform the main panel and the status bar show first.",
		"主面板和状态栏优先显示的平台。",
		"主面板與狀態列優先顯示的平台。",
		"메인 패널과 상태 표시줄에 먼저 표시할 플랫폼입니다.",
		"メインパネルとステータスバーで最初に表示するプラットフォームです。",
		"Платформа, которая показывается первой на главной панели и в строке состояния.",
		"La plateforme affichée en premier dans le panneau principal et la barre d'état.",
	},
	xPanelHeight: {
		"Main panel height", "主面板高度", "主面板高度", "메인 패널 높이", "メインパネルの高さ",
		"Высота главной панели", "Hauteur du panneau principal",
	},
	xPanelHeightHint: {
		"Defaults to 65% of the screen height; from 360 points up to 75%.", "默认为屏幕高度的 65%，最低 360 点，最高 75%。", "預設為螢幕高度的 65%，最低 360 點，最高 75%。",
		"기본값은 화면 높이의 65%이며, 최소 360포인트에서 최대 75%까지 조절할 수 있습니다.", "初期値は画面の高さの65%、最小360ポイント、最大75%です。", "По умолчанию — 65% высоты экрана; от 360 пунктов до 75%.", "Par défaut, 65 % de la hauteur de l’écran ; de 360 points à 75 %.",
	},
	xMacOnly: {
		"Applies to the macOS menu bar and panel only. Your saved choice is kept.",
		"仅适用于 macOS 菜单栏和面板，已保存的选择会保留。",
		"僅適用於 macOS 選單列與面板，已儲存的選擇會保留。",
		"macOS 메뉴 막대와 패널에만 적용됩니다. 저장된 선택은 유지됩니다.",
		"macOS のメニューバーとパネルにのみ適用されます。保存済みの選択は保持されます。",
		"Действует только в строке меню и панели macOS. Сохранённый выбор сохраняется.",
		"S'applique uniquement à la barre des menus et au panneau de macOS. Votre choix enregistré est conservé.",
	},
	xWidgetMac14: {
		"Widgets need macOS 14 or later. Your saved choice is kept.",
		"小组件需要 macOS 14 或更新版本，已保存的选择会保留。",
		"小工具需要 macOS 14 或更新版本，已儲存的選擇會保留。",
		"위젯은 macOS 14 이상에서 사용할 수 있습니다. 저장된 선택은 유지됩니다.",
		"ウィジェットには macOS 14 以降が必要です。保存済みの選択は保持されます。",
		"Виджеты требуют macOS 14 или новее. Сохранённый выбор сохраняется.",
		"Les widgets nécessitent macOS 14 ou une version ultérieure. Votre choix enregistré est conservé.",
	},
	xSettingsUnreadable: {
		"Settings can't be read", "无法读取设置", "無法讀取設定", "설정을 읽을 수 없습니다", "設定を読み込めません",
		"Не удалось прочитать настройки", "Impossible de lire les réglages",
	},
	xSettingsReadOnly: {
		"Settings are read-only and background tasks are paused. The stored value was not changed; fix or remove it, then restart Quosparr.",
		"设置处于只读状态，后台任务已暂停。已存储的值未被改动；请修复或移除后重新启动 Quosparr。",
		"設定為唯讀，背景工作已暫停。已儲存的值未被更動；請修復或移除後重新啟動 Quosparr。",
		"설정은 읽기 전용이며 백그라운드 작업이 일시 중지되었습니다. 저장된 값은 변경되지 않았습니다. 값을 수정하거나 삭제한 뒤 Quosparr를 다시 시작하세요.",
		"設定は読み取り専用で、バックグラウンド処理は停止しています。保存された値は変更されていません。修正または削除してから Quosparr を再起動してください。",
		"Настройки доступны только для чтения, фоновые задачи приостановлены. Сохранённое значение не изменено: исправьте или удалите его и перезапустите Quosparr.",
		"Les réglages sont en lecture seule et les tâches d'arrière-plan sont suspendues. La valeur enregistrée n'a pas été modifiée : corrigez-la ou supprimez-la, puis relancez Quosparr.",
	},
	xProxyBlocked: {
		"Network tasks are paused until the proxy settings are valid. Saving a valid proxy below replaces the damaged one.",
		"代理设置有效前网络任务已暂停。在下方保存有效的代理将替换已损坏的设置。",
		"代理設定有效前網路工作已暫停。在下方儲存有效的代理將取代已損壞的設定。",
		"프록시 설정이 올바를 때까지 네트워크 작업이 일시 중지됩니다. 아래에서 올바른 프록시를 저장하면 손상된 설정이 대체됩니다.",
		"プロキシ設定が有効になるまでネットワーク処理は停止しています。下で有効なプロキシを保存すると、破損した設定が置き換えられます。",
		"Сетевые задачи приостановлены, пока настройки прокси недействительны. Сохранение корректного прокси ниже заменит повреждённое значение.",
		"Les tâches réseau sont suspendues tant que les réglages du proxy ne sont pas valides. Enregistrer un proxy valide ci-dessous remplace la valeur endommagée.",
	},
	xSettingsSaveFailed: {
		"Could not save settings: %@", "无法保存设置：%@", "無法儲存設定：%@", "설정을 저장할 수 없습니다: %@",
		"設定を保存できませんでした: %@", "Не удалось сохранить настройки: %@", "Impossible d'enregistrer les réglages : %@",
	},
	xAccountsComingSoon: {
		"Account management is being rebuilt for this version. Your saved accounts are not changed.",
		"此版本的账号管理正在重建，已保存的账号不会被改动。",
		"此版本的帳號管理正在重建，已儲存的帳號不會被更動。",
		"이 버전의 계정 관리는 다시 구축하는 중입니다. 저장된 계정은 변경되지 않습니다.",
		"このバージョンのアカウント管理は再構築中です。保存済みのアカウントは変更されません。",
		"Управление аккаунтами для этой версии переписывается. Сохранённые аккаунты не изменяются.",
		"La gestion des comptes est en cours de reconstruction pour cette version. Vos comptes enregistrés ne sont pas modifiés.",
	},
	xTrayUnavailable: {
		"Cannot create the tray icon.", "无法创建托盘图标", "無法建立系統匣圖示", "트레이 아이콘을 만들 수 없습니다.",
		"トレイアイコンを作成できません。", "Не удалось создать значок в трее.", "Impossible de créer l'icône de la zone de notification.",
	},
	xLinuxDesktopDependencies: {
		"Quosparr needs GTK 3 and AppIndicator support. Install the desktop dependencies, then restart the app:\n%@",
		"Quosparr 需要 GTK 3 和 AppIndicator 支持，请安装桌面依赖后重新启动应用：\n%@",
		"Quosparr 需要 GTK 3 與 AppIndicator 支援，請安裝桌面相依套件後重新啟動應用程式：\n%@",
		"Quosparr에는 GTK 3와 AppIndicator 지원이 필요합니다. 데스크톱 의존성을 설치한 뒤 앱을 다시 시작하세요:\n%@",
		"Quosparr には GTK 3 と AppIndicator が必要です。デスクトップの依存パッケージをインストールしてから、アプリを再起動してください:\n%@",
		"Для Quosparr нужны GTK 3 и AppIndicator. Установите зависимости рабочего стола и перезапустите приложение:\n%@",
		"Quosparr nécessite GTK 3 et AppIndicator. Installez les dépendances du bureau, puis relancez l'application :\n%@",
	},
	xLinuxReleaseUnreadable: {
		"Could not identify the Linux distribution from /etc/os-release: %@",
		"无法从 /etc/os-release 识别 Linux 发行版：%@", "無法從 /etc/os-release 識別 Linux 發行版：%@",
		"/etc/os-release에서 Linux 배포판을 확인할 수 없습니다: %@",
		"/etc/os-release から Linux ディストリビューションを確認できませんでした: %@",
		"Не удалось определить дистрибутив Linux из /etc/os-release: %@",
		"Impossible d'identifier la distribution Linux depuis /etc/os-release : %@",
	},
	xPanelPlaceholder: {
		"The full panel is being built. Quota, reset credits and cost will appear here.",
		"完整面板正在构建中，额度、重置次数和费用将显示在这里。",
		"完整面板正在建置中，額度、重置次數與費用將顯示在這裡。",
		"전체 패널을 구축하는 중입니다. 할당량, 리셋 크레딧, 비용이 여기에 표시됩니다.",
		"完全なパネルは開発中です。クォータ、リセットクレジット、コストがここに表示されます。",
		"Полная панель в разработке. Здесь появятся квота, кредиты сброса и стоимость.",
		"Le panneau complet est en cours de développement. Le quota, les crédits de réinitialisation et le coût s'afficheront ici.",
	},
	xNotAvailableYet: {
		"Not available in this build yet.", "此版本暂未提供。", "此版本暫未提供。", "이 빌드에서는 아직 사용할 수 없습니다.",
		"このビルドではまだ利用できません。", "Пока недоступно в этой сборке.", "Pas encore disponible dans cette version.",
	},
	xProxyLocalSaveWarning: {
		"The password cannot be stored securely on this system yet, so a proxy with authentication cannot be saved.",
		"此系统上暂时无法安全保存密码，因此无法保存需要认证的代理。",
		"此系統上暫時無法安全儲存密碼，因此無法儲存需要驗證的代理。",
		"이 시스템에서는 아직 비밀번호를 안전하게 저장할 수 없어 인증이 필요한 프록시를 저장할 수 없습니다.",
		"このシステムではまだパスワードを安全に保存できないため、認証付きのプロキシは保存できません。",
		"Пока невозможно безопасно сохранить пароль в этой системе, поэтому прокси с аутентификацией сохранить нельзя.",
		"Le mot de passe ne peut pas encore être stocké de façon sécurisée sur ce système ; un proxy avec authentification ne peut donc pas être enregistré.",
	},
	xLegacyRunningTitle: {
		"CodexRunway is running", "CodexRunway 正在运行", "CodexRunway 正在執行", "CodexRunway가 실행 중입니다",
		"CodexRunway が実行中です", "CodexRunway запущен", "CodexRunway est en cours d’exécution",
	},
	xLegacyRunningDetail: {
		"Quosparr and the previous CodexRunway refresh and switch the same Codex and Grok sign-ins; running both can sign you out, so they cannot run together. Quosparr starts once CodexRunway quits. The data of CodexRunway is kept.",
		"Quosparr 与旧版 CodexRunway 会刷新和切换同一份 Codex、Grok 登录凭据，同时运行可能导致账号被登出，因此不能同时运行。退出 CodexRunway 后 Quosparr 会继续启动，旧版数据会保留。",
		"Quosparr 與舊版 CodexRunway 會重新整理和切換同一份 Codex、Grok 登入憑證，同時執行可能導致帳號被登出，因此不能同時執行。結束 CodexRunway 後 Quosparr 會繼續啟動，舊版資料會保留。",
		"Quosparr와 이전 CodexRunway는 같은 Codex, Grok 로그인 정보를 갱신하고 전환하므로 함께 실행하면 로그아웃될 수 있어 동시에 실행할 수 없습니다. CodexRunway를 종료하면 Quosparr가 이어서 시작되며, CodexRunway의 데이터는 유지됩니다.",
		"Quosparr と以前の CodexRunway は同じ Codex、Grok のログイン情報を更新・切り替えるため、同時に実行するとログアウトされることがあり、同時には実行できません。CodexRunway を終了すると Quosparr が起動を続けます。CodexRunway のデータは保持されます。",
		"Quosparr и прежний CodexRunway обновляют и переключают одни и те же входы Codex и Grok; при одновременной работе возможен выход из аккаунта, поэтому их нельзя запускать вместе. Quosparr продолжит запуск после выхода из CodexRunway. Данные CodexRunway сохранятся.",
		"Quosparr et l’ancien CodexRunway actualisent et changent les mêmes connexions Codex et Grok ; les exécuter ensemble peut vous déconnecter, ils ne peuvent donc pas fonctionner en même temps. Quosparr démarre dès que CodexRunway est quitté. Les données de CodexRunway sont conservées.",
	},
	xLegacyRunningManual: {
		"Quosparr cannot run while CodexRunway is running, because both manage the same sign-ins. Quit CodexRunway, then open Quosparr again.",
		"CodexRunway 运行期间无法启动 Quosparr，因为两者会管理同一份登录凭据。请先退出 CodexRunway，再重新打开 Quosparr。",
		"CodexRunway 執行期間無法啟動 Quosparr，因為兩者會管理同一份登入憑證。請先結束 CodexRunway，再重新開啟 Quosparr。",
		"CodexRunway가 실행 중일 때는 두 앱이 같은 로그인 정보를 관리하므로 Quosparr를 시작할 수 없습니다. CodexRunway를 종료한 뒤 Quosparr를 다시 여세요.",
		"CodexRunway の実行中は、両方が同じログイン情報を管理するため Quosparr を起動できません。CodexRunway を終了してから、Quosparr をもう一度開いてください。",
		"Quosparr нельзя запустить, пока работает CodexRunway: оба управляют одними и теми же входами. Закройте CodexRunway и снова откройте Quosparr.",
		"Quosparr ne peut pas démarrer tant que CodexRunway est en cours d’exécution, car les deux gèrent les mêmes connexions. Quittez CodexRunway, puis rouvrez Quosparr.",
	},
	xLegacyQuitAndContinue: {
		"Quit CodexRunway and Continue", "退出 CodexRunway 并继续", "結束 CodexRunway 並繼續", "CodexRunway 종료 후 계속",
		"CodexRunway を終了して続ける", "Закрыть CodexRunway и продолжить", "Quitter CodexRunway et continuer",
	},
	xLegacyQuitFailed: {
		"CodexRunway did not quit", "CodexRunway 没有退出", "CodexRunway 沒有結束", "CodexRunway가 종료되지 않았습니다",
		"CodexRunway が終了しませんでした", "CodexRunway не закрылся", "CodexRunway ne s’est pas fermé",
	},
}

// tx returns the supplemental text for loc, formatted with args. A missing
// language falls back to English, as the Swift tables do.
func tx(loc l10n.Localizer, k textKey, args ...any) string {
	row, ok := extraTexts[k]
	if !ok {
		return string(k)
	}
	i := slices.Index(l10n.Languages(), loc.Language())
	if i < 0 || row[i] == "" {
		i = 0
	}
	return l10n.Format(row[i], args...)
}
