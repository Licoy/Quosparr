package mcp

import "os/exec"

// Windows children cannot break away from the probe's job; the POSIX detached
// helper mode is only used by process_unix_test.go.
func probeTestDetach(cmd *exec.Cmd) {}
