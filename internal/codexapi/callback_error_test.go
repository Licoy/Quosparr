package codexapi

import (
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestCallbackListenErrorPreservesSafeCause(t *testing.T) {
	for _, cause := range []error{syscall.EADDRINUSE, os.ErrPermission} {
		err := &CallbackListenError{Cause: &net.OpError{Op: "private-operation", Err: &os.PathError{Path: "/private/callback-secret", Err: cause}}}
		if !errors.Is(err, ErrPortInUse) || !errors.Is(err, cause) {
			t.Fatal("callback listener lost compatibility or underlying cause")
		}
		if strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), cause.Error()) {
			t.Fatal("callback listener exposed unclassified cause text")
		}
		if cause == os.ErrPermission && errors.Is(err, syscall.EADDRINUSE) {
			t.Fatal("permission failure was classified as address already in use")
		}
	}
}
