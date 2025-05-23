package rate

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"
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
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("test")
	// Take burstCapacity tokens
	for i := range int(burstCapacity) {
		if !limiter.TakeToken(id) {
			t.Errorf("TakeToken failed at i=%d, should be able to take token", i)
		}
	}

	// Should be rate limited after burst
	if limiter.TakeToken(id) {
		t.Error("TakeToken should be rate limited after burst")
	}
}

// TestAIMDLimiterRoundingToPowerOfTwo verifies numBuckets is rounded up to the next power of two
func TestAIMDLimiterRoundingToPowerOfTwo(t *testing.T) {
	// Test with non-power-of-two bucket size
	limiter, err := NewAIMDTokenBucketLimiter(
		3, // Not power of two, should be rounded up to 4
		burstCapacity,
		aimdMinRate,
		aimdMaxRate,
		aimdInitRate,
		aimdIncreaseByRate,
		aimdDecreaseByFactor,
		time.Second,
	)
	if err != nil {
		t.Fatalf("Expected no error with non-power-of-two bucket count, got: %v", err)
	}

	// Check that the inner token bucket limiter has 4 buckets (next power of two after 3)
	if limiter.limiter.buckets.Len() != 4 {
		t.Errorf("Expected numBuckets to be rounded up to 4, got %d", limiter.limiter.buckets.Len())
	}
}

// TestNewAIMDTokenBucketLimiterError tests error cases in NewAIMDTokenBucketLimiter
func TestNewAIMDTokenBucketLimiterError(t *testing.T) {
	tests := []struct {
		name              string
		numBuckets        uint
		burstCapacity     uint8
		rateMin           float64
		rateMax           float64
		rateInit          float64
		rateAI            float64
		rateMD            float64
		rateUnit          time.Duration
		wantErr           bool
		errContainsString string
	}{
		{
			name:          "valid parameters",
			numBuckets:    16,
			burstCapacity: 10,
			rateMin:       1.0,
			rateMax:       10.0,
			rateInit:      5.0,
			rateAI:        1.0,
			rateMD:        2.0,
			rateUnit:      time.Second,
			wantErr:       false,
		},
		{
			name:              "invalid rateInit",
			numBuckets:        16,
			burstCapacity:     10,
			rateMin:           1.0,
			rateMax:           10.0,
			rateInit:          -1.0, // Invalid negative value
			rateAI:            1.0,
			rateMD:            2.0,
			rateUnit:          time.Second,
			wantErr:           true,
			errContainsString: "refillRate must be a positive",
		},
		{
			name:              "invalid refillRateUnit",
			numBuckets:        16,
			burstCapacity:     10,
			rateMin:           1.0,
			rateMax:           10.0,
			rateInit:          5.0,
			rateAI:            1.0,
			rateMD:            2.0,
			rateUnit:          -1 * time.Second, // Invalid negative duration
			wantErr:           true,
			errContainsString: "refillRateUnit must represent a positive duration",
		},
		{
			name:              "rate overflow",
			numBuckets:        16,
			burstCapacity:     10,
			rateMin:           1.0,
			rateMax:           10.0,
			rateInit:          1e300, // Very large value
			rateAI:            1.0,
			rateMD:            2.0,
			rateUnit:          time.Second,
			wantErr:           true,
			errContainsString: "refillRate per duration is too large",
		},
		{
			name:              "NaN rate init",
			numBuckets:        16,
			burstCapacity:     10,
			rateMin:           1.0,
			rateMax:           10.0,
			rateInit:          math.NaN(), // NaN
			rateAI:            1.0,
			rateMD:            2.0,
			rateUnit:          time.Second,
			wantErr:           true,
			errContainsString: "refillRate must be a positive",
		},
		{
			name:              "zero rate unit",
			numBuckets:        16,
			burstCapacity:     10,
			rateMin:           1.0,
			rateMax:           10.0,
			rateInit:          5.0,
			rateAI:            1.0,
			rateMD:            2.0,
			rateUnit:          0, // Zero duration
			wantErr:           true,
			errContainsString: "refillRateUnit must represent a positive duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAIMDTokenBucketLimiter(
				tt.numBuckets,
				tt.burstCapacity,
				tt.rateMin,
				tt.rateMax,
				tt.rateInit,
				tt.rateAI,
				tt.rateMD,
				tt.rateUnit,
			)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewAIMDTokenBucketLimiter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errContainsString != "" {
				// We got an error and have a string to check
				if !strings.Contains(err.Error(), tt.errContainsString) {
					t.Errorf("NewAIMDTokenBucketLimiter() error = %v, should contain %q", err, tt.errContainsString)
				}
			}
		})
	}
}

