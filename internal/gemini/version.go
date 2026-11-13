package gemini

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// MinimumClientVersion is the oldest Antigravity version sent to Cloud
// Code; older or unknown installs report it instead.
const MinimumClientVersion = "2.25.0"

var plistVersionPattern = regexp.MustCompile(`<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)

// versionFiles are the Info.plist and product.json locations of the
// Antigravity desktop app on this platform.
func versionFiles(home string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Antigravity.app/Contents/Info.plist",
			filepath.Join(home, "Applications", "Antigravity.app", "Contents", "Info.plist"),
		}
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		if local == "" {
			local = filepath.Join(home, "AppData", "Local")
		}
		return []string{filepath.Join(local, "Programs", "Antigravity", "resources", "app", "product.json")}
	}
	return []string{
		"/usr/share/antigravity/resources/app/product.json",
		"/opt/Antigravity/resources/app/product.json",
		filepath.Join(home, ".local", "share", "antigravity", "resources", "app", "product.json"),
	}
}

// DetectClientVersion reads the installed desktop app version from files,
// never below MinimumClientVersion.
func DetectClientVersion(files []string) string {
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if v := versionFromFile(path, data); v != "" {
			return atLeastMinimum(v)
		}
	}
	return MinimumClientVersion
}

// DefaultClientVersion is DetectClientVersion for the current user.
func DefaultClientVersion() string {
	home, _ := os.UserHomeDir()
	return DetectClientVersion(versionFiles(home))
}

func versionFromFile(path string, data []byte) string {
	if strings.HasSuffix(path, ".plist") {
		if m := plistVersionPattern.FindSubmatch(data); m != nil {
			return trimSpace(string(m[1]))
		}
		return ""
	}
	var product struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &product) != nil {
		return ""
	}
	return trimSpace(product.Version)
}

func atLeastMinimum(v string) string {
	if compareVersions(v, MinimumClientVersion) < 0 {
		return MinimumClientVersion
	}
	return v
}

// compareVersions compares dotted numeric versions; a part that is not a
// number makes the whole version count as unknown (lower).
func compareVersions(a, b string) int {
	pa, okA := versionParts(a)
	pb, okB := versionParts(b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return -1
	case !okB:
		return 1
	}
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) ([]int, bool) {
	fields := strings.Split(strings.SplitN(trimSpace(v), "-", 2)[0], ".")
	parts := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, len(parts) > 0
}

// clientPlatform is the Cloud Code ClientMetadata platform value.
func clientPlatform(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "DARWIN_ARM64"
	case "darwin/amd64":
		return "DARWIN_AMD64"
	case "windows/amd64":
		return "WINDOWS_AMD64"
	case "linux/amd64":
		return "LINUX_AMD64"
	case "linux/arm64":
		return "LINUX_ARM64"
	}
	return "PLATFORM_UNSPECIFIED"
}

// clientOS and clientArch are the names used in User-Agent headers.
func clientOS(goos string) string {
	switch goos {
	case "darwin", "windows":
		return goos
	}
	return "linux"
}

func clientArch(goarch string) string {
	if goarch == "arm64" {
		return "arm64"
	}
	return "amd64"
}

// ClientInstalled reports whether an Antigravity client is installed for
// home: the desktop app, the agy CLI on PATH, or the CLI's own directory.
func ClientInstalled(home string) bool {
	files := append(versionFiles(home), filepath.Join(home, officialDirName, officialCLIDirName))
	return clientInstalled(files, exec.LookPath)
}

func clientInstalled(files []string, lookPath func(string) (string, error)) bool {
	for _, path := range files {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	_, err := lookPath("agy")
	return err == nil
}
