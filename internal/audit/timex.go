package audit

import (
	"math/big"
	"strconv"
	"strings"
)

func uintRat(v uint64) *big.Rat {
	return new(big.Rat).SetInt(new(big.Int).SetUint64(v))
}

func intRat(v int64) *big.Rat {
	return new(big.Rat).SetInt64(v)
}

func rationalSeconds(value uint64, timescale uint32) string {
	if timescale == 0 {
		return "undefined"
	}
	return new(big.Rat).Quo(uintRat(value), big.NewRat(int64(timescale), 1)).RatString()
}

func signedRationalSeconds(value int64, timescale uint32) string {
	if timescale == 0 {
		return "undefined"
	}
	return new(big.Rat).Quo(intRat(value), big.NewRat(int64(timescale), 1)).RatString()
}

func ratSeconds(value *big.Rat, timescale uint32) string {
	if timescale == 0 {
		return "undefined"
	}
	return new(big.Rat).Quo(value, big.NewRat(int64(timescale), 1)).RatString()
}

func ratFloat(r *big.Rat) float64 {
	if r == nil {
		return 0
	}
	f, _ := r.Float64()
	return f
}

func decimalString(r *big.Rat, digits int) string {
	s := r.FloatString(digits)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "-0" {
		return "0"
	}
	return s
}

func intervalFrom(start, duration *big.Rat, timescale uint32) Interval {
	end := new(big.Rat).Add(start, duration)
	return Interval{
		Timescale:        timescale,
		StartUnits:       start.RatString(),
		EndUnits:         end.RatString(),
		DurationUnits:    duration.RatString(),
		StartSeconds:     ratSeconds(start, timescale),
		EndSeconds:       ratSeconds(end, timescale),
		DurationSeconds:  ratSeconds(duration, timescale),
		StartSecondsF:    ratFloat(ratSecondsRat(start, timescale)),
		EndSecondsF:      ratFloat(ratSecondsRat(end, timescale)),
		DurationSecondsF: ratFloat(ratSecondsRat(duration, timescale)),
	}
}

func ratSecondsRat(value *big.Rat, timescale uint32) *big.Rat {
	if timescale == 0 {
		return new(big.Rat)
	}
	return new(big.Rat).Quo(value, big.NewRat(int64(timescale), 1))
}

func pointFromUnits(units *big.Rat, timescale uint32) Point {
	return Point{
		Timescale:    timescale,
		UnitsExact:   units.RatString(),
		SecondsExact: ratSeconds(units, timescale),
		Seconds:      ratFloat(ratSecondsRat(units, timescale)),
	}
}

func roundedUnits(r *big.Rat) *RoundedUnits {
	return &RoundedUnits{
		Floor:   floorRat(r).String(),
		Nearest: floorRat(new(big.Rat).Add(r, big.NewRat(1, 2))).String(),
		Ceil:    ceilRat(r).String(),
	}
}

func floorRat(r *big.Rat) *big.Int {
	num := new(big.Int).Set(r.Num())
	den := r.Denom()
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if num.Sign() < 0 && rem.Sign() != 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q
}

func ceilRat(r *big.Rat) *big.Int {
	return floorRat(new(big.Rat).Neg(r)).Neg(floorRat(new(big.Rat).Neg(r)))
}

func formatUint(v uint64) string { return strconv.FormatUint(v, 10) }
