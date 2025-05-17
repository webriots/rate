//go:build windows

package time56

import "time"

// SystemNanoTime returns the current system time in nanoseconds. On
// Windows platforms, this uses time.Now().UnixNano() as the direct
// linkage to runtime.nanotime is not available.
func SystemNanoTime() int64 {
	return time.Now().UnixNano()
}
