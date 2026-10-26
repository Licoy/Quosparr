package claudedesktop

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Read only the documented-by-installed-code mode selector, never cookies,
// authentication stores, or unknown administrator/remote policy fields.
func verifyFirstPartyDeployment(paths []string) error {
	for _, root := range paths {
		path := filepath.Join(root, "claude_desktop_config.json")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.New("cannot verify Claude Desktop deployment mode")
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 4<<20 {
			return errors.New("Claude Desktop deployment configuration is not a bounded regular file")
		}
		if err := verifyDeploymentParents(path); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return errors.New("cannot verify Claude Desktop deployment mode")
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(info, opened) {
			file.Close()
			return errors.New("Claude Desktop deployment configuration changed while opening")
		}
		data, err := io.ReadAll(io.LimitReader(file, 4<<20+1))
		closeErr := file.Close()
		if err != nil || closeErr != nil || len(data) > 4<<20 {
			return errors.New("cannot verify Claude Desktop deployment mode")
		}
		var selector struct {
			DeploymentMode string `json:"deploymentMode"`
		}
		if json.Unmarshal(data, &selector) != nil {
			return errors.New("Claude Desktop deployment mode is invalid")
		}
		if selector.DeploymentMode == "3p" {
			return errors.New("Claude Desktop third-party deployments cannot be switched")
		}
		if selector.DeploymentMode != "1p" {
			return errors.New("Claude Desktop deployment mode cannot be determined")
		}
	}
	return nil
}

func thirdPartyConfigRoots(dataDir, platform string) []string {
	if platform != "windows" {
		return []string{dataDir + "-3p"}
	}
	// Windows keeps 3p configuration in LocalAppData, with an older Roaming
	// location supported by Desktop migration; native resolution supplies these.
	return nil
}

func verifyDeploymentParents(path string) error {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Claude Desktop deployment path is not a regular directory")
		}
		if filepath.Dir(dir) == dir {
			return nil
		}
	}
}
