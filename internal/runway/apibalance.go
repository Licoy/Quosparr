package runway

import (
	"math"
	"strconv"
	"strings"

	"github.com/licoy/quosparr/internal/apiquota"
	"github.com/licoy/quosparr/internal/l10n"
)

// APIBalanceAmount formats one amount of a relay balance in its unit:
// "$12.34", "¥12.30", "1.2M tokens", or the number and a custom symbol.
func APIBalanceAmount(unit string, v float64) string {
	switch strings.ToUpper(unit) {
	case apiquota.UnitUSD, "":
		return signed(v, "$"+strconv.FormatFloat(math.Abs(v), 'f', 2, 64))
	case apiquota.UnitCNY:
		return signed(v, "¥"+strconv.FormatFloat(math.Abs(v), 'f', 2, 64))
	case apiquota.UnitTokens:
		return CompactNumber(int64(v)) + " tokens"
	case apiquota.UnitRawQuota:
		return CompactNumber(int64(v))
	}
	return strconv.FormatFloat(v, 'f', 2, 64) + " " + unit
}

func signed(v float64, text string) string {
	if v < 0 {
		return "-" + text
	}
	return text
}

// APIBalanceText is the short balance: the remaining amount, "∞" when
// unlimited, else the used amount; "" without a reading.
func APIBalanceText(q apiquota.Quota) string {
	switch {
	case q.Status != apiquota.StatusAvailable:
		return ""
	case q.Unlimited:
		return "∞"
	case q.Remaining != nil:
		return APIBalanceAmount(q.Unit, *q.Remaining)
	case q.Used != nil:
		return APIBalanceAmount(q.Unit, *q.Used)
	}
	return ""
}

// APIBalanceDetail is "remaining / total" when both are known, else
// APIBalanceText.
func APIBalanceDetail(q apiquota.Quota) string {
	text := APIBalanceText(q)
	if text == "" || q.Unlimited || q.Total == nil || q.Remaining == nil {
		return text
	}
	return text + " / " + APIBalanceAmount(q.Unit, *q.Total)
}

// APIBalanceMeter turns a relay balance into a meter titled "Balance". The
// bar is the used share when the total is known; otherwise it is full while
// money is left. ValueText carries the amount the captions show.
func APIBalanceMeter(lc l10n.Localizer, q apiquota.Quota) (Meter, bool) {
	text := APIBalanceText(q)
	if text == "" {
		return Meter{}, false
	}
	used := 0
	if f, ok := q.UsedFraction(); ok {
		used = int(math.Round(f * 100))
	} else if !q.Unlimited && q.Remaining != nil && *q.Remaining <= 0 {
		used = 100
	}
	used = min(100, max(0, used))
	return Meter{
		Title:            lc.Text(l10n.KeyWidgetBalance),
		Source:           SourceStandard,
		UsedPercent:      used,
		RemainingPercent: 100 - used,
		Health:           HealthForUsedPercent(used),
		ValueText:        text,
		Balance:          true,
	}, true
}

// meterTitle is the short status title: the window name, or the balance
// meter's own title.
func meterTitle(lc l10n.Localizer, m Meter) string {
	if m.Balance || m.PreserveTitle {
		return m.Title
	}
	return ShortWindowTitle(lc, m.WindowMinutes)
}

// percentText is "NN%", or the amount of a balance meter.
func percentText(m Meter) string {
	if m.ValueText != "" {
		return m.ValueText
	}
	return strconv.Itoa(m.RemainingPercent) + "%"
}
