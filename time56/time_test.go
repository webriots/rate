package time56

import (
	"testing"
	"time"
)

func TestUnixNano(t *testing.T) {
	now := Unix(time.Now().UnixNano())
	if uint64(now)&^mask56 != 0 {
		t.Errorf("UnixNano has bits set outside 56 bits: got %x", uint64(now))
	}
}

func TestSub(t *testing.T) {
	base := Time(1000)
	sub := base.Sub(100)
	want := uint64(900)
	if uint64(sub) != want {
		t.Errorf("Sub: got %d, want %d", sub, want)
	}
}

func TestSubUnixNanoaround(t *testing.T) {
	base := Time(0)
	sub := base.Sub(1)
	want := mask56
	if uint64(sub) != want {
		t.Errorf("Sub wraparound: got %x, want %x", uint64(sub), want)
	}
}

func TestSinceBasic(t *testing.T) {
	now := Time(1000)
	then := Time(500)
	elapsed := now.Since(then)
	if elapsed != 500 {
		t.Errorf("Since: got %d, want 500", elapsed)
	}
}

func TestSinceUnixNanoaroundPositive(t *testing.T) {
	now := Time(50)
	then := Time(mask56 - 50)
	elapsed := now.Since(then)
	want := int64(101)
	if elapsed != want {
		t.Errorf("Since wraparound (positive): got %d, want %d", elapsed, want)
	}
}

func TestSinceNegative(t *testing.T) {
	now := Time(100)
	then := Time(200)
	elapsed := now.Since(then)
	want := int64(-100)
	if elapsed != want {
		t.Errorf("Since negative: got %d, want %d", elapsed, want)
	}
}

func TestSinceMaxPositive(t *testing.T) {
	now := Time((1 << 55) - 1)
	then := Time(0)
	elapsed := now.Since(then)
	want := int64((1 << 55) - 1)
	if elapsed != want {
		t.Errorf("Since max positive: got %d, want %d", elapsed, want)
	}
}

func TestSinceUnixNanoToNegative(t *testing.T) {
	now := Time(1 << 55)
	then := Time(0)
	elapsed := now.Since(then)
	want := int64(-1 << 55)
	if elapsed != want {
		t.Errorf("Since at sign bit: got %d, want %d", elapsed, want)
	}
}

func TestPackUnpack(t *testing.T) {
	// Normal value test
	values := []struct {
		val uint8
		ts  Time
	}{
		{0, 0},
		{1, 1},
		{255, Time(mask56)},
		{128, 1234567890},
		{42, Time((1 << 55) - 1)}, // Max positive ts value
	}

	for _, test := range values {
		packed := test.ts.Pack(test.val)
		gotVal, gotTs := Unpack(packed)
		if gotVal != test.val || gotTs != test.ts {
			t.Errorf("Pack/Unpack failed: val=%d ts=%d -> packed=0x%x -> val=%d ts=%d",
				test.val, test.ts, packed, gotVal, gotTs)
		}
	}
}

func TestPackUnpack_TimestampMasking(t *testing.T) {
	// High bits in Time must be masked out
	val := uint8(0xAA)
	// All bits set: this would "overflow" the timestamp field if not masked
	ts := Time(0xFFFFFFFFFFFFFFFF)
	packed := ts.Pack(val)
	gotVal, gotTs := Unpack(packed)

	if gotVal != val {
		t.Errorf("Expected val %d, got %d", val, gotVal)
	}
	if uint64(gotTs)&^mask56 != 0 {
		t.Errorf("Timestamp not properly masked, got 0x%x", gotTs)
	}
	if gotTs != Time(mask56) {
		t.Errorf("Expected ts 0x%x, got 0x%x", mask56, gotTs)
	}
}

func TestPackUnpack_AllValValues(t *testing.T) {
	const sampleTs = 0x123456789ABCD
	for val := 0; val <= 0xFF; val++ {
		packed := Time(sampleTs).Pack(uint8(val))
		gotVal, gotTs := Unpack(packed)
		if gotVal != uint8(val) {
			t.Fatalf("For val %d: Pack/Unpack returned val %d", val, gotVal)
		}
		if gotTs != Time(sampleTs) {
			t.Fatalf("For val %d: Pack/Unpack returned ts 0x%x", val, gotTs)
		}
	}
}

