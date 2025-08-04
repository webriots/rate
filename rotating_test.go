package rate

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webriots/rate/time56"
)

const (
	rotatingBurstCapacity = uint8(10)
	rotatingNumBuckets    = uint(8)
	rotatingRatePerSecond = 1.0
	rotatingRotationRate  = 100 * time.Millisecond
)

func DefaultRotatingLimiter() (*RotatingTokenBucketRateLimiter, error) {
	return NewRotatingTokenBucketRateLimiter(
		rotatingNumBuckets,
		rotatingBurstCapacity,
		rotatingRatePerSecond,
		time.Second,
		rotatingRotationRate,
	)
}

// TestRotatingTokenBucketLimiterCreation tests limiter creation and validation
func TestRotatingTokenBucketLimiterCreation(t *testing.T) {
	tests := []struct {
		name              string
		numBuckets        uint
		burstCapacity     uint8
		refillRate        float64
		refillRateUnit    time.Duration
		rotationRate      time.Duration
		expectError       bool
		expectedErrorText string
	}{
		{
			name:           "valid params",
			numBuckets:     8,
			burstCapacity:  10,
			refillRate:     1.0,
			refillRateUnit: time.Second,
			rotationRate:   100 * time.Millisecond,
			expectError:    false,
		},
		{
			name:              "zero rotation rate",
			numBuckets:        8,
			burstCapacity:     10,
			refillRate:        1.0,
			refillRateUnit:    time.Second,
			rotationRate:      0,
			expectError:       true,
			expectedErrorText: "rotationRate must represent a positive duration",
		},
		{
			name:              "negative rotation rate",
			numBuckets:        8,
			burstCapacity:     10,
			refillRate:        1.0,
			refillRateUnit:    time.Second,
			rotationRate:      -100 * time.Millisecond,
			expectError:       true,
			expectedErrorText: "rotationRate must represent a positive duration",
		},
		{
			name:              "underlying limiter error - negative refill rate",
			numBuckets:        8,
			burstCapacity:     10,
			refillRate:        -1.0,
			refillRateUnit:    time.Second,
			rotationRate:      100 * time.Millisecond,
			expectError:       true,
			expectedErrorText: "refillRate must be a positive, finite number",
		},
		{
			name:              "underlying limiter error - zero refill rate unit",
			numBuckets:        8,
			burstCapacity:     10,
			refillRate:        1.0,
			refillRateUnit:    0,
			rotationRate:      100 * time.Millisecond,
			expectError:       true,
			expectedErrorText: "refillRateUnit must represent a positive duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := NewRotatingTokenBucketRateLimiter(
				tt.numBuckets,
				tt.burstCapacity,
				tt.refillRate,
				tt.refillRateUnit,
				tt.rotationRate,
			)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if err.Error() != tt.expectedErrorText {
					t.Errorf("Expected error %q, got %q", tt.expectedErrorText, err.Error())
				}
				if limiter != nil {
					t.Errorf("Expected nil limiter on error, got %v", limiter)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if limiter == nil {
					t.Errorf("Expected non-nil limiter")
				}
			}
		})
	}
}

// TestRotatingTokenBucketLimiterBasicFunctionality tests basic token operations
func TestRotatingTokenBucketLimiterBasicFunctionality(t *testing.T) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("test-id")

	// Should be able to make burstCapacity calls (each consumes 1 token from each limiter)
	for i := 0; i < int(rotatingBurstCapacity); i++ {
		if !limiter.TakeToken(id) {
			t.Errorf("Should be able to take token %d", i)
		}
	}

	// Should be rate limited after burst
	if limiter.TakeToken(id) {
		t.Error("Should be rate limited after burst")
	}

	// Check should also return false
	if limiter.Check(id) {
		t.Error("Check should return false after burst")
	}

	// After some time, should be able to take tokens again
	tick(2 * time.Second) // Wait for refill

	if !limiter.TakeToken(id) {
		t.Error("Should be able to take token after refill")
	}
}

// TestRotatingTokenBucketLimiterImplementsInterface verifies interface compliance
func TestRotatingTokenBucketLimiterImplementsInterface(t *testing.T) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	var _ Limiter = limiter // Should compile if interface is implemented
}

