package rate

import (
	"hash/maphash"
	"time"

	"github.com/webriots/rate/time56"
)

// TokenBucketLimiter implements the token bucket algorithm for rate
// limiting. It maintains multiple buckets to distribute load and
// reduce contention. Each bucket has a fixed capacity and refills at
// a specified rate.
type TokenBucketLimiter struct {
	buckets       atomicSliceUint64 // Array of token buckets
	burstCapacity uint8             // Maximum tokens per bucket
	nanosPerToken int64             // Nanoseconds per token refill
}

// NewTokenBucketLimiter creates a new token bucket rate limiter with
// the specified parameters:
//
//   - numBuckets: number of token buckets (automatically rounded up
//     to the nearest power of two if not already a power of two, for
//     efficient hashing)
//   - burstCapacity: maximum number of tokens that can be consumed at
//     once
//   - refillRate: rate at which tokens are refilled
//   - refillRateUnit: time unit for refill rate calculations (e.g.,
//     time.Second)
//
// Returns a new TokenBucketLimiter instance and any error that
// occurred during creation. The numBuckets parameter is automatically
// rounded up to the nearest power of two if not already a power of
// two, for efficient hashing.
func NewTokenBucketLimiter(
	numBuckets uint,
	burstCapacity uint8,
	refillRate float64,
	refillRateUnit time.Duration,
) (*TokenBucketLimiter, error) {
	n := ceilPow2(uint64(numBuckets))
	bucket := newTokenBucket(burstCapacity, time56.Unix(nowfn()))
	packed := bucket.packed()

	buckets := newAtomicSliceUint64(int(n))
	for i := range buckets.Len() {
		buckets.Set(i, packed)
	}

	return &TokenBucketLimiter{
		buckets:       buckets,
		burstCapacity: burstCapacity,
		nanosPerToken: nanoRate(refillRateUnit, refillRate),
	}, nil
}

// Check returns whether a token would be available for the given ID
// without actually taking it. This is useful for preemptively
// checking if an operation would be rate limited before attempting
// it. Returns true if a token would be available, false otherwise.
func (t *TokenBucketLimiter) Check(id []byte) bool {
	return t.checkInner(t.index(id), t.nanosPerToken)
}

// TakeToken attempts to take a token for the given ID. It returns
// true if a token was successfully taken, false if the operation
// should be rate limited. This method is thread-safe and can be
// called concurrently from multiple goroutines.
func (t *TokenBucketLimiter) TakeToken(id []byte) bool {
	return t.takeTokenInner(t.index(id), t.nanosPerToken)
}

// checkInner is an internal method that checks if a token is
// available in the bucket at the specified index using the given
// refill rate. This is used by Check and is also used by other
// limiters that wrap this one.
func (t *TokenBucketLimiter) checkInner(index int, rate int64) bool {
	existing := t.buckets.Get(index)
	unpacked := unpack(existing)
	refilled := unpacked.refill(nowfn(), rate, t.burstCapacity)
	return refilled.level > 0
}

// takeTokenInner is an internal method that attempts to take a token
// from the bucket at the specified index using the given refill rate.
// This is used by TakeToken and is also used by other limiters that
// wrap this one. It uses atomic operations to ensure thread safety.
func (t *TokenBucketLimiter) takeTokenInner(index int, rate int64) bool {
	now := nowfn()
	for {
		existing := t.buckets.Get(index)
		unpacked := unpack(existing)
		refilled := unpacked.refill(now, rate, t.burstCapacity)
		consumed, ok := refilled.take()

		if consumed != unpacked && !t.buckets.CompareAndSwap(
			index,
			existing,
			consumed.packed(),
		) {
			continue
		}

		return ok
	}
}

// seed is used globally for index bucket hash generation.
var seed = maphash.MakeSeed()

// index calculates the bucket index for the given ID using maphash.
// The result is masked to ensure it falls within the range of valid
// buckets.
func (t *TokenBucketLimiter) index(id []byte) int {
	return int(maphash.Bytes(seed, id) & uint64(t.buckets.Len()-1))
}

// tokenBucket represents a single token bucket with a certain level
// (number of tokens) and a timestamp of when it was last refilled.
type tokenBucket struct {
	level uint8       // Current number of tokens in the bucket
	stamp time56.Time // Last time the bucket was refilled
}

// newTokenBucket creates a new token bucket with the specified level
// and timestamp.
func newTokenBucket(level uint8, stamp time56.Time) tokenBucket {
	return tokenBucket{level: level, stamp: stamp}
}

// refill updates the token bucket based on elapsed time since the
// last refill. It calculates how many tokens should be added based on
// the elapsed time and refill rate, and updates the bucket's level
// and timestamp accordingly. The bucket level will not exceed
// maxLevel.
func (b tokenBucket) refill(nowNS, rate int64, maxLevel uint8) tokenBucket {
	now := time56.Unix(nowNS)

	elapsed := now.Since(b.stamp)
	if elapsed <= 0 {
		return b
	}

	tokens := elapsed / rate
	if tokens <= 0 {
		return b
	}

	if avail := maxLevel - b.level; tokens < int64(avail) {
		b.level += uint8(tokens)
	} else {
		b.level = maxLevel
	}

	if remainder := elapsed % rate; remainder > 0 {
		b.stamp = now.Sub(remainder)
	} else {
		b.stamp = now
	}

	return b
}

// take attempts to take a token from the bucket. Returns the updated
// bucket and a boolean indicating whether a token was taken. If no
// tokens are available, the bucket remains unchanged and false is
// returned.
func (b tokenBucket) take() (tokenBucket, bool) {
	if b.level > 0 {
		b.level--
		return b, true
	} else {
		return b, false
	}
}

// packed converts the token bucket to a packed uint64 representation
// where the level is stored in the high 8 bits and the timestamp in
// the low 56 bits.
func (b tokenBucket) packed() uint64 {
	return b.stamp.Pack(b.level)
}

// unpack extracts a token bucket from its packed uint64
// representation. This is the inverse operation of packed().
func unpack(packed uint64) tokenBucket {
	return newTokenBucket(time56.Unpack(packed))
}

// nanoRate converts a refill rate from tokens per unit to nanoseconds
// per token. This is used to calculate how frequently tokens should
// be added to buckets.
func nanoRate(refillRateUnit time.Duration, refillRate float64) int64 {
	return int64(float64(refillRateUnit.Nanoseconds()) * refillRate)
}

// unitRate converts a rate in nanoseconds per token back to tokens
// per unit. This is the inverse operation of nanoRate and is used to
// provide human-readable rate values for APIs returning rate
// information.
func unitRate(refillRateUnit time.Duration, refillRateNanos int64) float64 {
	return float64(refillRateNanos) / float64(refillRateUnit.Nanoseconds())
}

// maxPow2 defines the maximum power of two value that ceilPow2 will
// return. This prevents potential overflow issues when rounding up
// values close to the maximum uint64 value. Using 2^62 allows safe
// bit manipulation while still providing an extremely large maximum
// bucket count.
const maxPow2 = 1 << 62

// ceilPow2 rounds up the given number to the nearest power of two. If
// the input is already a power of two, it returns the input
// unchanged. This implementation uses a bit manipulation algorithm
// for efficiency.
func ceilPow2(x uint64) uint64 {
	if x == 0 {
		return 1
	}

	if x >= maxPow2 {
		return maxPow2
	}

	x = x - 1
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	x |= x >> 32
	return x + 1
}
