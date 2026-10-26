package platform

import (
	"errors"
	"runtime"
	"syscall"
)

// NativeErrorKind classifies verified Windows system error numbers without
// reading external error text or calling system APIs. An empty kind is unknown.
func NativeErrorKind(err error) string { return nativeErrorKind(err, runtime.GOOS) }

func nativeErrorKind(err error, goos string) string {
	if goos != "windows" {
		return ""
	}
	// Win32 system errors and Winsock errors use native values, unlike Go's
	// synthetic Windows POSIX errno constants.
	for _, entry := range []struct {
		code syscall.Errno
		kind string
	}{
		{19, "read_only"},             // ERROR_WRITE_PROTECT
		{39, "disk_full"},             // ERROR_HANDLE_DISK_FULL
		{112, "disk_full"},            // ERROR_DISK_FULL
		{10048, "address_in_use"},     // WSAEADDRINUSE
		{10060, "timeout"},            // WSAETIMEDOUT
		{10061, "connection_refused"}, // WSAECONNREFUSED
	} {
		if errors.Is(err, entry.code) {
			return entry.kind
		}
	}
	return ""
}
