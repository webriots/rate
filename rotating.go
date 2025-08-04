package rate

import (
	"fmt"
	"hash/maphash"
	"sync/atomic"
	"time"

	"github.com/webriots/rate/time56"
)

// rotatingPair holds two TokenBucketLimiters for collision avoidance.
// The 'checked' limiter is the primary one used for rate limiting
// decisions. The 'ignored' limiter is checked but its result is
// discarded - it serves to maintain state during bucket rotation and
// provide collision avoidance. The 'rotated' timestamp indicates when
// this pair was last rotated.
type rotatingPair struct {
	checked *TokenBucketLimiter
	ignored *TokenBucketLimiter
	rotated time56.Time
}

// RotatingTokenBucketRateLimiter implements a collision-resistant
// token bucket rate limiter. It maintains two TokenBucketLimiters
// with different hash seeds and rotates between them periodically.
// This approach minimizes the impact of hash collisions on rate
// limiting accuracy.
//
// The limiter works by:
//  1. Maintaining two TokenBucketLimiters with different hash seeds
//  2. Using one as the primary "checked" limiter for rate limiting
//     decisions
//  3. Using the other as an "ignored" limiter to maintain state
//  4. Periodically rotating roles and generating new seeds
//  5. Always checking both limiters but only using the "checked"
//
// result
//
// This design ensures that hash collisions between different IDs only
// last for the duration of the rotation period, providing better
// fairness and accuracy compared to a single TokenBucketLimiter.
type RotatingTokenBucketRateLimiter struct {
	pair             atomic.Pointer[rotatingPair] // Current limiter pair
	nanosPerRotation int64                        // Rotation interval in nanoseconds
}

// NewRotatingTokenBucketRateLimiter creates a new collision-resistant
// token bucket rate limiter with the specified parameters:
//
//   - numBuckets: number of token buckets per limiter (automatically
//     rounded up to the nearest power of two if not already a power
//     of two, for efficient hashing)
//   - burstCapacity: maximum number of tokens that can be consumed at
//     once
//   - refillRate: rate at which tokens are refilled (must be positive
//     and finite)
//   - refillRateUnit: time unit for refill rate calculations (e.g.,
//     time.Second, must be a positive duration)
//   - rotationRate: how often to rotate the bucket pairs and generate
//     new hash seeds (must be a positive duration)
//
// The limiter creates two identical TokenBucketLimiters with
// different hash seeds. It rotates between them every rotationRate
// duration to minimize the impact of hash collisions. When rotation
// occurs:
//
//  1. The current "ignored" limiter becomes the new "checked" limiter
//  2. A copy of the new "checked" limiter is made with a fresh hash
//     seed to become the new "ignored" limiter
//  3. Both limiters are consulted on every operation, but only the
//     "checked" result determines the rate limiting decision
//
// This design ensures that any hash collisions between different IDs
// will only persist for at most one rotation period, providing better
// fairness and accuracy than a single TokenBucketLimiter.
//
// Input validation follows the same rules as NewTokenBucketLimiter,
// with an additional requirement that rotationRate must be positive.
//
// Returns a new RotatingTokenBucketRateLimiter instance and any error
// that occurred during creation.
func NewRotatingTokenBucketRateLimiter(
	numBuckets uint,
	burstCapacity uint8,
	refillRate float64,
	refillRateUnit time.Duration,
	rotationRate time.Duration,
) (*RotatingTokenBucketRateLimiter, error) {
	if rotationRate <= 0 {
		return nil, fmt.Errorf("rotationRate must represent a positive duration")
	}

	checked, err := NewTokenBucketLimiter(
		numBuckets,
		burstCapacity,
		refillRate,
		refillRateUnit,
	)
	if err != nil {
		return nil, err
	}

	// validation passed for exact params above, continue w/o checking
	// error for 100% coverage.

	ignored, _ := NewTokenBucketLimiter(
		numBuckets,
		burstCapacity,
		refillRate,
		refillRateUnit,
	)

	limiter := &RotatingTokenBucketRateLimiter{
		nanosPerRotation: rotationRate.Nanoseconds(),
	}

	pair := &rotatingPair{
		checked: checked,
		ignored: ignored,
		rotated: time56.Unix(nowfn()),
	}

	limiter.pair.Store(pair)

	return limiter, nil
}

// load retrieves the current rotatingPair, performing rotation if
// necessary. If enough time has passed since the last rotation
// (determined by nanosPerRotation), it creates a new pair by:
//
//  1. Making the current "ignored" limiter the new "checked" limiter
//  2. Creating a copy of the new "checked" limiter with a fresh hash
//     seed to become the new "ignored" limiter
//  3. Updating the rotation timestamp
//
// The method uses atomic compare-and-swap operations to handle
// concurrent access safely. If multiple goroutines attempt rotation
// simultaneously, only one will succeed, and the others will receive
// the newly rotated pair.
//
// This approach ensures that hash collisions are resolved
// periodically without affecting the thread-safety or performance of
// the limiter.
func (r *RotatingTokenBucketRateLimiter) load(nowNS int64) *rotatingPair {
	now := time56.Unix(nowNS)

	for {
		pair := r.pair.Load()

		if now.Since(pair.rotated) < r.nanosPerRotation {
			return pair
		}

		ignored := *pair.checked
		ignored.seed = maphash.MakeSeed()

		next := &rotatingPair{
			checked: pair.ignored,
			ignored: &ignored,
			rotated: now,
		}

		if r.pair.CompareAndSwap(pair, next) {
			return next
		}
	}
}

// Check returns whether a token would be available for the given ID
// without actually taking it. This is useful for preemptively
// checking if an operation would be rate limited before attempting
// it.
//
// The method operates on both the "checked" and "ignored" limiters
// using the same timestamp to maintain consistency. The "ignored"
// limiter is checked to keep its state synchronized, but its result
// is discarded. Only the result from the "checked" limiter determines
// the return value.
//
// This dual-checking approach ensures that state is maintained in
// both limiters during rotation periods, preventing token loss and
// maintaining collision resistance.
//
// Returns true if a token would be available, false otherwise. This
// method is thread-safe and can be called concurrently from multiple
// goroutines.
func (r *RotatingTokenBucketRateLimiter) Check(id []byte) bool {
	now := nowfn()
	pair := r.load(now)
	pair.ignored.checkWithNow(id, now)
	return pair.checked.checkWithNow(id, now)
}

// TakeToken attempts to take a token for the given ID. It returns
// true if a token was successfully taken, false if the operation
// should be rate limited.
//
// The method operates on both the "checked" and "ignored" limiters
// using the same timestamp to maintain consistency. Both limiters
// consume tokens to keep their state synchronized, but only the
// result from the "checked" limiter determines the return value.
//
// This design intentionally consumes 2x tokens per call to provide
// collision resistance. The trade-off is:
//
//	Cost: 2x token consumption rate
//	Benefit: Hash collisions only persist for one rotation period
//
// This dual-operation approach ensures that:
//
//  1. State is maintained in both limiters during rotation periods
//  2. When rotation occurs, the "ignored" limiter (with different
//     hash seed) becomes the new "checked" limiter, breaking hash
//     collisions
//  3. Hash collisions only affect accuracy for the duration of
//     rotationRate
//  4. The system provides better fairness than a single limiter
//
// This method is thread-safe and can be called concurrently from
// multiple goroutines.
func (r *RotatingTokenBucketRateLimiter) TakeToken(id []byte) bool {
	now := nowfn()
	pair := r.load(now)
	pair.ignored.takeTokenWithNow(id, now)
	return pair.checked.takeTokenWithNow(id, now)
}
