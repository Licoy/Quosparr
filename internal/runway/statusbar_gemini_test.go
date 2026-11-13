package runway

import (
	"testing"
	"time"

	"github.com/licoy/quosparr/internal/export"
	"github.com/licoy/quosparr/internal/l10n"
	"github.com/licoy/quosparr/internal/store"
)

func TestGeminiStatusTextPrefersTheFiveHourWindow(t *testing.T) {
	lc := l10n.New(l10n.English)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	weekly := Meter{WindowMinutes: WeeklyMinutes, Source: SourceStandard, RemainingPercent: 40}
	five := Meter{WindowMinutes: FiveHourMinutes, Source: SourceStandard, RemainingPercent: 70}
	if got := GeminiStatusText(lc, GeminiState{Meters: []Meter{weekly, five}}, now); got != "70%" {
		t.Fatalf("five-hour first: %q", got)
	}
	if got := GeminiStatusText(lc, GeminiState{Meters: []Meter{weekly}}, now); got != "40%" {
		t.Fatalf("weekly fallback: %q", got)
	}
	for av, key := range map[GeminiAvailability]l10n.Key{
		GeminiNotLoggedIn: l10n.KeyStatusLogin, GeminiValidationRequired: l10n.KeyStatusLogin,
		GeminiLoading: l10n.KeyStatusWait, GeminiReady: l10n.KeyStatusUnknown, GeminiRateLimited: l10n.KeyStatusError,
	} {
		if got := GeminiStatusText(lc, GeminiState{Availability: av}, now); got != lc.Text(key) {
			t.Errorf("%v: %q", av, got)
		}
	}
}

func TestGeminiWidgetBlockFollowsTheBridge(t *testing.T) {
	p := store.DefaultPreferences()
	if got := geminiWidgetProvider(GeminiState{}, p); got.Provider != export.ProviderGemini {
		t.Fatalf("missing bridge block: %+v", got)
	}
	block := export.NewProviderSnapshot(export.ProviderSnapshot{Provider: export.ProviderGemini, Availability: export.Available})
	if got := geminiWidgetProvider(GeminiState{Widget: &block}, p); got.Availability != export.Available {
		t.Fatalf("bridge block: %+v", got)
	}
}