// TestUint64 tests the Uint64 method ensuring it returns the correct 56-bit value.
func TestUint64(t *testing.T) {
	testCases := []struct {
		name     string
		input    Time
		expected uint64
	}{
		{
			name:     "Zero value",
			input:    Time(0),
			expected: 0,
		},
		{
			name:     "Small value",
			input:    Time(123),
			expected: 123,
		},
		{
			name:     "Medium value",
			input:    Time(0xABCDEF),
			expected: 0xABCDEF,
		},
		{
			name:     "Large value within range",
			input:    Time(mask56),
			expected: mask56,
		},
		{
			name:     "Value with high bits (should be masked)",
			input:    Time(0xFFFFFFFFFFFFFFFF),
			expected: mask56,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.input.Uint64()
			if result != tc.expected {
				t.Errorf("Expected: 0x%x, Got: 0x%x", tc.expected, result)
			}

			// Ensure high bits are always zero
			if result&^mask56 != 0 {
				t.Errorf("Result has high bits set: 0x%x", result)
			}
		})
	}
}

// TestNew tests the New function for creating Time values from uint64.
func TestNew(t *testing.T) {
	testCases := []struct {
		name     string
		input    uint64
		expected Time
	}{
		{
			name:     "Zero value",
			input:    0,
			expected: Time(0),
		},
		{
			name:     "Small value",
			input:    123,
			expected: Time(123),
		},
		{
			name:     "Medium value",
			input:    0xABCDEF,
			expected: Time(0xABCDEF),
		},
		{
			name:     "Value at mask edge",
			input:    mask56,
			expected: Time(mask56),
		},
		{
			name:     "Value exceeding mask (should be truncated)",
			input:    0xF000000000000000 | 0xABCDEF,
			expected: Time(0xABCDEF),
		},
		{
			name:     "Maximum uint64 (should be truncated to mask56)",
			input:    0xFFFFFFFFFFFFFFFF,
			expected: Time(mask56),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := New(tc.input)
			if result != tc.expected {
				t.Errorf("Expected: 0x%x, Got: 0x%x", tc.expected, result)
			}

			// Double-check Uint64 method returns consistent value
			if result.Uint64() != uint64(tc.expected) {
				t.Errorf("Inconsistency between New and Uint64: 0x%x vs 0x%x",
					result.Uint64(), uint64(tc.expected))
			}

			// Ensure no bits above the mask are set
			if uint64(result)&^mask56 != 0 {
				t.Errorf("Result has bits set outside mask: 0x%x", uint64(result))
			}
		})
	}
}

// TestNewAndUnixConsistency tests that New and Unix create consistent results.
func TestNewAndUnixConsistency(t *testing.T) {
	now := time.Now().UnixNano()

	// Create Time using both New and Unix
	t1 := New(uint64(now))
	t2 := Unix(now)

	if t1 != t2 {
		t.Errorf("New and Unix produced different results: 0x%x vs 0x%x", t1, t2)
	}

	// Both should be properly masked
	if uint64(t1)&^mask56 != 0 || uint64(t2)&^mask56 != 0 {
		t.Errorf("Results not properly masked: t1=0x%x, t2=0x%x", t1, t2)
	}
}

func BenchmarkSince(b *testing.B) {
	now := Time(1000)
	then := Time(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = now.Since(then)
	}
}

func BenchmarkSinceNegative(b *testing.B) {
	now := Time(100)
	then := Time(200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = now.Since(then)
	}
}

func BenchmarkPack(b *testing.B) {
	t := Time(0x123456789ABCD)
	val := uint8(42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = t.Pack(val)
	}
}

func BenchmarkUnpack(b *testing.B) {
	packed := uint64(0xAA123456789ABCD)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Unpack(packed)
	}
}
