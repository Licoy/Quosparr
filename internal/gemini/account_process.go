package gemini

import (
	"path"
	"strconv"
	"strings"
)

// parseProcessIDs reads `ps -axo pid=,comm=` output. comm is the executable
// path on macOS (it may contain spaces) and the short name on Linux.
func parseProcessIDs(output string, self int) []int {
	var pids []int
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		cut := strings.IndexAny(line, " \t")
		if cut <= 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:cut])
		if err != nil || pid == self {
			continue
		}
		if isAntigravityExecutable(strings.TrimSpace(line[cut:]), false) {
			pids = append(pids, pid)
		}
	}
	return pids
}

// isAntigravityExecutable matches the desktop app (executable "Antigravity",
// macOS bundle com.google.antigravity at …/Antigravity.app/Contents/MacOS/)
// and the agy CLI. Helper processes end with their parent and are ignored.
func isAntigravityExecutable(executable string, windows bool) bool {
	if windows {
		executable = strings.ReplaceAll(executable, `\`, "/")
	}
	name := strings.ToLower(path.Base(executable))
	if windows {
		name = strings.TrimSuffix(name, ".exe")
	}
	return name == "antigravity" || name == "agy"
}

func windowsProcessID(pid uint32, executable string, self int) (int, bool) {
	if pid == uint32(self) || !isAntigravityExecutable(executable, true) {
		return 0, false
	}
	return int(pid), true
}
