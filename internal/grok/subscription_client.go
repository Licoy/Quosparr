package grok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/licoy/quosparr/internal/netx"
)

const subscriptionsURL = "https://grok.com/rest/subscriptions"

var errSubscriptionIdentity = errors.New("identity mismatch")
var errSubscriptionMismatch = errors.New("subscription mismatch")
var errSubscriptionResponse = errors.New("invalid response")

type subscriptionUser struct {
	UserID string `json:"userId"`
	Tier   string `json:"subscriptionTier"`
}

type subscriptionEntry struct {
	SubscriptionInfo
	Status string `json:"status"`
	UserID string `json:"xaiUserId"`
}

func (c *BillingClient) asyncSubscription(ctx context.Context, token, version string) <-chan subscriptionResult {
	ch := make(chan subscriptionResult, 1)
	go func() {
		info, err := c.fetchSubscription(ctx, token, version)
		ch <- subscriptionResult{info, err}
	}()
	return ch
}

type subscriptionResult struct {
	info *SubscriptionInfo
	err  error
}

func (c *BillingClient) fetchSubscription(ctx context.Context, token, version string) (*SubscriptionInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, billingTimeout)
	defer cancel()
	data, err := c.subscriptionGET(ctx, c.base()+"/user?include=subscription", token, version)
	if err != nil {
		return nil, fmt.Errorf("user: %w", err)
	}
	var user subscriptionUser
	if json.Unmarshal(data, &user) != nil || strings.TrimSpace(user.UserID) == "" {
		return nil, fmt.Errorf("user: %w", errSubscriptionResponse)
	}
	u := subscriptionsURL
	if c != nil && c.SubscriptionsURL != "" {
		u = c.SubscriptionsURL
	}
	data, err = c.subscriptionGET(ctx, u, token, version)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: %w", err)
	}
	var response struct {
		Subscriptions []subscriptionEntry `json:"subscriptions"`
	}
	if json.Unmarshal(data, &response) != nil || response.Subscriptions == nil {
		return nil, fmt.Errorf("subscriptions: %w", errSubscriptionResponse)
	}
	info, err := selectSubscription(response.Subscriptions, user)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: %w", err)
	}
	return info, nil
}

func selectSubscription(entries []subscriptionEntry, user subscriptionUser) (*SubscriptionInfo, error) {
	var selected *SubscriptionInfo
	var best int
	for _, entry := range entries {
		if entry.Status != "SUBSCRIPTION_STATUS_ACTIVE" {
			continue
		}
		if entry.UserID != user.UserID {
			return nil, errSubscriptionIdentity
		}
		rank := subscriptionRank(subscriptionProtoTier(entry.Tier))
		if rank == 0 {
			switch subscriptionProtoTier(entry.Tier) {
			case TierXBasic, TierXPremium, TierXPremiumPlus:
				continue
			default:
				return nil, errSubscriptionMismatch
			}
		}
		if rank > best || (rank == best && entry.BillingSystem == "BILLING_SYSTEM_COMMERCE") {
			value := entry.SubscriptionInfo
			selected, best = &value, rank
		}
	}
	rank := subscriptionUserRank(user.Tier)
	if selected == nil {
		switch user.Tier {
		case "", "Free", "XBasic", "XPremium", "XPremiumPlus":
			return nil, nil
		default:
			return nil, errSubscriptionMismatch
		}
	}
	if rank == 0 || rank != best {
		return nil, errSubscriptionMismatch
	}
	if _, ok := selected.DisplayName(); !ok {
		return nil, errSubscriptionMismatch
	}
	return selected, nil
}

func subscriptionUserRank(tier string) int {
	switch tier {
	case "SuperGrokPro":
		return 4
	case "SuperGrokPlus":
		return 3
	case "GrokPro", "SuperGrok":
		return 2
	case "SuperGrokLite":
		return 1
	}
	return 0
}

func subscriptionRank(tier TierKind) int {
	switch tier {
	case TierSuperGrokHeavy:
		return 4
	case TierSuperGrokPlus:
		return 3
	case TierSuperGrok:
		return 2
	case TierSuperGrokLite:
		return 1
	}
	return 0
}

// subscriptionGET never sends cookies or follows redirects carrying credentials
func (c *BillingClient) subscriptionGET(ctx context.Context, rawURL, token, version string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, billingTimeout)
	defer cancel()
	var client *http.Client
	if c != nil && c.HTTP != nil {
		client = c.HTTP
	} else {
		var err error
		client, err = netx.ClientFor(ctx, netx.WithoutCookies)
		if err != nil {
			return nil, errors.New("request failed")
		}
	}
	copy := *client
	copy.Jar = nil
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("request failed")
	}
	applyIdentity(req.Header, token, version)
	req.Header.Set("Accept", "application/json")
	response, err := copy.Do(req)
	if err != nil {
		return nil, subscriptionTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, billingMaxBodyBytes+1))
	if err != nil {
		return nil, subscriptionTransportError(err)
	}
	if len(data) > billingMaxBodyBytes {
		return nil, errSubscriptionResponse
	}
	return data, nil
}

func subscriptionTransportError(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("timeout")
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return errors.New("timeout")
	}
	return errors.New("request failed")
}

func subscriptionErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, stage := range []string{"user: ", "subscriptions: "} {
		if strings.HasPrefix(text, stage) {
			return stage + subscriptionErrorText(errors.New(strings.TrimPrefix(text, stage)))
		}
	}
	switch text {
	case "request failed", "timeout", "canceled", "authentication required", "invalid response", "identity mismatch", "subscription mismatch":
		return text
	}
	if strings.HasPrefix(text, "HTTP ") {
		if code, err := strconv.Atoi(strings.TrimPrefix(text, "HTTP ")); err == nil && code >= 100 && code <= 599 {
			return text
		}
	}
	return "request failed"
}

func mergeSubscription(s BillingSnapshot, result subscriptionResult) BillingSnapshot {
	s.SubscriptionError = subscriptionErrorText(result.err)
	if result.err != nil || result.info == nil {
		return s
	}
	s.Subscription = result.info
	if name, ok := result.info.DisplayName(); ok {
		s.Plan = &name
	}
	return s
}
