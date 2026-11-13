package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const lobeVersion = "1.95.1"
const lobeIntegrity = "Hw7EPPgVnC4NZLXBfTNJG6hyQgqECfUPC11VVXodPSr1aebKcFxDZlSpxhWwYNdCc6bhxps/x5TtXoPmfKH2ag=="

var providerFiles = []string{"codex-color.svg", "codex.svg", "grok.svg", "claude-color.svg", "claude.svg", "gemini-color.svg", "gemini.svg"}

// writeProviders copies the provider SVGs, unchanged, from the pinned
// official LobeHub package; building the app uses the embedded local files.
func writeProviders(root string) error {
	archive := filepath.Join(os.TempDir(), "quosparr-icons", "lobehub-icons-static-svg-"+lobeVersion+".tgz")
	if _, err := os.Stat(archive); os.IsNotExist(err) {
		url := "https://registry.npmjs.org/@lobehub/icons-static-svg/-/icons-static-svg-" + lobeVersion + ".tgz"
		if err := download(url, archive); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	icons, err := readProviders(data)
	if err != nil {
		return err
	}
	dest := filepath.Join(root, "internal/app/assets/providers")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, name := range providerFiles {
		if err := os.WriteFile(filepath.Join(dest, name), icons[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readProviders(data []byte) (map[string][]byte, error) {
	digest := sha512.Sum512(data)
	if base64.StdEncoding.EncodeToString(digest[:]) != lobeIntegrity {
		return nil, fmt.Errorf("LobeHub Icons %s archive integrity mismatch", lobeVersion)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	icons := make(map[string][]byte, len(providerFiles))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		for _, name := range providerFiles {
			if h.Typeflag != tar.TypeReg || h.Name != "package/icons/"+name {
				continue
			}
			icons[name], err = io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, name := range providerFiles {
		if len(icons[name]) == 0 {
			return nil, fmt.Errorf("LobeHub Icons %s missing %s", lobeVersion, name)
		}
	}
	return icons, nil
}
