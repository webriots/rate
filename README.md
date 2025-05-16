# Rate

[![Go Reference](https://pkg.go.dev/badge/github.com/webriots/rate.svg)](https://pkg.go.dev/github.com/webriots/rate)
[![Go Report Card](https://goreportcard.com/badge/github.com/webriots/rate)](https://goreportcard.com/report/github.com/webriots/rate)
[![Coverage Status](https://coveralls.io/repos/github/webriots/rate/badge.svg?branch=main)](https://coveralls.io/github/webriots/rate?branch=main)

A high-performance rate limiting library for Go applications with multiple rate limiting strategies.

## Features

- **Ultra-Fast**: Core operations complete in 1-15ns with zero allocations
- **Thread-Safe**: All operations are designed to be thread-safe using optimized atomic operations
- **Zero External Dependencies**: Relies solely on the Go standard library
- **Memory Efficient**: Uses compact data structures and optimized storage
  - Packed representations for token buckets
  - Custom 56-bit timestamp implementation
  - Efficient atomic slice implementations
- **Multiple Rate Limiting Strategies**:
  - TokenBucketLimiter: Classic token bucket algorithm with multiple buckets
  - AIMDTokenBucketLimiter: Additive-Increase/Multiplicative-Decrease algorithm inspired by TCP congestion control
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
    // - 1024 buckets (automatically rounded to nearest power of 2 if not already a power of 2)
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
    if limiter.Check(id) {
        fmt.Println("Token would be available")
    }
}
```

[Go Playground](https://go.dev/play/p/70DxTct1Dl3)

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
    // - 1024 buckets (automatically rounded to nearest power of 2 if not already a power of 2)
    // - 10 tokens burst capacity
    // - Min rate: 1 token/s, Max rate: 100 tokens/s, Initial rate: 10 tokens/s
    // - Increase by 1 token/s on success, decrease by factor of 2 on failure
    limiter, err := rate.NewAIMDTokenBucketLimiter(
        1024,      // numBuckets
        10,        // burstCapacity
        1.0,       // rateMin
        100.0,     // rateMax
        10.0,      // rateInit
        1.0,       // rateAdditiveIncrease
        2.0,       // rateMultiplicativeDecrease
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
            // If failed (e.g., downstream service is overloaded),
            // decrease the rate to back off
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

[Go Playground](https://go.dev/play/p/S9Xjz5Dojbv)

## Detailed Usage

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

### AIMDTokenBucketLimiter

The AIMD (Additive Increase, Multiplicative Decrease) algorithm provides dynamic rate adjustments inspired by TCP congestion control. It gradually increases token rates during successful operations and quickly reduces rates when encountering failures.

```go
limiter, err := rate.NewAIMDTokenBucketLimiter(
    numBuckets,                // Number of buckets (automatically rounded to nearest power of 2 if not already a power of 2)
    burstCapacity,             // Maximum tokens per bucket
    rateMin,                   // Minimum token refill rate
    rateMax,                   // Maximum token refill rate
    rateInit,                  // Initial token refill rate
    rateAdditiveIncrease,      // Amount to increase rate on success
    rateMultiplicativeDecrease, // Factor to decrease rate on failure
    rateUnit,                  // Time unit for rate calculations
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
BenchmarkTokenBucketCheck-10                    164063450                7.279 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketTakeToken-10                150341438                7.981 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketParallel-10                 1000000000               1.115 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketContention-10               1000000000               1.001 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketWithRefill-10               138591127                8.699 ns/op           0 B/op          0 allocs/op

# Bucket creation (different sizes)
BenchmarkTokenBucketCreateSmall-10              16974966                71.35 ns/op          192 B/op          2 allocs/op
BenchmarkTokenBucketCreateMedium-10               696861              1781 ns/op            8256 B/op          2 allocs/op
BenchmarkTokenBucketCreateLarge-10                 50068             24202 ns/op          131137 B/op          2 allocs/op

# Internals
BenchmarkTokenBucketPacked-10                   1000000000               0.3104 ns/op          0 B/op          0 allocs/op
BenchmarkTokenBucketUnpack-10                   1000000000               0.3103 ns/op          0 B/op          0 allocs/op
BenchmarkTokenBucketIndex-10                    265611426                4.510 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketRefill-10                   591397021                2.023 ns/op           0 B/op          0 allocs/op

# Real-world scenarios
BenchmarkTokenBucketManyIDs-10                  131259206                9.211 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketDynamicID-10                40364173                27.98 ns/op            7 B/op          0 allocs/op
BenchmarkTokenBucketRealWorldRequestRate-10     1000000000               1.148 ns/op           0 B/op          0 allocs/op
BenchmarkTokenBucketHighContention-10           1000000000               1.078 ns/op           0 B/op          0 allocs/op
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
            limiter.IncreaseRateForID(backendID)
        } else if isOverloadError(err) {
            // Backend is overloaded, decrease rate
            limiter.DecreaseRateForID(backendID)
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

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the [MIT License](LICENSE).
