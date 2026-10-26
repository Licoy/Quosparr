//go:build darwin

package claudedesktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"golang.org/x/sys/unix"
)

func NewNativeHost(home string) *NativeHost {
	h := newHost(home, "darwin")
	h.candidates = func(context.Context) ([]string, error) {
		return []string{"/Applications/Claude.app", filepath.Join(home, "Applications", "Claude.app")}, nil
	}
	h.dataDir = func(context.Context, string) (string, error) {
		dir := filepath.Join(home, "Library", "Application Support", "Claude")
		return dir, verifyFirstPartyDeployment(thirdPartyConfigRoots(dir, "darwin"))
	}
	h.processes = darwinDesktopProcesses
	return h
}

func darwinDesktopProcesses(ctx context.Context, app Application) (result []hostProcess, err error) {
	library, err := purego.Dlopen("/usr/lib/libproc.dylib", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("inspect Claude Desktop processes: %w", err)
	}
	defer purego.Dlclose(library)
	var pidPath func(int32, unsafe.Pointer, uint32) int32
	purego.RegisterLibFunc(&pidPath, library, "proc_pidpath")
	entries, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			closeHostProcesses(result)
			result = nil
		}
	}()
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if entry.Eproc.Ucred.Uid != uint32(os.Getuid()) || entry.Proc.P_pid <= 0 {
			continue
		}
		var buffer [4096]byte
		n := pidPath(entry.Proc.P_pid, unsafe.Pointer(&buffer[0]), uint32(len(buffer)))
		if n <= 0 || n > int32(len(buffer)) {
			current, e := unix.SysctlKinfoProc("kern.proc.pid", int(entry.Proc.P_pid))
			if errors.Is(e, unix.ESRCH) || errors.Is(e, unix.ENOENT) || e == nil && (current.Proc.P_pid == 0 || current.Proc.P_stat == 5) {
				continue
			}
			return result, errors.New("cannot verify a current-user process executable")
		}
		exe := strings.TrimRight(string(buffer[:n]), "\x00")
		if !belongsToApplication(exe, app, "darwin") {
			if err := rejectOtherDesktop(ctx, exe, app, "darwin", runHostCommand); err != nil {
				return result, err
			}
			continue
		}
		pid, birth := entry.Proc.P_pid, entry.Proc.P_starttime
		process := hostProcess{executable: exe, close: func() {}}
		process.exited = func() (bool, error) {
			current, e := unix.SysctlKinfoProc("kern.proc.pid", int(pid))
			if errors.Is(e, unix.ESRCH) || errors.Is(e, unix.ENOENT) {
				return true, nil
			}
			if e != nil {
				return false, e
			}
			return current.Proc.P_pid != pid || current.Proc.P_starttime != birth || current.Proc.P_stat == 5, nil
		}
		if sameExecutable(exe, app.Executable, "darwin") {
			terminate, release, e := darwinApplicationTermination(entry)
			if e != nil {
				return result, e
			}
			process.close = release
			process.terminate = func() error {
				exited, e := process.exited()
				if e != nil || exited {
					return e
				}
				return terminate()
			}
		}
		result = append(result, process)
	}
	return result, nil
}

// Retain the NSRunningApplication instance before signaling so PID reuse
// cannot retarget a later lookup at an unrelated application.
func darwinApplicationTermination(entry unix.KinfoProc) (terminate func() error, release func(), err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	framework, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, nil, err
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	app := objc.ID(objc.GetClass("NSRunningApplication")).Send(objc.RegisterName("runningApplicationWithProcessIdentifier:"), entry.Proc.P_pid)
	if app == 0 {
		purego.Dlclose(framework)
		return nil, nil, errors.New("Claude Desktop process is not a running application")
	}
	app.Send(objc.RegisterName("retain"))
	release = func() { app.Send(objc.RegisterName("release")); purego.Dlclose(framework) }
	current, err := unix.SysctlKinfoProc("kern.proc.pid", int(entry.Proc.P_pid))
	if err != nil || current.Proc.P_starttime != entry.Proc.P_starttime || current.Eproc.Ucred.Uid != entry.Eproc.Ucred.Uid {
		release()
		return nil, nil, errors.New("Claude Desktop process identity changed")
	}
	terminate = func() error {
		if objc.Send[bool](app, objc.RegisterName("isTerminated")) {
			return nil
		}
		if !objc.Send[bool](app, objc.RegisterName("terminate")) {
			return errors.New("Claude Desktop refused termination")
		}
		return nil
	}
	return terminate, release, nil
}
