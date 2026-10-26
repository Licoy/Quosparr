package claudedesktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestElectronHelperRoleUsesOnlyInjectedCommandLineFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cmdline")
	for _, fixture := range []struct {
		data   string
		helper bool
	}{{"/fixture/claude-desktop\x00", false}, {"/fixture/claude-desktop\x00--type=renderer\x00", true}, {"/fixture/claude-desktop\x00--type=utility\x00--utility-sub-type=network.mojom.NetworkService\x00", true}, {"/fixture/claude-desktop\x00--type\x00gpu-process\x00", true}, {"/fixture/claude-desktop\x00--name=--type=renderer\x00", false}} {
		if err := os.WriteFile(path, []byte(fixture.data), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := desktopHelperCommand(path)
		if err != nil || got != fixture.helper {
			t.Fatalf("helper=%v err=%v", got, err)
		}
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := desktopHelperCommand(path); err == nil {
		t.Fatal("empty role accepted")
	}
}
