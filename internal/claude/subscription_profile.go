package claude

import (
	"context"
	"encoding/json"
)

// subscriptionFromProfile uses the same organization fields as Claude Code.
// Unknown organization types stay unknown rather than guessing a paid plan.
func subscriptionFromProfile(raw []byte) (string, string, error) {
	var profile struct {
		Organization struct {
			Type string `json:"organization_type"`
			Tier string `json:"rate_limit_tier"`
		} `json:"organization"`
	}
	if json.Unmarshal(raw, &profile) != nil {
		return "", "", ErrUsageParse
	}
	plans := map[string]string{"claude_pro": "pro", "claude_max": "max", "claude_team": "team", "claude_enterprise": "enterprise"}
	return plans[profile.Organization.Type], profile.Organization.Tier, nil
}

func (m *Module) completeSubscription(ctx context.Context, id string, doc AuthDocument) error {
	acc, err := m.reload(id)
	if err != nil {
		return err
	}
	if acc.SubscriptionType != "" && (acc.Tier() != TierMax || acc.RateLimitTier != "") {
		return nil
	}
	raw, _, err := m.Usage.FetchProfile(ctx, doc.OAuth().AccessToken)
	if err != nil {
		return err
	}
	identity, err := IdentityFromProfile(raw)
	if err != nil || identity.AccountUUID == "" || identity.OrganizationUUID == "" || identity.StableID() != id {
		return classifiedError(StageReadIdentity, KindInvalidData, ErrIdentityRequired)
	}
	subscription, tier, err := subscriptionFromProfile(raw)
	if err != nil {
		return classifiedError(StageReadIdentity, KindInvalidData, err)
	}
	// A profile without a recognized plan must not turn a tier string into Max.
	if subscription == "" {
		return nil
	}
	acc.SubscriptionType, acc.RateLimitTier = subscription, tier
	return m.Store.UpdateMetadata(acc)
}
