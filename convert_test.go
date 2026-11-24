package main

import (
	"math/big"
	"testing"
)

func rat(value string) *big.Rat {
	parsed, ok := new(big.Rat).SetString(value)
	if !ok {
		panic("invalid test rational: " + value)
	}
	return parsed
}

func TestConvertUsesRationalQuantization(t *testing.T) {
	source := rat("24000/1001")
	target := rat("25")
	cues := []*Cue{{
		Index: 1,
		Start: rat("1000"),
		End:   rat("2000"),
		Text:  "hello",
	}}

	converted, report, err := convertCues(cues, source, []*big.Rat{target}, "nearest")
	if err != nil {
		t.Fatal(err)
	}
	if got := converted[0].Start.RatString(); got != "1043" {
		t.Fatalf("start = %s, want 1043", got)
	}
	if got := converted[0].End.RatString(); got != "2085" {
		t.Fatalf("end = %s, want 2085", got)
	}
	wantError := rat("-7/24")
	if got := report.Steps[0].Quantization[0].MillisecondErr; got.Cmp(wantError) != 0 {
		t.Fatalf("start error = %s, want %s", got.RatString(), wantError.RatString())
	}
}

func TestRoundTripDeltasAreNotGloballyCorrectable(t *testing.T) {
	source := rat("24000/1001")
	target := rat("25")
	cues := []*Cue{
		{Index: 1, Start: rat("1"), End: rat("960"), Text: "first"},
		{Index: 2, Start: rat("960"), End: rat("2000"), Text: "second"},
	}

	_, report, err := convertCues(cues, source, []*big.Rat{target, source}, "floor")
	if err != nil {
		t.Fatal(err)
	}
	if !report.RoundTrip.Available {
		t.Fatal("round-trip report was not generated")
	}
	if report.RoundTrip.ConstantOffset {
		t.Fatalf("unexpected constant offset %s", report.RoundTrip.SharedOffset.RatString())
	}
	if report.RoundTrip.MinDelta.Cmp(report.RoundTrip.MaxDelta) >= 0 {
		t.Fatalf("min delta %s must be below max delta %s",
			report.RoundTrip.MinDelta.RatString(), report.RoundTrip.MaxDelta.RatString())
	}
}

func TestValidationFindsZeroDurationDuplicatesAndCrossMidnight(t *testing.T) {
	cues := []*Cue{
		{Index: 1, Start: rat("86400000"), End: rat("86400000"), Text: "zero and midnight"},
		{Index: 2, Start: rat("86400000"), End: rat("86400001"), Text: "duplicate and midnight"},
	}

	issues := validateCues(cues, 0)
	kinds := map[string]int{}
	for _, issue := range issues {
		kinds[issue.Kind]++
	}
	for _, kind := range []string{"zero_duration", "duplicate_start", "start_crosses_midnight", "end_crosses_midnight"} {
		if kinds[kind] == 0 {
			t.Fatalf("missing issue kind %s; got %#v", kind, kinds)
		}
	}
	if !hasError(issues) {
		t.Fatal("zero-duration issue must be an error")
	}
}

func TestParseAndFormatSRT(t *testing.T) {
	input := []byte("1\n00:00:01,000 --> 00:00:02,000\nhello\n")
	cues, err := parseSRT(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := cues[0].Start.RatString(); got != "1000" {
		t.Fatalf("start = %s, want 1000", got)
	}
	rendered, err := formatSRT(cues)
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n00:00:01,000 --> 00:00:02,000\nhello\n"
	if rendered != want {
		t.Fatalf("rendered SRT = %q, want %q", rendered, want)
	}
}

func TestTimestampBeyondOneDayUsesArbitraryPrecision(t *testing.T) {
	value := "999999:00:00,001"
	parsed, err := parseTimestamp(value)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := formatRatMilliseconds(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if rendered != value {
		t.Fatalf("rendered timestamp = %q, want %q", rendered, value)
	}
}
