package time56

import _ "unsafe"

//go:noescape
//go:linkname nanotime runtime.nanotime
func nanotime() int64

// SystemNanoTime returns a monotonic system time in nanoseconds. This
// uses a direct linkage to runtime.nanotime for improved performance.
func SystemNanoTime() int64 {
	return nanotime()
}
