package resettoday

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/licoy/quosparr/internal/export"
)

// Reaction service constants of CodexFeed.
const (
	ReactionCookieName = export.ReactionCookieName
	ReactionOrigin     = "https://didcodexreset.com"
	ReactionReferer    = "https://didcodexreset.com/"
	ReactionURL        = "https://didcodexreset.com/api/reaction"
	DefaultPollMs      = 5_000
)

// ReactionPolarity is which answer a reaction button belongs to.
type ReactionPolarity string

// Polarities.
const (
	PolarityYes ReactionPolarity = "yes"
	PolarityNo  ReactionPolarity = "no"
)

// ReactionError is a reaction fetch failure.
type ReactionError string

func (e ReactionError) Error() string { return "reaction: " + string(e) }

// Reaction errors.
const (
	ErrReactionDisabled       ReactionError = "disabled"
	ErrReactionForbidden      ReactionError = "forbidden"
	ErrReactionRateLimited    ReactionError = "rate limited"
	ErrReactionUnavailable    ReactionError = "unavailable"
	ErrReactionInvalidPayload ReactionError = "invalid payload"
)

// ReactionSnapshot is the hosted reaction counter (the Swift
// RateLimitResetTodayReactionSnapshot).
type ReactionSnapshot struct {
	Enabled    bool
	Ready      bool
	Polarity   *ReactionPolarity
	EpochID    *string
	Seed       *int
	Count      *int
	Remaining  *int
	DailyLimit int
	PollMs     int
}

// NewReactionSnapshot clamps pollMs like the Swift initializer.
func NewReactionSnapshot(s ReactionSnapshot) ReactionSnapshot {
	s.PollMs = ClampedPollMs(s.PollMs)
	return s
}

// DisabledReaction is the snapshot of a service that is switched off.
func DisabledReaction() ReactionSnapshot {
	return ReactionSnapshot{DailyLimit: 0, PollMs: DefaultPollMs}
}

// IsVisible: the button is shown.
func (s ReactionSnapshot) IsVisible() bool {
	return s.Enabled && s.Ready && s.Polarity != nil && s.Count != nil
}

// IsExhausted: the daily limit is used up.
func (s ReactionSnapshot) IsExhausted() bool { return IsExhausted(s.Remaining, s.DailyLimit) }

// PollInterval is the wait between polls.
func (s ReactionSnapshot) PollInterval() float64 { return float64(s.PollMs) / 1000 }

// ReactionDevMock is the fixture for a dev mock kind.
func ReactionDevMock(kind DevMockKind, count int) ReactionSnapshot {
	polarity := PolarityNo
	if kind == MockYes {
		polarity = PolarityYes
	}
	epoch := "dev-mock-epoch"
	seed, c := count, count
	return ReactionSnapshot{
		Enabled: true, Ready: true, Polarity: &polarity, EpochID: &epoch,
		Seed: &seed, Count: &c, PollMs: DefaultPollMs,
	}
}

// ReactionLocal is the state before an optimistic click.
type ReactionLocal struct {
	EpochID           *string
	PreviousCount     int
	PreviousRemaining *int
}

// ReactionPostResult is the decoded answer of a click.
type ReactionPostResult struct {
	OK    bool
	Error *string
	Data  *ReactionSnapshot
}

// ReactionReconcile is the state to keep after a click.
type ReactionReconcile struct {
	Count     int
	Remaining *int
	Exhausted bool
	EpochID   *string
	Payload   *ReactionSnapshot
}

// ClampedPollMs keeps the poll interval within 1 to 10 seconds.
func ClampedPollMs(v int) int { return min(max(v, 1_000), 10_000) }

// IsExhausted: a positive daily limit with nothing remaining.
func IsExhausted(remaining *int, dailyLimit int) bool {
	if dailyLimit <= 0 || remaining == nil {
		return false
	}
	return *remaining <= 0
}

// PositiveDelta is how much the count grew within one epoch, never negative.
func PositiveDelta(previousCount, nextCount *int, previousEpoch, nextEpoch *string) int {
	if previousEpoch == nil || nextEpoch == nil || *previousEpoch != *nextEpoch {
		return 0
	}
	if previousCount == nil || nextCount == nil {
		return 0
	}
	if d := *nextCount - *previousCount; d > 0 {
		return d
	}
	return 0
}

// OptimisticClick counts the click locally before the post returns.
func OptimisticClick(s ReactionSnapshot) ReactionSnapshot {
	next := s
	count := 1
	if s.Count != nil {
		count = *s.Count + 1
	}
	next.Count = &count
	if s.DailyLimit > 0 && s.Remaining != nil {
		r := max(0, *s.Remaining-1)
		next.Remaining = &r
	}
	return next
}

