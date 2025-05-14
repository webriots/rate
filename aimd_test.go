package rate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const (
	aimdMinRate          = 1.0   // 1 token/s
	aimdMaxRate          = 100.0 // 100 tokens/s
	aimdInitRate         = 1.0   // 1 tokens/s
	aimdIncreaseByRate   = 1.0   // 1 tokens/s
	aimdDecreaseByFactor = 2.0
)

// DefaultAIMDLimiter creates an AIMDLimiter with default settings for testing.
func DefaultAIMDLimiter() (*AIMDTokenBucketLimiter, error) {
	return NewAIMDTokenBucketLimiter(
		numBuckets,
		burstCapacity,
		aimdMinRate,
		aimdMaxRate,
		aimdInitRate,
		aimdIncreaseByRate,
		aimdDecreaseByFactor,
		time.Second,
	)
}

// TestAIMDBasic verifies basic token taking and rate initialization.
func TestAIMDBasic(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("test")
	// Take burstCapacity tokens
	for i := range int(burstCapacity) {
		r.True(limiter.TakeToken(id), "should take token %d", i)
	}
	r.False(limiter.TakeToken(id), "should be rate limited after burst")
}

// TestAIMDLimiterError tests error case in constructor
func TestAIMDLimiterError(t *testing.T) {
	r := require.New(t)
	// Test with non-power-of-two bucket size which should fail
	_, err := NewAIMDTokenBucketLimiter(
		3, // Not power of two
		burstCapacity,
		aimdMinRate,
		aimdMaxRate,
		aimdInitRate,
		aimdIncreaseByRate,
		aimdDecreaseByFactor,
		time.Second,
	)
	r.Error(err, "should error on non-power-of-two bucket count")
}

// TestAIMDIncreaseRate checks that rate increases on successful takes.
func TestAIMDIncreaseRate(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("increase")
	index := limiter.limiter.index(id)
	initialRate := limiter.rates.Get(index)

	// Take tokens and increase rate
	for range 5 {
		r.True(limiter.TakeToken(id))
		limiter.IncreaseRate(id)
		tick(10 * time.Millisecond) // Simulate time passing for refill
	}

	finalRate := limiter.rates.Get(index)
	r.Greater(finalRate, initialRate, "rate should increase")
	r.Equal(sec(initialRate)+5, sec(finalRate), "rate should increase by 5")
}

// TestAIMDDecreaseRate checks that rate decreases on throttling feedback.
func TestAIMDDecreaseRate(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("decrease")
	index := limiter.limiter.index(id)

	// Increase rate first
	for range 10 {
		limiter.IncreaseRate(id)
	}
	r.Equal(11.0, sec(limiter.rates.Get(index))) // 1 + 10 = 11.0

	// Simulate throttling
	limiter.DecreaseRate(id)
	r.Equal(5.5, sec(limiter.rates.Get(index))) // 11 / 2 = 5.5

	limiter.DecreaseRate(id)
	r.Equal(2.75, sec(limiter.rates.Get(index))) // 5.5 / 2 = 2.75

	limiter.DecreaseRate(id)
	r.Equal(1.375, sec(limiter.rates.Get(index))) // 2.75 / 2 < 1.375

	limiter.DecreaseRate(id)
	r.Equal(aimdMinRate, sec(limiter.rates.Get(index))) // 1.375 / 2 < 1, so minRate
}

// TestAIMDRateEdgeCases verifies rate increase and decrease edge cases
func TestAIMDRateEdgeCases(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("edge-cases")
	index := limiter.limiter.index(id)

	// Test increase when rate equals max rate (early return case)
	limiter.rates.Set(index, limiter.rateMax)
	initialRate := limiter.rates.Get(index)
	limiter.IncreaseRate(id)
	r.Equal(initialRate, limiter.rates.Get(index), "rate should not change when at max")

	// Test case where rate == next (no change needed)
	limiter.rates.Set(index, limiter.rateMax-1)
	initialRate = limiter.rates.Get(index)
	/* next := initialRate + 1 */
	// Force a situation where rate + increase == rate (unlikely in real usage)
	// This happens when increase is so small it doesn't change the int64 value
	limiter.rateAI = 0 // artificially set to 0 for test
	limiter.IncreaseRate(id)
	r.Equal(initialRate, limiter.rates.Get(index), "rate should not change when increase is too small")

	// Test decrease when rate equals min rate (early return case)
	limiter.rates.Set(index, limiter.rateMin)
	initialRate = limiter.rates.Get(index)
	limiter.DecreaseRate(id)
	r.Equal(initialRate, limiter.rates.Get(index), "rate should not change when at min")
}

