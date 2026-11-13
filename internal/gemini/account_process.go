package gemini

import (
	"context"
	"path"
	"strconv"
	"strings"
)

// antigravityProcess is one running Antigravity client process.
type antigravityProcess struct {
	pid    int
	client string // SessionClientApp or SessionClientCLI
}

// parseProcesses reads `ps -axo pid=,comm=` output. comm is the executable
// path on macOS (it may contain spaces) and the short name on Linux.
func parseProcesses(output string, self int) []antigravityProcess {
	var found []antigravityProcess
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
		if client := antigravityClient(strings.TrimSpace(line[cut:]), false); client != "" {
			found = append(found, antigravityProcess{pid: pid, client: client})
		}
	}
	return found
}

func parseProcessIDs(output string, self int) []int {
	return processIDs(parseProcesses(output, self))
}

func processIDs(found []antigravityProcess) []int {
	var pids []int
	for _, p := range found {
		pids = append(pids, p.pid)
	}
	return pids
}

// RunningClients reports which Antigravity clients have a process: the
// desktop App (SessionClientApp) and agy (SessionClientCLI).
func RunningClients(ctx context.Context) (map[string]bool, error) {
	found, err := systemProcesses(ctx)
	if err != nil {
		return nil, err
	}
	running := map[string]bool{}
	for _, p := range found {
		running[p.client] = true
	}
	return running, nil
}

// antigravityClient matches the desktop app (executable "Antigravity",
// macOS bundle com.google.antigravity at …/Antigravity.app/Contents/MacOS/)
// and the agy CLI. Helper processes end with their parent and are ignored.
func antigravityClient(executable string, windows bool) string {
	if windows {
		executable = strings.ReplaceAll(executable, `\`, "/")
	}
	name := strings.ToLower(path.Base(executable))
	if windows {
		name = strings.TrimSuffix(name, ".exe")
	}
	switch name {
	case "antigravity":
		return SessionClientApp
	case "agy":
		return SessionClientCLI
	}
	return ""
}

func isAntigravityExecutable(executable string, windows bool) bool {
	return antigravityClient(executable, windows) != ""
}

func windowsProcess(pid uint32, executable string, self int) (antigravityProcess, bool) {
	if pid == uint32(self) {
		return antigravityProcess{}, false
	}
	client := antigravityClient(executable, true)
	return antigravityProcess{pid: int(pid), client: client}, client != ""
}
