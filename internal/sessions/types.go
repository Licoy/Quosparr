package sessions

import (
	"fmt"
	"math"
	"time"

	"github.com/licoy/quosparr/internal/export/swiftjson"
)

// State is how a session looks in the recent list.
type State string

const (
	StateRecent         State = "recent"
	StateNeedsAttention State = "needsAttention"
	StateFailed         State = "failed"
)

// Totals is the token and turn count of one session, the Swift
// ApiEquivalentTotals. It is shared with the cost summary of the status file
// (package export), so the JSON names are the Swift ones. Until the usage
// package exists this is its stand-in.
type Totals struct {
	TotalTokens         int `json:"totalTokens"`
	UncachedInputTokens int `json:"uncachedInputTokens"`
	CachedInputTokens   int `json:"cachedInputTokens"`
	OutputTokens        int `json:"outputTokens"`
	Turns               int `json:"turns"`
	Threads             int `json:"threads"`
}

// ArithmeticError reports token numbers that cannot be used: a negative or
// non-integer count, cached tokens above the input tokens, or an overflow.
type ArithmeticError struct {
	Overflow bool
	Field    string
}

func (e *ArithmeticError) Error() string {
	if e.Overflow {
		return fmt.Sprintf("integer overflow in %s", e.Field)
	}
	return fmt.Sprintf("invalid value in %s", e.Field)
}

func checkedAdd(a, b int, field string) (int, error) {
	if (b > 0 && a > math.MaxInt-b) || (b < 0 && a < math.MinInt-b) {
		return 0, &ArithmeticError{Overflow: true, Field: field}
	}
	return a + b, nil
}

// Add returns t+o, failing on overflow.
func (t Totals) Add(o Totals) (Totals, error) {
	var r Totals
	var err error
	for _, f := range []struct {
		dst  *int
		a, b int
		name string
	}{
		{&r.TotalTokens, t.TotalTokens, o.TotalTokens, "total tokens"},
		{&r.UncachedInputTokens, t.UncachedInputTokens, o.UncachedInputTokens, "uncached input tokens"},
		{&r.CachedInputTokens, t.CachedInputTokens, o.CachedInputTokens, "cached input tokens"},
		{&r.OutputTokens, t.OutputTokens, o.OutputTokens, "output tokens"},
		{&r.Turns, t.Turns, o.Turns, "turns"},
		{&r.Threads, t.Threads, o.Threads, "threads"},
	} {
		if *f.dst, err = checkedAdd(f.a, f.b, f.name); err != nil {
			return Totals{}, err
		}
	}
	return r, nil
}

// NewTotals is ApiEquivalentTotals(validating:): input includes the cached
// part, as in Codex token_count events.
func NewTotals(input, cached, output, turns, threads int) (Totals, error) {
	if input < 0 || cached < 0 || output < 0 || cached > input || turns < 0 || threads < 0 {
		return Totals{}, &ArithmeticError{Field: "token usage"}
	}
	uncached := input - cached
	total, err := checkedAdd(input, output, "total tokens")
	if err != nil {
		return Totals{}, err
	}
	return Totals{
		TotalTokens:         total,
		UncachedInputTokens: uncached,
		CachedInputTokens:   cached,
		OutputTokens:        output,
		Turns:               turns,
		Threads:             threads,
	}, nil
}

// Item is one entry of the recent sessions list.
type Item struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	ProjectName    string             `json:"projectName"`
	Cwd            *string            `json:"cwd"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	State          State              `json:"state"`
	Totals         Totals             `json:"totals"`
	EstimatedUSD   *swiftjson.Decimal `json:"estimatedUSD"`
	SourcePath     string             `json:"-"`
	UnpricedTokens int64              `json:"unpricedTokens,omitempty,omitzero"`
	// Model is the model of a Grok session. The Swift Codex item has none, so
	// it is not part of the JSON.
	Model *string `json:"-"`
}

// Summary is the recent sessions list, newest first.
type Summary struct {
	Items []Item `json:"items"`
}

// Pricer prices one request of one model in USD. ok is false for a model without a
// price, which is then left out of the session's estimate. The price table
// belongs to the usage package; a nil Pricer prices nothing.
type Pricer func(model string, totals Totals) (usd swiftjson.Decimal, ok bool)
