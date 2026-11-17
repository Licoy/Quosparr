package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// TierKind is the normalized plan.
type TierKind string

const (
	TierUnknown TierKind = "unknown"
	TierFree    TierKind = "free"
	TierPro     TierKind = "pro"
	TierUltra   TierKind = "ultra"
)

// Tier is the plan with the raw Cloud Code id and name.
type Tier struct {
	Kind TierKind `json:"kind"`
	ID   string   `json:"id,omitempty"`
	Name string   `json:"name,omitempty"`
}

// ResolveTier uses the paid tier, else the current tier: an id containing
// "ultra" is Ultra, "pro" is Pro, any other id Free, no id Unknown.
func ResolveTier(paidID, paidName, currentID, currentName string) Tier {
	id, name := trimSpace(paidID), trimSpace(paidName)
	if id == "" {
		id, name = trimSpace(currentID), trimSpace(currentName)
	}
	lower := strings.ToLower(id)
	kind := TierFree
	switch {
	case id == "":
		kind = TierUnknown
	case strings.Contains(lower, "ultra"):
		kind = TierUltra
	case strings.Contains(lower, "pro"):
		kind = TierPro
	}
	return Tier{Kind: kind, ID: id, Name: name}
}

// Bucket families and windows of the known quota buckets.
const (
	FamilyClaude = "claude"
	FamilyGemini = "gemini"
	FamilyArgon  = "argon"
	FamilyOther  = "other"

	WindowFiveHour = "5h"
	WindowWeekly   = "weekly"
)

// knownBuckets maps the bucket ids Cloud Code is known to send.
var knownBuckets = map[string][2]string{
	"claude:5h":               {FamilyClaude, WindowFiveHour},
	"3p-5h":                   {FamilyClaude, WindowFiveHour},
	"claude:weekly":           {FamilyClaude, WindowWeekly},
	"3p-weekly":               {FamilyClaude, WindowWeekly},
	"gemini:5h":               {FamilyGemini, WindowFiveHour},
	"gemini-5h":               {FamilyGemini, WindowFiveHour},
	"gemini:weekly":           {FamilyGemini, WindowWeekly},
	"gemini-weekly":           {FamilyGemini, WindowWeekly},
	"argon:weekly":            {FamilyArgon, WindowWeekly},
	"gemini-argon-wtu-weekly": {FamilyArgon, WindowWeekly},
}

// QuotaBucket is one quota window. RemainingFraction is 0…1; Cloud Code
// omits a zero value, so an absent fraction reads as exhausted.
type QuotaBucket struct {
	ID                string     `json:"id"`
	Group             string     `json:"group,omitempty"`
	Family            string     `json:"family"`
	Window            string     `json:"window,omitempty"`
	DisplayName       string     `json:"displayName,omitempty"`
	RemainingFraction float64    `json:"remainingFraction"`
	ResetAt           *time.Time `json:"resetAt,omitempty"`
}

// ModelQuota is one model of fetchAvailableModels. RemainingFraction is nil
// when the model reports no quota.
type ModelQuota struct {
	ID                string     `json:"id"`
	DisplayName       string     `json:"displayName,omitempty"`
	Model             string     `json:"model,omitempty"`
	Recommended       bool       `json:"recommended,omitempty"`
	RemainingFraction *float64   `json:"remainingFraction,omitempty"`
	ResetAt           *time.Time `json:"resetAt,omitempty"`
}

// Credit is one entry of paidTier.availableCredits.
type Credit struct {
	Type            string  `json:"type"`
	Amount          float64 `json:"amount"`
	MinimumForUsage float64 `json:"minimumForUsage,omitempty"`
}

// Validation is a pending browser verification or subscription request.
type Validation struct {
	Reason string `json:"reason"`
	URL    string `json:"url,omitempty"`
}

// Quota snapshot sources.
const (
	SourceSummary = "summary"
	SourceModels  = "models"
)

// QuotaSnapshot is one reading, cached as the account's CachedQuota.
type QuotaSnapshot struct {
	Tier    Tier          `json:"tier"`
	Project string        `json:"project,omitempty"`
	Source  string        `json:"source,omitempty"`
	Buckets []QuotaBucket `json:"buckets,omitempty"`
	Models  []ModelQuota  `json:"models,omitempty"`
	Credits []Credit      `json:"credits,omitempty"`
	// Validation is set when the last reading was refused with a 403
	// verification or subscription request; the rest is the previous reading.
	Validation *Validation `json:"validation,omitempty"`
	UpdatedAt  time.Time   `json:"updatedAt"`
}

// MarshalQuotaSnapshot renders s as compact JSON.
func MarshalQuotaSnapshot(s QuotaSnapshot) (json.RawMessage, error) { return json.Marshal(s) }

// ParseQuotaSnapshot decodes a cached snapshot.
func ParseQuotaSnapshot(data []byte) (QuotaSnapshot, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return QuotaSnapshot{}, ErrQuotaSnapshotJSON
	}
	var s QuotaSnapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return QuotaSnapshot{}, ErrQuotaSnapshotJSON
	}
	if s.Tier.Kind == "" {
		s.Tier.Kind = TierUnknown
	}
	return s, nil
}

// withValidation returns cached (or an empty snapshot) marked with v.
func withValidation(cached json.RawMessage, v Validation, now time.Time) (json.RawMessage, error) {
	s, err := ParseQuotaSnapshot(cached)
	if err != nil {
		s = QuotaSnapshot{Tier: Tier{Kind: TierUnknown}, UpdatedAt: now}
	}
	s.Validation = &v
	return MarshalQuotaSnapshot(s)
}

// bucketFrom classifies a bucket id. Unknown ids keep their display name
// and fall into FamilyOther, with the window guessed from the id.
func bucketFrom(id, group, displayName string, fraction float64, reset *time.Time) QuotaBucket {
	b := QuotaBucket{ID: id, Group: group, DisplayName: displayName, RemainingFraction: clampFraction(fraction), ResetAt: reset}
	if known, ok := knownBuckets[strings.ToLower(id)]; ok {
		b.Family, b.Window = known[0], known[1]
		return b
	}
	lower := strings.ToLower(id)
	b.Family = FamilyOther
	switch {
	case strings.Contains(lower, "5h"):
		b.Window = WindowFiveHour
	case strings.Contains(lower, "weekly"):
		b.Window = WindowWeekly
	}
	return b
}

func clampFraction(f float64) float64 {
	switch {
	case f != f, f < 0:
		return 0
	case f > 1:
		return 1
	}
	return f
}
