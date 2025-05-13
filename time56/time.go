// Package time56 provides a specialized 56-bit timestamp
// implementation for space-efficient, high-resolution timing within a
// limited time range. It allows for compact storage of timestamps
// alongside other data by using only 56 bits, leaving 8 bits
// available for additional information.
package time56

// mask56 is a constant for the 56-bit mask (0x00FFFFFFFFFFFFFF) Used
// to ensure values stay within 56 bits by masking off the high 8
// bits.
const mask56 = uint64(0x00FFFFFFFFFFFFFF)

// Time represents a 56-bit timestamp (nanoseconds, truncated to 56
// bits).
//
// This type is designed for space-efficient, high-resolution timing
// where:
//
//   - All timestamps are generated and compared within ~1.14 years
//     (~36 days short of 1.25 years)
//   - Timestamps only need to be meaningful relative to each other,
//     not absolutely
//   - The 56-bit limitation allows the high 8 bits to be used for
//     other data
//
// The 56-bit limit gives a range of 2^56 nanoseconds ≈ 2.27 years
// before overflow. Wraparound is handled gracefully for time
// differences.
type Time uint64

// Pack combines a uint8 value and this Time56 timestamp into a single
// uint64. The result has the format: [8 bits of val][56 bits of
// timestamp] This allows efficiently storing a small value alongside
// the timestamp.
func (t Time) Pack(val uint8) uint64 {
	return (uint64(val) << 56) | (uint64(t) & mask56)
}

// Since returns the elapsed nanoseconds between this time and the
// given time. The result is an int64 that can be positive (this time
// is later) or negative (this time is earlier).
//
// This method correctly handles wraparound cases where the timestamps
// have overflowed the 56-bit limit.
func (t Time) Since(then Time) int64 {
	diff := int64((uint64(t) - uint64(then)) & mask56)
	return diff - ((diff & (1 << 55)) << 1)
}

// Sub returns a new Time that is n nanoseconds earlier than this
// time. This method handles wraparound by properly masking the result
// to 56 bits.
func (t Time) Sub(n int64) Time {
	return Time((uint64(t) - uint64(n)) & mask56)
}

// Uint64 returns the 56-bit timestamp as a uint64 value. The returned
// value will always have its high 8 bits set to zero.
func (t Time) Uint64() uint64 {
	return uint64(t) & mask56
}

// New creates a new Time from a uint64 value by truncating it to 56
// bits. Any bits above the 56th bit will be discarded.
func New(t uint64) Time {
	return Time(t & mask56)
}

// Unix creates a new Time from a Unix nanosecond timestamp by
// truncating it to 56 bits. This is useful for converting standard
// time.Time Unix timestamps to Time56 format.
func Unix(ns int64) Time {
	return Time(uint64(ns) & mask56)
}

// Unpack extracts the 8-bit value and 56-bit timestamp from a packed
// uint64. This is the inverse operation of Pack. Returns the high 8
// bits as a uint8 and the low 56 bits as a Time.
func Unpack(packed uint64) (uint8, Time) {
	return uint8(packed >> 56), Time(packed & mask56)
}
