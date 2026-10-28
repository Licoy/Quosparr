package app

const (
	xGrokPlanIdentityStage        textKey = "grokPlanIdentityStage"
	xGrokPlanSubscriptionStage    textKey = "grokPlanSubscriptionStage"
	xGrokPlanRequestFailed        textKey = "grokPlanRequestFailed"
	xGrokPlanTimeout              textKey = "grokPlanTimeout"
	xGrokPlanCanceled             textKey = "grokPlanCanceled"
	xGrokPlanAuthentication       textKey = "grokPlanAuthentication"
	xGrokPlanInvalidResponse      textKey = "grokPlanInvalidResponse"
	xGrokPlanIdentityMismatch     textKey = "grokPlanIdentityMismatch"
	xGrokPlanSubscriptionMismatch textKey = "grokPlanSubscriptionMismatch"
	xGrokPlanHTTPStatus           textKey = "grokPlanHTTPStatus"
	xGrokXSubscription            textKey = "grokXSubscription"
	xGrokLegacySubscription       textKey = "grokLegacySubscription"
	xGrokPlanUnavailable          textKey = "grokPlanUnavailable"
)

func init() {
	extraTexts[xGrokPlanIdentityStage] = [7]string{"Account metadata", "账号信息", "帳號資訊", "계정 정보", "アカウント情報", "Сведения об аккаунте", "Métadonnées du compte"}
	extraTexts[xGrokPlanSubscriptionStage] = [7]string{"Subscription metadata", "订阅信息", "訂閱資訊", "구독 정보", "サブスクリプション情報", "Сведения о подписке", "Métadonnées d’abonnement"}

	extraTexts[xGrokPlanRequestFailed] = [7]string{"Request failed", "请求失败", "請求失敗", "요청 실패", "リクエスト失敗", "Ошибка запроса", "Échec de la requête"}
	extraTexts[xGrokPlanTimeout] = [7]string{"Request timed out", "请求超时", "請求逾時", "요청 시간 초과", "リクエストのタイムアウト", "Время запроса истекло", "Délai de requête dépassé"}
	extraTexts[xGrokPlanCanceled] = [7]string{"Request canceled", "请求已取消", "請求已取消", "요청 취소됨", "リクエストがキャンセルされました", "Запрос отменён", "Requête annulée"}
	extraTexts[xGrokPlanAuthentication] = [7]string{"Plan metadata authentication required", "套餐信息需要身份验证", "方案資訊需要身分驗證", "요금제 정보 인증 필요", "プラン情報の認証が必要です", "Нужна авторизация для сведений о тарифе", "Authentification requise pour les détails de l’offre"}
	extraTexts[xGrokPlanInvalidResponse] = [7]string{"Invalid plan response", "套餐响应格式无效", "方案回應格式無效", "잘못된 요금제 응답", "無効なプラン応答", "Некорректный ответ о тарифе", "Réponse d’offre invalide"}
	extraTexts[xGrokPlanIdentityMismatch] = [7]string{"Plan account identity mismatch", "套餐账号身份不匹配", "方案帳號身分不符", "요금제 계정 불일치", "プランのアカウントが一致しません", "Аккаунт тарифа не совпадает", "Le compte de l’offre ne correspond pas"}
	extraTexts[xGrokPlanSubscriptionMismatch] = [7]string{"Subscription metadata mismatch", "订阅信息不匹配", "訂閱資訊不符", "구독 정보 불일치", "サブスクリプション情報が一致しません", "Сведения о подписке не совпадают", "Les métadonnées d’abonnement ne correspondent pas"}
	extraTexts[xGrokPlanHTTPStatus] = [7]string{"Plan request HTTP status %@", "套餐请求 HTTP 状态 %@", "方案請求 HTTP 狀態 %@", "요금제 요청 HTTP 상태 %@", "プランリクエスト HTTP ステータス %@", "HTTP-статус запроса тарифа %@", "Statut HTTP de la requête d’offre %@"}

	extraTexts[xGrokXSubscription] = [7]string{
		"The X Subscription plan", "The X Subscription 套餐", "The X Subscription 方案", "The X Subscription 요금제", "The X Subscription プラン", "Тариф The X Subscription", "Offre The X Subscription",
	}
	extraTexts[xGrokLegacySubscription] = [7]string{
		"Legacy subscription", "旧版订阅", "舊版訂閱", "기존 구독", "旧版サブスクリプション", "Прежняя подписка", "Ancien abonnement",
	}
	extraTexts[xGrokPlanUnavailable] = [7]string{
		"Plan details unavailable; quota is unaffected", "套餐信息读取失败，额度不受影响", "方案資訊讀取失敗，額度不受影響", "요금제 정보를 불러올 수 없습니다; 사용량에는 영향이 없습니다", "プラン情報を取得できません、利用枠への影響はありません", "Сведения о тарифе недоступны; квота не затронута", "Détails de l’offre indisponibles ; le quota n’est pas affecté",
	}
}
