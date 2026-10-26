package claudedesktop

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopMSIXManifestIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "AnthropicPBC.Claude_2.9939.2.0_x64__fnn82j28hfe8t")
	exe := filepath.Join(root, "app", "Claude.exe")
	gotRoot, family := desktopMSIXLocation(exe)
	if gotRoot != root || family != "AnthropicPBC.Claude_fnn82j28hfe8t" {
		t.Fatalf("%q %q", gotRoot, family)
	}
	identity := desktopMSIX{Root: root, Family: family}
	manifest := `<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"><Identity Name="AnthropicPBC.Claude"/><Applications><Application Id="Claude" Executable="app\Claude.exe"/></Applications></Package>`
	got, err := parseDesktopMSIX(strings.NewReader(manifest), exe, identity)
	if err != nil || got.AppID != "Claude" {
		t.Fatalf("%#v %v", got, err)
	}
	for _, invalid := range []string{
		strings.Replace(manifest, "AnthropicPBC.Claude", "OtherPublisher", 1),
		strings.Replace(manifest, `app\Claude.exe`, `app\claude-cli.exe`, 1),
		strings.Replace(manifest, `Id="Claude"`, `Id="Claude!malicious"`, 1),
		strings.Replace(manifest, `app\Claude.exe`, `..\Claude.exe`, 1),
		`<broken`,
	} {
		if _, err := parseDesktopMSIX(strings.NewReader(invalid), exe, identity); err == nil {
			t.Fatalf("invalid manifest accepted: %q", invalid)
		}
	}
	if root, family := desktopMSIXLocation(filepath.Join(t.TempDir(), "Claude.exe")); root != "" || family != "" {
		t.Fatal("classic EXE mistaken for MSIX")
	}
}

func TestDesktopMSIXActivationUsesExactAUMIDAndChecksReturnedProcess(t *testing.T) {
	identity := desktopMSIX{Family: "AnthropicPBC.Claude_fnn82j28hfe8t", AppID: "Claude"}
	for _, mode := range []string{"success", "activation-failed", "missing-pid", "wrong-process"} {
		t.Run(mode, func(t *testing.T) {
			childCalled := false
			api := desktopActivationAPI{
				activate: func(aumid string) (uint32, error) {
					if aumid != "AnthropicPBC.Claude_fnn82j28hfe8t!Claude" {
						t.Fatal(aumid)
					}
					if mode == "activation-failed" {
						return 0, errors.New("fixture failure")
					}
					if mode == "missing-pid" {
						return 0, nil
					}
					return 1234, nil
				},
				child: func(pid uint32, exe string) (desktopChild, error) {
					childCalled = true
					if pid != 1234 || exe != "fixture/Claude.exe" {
						t.Fatal("wrong activated process")
					}
					if mode == "wrong-process" {
						return nil, errors.New("wrong executable or owner")
					}
					return fakeDesktopChild{make(chan error)}, nil
				},
			}
			_, err := startDesktopMSIX(identity, "fixture/Claude.exe", api)
			if (err == nil) != (mode == "success") {
				t.Fatal(err)
			}
			if (mode == "activation-failed" || mode == "missing-pid") && childCalled {
				t.Fatal("opened an unverified process")
			}
		})
	}
}
