package rate

import "time"

// AIMDTokenBucketLimiter wraps a TokenBucketLimiter to implement
// Additive Increase Multiplicative Decrease (AIMD) rate limiting.
// This provides a dynamic rate limiting strategy that gradually
// increases token rates during successful operations and quickly
// reduces rates when encountering failures or congestion. This is
// similar to the congestion control algorithm used in TCP.
type AIMDTokenBucketLimiter struct {
	limiter *TokenBucketLimiter
	rates   atomicSliceInt64 // Per-bucket rates in tokens per second
	rateMin int64            // Minimum rate (tokens per unit)
	rateMax int64            // Maximum rate (tokens per unit)
	rateAI  int64            // Additive increase (tokens per unit)
	rateMD  float64          // Multiplicative decrease (multiplier)
}

// NewAIMDTokenBucketLimiter creates a new AIMD token bucket limiter
// with the given parameters:
//
//   - numBuckets: number of token buckets (automatically rounded up
//     to the nearest power of two if not already a power of two, for
//     efficient hashing)
//   - burstCapacity: max number of tokens that can be consumed at
//     once
//   - rateMin: minimum token refill rate
//   - rateMax: maximum token refill rate
//   - rateInit: initial token refill rate
//   - rateAdditiveIncrease: amount to increase rate by on success
//   - rateMultiplicativeDecrease: factor to decrease rate by on
//     failure
//   - rateUnit: time unit for rate calculations (e.g., time.Second)
//
// All rates are expressed in tokens per rateUnit.
func NewAIMDTokenBucketLimiter(
	numBuckets uint,
	burstCapacity uint8,
	rateMin float64,
	rateMax float64,
	rateInit float64,
	rateAdditiveIncrease float64,
	rateMultiplicativeDecrease float64,
	rateUnit time.Duration,
) (*AIMDTokenBucketLimiter, error) {
	// no errors currently thrown in NewTokenBucketLimiter(), ignore for
	// code coverage
	limiter, _ := NewTokenBucketLimiter(
		numBuckets,
		burstCapacity,
		rateInit,
		rateUnit,
	)

	rate := nanoRate(rateUnit, rateInit)
	rates := newAtomicSliceInt64(limiter.buckets.Len())
	for i := range rates.Len() {
		rates.Set(i, rate)
	}

	return &AIMDTokenBucketLimiter{
		limiter: limiter,
		rates:   rates,
		rateMin: nanoRate(rateUnit, rateMin),
		rateMax: nanoRate(rateUnit, rateMax),
		rateAI:  nanoRate(rateUnit, rateAdditiveIncrease),
		rateMD:  rateMultiplicativeDecrease,
	}, nil
}

// TakeToken attempts to take a token for the given ID from the
// appropriate bucket. It returns true if a token was successfully
// taken, false otherwise. This method is thread-safe and can be
// called concurrently from multiple goroutines.
func (a *AIMDTokenBucketLimiter) TakeToken(id []byte) bool {
	index := a.limiter.index(id)
	rate := a.rates.Get(index)
	return a.limiter.takeTokenInner(index, rate)
}

// Check returns whether a token would be available for the given ID
// without actually taking it. This is useful for preemptively
// checking if an operation would be rate limited before attempting
// it. Returns true if a token would be available, false otherwise.
// This method is thread-safe and can be called concurrently from
// multiple goroutines.
func (a *AIMDTokenBucketLimiter) Check(id []byte) bool {
	index := a.limiter.index(id)
	rate := a.rates.Get(index)
	return a.limiter.checkInner(index, rate)
}

// IncreaseRate additively increases the rate for the bucket
// associated with the given ID. This implements the "additive
// increase" part of the AIMD algorithm, typically called on
// successful operations. The rate is increased by rateAI up to the
// maximum rate (rateMax). This method is thread-safe and uses atomic
// operations to ensure consistency.
func (a *AIMDTokenBucketLimiter) IncreaseRate(id []byte) {
	index := a.limiter.index(id)
	for {
		rate := a.rates.Get(index)
		if rate == a.rateMax {
			return
		}

		next := a.rateMax
		if avail := a.rateMax - rate; a.rateAI < avail {
			next = rate + a.rateAI
		}

		if rate == next {
			return
		}

		if a.rates.CompareAndSwap(index, rate, next) {
			return
		}
	}
}

// DecreaseRate multiplicatively decreases the rate for the bucket
// associated with the given ID. This implements the "multiplicative
// decrease" part of the AIMD algorithm, typically called when
// congestion or failures occur. The rate is decreased by dividing by
// rateMD, but will not go below the minimum rate (rateMin). This
// method is thread-safe and uses atomic operations to ensure
// consistency.
func (a *AIMDTokenBucketLimiter) DecreaseRate(id []byte) {
	index := a.limiter.index(id)
	for {
		rate := a.rates.Get(index)
		if rate == a.rateMin {
			return
		}

		next := max(int64(float64(rate)/a.rateMD), a.rateMin)
		if rate == next {
			return
		}

		if a.rates.CompareAndSwap(index, rate, next) {
			return
		}
	}
}
