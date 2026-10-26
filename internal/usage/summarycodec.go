package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/licoy/quosparr/internal/platform"
	"github.com/licoy/quosparr/internal/usage/decimal"
)

// SummaryCacheFileName is the persisted summary inside ~/.quosparr.
const SummaryCacheFileName = "api-equivalent-cost.json"

// DefaultSummaryCachePath returns <home>/.quosparr/api-equivalent-cost.json.
func DefaultSummaryCachePath(home string) string {
	return filepath.Join(home, ".quosparr", SummaryCacheFileName)
}

// ErrInvalidSummaryJSON reports a summary document that cannot be decoded.
var ErrInvalidSummaryJSON = errors.New("usage: invalid summary document")

// --- encoding ---------------------------------------------------------------

type windowDoc struct {
	Start    string  `json:"start"`
	Duration float64 `json:"duration"`
}

type dailyDoc struct {
	UnpricedTokens  int64            `json:"unpricedTokens,omitempty"`
	Date            string           `json:"date"`
	Totals          Totals           `json:"totals"`
	EstimatedUSD    *decimal.Decimal `json:"estimatedUSD,omitempty"`
	RawCredits      float64          `json:"rawCredits"`
	CreditsReported *bool            `json:"creditsReported,omitempty"`
	TotalsReported  *bool            `json:"totalsReported,omitempty"`
}

type breakdownDoc struct {
	UnpricedTokens int64            `json:"unpricedTokens,omitempty"`
	Name           string           `json:"name"`
	Totals         Totals           `json:"totals"`
	EstimatedUSD   *decimal.Decimal `json:"estimatedUSD,omitempty"`
	RawCredits     float64          `json:"rawCredits"`
}

type summaryDoc struct {
	UnpricedTokens int64            `json:"unpricedTokens,omitempty"`
	Source         Source           `json:"source"`
	Confidence     Confidence       `json:"confidence"`
	Window         windowDoc        `json:"window"`
	EstimatedUSD   *decimal.Decimal `json:"estimatedUSD,omitempty"`
	Totals         Totals           `json:"totals"`
	DailyRows      []dailyDoc       `json:"dailyRows"`
	ModelRows      []breakdownDoc   `json:"modelRows"`
	ProjectRows    []breakdownDoc   `json:"projectRows"`
	ClientRows     []breakdownDoc   `json:"clientRows"`
	RawCredits     float64          `json:"rawCredits"`
	Warnings       []string         `json:"warnings"`
	PricingVersion string           `json:"pricingVersion"`
	CalculatedAt   string           `json:"calculatedAt"`
}

// formatISO8601 renders t as the whole-second UTC form Foundation's .iso8601
// date strategy writes; fractional seconds are dropped (floored).
func formatISO8601(t time.Time) string {
	return time.Unix(t.Unix(), 0).UTC().Format("2006-01-02T15:04:05Z")
}

func breakdownDocs(rows []BreakdownRow) []breakdownDoc {
	out := make([]breakdownDoc, len(rows))
	for i, r := range rows {
		out[i] = breakdownDoc{r.UnpricedTokens, r.Name, r.Totals, r.EstimatedUSD, r.RawCredits}
	}
	return out
}

// EncodeSummary writes s in the persisted format (a superset-compatible form
// of the Swift cache file: ISO-8601 dates at second precision, a window of
// start and duration, Decimals as bare numbers). It fails when a credit value
// is not finite or a Decimal overflowed.
func EncodeSummary(s Summary) ([]byte, error) {
	doc := summaryDoc{
		Source: s.Source, Confidence: s.Confidence, UnpricedTokens: s.UnpricedTokens,
		Window:       windowDoc{Start: formatISO8601(s.Window.Start), Duration: unixSeconds(s.Window.End) - unixSeconds(s.Window.Start)},
		EstimatedUSD: s.EstimatedUSD, Totals: s.Totals,
		DailyRows:   make([]dailyDoc, len(s.DailyRows)),
		ModelRows:   breakdownDocs(s.ModelRows),
		ProjectRows: breakdownDocs(s.ProjectRows),
		ClientRows:  breakdownDocs(s.ClientRows),
		RawCredits:  s.RawCredits, Warnings: append([]string{}, s.Warnings...),
		PricingVersion: s.PricingVersion, CalculatedAt: formatISO8601(s.CalculatedAt),
	}
	for i, r := range s.DailyRows {
		doc.DailyRows[i] = dailyDoc{r.UnpricedTokens, r.Date, r.Totals, r.EstimatedUSD, r.RawCredits, r.CreditsReported, r.TotalsReported}
	}
	return marshalSwiftJSON(doc)
}

