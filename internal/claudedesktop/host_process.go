package claudedesktop

import (
	"bytes"
	"errors"
	"io"
	"os"
)

func desktopHelperCommand(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, errors.New("cannot identify Claude Desktop process role")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 256<<10+1))
	if err != nil || len(data) == 0 || len(data) > 256<<10 {
		return false, errors.New("Claude Desktop process role cannot be determined")
	}
	for _, argument := range bytes.Split(data, []byte{0}) {
		if bytes.Equal(argument, []byte("--type")) || bytes.HasPrefix(argument, []byte("--type=")) {
			return true, nil
		}
	}
	return false, nil
}
