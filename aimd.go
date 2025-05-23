package rate

import "time"

// AIMDTokenBucketLimiter wraps a TokenBucketLimiter to implement
// Additive Increase Multiplicative Decrease (AIMD) rate limiting.
// This provides a dynamic rate limiting strategy that gradually
// increases token rates during successful operations and quickly
// reduces rates when encountering failures or congestion. This is
// similar to the congestion control algorithm used in TCP.
type AIMDTokenBucketLimiter struct {
	limiter  *TokenBucketLimiter
	rates    atomicSliceInt64 // Per-bucket rates in tokens per second
	rateMin  int64            // Minimum rate (tokens per unit)
	rateMax  int64            // Maximum rate (tokens per unit)
	rateAI   int64            // Additive increase (tokens per unit)
	rateMD   float64          // Multiplicative decrease (multiplier)
	rateUnit time.Duration    // Time unit for rate calculations
}

// NewAIMDTokenBucketLimiter creates a new AIMD token bucket limiter
// with the given parameters:
//
//   - numBuckets: number of token buckets (automatically rounded up
//     to the nearest power of two if not already a power of two, for
//     efficient hashing)
//   - burstCapacity: max number of tokens that can be consumed at
//     once
//   - rateMin: minimum token refill rate (must be positive and
//     finite)
//   - rateMax: maximum token refill rate (must be greater than or
//     equal to rateMin)
//   - rateInit: initial token refill rate (must be between rateMin
//     and rateMax)
//   - rateAdditiveIncrease: amount to increase rate by on success
//     (typically a small value)
//   - rateMultiplicativeDecrease: factor to decrease rate by on
//     failure (typically a value like 2.0 meaning "divide by 2")
//   - rateUnit: time unit for rate calculations (e.g., time.Second,
//     must be positive)
//
// All rates are expressed in tokens per rateUnit.
//
// Input validation:
//
//   - If rateInit or rateUnit parameters are invalid, returns the
//     same error that would be returned by NewTokenBucketLimiter
//   - If refillRate is not a positive, finite number (e.g., negative,
//     zero, NaN, or infinity), returns an error with message
//     "refillRate must be a positive, finite number"
//   - If refillRateUnit is not a positive duration, returns an error
//     with message "refillRateUnit must represent a positive
//     duration"
//   - If the product of refillRate and refillRateUnit (in
//     nanoseconds) exceeds maximum representable value, returns an
//     error with message "refillRate per duration is too large"
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
	limiter, err := NewTokenBucketLimiter(
		numBuckets,
		burstCapacity,
		rateInit,
		rateUnit,
	)
	if err != nil {
		return nil, err
	}

	rate := nanoRate(rateUnit, rateInit)
	rates := newAtomicSliceInt64(limiter.buckets.Len())
	for i := range rates.Len() {
		rates.Set(i, rate)
	}

	return &AIMDTokenBucketLimiter{
		limiter:  limiter,
		rates:    rates,
		rateMin:  nanoRate(rateUnit, rateMin),
		rateMax:  nanoRate(rateUnit, rateMax),
		rateAI:   nanoRate(rateUnit, rateAdditiveIncrease),
		rateMD:   rateMultiplicativeDecrease,
		rateUnit: rateUnit,
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
//
// Returns the current rate (in tokens per rateUnit) for the bucket
// before the increase was applied.
func (a *AIMDTokenBucketLimiter) IncreaseRate(id []byte) float64 {
	index := a.limiter.index(id)
	for {
		rate := a.rates.Get(index)
		if rate == a.rateMax {
			return unitRate(a.rateUnit, rate)
		}

		next := a.rateMax
		if avail := a.rateMax - rate; a.rateAI < avail {
			next = rate + a.rateAI
		}

		if rate == next {
			return unitRate(a.rateUnit, rate)
		}

		if a.rates.CompareAndSwap(index, rate, next) {
			return unitRate(a.rateUnit, rate)
		}
	}
}

// DecreaseRate multiplicatively decreases the rate for the bucket
// associated with the given ID. This implements the "multiplicative
// decrease" part of the AIMD algorithm, typically called when
// congestion or failures occur. The rate is decreased by dividing the
// distance from rateMin by rateMD, calculated as: rateMin +
// (currentRate - rateMin) / rateMD. This ensures more gradual
// decreases near the minimum rate. The rate will not go below the
// minimum rate (rateMin). This method is thread-safe and uses atomic
// operations to ensure consistency.
//
// Returns the current rate (in tokens per rateUnit) for the bucket
// before the decrease was applied.
func (a *AIMDTokenBucketLimiter) DecreaseRate(id []byte) float64 {
	index := a.limiter.index(id)
	for {
		rate := a.rates.Get(index)
		if rate == a.rateMin {
			return unitRate(a.rateUnit, rate)
		}

		next := max(a.rateMin, a.rateMin+int64(float64(rate-a.rateMin)/a.rateMD))
		if rate == next {
			return unitRate(a.rateUnit, rate)
		}

		if a.rates.CompareAndSwap(index, rate, next) {
			return unitRate(a.rateUnit, rate)
		}
	}
}

// Rate returns the current token rate for the bucket associated with
// the given ID. The rate is expressed in tokens per rateUnit (e.g.,
// tokens per second if rateUnit is time.Second). This method is
// thread-safe and can be called concurrently from multiple
// goroutines.
func (a *AIMDTokenBucketLimiter) Rate(id []byte) float64 {
	index := a.limiter.index(id)
	rate := a.rates.Get(index)
	return unitRate(a.rateUnit, rate)
}
