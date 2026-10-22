//go:build !darwin && !windows

package app

import "github.com/egoist/mygo"

// nativeFloatingBelowFront: the native floating host runs on macOS and
// Windows.
func nativeFloatingBelowFront(*mygo.Window) (below, known bool) { return false, false }
