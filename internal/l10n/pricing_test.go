package l10n

import (
	"strings"
	"testing"
)

func TestPricingTranslationsComplete(t *testing.T) {
	for key, row := range pricingStrings {
		for i, text := range row {
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s missing %s", key, Languages()[i])
			}
		}
	}
}