// --- decoding ---------------------------------------------------------------

type totalsRaw struct {
	TotalTokens         json.RawMessage `json:"totalTokens"`
	UncachedInputTokens json.RawMessage `json:"uncachedInputTokens"`
	CachedInputTokens   json.RawMessage `json:"cachedInputTokens"`
	OutputTokens        json.RawMessage `json:"outputTokens"`
	Turns               json.RawMessage `json:"turns"`
	Threads             json.RawMessage `json:"threads"`
}

type breakdownRaw struct {
	UnpricedTokens *int64          `json:"unpricedTokens"`
	Name           *string         `json:"name"`
	Totals         *totalsRaw      `json:"totals"`
	EstimatedUSD   json.RawMessage `json:"estimatedUSD"`
	RawCredits     json.RawMessage `json:"rawCredits"`
}

type dailyRaw struct {
	UnpricedTokens  *int64          `json:"unpricedTokens"`
	Date            *string         `json:"date"`
	Totals          *totalsRaw      `json:"totals"`
	EstimatedUSD    json.RawMessage `json:"estimatedUSD"`
	RawCredits      json.RawMessage `json:"rawCredits"`
	CreditsReported *bool           `json:"creditsReported"`
	TotalsReported  *bool           `json:"totalsReported"`
}

type windowRaw struct {
	Start    *string         `json:"start"`
	Duration json.RawMessage `json:"duration"`
}

type summaryRaw struct {
	UnpricedTokens *int64          `json:"unpricedTokens"`
	Source         *string         `json:"source"`
	Confidence     *string         `json:"confidence"`
	Window         *windowRaw      `json:"window"`
	EstimatedUSD   json.RawMessage `json:"estimatedUSD"`
	Totals         *totalsRaw      `json:"totals"`
	DailyRows      *[]dailyRaw     `json:"dailyRows"`
	ModelRows      *[]breakdownRaw `json:"modelRows"`
	ProjectRows    *[]breakdownRaw `json:"projectRows"`
	ClientRows     *[]breakdownRaw `json:"clientRows"`
	RawCredits     json.RawMessage `json:"rawCredits"`
	Warnings       *[]string       `json:"warnings"`
	PricingVersion *string         `json:"pricingVersion"`
	CalculatedAt   *string         `json:"calculatedAt"`
}

func isNull(raw json.RawMessage) bool { return len(raw) == 0 || bytes.Equal(raw, []byte("null")) }

func numberRaw(raw json.RawMessage) ([]byte, bool) {
	if len(raw) == 0 || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return nil, false
	}
	return raw, true
}

func rawInt(raw json.RawMessage) (int64, error) {
	tok, ok := numberRaw(raw)
	if !ok {
		return 0, ErrInvalidSummaryJSON
	}
	n, ok := parseIntegralJSON(tok)
	if !ok {
		return 0, ErrInvalidSummaryJSON
	}
	return n, nil
}

func rawFloat(raw json.RawMessage) (float64, error) {
	tok, ok := numberRaw(raw)
	if !ok || !validJSONNumber(tok) {
		return 0, ErrInvalidSummaryJSON
	}
	f, err := strconv.ParseFloat(string(tok), 64)
	if err != nil || math.IsInf(f, 0) {
		return 0, ErrInvalidSummaryJSON
	}
	return f, nil
}

func optDecimal(raw json.RawMessage) (*decimal.Decimal, error) {
	if isNull(raw) {
		return nil, nil
	}
	var d decimal.Decimal
	if _, ok := numberRaw(raw); !ok || d.UnmarshalJSON(raw) != nil {
		return nil, ErrInvalidSummaryJSON
	}
	return &d, nil
}

