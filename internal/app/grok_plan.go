package app

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/grok"
	"github.com/licoy/quosparr/internal/l10n"
)

type grokPlanInfo struct {
	plan, billingSystem string
	compact             bool
}

// All three account surfaces use the same safe label and product explanation
func grokPlanBadgeInfo(c *ui.Context, l l10n.Localizer, info grokPlanInfo) {
	label, ok := grok.SafeTierDisplayName(info.plan)
	look := plainLook(c.Theme())
	if !ok {
		label = l.Text(l10n.KeyProviderGrok)
	} else if tier := grok.TierResolve(label); tier.DisplayName() == label {
		look = grokTierLook(c.Theme(), tier)
	}
	vertical, horizontal := float32(3), float32(7)
	if info.compact {
		vertical, horizontal = 1, 5
	}
	tip := label
	switch label {
	case grok.TierXSubscriptionPlus.DisplayName(), grok.TierXSubscriptionPremium.DisplayName(), grok.TierXSubscriptionSuper.DisplayName(), grok.TierXSubscriptionUltra.DisplayName():
		tip += "\n" + tx(l, xGrokXSubscription)
	default:
		if info.billingSystem == "BILLING_SYSTEM_LEGACY" {
			tip += "\n" + tx(l, xGrokLegacySubscription)
		}
	}
	tierBadgePadded(c, look, label, vertical, horizontal).MaxWidth(200).Tooltip(tip)
}

func grokPlanNotice(c *ui.Context, l l10n.Localizer, reason string) {
	if reason == "" {
		return
	}
	text := tx(l, xGrokPlanUnavailable)
	dimText(c, text).MaxLines(2).Tooltip(text + "\n" + grokPlanErrorText(l, reason))
}

// Cached diagnostics are untrusted too, only fixed categories and HTTP status
// codes may appear in a tooltip, never upstream errors or response bodies
func safeGrokPlanError(reason string) string {
	stage := ""
	if prefix, category, ok := strings.Cut(reason, ": "); ok && (prefix == "user" || prefix == "subscriptions") {
		stage, reason = prefix+": ", category
	}
	switch reason {
	case "request failed", "timeout", "canceled", "authentication required", "invalid response", "identity mismatch", "subscription mismatch":
		return stage + reason
	}
	if code, err := strconv.Atoi(strings.TrimPrefix(reason, "HTTP ")); err == nil && code >= 100 && code <= 599 && reason == "HTTP "+strconv.Itoa(code) {
		return stage + reason
	}
	return stage + "request failed"
}

func grokPlanProjection(snap grok.BillingSnapshot) (plan, billingSystem, planError string) {
	if snap.Plan != nil {
		plan, _ = grok.SafeTierDisplayName(*snap.Plan)
	}
	if snap.Subscription != nil {
		// Preserve the explicit metadata result, unknown billing systems do
		// not establish a legacy or Commerce subscription
		if name, ok := snap.Subscription.DisplayName(); ok {
			plan, _ = grok.SafeTierDisplayName(name)
		}
		switch snap.Subscription.BillingSystem {
		case "BILLING_SYSTEM_LEGACY", "BILLING_SYSTEM_COMMERCE":
			billingSystem = snap.Subscription.BillingSystem
		}
	}
	if snap.SubscriptionError != "" {
		planError = safeGrokPlanError(snap.SubscriptionError)
	}
	return
}

func grokPlanErrorText(l l10n.Localizer, reason string) string {
	reason = safeGrokPlanError(reason)
	stage := ""
	if prefix, category, ok := strings.Cut(reason, ": "); ok {
		reason = category
		stage = tx(l, pick(prefix == "user", xGrokPlanIdentityStage, xGrokPlanSubscriptionStage)) + ": "
	}
	keys := map[string]textKey{
		"request failed":          xGrokPlanRequestFailed,
		"timeout":                 xGrokPlanTimeout,
		"canceled":                xGrokPlanCanceled,
		"authentication required": xGrokPlanAuthentication,
		"invalid response":        xGrokPlanInvalidResponse,
		"identity mismatch":       xGrokPlanIdentityMismatch,
		"subscription mismatch":   xGrokPlanSubscriptionMismatch,
	}
	if key, ok := keys[reason]; ok {
		return stage + tx(l, key)
	}
	return stage + tx(l, xGrokPlanHTTPStatus, strings.TrimPrefix(reason, "HTTP "))
}
