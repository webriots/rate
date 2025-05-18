package rate

import "testing"

// TestAtomicSliceBasic verifies basic Get, Set, CompareAndSet, and Max operations.
func TestAtomicSliceBasic(t *testing.T) {
	// Create a slice of length 3
	slice := newAtomicSliceInt64(3)

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
