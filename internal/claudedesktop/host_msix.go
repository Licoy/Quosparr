package claudedesktop

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type desktopMSIX struct{ Root, Family, AppID string }

func desktopMSIXLocation(executable string) (root, family string) {
	dir := filepath.Dir(executable)
	for range 3 {
		name := filepath.Base(dir)
		for _, known := range []struct{ name, publisher string }{{"Claude", "pzs8sxrjxfjjc"}, {"AnthropicPBC.Claude", "fnn82j28hfe8t"}} {
			if strings.HasPrefix(strings.ToLower(name), strings.ToLower(known.name)+"_") && strings.HasSuffix(strings.ToLower(name), "__"+known.publisher) {
				return dir, known.name + "_" + known.publisher
			}
		}
		dir = filepath.Dir(dir)
	}
	return "", ""
}

func readDesktopMSIX(executable string) (desktopMSIX, error) {
	root, family := desktopMSIXLocation(executable)
	if root == "" {
		return desktopMSIX{}, nil
	}
	file, err := os.Open(filepath.Join(root, "AppxManifest.xml"))
	if err != nil {
		return desktopMSIX{}, errors.New("cannot read Claude Desktop MSIX identity")
	}
	defer file.Close()
	return parseDesktopMSIX(io.LimitReader(file, 2<<20), executable, desktopMSIX{Root: root, Family: family})
}

func parseDesktopMSIX(reader io.Reader, executable string, identity desktopMSIX) (desktopMSIX, error) {
	var manifest struct {
		Identity struct {
			Name string `xml:"Name,attr"`
		} `xml:"Identity"`
		Apps []struct {
			ID         string `xml:"Id,attr"`
			Executable string `xml:"Executable,attr"`
		} `xml:"Applications>Application"`
	}
	if xml.NewDecoder(reader).Decode(&manifest) != nil {
		return desktopMSIX{}, errors.New("invalid Claude Desktop MSIX manifest")
	}
	name := strings.TrimSuffix(identity.Family, "_"+identity.Family[strings.LastIndexByte(identity.Family, '_')+1:])
	if manifest.Identity.Name != name {
		return desktopMSIX{}, errors.New("Claude Desktop MSIX publisher identity does not match installation")
	}
	for _, app := range manifest.Apps {
		relative := strings.ReplaceAll(app.Executable, `\`, string(filepath.Separator))
		if relative == "" || filepath.IsAbs(relative) || strings.HasPrefix(filepath.Clean(relative), "..") {
			continue
		}
		if sameExecutable(filepath.Join(identity.Root, relative), executable, "windows") {
			if identity.AppID != "" || app.ID == "" || strings.ContainsAny(app.ID, `\/!`) {
				return desktopMSIX{}, errors.New("Claude Desktop MSIX application identity is ambiguous")
			}
			identity.AppID = app.ID
		}
	}
	if identity.AppID == "" {
		return desktopMSIX{}, errors.New("Claude Desktop executable is not registered in the MSIX manifest")
	}
	return identity, nil
}

// Broker activation has no environment parameter, so Quosparr secrets and
// custom userData overrides cannot be forwarded to the Windows service.
type desktopActivationAPI struct {
	activate func(string) (uint32, error)
	child    func(uint32, string) (desktopChild, error)
}

func startDesktopMSIX(identity desktopMSIX, exe string, api desktopActivationAPI) (desktopChild, error) {
	if identity.Family == "" || identity.AppID == "" {
		return nil, errors.New("Claude Desktop package identity is incomplete")
	}
	pid, err := api.activate(identity.Family + "!" + identity.AppID)
	if err != nil {
		return nil, err
	}
	if pid == 0 {
		return nil, errors.New("Claude Desktop package activation returned no process")
	}
	return api.child(pid, exe)
}
