package app

const (
	xStatusBarTab             textKey = "statusBarTab"
	xStatusBarPreview         textKey = "statusBarPreview"
	xStatusBarPreviewHint     textKey = "statusBarPreviewHint"
	xStatusBarFollowHint      textKey = "statusBarFollowHint"
	xStatusBarBattery         textKey = "statusBarBatteryStyle"
	xStatusBarArrangementHint textKey = "statusBarArrangementHint"
	xStatusBarSeparateHint    textKey = "statusBarSeparateHint"
	xStatusBarJoin            textKey = "statusBarJoin"
	xStatusBarJoinProvider    textKey = "statusBarJoinProvider"
	xStatusBarShowProvider    textKey = "statusBarShowProvider"
	xStatusBarOptions         textKey = "statusBarOptions"
	xStatusBarWindow          textKey = "statusBarWindow"
	xStatusBarWindowAuto      textKey = "statusBarWindowAuto"
	xStatusBarWindowHint      textKey = "statusBarWindowHint"
	xStatusBarIcons           textKey = "statusBarIcons"
	xStatusBarIconsHint       textKey = "statusBarIconsHint"
	xBatteryOrientation       textKey = "batteryOrientation"
	xBatteryOrientationHint   textKey = "batteryOrientationHint"
	xBatteryHorizontal        textKey = "batteryHorizontal"
	xBatteryVertical          textKey = "batteryVertical"
	xBatteryText              textKey = "batteryText"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xStatusBarTab:         {"Menu Bar Item", "状态栏组件", "狀態欄元件", "메뉴 막대 항목", "メニューバー項目", "Значок в строке меню", "Élément de la barre de menus"},
		xStatusBarPreview:     {"Preview", "预览", "預覽", "미리 보기", "プレビュー", "Предпросмотр", "Aperçu"},
		xStatusBarPreviewHint: {"Sample quotas in the current settings; the menu bar shows your own", "示例额度，按当前设置显示；菜单栏显示实际额度", "範例額度，依目前設定顯示；選單列顯示實際額度", "현재 설정으로 표시한 예시 할당량이며, 메뉴 막대에는 실제 할당량이 표시됩니다", "現在の設定で表示したサンプルです。メニューバーには実際のクォータが表示されます", "Пример квот с текущими настройками; в строке меню показываются ваши", "Quotas d’exemple avec les réglages actuels ; la barre de menus affiche les vôtres"},
		xStatusBarArrangementHint: {
			"Each provider is a separate item; group two to stack them in one",
			"每个服务商单独显示；与上方组合可将两个服务商上下叠放在同一组件中",
			"每個服務商單獨顯示；與上方組合可將兩個服務商上下疊放在同一元件中",
			"공급자마다 따로 표시하며, 위와 묶으면 두 공급자를 한 항목에 위아래로 쌓습니다",
			"プロバイダーごとに個別に表示します。上と組み合わせると 2 つを 1 項目に上下に重ねます",
			"Каждый провайдер показывается отдельно; объединение с верхним ставит двоих друг над другом в одном элементе",
			"Chaque fournisseur est affiché à part ; regroupez-en deux pour les empiler dans un même élément",
		},
		xStatusBarSeparateHint: {
			"This style shows every provider separately",
			"此样式下每个服务商单独显示",
			"此樣式下每個服務商單獨顯示",
			"이 스타일에서는 공급자마다 따로 표시합니다",
			"このスタイルでは各プロバイダーを個別に表示します",
			"В этом стиле каждый провайдер показывается отдельно",
			"Ce style affiche chaque fournisseur à part",
		},
		xStatusBarJoin: {"Group with above", "与上方组合", "與上方組合", "위와 묶기", "上と組み合わせる", "Объединить с верхним", "Grouper avec le précédent"},
		xStatusBarFollowHint: {
			"Shows the provider selected in the main panel",
			"显示主面板当前选中的服务商",
			"顯示主面板目前選取的服務商",
			"메인 패널에서 선택한 공급자를 표시합니다",
			"メインパネルで選択中のプロバイダーを表示します",
			"Показывает провайдера, выбранного на главной панели",
			"Affiche le fournisseur sélectionné dans le panneau principal",
		},
		xStatusBarBattery:      {"Battery", "电池", "電池", "배터리", "バッテリー", "Батарея", "Batterie"},
		xStatusBarJoinProvider: {"Group %@ with above", "将 %@ 与上方组合", "將 %@ 與上方組合", "%@ 위와 묶기", "%@ を上と組み合わせる", "Объединить %@ с верхним", "Grouper %@ avec le précédent"},
		xStatusBarShowProvider: {"Show %@", "显示 %@", "顯示 %@", "%@ 표시", "%@ を表示", "Показывать %@", "Afficher %@"},
		xStatusBarOptions:      {"Display", "显示", "顯示", "표시", "表示", "Отображение", "Affichage"},
		xStatusBarWindow:       {"Quota window", "额度周期", "額度週期", "할당량 기간", "クォータ期間", "Период квоты", "Période de quota"},
		xStatusBarWindowAuto:   {"Automatic", "自动", "自動", "자동", "自動", "Автоматически", "Automatique"},
		xStatusBarWindowHint: {
			"Automatic shows the two main windows of a provider on its own and the main window of grouped ones; a provider without the chosen window shows its main one",
			"自动：单独显示的服务商显示两个主要周期，组合显示时各显示主要周期；没有所选周期的服务商显示其主要周期",
			"自動：單獨顯示的服務商顯示兩個主要週期，組合顯示時各顯示主要週期；沒有所選週期的服務商顯示其主要週期",
			"자동: 단독 공급자는 주요 기간 두 개를, 묶인 공급자는 주요 기간을 표시하며, 선택한 기간이 없는 공급자는 주요 기간을 표시합니다",
			"自動：単独のプロバイダーは主要な 2 期間、組み合わせ時は主要期間を表示します。選んだ期間がない場合は主要期間を表示します",
			"Автоматически: отдельный провайдер показывает два основных периода, объединённые — основной; без выбранного периода показывается основной",
			"Automatique : les deux périodes principales d’un fournisseur seul, la principale de ceux regroupés ; sans la période choisie, la principale est affichée",
		},
		xStatusBarIcons: {"Show provider icons", "显示服务商图标", "顯示服務商圖示", "공급자 아이콘 표시", "プロバイダーのアイコンを表示", "Показывать значки провайдеров", "Afficher les icônes des fournisseurs"},
		xStatusBarIconsHint: {
			"Without icons, stacked rows are told apart by window name or order",
			"关闭后，叠放的行以周期名称或顺序区分",
			"關閉後，疊放的列以週期名稱或順序區分",
			"끄면 쌓인 행을 기간 이름이나 순서로 구분합니다",
			"オフにすると、重ねた行は期間名または順序で区別します",
			"Без значков строки различаются по названию периода или порядку",
			"Sans icônes, les lignes empilées se distinguent par le nom de la période ou l’ordre",
		},
		xBatteryOrientation: {"Battery orientation", "电池方向", "電池方向", "배터리 방향", "バッテリーの向き", "Ориентация батареи", "Orientation de la batterie"},
		xBatteryOrientationHint: {
			"Horizontal puts the percentage inside the battery and the detail beside it; vertical stands the batteries up beside the text",
			"横向：百分比显示在电池内，详情在旁；竖向：电池竖立，文字显示在旁",
			"橫向：百分比顯示在電池內，詳情在旁；直向：電池直立，文字顯示在旁",
			"가로: 배터리 안에 백분율, 옆에 상세 정보를 표시합니다. 세로: 배터리를 세우고 옆에 텍스트를 표시합니다",
			"横：バッテリー内に割合、横に詳細を表示します。縦：バッテリーを立てて横にテキストを表示します",
			"Горизонтально: процент внутри батареи, подробности рядом; вертикально: батареи стоят рядом с текстом",
			"Horizontale : le pourcentage dans la batterie, le détail à côté ; verticale : les batteries debout à côté du texte",
		},
		xBatteryHorizontal: {"Horizontal", "横向", "橫向", "가로", "横", "Горизонтально", "Horizontale"},
		xBatteryVertical:   {"Vertical", "竖向", "直向", "세로", "縦", "Вертикально", "Verticale"},
		xBatteryText:       {"Text beside battery", "电池旁显示", "電池旁顯示", "배터리 옆 텍스트", "バッテリー横のテキスト", "Текст рядом с батареей", "Texte à côté de la batterie"},
	} {
		extraTexts[key] = texts
	}
}
