package codexapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ErrDecode marks a response body that does not have the expected shape.
var ErrDecode = errors.New("unexpected response format")

func decodeErrorf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDecode, fmt.Sprintf(format, args...))
}

// DecodeQuota decodes the wham/usage response.
func DecodeQuota(data []byte, now time.Time) (QuotaSnapshot, error) {
	var resp struct {
		PlanType         *string         `json:"plan_type"`
		RateLimit        *rawRateLimit   `json:"rate_limit"`
		Credits          json.RawMessage `json:"credits"`
		AdditionalLimits json.RawMessage `json:"additional_rate_limits"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return QuotaSnapshot{}, decodeErrorf("usage: %v", err)
	}
	if resp.RateLimit == nil || resp.RateLimit.Primary == nil {
		return QuotaSnapshot{}, decodeErrorf("usage: rate_limit.primary_window is missing")
	}
	primary, err := resp.RateLimit.Primary.window()
	if err != nil {
		return QuotaSnapshot{}, err
	}
	snapshot := QuotaSnapshot{Primary: primary, UpdatedAt: now}
	if resp.PlanType != nil {
		snapshot.Plan = *resp.PlanType
	}
	if resp.RateLimit.Secondary != nil {
		secondary, err := resp.RateLimit.Secondary.window()
		if err != nil {
			return QuotaSnapshot{}, err
		}
		snapshot.Secondary = &secondary
	}
	snapshot.CreditsBalance = decodeCreditsBalance(resp.Credits)
	snapshot.AdditionalWindows = decodeAdditionalLimits(resp.AdditionalLimits)
	return snapshot, nil
}

type rawRateLimit struct {
	Primary   *rawWindow `json:"primary_window"`
	Secondary *rawWindow `json:"secondary_window"`
}

type rawWindow struct {
	UsedPercent        json.RawMessage `json:"used_percent"`
	ResetAt            json.RawMessage `json:"reset_at"`
	LimitWindowSeconds json.RawMessage `json:"limit_window_seconds"`
}

func (w *rawWindow) window() (RateWindow, error) {
	exact, ok := flexibleFloat(w.UsedPercent)
	if !ok {
		return RateWindow{}, decodeErrorf("usage: used_percent is not a finite number")
	}
	resetAt, ok := wholeNumber(w.ResetAt)
	if !ok {
		return RateWindow{}, decodeErrorf("usage: reset_at is not an integer")
	}
	seconds, ok := wholeNumber(w.LimitWindowSeconds)
	if !ok {
		return RateWindow{}, decodeErrorf("usage: limit_window_seconds is not an integer")
	}
	window := RateWindow{UsedPercent: int(math.Round(exact)), UsedPercentExact: exact}
	if seconds > 0 {
		window.WindowMinutes = int(seconds / 60)
	}
	if resetAt > 0 {
		window.ResetsAt = time.Unix(resetAt, 0).UTC()
	}
	return window, nil
}

// flexibleFloat accepts a JSON number or a numeric string, finite only.
func flexibleFloat(raw json.RawMessage) (float64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, false
	}
	if strings.HasPrefix(text, `"`) {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return 0, false
		}
		text = s
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// wholeNumber accepts a JSON integer (an integral float such as 1.0 as well).
func wholeNumber(raw json.RawMessage) (int64, bool) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || strings.HasPrefix(text, `"`) {
		return 0, false
	}
	if n, err := strconv.ParseInt(text, 10, 64); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<62 {
		return int64(f), true
	}
	return 0, false
}

