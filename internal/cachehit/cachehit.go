// Package cachehit contains provider-neutral cache-hit summaries.
package cachehit

import (
	"fmt"
	"math"
	"time"
)

// Totals is the token-weighted cache-hit input of a set of samples.
type Totals struct {
	InputTokens       int64
	CachedInputTokens int64
	Samples           int
	MissingSamples    int
}

// Percent returns the cache-hit percentage when there is valid input.
func (t Totals) Percent() (float64, bool) {
	if t.InputTokens <= 0 {
		return 0, false
	}
	return float64(t.CachedInputTokens) / float64(t.InputTokens) * 100, true
}

// AddSample adds one complete sample after validating its counters.
func (t *Totals) AddSample(input, cached int64) error {
	if input < 0 || cached < 0 || cached > input {
		return fmt.Errorf("cachehit: invalid token counters")
	}
	if input > math.MaxInt64-t.InputTokens || cached > math.MaxInt64-t.CachedInputTokens || t.Samples == math.MaxInt {
		return fmt.Errorf("cachehit: counter overflow")
	}
	t.InputTokens += input
	t.CachedInputTokens += cached
	t.Samples++
	return nil
}

// AddMissing records one sample whose cache counters were incomplete.
func (t *Totals) AddMissing() error {
	if t.MissingSamples == math.MaxInt {
		return fmt.Errorf("cachehit: counter overflow")
	}
	t.MissingSamples++
	return nil
}

// Row is one model or session breakdown.
type Row struct {
	ID     string
	Name   string
	Totals Totals
}

// Report is one provider's cache-hit summary for an exact time range.
type Report struct {
	Start        time.Time
	End          time.Time
	CalculatedAt time.Time
	Totals       Totals
	Models       []Row
	Sessions     []Row
	// UnreadLines counts log lines that may hold usage but could not be read
	// (oversized or malformed), from logs modified since Start. They are
	// skipped, so a positive count marks the report as incomplete.
	UnreadLines int
}

// Clone returns a copy whose slices can be changed independently.
func (r Report) Clone() Report {
	c := r
	c.Models = append([]Row(nil), r.Models...)
	c.Sessions = append([]Row(nil), r.Sessions...)
	return c
}
