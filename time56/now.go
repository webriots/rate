//go:build !windows

package time56

import _ "unsafe"

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// SystemNanoTime returns the current system time in nanoseconds. On
// non-Windows platforms, this uses a direct linkage to
// runtime.nanotime for improved performance.
func SystemNanoTime() int64 {
	return nanotime()
}
