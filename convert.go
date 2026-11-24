package main

import (
	"fmt"
	"math/big"
)

type Bound string

const (
	startBound Bound = "in"
	endBound   Bound = "out"
)

type QuantError struct {
	CueIndex       int
	Bound          Bound
	Exact          *big.Rat
	Quantized      *big.Rat
	MillisecondErr *big.Rat
}

type ConversionStep struct {
	Number       int
	SourceRate   *big.Rat
	TargetRate   *big.Rat
	Factor       *big.Rat
	Rounding     string
	Quantization []QuantError
}

type Issue struct {
	Stage    int
	CueIndex int
	Kind     string
	Severity string
	Message  string
}

type RoundTripReport struct {
	Available      bool
	ConstantOffset bool
	MinDelta       *big.Rat
	MaxDelta       *big.Rat
	SharedOffset   *big.Rat
}

type ConversionReport struct {
	SourceRate *big.Rat
	Steps      []ConversionStep
	Issues     []Issue
	RoundTrip  RoundTripReport
}

func parseRate(value string) (*big.Rat, error) {
	rate, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("invalid frame rate %q; use a decimal or fraction such as 23.976 or 24000/1001", value)
	}
	if rate.Sign() <= 0 {
		return nil, fmt.Errorf("frame rate %s must be positive", rate.RatString())
	}
	return rate, nil
}

func convertCues(cues []*Cue, sourceRate *big.Rat, targetRates []*big.Rat, rounding string) ([]*Cue, ConversionReport, error) {
	if sourceRate == nil || sourceRate.Sign() <= 0 {
		return nil, ConversionReport{}, fmt.Errorf("source frame rate must be positive")
	}
	if len(targetRates) == 0 {
		return nil, ConversionReport{}, fmt.Errorf("at least one target frame rate is required")
	}
	if rounding != "nearest" && rounding != "floor" {
		return nil, ConversionReport{}, fmt.Errorf("rounding must be nearest or floor")
	}

	report := ConversionReport{SourceRate: new(big.Rat).Set(sourceRate)}
	working := cloneCues(cues)
	report.Issues = append(report.Issues, validateCues(working, 0)...)

	currentRate := new(big.Rat).Set(sourceRate)
	for stepNumber, targetRate := range targetRates {
		factor := new(big.Rat).Quo(targetRate, currentRate)
		step := ConversionStep{
			Number:     stepNumber + 1,
			SourceRate: new(big.Rat).Set(currentRate),
			TargetRate: new(big.Rat).Set(targetRate),
			Factor:     factor,
			Rounding:   rounding,
		}

		for _, cue := range working {
			var startErr, endErr QuantError
			cue.Start, startErr = convertTimestamp(cue.Index, startBound, cue.Start, factor, rounding)
			cue.End, endErr = convertTimestamp(cue.Index, endBound, cue.End, factor, rounding)
			step.Quantization = append(step.Quantization, startErr, endErr)
		}

		report.Steps = append(report.Steps, step)
		report.Issues = append(report.Issues, validateCues(working, step.Number)...)
		currentRate.Set(targetRate)
	}

	if currentRate.Cmp(sourceRate) == 0 {
		report.RoundTrip = analyzeRoundTrip(cues, working)
	}
	return working, report, nil
}

func convertTimestamp(cueIndex int, bound Bound, value, factor *big.Rat, rounding string) (*big.Rat, QuantError) {
	exact := new(big.Rat).Mul(value, factor)
	quantized, err := quantizeMilliseconds(exact, rounding)
	return quantized, QuantError{
		CueIndex:       cueIndex,
		Bound:          bound,
		Exact:          exact,
		Quantized:      new(big.Rat).Set(quantized),
		MillisecondErr: new(big.Rat).Set(err),
	}
}

func quantizeMilliseconds(exact *big.Rat, rounding string) (*big.Rat, *big.Rat) {
	num := new(big.Int).Set(exact.Num())
	denom := new(big.Int).Set(exact.Denom())
	var roundedNumerator *big.Int

	if rounding == "floor" {
		roundedNumerator = new(big.Int).Quo(num, denom)
	} else {
		twiceNumerator := new(big.Int).Mul(num, big.NewInt(2))
		twiceNumerator.Add(twiceNumerator, denom)
		twiceDenominator := new(big.Int).Mul(denom, big.NewInt(2))
		roundedNumerator = new(big.Int).Quo(twiceNumerator, twiceDenominator)
	}

	quantized := new(big.Rat).SetInt(roundedNumerator)
	err := new(big.Rat).Sub(exact, quantized)
	return quantized, err
}

func cloneCues(cues []*Cue) []*Cue {
	cloned := make([]*Cue, len(cues))
	for i, cue := range cues {
		cloned[i] = &Cue{
			Index: cue.Index,
			Start: new(big.Rat).Set(cue.Start),
			End:   new(big.Rat).Set(cue.End),
			Text:  cue.Text,
		}
	}
	return cloned
}

func analyzeRoundTrip(original, converted []*Cue) RoundTripReport {
	report := RoundTripReport{Available: true}
	var firstDelta *big.Rat

	for i := range original {
		for _, delta := range []*big.Rat{
			new(big.Rat).Sub(converted[i].Start, original[i].Start),
			new(big.Rat).Sub(converted[i].End, original[i].End),
		} {
			if firstDelta == nil {
				firstDelta = new(big.Rat).Set(delta)
				report.MinDelta = new(big.Rat).Set(delta)
				report.MaxDelta = new(big.Rat).Set(delta)
				continue
			}
			if delta.Cmp(report.MinDelta) < 0 {
				report.MinDelta.Set(delta)
			}
			if delta.Cmp(report.MaxDelta) > 0 {
				report.MaxDelta.Set(delta)
			}
			if delta.Cmp(firstDelta) != 0 {
				report.ConstantOffset = false
			}
		}
	}

	report.ConstantOffset = firstDelta != nil && report.MinDelta.Cmp(report.MaxDelta) == 0
	if report.ConstantOffset {
		report.SharedOffset = new(big.Rat).Set(firstDelta)
	}
	return report
}