// TestAIMDDecreaseWithUnchangedRate explicitly tests the case where
// the rate doesn't change after division by rateMD
func TestAIMDDecreaseWithUnchangedRate(t *testing.T) {
	r := require.New(t)

	// Create a custom limiter for this test
	limiter, err := NewAIMDTokenBucketLimiter(
		numBuckets,
		burstCapacity,
		aimdMinRate,
		aimdMaxRate,
		aimdInitRate,
		aimdIncreaseByRate,
		1.0, // Set rateMD to 1.0 which will make rate/rateMD = rate
		time.Second,
	)
	r.NoError(err)

	id := []byte("rate-unchanged")
	index := limiter.limiter.index(id)

	// Set a rate well above the minimum
	testRate := limiter.rateMin * 10
	limiter.rates.Set(index, testRate)
	initialRate := limiter.rates.Get(index)

	// With rateMD = 1.0, the calculation rate/rateMD will equal rate
	// So this should trigger the "rate == next" case in DecreaseRate
	limiter.DecreaseRate(id)

	// Rate should remain unchanged
	r.Equal(initialRate, limiter.rates.Get(index),
		"rate should not change when divided by rateMD=1.0")
}

// TestAIMDRateLimits verifies that rates stay within minRate and maxRate.
func TestAIMDRateLimits(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("limits")
	index := limiter.limiter.index(id)

	// Increase rate beyond max
	for range int(aimdMaxRate + 10) {
		limiter.IncreaseRate(id)
	}
	r.Equal(aimdMaxRate, sec(limiter.rates.Get(index)), "rate should not exceed maxRate")

	// Decrease rate below min
	for range 10 {
		limiter.DecreaseRate(id)
	}
	r.Equal(aimdMinRate, sec(limiter.rates.Get(index)), "rate should not drop below minRate")
}

// TestAIMDConcurrency tests concurrent token takes and rate adjustments.
func TestAIMDConcurrency(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("concurrent")
	var group errgroup.Group
	const threads = 10
	const iterations = 50

	for range threads {
		group.Go(func() error {
			for range iterations {
				if limiter.TakeToken(id) {
					limiter.IncreaseRate(id)
				} else {
					limiter.DecreaseRate(id)
				}
				tick(1 * time.Millisecond)
			}
			return nil
		})
	}

	r.NoError(group.Wait())
	rate := limiter.rates.Get(limiter.limiter.index(id))
	r.GreaterOrEqual(sec(rate), aimdMinRate)
	r.LessOrEqual(sec(rate), aimdMaxRate)
}

// TestAIMDRateAfterBurst tests rate limiting after burst with AIMD adjustments.
func TestAIMDRateAfterBurst(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("rate")
	// Exhaust burst capacity
	for range int(burstCapacity) {
		r.True(limiter.TakeToken(id))
	}
	r.False(limiter.TakeToken(id))

	allowed := 0
	for range 15 {
		if limiter.TakeToken(id) {
			allowed++
			limiter.IncreaseRate(id)
		}
		tick(100 * time.Millisecond)
	}

	r.Greater(allowed, 0, "some tokens should refill based on rate")
}

// TestAIMDCheck verifies that Check method functions correctly.
func TestAIMDCheck(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("check-test")

	// Initially, tokens should be available
	r.True(limiter.Check(id), "should have tokens available initially")

	// After taking all tokens, no tokens should be available
	for range int(burstCapacity) {
		r.True(limiter.TakeToken(id), "should take token")
	}

	// Check should return false when no tokens are available
	r.False(limiter.Check(id), "should be rate limited after burst")

	// Check should not consume tokens
	r.False(limiter.Check(id), "check should not consume tokens")

	// After some time, tokens should be available again
	tick(time.Second * 2) // Allow refill based on rate

	// Check should now return true
	r.True(limiter.Check(id), "tokens should be available after refill")

	// Taking tokens should still work after checking
	r.True(limiter.TakeToken(id), "should be able to take token after check")
}

// TestAIMDCheckAfterConsumption tests that Check reports correctly after tokens have been used.
func TestAIMDCheckAfterConsumption(t *testing.T) {
	r := require.New(t)
	limiter, err := DefaultAIMDLimiter()
	r.NoError(err)

	id := []byte("check-vs-take")

	// Initially should have tokens
	r.True(limiter.Check(id), "should initially have tokens")

	// Take all tokens
	for i := 0; i < int(burstCapacity); i++ {
		r.True(limiter.TakeToken(id), "should take token %d", i)
	}

	// Bucket should now be empty
	r.False(limiter.TakeToken(id), "should not take token when empty")
	// Check should also report empty
	r.False(limiter.Check(id), "check should return false when bucket empty")

	// Wait for some refill
	tick(time.Second)

	// Should eventually have tokens again
	r.True(limiter.Check(id), "check should return true after refill")
	r.True(limiter.TakeToken(id), "take should succeed after refill")
}

func sec(rate int64) float64 {
	return unitRate(time.Second, rate)
}

func unitRate(refillRateUnit time.Duration, refillRateNanos int64) float64 {
	return float64(refillRateNanos) / float64(refillRateUnit.Nanoseconds())
}
