package runway

import (
	"sort"
	"strconv"
	"time"

	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/codexapi"
	"github.com/licoy/quosparr/internal/l10n"
)

// ExpiringThreshold is how close to expiry a reset credit counts as expiring.
const ExpiringThreshold = 7 * 24 * time.Hour

// creditProgressSpan is the lifetime a full progress bar stands for.
const creditProgressSpan = 30 * 24 * time.Hour

// CreditRisk classifies one reset credit.
type CreditRisk int

// CreditRisk values.
const (
	CreditAvailable CreditRisk = iota
	CreditExpiring
	CreditUnavailable
)

// ClassifyCredit is Swift ResetCreditRisk.classify.
func ClassifyCredit(c codexapi.ResetCredit) CreditRisk {
	if c.Status != "available" {
		return CreditUnavailable
	}
	if c.ExpiresAt.IsZero() {
		return CreditAvailable
	}
	if c.RemainingSeconds <= ExpiringThreshold.Seconds() {
		return CreditExpiring
	}
	return CreditAvailable
}

// ResetCreditSummary counts the reset credits of one reading.
type ResetCreditSummary struct {
	AvailableCount       int
	StableAvailableCount int
	ExpiringCount        int
	UnavailableCount     int
	TotalCount           int
	// NextExpiry / LatestExpiry are zero when no available credit expires.
	NextExpiry            time.Time
	NextExpiryRemaining   float64
	LatestExpiry          time.Time
	LatestExpiryRemaining float64
	UpdatedAt             time.Time
}

// SummarizeCredits builds the summary of a reset credits reading.
func SummarizeCredits(s codexapi.ResetCreditsSnapshot) ResetCreditSummary {
	sum := ResetCreditSummary{AvailableCount: s.AvailableCount, TotalCount: len(s.Credits), UpdatedAt: s.UpdatedAt}
	var next, latest *codexapi.ResetCredit
	for i := range s.Credits {
		c := &s.Credits[i]
		switch ClassifyCredit(*c) {
		case CreditAvailable:
			sum.StableAvailableCount++
		case CreditExpiring:
			sum.ExpiringCount++
		case CreditUnavailable:
			sum.UnavailableCount++
		}
		if c.Status != "available" || c.ExpiresAt.IsZero() {
			continue
		}
		if next == nil || c.ExpiresAt.Before(next.ExpiresAt) {
			next = c
		}
		if latest == nil || c.ExpiresAt.After(latest.ExpiresAt) {
			latest = c
		}
	}
	if next != nil {
		sum.NextExpiry, sum.NextExpiryRemaining = next.ExpiresAt, next.RemainingSeconds
	}
	if latest != nil {
		sum.LatestExpiry, sum.LatestExpiryRemaining = latest.ExpiresAt, latest.RemainingSeconds
	}
	return sum
}

// SortedCreditsByExpiry orders credits by expiry (earliest first, those
// without expiry last, then by id). The input is not modified.
func SortedCreditsByExpiry(credits []codexapi.ResetCredit) []codexapi.ResetCredit {
	out := append([]codexapi.ResetCredit(nil), credits...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case !a.ExpiresAt.IsZero() && !b.ExpiresAt.IsZero():
			return a.ExpiresAt.Before(b.ExpiresAt)
		case !a.ExpiresAt.IsZero():
			return true
		case !b.ExpiresAt.IsZero():
			return false
		}
		return a.ID < b.ID
	})
	return out
}

// CreditDetail is one row of the reset credit list.
type CreditDetail struct {
	ID         string
	Title      string
	StatusText string
	State      CreditRisk
	// ExpiresAt is zero when the credit never expires.
	ExpiresAt         time.Time
	RemainingDuration time.Duration
	// RemainingProgress is 0...1: the share of a 30 day lifetime that is left.
	RemainingProgress float64
}

// CreditDetails builds the sorted detail rows.
func CreditDetails(lc l10n.Localizer, s codexapi.ResetCreditsSnapshot) []CreditDetail {
	sorted := SortedCreditsByExpiry(s.Credits)
	out := make([]CreditDetail, 0, len(sorted))
	for i, c := range sorted {
		remaining := max(0, c.RemainingSeconds)
		progress := 1.0
		if !c.ExpiresAt.IsZero() {
			progress = min(1, remaining/creditProgressSpan.Seconds())
		}
		id := c.ID
		if id == "" {
			id = strconv.Itoa(i)
		}
		out = append(out, CreditDetail{
			ID:                id,
			Title:             lc.Text(l10n.KeyCredit) + " " + strconv.Itoa(i+1),
			StatusText:        CreditStatusText(lc, c.Status),
			State:             ClassifyCredit(c),
			ExpiresAt:         c.ExpiresAt,
			RemainingDuration: time.Duration(remaining * float64(time.Second)),
			RemainingProgress: progress,
		})
	}
	return out
}

// CreditStatusText localizes a credit status.
func CreditStatusText(lc l10n.Localizer, status string) string {
	switch status {
	case "available":
		return lc.Text(l10n.KeyStatusAvailable)
	case "used":
		return lc.Text(l10n.KeyStatusUsed)
	}
	return lc.Text(l10n.KeyStatusUnknown)
}

// CreditUpdatedText is ResetCreditUpdatedAt, or "--" before the first reading.
func CreditUpdatedText(t time.Time, lang l10n.Language, loc *time.Location) string {
	if t.IsZero() {
		return "--"
	}
	return ResetCreditUpdatedAt(t, lang, loc)
}

// ExpiryDuration is the remaining time of a credit, "--" when unknown
// (Swift ResetCreditExpiryPresentation.duration).
func ExpiryDuration(remaining *float64, lang l10n.Language) string {
	if remaining == nil {
		return "--"
	}
	return DurationLocalized(*remaining, lang, false)
}

// ExpiryHelp is the tooltip of the reset credit expiry (Swift
// ResetCreditExpiryPresentation.help). expiry is zero when unknown.
func ExpiryHelp(lc l10n.Localizer, expiry, updatedAt time.Time, loc *time.Location) string {
	lang := lc.Language()
	exp := lc.Text(l10n.KeyResetExpiryUnavailable)
	if !expiry.IsZero() {
		exp = ResetCreditExpiresAt(expiry, lang, loc)
	}
	updated := CreditUpdatedText(updatedAt, lang, loc)
	return exp + "\n" + lc.Text(l10n.KeyLastUpdated) + ": " + updated + "\n" + lc.Text(l10n.KeyResetExpirySummaryHelp)
}

// cacheAccountResetCredits retains only derived expiry times of available cards,
// never their ids; a count without details must leave expiry unknown.
func cacheAccountResetCredits(account codex.ManagedAccount, credits codexapi.ResetCreditsSnapshot, now time.Time) codex.ManagedAccount {
	if account.ResetCredits != nil && account.ResetCredits.CheckedAt.After(credits.UpdatedAt) {
		return account
	}
	updated := account.ApplyResetCredits(&credits.AvailableCount, &credits.UpdatedAt, "")
	if credits.AvailableCount <= 0 {
		return updated
	}
	for _, card := range credits.Credits {
		if card.Status == "available" && card.ExpiresAt.After(now) && card.ExpiresAt.After(credits.UpdatedAt) {
			updated.ResetCredits.AvailableExpiries = append(updated.ResetCredits.AvailableExpiries, card.ExpiresAt)
		}
	}
	sort.Slice(updated.ResetCredits.AvailableExpiries, func(i, j int) bool {
		return updated.ResetCredits.AvailableExpiries[i].Before(updated.ResetCredits.AvailableExpiries[j])
	})
	return updated
}