func (t *totalsRaw) decode() (Totals, error) {
	if t == nil {
		return Totals{}, ErrInvalidSummaryJSON
	}
	var out Totals
	for _, f := range []struct {
		raw json.RawMessage
		dst *int64
	}{
		{t.TotalTokens, &out.TotalTokens}, {t.UncachedInputTokens, &out.UncachedInputTokens},
		{t.CachedInputTokens, &out.CachedInputTokens}, {t.OutputTokens, &out.OutputTokens},
		{t.Turns, &out.Turns}, {t.Threads, &out.Threads},
	} {
		n, err := rawInt(f.raw)
		if err != nil {
			return Totals{}, err
		}
		*f.dst = n
	}
	return out, nil
}

func decodeBreakdowns(rows []breakdownRaw) ([]BreakdownRow, error) {
	out := make([]BreakdownRow, len(rows))
	for i, r := range rows {
		if r.Name == nil {
			return nil, ErrInvalidSummaryJSON
		}
		t, err := r.Totals.decode()
		if err != nil {
			return nil, err
		}
		usd, err := optDecimal(r.EstimatedUSD)
		if err != nil {
			return nil, err
		}
		credits, err := rawFloat(r.RawCredits)
		if err != nil {
			return nil, err
		}
		unpriced, err := decodeUnpriced(r.UnpricedTokens)
		if err != nil {
			return nil, err
		}
		out[i] = BreakdownRow{Name: *r.Name, Totals: t, EstimatedUSD: usd, RawCredits: credits, UnpricedTokens: unpriced}
	}
	return out, nil
}

func parseISO8601Seconds(s string) (time.Time, error) {
	if !iso8601Seconds.MatchString(s) {
		return time.Time{}, ErrInvalidSummaryJSON
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, ErrInvalidSummaryJSON
	}
	return t, nil
}

func (w *windowRaw) decode() (Window, error) {
	if w == nil || w.Start == nil {
		return Window{}, ErrInvalidSummaryJSON
	}
	start, err := parseISO8601Seconds(*w.Start)
	if err != nil {
		return Window{}, err
	}
	dur, err := rawFloat(w.Duration)
	if err != nil || dur < 0 {
		return Window{}, ErrInvalidSummaryJSON
	}
	whole, frac := math.Modf(dur)
	if whole > 1<<40 {
		return Window{}, ErrInvalidSummaryJSON
	}
	end := start.Add(time.Duration(whole) * time.Second).Add(time.Duration(math.Round(frac * 1e9)))
	return Window{Start: start, End: end}, nil
}

// DecodeSummary parses a persisted summary. Every field the Swift decoder
// requires is required here (projectRows and the estimated amounts are
// optional); anything else is ErrInvalidSummaryJSON.
func DecodeSummary(data []byte) (Summary, error) {
	var r summaryRaw
	if err := json.Unmarshal(data, &r); err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrInvalidSummaryJSON, err)
	}
	if r.Source == nil || r.Confidence == nil || r.DailyRows == nil || r.ModelRows == nil || r.ClientRows == nil ||
		r.Warnings == nil || r.PricingVersion == nil || r.CalculatedAt == nil {
		return Summary{}, ErrInvalidSummaryJSON
	}
	s := Summary{Source: Source(*r.Source), Confidence: Confidence(*r.Confidence),
		Warnings: append([]string{}, *r.Warnings...), PricingVersion: *r.PricingVersion}
	switch s.Source {
	case SourceLocalSessions, SourceOnlineAnalytics, SourceUnavailable:
	default:
		return Summary{}, ErrInvalidSummaryJSON
	}
	switch s.Confidence {
	case ConfidencePriced, ConfidencePartiallyPriced, ConfidenceTokensOnly, ConfidenceUnavailable:
	default:
		return Summary{}, ErrInvalidSummaryJSON
	}
	var err error
	if s.UnpricedTokens, err = decodeUnpriced(r.UnpricedTokens); err != nil {
		return Summary{}, err
	}
	if s.Window, err = r.Window.decode(); err != nil {
		return Summary{}, err
	}
	if s.EstimatedUSD, err = optDecimal(r.EstimatedUSD); err != nil {
		return Summary{}, err
	}
	if s.Totals, err = r.Totals.decode(); err != nil {
		return Summary{}, err
	}
	if s.RawCredits, err = rawFloat(r.RawCredits); err != nil {
		return Summary{}, err
	}
	if s.CalculatedAt, err = parseISO8601Seconds(*r.CalculatedAt); err != nil {
		return Summary{}, err
	}
	s.DailyRows = make([]DailyRow, len(*r.DailyRows))
	for i, d := range *r.DailyRows {
		if d.Date == nil {
			return Summary{}, ErrInvalidSummaryJSON
		}
		row := DailyRow{Date: *d.Date, CreditsReported: d.CreditsReported, TotalsReported: d.TotalsReported}
		if row.UnpricedTokens, err = decodeUnpriced(d.UnpricedTokens); err != nil {
			return Summary{}, err
		}
		if row.Totals, err = d.Totals.decode(); err != nil {
			return Summary{}, err
		}
		if row.EstimatedUSD, err = optDecimal(d.EstimatedUSD); err != nil {
			return Summary{}, err
		}
		if row.RawCredits, err = rawFloat(d.RawCredits); err != nil {
			return Summary{}, err
		}
		s.DailyRows[i] = row
	}
	if s.ModelRows, err = decodeBreakdowns(*r.ModelRows); err != nil {
		return Summary{}, err
	}
	if r.ProjectRows != nil {
		s.ProjectRows, err = decodeBreakdowns(*r.ProjectRows)
	} else {
		s.ProjectRows = []BreakdownRow{}
	}
	if err != nil {
		return Summary{}, err
	}
	if s.ClientRows, err = decodeBreakdowns(*r.ClientRows); err != nil {
		return Summary{}, err
	}
	return s, nil
}