// ReconcileAfterPost merges the post answer with the local optimistic state. A
// failed or unusable answer rolls the count back.
func ReconcileAfterPost(local ReactionLocal, resp *ReactionPostResult) ReactionReconcile {
	if resp == nil || !resp.OK || resp.Data == nil || !resp.Data.Ready {
		out := ReactionReconcile{
			Count: local.PreviousCount, Remaining: local.PreviousRemaining, EpochID: local.EpochID,
		}
		if resp != nil {
			out.Exhausted = resp.Error != nil && *resp.Error == "daily_limit"
			out.Payload = resp.Data
		}
		return out
	}
	d := resp.Data
	out := ReactionReconcile{
		Count: local.PreviousCount, Remaining: local.PreviousRemaining,
		Exhausted: IsExhausted(d.Remaining, d.DailyLimit), EpochID: d.EpochID, Payload: d,
	}
	if d.Count != nil {
		out.Count = *d.Count
	}
	if d.Remaining != nil {
		out.Remaining = d.Remaining
	}
	return out
}

// VisitorIDFromResponse reads the visitor ID the service minted from the
// response's hr_react cookie.
func VisitorIDFromResponse(resp *http.Response) (string, bool) {
	for _, c := range resp.Cookies() {
		if c.Name == ReactionCookieName {
			return export.ParseVisitorID(c.Value)
		}
	}
	for _, h := range resp.Header.Values("Set-Cookie") {
		if id, ok := export.VisitorIDFromSetCookie(h); ok {
			return id, true
		}
	}
	return "", false
}

// DecodeReactionEnvelope parses {"ok":..,"error":..,"data":{..}}.
func DecodeReactionEnvelope(data []byte) (ReactionPostResult, error) {
	o, err := parseObject(data, "reaction envelope")
	if err != nil {
		return ReactionPostResult{}, err
	}
	var out ReactionPostResult
	if out.OK, err = o.boolean("ok"); err != nil {
		return out, err
	}
	if out.Error, err = o.optStr("error"); err != nil {
		return out, err
	}
	if raw, ok := o.optional("data"); ok {
		payload, err := parseObject(raw, "data")
		if err != nil {
			return out, err
		}
		snap, err := decodeReactionPayload(payload)
		if err != nil {
			return out, err
		}
		out.Data = &snap
	}
	return out, nil
}

func (o obj) boolean(key string) (bool, error) {
	raw, err := o.required(key)
	if err != nil {
		return false, err
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, invalid("%s must be a boolean.", key)
	}
	return b, nil
}

func decodeInt(raw json.RawMessage, key string) (int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] == '"' {
		return 0, invalid("%s must be an integer.", key)
	}
	var f float64
	if err := json.Unmarshal(trimmed, &f); err != nil || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, invalid("%s must be an integer.", key)
	}
	return int(f), nil
}

func (o obj) optInt(key string) (*int, error) {
	raw, ok := o.optional(key)
	if !ok {
		return nil, nil
	}
	v, err := decodeInt(raw, key)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func decodeReactionPayload(o obj) (ReactionSnapshot, error) {
	var s ReactionSnapshot
	var err error
	if s.Enabled, err = o.boolean("enabled"); err != nil {
		return s, err
	}
	if s.Ready, err = o.boolean("ready"); err != nil {
		return s, err
	}
	polarity, err := o.optStr("polarity")
	if err != nil {
		return s, err
	}
	if polarity != nil {
		if p := ReactionPolarity(*polarity); p == PolarityYes || p == PolarityNo {
			s.Polarity = &p
		}
	}
	if s.EpochID, err = o.optStr("epochId"); err != nil {
		return s, err
	}
	if s.Seed, err = o.optInt("seed"); err != nil {
		return s, err
	}
	if s.Count, err = o.optInt("count"); err != nil {
		return s, err
	}
	if s.Remaining, err = o.optInt("remaining"); err != nil {
		return s, err
	}
	raw, err := o.required("dailyLimit")
	if err != nil {
		return s, err
	}
	if s.DailyLimit, err = decodeInt(raw, "dailyLimit"); err != nil {
		return s, err
	}
	poll, err := o.optInt("pollMs")
	if err != nil {
		return s, err
	}
	s.PollMs = DefaultPollMs
	if poll != nil {
		s.PollMs = *poll
	}
	return NewReactionSnapshot(s), nil
}

// FormatCount groups thousands with commas, the decimal style of both
// supported languages (English and Simplified Chinese).
func FormatCount(count int) string {
	digits := strconv.Itoa(count)
	sign := ""
	if count < 0 {
		sign, digits = "-", digits[1:]
	}
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return sign + b.String()
}

// FormatDelta is "+N" for a positive delta and "" otherwise.
func FormatDelta(delta int) string {
	if delta <= 0 {
		return ""
	}
	return "+" + FormatCount(delta)
}
