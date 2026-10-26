package claudedesktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentModeReadsOnlyBoundedFixtureConfiguration(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "claude_desktop_config.json")
	if err := verifyFirstPartyDeployment([]string{root}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		value   string
		allowed bool
	}{{`{"deploymentMode":"1p","unrelated":"ignored"}`, true}, {`{"deploymentMode":"3p"}`, false}, {`{"deploymentMode":"unknown"}`, false}, {`{"deploymentMode":1}`, false}, {`{}`, false}, {`broken`, false}} {
		if err := os.WriteFile(path, []byte(item.value), 0600); err != nil {
			t.Fatal(err)
		}
		err := verifyFirstPartyDeployment([]string{root})
		if (err == nil) != item.allowed {
			t.Fatalf("fixture mode %q error %v", item.value, err)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "fixture-only.json")
	if err := os.WriteFile(outside, []byte(`{"deploymentMode":"1p"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err == nil {
		if err := verifyFirstPartyDeployment([]string{root}); err == nil {
			t.Fatal("symlink mode configuration was followed")
		}
	}
}

func TestDesktopDataDirectoryMustRemainStableWhenArchived(t *testing.T) {
	root := t.TempDir()
	canonical, legacy := filepath.Join(root, "current-package"), filepath.Join(root, "legacy")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := chooseDesktopDataDir([]string{canonical, legacy}, "windows"); err == nil {
		t.Fatal("selected a different installation's legacy profile")
	}
	if err := os.Remove(legacy); err != nil {
		t.Fatal(err)
	}
	got, err := chooseDesktopDataDir([]string{canonical, legacy}, "windows")
	if err != nil || got != canonical {
		t.Fatalf("%q %v", got, err)
	}
}
