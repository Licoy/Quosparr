// Package speed measures how fast the models answer, from timestamps the
// official CLIs already write into their local session logs: the time to the
// first token of each model call and the output tokens per second while it
// streams. It keeps no session text, only the derived numbers.
package speed

import "time"

const (
	// SessionLimit is how many of the newest sessions a report reads.
	SessionLimit = 10
	// Window is how far back a sample may lie to count.
	Window = 7 * 24 * time.Hour
)

// Tier is the service tier a model call ran at, as its log records it.
type Tier string

const (
	// TierUnknown is a call whose log does not say its tier.
	TierUnknown Tier = ""
	// TierStandard is the default tier.
	TierStandard Tier = "standard"
	// TierFast is the paid faster tier (Codex priority, Claude fast mode).
	TierFast Tier = "fast"
	// TierUltrafast is the Codex ultrafast tier.
	TierUltrafast Tier = "ultrafast"
)

// Sample is one measurement of a model call. Codex logs a full sample per
// response; Grok logs the first token per model call and the token count per
// turn, so its samples carry either the TTFT or the output part.
type Sample struct {
	// At is when the request was sent.
	At     time.Time
	Model  string
	Effort string // reasoning effort, "" when unknown
	Tier   Tier
	// TTFT is the wait from the request to the first streamed item, valid
	// when HasTTFT.
	TTFT    time.Duration
	HasTTFT bool
	// OutputTokens (reasoning included) streamed during Decode; both zero
	// when the sample has no output part.
	OutputTokens int64
	Decode       time.Duration
}

// Session is the samples of one conversation, with what identifies it.
type Session struct {
	ID          string
	Title       string
	ProjectName string
	UpdatedAt   time.Time
	Samples     []Sample
}

// Stat is the speed of one model at one reasoning effort and tier. Tier is
// TierUnknown for every stat of a report that is not Tiered.
type Stat struct {
	Model  string
	Effort string
	Tier   Tier
	// Calls counts the samples with a TTFT; TTFTP50 and TTFTP90 are their
	// percentiles (zero when Calls is 0).
	Calls            int
	TTFTP50, TTFTP90 time.Duration
	// OutputTokens and Decode are the sums over the output parts.
	OutputTokens int64
	Decode       time.Duration
}

// HasTPS reports whether the stat has any output to rate.
func (s Stat) HasTPS() bool { return s.OutputTokens > 0 && s.Decode > 0 }

// TPS is the output tokens per second of streaming, 0 without output.
func (s Stat) TPS() float64 {
	if !s.HasTPS() {
		return 0
	}
	return float64(s.OutputTokens) / s.Decode.Seconds()
}

// SessionStat is the per-model speed of one analysed conversation.
type SessionStat struct {
	ID          string
	Title       string
	ProjectName string
	UpdatedAt   time.Time
	// Groups are the session's model/effort pairs, most calls first.
	Groups []Stat
}

// Report is the speed of the recently analysed conversations.
type Report struct {
	// Groups merge every analysed session by model and effort, most calls
	// first.
	Groups []Stat
	// Sessions are the analysed conversations, newest first; Sessions[0] is
	// the one the card names.
	Sessions []SessionStat
	// NoTTFT marks a source whose logs have no first-token time, so the
	// views leave the TTFT and call count columns out.
	NoTTFT bool
	// Tiered marks a report that groups calls by tier because it has at least
	// one Fast or Ultrafast sample.
	Tiered bool
	// From and To bound the samples; CalculatedAt is when the report was built.
	From, To     time.Time
	CalculatedAt time.Time
}

// Empty reports whether the report has nothing to show.
func (r Report) Empty() bool { return len(r.Groups) == 0 }
