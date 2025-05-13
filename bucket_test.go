package rate

import (
	"strconv"
	"sync/atomic"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/webriots/rate/time56"
	"golang.org/x/sync/errgroup"
)

const (
	burstCapacity = uint8(10)
	numBuckets    = uint(131072)
	ratePerSecond = 1.0
)

func DefaultLimiter() (*TokenBucketLimiter, error) {
	return NewTokenBucketLimiter(numBuckets, burstCapacity, ratePerSecond, time.Second)
}

// TestTokenBucketLimiterError tests constructor error case
func TestTokenBucketLimiterError(t *testing.T) {
	r := require.New(t)

	// Try creating with non-power-of-two buckets
	_, err := NewTokenBucketLimiter(3, burstCapacity, ratePerSecond, time.Second)
	r.Error(err, "should error on non-power-of-two bucket count")
	r.Contains(err.Error(), "power of two", "error message should mention power of two")
}

func TestTokenBucketRefill(t *testing.T) {
	r := require.New(t)

	limiter, err := DefaultLimiter()
	r.NoError(err)

	fullBucket := newTokenBucket(burstCapacity, 0)
	r.Equal(fullBucket.refill(1<<40, limiter.refillIntervalNanos, burstCapacity), fullBucket)
}

func TestTokenBucketPackUnpack(t *testing.T) {
	r := require.New(t)

	f := func(ts uint64, level uint8) bool {
		tb := tokenBucket{
			stamp: time56.New(ts),
			level: level,
		}
		packed := tb.packed()
		unpacked := unpack(packed)
		return unpacked == tb
	}

	r.NoError(quick.Check(f, nil))
}

func TestTokenBucketRefillOld(t *testing.T) {
	r := require.New(t)

	limiter, err := DefaultLimiter()
	r.NoError(err)

	old := tokenBucket{}
	refilled := old.refill(1<<40, limiter.refillIntervalNanos, limiter.burstCapacity)

	r.Equal(burstCapacity, refilled.level)
	r.Greater(refilled.stamp.Uint64(), uint64(0))
}

func TestTokenBucketAddSingleToken(t *testing.T) {
	r := require.New(t)

	limiter, err := DefaultLimiter()
	r.NoError(err)

	f := func(start int64) bool {
		end := start + limiter.refillIntervalNanos
		return newTokenBucket(0, time56.Unix(start)).refill(end, limiter.refillIntervalNanos, limiter.burstCapacity).level == 1
	}

	r.NoError(quick.Check(f, nil))
}

func TestTokenBucketNoTokenRefill(t *testing.T) {
	r := require.New(t)

	limiter, err := DefaultLimiter()
	r.NoError(err)

	f := func(start int64) bool {
		end := start + limiter.refillIntervalNanos - 1
		return newTokenBucket(0, time56.Unix(start)).refill(end, limiter.refillIntervalNanos, limiter.burstCapacity).level == 0
	}

	r.NoError(quick.Check(f, nil))
}

func TestTokenBucketEmptyNoChange(t *testing.T) {
	r := require.New(t)

	bucket := newTokenBucket(0, 0)
	taken, changed := bucket.take()

	r.Equal(bucket, taken)
	r.False(changed)
}

func TestTokenBucketNonEmptyDecrement(t *testing.T) {
	r := require.New(t)

	taken, changed := newTokenBucket(10, 0).take()

	expect := tokenBucket{
		level: 9,
		stamp: 0,
	}

	r.Equal(expect, taken)
	r.True(changed)
}

func TestTokenBucketRateOnlyAfterBurst(t *testing.T) {
	r := require.New(t)

	limiter, err := DefaultLimiter()
	r.NoError(err)

	ids := GenIDs(100)

	for range burstCapacity {
		for _, id := range ids {
			r.True(limiter.TakeToken(id))
		}
	}

	for _, id := range ids {
		r.False(limiter.TakeToken(id))
	}
}

