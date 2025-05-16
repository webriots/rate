package rate

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/quick"
	"time"

	"github.com/webriots/rate/time56"
)

const (
	burstCapacity = uint8(10)
	numBuckets    = uint(131072)
	ratePerSecond = 1.0
)

func DefaultLimiter() (*TokenBucketLimiter, error) {
	return NewTokenBucketLimiter(numBuckets, burstCapacity, ratePerSecond, time.Second)
}

// TestTokenBucketRoundingToPowerOfTwo verifies numBuckets is rounded up to the next power of two
func TestTokenBucketRoundingToPowerOfTwo(t *testing.T) {
	// Try creating with non-power-of-two buckets
	limiter, err := NewTokenBucketLimiter(3, burstCapacity, ratePerSecond, time.Second)
	if err != nil {
		t.Fatalf("Expected no error with non-power-of-two bucket count, got: %v", err)
	}

	// Should be rounded up to 4 (next power of two after 3)
	if limiter.numBuckets != 4 {
		t.Errorf("Expected numBuckets to be rounded up to 4, got %d", limiter.numBuckets)
	}

	// Test with a power of two (should remain unchanged)
	limiter, err = NewTokenBucketLimiter(16, burstCapacity, ratePerSecond, time.Second)
	if err != nil {
		t.Fatalf("Expected no error with power-of-two bucket count, got: %v", err)
	}

	if limiter.numBuckets != 16 {
		t.Errorf("Expected numBuckets to remain 16, got %d", limiter.numBuckets)
	}
}

func TestTokenBucketRefill(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	fullBucket := newTokenBucket(burstCapacity, 0)
	result := fullBucket.refill(1<<40, limiter.refillIntervalNanos, burstCapacity)

	if fullBucket != result {
		t.Errorf("refill() = %v, want %v", result, fullBucket)
	}
}

func TestTokenBucketPackUnpack(t *testing.T) {
	f := func(ts uint64, level uint8) bool {
		tb := tokenBucket{
			stamp: time56.New(ts),
			level: level,
		}
		packed := tb.packed()
		unpacked := unpack(packed)
		return unpacked == tb
	}

	if err := quick.Check(f, nil); err != nil {
		t.Errorf("quick.Check failed: %v", err)
	}
}

func TestTokenBucketRefillOld(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	old := tokenBucket{}
	refilled := old.refill(1<<40, limiter.refillIntervalNanos, limiter.burstCapacity)

	if refilled.level != burstCapacity {
		t.Errorf("refilled.level = %d, want %d", refilled.level, burstCapacity)
	}

	if refilled.stamp.Uint64() <= 0 {
		t.Errorf("refilled.stamp = %d, want > 0", refilled.stamp.Uint64())
	}
}

func TestTokenBucketAddSingleToken(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	f := func(start int64) bool {
		end := start + limiter.refillIntervalNanos
		return newTokenBucket(0, time56.Unix(start)).refill(end, limiter.refillIntervalNanos, limiter.burstCapacity).level == 1
	}

	if err := quick.Check(f, nil); err != nil {
		t.Errorf("quick.Check failed: %v", err)
	}
}

func TestTokenBucketNoTokenRefill(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	f := func(start int64) bool {
		end := start + limiter.refillIntervalNanos - 1
		return newTokenBucket(0, time56.Unix(start)).refill(end, limiter.refillIntervalNanos, limiter.burstCapacity).level == 0
	}

	if err := quick.Check(f, nil); err != nil {
		t.Errorf("quick.Check failed: %v", err)
	}
}

func TestTokenBucketEmptyNoChange(t *testing.T) {
	bucket := newTokenBucket(0, 0)
	taken, changed := bucket.take()

	if bucket != taken {
		t.Errorf("take(): got %v, want %v", taken, bucket)
	}

	if changed {
		t.Error("take(): changed = true, want false")
	}
}