// TestRotatingTokenBucketLimiterRotation tests bucket rotation behavior
func TestRotatingTokenBucketLimiterRotation(t *testing.T) {
	rotationRate := 50 * time.Millisecond
	limiter, err := NewRotatingTokenBucketRateLimiter(
		rotatingNumBuckets,
		rotatingBurstCapacity,
		rotatingRatePerSecond,
		time.Second,
		rotationRate,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// Load initial pair
	pair1 := limiter.load(nowfn())
	if pair1 == nil {
		t.Fatal("Expected non-nil pair")
	}

	// Before rotation period, should get same pair
	pair2 := limiter.load(nowfn())
	if pair1 != pair2 {
		t.Error("Should get same pair before rotation period")
	}

	// After rotation period, should get new pair
	tick(rotationRate + 1*time.Millisecond)
	pair3 := limiter.load(nowfn())
	if pair1 == pair3 {
		t.Error("Should get different pair after rotation period")
	}

	// New pair should have checked = old ignored, and new ignored
	if pair3.checked != pair1.ignored {
		t.Error("New pair's checked should be old pair's ignored")
	}

	// New ignored should have different seed than checked
	if pair3.ignored.seed == pair3.checked.seed {
		t.Error("New ignored should have different seed than checked")
	}
}

// TestRotatingTokenBucketLimiterCollisionAvoidance tests collision handling
func TestRotatingTokenBucketLimiterCollisionAvoidance(t *testing.T) {
	rotationRate := 50 * time.Millisecond
	limiter, err := NewRotatingTokenBucketRateLimiter(
		4, // Small number of buckets to increase collision chances
		rotatingBurstCapacity,
		rotatingRatePerSecond,
		time.Second,
		rotationRate,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// Find two IDs that collide in the current buckets
	id1 := []byte("collision-test-1")
	id2 := []byte("collision-test-2")

	pair := limiter.load(nowfn())
	index1 := pair.checked.index(id1)
	index2 := pair.checked.index(id2)

	// If they don't collide, try more IDs
	for i := 3; index1 != index2 && i < 100; i++ {
		id2 = []byte("collision-test-" + string(rune('0'+i)))
		index2 = pair.checked.index(id2)
	}

	if index1 != index2 {
		t.Skip("Could not find colliding IDs in test")
	}

	// Exhaust tokens for id1 (make burstCapacity calls)
	for i := 0; i < int(rotatingBurstCapacity); i++ {
		if !limiter.TakeToken(id1) {
			t.Errorf("Should be able to take token %d for id1", i)
		}
	}

	// id2 should also be rate limited due to collision
	if limiter.TakeToken(id2) {
		t.Error("id2 should be rate limited due to collision with id1")
	}

	// After rotation, new seed should resolve collision
	tick(rotationRate + 1*time.Millisecond)

	// Wait for some token refill as well
	tick(2 * time.Second)

	// At least one of them should be able to take tokens now
	// (assuming new seed resolves collision)
	canTake1 := limiter.Check(id1)
	canTake2 := limiter.Check(id2)

	if !canTake1 && !canTake2 {
		t.Error("After rotation, at least one ID should be able to take tokens")
	}
}

// TestRotatingTokenBucketLimiterTokenPreservation ensures tokens aren't lost during rotation
func TestRotatingTokenBucketLimiterTokenPreservation(t *testing.T) {
	rotationRate := 50 * time.Millisecond
	limiter, err := NewRotatingTokenBucketRateLimiter(
		rotatingNumBuckets,
		rotatingBurstCapacity,
		rotatingRatePerSecond,
		time.Second,
		rotationRate,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("preservation-test")

	// Take some tokens but not all
	tokensToTake := int(rotatingBurstCapacity) / 2 // Take half of available calls
	for i := 0; i < tokensToTake; i++ {
		if !limiter.TakeToken(id) {
			t.Errorf("Should be able to take token %d", i)
		}
	}

	// Should still have tokens available
	if !limiter.Check(id) {
		t.Error("Should still have tokens available")
	}

	// Trigger rotation
	tick(rotationRate + 1*time.Millisecond)

	// Should still have tokens available after rotation
	// (because the old bucket becomes the new ignored bucket,
	// and we take from both checked and ignored)
	if !limiter.Check(id) {
		t.Error("Should still have tokens available after rotation")
	}

	// Take remaining tokens
	remaining := int(rotatingBurstCapacity) - tokensToTake
	for i := 0; i < remaining; i++ {
		if !limiter.TakeToken(id) {
			t.Errorf("Should be able to take remaining token %d", i)
		}
	}

	// Now should be rate limited
	if limiter.TakeToken(id) {
		t.Error("Should be rate limited after taking all tokens")
	}
}

// TestRotatingTokenBucketLimiterConcurrency tests concurrent access
func TestRotatingTokenBucketLimiterConcurrency(t *testing.T) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	const numWorkers = 10
	const numOperations = 100

	var wg sync.WaitGroup
	var successCount int64

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			id := []byte("worker-" + string(rune('0'+workerID)))

			for j := 0; j < numOperations; j++ {
				if limiter.TakeToken(id) {
					atomic.AddInt64(&successCount, 1)
				}
				// Also test Check method
				limiter.Check(id)
			}
		}(i)
	}

	wg.Wait()

	// Should have some successful token takes, but exact count depends on timing
	finalSuccessCount := atomic.LoadInt64(&successCount)
	if finalSuccessCount == 0 {
		t.Error("Expected some successful token takes")
	}
}

