package grok

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionGenerations(t *testing.T) {
	for _, test := range []struct {
		proto, legacy, commerce string
		tier                    TierKind
	}{
		{"SUPER_GROK_LITE", "SuperGrok Lite", "X · Plus", TierXSubscriptionPlus},
		{"GROK_PRO", "SuperGrok", "X · Premium", TierXSubscriptionPremium},
		{"SUPER_GROK_PLUS", "SuperGrok Plus", "X · Super", TierXSubscriptionSuper},
		{"SUPER_GROK_PRO", "SuperGrok Heavy", "X · Ultra", TierXSubscriptionUltra},
	} {
		for _, system := range []string{"LEGACY", "COMMERCE", ""} {
			info := SubscriptionInfo{Tier: "SUBSCRIPTION_TIER_" + test.proto}
			want := info.Tier
			if system != "" {
				info.BillingSystem = "BILLING_SYSTEM_" + system
			}
			if system == "LEGACY" {
				want = test.legacy
			}
			if system == "COMMERCE" {
				want = test.commerce
			}
			if system == "" {
				if _, ok := info.DisplayName(); ok {
					t.Error("missing generation displayed")
				}
				continue
			}
			if got, ok := info.DisplayName(); !ok || got != want {
				t.Errorf("%+v = %q %v, want %q", info, got, ok, want)
			}
		}
		for _, alias := range []string{test.commerce, strings.TrimPrefix(test.commerce, "X · "), "xpass:" + strings.ToLower(strings.TrimPrefix(test.commerce, "X · "))} {
			if got := TierResolve(alias); got != test.tier {
				t.Errorf("%s = %v, want %v", alias, got, test.tier)
			}
			if got, ok := SafeTierDisplayName(alias); !ok || got != test.commerce {
				t.Errorf("safe %s = %q %v", alias, got, ok)
			}
		}
		if got := TierResolve(test.legacy).DisplayName(); got != test.legacy {
			t.Errorf("legacy %s = %s", test.legacy, got)
		}
	}
	for _, alias := range []struct{ raw, want string }{{"GrokPro", "SuperGrok"}, {"SuperGrokPro", "SuperGrok Heavy"}, {"supergrok_lite", "SuperGrok Lite"}, {"x_premium", "X Premium"}} {
		if got, ok := SafeTierDisplayName(alias.raw); !ok || got != alias.want {
			t.Errorf("legacy alias %q = %q %v", alias.raw, got, ok)
		}
	}
	for _, name := range []string{"X Basic", "X Premium", "X Premium+"} {
		if got := TierResolve(name).DisplayName(); got != name {
			t.Errorf("legacy %s = %s", name, got)
		}
	}
	if TierFromJWTClaim(8) != TierUnknown {
		t.Fatal("new JWT number guessed")
	}
}

func TestSafeTierDisplayName(t *testing.T) {
	for _, value := range []string{"Enterprise 2027", "future premium tier", "Tier 42", "SUBSCRIPTION_TIER_NEW"} {
		if got, ok := SafeTierDisplayName(value); !ok || got != value {
			t.Errorf("unknown %q = %q %v", value, got, ok)
		}
	}
	for _, value := range []string{"", "\nFree", "SuperGrok\x1b", "X\u202ePremium", strings.Repeat("x", 81), `{"tier":"Plus"}`, "https://x.example?t=secret", "eyJhbGciOiJIUzI1NiJ9.payload.signature", "sk-secret", "Bearer abc", "access_token", "abc@example.com"} {
		if got, ok := SafeTierDisplayName(value); ok {
			t.Errorf("unsafe input displayed as %q", got)
		}
	}
}

