package claudedesktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOtherDesktopInstallationBlocksSnapshotButCLIIsIgnored(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			h := fixtureHost(t, platform)
			selected, err := h.Resolve(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(h.home, "old-app-2.0", filepath.Base(selected.Executable))
			if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
				t.Fatal(err)
			}
			data := []byte("fixture only")
			if platform == "linux" {
				data = fixtureDesktopELF()
			}
			if err := os.WriteFile(other, data, 0700); err != nil {
				t.Fatal(err)
			}
			if err := rejectOtherDesktop(context.Background(), other, selected, platform, h.run); err != nil {
				t.Fatal("CLI without Desktop identity was mistaken for Desktop", err)
			}
			writeHostArchive(t, filepath.Join(filepath.Dir(other), "resources", "app.asar"), `{"name":"@ant/desktop","productName":"Claude","main":".vite/build/index.pre.js"}`)
			if err := rejectOtherDesktop(context.Background(), other, selected, platform, h.run); err == nil {
				t.Fatal("other verified Desktop was ignored")
			}
			if err := rejectOtherDesktop(context.Background(), selected.Executable, selected, platform, h.run); err != nil {
				t.Fatal("selected application rejected as another installation", err)
			}
		})
	}
}

func TestOtherMacDesktopIsIdentifiedByBundleNotByProcessName(t *testing.T) {
	selected := Application{Path: "/fixture/Applications/Claude.app", Executable: "/fixture/Applications/Claude.app/Contents/MacOS/Claude"}
	other := "/fixture/home/Applications/Claude.app/Contents/MacOS/Claude"
	calls := 0
	run := func(_ context.Context, exe string, args ...string) ([]byte, error) {
		calls++
		if exe == "/usr/bin/plutil" {
			return []byte("com.anthropic.claudefordesktop"), nil
		}
		if exe == "/usr/bin/codesign" {
			return nil, nil
		}
		return nil, errors.New("unexpected fixture command")
	}
	if err := rejectOtherDesktop(context.Background(), other, selected, "darwin", run); err == nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	calls = 0
	if err := rejectOtherDesktop(context.Background(), "/fixture/bin/claude", selected, "darwin", run); err != nil || calls != 0 {
		t.Fatal("CLI was mistaken for Desktop", err)
	}
	run = func(context.Context, string, ...string) ([]byte, error) { return []byte("com.other.app"), nil }
	if err := rejectOtherDesktop(context.Background(), other, selected, "darwin", run); err != nil {
		t.Fatal("same-name unrelated bundle was rejected", err)
	}
}
