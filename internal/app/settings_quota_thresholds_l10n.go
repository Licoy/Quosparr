package app

const (
	xQuotaThresholds        textKey = "quotaThresholds"
	xQuotaThresholdHint     textKey = "quotaThresholdHint"
	xQuotaThresholdRules    textKey = "quotaThresholdRules"
	xQuotaThresholdRemove   textKey = "quotaThresholdRemove"
	xQuotaThresholdAdd      textKey = "quotaThresholdAdd"
	xQuotaThresholdDefaults textKey = "quotaThresholdDefaults"
	xQuotaThresholdInvalid  textKey = "quotaThresholdInvalid"
)

func init() {
	for key, texts := range map[textKey][7]string{
		xQuotaThresholds:        {"Codex used quota thresholds", "Codex 已用配额阈值", "Codex 已用配額門檻", "Codex 사용 할당량 임계값", "Codex 使用済みクォータのしきい値", "Пороги использованной квоты Codex", "Seuils de quota utilisé de Codex"},
		xQuotaThresholdHint:     {"Search or choose a percentage; click × to remove; changes apply immediately", "输入搜索或选择百分比，点击 × 移除，更改立即生效", "輸入搜尋或選擇百分比，點擊 × 移除，變更立即生效", "검색하거나 백분율을 선택하세요. ×로 삭제하며 변경은 즉시 적용됩니다", "検索または割合を選択 · × で削除 · 変更はすぐに適用", "Найдите или выберите процент; × удаляет; изменения применяются сразу", "Recherchez ou choisissez un pourcentage ; × pour retirer ; application immédiate"},
		xQuotaThresholdRules:    {"Only the highest crossed threshold alerts; 0% is reached on the first reading", "跨过多档时只提醒最高档，0% 在首次读取时即视为达到", "跨過多檔時只提醒最高檔，0% 在首次讀取時即視為達到", "한 번에 넘은 가장 높은 임계값만 알리며 0%는 첫 읽기부터 도달로 간주합니다", "一度に超えた最高のしきい値のみ通知し、0% は初回読み取りで到達と見なします", "Уведомляется наивысший пересечённый порог; 0% достигнут при первом чтении", "Seul le seuil franchi le plus élevé déclenche une alerte ; 0 % est atteint dès la première lecture"},
		xQuotaThresholdRemove:   {"Remove %d%% threshold", "移除 %d%% 阈值", "移除 %d%% 門檻", "%d%% 임계값 삭제", "%d%% のしきい値を削除", "Удалить порог %d%%", "Supprimer le seuil %d%%"},
		xQuotaThresholdAdd:      {"Add threshold", "添加阈值", "新增門檻", "임계값 추가", "しきい値を追加", "Добавить порог", "Ajouter un seuil"},
		xQuotaThresholdDefaults: {"Restore defaults", "恢复默认", "還原預設", "기본값 복원", "既定値に戻す", "По умолчанию", "Valeurs par défaut"},
		xQuotaThresholdInvalid:  {"Could not update quota thresholds: %v", "无法更新配额阈值：%v", "無法更新配額門檻：%v", "할당량 임계값을 업데이트할 수 없습니다: %v", "クォータのしきい値を更新できません：%v", "Не удалось обновить пороги квоты: %v", "Impossible de modifier les seuils de quota : %v"},
	} {
		extraTexts[key] = texts
	}
}
