package rate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAtomicSliceBasic verifies basic Get, Set, CompareAndSet, and Max operations.
func TestAtomicSliceBasic(t *testing.T) {
	r := require.New(t)
	// Create a slice of length 3
	a := newAtomicSliceInt64(3)
	// Initial values should be zero
	r.Equal(int64(0), a.Get(0))
	// Set and Get
	a.Set(1, 42)
	r.Equal(int64(42), a.Get(1))
	// CompareAndSet success
	ok := a.CompareAndSwap(1, 42, 100)
	r.True(ok)
	r.Equal(int64(100), a.Get(1))
	// CompareAndSet failure
	ok = a.CompareAndSwap(1, 42, 200)
	r.False(ok)
	r.Equal(int64(100), a.Get(1))
}

// TestAtomicSliceUint64Basic verifies basic operations for atomicSliceUint64.
func TestAtomicSliceUint64Basic(t *testing.T) {
	r := require.New(t)
	// Create a slice of length 3
	a := newAtomicSliceUint64(3)
	// Initial values should be zero
	r.Equal(uint64(0), a.Get(0))
	// Set and Get
	a.Set(1, 42)
	r.Equal(uint64(42), a.Get(1))
	// CompareAndSet success
	ok := a.CompareAndSwap(1, 42, 100)
	r.True(ok)
	r.Equal(uint64(100), a.Get(1))
	// CompareAndSet failure
	ok = a.CompareAndSwap(1, 42, 200)
	r.False(ok)
	r.Equal(uint64(100), a.Get(1))
}
