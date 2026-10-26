package platform

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestNativeErrorKindWindowsNumbers(t *testing.T) {
	for _, tc := range []struct {
		code syscall.Errno
		want string
	}{
		{19, "read_only"}, {39, "disk_full"}, {112, "disk_full"},
		{10048, "address_in_use"}, {10060, "timeout"}, {10061, "connection_refused"},
	} {
		for _, err := range []error{tc.code, &os.PathError{Op: "write", Path: "/private-secret", Err: tc.code}, errors.Join(errors.New("unknown"), tc.code)} {
			if got := nativeErrorKind(err, "windows"); got != tc.want {
				t.Fatalf("code %d kind %q, want %q", tc.code, got, tc.want)
			}
			for _, goos := range []string{"darwin", "linux"} {
				if got := nativeErrorKind(err, goos); got != "" {
					t.Fatalf("native Windows code interpreted on %s", goos)
				}
			}
		}
	}
}
func TestNativeErrorKindDoesNotGuess(t *testing.T) {
	for _, err := range []error{nil, syscall.Errno(99999), errors.New("write: disk full (112)"), fmt.Errorf("timeout: %s", "10060")} {
		if got := nativeErrorKind(err, "windows"); got != "" {
			t.Fatalf("unknown or text-only error guessed as %s", got)
		}
	}
	want := ""
	if runtime.GOOS == "windows" {
		want = "disk_full"
	}
	if got := NativeErrorKind(syscall.Errno(112)); got != want {
		t.Fatalf("runtime dispatch = %q, want %q", got, want)
	}
}
