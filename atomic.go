package rate

import "sync/atomic"

// atomicSliceInt64 is a slice of int64 values with atomic operations.
// It provides thread-safe access to elements in the slice.
type atomicSliceInt64 []int64

// newAtomicSliceInt64 creates a new atomicSliceInt64 with the
// specified size.
func newAtomicSliceInt64(size int) atomicSliceInt64 {
	return make([]int64, size)
}

// Get atomically loads and returns the value at the specified index.
func (a atomicSliceInt64) Get(index int) int64 {
	return atomic.LoadInt64(&a[index])
}

// Set atomically stores the value at the specified index.
func (a atomicSliceInt64) Set(index int, value int64) {
	atomic.StoreInt64(&a[index], value)
}

// CompareAndSwap atomically compares the value at the specified index
// with old and swaps it with new if they match. Returns true if the
// swap occurred.
func (a atomicSliceInt64) CompareAndSwap(index int, old, new int64) bool {
	return atomic.CompareAndSwapInt64(&a[index], old, new)
}

// atomicSliceUint64 is a slice of uint64 values with atomic
// operations. It provides thread-safe access to elements in the
// slice.
type atomicSliceUint64 []uint64

// newAtomicSliceUint64 creates a new atomicSliceUint64 with the
// specified size.
func newAtomicSliceUint64(size int) atomicSliceUint64 {
	return make([]uint64, size)
}

// Get atomically loads and returns the value at the specified index.
func (a atomicSliceUint64) Get(index int) uint64 {
	return atomic.LoadUint64(&a[index])
}

// Set atomically stores the value at the specified index.
func (a atomicSliceUint64) Set(index int, value uint64) {
	atomic.StoreUint64(&a[index], value)
}

// CompareAndSwap atomically compares the value at the specified index
// with old and swaps it with new if they match. Returns true if the
// swap occurred.
func (a atomicSliceUint64) CompareAndSwap(index int, old, new uint64) bool {
	return atomic.CompareAndSwapUint64(&a[index], old, new)
}
