package claudedesktop

import "errors"

type desktopWindow struct {
	handle uintptr
	pid    uint32
}
type desktopCloseAPI struct {
	windows     func() ([]desktopWindow, error)
	sameProcess func() (bool, error)
	postClose   func(uintptr) error
}

func requestDesktopWindowsClose(pid uint32, api desktopCloseAPI) error {
	windows, err := api.windows()
	if err != nil {
		return err
	}
	matched := false
	for _, window := range windows {
		if window.pid != pid {
			continue
		}
		same, err := api.sameProcess()
		if err != nil {
			return err
		}
		if !same {
			return errors.New("Claude Desktop process identity changed before close request")
		}
		if err := api.postClose(window.handle); err != nil {
			return err
		}
		matched = true
	}
	if !matched {
		return errors.New("Claude Desktop has no window accepting a normal close request; quit it manually before switching")
	}
	return nil
}

func markWindowsWindowHost(process hostProcess, pid uint32, windows []desktopWindow) hostProcess {
	if process.terminate == nil {
		return process
	}
	for _, window := range windows {
		if window.pid == pid {
			return process
		}
	}
	process.terminate = nil
	return process
}
