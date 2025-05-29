package rate

import (
	"math"
	"testing"
)

// TestAtomicSliceUint64Basic verifies basic operations for atomicSliceUint64.
func TestAtomicSliceUint64Basic(t *testing.T) {
	// Create a slice of length 3
	slice := newAtomicSliceUint64(3)

	// Initial values should be zero
	if got := slice.Get(0); got != 0 {
		t.Errorf("Initial value: got %d, want 0", got)
	}

	// Set and Get
	slice.Set(1, 42)
	if got := slice.Get(1); got != 42 {
		t.Errorf("After Set: got %d, want 42", got)
	}

	// CompareAndSet success
	ok := slice.CompareAndSwap(1, 42, 100)
	if !ok {
		t.Error("CompareAndSwap should succeed")
	}
	if got := slice.Get(1); got != 100 {
		t.Errorf("After CompareAndSwap: got %d, want 100", got)
	}

	// CompareAndSet failure
	ok = slice.CompareAndSwap(1, 42, 200)
	if ok {
		t.Error("CompareAndSwap should fail")
	}
	if got := slice.Get(1); got != 100 {
		t.Errorf("After failed CompareAndSwap: got %d, should still be 100", got)
	}

	// Check Len
	if got := slice.Len(); got != 3 {
		t.Errorf("Len: got %d, want 3", got)
	}
}

// TestAtomicSliceFloat64Basic verifies basic operations for atomicSliceFloat64.
func TestAtomicSliceFloat64Basic(t *testing.T) {
	// Create a slice of length 3
	slice := newAtomicSliceFloat64(3)

	// Initial values should be zero
	if got := slice.Get(0); got != 0.0 {
		t.Errorf("Initial value: got %f, want 0.0", got)
	}

	// Set and Get
	slice.Set(1, 42.5)
	if got := slice.Get(1); got != 42.5 {
		t.Errorf("After Set: got %f, want 42.5", got)
	}

	// CompareAndSet success
	ok := slice.CompareAndSwap(1, 42.5, 100.5)
	if !ok {
		t.Error("CompareAndSwap should succeed")
	}
	if got := slice.Get(1); got != 100.5 {
		t.Errorf("After CompareAndSwap: got %f, want 100.5", got)
	}

	// CompareAndSet failure
	ok = slice.CompareAndSwap(1, 42.5, 200.5)
	if ok {
		t.Error("CompareAndSwap should fail")
	}
	if got := slice.Get(1); got != 100.5 {
		t.Errorf("After failed CompareAndSwap: got %f, should still be 100.5", got)
	}

	// Check Len
	if got := slice.Len(); got != 3 {
		t.Errorf("Len: got %d, want 3", got)
	}

	// Test with special float values
	slice.Set(0, math.NaN())
	if !math.IsNaN(slice.Get(0)) {
		t.Error("Expected NaN value")
	}

	slice.Set(1, math.Inf(1))
	if !math.IsInf(slice.Get(1), 1) {
		t.Error("Expected positive infinity")
	}

	slice.Set(2, math.Inf(-1))
	if !math.IsInf(slice.Get(2), -1) {
		t.Error("Expected negative infinity")
	}
}