func TestTokenBucketNonEmptyDecrement(t *testing.T) {
	taken, changed := newTokenBucket(10, 0).take()

	expect := tokenBucket{
		level: 9,
		stamp: 0,
	}

	if taken != expect {
		t.Errorf("take(): got %v, want %v", taken, expect)
	}

	if !changed {
		t.Error("take(): changed = false, want true")
	}
}

func TestTokenBucketRateOnlyAfterBurst(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ids := GenIDs(100)

	for range burstCapacity {
		for _, id := range ids {
			if !limiter.TakeToken(id) {
				t.Error("TakeToken should succeed during burst")
			}
		}
	}

	for _, id := range ids {
		if limiter.TakeToken(id) {
			t.Error("TakeToken should fail after burst")
		}
	}
}

func TestTokenBucketRateAfterBurst(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	ids := GenIDs(100)

	for range burstCapacity {
		for _, id := range ids {
			if !limiter.TakeToken(id) {
				t.Error("TakeToken should succeed during burst")
			}
		}
	}

	// No tokens remaining
	for _, id := range ids {
		if limiter.TakeToken(id) {
			t.Error("TakeToken should fail after burst")
		}
	}

	var (
		allowed atomic.Int64
		wg      sync.WaitGroup
	)

	threads := 4
	sleep := 10 * time.Millisecond
	duration := 10 * time.Second
	attempts := threads * int(duration/sleep)
	semaphore := make(chan struct{}, threads)

	start := nowfn().UnixNano()

	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			tick(sleep)
			for _, id := range ids {
				if limiter.TakeToken(id) {
					allowed.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// compute elapsed simulated time
	elapsed := time.Duration(nowfn().UnixNano() - start)
	rate := (float64(allowed.Load()) / float64(len(ids))) / elapsed.Seconds()

	if rate < ratePerSecond-0.1 {
		t.Errorf("Rate too low: got %f, want at least %f", rate, ratePerSecond-0.1)
	}

	if rate > ratePerSecond+0.1 {
		t.Errorf("Rate too high: got %f, want at most %f", rate, ratePerSecond+0.1)
	}
}

func GenIDs(count int) [][]byte {
	ids := make([][]byte, count)
	for i := range ids {
		ids[i] = []byte(strconv.Itoa(i))
	}
	return ids
}

// Benchmark tests

func BenchmarkTokenBucketCreate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = DefaultLimiter()
	}
}

func BenchmarkTokenBucketCheck(b *testing.B) {
	limiter, _ := DefaultLimiter()
	id := []byte("benchmark-id")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Check(id)
	}
}

// TestTokenBucketCheck verifies the Check method's non-consuming behavior.
func TestTokenBucketCheck(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("check-test")

	// Check should return true initially when tokens are available
	if !limiter.Check(id) {
		t.Error("Check should return true when tokens are available")
	}

	// Multiple checks should not consume tokens
	for range 5 {
		if !limiter.Check(id) {
			t.Error("Repeated checks should not consume tokens")
		}
	}

	// We should still be able to take all burst capacity tokens
	for i := range int(burstCapacity) {
		if !limiter.TakeToken(id) {
			t.Errorf("Failed to take token at i=%d after checks", i)
		}
	}

	// After taking all tokens, check should return false
	if limiter.Check(id) {
		t.Error("Check should return false when no tokens available")
	}

	// Check should continue to return false and not affect state
	if limiter.Check(id) {
		t.Error("Check should consistently return false when no tokens")
	}

	// Wait for refill
	tick(time.Second * 2)

	// Now check should return true again
	if !limiter.Check(id) {
		t.Error("Check should return true after refill")
	}
}

