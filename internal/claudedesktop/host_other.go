package claudedesktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func rejectOtherDesktop(ctx context.Context, exe string, selected Application, platform string, run commandRunner) error {
	if belongsToApplication(exe, selected, platform) {
		return nil
	}
	other, err := isOtherDesktopInstallation(ctx, exe, platform, run)
	if err != nil {
		return err
	}
	if other {
		return errors.New("another Claude Desktop installation is running; quit all other Claude Desktop instances before switching")
	}
	return nil
}

func isOtherDesktopInstallation(ctx context.Context, exe, platform string, run commandRunner) (bool, error) {
	if platform == "darwin" {
		// A renamed .app is still identified by its signed bundle identity.
		bundle := ""
		for dir := filepath.Dir(exe); filepath.Dir(dir) != dir; dir = filepath.Dir(dir) {
			if filepath.Ext(dir) == ".app" {
				bundle = dir
			}
		}
		if bundle == "" {
			return false, nil
		}
		// Avoid inspecting unrelated app packages unless their executable/bundle
		// path suggests Claude; this does not authorize signaling by name.
		if !strings.Contains(strings.ToLower(exe), "claude") {
			return false, nil
		}
		id, err := run(ctx, "/usr/bin/plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-", filepath.Join(bundle, "Contents", "Info.plist"))
		if err != nil {
			return false, errors.New("cannot verify another suspected Claude Desktop application; quit it manually")
		}
		if strings.TrimSpace(string(id)) != "com.anthropic.claudefordesktop" {
			return false, nil
		}
		if _, err := run(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", bundle); err != nil {
			return false, errors.New("cannot verify another Claude Desktop installation; quit it manually")
		}
		return true, nil
	}
	archive := filepath.Join(filepath.Dir(exe), "resources", "app.asar")
	if _, err := os.Stat(archive); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, errors.New("cannot inspect another suspected Claude Desktop installation")
	}
	if platform == "windows" && !strings.EqualFold(filepath.Base(exe), "Claude.exe") && !strings.EqualFold(filepath.Base(exe), "chrome_crashpad_handler.exe") {
		return false, nil
	}
	if platform == "linux" {
		if err := verifyDesktopELF(exe); err != nil {
			return false, nil
		}
	}
	if err := verifyDesktopArchive(archive); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return false, errors.New("cannot verify another suspected Claude Desktop installation")
		}
		return false, nil // A CLI or another Electron application's archive is not Desktop.
	}
	return true, nil
}
