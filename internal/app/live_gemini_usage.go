package app

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/licoy/quosparr/internal/gemini"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/runway"
)

// geminiBucketRank orders the known quota windows: the Gemini 5 hours and
// week first, then Claude, then Argon; unknown buckets keep their order
// after them.
func geminiBucketRank(b gemini.QuotaBucket) int {
	switch b.Family + ":" + b.Window {
	case gemini.FamilyGemini + ":" + gemini.WindowFiveHour:
		return 0
	case gemini.FamilyGemini + ":" + gemini.WindowWeekly:
		return 1
	case gemini.FamilyClaude + ":" + gemini.WindowFiveHour:
		return 2
	case gemini.FamilyClaude + ":" + gemini.WindowWeekly:
		return 3
	case gemini.FamilyArgon + ":" + gemini.WindowWeekly:
		return 4
	}
	return 5
}

// geminiBuckets are the buckets in display order, one per known window.
func geminiBuckets(buckets []gemini.QuotaBucket) []gemini.QuotaBucket {
	out := make([]gemini.QuotaBucket, 0, len(buckets))
	seen := map[int]bool{}
	for _, b := range buckets {
		if rank := geminiBucketRank(b); rank < 5 {
			if seen[rank] {
				continue
			}
			seen[rank] = true
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool { return geminiBucketRank(out[i]) < geminiBucketRank(out[j]) })
	return out
}

func geminiWindowMinutes(window string) int {
	switch window {
	case gemini.WindowFiveHour:
		return runway.FiveHourMinutes
	case gemini.WindowWeekly:
		return runway.WeeklyMinutes
	}
	return 0
}

// geminiBucketTitle is "Gemini · 5-hour"; an unknown bucket keeps the name
// Cloud Code gave it.
func geminiBucketTitle(lc l10n.Localizer, b gemini.QuotaBucket) string {
	family := ""
	switch b.Family {
	case gemini.FamilyGemini:
		family = "Gemini"
	case gemini.FamilyClaude:
		family = "Claude"
	case gemini.FamilyArgon:
		family = "Argon"
	default:
		if b.DisplayName != "" {
			return b.DisplayName
		}
		family = b.ID
	}
	switch b.Window {
	case gemini.WindowFiveHour:
		return family + " · " + lc.Text(l10n.KeyFiveHourUsage)
	case gemini.WindowWeekly:
		return family + " · " + lc.Text(l10n.KeyWeeklyUsage)
	}
	return family
}

func geminiUsedPercent(remaining float64) int {
	return clampPercent(int(math.Round((1 - remaining) * 100)))
}

// geminiMeters are the quota bars of a reading. The Gemini windows are the
// standard meters; the other families are model-specific. Without buckets
// the per-model fractions stand in, the recommended models first.
func geminiMeters(lc l10n.Localizer, snap gemini.QuotaSnapshot, now time.Time) []runway.Meter {
	var meters []runway.Meter
	for _, b := range geminiBuckets(snap.Buckets) {
		window := runway.MeterWindow{UsedPercent: geminiUsedPercent(b.RemainingFraction), WindowMinutes: geminiWindowMinutes(b.Window)}
		if b.ResetAt != nil {
			window.ResetsAt = *b.ResetAt
		}
		source := runway.SourceModelSpecific
		if b.Family == gemini.FamilyGemini {
			source = runway.SourceStandard
		}
		meters = append(meters, runway.NewMeter(geminiBucketTitle(lc, b), window, now, runway.DefaultMarkers(), source))
	}
	if len(meters) > 0 {
		return meters
	}
	for i, m := range geminiModels(snap.Models) {
		window := runway.MeterWindow{UsedPercent: geminiUsedPercent(*m.RemainingFraction)}
		if m.ResetAt != nil {
			window.ResetsAt = *m.ResetAt
		}
		source := runway.SourceModelSpecific
		if i == 0 {
			source = runway.SourceStandard
		}
		meters = append(meters, runway.NewMeter(geminiModelTitle(m), window, now, runway.DefaultMarkers(), source))
	}
	return meters
}

// geminiModels are the models that report a quota, recommended first, one
// per title.
func geminiModels(models []gemini.ModelQuota) []gemini.ModelQuota {
	var out []gemini.ModelQuota
	seen := map[string]bool{}
	for _, m := range models {
		title := geminiModelTitle(m)
		if m.RemainingFraction == nil || seen[title] {
			continue
		}
		seen[title] = true
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Recommended && !out[j].Recommended })
	return out
}

func geminiModelTitle(m gemini.ModelQuota) string {
	switch {
	case m.DisplayName != "":
		return m.DisplayName
	case m.Model != "":
		return m.Model
	}
	return m.ID
}

// geminiTierLabel is the badge text of a plan: Free, Pro, Ultra, or the
// provider name while the plan is unknown.
func geminiTierLabel(lc l10n.Localizer, kind string) string {
	switch gemini.TierKind(kind) {
	case gemini.TierFree:
		return lc.Text(l10n.KeyPlanFree)
	case gemini.TierPro:
		return "Pro"
	case gemini.TierUltra:
		return "Ultra"
	}
	return lc.Text(l10n.KeyProviderGemini)
}

// geminiPlanName is the plan as Cloud Code names it, else the tier label.
func geminiPlanName(lc l10n.Localizer, tier gemini.Tier) string {
	if tier.Name != "" {
		return tier.Name
	}
	if tier.Kind == gemini.TierUnknown || tier.Kind == "" {
		return ""
	}
	return geminiTierLabel(lc, string(tier.Kind))
}

// geminiCreditLines are the credit balances, one row per credit type.
func geminiCreditLines(lc l10n.Localizer, credits []gemini.Credit) []runway.DetailLine {
	var out []runway.DetailLine
	for _, c := range credits {
		title := lc.Text(l10n.KeyGeminiCredits)
		if c.Type != "" {
			title += " · " + c.Type
		}
		out = append(out, runway.DetailLine{Title: title, Value: strconv.FormatFloat(c.Amount, 'f', -1, 64)})
	}
	return out
}

// geminiDetailLines are the rows of the quota details popover: the plan,
// every window with its reset, the per-model fractions and the credits.
func geminiDetailLines(lc l10n.Localizer, snap gemini.QuotaSnapshot, updated time.Time) []runway.DetailLine {
	lines := []runway.DetailLine{{Title: lc.Text(l10n.KeyGeminiPlan), Value: firstNonEmptyText(geminiPlanName(lc, snap.Tier), lc.Text(l10n.KeyUnknown))}}
	left := func(fraction float64, reset *time.Time) string {
		text := l10n.Format("%d%% %s", 100-geminiUsedPercent(fraction), lc.Text(l10n.KeyLeft))
		if reset != nil {
			text += " · " + reset.Local().Format("2006-01-02 15:04")
		}
		return text
	}
	for _, b := range geminiBuckets(snap.Buckets) {
		lines = append(lines, runway.DetailLine{Title: geminiBucketTitle(lc, b), Value: left(b.RemainingFraction, b.ResetAt)})
	}
	for _, m := range geminiModels(snap.Models) {
		lines = append(lines, runway.DetailLine{Title: geminiModelTitle(m), Value: left(*m.RemainingFraction, m.ResetAt)})
	}
	lines = append(lines, geminiCreditLines(lc, snap.Credits)...)
	if !updated.IsZero() {
		lines = append(lines, runway.DetailLine{Title: lc.Text(l10n.KeyLastUpdated), Value: updated.Local().Format("2006-01-02 15:04")})
	}
	return lines
}

func firstNonEmptyText(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