// TestTokenBucketCheckInner verifies the checkInner method with different rates.
func TestTokenBucketCheckInner(t *testing.T) {
	limiter, err := DefaultLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("inner-check-test")
	index := limiter.index(id)

	// Different rates to test
	fastRate := limiter.refillIntervalNanos / 2 // Faster refill
	normalRate := limiter.refillIntervalNanos   // Normal refill
	slowRate := limiter.refillIntervalNanos * 2 // Slower refill

	// Initially all should return true
	if !limiter.checkInner(index, fastRate) {
		t.Error("Fast rate check should be true initially")
	}

	if !limiter.checkInner(index, normalRate) {
		t.Error("Normal rate check should be true initially")
	}

	if !limiter.checkInner(index, slowRate) {
		t.Error("Slow rate check should be true initially")
	}

	// Exhaust the tokens
	for range int(burstCapacity) {
		if !limiter.TakeToken(id) {
			t.Error("TakeToken should succeed")
		}
	}

	if limiter.TakeToken(id) {
		t.Error("TakeToken should fail after burst")
	}

	// All rates should return false immediately after exhaustion
	if limiter.checkInner(index, fastRate) {
		t.Error("Fast rate check should be false after exhaustion")
	}

	if limiter.checkInner(index, normalRate) {
		t.Error("Normal rate check should be false after exhaustion")
	}

	if limiter.checkInner(index, slowRate) {
		t.Error("Slow rate check should be false after exhaustion")
	}

	// Wait for partial refill
	tick(500 * time.Millisecond)

	// Test different rate refill states without capturing results
	_ = limiter.checkInner(index, fastRate)
	_ = limiter.checkInner(index, normalRate)
	_ = limiter.checkInner(index, slowRate)

	// After full refill time for normal rate
	tick(time.Second)

	// Normal rate should definitely have refilled
	if !limiter.checkInner(index, normalRate) {
		t.Error("Normal rate should refill after 1 second")
	}
}

func BenchmarkTokenBucketTakeToken(b *testing.B) {
	limiter, _ := DefaultLimiter()
	id := []byte("benchmark-id")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.TakeToken(id)
	}
}

func BenchmarkTokenBucketPacked(b *testing.B) {
	bucket := newTokenBucket(5, time56.Unix(time.Now().UnixNano()))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bucket.packed()
	}
}

func BenchmarkTokenBucketUnpack(b *testing.B) {
	bucket := newTokenBucket(5, time56.Unix(time.Now().UnixNano()))
	packed := bucket.packed()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = unpack(packed)
	}
}

func BenchmarkTokenBucketIndex(b *testing.B) {
	limiter, _ := DefaultLimiter()
	ids := GenIDs(100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.index(ids[i%len(ids)])
	}
}

func BenchmarkTokenBucketRefill(b *testing.B) {
	limiter, _ := DefaultLimiter()
	bucket := newTokenBucket(5, time56.Unix(time.Now().UnixNano()))
	now := time.Now().UnixNano()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket.refill(now, limiter.refillIntervalNanos, limiter.burstCapacity)
	}
}

func BenchmarkTokenBucketTake(b *testing.B) {
	bucket := newTokenBucket(burstCapacity, time56.Unix(time.Now().UnixNano()))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket, _ = bucket.take()
		if i%int(burstCapacity) == 0 {
			// Reset bucket level periodically to avoid running out of tokens
			bucket = newTokenBucket(burstCapacity, bucket.stamp)
		}
	}
}

func BenchmarkTokenBucketParallel(b *testing.B) {
	limiter, _ := DefaultLimiter()
	ids := GenIDs(1000)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			limiter.TakeToken(ids[i%len(ids)])
			i++
		}
	})
}

func BenchmarkTokenBucketManyIDs(b *testing.B) {
	limiter, _ := DefaultLimiter()
	ids := GenIDs(10000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.TakeToken(ids[i%len(ids)])
	}
}

func BenchmarkTokenBucketContention(b *testing.B) {
	limiter, _ := DefaultLimiter()
	// Use a single ID to maximize contention on a single bucket
	id := []byte("high-contention-id")

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.TakeToken(id)
		}
	})
}

// Additional benchmarks to explore allocation behavior with different parameters

func BenchmarkTokenBucketCreateSmall(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = NewTokenBucketLimiter(16, 5, 1.0, time.Second)
	}
}

func BenchmarkTokenBucketCreateMedium(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = NewTokenBucketLimiter(1024, 10, 1.0, time.Second)
	}
}