// TestRotatingTokenBucketLimiterLoadLogic tests the load method specifically
func TestRotatingTokenBucketLimiterLoadLogic(t *testing.T) {
	rotationRate := 100 * time.Millisecond
	limiter, err := NewRotatingTokenBucketRateLimiter(
		rotatingNumBuckets,
		rotatingBurstCapacity,
		rotatingRatePerSecond,
		time.Second,
		rotationRate,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	now := nowfn()

	// First load should return initial pair
	pair1 := limiter.load(now)
	if pair1 == nil {
		t.Fatal("Expected non-nil pair")
	}

	// Load with same timestamp should return same pair
	pair2 := limiter.load(now)
	if pair1 != pair2 {
		t.Error("Same timestamp should return same pair")
	}

	// Load with timestamp before rotation should return same pair
	beforeRotation := now + rotationRate.Nanoseconds() - 1
	pair3 := limiter.load(beforeRotation)
	if pair1 != pair3 {
		t.Error("Before rotation timestamp should return same pair")
	}

	// Load with timestamp at rotation should trigger rotation
	atRotation := now + rotationRate.Nanoseconds()
	pair4 := limiter.load(atRotation)
	if pair1 == pair4 {
		t.Error("At rotation timestamp should return new pair")
	}

	// Verify rotation happened correctly
	if pair4.checked != pair1.ignored {
		t.Error("New checked should be old ignored")
	}

	if pair4.rotated != time56.Unix(atRotation) {
		t.Error("New pair should have updated rotation timestamp")
	}
}

// TestRotatingTokenBucketLimiterConcurrentRotation tests concurrent rotation scenarios
func TestRotatingTokenBucketLimiterConcurrentRotation(t *testing.T) {
	rotationRate := 10 * time.Millisecond // Very short rotation for testing
	limiter, err := NewRotatingTokenBucketRateLimiter(
		rotatingNumBuckets,
		rotatingBurstCapacity,
		rotatingRatePerSecond,
		time.Second,
		rotationRate,
	)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	const numWorkers = 5
	var wg sync.WaitGroup

	// Start multiple workers that will likely trigger concurrent rotation
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			id := []byte("concurrent-worker-" + string(rune('0'+workerID)))

			// Wait for rotation to be likely
			tick(rotationRate + 1*time.Millisecond)

			// Try to trigger rotation concurrently
			for j := 0; j < 10; j++ {
				limiter.TakeToken(id)
				limiter.Check(id)
			}
		}(i)
	}

	wg.Wait()
	// Test should not panic or deadlock
}

// TestRotatingTokenBucketLimiterDifferentIDs tests behavior with different IDs
func TestRotatingTokenBucketLimiterDifferentIDs(t *testing.T) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id1 := []byte("different-id-1")
	id2 := []byte("different-id-2")

	// Check if these IDs hash to the same bucket (hash collision)
	pair := limiter.load(nowfn())
	index1 := pair.checked.index(id1)
	index2 := pair.checked.index(id2)

	if index1 == index2 {
		// Hash collision case - IDs share the same bucket
		t.Logf("Hash collision detected: both IDs map to bucket %d", index1)

		// With collision, they share tokens from the same bucket
		// We can make burstCapacity total calls between both IDs
		totalCalls := 0
		for totalCalls < int(rotatingBurstCapacity) {
			if limiter.TakeToken(id1) {
				totalCalls++
			} else {
				break
			}
			if totalCalls < int(rotatingBurstCapacity) && limiter.TakeToken(id2) {
				totalCalls++
			} else {
				break
			}
		}

		// Both should now be rate limited (sharing exhausted bucket)
		if limiter.TakeToken(id1) {
			t.Error("id1 should be rate limited after bucket exhaustion")
		}
		if limiter.TakeToken(id2) {
			t.Error("id2 should be rate limited after bucket exhaustion")
		}
	} else {
		// No collision case - IDs have independent buckets
		t.Logf("No collision: id1 maps to bucket %d, id2 maps to bucket %d", index1, index2)

		// Each ID should have independent rate limiting (make burstCapacity calls each)
		for i := 0; i < int(rotatingBurstCapacity); i++ {
			if !limiter.TakeToken(id1) {
				t.Errorf("Should be able to take token %d for id1", i)
			}
			if !limiter.TakeToken(id2) {
				t.Errorf("Should be able to take token %d for id2", i)
			}
		}

		// Both should be rate limited now
		if limiter.TakeToken(id1) {
			t.Error("id1 should be rate limited")
		}
		if limiter.TakeToken(id2) {
			t.Error("id2 should be rate limited")
		}
	}
}

// BenchmarkRotatingTokenBucketLimiterTakeToken benchmarks TakeToken performance
func BenchmarkRotatingTokenBucketLimiterTakeToken(b *testing.B) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		b.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("benchmark-id")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		limiter.TakeToken(id)
	}
}

// BenchmarkRotatingTokenBucketLimiterCheck benchmarks Check performance
func BenchmarkRotatingTokenBucketLimiterCheck(b *testing.B) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		b.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("benchmark-check-id")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		limiter.Check(id)
	}
}

// BenchmarkRotatingTokenBucketLimiterConcurrent benchmarks concurrent access
func BenchmarkRotatingTokenBucketLimiterConcurrent(b *testing.B) {
	limiter, err := DefaultRotatingLimiter()
	if err != nil {
		b.Fatalf("Failed to create limiter: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		id := []byte("concurrent-bench")
		for pb.Next() {
			limiter.TakeToken(id)
		}
	})
}
