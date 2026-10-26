//go:build linux

package claudedesktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func NewNativeHost(home string) *NativeHost {
	h := newHost(home, "linux")
	h.candidates = func(context.Context) ([]string, error) {
		return []string{"/usr/bin/claude-desktop", "/usr/local/bin/claude-desktop", "/opt/Claude/claude-desktop", "/opt/claude-desktop/claude-desktop", "/usr/lib/claude-desktop/claude-desktop"}, nil
	}
	h.dataDir = func(context.Context, string) (string, error) {
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		if !filepath.IsAbs(config) {
			return "", errors.New("XDG_CONFIG_HOME must be an absolute path")
		}
		dir := filepath.Join(config, "Claude")
		return dir, verifyFirstPartyDeployment(thirdPartyConfigRoots(dir, "linux"))
	}
	h.processes = func(ctx context.Context, app Application) ([]hostProcess, error) {
		return linuxDesktopProcesses(ctx, app, "/proc", uint32(os.Getuid()))
	}
	return h
}

func linuxDesktopProcesses(ctx context.Context, app Application, root string, uid uint32) (result []hostProcess, err error) {
	entries, err := os.ReadDir(root)
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
		pid, e := strconv.Atoi(entry.Name())
		if e != nil || pid <= 0 {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		info, e := os.Stat(dir)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return result, e
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return result, errors.New("cannot inspect Linux process owner")
		}
		if owner.Uid != uid {
			continue
		}
		exe, e := os.Readlink(filepath.Join(dir, "exe"))
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return result, fmt.Errorf("inspect Linux process executable: %w", e)
		}
		if strings.HasSuffix(exe, " (deleted)") && belongsToApplication(strings.TrimSuffix(exe, " (deleted)"), app, "linux") {
			return result, errors.New("Claude Desktop was updated while running; quit it manually before switching")
		}
		if !belongsToApplication(exe, app, "linux") {
			if err := rejectOtherDesktop(ctx, exe, app, "linux", runHostCommand); err != nil {
				return result, err
			}
			continue
		}
		process, e := openLinuxDesktopProcess(pid, dir, exe, app)
		if e != nil {
			return result, e
		}
		result = append(result, process)
	}
	return result, nil
}

func linuxProcessBirth(dir string) (string, error) {
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return "", err
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return "", errors.New("invalid Linux process identity")
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) <= 19 {
		return "", errors.New("incomplete Linux process identity")
	}
	return fields[19], nil // field 22, after pid and the parenthesized command
}

func openLinuxDesktopProcess(pid int, dir, exe string, app Application) (hostProcess, error) {
	birth, err := linuxProcessBirth(dir)
	if err != nil {
		return hostProcess{}, err
	}
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return hostProcess{}, fmt.Errorf("cannot safely track Claude Desktop process with pidfd: %w", err)
	}
	current, err := linuxProcessBirth(dir)
	actual, e := os.Readlink(filepath.Join(dir, "exe"))
	if err != nil || e != nil || birth != current || actual != exe {
		unix.Close(fd)
		return hostProcess{}, errors.New("Claude Desktop process changed while inspecting it")
	}
	process := hostProcess{executable: exe, close: func() { unix.Close(fd) }}
	process.exited = func() (bool, error) {
		poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err := unix.Poll(poll, 0)
		if err != nil {
			return false, err
		}
		if poll[0].Revents&unix.POLLNVAL != 0 {
			return false, errors.New("Claude Desktop process handle became invalid")
		}
		return poll[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0, nil
	}
	helper, err := desktopHelperCommand(filepath.Join(dir, "cmdline"))
	if err != nil {
		unix.Close(fd)
		return hostProcess{}, err
	}
	if sameExecutable(exe, app.Executable, "linux") && !helper {
		process.terminate = func() error {
			exited, err := process.exited()
			if err != nil || exited {
				return err
			}
			// Recheck exec identity too: pidfd prevents PID reuse, while an exec
			// could replace the image without changing the numeric process identity.
			now, identityErr := linuxProcessBirth(dir)
			image, imageErr := os.Readlink(filepath.Join(dir, "exe"))
			if identityErr != nil || imageErr != nil || now != birth || image != exe {
				return errors.New("Claude Desktop process identity changed before termination")
			}
			// The pidfd refers to this exact process even if its numeric PID is reused.
			err = unix.PidfdSendSignal(fd, unix.SIGTERM, nil, 0)
			if errors.Is(err, unix.ESRCH) {
				return nil
			}
			return err
		}
	}
	return process, nil
}
