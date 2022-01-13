package xdag

import (
	"fmt"
	"time"
)

const (
	// MainEra is XDAG era start time
	MainEra uint64 = 0x16940000000
)

// MainTime returns a time period index, where a period is 64 seconds long
func MainTime(t uint64) uint64 {
	return t >> 16
}

//  gets XDAG timestamp of current time
func GetXTimestamp() uint64 {
	t := time.Now().UTC().UnixNano()
	sec := t / 1e9
	usec := (t - sec*1e9) / 1e3
	xmsec := (usec << 10) / 1e6
	return uint64(sec)<<10 | uint64(xmsec)
}

// CurrentMainTime returns a time period index of current time
func CurrentMainTime() uint64 {
	return MainTime(GetXTimestamp())
}

// StartMainTime returns the time period index corresponding to the start of the network
func StartMainTime() uint64 {
	return MainTime(MainEra)
}

// Xtime2str converts xtime_t to string representation
func Xtime2str(t uint64) string {
	msec := ((t & 0x3ff) * 1e3) >> 10
	tm := time.Unix(int64(t>>10), 0)
	return tm.Format("2006-01-02 15:04:05") + fmt.Sprintf(".%03d", msec)
}