func TestSelectSubscription(t *testing.T) {
	entry := func(tier, system, status, user string) subscriptionEntry {
		return subscriptionEntry{SubscriptionInfo: SubscriptionInfo{"SUBSCRIPTION_TIER_" + tier, "BILLING_SYSTEM_" + system}, Status: "SUBSCRIPTION_STATUS_" + status, UserID: user}
	}
	heavy := entry("SUPER_GROK_PRO", "LEGACY", "ACTIVE", "a")
	commerce := entry("SUPER_GROK_PRO", "COMMERCE", "ACTIVE", "a")
	for _, tc := range []struct {
		name string
		rows []subscriptionEntry
		user subscriptionUser
		want string
		err  error
	}{
		{"legacy with old X", []subscriptionEntry{entry("X_PREMIUM", "COMMERCE", "ACTIVE", "a"), heavy}, subscriptionUser{"a", "SuperGrokPro"}, "SuperGrok Heavy", nil},
		{"same tier commerce", []subscriptionEntry{commerce, heavy}, subscriptionUser{"a", "SuperGrokPro"}, "X · Ultra", nil},
		{"same tier commerce reverse", []subscriptionEntry{heavy, commerce}, subscriptionUser{"a", "SuperGrokPro"}, "X · Ultra", nil},
		{"inactive excluded", []subscriptionEntry{entry("SUPER_GROK_PRO", "COMMERCE", "INACTIVE", "a"), entry("GROK_PRO", "LEGACY", "ACTIVE", "a")}, subscriptionUser{"a", "GrokPro"}, "SuperGrok", nil},
		{"wrong user", []subscriptionEntry{heavy}, subscriptionUser{"b", "SuperGrokPro"}, "", errSubscriptionIdentity},
		{"missing identity", []subscriptionEntry{entry("SUPER_GROK_PRO", "LEGACY", "ACTIVE", "")}, subscriptionUser{"a", "SuperGrokPro"}, "", errSubscriptionIdentity},
		{"mismatched effective tier", []subscriptionEntry{heavy}, subscriptionUser{"a", "GrokPro"}, "", errSubscriptionMismatch},
		{"unknown current tier", []subscriptionEntry{heavy}, subscriptionUser{"a", "Future"}, "", errSubscriptionMismatch},
		{"missing current tier", []subscriptionEntry{heavy}, subscriptionUser{"a", ""}, "", errSubscriptionMismatch},
		{"unknown active beside known", []subscriptionEntry{heavy, entry("FUTURE", "COMMERCE", "ACTIVE", "a")}, subscriptionUser{"a", "SuperGrokPro"}, "", errSubscriptionMismatch},
		{"missing billing system", []subscriptionEntry{entry("SUPER_GROK_PRO", "", "ACTIVE", "a")}, subscriptionUser{"a", "SuperGrokPro"}, "", errSubscriptionMismatch},
		{"SuperGrok user label", []subscriptionEntry{entry("GROK_PRO", "LEGACY", "ACTIVE", "a")}, subscriptionUser{"a", "SuperGrok"}, "SuperGrok", nil},
		{"empty", nil, subscriptionUser{"a", ""}, "", nil},
		{"free empty", nil, subscriptionUser{"a", "Free"}, "", nil},
		{"paid empty", nil, subscriptionUser{"a", "SuperGrokPro"}, "", errSubscriptionMismatch},
		{"paid inactive", []subscriptionEntry{entry("SUPER_GROK_PRO", "LEGACY", "INACTIVE", "a")}, subscriptionUser{"a", "SuperGrokPro"}, "", errSubscriptionMismatch},
		{"paid only old X", []subscriptionEntry{entry("X_PREMIUM", "LEGACY", "ACTIVE", "a")}, subscriptionUser{"a", "SuperGrokPro"}, "", errSubscriptionMismatch},
		{"unknown current empty", nil, subscriptionUser{"a", "Future"}, "", errSubscriptionMismatch},
		{"only old X", []subscriptionEntry{entry("X_PREMIUM", "COMMERCE", "ACTIVE", "a")}, subscriptionUser{"a", ""}, "", nil},
		{"old X Basic user", []subscriptionEntry{entry("X_BASIC", "LEGACY", "ACTIVE", "a")}, subscriptionUser{"a", "XBasic"}, "", nil},
		{"old X Premium user", []subscriptionEntry{entry("X_PREMIUM", "LEGACY", "ACTIVE", "a")}, subscriptionUser{"a", "XPremium"}, "", nil},
		{"old X Premium Plus user", []subscriptionEntry{entry("X_PREMIUM_PLUS", "LEGACY", "ACTIVE", "a")}, subscriptionUser{"a", "XPremiumPlus"}, "", nil},
		{"unknown tier", []subscriptionEntry{entry("FUTURE", "COMMERCE", "ACTIVE", "a")}, subscriptionUser{"a", ""}, "", errSubscriptionMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectSubscription(tc.rows, tc.user)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err %v, want %v", err, tc.err)
			}
			name := ""
			if got != nil {
				name, _ = got.DisplayName()
			}
			if name != tc.want {
				t.Errorf("name %q, want %q", name, tc.want)
			}
		})
	}
}

func TestSubscriptionSnapshotRoundTripAndPriority(t *testing.T) {
	old := "SuperGrok Heavy"
	s := BillingSnapshot{Plan: &old, Source: BillingSourceCurrent, UpdatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	raw, err := s.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseBillingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Subscription != nil || decoded.SubscriptionError != "" || *decoded.Plan != old {
		t.Fatal("legacy cache upgraded")
	}
	info := &SubscriptionInfo{"SUBSCRIPTION_TIER_SUPER_GROK_PRO", "BILLING_SYSTEM_COMMERCE"}
	s = mergeSubscription(s, subscriptionResult{info: info})
	if *s.Plan != "X · Ultra" {
		t.Fatalf("metadata not authoritative: %s", *s.Plan)
	}
	s = mergeSubscription(s, subscriptionResult{err: errors.New("HTTP 403")})
	raw, err = s.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = ParseBillingSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Subscription == nil || *decoded.Subscription != *info || decoded.SubscriptionError != "HTTP 403" {
		t.Fatalf("roundtrip %+v", decoded)
	}
	if subscriptionErrorText(errors.New("secret body")) != "request failed" {
		t.Fatal("raw error escaped")
	}
}

func TestRawPlanDoesNotLoseUnknownTier(t *testing.T) {
	s := BillingSnapshot{Plan: rawBillingPlan([]byte(`{"subscriptionTier":"Future Premium"}`))}
	if s.Plan == nil || *s.Plan != "Future Premium" {
		t.Fatal("billing label lost")
	}
	s = mergePlan(s, billingPart{data: []byte(`{"subscription_tier_display":"Premium"}`)}, syntheticTierToken(5))
	if s.Plan == nil || *s.Plan != "X · Premium" {
		t.Fatal("settings priority lost")
	}
}
