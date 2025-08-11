package rate

import (
	"fmt"
	"math"
	"time"
)

// AIMDTokenBucketLimiter wraps a TokenBucketLimiter to implement
// Additive Increase Multiplicative Decrease (AIMD) rate limiting.
// This provides a dynamic rate limiting strategy that gradually
// increases token rates during successful operations and quickly
// reduces rates when encountering failures or congestion. This is
// similar to the congestion control algorithm used in TCP.
type AIMDTokenBucketLimiter struct {
	limiter  *TokenBucketLimiter
	rates    atomicSliceFloat64 // Per-bucket rates in tokens per unit
	rateMin  float64            // Minimum rate (tokens per unit)
	rateMax  float64            // Maximum rate (tokens per unit)
	rateAI   float64            // Additive increase (tokens per unit)
	rateMD   float64            // Multiplicative decrease (multiplier)
	rateUnit time.Duration      // Time unit for rate calculations
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
//   - If rateInit is not a positive, finite number (e.g., negative,
//     zero, NaN, or infinity), returns an error with message
//     "refillRate must be a positive, finite number"
//   - If rateUnit is not a positive duration, returns an error
//     with message "refillRateUnit must represent a positive
//     duration"
//   - If the product of rateInit and rateUnit (in nanoseconds)
//     exceeds maximum representable value, returns an error with
//     message "refillRate per duration is too large"
//   - If rateMin is not a positive, finite number (e.g., negative,
//     zero, NaN, or infinity), returns an error with message
//     "rateMin must be a positive, finite number"
//   - If rateMax is not a positive, finite number (e.g., negative,
//     zero, NaN, or infinity), returns an error with message
//     "rateMax must be a positive, finite number"
//   - If rateMin is greater than rateMax, returns an error with
//     message "rateMin must be less than equal to rateMax"
//   - If rateInit is not between rateMin and rateMax (inclusive),
//     returns an error with message "rateInit must be a positive,
//     finite number between rateMin and rateMax"
//   - If rateAdditiveIncrease is not a positive, finite number
//     (e.g., negative, zero, NaN, or infinity), returns an error
//     with message "rateAdditiveIncrease must be a positive, finite
//     number"
//   - If rateMultiplicativeDecrease is not a finite number greater
//     than or equal to 1.0 (e.g., NaN, infinity, or less than 1.0),
//     returns an error with message "rateMultiplicativeDecrease must
//     be a finite number greater than or equal to 1.0"
//   - If the product of rateMin, rateMax, or rateAdditiveIncrease
//     with rateUnit (in nanoseconds) exceeds maximum representable
//     value, returns an error with message "[parameter] per duration
//     is too large"
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

	if math.IsNaN(rateMin) || math.IsInf(rateMin, 0) || rateMin <= 0 {
		return nil, fmt.Errorf("rateMin must be a positive, finite number")
	}

	if math.IsNaN(rateMax) || math.IsInf(rateMax, 0) || rateMax <= 0 {
		return nil, fmt.Errorf("rateMax must be a positive, finite number")
	}

	if rateMin > rateMax {
		return nil, fmt.Errorf("rateMin must be less than equal to rateMax")
	}

	if math.IsNaN(rateInit) || math.IsInf(rateInit, 0) || rateInit < rateMin || rateInit > rateMax {
		return nil, fmt.Errorf("rateInit must be a positive, finite number between rateMin and rateMax")
	}

	if math.IsNaN(rateAdditiveIncrease) || math.IsInf(rateAdditiveIncrease, 0) || rateAdditiveIncrease <= 0 {
		return nil, fmt.Errorf("rateAdditiveIncrease must be a positive, finite number")
	}

	if math.IsNaN(rateMultiplicativeDecrease) || math.IsInf(rateMultiplicativeDecrease, 0) || rateMultiplicativeDecrease < 1.0 {
		return nil, fmt.Errorf("rateMultiplicativeDecrease must be a finite number greater than or equal to 1.0")
	}

	rateParams := []struct {
		value float64
		name  string
	}{
		{rateMin, "rateMin"},
		{rateMax, "rateMax"},
		{rateAdditiveIncrease, "rateAdditiveIncrease"},
	}

	rateUnitNanos := float64(rateUnit.Nanoseconds()) // validated by NewTokenBucketLimiter
	for _, rateParam := range rateParams {
		if rateUnitNanos > math.MaxFloat64/rateParam.value {
			return nil, fmt.Errorf("%s per duration is too large", rateParam.name)
		}
	}

	rates := newAtomicSliceFloat64(limiter.buckets.Len())
	for i := range rates.Len() {
		rates.Set(i, rateInit)
	}

	return &AIMDTokenBucketLimiter{
		limiter:  limiter,
		rates:    rates,
		rateMin:  rateMin,
		rateMax:  rateMax,
		rateAI:   rateAdditiveIncrease,
		rateMD:   rateMultiplicativeDecrease,
		rateUnit: rateUnit,
	}, nil
}

// TakeToken attempts to take a token for the given ID from the
// appropriate bucket. It returns true if a token was successfully
// taken, false otherwise. This method is thread-safe and can be
// called concurrently from multiple goroutines.
func (a *AIMDTokenBucketLimiter) TakeToken(id []byte) bool {
	return a.TakeTokens(id, 1)
}

// TakeTokens attempts to take n tokens for the given ID. It returns
// true if all n tokens were successfully taken, false if the
// operation should be rate limited. This method is thread-safe and
// can be called concurrently from multiple goroutines. The operation
// is atomic: either all n tokens are taken, or none are taken.
func (a *AIMDTokenBucketLimiter) TakeTokens(id []byte, n uint8) bool {
	now := nowfn()
	index := a.limiter.index(id)
	rate := a.rates.Get(index)
	nano := nanoRate(a.rateUnit, rate)
	return a.limiter.takeTokenInner(index, nano, now, n)
}

// CheckToken returns whether a token would be available for the given
// ID without actually taking it. This is useful for preemptively
// checking if an operation would be rate limited before attempting
// it. Returns true if a token would be available, false otherwise.
// This method is thread-safe and can be called concurrently from
// multiple goroutines.
func (a *AIMDTokenBucketLimiter) CheckToken(id []byte) bool {
	return a.CheckTokens(id, 1)
}

// CheckTokens returns whether n tokens would be available for the
// given ID without actually taking them. This is useful for
// preemptively checking if an operation would be rate limited before
// attempting it. Returns true if all n tokens would be available,
// false otherwise. This method is thread-safe and can be called
// concurrently from multiple goroutines.
func (a *AIMDTokenBucketLimiter) CheckTokens(id []byte, n uint8) bool {
	now := nowfn()
	index := a.limiter.index(id)
	rate := a.rates.Get(index)
	nano := nanoRate(a.rateUnit, rate)
	return a.limiter.checkInner(index, nano, now, n)
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
			return rate
		}

		next := a.rateMax
		if avail := a.rateMax - rate; a.rateAI < avail {
			next = rate + a.rateAI
		}

		if rate == next {
			return rate
		}

		if a.rates.CompareAndSwap(index, rate, next) {
			return rate
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
			return rate
		}

		next := max(a.rateMin, a.rateMin+(rate-a.rateMin)/a.rateMD)
		if rate == next {
			return rate
		}

		if a.rates.CompareAndSwap(index, rate, next) {
			return rate
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
	return a.rates.Get(index)
}
