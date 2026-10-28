//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

// CreatePrivateDirectory attaches a protected owner-only ACL at creation.
// An existing path is an error, including a directory created concurrently.
func CreatePrivateDirectory(path string) error {
	sd, err := ownerSecurityDescriptor()
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	return windows.CreateDirectory(name, &attrs)
}