// TestAIMDIncreaseRate checks that rate increases on successful takes.
func TestAIMDIncreaseRate(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("increase")
	index := limiter.limiter.index(id)
	initialRate := limiter.rates.Get(index)

	// Take tokens and increase rate
	for range 5 {
		if !limiter.TakeToken(id) {
			t.Error("TakeToken should succeed")
		}
		limiter.IncreaseRate(id)
		tick(10 * time.Millisecond) // Simulate time passing for refill
	}

	finalRate := limiter.rates.Get(index)
	if finalRate <= initialRate {
		t.Error("Rate should increase")
	}

	expectedRate := sec(initialRate) + 5
	if math.Abs(sec(finalRate)-expectedRate) > 0.001 {
		t.Errorf("Rate should increase by 5: got %f, want %f", sec(finalRate), expectedRate)
	}
}

// TestAIMDDecreaseRate checks that rate decreases on throttling feedback.
func TestAIMDDecreaseRate(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("decrease")
	index := limiter.limiter.index(id)

	// Increase rate first
	for range 10 {
		limiter.IncreaseRate(id)
	}

	rate := sec(limiter.rates.Get(index))
	if math.Abs(rate-11.0) > 0.001 { // 1 + 10 = 11.0
		t.Errorf("Initial rate incorrect: got %f, want 11.0", rate)
	}

	// Simulate throttling
	limiter.DecreaseRate(id)
	rate = sec(limiter.rates.Get(index))
	if math.Abs(rate-5.5) > 0.001 { // 11 / 2 = 5.5
		t.Errorf("After first decrease: got %f, want 5.5", rate)
	}

	limiter.DecreaseRate(id)
	rate = sec(limiter.rates.Get(index))
	if math.Abs(rate-2.75) > 0.001 { // 5.5 / 2 = 2.75
		t.Errorf("After second decrease: got %f, want 2.75", rate)
	}

	limiter.DecreaseRate(id)
	rate = sec(limiter.rates.Get(index))
	if math.Abs(rate-1.375) > 0.001 { // 2.75 / 2 = 1.375
		t.Errorf("After third decrease: got %f, want 1.375", rate)
	}

	limiter.DecreaseRate(id)
	rate = sec(limiter.rates.Get(index))
	if math.Abs(rate-aimdMinRate) > 0.001 { // 1.375 / 2 < 1, so minRate
		t.Errorf("After fourth decrease: got %f, want %f (minRate)", rate, aimdMinRate)
	}
}

// TestAIMDRateEdgeCases verifies rate increase and decrease edge cases
func TestAIMDRateEdgeCases(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("edge-cases")
	index := limiter.limiter.index(id)

	// Test increase when rate equals max rate (early return case)
	limiter.rates.Set(index, limiter.rateMax)
	initialRate := limiter.rates.Get(index)
	limiter.IncreaseRate(id)
	if initialRate != limiter.rates.Get(index) {
		t.Error("Rate should not change when at max")
	}

	// Test case where rate == next (no change needed)
	limiter.rates.Set(index, limiter.rateMax-1)
	initialRate = limiter.rates.Get(index)
	// Force a situation where rate + increase == rate (unlikely in real usage)
	// This happens when increase is so small it doesn't change the int64 value
	limiter.rateAI = 0 // artificially set to 0 for test
	limiter.IncreaseRate(id)
	if initialRate != limiter.rates.Get(index) {
		t.Error("Rate should not change when increase is too small")
	}

	// Test decrease when rate equals min rate (early return case)
	limiter.rates.Set(index, limiter.rateMin)
	initialRate = limiter.rates.Get(index)
	limiter.DecreaseRate(id)
	if initialRate != limiter.rates.Get(index) {
		t.Error("Rate should not change when at min")
	}
}