// --- cache store ------------------------------------------------------------

// CacheState says what Load found.
type CacheState int

const (
	// CacheMissing means there is no cache file.
	CacheMissing CacheState = iota
	// CacheLoaded means Summary is valid and priced with a usable version.
	CacheLoaded
	// CacheIncompatible means a file exists but is unreadable, corrupt or was
	// priced with a stale built-in table. It is left in place; the next Save
	// replaces it.
	CacheIncompatible
)

// CacheLoad is the result of CacheStore.Load.
type CacheLoad struct {
	State   CacheState
	Summary Summary
	// Reason explains CacheIncompatible ("unreadable", "corrupt", "stale-pricing").
	Reason string
}

// CacheStore persists the last calculated Summary so the panel can show it
// before a scan finishes.
type CacheStore struct {
	Path string
}

// Load reads the cache. Only a missing file is CacheMissing.
func (c CacheStore) Load() CacheLoad {
	data, err := os.ReadFile(c.Path)
	if errors.Is(err, os.ErrNotExist) {
		return CacheLoad{State: CacheMissing}
	}
	if err != nil {
		return CacheLoad{State: CacheIncompatible, Reason: "unreadable"}
	}
	s, err := DecodeSummary(data)
	if err != nil {
		return CacheLoad{State: CacheIncompatible, Reason: "corrupt"}
	}
	if legacyCoverageMissing(data, s) {
		return CacheLoad{State: CacheIncompatible, Reason: "stale-coverage"}
	}
	if !IsCompatiblePricingVersion(s.PricingVersion) {
		return CacheLoad{State: CacheIncompatible, Reason: "stale-pricing"}
	}
	return CacheLoad{State: CacheLoaded, Summary: s}
}

// Save writes s atomically (directory 0700, file 0600).
func (c CacheStore) Save(s Summary) error {
	data, err := EncodeSummary(s)
	if err != nil {
		return err
	}
	return platform.WriteFileAtomic(c.Path, data, 0o600)
}

func decodeUnpriced(n *int64) (int64, error) {
	if n == nil {
		return 0, nil
	}
	if *n < 0 {
		return 0, ErrInvalidSummaryJSON
	}
	return *n, nil
}

// Old local summaries could label mixed known/unknown usage as fully priced.
// Reject these caches for local recomputation instead of inventing per-day coverage.
func legacyCoverageMissing(data []byte, s Summary) bool {
	if s.Source != SourceLocalSessions || s.Confidence != ConfidencePriced {
		return false
	}
	var coverage struct {
		UnpricedTokens *int64 `json:"unpricedTokens"`
	}
	if json.Unmarshal(data, &coverage) != nil || coverage.UnpricedTokens != nil {
		return false
	}
	for _, warning := range s.Warnings {
		if strings.HasPrefix(warning, "unknown-model:") || strings.HasPrefix(warning, "missing-") || strings.HasPrefix(warning, "ambiguous-tier:") {
			return true
		}
	}
	return false
}
