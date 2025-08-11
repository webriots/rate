# Rate

[![Go Reference](https://pkg.go.dev/badge/github.com/webriots/rate.svg)](https://pkg.go.dev/github.com/webriots/rate)
[![Go Report Card](https://goreportcard.com/badge/github.com/webriots/rate)](https://goreportcard.com/report/github.com/webriots/rate)
[![Coverage Status](https://coveralls.io/repos/github/webriots/rate/badge.svg?branch=main)](https://coveralls.io/github/webriots/rate?branch=main)

A high-performance rate limiter library for Go applications with multiple rate limiting strategies.

## Features

- **Ultra-Fast**: Core operations execute in single digit *nanoseconds* with zero allocations, enabling hundreds of millions of operations per second
- **Thread-Safe**: All operations are designed to be thread-safe using optimized atomic operations
- **Zero External Dependencies**: Relies solely on the Go standard library
- **Memory Efficient**: Uses compact data structures and optimized storage
  - Packed representations for token buckets
  - Custom 56-bit timestamp implementation
  - Efficient atomic slice implementations
- **Multiple Rate Limiting Strategies**:
  - TokenBucketLimiter: Classic token bucket algorithm with multiple buckets
  - AIMDTokenBucketLimiter: Additive-Increase/Multiplicative-Decrease algorithm inspired by TCP congestion control
  - RotatingTokenBucketRateLimiter: Collision-resistant rate limiter with periodic hash seed rotation
- **Highly Scalable**: Designed for high-throughput concurrent systems
  - Multiple buckets distribute load across different request IDs
  - Low contention design for concurrent access patterns

## Installation

```shell
go get github.com/webriots/rate
```

## Quick Start

### Token Bucket Rate Limiter

```go
package main

import (
	"fmt"
	"time"

	"github.com/webriots/rate"
)

func main() {
	// Create a new token bucket limiter:
	// - 1024 buckets (automatically rounded to nearest power of 2 if
	//   not already a power of 2)
	// - 10 tokens burst capacity
	// - 100 tokens per second refill rate
	limiter, err := rate.NewTokenBucketLimiter(1024, 10, 100, time.Second)
	if err != nil {
		panic(err)
	}

	// Try to take a token for a specific ID
	id := []byte("user-123")

	if limiter.TakeToken(id) {
		fmt.Println("Token acquired, proceeding with request")
		// ... process the request
	} else {
		fmt.Println("Rate limited, try again later")
		// ... return rate limit error
	}

	// Check without consuming
	if limiter.CheckToken(id) {
		fmt.Println("Token would be available")
	}

	// Take multiple tokens at once
	if limiter.TakeTokens(id, 5) {
		fmt.Println("Successfully took 5 tokens")
	}

	// Check if multiple tokens would be available
	if limiter.CheckTokens(id, 3) {
		fmt.Println("3 tokens would be available")
	}
}
```

[Go Playground](https://go.dev/play/p/UMxQDHfqKqD)

### AIMD Rate Limiter

```go
package main

import (
	"fmt"
	"time"

	"github.com/webriots/rate"
)

func main() {
	// Create an AIMD token bucket limiter:
	// - 1024 buckets (automatically rounded to nearest power of 2 if
	//   not already a power of 2)
	// - 10 tokens burst capacity
	// - Min rate: 1 token/s, Max rate: 100 tokens/s, Initial rate: 10
	//   tokens/s
	// - Increase by 1 token/s on success, decrease by factor of 2 on
	//   failure
	limiter, err := rate.NewAIMDTokenBucketLimiter(
		1024,  // numBuckets
		10,    // burstCapacity
		1.0,   // rateMin
		100.0, // rateMax
		10.0,  // rateInit
		1.0,   // rateAdditiveIncrease
		2.0,   // rateMultiplicativeDecrease
		time.Second,
	)
	if err != nil {
		panic(err)
	}

	id := []byte("api-client-123")

	// Try to take a token
	if limiter.TakeToken(id) {
		// Process the request
		success := processRequest()

		if success {
			// If successful, increase the rate
			limiter.IncreaseRate(id)
		} else {
			// If failed (e.g., downstream service is overloaded), decrease
			// the rate to back off
			limiter.DecreaseRate(id)
		}
	} else {
		// We're being rate limited
		fmt.Println("Rate limited, try again later")
	}
}

func processRequest() bool {
	// Your request processing logic
	fmt.Println("Processing request")
	return true
}
```

[Go Playground](https://go.dev/play/p/2AEPxptA2xd)

### Rotating Token Bucket Rate Limiter

```go
package main

import (
	"fmt"
	"time"

	"github.com/webriots/rate"
)

func main() {
	// Create a rotating token bucket limiter:
	// - 1024 buckets (automatically rounded to nearest power of 2 if
	//   not already a power of 2)
	// - 10 tokens burst capacity
	// - 100 tokens per second refill rate
	// - Rotation interval automatically calculated: (10/100)*5 = 0.5
	//   seconds
	limiter, err := rate.NewRotatingTokenBucketLimiter(
		1024,        // numBuckets
		10,          // burstCapacity
		100,         // refillRate
		time.Second, // refillRateUnit
	)
	if err != nil {
		panic(err)
	}

	// Use just like TokenBucketLimiter
	id := []byte("user-456")

	if limiter.TakeToken(id) {
		fmt.Println("Token acquired, proceeding with request")
		// ... process the request
	} else {
		fmt.Println("Rate limited, try again later")
		// ... return rate limit error
	}

	// Check without consuming
	if limiter.CheckToken(id) {
		fmt.Println("Token would be available")
	}

	// The rotating limiter also supports multi-token operations
	if limiter.TakeTokens(id, 3) {
		fmt.Println("Successfully took 3 tokens")
	}
}
```

[Go Playground](https://go.dev/play/p/6UJN8B8F3um)

## Detailed Usage

### Common Interface

All rate limiters in this package implement the `Limiter` interface:

```go
type Limiter interface {
	CheckToken([]byte) bool         // Check if a single token is available
	CheckTokens([]byte, uint8) bool // Check if n tokens are available
	TakeToken([]byte) bool          // Take a single token
	TakeTokens([]byte, uint8) bool  // Take n tokens atomically
}
```

This allows you to write code that works with any rate limiter implementation:

```go
func processRequest(limiter rate.Limiter, clientID []byte, tokensNeeded uint8) error {
	// Check if we have enough tokens without consuming them
	if !limiter.CheckTokens(clientID, tokensNeeded) {
		return fmt.Errorf("rate limited: need %d tokens", tokensNeeded)
	}

	// Actually take the tokens
	if !limiter.TakeTokens(clientID, tokensNeeded) {
		return fmt.Errorf("rate limited: tokens no longer available")
	}

	// Process the request...
	return nil
}
```

### TokenBucketLimiter

The token bucket algorithm is a common rate limiting strategy that allows for controlled bursting. It maintains a "bucket" of tokens that refills at a constant rate, and each request consumes a token.

```go
limiter, err := rate.NewTokenBucketLimiter(
    numBuckets,     // Number of buckets (automatically rounded to nearest power of 2 if not already a power of 2)
    burstCapacity,  // Maximum number of tokens in each bucket
    refillRate,     // Rate at which tokens are refilled
    refillRateUnit, // Time unit for refill rate
)
```

#### Parameters:

- `numBuckets`: Number of token buckets (automatically rounded up to the nearest power of two if not already a power of two)
- `burstCapacity`: Maximum number of tokens that can be consumed at once
- `refillRate`: Rate at which tokens are refilled
- `refillRateUnit`: Time unit for refill rate calculations (e.g., time.Second)

#### Input Validation:

The constructor performs validation on all parameters and returns descriptive errors:

- `refillRate` must be a positive, finite number (not NaN, infinity, zero, or negative)
- `refillRateUnit` must represent a positive duration
- The product of `refillRate` and `refillRateUnit` must not overflow when converted to nanoseconds

#### Methods:

- `CheckToken(id []byte) bool`: Checks if a token would be available without consuming it
- `CheckTokens(id []byte, n uint8) bool`: Checks if n tokens would be available without consuming them
- `TakeToken(id []byte) bool`: Attempts to take a single token, returns true if successful
- `TakeTokens(id []byte, n uint8) bool`: Attempts to take n tokens atomically, returns true if all n tokens were taken

**Multi-token operations:**
The `TakeTokens` and `CheckTokens` methods support atomic multi-token operations. When requesting multiple tokens:
- The operation is all-or-nothing: either all requested tokens are taken/available, or none are
- The maximum number of tokens that can be requested is limited by the burst capacity
- These methods are useful for operations that require multiple "units" of rate limiting

#### Token Bucket Algorithm Explained

The token bucket algorithm uses a simple metaphor of a bucket that holds tokens:

```
┌─────────────────────────────────────┐
│                                     │
│  ┌───┐  ┌───┐  ┌───┐  ┌   ┐  ┌   ┐  │
│  │ T │  │ T │  │ T │  │   │  │   │  │
│  └───┘  └───┘  └───┘  └   ┘  └   ┘  │
│    │      │      │                  │
│    ▼      ▼      ▼                  │
│  avail  avail  avail                │
│                                     │
│  Available: 3 tokens │ Capacity: 5  │
└─────────────────────────────────────┘
      ▲
      │ Refill rate: 100 tokens/second
    ┌───┐  ┌───┐
    │ T │  │ T │  ...  ← New tokens added at constant rate
    └───┘  └───┘
```

**How it works:**

1. **Bucket Initialization**:
   - Each bucket starts with `burstCapacity` tokens (full)
   - A timestamp is recorded when the bucket is created

2. **Token Consumption**:
   - When a request arrives, it attempts to take a token
   - If a token is available, it's removed from the bucket and the request proceeds
   - If no tokens are available, the request is rate-limited

3. **Token Refill**:
   - Tokens are conceptually added to the bucket at a constant rate (`refillRate` per `refillRateUnit`)
   - In practice, the refill is calculated lazily only when tokens are requested
   - The formula is: `refill = (currentTime - lastUpdateTime) * refillRate`
   - Tokens are never added beyond the maximum `burstCapacity`

4. **Burst Handling**:
   - The bucket can temporarily allow higher rates up to `burstCapacity`
   - This accommodates traffic spikes while maintaining a long-term average rate

**Implementation Details:**

- This library uses a sharded approach with multiple buckets to reduce contention
- Each request ID is consistently hashed to the same bucket
- The bucket stores both token count and timestamp in a single uint64 for efficiency

### AIMDTokenBucketLimiter

The AIMD (Additive Increase, Multiplicative Decrease) algorithm provides dynamic rate adjustments inspired by TCP congestion control. It gradually increases token rates during successful operations and quickly reduces rates when encountering failures.

```go
limiter, err := rate.NewAIMDTokenBucketLimiter(
    numBuckets,                 // Number of buckets (automatically rounded to nearest power of 2 if not already a power of 2)
    burstCapacity,              // Maximum tokens per bucket
    rateMin,                    // Minimum token refill rate
    rateMax,                    // Maximum token refill rate
    rateInit,                   // Initial token refill rate
    rateAdditiveIncrease,       // Amount to increase rate on success
    rateMultiplicativeDecrease, // Factor to decrease rate on failure
    rateUnit,                   // Time unit for rate calculations
)
```

#### Parameters:

- `numBuckets`: Number of token buckets (automatically rounded up to the nearest power of two if not already a power of two)
- `burstCapacity`: Maximum number of tokens that can be consumed at once
- `rateMin`: Minimum token refill rate
- `rateMax`: Maximum token refill rate
- `rateInit`: Initial token refill rate
- `rateAdditiveIncrease`: Amount to increase rate by on success
- `rateMultiplicativeDecrease`: Factor to decrease rate by on failure
- `rateUnit`: Time unit for rate calculations (e.g., time.Second)

#### Input Validation:

The constructor performs comprehensive validation on all parameters and returns descriptive errors:

- `rateInit` must be a positive, finite number (not NaN, infinity, zero, or negative)
- `rateUnit` must represent a positive duration
- `rateMin` must be a positive, finite number
- `rateMax` must be a positive, finite number
- `rateMin` must be less than or equal to `rateMax`
- `rateInit` must be between `rateMin` and `rateMax` (inclusive)
- `rateAdditiveIncrease` must be a positive, finite number
- `rateMultiplicativeDecrease` must be a finite number greater than or equal to 1.0
- The product of `rateInit`, `rateMin`, `rateMax`, or `rateAdditiveIncrease` with `rateUnit` must not overflow when converted to nanoseconds

#### Methods:

- `CheckToken(id []byte) bool`: Checks if a token would be available without consuming it
- `CheckTokens(id []byte, n uint8) bool`: Checks if n tokens would be available without consuming them
- `TakeToken(id []byte) bool`: Attempts to take a single token, returns true if successful
- `TakeTokens(id []byte, n uint8) bool`: Attempts to take n tokens atomically, returns true if all n tokens were taken
- `IncreaseRate(id []byte)`: Additively increases the rate for the given ID (on success)
- `DecreaseRate(id []byte)`: Multiplicatively decreases the rate for the given ID (on failure)

#### AIMD Algorithm Explained

The AIMD algorithm dynamically adjusts token refill rates based on success or failure feedback, similar to how TCP adjusts its congestion window:

```
   Rate
   ▲
   │
   │
   │
rateMax ─────────────────────────────────────────
   │                       ╱       │
   │                     ╱         │
   │                   ╱           │           ╱
   │                 ╱             │         ╱
   │               ╱               │       ╱
   │             ╱                 │     ╱
   │           ╱                   │   ╱
   │         ╱                     │ ╱
   │       ╱                       ▼ Multiplicative
   │     ╱                           Decrease
   │   ╱                             (rate / 2)
   │ ╱
rateInit
   │ ↑
   │ Additive
   │ Increase
   │ (+1)
rateMin ─────────────────────────────────────────
   │
   └─────────────────────────────────────────────► Time
              Success           Failure
```

**How it works:**

1. **Initialization**:
   - Each bucket starts with a rate of `rateInit` tokens per time unit
   - Token buckets are created with `burstCapacity` tokens

2. **Adaptive Rate Adjustment**:
   - When operations succeed, the rate increases linearly by `rateAdditiveIncrease`
     - `newRate = currentRate + rateAdditiveIncrease`
   - When operations fail, the rate decreases multiplicatively by `rateMultiplicativeDecrease`
     - `newRate = rateMin + (currentRate - rateMin) / rateMultiplicativeDecrease`
   - Rates are bounded between `rateMin` (lower bound) and `rateMax` (upper bound)

3. **Practical Applications**:
   - Gracefully adapts to changing capacity of backend systems
   - Quickly backs off when systems are overloaded (multiplicative decrease)
   - Slowly probes for increased capacity when systems are healthy (additive increase)
   - Maintains stability even under varying load conditions

**Implementation Details:**

- Each ID has its own dedicated rate that adjusts independently
- The actual token bucket mechanics remain the same as TokenBucketLimiter
- Rate changes are applied atomically using lock-free operations
- Rate information is stored alongside token information in the bucket

### RotatingTokenBucketRateLimiter

The Rotating Token Bucket Rate Limiter addresses a fundamental limitation of hash-based rate limiters: hash collisions between different IDs can cause unfair rate limiting. This limiter maintains two TokenBucketLimiters with different hash seeds and automatically rotates between them to minimize collision impact while ensuring correctness.

```go
limiter, err := rate.NewRotatingTokenBucketLimiter(
    numBuckets,     // Number of buckets (automatically rounded to nearest power of 2 if not already a power of 2)
    burstCapacity,  // Maximum tokens per bucket
    refillRate,     // Rate at which tokens are refilled
    refillRateUnit, // Time unit for refill rate
)
```

#### Parameters:

- `numBuckets`: Number of token buckets per limiter (automatically rounded up to the nearest power of two if not already a power of two)
- `burstCapacity`: Maximum number of tokens that can be consumed at once
- `refillRate`: Rate at which tokens are refilled
- `refillRateUnit`: Time unit for refill rate calculations (e.g., time.Second)

#### Automatic Rotation Calculation:

The rotation interval is automatically calculated to ensure 99.99% statistical convergence of all token buckets to steady state before rotation occurs. This eliminates state inconsistency issues and guarantees correctness:

```
rotationInterval = (burstCapacity / refillRate * refillRateUnit) * 5.0
```

For example:
- `burstCapacity=100, refillRate=250/second` → rotation every 2 seconds
- `burstCapacity=10, refillRate=1/second` → rotation every 50 seconds

#### Input Validation:

The constructor performs validation on all parameters and returns descriptive errors:

- `refillRate` must be a positive, finite number (not NaN, infinity, zero, or negative)
- `refillRateUnit` must represent a positive duration
- The product of `refillRate` and `refillRateUnit` must not overflow when converted to nanoseconds

#### Methods:

- `CheckToken(id []byte) bool`: Checks if a token would be available without consuming it
- `CheckTokens(id []byte, n uint8) bool`: Checks if n tokens would be available without consuming them
- `TakeToken(id []byte) bool`: Attempts to take a single token, returns true if successful
- `TakeTokens(id []byte, n uint8) bool`: Attempts to take n tokens atomically, returns true if all n tokens were taken
- `RotationInterval() time.Duration`: Returns the automatically calculated rotation interval

#### Collision-Resistant Algorithm Explained

The rotating limiter solves the hash collision problem by maintaining two token bucket limiters and periodically rotating their roles:

```
Time: 0s                    Time: 30s (rotation)           Time: 60s (rotation)
┌─────────────────────────┐ ┌─────────────────────────┐ ┌─────────────────────────┐
│ Limiter A (checked)     │ │ Limiter B (checked)     │ │ Limiter C (checked)     │
│ - seed: 0x1234          │→│ - seed: 0x5678          │→│ - seed: 0x9ABC          │
│ - decision: ✓ enforced  │ │ - decision: ✓ enforced  │ │ - decision: ✓ enforced  │
│                         │ │                         │ │                         │
│ Limiter B (ignored)     │ │ Limiter C (ignored)     │ │ Limiter D (ignored)     │
│ - seed: 0x5678          │ │ - seed: 0x9ABC          │ │ - seed: 0xDEF0          │
│ - decision: ✗ discarded │ │ - decision: ✗ discarded │ │ - decision: ✗ discarded │
└─────────────────────────┘ └─────────────────────────┘ └─────────────────────────┘

Hash Collision Scenario:
┌─────────────────────────────────────────────────────────────────────────────────┐
│ ID "user-123" and "user-456" both hash to bucket 42 in Limiter A (collision!)   │
│ They compete for the same tokens → unfair rate limiting                         │
│                                                                                 │
│ After rotation (30s later):                                                     │
│ - Limiter B becomes "checked" with seed 0x5678                                  │
│ - Very likely: "user-123" → bucket 15, "user-456" → bucket 73 (no collision!)   │
│ - Collision resolved! Each ID now has independent rate limiting                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

**How it works:**

1. **Dual Limiter Architecture**:
   - Maintains two identical TokenBucketLimiters with different hash seeds
   - "Checked" limiter: its result determines rate limiting decisions
   - "Ignored" limiter: maintains state but its result is discarded

2. **Token Consumption Strategy**:
   - Each `TakeToken()` call consumes tokens from **both** limiters
   - Only the "checked" limiter's success/failure determines the result
   - **Trade-off**: 2x token consumption rate for collision resistance

3. **Periodic Rotation**:
   - Every `rotationRate` duration, the limiters rotate roles:
     - Old "ignored" becomes new "checked" (with established state)
     - Copy of new "checked" becomes new "ignored" (with fresh hash seed)
   - Hash collisions are broken by the new hash seed

4. **Collision Impact Mitigation**:
   - Hash collisions only persist for maximum one `rotationRate` period
   - Different hash seeds make collisions probabilistically unlikely after rotation
   - Provides better long-term fairness compared to static hash seeds

**Benefits:**
- ✅ **Transient Collisions**: Hash collisions are temporary (max duration = rotation period)
- ✅ **Better Fairness**: Different IDs are less likely to be permanently penalized
- ✅ **Automatic Recovery**: System self-heals from collision scenarios
- ✅ **Drop-in Replacement**: Same interface as TokenBucketLimiter

**Trade-offs:**
- ❌ **Higher Resource Usage**: 2x token consumption and memory usage
- ❌ **Slight Performance Overhead**: ~2x slower than single TokenBucketLimiter
- ❌ **Configuration Complexity**: Additional `rotationRate` parameter to tune

#### Implementation Details:

- Uses atomic operations for thread-safe rotation
- Both limiters are always synchronized with the same timestamp
- Rotation is triggered lazily during token operations
- Memory usage is approximately 2x that of TokenBucketLimiter
- Hash seed generation uses Go's `hash/maphash.MakeSeed()` for quality randomness

#### When to Use Rotating Token Bucket Limiter:

**Use when:**
- Hash collision fairness is critical for your application
- You have many distinct IDs with similar access patterns
- Temporary rate limit inaccuracy is acceptable for better long-term fairness
- You can afford 2x resource usage for improved collision resistance

**Don't use when:**
- Resource efficiency is more important than collision fairness
- You have very few distinct IDs (low collision probability)
- Your rate limits are very loose (collisions don't significantly impact users)
- Real-time precision is more important than long-term fairness

## Performance

The library is optimized for high performance and efficiency:

- Core operations complete in 1-15ns with zero allocations
- Efficient memory usage through packed representations
- Minimal contention in multi-threaded environments

### Benchmarks

Run the benchmarks on your machine with:

```shell
# Run all benchmarks with memory allocation stats
go test -bench=. -benchmem

# Run specific benchmark
go test -bench=BenchmarkTokenBucketCheck -benchmem
```

Here are the results from an Apple M1 Pro:

```
# Core operations
BenchmarkTokenBucketCheck-10                    203429409                5.919 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketTakeToken-10                165718064                7.240 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketParallel-10                 841380494                1.419 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketContention-10               655109293                1.827 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketWithRefill-10               165840525                7.110 ns/op           0 B/op          0 allocs/op

# Bucket creation (different sizes)
BenchmarkTokenBucketCreateSmall-10               16929116               71.00 ns/op          192 B/op          2 allocs/op
BenchmarkTokenBucketCreateMedium-10                664831             1820 ns/op            8256 B/op          2 allocs/op
BenchmarkTokenBucketCreateLarge-10                  45537            25471 ns/op          131137 B/op          2 allocs/op

# Internals
BenchmarkTokenBucketPacked-10                   1000000000               0.3103 ns/op          0 B/op          0 allocs/op
BenchmarkTokenBucketUnpack-10                   1000000000               0.3103 ns/op          0 B/op          0 allocs/op
BenchmarkTokenBucketIndex-10                     265611426               4.510 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketRefill-10                    591397021               2.023 ns/op           0 B/op          0 allocs/op

# Real-world scenarios
BenchmarkTokenBucketManyIDs-10                   139485549               8.590 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketDynamicID-10                 93835521               12.87 ns/op            0 B/op          0 allocs/op
BenchmarkTokenBucketRealWorldRequestRate-10      853565757               1.401 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketHighContention-10            579507058               2.068 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketWithSystemClock-10           459682273               2.605 ns/op           0 B/op          0 allocs/op

# Rotating limiter operations
BenchmarkRotatingTokenBucketLimiterTakeToken-10   80090768              14.93 ns/op            0 B/op          0 allocs/op
BenchmarkRotatingTokenBucketLimiterCheck-10       91934064              13.16 ns/op            0 B/op          0 allocs/op
BenchmarkRotatingTokenBucketLimiterConcurrent-10 687345256               1.744 ns/op           0 B/op          0 allocs/op
```

## Advanced Usage

### Choosing the Optimal Number of Buckets

The `numBuckets` parameter is automatically rounded up to the nearest power of two if not already a power of two (e.g., 256, 1024, 4096) for efficient hashing and is a critical factor in the limiter's performance and fairness.

#### How Bucket Selection Works

When you call `TakeToken(id)` or similar methods, the library:

1. Hashes the ID using Go's built-in hash/maphash algorithm (non-cryptographic, high performance)
2. Uses the hash value & (numBuckets-1) to determine the bucket index
3. Takes/checks a token from that specific bucket

```go
// Simplified version of the internal hashing mechanism
func index(id []byte) int {
    return int(maphash.Bytes(seed, id) & (numBuckets - 1))
}
```

#### Understanding Collision Probability

A collision occurs when two different IDs map to the same bucket. When this happens:
- Those IDs share the same rate limit, which may be undesirable
- Higher contention can occur on that bucket when accessed concurrently

The probability of collision depends on:
1. Number of buckets (`numBuckets`)
2. Number of distinct IDs being rate limited

The birthday paradox formula gives us the probability of *at least one* collision:

| Number of Distinct IDs | Buckets=1024 | Buckets=4096 | Buckets=16384 | Buckets=65536 |
|------------------------|--------------|--------------|---------------|---------------|
| 10                     | 4.31%        | 1.09%        | 0.27%         | 0.07%         |
| 100                    | 99.33%       | 70.43%       | 26.12%        | 7.28%         |
| 1,000                  | 100.00%      | 100.00%      | 100.00%       | 99.95%        |
| 10,000                 | 100.00%      | 100.00%      | 100.00%       | 100.00%       |
| 100,000                | 100.00%      | 100.00%      | 100.00%       | 100.00%       |

However, the probability of collision doesn't tell the whole story. What matters more is the **percentage of IDs involved in collisions** and the **expected number of collisions**:

| Configuration | Load Factor | Expected Collisions | % of IDs in Collisions |
|---------------|------------|---------------------|------------------------|
| 100 IDs in 65,536 buckets | 0.0015 | ~0.08 | ~0.15% |
| 1,000 IDs in 65,536 buckets | 0.0153 | ~7.6 | ~1.5% |
| 10,000 IDs in 65,536 buckets | 0.1526 | ~763 | ~7.3% |

#### Guidelines for Choosing `numBuckets`

It's important to understand that even though the probability of having *at least one* collision approaches 100% quickly as the number of IDs increases, the **percentage of IDs affected by collisions** grows much more slowly.

Based on the percentage of IDs involved in collisions:

- For **small** systems (≤100 IDs):
  - 4,096 buckets: ~2.4% of IDs will be involved in collisions
  - 16,384 buckets: ~0.6% of IDs will be involved in collisions

- For **medium** systems (101-1,000 IDs):
  - 16,384 buckets: ~6% of IDs will be involved in collisions
  - 65,536 buckets: ~1.5% of IDs will be involved in collisions

- For **large** systems (1,001-10,000 IDs):
  - 65,536 buckets: ~7.3% of IDs will be involved in collisions
  - 262,144 buckets (2^18): ~1.9% of IDs will be involved in collisions

- For **very large** systems (>10,000 IDs):
  - 1,048,576 buckets (2^20): ~4.7% of IDs among 100,000 will be involved in collisions

**General Rule of Thumb**: To keep collision impact below 5% of IDs, maintain a load factor (IDs/buckets) below 0.1.

**Memory Impact**:
- Each bucket uses 8 bytes (uint64)
- 65,536 buckets = 512 KB of memory
- 1,048,576 buckets = 8 MB of memory

For systems with many distinct IDs where even low collision percentages are unacceptable, consider:
- Sharding rate limiters by logical partitions (e.g., user type, region)
- Using multiple rate limiters with different hashing algorithms
- Implementing a consistent hashing approach

> **Note**: While having some collisions is usually acceptable for rate limiting (it just means some IDs might share limits), the key is to keep the percentage of affected IDs low enough for your use case.

Memory usage scales linearly with `numBuckets`, so there's a trade-off between collision probability and memory consumption.

```go
// Example for a system with ~5,000 distinct IDs
limiter, err := rate.NewTokenBucketLimiter(
    65536,    // numBuckets (will remain 2^16 since it's already a power of 2) to maintain <10% collision probability
    10,       // burstCapacity
    100,      // refillRate
    time.Second,
)
```

> **Note**: If you observe uneven rate limiting across different IDs, consider increasing the number of buckets to reduce collision probability.

### Custom Rate Limiting Scenarios

#### High-Volume API Rate Limiting

```go
// API rate limiting with 100 requests per second, 10 burst capacity
limiter, _ := rate.NewTokenBucketLimiter(1024, 10, 100, time.Second)

// Take a token for a specific API key
apiKey := []byte("customer-api-key")
if limiter.TakeToken(apiKey) {
    // Process API request
} else {
    // Return 429 Too Many Requests
}
```

#### Adaptive Backend Protection with AIMD

```go
// Create AIMD limiter for dynamic backend protection
limiter, _ := rate.NewAIMDTokenBucketLimiter(
    4096,   // numBuckets
    20,     // burstCapacity
    1.0,    // rateMin
    1000.0, // rateMax
    100.0,  // rateInit
    10.0,   // rateAdditiveIncrease
    2.0,    // rateMultiplicativeDecrease
    time.Second,
)

func handleRequest(backendID []byte) {
    if limiter.TakeToken(backendID) {
        err := callBackendService()
        if err == nil {
            // Backend is healthy, increase rate
            limiter.IncreaseRate(backendID)
        } else if isOverloadError(err) {
            // Backend is overloaded, decrease rate
            limiter.DecreaseRate(backendID)
        }
    } else {
        // Circuit breaking, return error immediately
    }
}
```

## Design Philosophy

The library is designed with these principles in mind:

1. **Performance First**: Every operation is optimized for minimal CPU and memory usage
2. **Thread Safety**: All operations are thread-safe for concurrent environments without lock contention
3. **Memory Efficiency**: Uses packed representation and custom implementations for efficient storage
4. **Simplicity**: Provides simple interfaces for common rate limiting patterns
5. **Flexibility**: Multiple strategies to handle different rate limiting requirements

## Thread-Safety Guarantees

The `rate` package is designed for high-concurrency environments with the following guarantees:

### Atomic Operations

- All token bucket operations use lock-free atomic operations
- Token count updates and timestamp operations use atomic Compare-And-Swap (CAS)
- No mutexes or traditional locks are used, eliminating lock contention
- Bucket selection uses thread-safe hash calculations to map IDs to buckets

### Concurrent Access Patterns

- **Read Operations**: Multiple goroutines can safely check token availability concurrently
- **Write Operations**: Multiple goroutines can safely take tokens and update rates concurrently
- **Creation**: Limiter creation is thread-safe and can be performed from any goroutine
- **Refill**: Token refill happens automatically during token operations without requiring explicit locks

### Memory Ordering

- All atomic operations enforce appropriate memory ordering guarantees
- Reads and writes are properly synchronized across multiple cores/CPUs
- Operations follow Go's memory model for concurrent access

### Contention Handling

- Multiple buckets distribute load to minimize atomic operation contention
- Rate updates for one ID don't block operations on other IDs
- The implementation uses a sharded approach where each ID maps to a specific bucket

### Limitations

- The library doesn't guarantee perfect fairness across all IDs
- When multiple IDs hash to the same bucket (collision), they share the same rate limit
- Very high contention on a single ID might experience CAS retry loops

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the [MIT License](LICENSE).
