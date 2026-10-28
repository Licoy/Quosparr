package grok

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SubscriptionInfo preserves the selected subscription's original tier and
// billing system, since the same tier is used by both product generations.
type SubscriptionInfo struct {
	Tier          string `json:"tier"`
	BillingSystem string `json:"billingSystem"`
}

// DisplayName resolves only the billing system explicitly returned for this
// subscription; unified billing and allowance amounts do not identify a plan.
func (s SubscriptionInfo) DisplayName() (string, bool) {
	tier := subscriptionProtoTier(s.Tier)
	if tier == TierUnknown {
		return "", false
	}
	if s.BillingSystem == "BILLING_SYSTEM_COMMERCE" {
		switch tier {
		case TierSuperGrokLite:
			tier = TierXSubscriptionPlus
		case TierSuperGrok:
			tier = TierXSubscriptionPremium
		case TierSuperGrokPlus:
			tier = TierXSubscriptionSuper
		case TierSuperGrokHeavy:
			tier = TierXSubscriptionUltra
		}
	} else if s.BillingSystem != "BILLING_SYSTEM_LEGACY" {
		// Reused proto tiers alone cannot establish the product generation
		return "", false
	}
	return tier.DisplayName(), true
}

func subscriptionNamedTier(raw string) TierKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "plus", "x · plus", "xpass:plus":
		return TierXSubscriptionPlus
	case "premium", "x · premium", "xpass:premium":
		return TierXSubscriptionPremium
	case "super", "x · super", "xpass:super":
		return TierXSubscriptionSuper
	case "ultra", "x · ultra", "xpass:ultra":
		return TierXSubscriptionUltra
	}
	return TierUnknown
}

func subscriptionProtoTier(raw string) TierKind {
	switch raw {
	case "SUBSCRIPTION_TIER_SUPER_GROK_LITE":
		return TierSuperGrokLite
	case "SUBSCRIPTION_TIER_GROK_PRO":
		return TierSuperGrok
	case "SUBSCRIPTION_TIER_SUPER_GROK_PLUS":
		return TierSuperGrokPlus
	case "SUBSCRIPTION_TIER_SUPER_GROK_PRO":
		return TierSuperGrokHeavy
	case "SUBSCRIPTION_TIER_X_BASIC":
		return TierXBasic
	case "SUBSCRIPTION_TIER_X_PREMIUM":
		return TierXPremium
	case "SUBSCRIPTION_TIER_X_PREMIUM_PLUS":
		return TierXPremiumPlus
	}
	return TierUnknown
}

// SafeTierDisplayName is the UI boundary for cached or server-supplied labels
// Unknown names remain neutral text; structured data and credential-like values
// are rejected rather than displayed or fuzzily classified as a paid tier.
func SafeTierDisplayName(raw string) (string, bool) {
	text, ok := safeTierText(raw)
	if !ok {
		return "", false
	}
	if tier := tierResolveExact(text); tier != TierUnknown {
		return tier.DisplayName(), true
	}
	return text, true
}

func safeTierText(raw string) (string, bool) {
	if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 80 {
		return "", false
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", false
		}
	}
	text := strings.TrimSpace(raw)
	if text == "" || strings.ContainsAny(text, "{}[]<>/\\=@\"`") || strings.Count(text, ".") >= 2 || (strings.Contains(text, ":") && subscriptionNamedTier(text) == TierUnknown) {
		return "", false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "token") || strings.Contains(lower, "bearer") || strings.Contains(lower, "secret") || strings.HasPrefix(lower, "sk-") {
		return "", false
	}
	return text, true
}