func BenchmarkTokenBucketCreateLarge(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = NewTokenBucketLimiter(16384, 20, 1.0, time.Second)
	}
}

func BenchmarkTokenBucketDynamicID(b *testing.B) {
	limiter, _ := DefaultLimiter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create a new ID for each iteration to test allocation behavior
		id := []byte(strconv.Itoa(i))
		limiter.TakeToken(id)
	}
}

// Realistic workload scenarios

func BenchmarkTokenBucketRealWorldRequestRate(b *testing.B) {
	// Simulate a real-world API rate limiting scenario
	// 100 requests per second, 10 burst capacity
	limiter, _ := NewTokenBucketLimiter(1024, 10, 100, time.Second)

	// Create a set of IDs representing different API clients
	numClients := 50
	clients := GenIDs(numClients)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var counter int
		for pb.Next() {
			// Simulate different clients making requests
			clientID := counter % numClients
			counter++
			limiter.TakeToken(clients[clientID])
		}
	})
}

func BenchmarkTokenBucketHighContention(b *testing.B) {
	// Test with very high contention - many goroutines hitting the same buckets
	limiter, _ := DefaultLimiter()

	// Only a few IDs to maximize contention
	ids := GenIDs(5)

	// Run with high parallelism to test contention
	b.SetParallelism(100)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var counter int
		for pb.Next() {
			// Cycle through the small set of IDs to create contention
			id := counter % len(ids)
			counter++
			limiter.TakeToken(ids[id])
		}
	})
}

func BenchmarkTokenBucketWithRefill(b *testing.B) {
	// Test token bucket with periodic refills
	limiter, _ := DefaultLimiter()
	id := []byte("refill-test-id")

	b.ReportAllocs()
	b.ResetTimer()

	// Each iteration simulates time passing and refilling
	for i := 0; i < b.N; i++ {
		if i%100 == 0 {
			// Periodically advance time to allow refill
			tick(time.Second)
		}
		limiter.TakeToken(id)
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && len(substr) > 0 && s != substr && s != "" && (s == substr || containsSubstringHelper(s, substr))
}

func containsSubstringHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestCeilPow2 tests the ceilPow2 function
func TestCeilPow2(t *testing.T) {
	testCases := []struct {
		input    uint64
		expected uint64
	}{
		{0, 0},                   // Edge case: 0 rounds to 0
		{1, 1},                   // Power of 2 stays the same
		{2, 2},                   // Power of 2 stays the same
		{3, 4},                   // Round up to next power of 2
		{4, 4},                   // Power of 2 stays the same
		{5, 8},                   // Round up to next power of 2
		{7, 8},                   // Round up to next power of 2
		{8, 8},                   // Power of 2 stays the same
		{9, 16},                  // Round up to next power of 2
		{15, 16},                 // Round up to next power of 2
		{16, 16},                 // Power of 2 stays the same
		{17, 32},                 // Round up to next power of 2
		{31, 32},                 // Round up to next power of 2
		{32, 32},                 // Power of 2 stays the same
		{33, 64},                 // Round up to next power of 2
		{100, 128},               // Round up to next power of 2
		{127, 128},               // Round up to next power of 2
		{128, 128},               // Power of 2 stays the same
		{129, 256},               // Round up to next power of 2
		{1 << 31, 1 << 31},       // Power of 2 stays the same
		{(1 << 31) + 1, 1 << 32}, // Round up to next power of 2
		{1 << 32, 1 << 32},       // Power of 2 stays the same
		{(1 << 32) + 1, 1 << 33}, // Round up to next power of 2
		{1 << 63, 1 << 63},       // Power of 2 stays the same
		{(1 << 63) - 1, 1 << 63}, // Round up to next power of 2 (max possible)
		// Note: Testing (1<<63)+1 would cause overflow in ceilPow2
	}

	for _, tc := range testCases {
		result := ceilPow2(tc.input)
		if result != tc.expected {
			t.Errorf("ceilPow2(%d) = %d, expected %d", tc.input, result, tc.expected)
		}
	}
}
