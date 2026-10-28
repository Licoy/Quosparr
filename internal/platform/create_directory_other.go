//go:build !windows

package platform

import "os"

// CreatePrivateDirectory creates exactly one new private directory and refuses an existing path.
func CreatePrivateDirectory(path string) error {
	return os.Mkdir(path, 0700)
}
