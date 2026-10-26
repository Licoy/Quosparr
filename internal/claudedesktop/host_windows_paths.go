package claudedesktop

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type windowsInstallRoots struct{ Local, System string }

func windowsCandidatePaths(ctx context.Context, run commandRunner, roots windowsInstallRoots) ([]string, error) {
	local := roots.Local
	paths := []string{filepath.Join(local, "Programs", "Claude", "Claude.exe"), filepath.Join(local, "Claude", "Claude.exe")}
	versions, err := filepath.Glob(filepath.Join(local, "AnthropicClaude", "app-*", "Claude.exe"))
	if err != nil {
		return nil, err
	}
	paths = append(paths, versions...)
	versions, err = filepath.Glob(filepath.Join(local, "Claude", "app-*", "Claude.exe"))
	if err != nil {
		return nil, err
	}
	paths = append(paths, versions...)
	system := roots.System
	// Package discovery uses a fixed script, never shell interpolation of a path.
	output, err := run(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; Get-AppxPackage | Where-Object { $_.Name -eq 'Claude' -or $_.Name -eq 'AnthropicPBC.Claude' } | Select-Object -ExpandProperty InstallLocation`)
	if err != nil {
		return nil, fmt.Errorf("discover Claude Desktop MSIX installation: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		dir := strings.TrimSpace(line)
		if dir == "" {
			continue
		}
		if !filepath.IsAbs(dir) {
			return nil, errors.New("Claude Desktop MSIX installation path is invalid")
		}
		paths = append(paths, filepath.Join(dir, "app", "Claude.exe"), filepath.Join(dir, "Claude.exe"))
	}
	return paths, nil
}

func windowsDataDirCandidates(exe, roaming, local string) []string {
	paths := []string{}
	if _, family := desktopMSIXLocation(exe); family != "" {
		paths = append(paths, filepath.Join(local, "Packages", family, "LocalCache", "Roaming", "Claude"))
	}
	if strings.HasPrefix(roaming, `\\`) {
		paths = append(paths, filepath.Join(local, "Claude-Data"))
	}
	return append(paths, filepath.Join(roaming, "Claude"))
}
