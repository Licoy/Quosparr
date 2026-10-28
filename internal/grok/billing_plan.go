package grok

func rawBillingPlan(responses ...[]byte) *string {
	for _, data := range responses {
		if response, err := parseBillingResponse(data); err == nil && response.hasTier {
			if display, ok := SafeTierDisplayName(response.tier); ok {
				return &display
			}
		}
	}
	return nil
}

func settingsRawPlan(data []byte) (string, bool) {
	root, err := parseLooseJSON(data)
	if err != nil || root.kind != jsonObject {
		return "", false
	}
	for _, key := range []string{"subscription_tier_display", "subscriptionTierDisplay", "subscription_tier", "subscriptionTier"} {
		if raw, ok := root.get(key).str(); ok {
			if display, ok := SafeTierDisplayName(raw); ok {
				return display, true
			}
		}
	}
	return "", false
}

func mergePlan(snap BillingSnapshot, settings billingPart, token string) BillingSnapshot {
	if settings.err == nil {
		if plan, ok := settingsRawPlan(settings.data); ok {
			snap.Plan = &plan
			return snap
		}
	}
	if snap.Plan != nil {
		if display, ok := SafeTierDisplayName(*snap.Plan); ok {
			snap.Plan = &display
			return snap
		}
		snap.Plan = nil
	}
	// JWT claims keep their established legacy mapping when metadata is absent
	if name, ok := TierDisplayNameFromAccessToken(token); ok {
		snap.Plan = &name
	}
	return snap
}
