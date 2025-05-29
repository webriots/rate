package rate

import (
	"math"
	"sync/atomic"
)

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

// Len returns the length of the atomic slice.
func (a atomicSliceUint64) Len() int {
	return len(a)
}

// atomicSliceFloat64 is a slice of float64 values with atomic
// operations. It provides thread-safe access to elements in the
// slice. Float64 values are stored as uint64 bits for atomic
// operations.
type atomicSliceFloat64 []uint64

// newAtomicSliceFloat64 creates a new atomicSliceFloat64 with the
// specified size.
func newAtomicSliceFloat64(size int) atomicSliceFloat64 {
	return make([]uint64, size)
}

// Get atomically loads and returns the float64 value at the specified
// index.
func (a atomicSliceFloat64) Get(index int) float64 {
	bits := atomic.LoadUint64(&a[index])
	return math.Float64frombits(bits)
}

// Set atomically stores the float64 value at the specified index.
func (a atomicSliceFloat64) Set(index int, value float64) {
	bits := math.Float64bits(value)
	atomic.StoreUint64(&a[index], bits)
}

// CompareAndSwap atomically compares the float64 value at the
// specified index with old and swaps it with new if they match.
// Returns true if the swap occurred.
func (a atomicSliceFloat64) CompareAndSwap(index int, old, new float64) bool {
	oldBits := math.Float64bits(old)
	newBits := math.Float64bits(new)
	return atomic.CompareAndSwapUint64(&a[index], oldBits, newBits)
}

// Len returns the length of the atomic slice.
func (a atomicSliceFloat64) Len() int {
	return len(a)
}
