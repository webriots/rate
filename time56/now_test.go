package time56

import (
	"testing"
	"time"
)

func TestSystemNanoTime(t *testing.T) {
	// Test that SystemNanoTime returns a non-zero value
	sysTime := SystemNanoTime()
	if sysTime <= 0 {
		t.Errorf("SystemNanoTime should return a positive value, got: %d", sysTime)
	}

	// Test multiple calls return increasing values
	first := SystemNanoTime()
	time.Sleep(time.Millisecond) // Sleep to ensure time changes
	second := SystemNanoTime()

	if first >= second {
		t.Errorf("Multiple calls to SystemNanoTime should return increasing values: first=%d, second=%d", first, second)
	}
}
