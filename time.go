package otioflat

import (
	"fmt"
	"math/big"
)

// RationalTime is the integer subset of opentime.RationalTime.
// It represents Value/Rate seconds.
type RationalTime struct {
	Value int64 `json:"value"`
	Rate  int64 `json:"rate"`
}

// TimeRange is the integer subset of opentime.TimeRange.
type TimeRange struct {
	StartTime RationalTime `json:"start_time"`
	Duration  RationalTime `json:"duration"`
}

func timeRat(t RationalTime) (*big.Rat, error) {
	if t.Rate <= 0 {
		return nil, fmt.Errorf("otioflat: rational time rate must be positive, got %d", t.Rate)
	}
	return big.NewRat(t.Value, t.Rate), nil
}

func rationalFromRat(r *big.Rat) (RationalTime, error) {
	num := r.Num()
	den := r.Denom()
	if !num.IsInt64() || !den.IsInt64() {
		return RationalTime{}, fmt.Errorf("otioflat: time value exceeds int64: %s", r)
	}
	return RationalTime{Value: num.Int64(), Rate: den.Int64()}, nil
}

func addTime(a, b RationalTime) (RationalTime, error) {
	ar, err := timeRat(a)
	if err != nil {
		return RationalTime{}, err
	}
	br, err := timeRat(b)
	if err != nil {
		return RationalTime{}, err
	}
	return rationalFromRat(new(big.Rat).Add(ar, br))
}

func maxTime(a, b RationalTime) (RationalTime, error) {
	ar, err := timeRat(a)
	if err != nil {
		return RationalTime{}, err
	}
	br, err := timeRat(b)
	if err != nil {
		return RationalTime{}, err
	}
	if ar.Cmp(br) >= 0 {
		return a, nil
	}
	return b, nil
}

func validateTime(t RationalTime) error {
	if t.Rate <= 0 {
		return fmt.Errorf("otioflat: rational time rate must be positive, got %d", t.Rate)
	}
	return nil
}

func validateTimeRange(r *TimeRange) error {
	if r == nil {
		return nil
	}
	if err := validateTime(r.StartTime); err != nil {
		return err
	}
	if err := validateTime(r.Duration); err != nil {
		return err
	}
	if r.Duration.Value < 0 {
		return fmt.Errorf("otioflat: time range duration must be non-negative, got %d/%d", r.Duration.Value, r.Duration.Rate)
	}
	return nil
}