// TestAIMDDecreaseWithUnchangedRate explicitly tests the case where
// the rate doesn't change after division by rateMD
func TestAIMDDecreaseWithUnchangedRate(t *testing.T) {
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
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

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
	if initialRate != limiter.rates.Get(index) {
		t.Error("Rate should not change when divided by rateMD=1.0")
	}
}

// TestAIMDRateLimits verifies that rates stay within minRate and maxRate.
func TestAIMDRateLimits(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("limits")
	index := limiter.limiter.index(id)

	// Increase rate beyond max
	for range int(aimdMaxRate + 10) {
		limiter.IncreaseRate(id)
	}

	rate := sec(limiter.rates.Get(index))
	if math.Abs(rate-aimdMaxRate) > 0.001 {
		t.Errorf("Rate should not exceed maxRate: got %f, want %f", rate, aimdMaxRate)
	}

	// Decrease rate below min
	for range 10 {
		limiter.DecreaseRate(id)
	}

	rate = sec(limiter.rates.Get(index))
	if math.Abs(rate-aimdMinRate) > 0.001 {
		t.Errorf("Rate should not drop below minRate: got %f, want %f", rate, aimdMinRate)
	}
}

// TestAIMDConcurrency tests concurrent token takes and rate adjustments.
func TestAIMDConcurrency(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("concurrent")
	var wg sync.WaitGroup
	const threads = 10
	const iterations = 50

	for range threads {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for range iterations {
				if limiter.TakeToken(id) {
					limiter.IncreaseRate(id)
				} else {
					limiter.DecreaseRate(id)
				}
				tick(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	rate := limiter.rates.Get(limiter.limiter.index(id))
	ratePerSecond := sec(rate)

	if ratePerSecond < aimdMinRate {
		t.Errorf("Rate below minimum: got %f, want at least %f", ratePerSecond, aimdMinRate)
	}

	if ratePerSecond > aimdMaxRate {
		t.Errorf("Rate above maximum: got %f, want at most %f", ratePerSecond, aimdMaxRate)
	}
}

// TestAIMDRateAfterBurst tests rate limiting after burst with AIMD adjustments.
func TestAIMDRateAfterBurst(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("rate")
	// Exhaust burst capacity
	for range int(burstCapacity) {
		if !limiter.TakeToken(id) {
			t.Error("TakeToken should succeed during burst")
		}
	}

	if limiter.TakeToken(id) {
		t.Error("TakeToken should fail after burst")
	}

	allowed := 0
	for range 15 {
		if limiter.TakeToken(id) {
			allowed++
			limiter.IncreaseRate(id)
		}
		tick(100 * time.Millisecond)
	}

	if allowed == 0 {
		t.Error("Some tokens should refill based on rate")
	}
}

// TestAIMDCheck verifies that Check method functions correctly.
func TestAIMDCheck(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("check-test")

	// Initially, tokens should be available
	if !limiter.Check(id) {
		t.Error("Check should return true initially (tokens available)")
	}

	// After taking all tokens, no tokens should be available
	for range int(burstCapacity) {
		if !limiter.TakeToken(id) {
			t.Error("TakeToken should succeed")
		}
	}

	// Check should return false when no tokens are available
	if limiter.Check(id) {
		t.Error("Check should return false after burst (rate limited)")
	}

	// Check should not consume tokens
	if limiter.Check(id) {
		t.Error("Check should not consume tokens")
	}

	// After some time, tokens should be available again
	tick(time.Second * 2) // Allow refill based on rate

	// Check should now return true
	if !limiter.Check(id) {
		t.Error("Check should return true after refill")
	}

	// Taking tokens should still work after checking
	if !limiter.TakeToken(id) {
		t.Error("TakeToken should succeed after refill")
	}
}

// TestAIMDCheckAfterConsumption tests that Check reports correctly after tokens have been used.
func TestAIMDCheckAfterConsumption(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("check-vs-take")

	// Initially should have tokens
	if !limiter.Check(id) {
		t.Error("Check should return true initially (tokens available)")
	}

	// Take all tokens
	for i := 0; i < int(burstCapacity); i++ {
		if !limiter.TakeToken(id) {
			t.Errorf("TakeToken should succeed at i=%d", i)
		}
	}

	// Bucket should now be empty
	if limiter.TakeToken(id) {
		t.Error("TakeToken should fail when bucket is empty")
	}

	// Check should also report empty
	if limiter.Check(id) {
		t.Error("Check should return false when bucket is empty")
	}

	// Wait for some refill
	tick(time.Second)

	// Should eventually have tokens again
	if !limiter.Check(id) {
		t.Error("Check should return true after refill")
	}

	if !limiter.TakeToken(id) {
		t.Error("TakeToken should succeed after refill")
	}
}

// TestAIMDRate tests the Rate method returns the correct token rate.
func TestAIMDRate(t *testing.T) {
	limiter, err := DefaultAIMDLimiter()
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	id := []byte("rate-test")
	index := limiter.limiter.index(id)

	// Check initial rate
	initialRateExpected := aimdInitRate
	initialRateActual := limiter.Rate(id)
	if math.Abs(initialRateActual-initialRateExpected) > 0.001 {
		t.Errorf("Initial rate incorrect: got %f, want %f", initialRateActual, initialRateExpected)
	}

	// Set a custom rate and check that it's reported correctly
	customRate := 42.0
	limiter.rates.Set(index, nanoRate(time.Second, customRate))

	rateActual := limiter.Rate(id)
	if math.Abs(rateActual-customRate) > 0.001 {
		t.Errorf("Rate incorrect after setting: got %f, want %f", rateActual, customRate)
	}

	// Increase rate and check both the return value and that Rate method reports the increase
	returnedRate := limiter.IncreaseRate(id)
	increasedRateExpected := customRate + aimdIncreaseByRate
	increasedRateActual := limiter.Rate(id)

	// Verify IncreaseRate returned the previous rate
	if math.Abs(returnedRate-customRate) > 0.001 {
		t.Errorf("IncreaseRate returned incorrect rate: got %f, want %f", returnedRate, customRate)
	}

	// Verify new rate is correctly reported by Rate()
	if math.Abs(increasedRateActual-increasedRateExpected) > 0.001 {
		t.Errorf("Rate incorrect after increase: got %f, want %f", increasedRateActual, increasedRateExpected)
	}

	// Decrease rate and check both the return value and that Rate method reports the decrease
	returnedRate = limiter.DecreaseRate(id)
	decreasedRateExpected := increasedRateExpected / aimdDecreaseByFactor
	decreasedRateActual := limiter.Rate(id)

	// Verify DecreaseRate returned the previous rate
	if math.Abs(returnedRate-increasedRateExpected) > 0.001 {
		t.Errorf("DecreaseRate returned incorrect rate: got %f, want %f", returnedRate, increasedRateExpected)
	}

	// Verify new rate is correctly reported by Rate()
	if math.Abs(decreasedRateActual-decreasedRateExpected) > 0.001 {
		t.Errorf("Rate incorrect after decrease: got %f, want %f", decreasedRateActual, decreasedRateExpected)
	}
}

func sec(rate int64) float64 {
	return unitRate(time.Second, rate)
}