func TestTokenBucketRateAfterBurst(t *testing.T) {
	r := require.New(t)

	limiter, err := DefaultLimiter()
	r.NoError(err)

	ids := GenIDs(100)

	for range burstCapacity {
		for _, id := range ids {
			r.True(limiter.TakeToken(id))
		}
	}

	// No tokens remaining
	for _, id := range ids {
		r.False(limiter.TakeToken(id))
	}

	var (
		allowed atomic.Int64
		group   errgroup.Group
	)

	threads := 4
	sleep := 10 * time.Millisecond
	duration := 10 * time.Second
	attempts := threads * int(duration/sleep)

	group.SetLimit(threads)

	start := nowfn().UnixNano()

	// simulate concurrent token attempts with time advancement
	for range attempts {
		group.Go(func() error {
			tick(sleep)
			for _, id := range ids {
				if limiter.TakeToken(id) {
					allowed.Add(1)
				}
			}
			return nil
		})
	}

	r.NoError(group.Wait())

	// compute elapsed simulated time
	elapsed := time.Duration(nowfn().UnixNano() - start)
	rate := (float64(allowed.Load()) / float64(len(ids))) / elapsed.Seconds()

	r.GreaterOrEqual(rate, ratePerSecond-0.1)
	r.LessOrEqual(rate, ratePerSecond+0.1)
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
	r := require.New(t)
	limiter, err := DefaultLimiter()
	r.NoError(err)

	id := []byte("check-test")

	// Check should return true initially when tokens are available
	r.True(limiter.Check(id), "check should return true when tokens are available")

	// Multiple checks should not consume tokens
	for range 5 {
		r.True(limiter.Check(id), "repeated checks should not consume tokens")
	}

	// We should still be able to take all burst capacity tokens
	for i := range int(burstCapacity) {
		r.True(limiter.TakeToken(id), "should take token %d after checks", i)
	}

	// After taking all tokens, check should return false
	r.False(limiter.Check(id), "check should return false when no tokens available")

	// Check should continue to return false and not affect state
	r.False(limiter.Check(id), "check should consistently return false when no tokens")

	// Wait for refill
	tick(time.Second * 2)

	// Now check should return true again
	r.True(limiter.Check(id), "check should return true after refill")
}

// TestTokenBucketCheckInner verifies the checkInner method with different rates.
func TestTokenBucketCheckInner(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultLimiter()
	r.NoError(err)

	id := []byte("inner-check-test")
	index := limiter.index(id)

	// Different rates to test
	fastRate := limiter.refillIntervalNanos / 2 // Faster refill
	normalRate := limiter.refillIntervalNanos   // Normal refill
	slowRate := limiter.refillIntervalNanos * 2 // Slower refill

	// Initially all should return true
	r.True(limiter.checkInner(index, fastRate), "fast rate check should be true")
	r.True(limiter.checkInner(index, normalRate), "normal rate check should be true")
	r.True(limiter.checkInner(index, slowRate), "slow rate check should be true")

	// Exhaust the tokens
	for range int(burstCapacity) {
		r.True(limiter.TakeToken(id))
	}
	r.False(limiter.TakeToken(id))

	// All rates should return false immediately after exhaustion
	r.False(limiter.checkInner(index, fastRate), "fast rate check should be false")
	r.False(limiter.checkInner(index, normalRate), "normal rate check should be false")
	r.False(limiter.checkInner(index, slowRate), "slow rate check should be false")

	// Wait for partial refill
	tick(500 * time.Millisecond)

	// Test different rate refill states without capturing results
	_ = limiter.checkInner(index, fastRate)
	_ = limiter.checkInner(index, normalRate)
	_ = limiter.checkInner(index, slowRate)

	// After full refill time for normal rate
	tick(time.Second)

	// Normal rate should definitely have refilled
	r.True(limiter.checkInner(index, normalRate), "normal rate should refill after 1 second")
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
