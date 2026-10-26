package claudedesktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWindowsDiscoveryUsesLiteralPathsAndFixedPackageCommand(t *testing.T) {
	root := t.TempDir()
	local, system := filepath.Join(root, "local with spaces"), filepath.Join(root, "system")
	version := filepath.Join(local, "AnthropicClaude", "app-2.0", "Claude.exe")
	if err := os.MkdirAll(filepath.Dir(version), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(version, []byte("fixture, never execute"), 0600); err != nil {
		t.Fatal(err)
	}
	msix := filepath.Join(root, "WindowsApps", "AnthropicPBC.Claude_2_x64__fnn82j28hfe8t")
	called := false
	run := func(_ context.Context, command string, args ...string) ([]byte, error) {
		called = true
		if command != filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe") || len(args) != 4 || !reflect.DeepEqual(args[:3], []string{"-NoProfile", "-NonInteractive", "-Command"}) {
			t.Fatalf("wrong command %q %q", command, args)
		}
		if strings.Contains(args[3], local) || !strings.Contains(args[3], "Get-AppxPackage") {
			t.Fatal("user path interpolated into PowerShell")
		}
		return []byte(msix + "\r\n"), nil
	}
	paths, err := windowsCandidatePaths(context.Background(), run, windowsInstallRoots{Local: local, System: system})
	if err != nil || !called {
		t.Fatal(err)
	}
	for _, want := range []string{version, filepath.Join(msix, "app", "Claude.exe"), filepath.Join(msix, "Claude.exe")} {
		found := false
		for _, path := range paths {
			found = found || path == want
		}
		if !found {
			t.Fatalf("missing candidate %s", want)
		}
	}
	run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("fixture registry/package discovery denied")
	}
	if _, err := windowsCandidatePaths(context.Background(), run, windowsInstallRoots{Local: local, System: system}); err == nil {
		t.Fatal("package discovery failure swallowed")
	}
}

func TestWindowsProfilesMatchSelectedPackageFamily(t *testing.T) {
	root := t.TempDir()
	roaming, local := filepath.Join(root, "roaming"), filepath.Join(root, "local")
	for _, item := range []struct{ folder, family string }{{"Claude_2_x64__pzs8sxrjxfjjc", "Claude_pzs8sxrjxfjjc"}, {"AnthropicPBC.Claude_2_arm64__fnn82j28hfe8t", "AnthropicPBC.Claude_fnn82j28hfe8t"}} {
		paths := windowsDataDirCandidates(filepath.Join(root, item.folder, "app", "Claude.exe"), roaming, local)
		if !reflect.DeepEqual(paths, []string{filepath.Join(local, "Packages", item.family, "LocalCache", "Roaming", "Claude"), filepath.Join(roaming, "Claude")}) {
			t.Fatal(paths)
		}
	}
	if got := windowsDataDirCandidates(filepath.Join(root, "classic", "Claude.exe"), roaming, local); !reflect.DeepEqual(got, []string{filepath.Join(roaming, "Claude")}) {
		t.Fatal(got)
	}
}