func decodeCreditsBalance(raw json.RawMessage) *float64 {
	var credits struct {
		Balance json.RawMessage `json:"balance"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &credits) != nil {
		return nil
	}
	if v, ok := flexibleFloat(credits.Balance); ok {
		return &v
	}
	return nil
}

// decodeAdditionalLimits is lenient like the Swift decoder: a malformed list
// is treated as empty, and entries without a window are dropped.
func decodeAdditionalLimits(raw json.RawMessage) []NamedRateWindow {
	var items []struct {
		LimitName      string        `json:"limit_name"`
		MeteredFeature string        `json:"metered_feature"`
		RateLimit      *rawRateLimit `json:"rate_limit"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var result []NamedRateWindow
	for _, item := range items {
		if item.RateLimit == nil {
			continue
		}
		source := item.RateLimit.Primary
		if source == nil {
			source = item.RateLimit.Secondary
		}
		if source == nil {
			continue
		}
		window, err := source.window()
		if err != nil {
			return nil // the whole list failed to decode in Swift
		}
		name := firstNonEmpty(item.LimitName, item.MeteredFeature)
		if name == "" {
			name = "Codex extra limit"
		}
		result = append(result, NamedRateWindow{Name: name, Window: window})
	}
	return result
}

// DecodeResetCredits decodes the wham/rate-limit-reset-credits response: an
// object with available_count and credits, or a bare list of credits.
func DecodeResetCredits(data []byte, now time.Time) (ResetCreditsSnapshot, error) {
	trimmed := bytes.TrimSpace(data)
	var items []rawCredit
	var availableCount *int
	switch {
	case bytes.HasPrefix(trimmed, []byte("[")):
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return ResetCreditsSnapshot{}, decodeErrorf("reset credits: %v", err)
		}
	default:
		var resp struct {
			AvailableCount *int            `json:"available_count"`
			Credits        json.RawMessage `json:"credits"`
		}
		if err := json.Unmarshal(trimmed, &resp); err != nil {
			return ResetCreditsSnapshot{}, decodeErrorf("reset credits: %v", err)
		}
		availableCount = resp.AvailableCount
		// A malformed credits list is ignored, as in the Swift decoder.
		if len(resp.Credits) > 0 && json.Unmarshal(resp.Credits, &items) != nil {
			items = nil
		}
	}
	credits := make([]ResetCredit, 0, len(items))
	available := 0
	for _, item := range items {
		credit := ResetCredit{
			ID:        derefString(item.ID),
			Status:    "unknown",
			CreatedAt: flexibleTime(item.CreatedAt),
			ExpiresAt: flexibleTime(item.ExpiresAt),
		}
		if item.Status != nil {
			credit.Status = *item.Status
		}
		if !credit.ExpiresAt.IsZero() {
			credit.RemainingSeconds = math.Max(0, credit.ExpiresAt.Sub(now).Seconds())
		}
		if credit.Status == "available" {
			available++
		}
		credits = append(credits, credit)
	}
	count := available
	if availableCount != nil {
		count = *availableCount
	}
	if count < 0 {
		return ResetCreditsSnapshot{}, decodeErrorf("reset credits: negative available_count")
	}
	return ResetCreditsSnapshot{AvailableCount: count, Credits: credits, UpdatedAt: now}, nil
}

type rawCredit struct {
	ID        *string         `json:"id"`
	Status    *string         `json:"status"`
	CreatedAt json.RawMessage `json:"created_at"`
	ExpiresAt json.RawMessage `json:"expires_at"`
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// flexibleTime reads epoch seconds or an ISO 8601 string; anything else is
// unknown (zero).
func flexibleTime(raw json.RawMessage) time.Time {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return time.Time{}
	}
	if strings.HasPrefix(text, `"`) {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return time.Time{}
		}
		t, _ := parseTimestamp(s)
		return t
	}
	if v, err := strconv.ParseFloat(text, 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
		return unixSeconds(v)
	}
	return time.Time{}
}

// DecodeTokenUsageProfile decodes the wham/profiles/me response.
func DecodeTokenUsageProfile(data []byte) (TokenUsageProfile, error) {
	var resp struct {
		Stats struct {
			Buckets []struct {
				StartDate string `json:"start_date"`
				Tokens    int64  `json:"tokens"`
			} `json:"daily_usage_buckets"`
		} `json:"stats"`
		Metadata *struct {
			StatsError  string `json:"stats_error"`
			StatsAsOf   string `json:"stats_as_of"`
			GeneratedAt string `json:"generated_at"`
		} `json:"metadata"`
	}
	var probe struct {
		Stats map[string]json.RawMessage `json:"stats"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return TokenUsageProfile{}, decodeErrorf("profile: %v", err)
	}
	_ = json.Unmarshal(data, &probe)
	if raw, ok := probe.Stats["daily_usage_buckets"]; !ok || string(bytes.TrimSpace(raw)) == "null" {
		if resp.Metadata != nil && strings.TrimSpace(resp.Metadata.StatsError) != "" {
			return TokenUsageProfile{}, decodeErrorf("Codex profile stats are unavailable")
		}
		return TokenUsageProfile{}, decodeErrorf("Codex profile daily usage is missing")
	}
	if resp.Metadata != nil && strings.TrimSpace(resp.Metadata.StatsError) != "" {
		return TokenUsageProfile{}, decodeErrorf("Codex profile stats are unavailable")
	}
	daily := make(map[string]int, len(resp.Stats.Buckets))
	for _, b := range resp.Stats.Buckets {
		if !validDayKey(b.StartDate) || b.Tokens < 0 {
			return TokenUsageProfile{}, decodeErrorf("Codex profile daily usage contains an invalid value")
		}
		if _, dup := daily[b.StartDate]; dup {
			return TokenUsageProfile{}, decodeErrorf("Codex profile daily usage contains a duplicate date")
		}
		daily[b.StartDate] = int(b.Tokens)
	}
	profile := TokenUsageProfile{DailyTokens: daily}
	if resp.Metadata != nil {
		profile.StatsAsOf = strings.TrimSpace(resp.Metadata.StatsAsOf)
		if profile.StatsAsOf != "" && !validDayKey(profile.StatsAsOf) {
			return TokenUsageProfile{}, decodeErrorf("Codex profile stats date is invalid")
		}
		if generated := strings.TrimSpace(resp.Metadata.GeneratedAt); generated != "" {
			t, ok := parseTimestamp(generated)
			if !ok {
				return TokenUsageProfile{}, decodeErrorf("Codex profile generation timestamp is invalid")
			}
			profile.GeneratedAt = t
		}
	}
	return profile, nil
}

// validDayKey reports whether s is a real calendar day written YYYY-MM-DD.
func validDayKey(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}
