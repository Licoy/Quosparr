package mcp

import (
	"bufio"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// Windows children cannot break away from the probe's job; the POSIX detached
// helper mode is only used by process_unix_test.go.
func probeTestDetach(cmd *exec.Cmd) {}

func probeTestChildExit(t *testing.T, conn net.Conn) func() {
	t.Helper()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal("missing child identity", err)
	}
	pid, err := strconv.ParseUint(strings.TrimSpace(line), 10, 32)
	if err != nil || pid == 0 {
		t.Fatal("invalid child identity")
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.CloseHandle(process) })
	return func() {
		t.Helper()
		// Job termination may reset TCP instead of sending EOF on Windows;
		// the process handle verifies actual exit independently of the socket.
		event, err := windows.WaitForSingleObject(process, 1000)
		if err != nil || event != windows.WAIT_OBJECT_0 {
			t.Fatalf("descendant did not exit: event=%d error=%v", event, err)
		}
	}
}
