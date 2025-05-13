package rate

import (
	"sync/atomic"
	"time"
)

// clock overrides nowfn and allows manual time advancement
var clock atomic.Int64

func init() {
	clock.Store(time.Now().UnixNano())
	nowfn = func() time.Time { return time.Unix(0, clock.Load()) }
}

// tick moves simulated time forward by the given duration
func tick(d time.Duration) {
	clock.Add(d.Nanoseconds())
}
