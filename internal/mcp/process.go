package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

func newProbeProcess(ctx context.Context, draft Draft) (*probeWire, error) {
	env := probeProcessEnv(draft.Env)
	command, err := probeExecutable(draft.Command, env)
	if err != nil {
		return nil, probeErr("process_start", "connect")
	}
	cmd := exec.Command(command, draft.Args...)
	cmd.Env = env
	cmd.Dir = draft.Cwd
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, probeErr("process_start", "connect")
	}
	out, outputWriter, err := os.Pipe()
	cmd.Stdout = outputWriter
	if err != nil {
		in.Close()
		return nil, probeErr("process_start", "connect")
	}
	// A real null-device file avoids an os/exec copy goroutine waiting forever
	// when a daemonized descendant inherits stderr. No stderr bytes are retained.
	stderr, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		in.Close()
		out.Close()
		outputWriter.Close()
		return nil, probeErr("process_start", "connect")
	}
	cmd.Stderr = stderr
	cleanup, err := startProbeProcess(cmd)
	stderr.Close()
	outputWriter.Close()
	if err != nil {
		in.Close()
		out.Close()
		return nil, probeErr("process_start", "connect")
	}
	frames := make(chan probeFrame, 32)
	streamCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), probeLimit)
		for scanner.Scan() {
			select {
			case frames <- probeFrame{data: scanner.Text()}:
			case <-streamCtx.Done():
				return
			}
		}
		if scanner.Err() != nil {
			select {
			case frames <- probeFrame{err: probeErr("response_too_large", "connect")}:
			case <-streamCtx.Done():
			}
		}
	}()
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	var once sync.Once
	w := &probeWire{}
	w.close = func() {
		once.Do(func() {
			cancel()
			in.Close()
			select {
			case <-done:
			case <-time.After(100 * time.Millisecond):
			}
			cleanup()
			out.Close()
			<-done
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			w.close()
		case <-streamCtx.Done():
		}
	}()
	id := 0
	w.call = func(callCtx context.Context, method string, params map[string]any, notification bool) (json.RawMessage, error) {
		id++
		payload := probeRequest(w.version, method, params, id, notification)
		written := make(chan error, 1)
		go func() { _, e := in.Write(payload); written <- e }()
		select {
		case <-callCtx.Done():
			return nil, probeContextError(callCtx, probeStage(method), callCtx.Err())
		case e := <-written:
			if e != nil {
				return nil, probeErr("process_exit", probeStage(method))
			}
		}
		if notification {
			return nil, nil
		}
		raw, e := probeReadFrames(callCtx, frames, id, method)
		if e != nil {
			select {
			case <-done:
				if waitErr != nil {
					code := -1
					if cmd.ProcessState != nil {
						code = cmd.ProcessState.ExitCode()
					}
					return nil, &ProbeError{Code: "process_exit", Stage: probeStage(method), ExitCode: code}
				}
			default:
			}
		}
		return raw, e
	}
	return w, nil
}

func probeProcessEnv(explicit map[string]string) []string {
	// HOME and platform runtime paths support normal launchers; no provider token,
	// QUOSPARR, CODEX_HOME, CLAUDE_CONFIG_DIR or application proxy variables leak.
	keys := []string{"PATH", "HOME", "USER", "LOGNAME", "TMPDIR", "TMP", "TEMP", "LANG", "LC_ALL", "LC_CTYPE", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PROGRAMFILES", "PROGRAMFILES(X86)", "PROGRAMDATA"}
	inherited := []string{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			inherited = append(inherited, k+"="+v)
		}
	}
	return probeMergeEnv(inherited, explicit, runtime.GOOS == "windows")
}

func probeMergeEnv(inherited []string, explicit map[string]string, fold bool) []string {
	values := map[string]string{}
	canonical := func(k string) string {
		if fold {
			return strings.ToUpper(k)
		}
		return k
	}
	for _, item := range inherited {
		k, v, ok := strings.Cut(item, "=")
		if ok {
			values[canonical(k)] = v
		}
	}
	// Sort explicit keys so even duplicate Windows spellings have deterministic
	// precedence; normal drafts should use only one spelling of a variable.
	keys := make([]string, 0, len(explicit))
	for k := range explicit {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		values[canonical(k)] = explicit[k]
	}
	env := make([]string, 0, len(values))
	for k, v := range values {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	return env
}
func probeExecutable(command string, env []string) (string, error) {
	if filepath.IsAbs(command) || strings.ContainsAny(command, "/\\") {
		return command, nil
	}
	path := ""
	extensions := []string{""}
	for _, item := range env {
		k, v, _ := strings.Cut(item, "=")
		if strings.EqualFold(k, "PATH") {
			path = v
		}
		if runtime.GOOS == "windows" && strings.EqualFold(k, "PATHEXT") {
			extensions = append(extensions, filepath.SplitList(v)...)
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		for _, ext := range extensions {
			candidate := filepath.Join(dir, command+ext)
			info, e := os.Stat(candidate)
			if e == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0111 != 0) {
				return candidate, nil
			}
		}
	}
	return "", os.ErrNotExist
}
