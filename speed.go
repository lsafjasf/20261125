package mediaclock

import (
	"errors"
	"math/big"
	"time"
)

// Speed is an exact playback rate: mediaDuration / deviceDuration = Num / Den.
type Speed struct {
	Num int64
	Den int64
}

var (
	SpeedNormal = Speed{Num: 1, Den: 1}
	SpeedHalf   = Speed{Num: 1, Den: 2}
)

func NewSpeed(num, den int64) (Speed, error) {
	if num <= 0 || den <= 0 {
		return Speed{}, errors.New("mediaclock: speed must be positive")
	}
	g := gcd64(num, den)
	return Speed{Num: num / g, Den: den / g}, nil
}

func MustSpeed(num, den int64) Speed {
	speed, err := NewSpeed(num, den)
	if err != nil {
		panic(err)
	}
	return speed
}

// SpeedFromFloat converts common UI rates such as 1.25 without cumulative
// floating-point error.
func SpeedFromFloat(rate float64) (Speed, error) {
	if rate <= 0 {
		return Speed{}, errors.New("mediaclock: invalid playback speed")
	}
	r := new(big.Rat).SetFloat64(rate)
	if r == nil {
		return Speed{}, errors.New("mediaclock: invalid playback speed")
	}
	num := r.Num()
	den := r.Denom()
	if !num.IsInt64() || !den.IsInt64() {
		return Speed{}, errors.New("mediaclock: playback speed is out of range")
	}
	return NewSpeed(num.Int64(), den.Int64())
}

func (s Speed) IsZero() bool {
	return s.Num == 0 || s.Den == 0
}

func (s Speed) Float64() float64 {
	if s.IsZero() {
		return 0
	}
	return float64(s.Num) / float64(s.Den)
}

// MediaDuration maps elapsed device time to source/media duration.
func (s Speed) MediaDuration(device time.Duration) time.Duration {
	if s.IsZero() || s == SpeedNormal {
		return device
	}
	value := scaleInt64(device.Nanoseconds(), s.Num, s.Den)
	return time.Duration(clampInt64(value))
}

// DeviceDuration is the inverse mapping used by deterministic simulations.
func (s Speed) DeviceDuration(media time.Duration) time.Duration {
	if s.IsZero() || s == SpeedNormal {
		return media
	}
	value := scaleInt64(media.Nanoseconds(), s.Den, s.Num)
	return time.Duration(clampInt64(value))
}

func scaleInt64(value, num, den int64) int64 {
	if den == 0 {
		return 0
	}
	n := big.NewInt(value)
	n.Mul(n, big.NewInt(num))
	n.Quo(n, big.NewInt(den))
	if !n.IsInt64() {
		if value >= 0 {
			return maxInt64
		}
		return minInt64
	}
	return n.Int64()
}

func clampInt64(v int64) int64 { return v }

const (
	maxInt64 = int64(^uint64(0) >> 1)
	minInt64 = -maxInt64 - 1
)

func gcd64(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
