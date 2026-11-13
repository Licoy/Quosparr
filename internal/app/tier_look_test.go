package app

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/licoy/quosparr/internal/claude"
	"github.com/licoy/quosparr/internal/codex"
	"github.com/licoy/quosparr/internal/gemini"
)

func TestClaudeTierLooksAreDistinct(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := &ui.Theme{Dark: dark}
		plain := plainLook(theme)
		seen := map[claude.TierKind]tierLook{}
		for _, k := range previewClaudeTiers {
			look := claudeTierLook(theme, k)
			if reflect.DeepEqual(look, plain) {
				t.Errorf("dark=%t %s uses the plain look", dark, k.DisplayName())
			}
			seen[k] = look
		}
		for _, pair := range [][2]claude.TierKind{{claude.TierPro, claude.TierMax5x}, {claude.TierMax5x, claude.TierMax20x}, {claude.TierTeam, claude.TierEnterprise}} {
			if reflect.DeepEqual(seen[pair[0]], seen[pair[1]]) {
				t.Errorf("dark=%t %s and %s share a look", dark, pair[0].DisplayName(), pair[1].DisplayName())
			}
		}
		if !reflect.DeepEqual(claudeTierLook(theme, claude.TierUnknown), plain) {
			t.Errorf("dark=%t unknown tier must stay plain", dark)
		}
		if reflect.DeepEqual(codexTierLook(theme, codex.TierBusinessPremium), plain) {
			t.Errorf("dark=%t Business Premium uses the plain look", dark)
		}
	}
}

func TestBusinessPremiumLabel(t *testing.T) {
	if accountTierKey(codex.TierBusinessPremium) == accountTierKey(codex.TierBusiness) {
		t.Fatal("Business Premium needs its own label")
	}
}

func TestGeminiTierLooksAreDistinct(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := &ui.Theme{Dark: dark}
		plain := plainLook(theme)
		looks := map[gemini.TierKind]tierLook{}
		for _, k := range []gemini.TierKind{gemini.TierFree, gemini.TierPro, gemini.TierUltra} {
			looks[k] = geminiTierLook(theme, k)
			if reflect.DeepEqual(looks[k], plain) {
				t.Errorf("dark=%t %s uses the plain look", dark, k)
			}
		}
		if reflect.DeepEqual(looks[gemini.TierFree], looks[gemini.TierPro]) || reflect.DeepEqual(looks[gemini.TierPro], looks[gemini.TierUltra]) {
			t.Errorf("dark=%t Gemini tiers share a look", dark)
		}
		if !reflect.DeepEqual(geminiTierLook(theme, gemini.TierUnknown), plain) {
			t.Errorf("dark=%t unknown Gemini tier must stay plain", dark)
		}
	}
}
