package l10n

import (
	"strings"
	"testing"
)

func TestCodexRuntimeTranslationsComplete(t *testing.T) {
	for key, row := range codexRuntimeStrings {
		for i, value := range row {
			if strings.TrimSpace(value) == "" {
				t.Errorf("%s missing %s", key, Languages()[i])
			}
			if got := New(Languages()[i]).CodexRuntime(key); got != value {
				t.Errorf("%s lookup mismatch", key)
			}
		}
	}
}
