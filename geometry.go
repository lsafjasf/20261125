package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Orientation string

const (
	OrientationLandscape Orientation = "landscape"
	OrientationPortrait  Orientation = "portrait"
	OrientationSquare    Orientation = "square"
)

func orientationOf(width, height int) Orientation {
	switch {
	case width > height:
		return OrientationLandscape
	case width < height:
		return OrientationPortrait
	default:
		return OrientationSquare
	}
}

func orientationOfF(width, height float64) Orientation {
	const eps = 0.01
	switch {
	case width-height > eps:
		return OrientationLandscape
	case height-width > eps:
		return OrientationPortrait
	default:
		return OrientationSquare
	}
}

func rotateDims(width, height, turnsCW int) (int, int) {
	turns := normTurnsCW(turnsCW)
	if turns == 1 || turns == 3 {
		return height, width
	}
	return width, height
}

func normTurnsCW(turns int) int {
	return ((turns % 4) + 4) % 4
}

// MatrixInfo keeps the original container matrix together with the normalized
// interpretation. The raw values are important because "rotation=90" tags and
// FFprobe's rotation field use different sign conventions.
type MatrixInfo struct {
	Source             string     `json:"source"`
	Raw                [9]int64   `json:"raw"`
	Linear             [4]float64 `json:"linearABCD"`
	FFprobeRotation    *float64   `json:"ffprobeRotationDegrees,omitempty"`
	LegacyTagRotation  *int       `json:"legacyTagRotationDegrees,omitempty"`
	DegreesClockwise   int        `json:"degreesClockwise"`
	TurnsClockwise     int        `json:"turnsClockwise"`
	SwapsAxes          bool       `json:"swapsAxes"`
	QuarterTurn        bool       `json:"quarterTurn"`
	Reflection         bool       `json:"reflection"`
	LinearIdentity     bool       `json:"linearIdentity"`
	Convention         string     `json:"convention"`
	InterpretationNote string     `json:"interpretationNote,omitempty"`
}

func matrixFromRaw(raw [9]int64, source string) MatrixInfo {
	// ISO BMFF/QuickTime rotation entries are 16.16 fixed point. The bottom
	// row's scale/perspective entry commonly uses 2.30, but rotation needs
	// only the upper-left 2x2 linear part.
	a := float64(raw[0]) / 65536.0
	b := float64(raw[1]) / 65536.0
	c := float64(raw[3]) / 65536.0
	d := float64(raw[4]) / 65536.0

	m := MatrixInfo{
		Source:     source,
		Raw:        raw,
		Linear:     [4]float64{a, b, c, d},
		Convention: "row-major tkhd/display-matrix; FFprobe -90 equals one 90-degree clockwise display turn",
	}
	turns, quarter, identity, reflection := classifyLinear(a, b, c, d)
	m.TurnsClockwise = turns
	m.DegreesClockwise = turns * 90
	m.SwapsAxes = turns == 1 || turns == 3
	m.QuarterTurn = quarter
	m.LinearIdentity = identity
	m.Reflection = reflection
	return m
}

func classifyLinear(a, b, c, d float64) (turns int, quarter, identity, reflection bool) {
	const eps = 0.03
	near := func(x, target float64) bool { return math.Abs(x-target) <= eps }
	determinant := a*d - b*c
	reflection = determinant < -0.5

	switch {
	case near(a, 1) && near(b, 0) && near(c, 0) && near(d, 1):
		return 0, true, true, reflection
	case near(a, 0) && near(b, 1) && near(c, -1) && near(d, 0):
		// Verified against FFmpeg 7: [0,1;-1,0] is reported as rotation=-90
		// and its autorotated frame is 90 degrees clockwise.
		return 1, true, false, reflection
	case near(a, -1) && near(b, 0) && near(c, 0) && near(d, -1):
		return 2, true, false, reflection
	case near(a, 0) && near(b, -1) && near(c, 1) && near(d, 0):
		return 3, true, false, reflection
	default:
		return 0, false, false, reflection
	}
}

func turnsFromFFprobeRotation(degrees float64) int {
	return normTurnsCW(int(math.Round(-degrees / 90.0)))
}

func turnsFromLegacyRotateTag(degrees int) int {
	// The historical MP4 stream tag "rotate=90" is normally the positive
	// clockwise/sensor orientation value; FFprobe's side-data rotation is -90.
	return normTurnsCW(int(math.Round(float64(degrees) / 90.0)))
}

func parseRatio(value string) (num, den int, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "N/A" || value == "0:1" {
		return 0, 0, false
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || f == 0 {
			return 0, 0, false
		}
		return int(math.Round(f)), 1, true
	}
	n, err1 := strconv.Atoi(parts[0])
	d, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || n <= 0 || d <= 0 {
		return 0, 0, false
	}
	return n, d, true
}

func sarAdjustedWidth(width, sarNum, sarDen int) float64 {
	if sarNum <= 0 || sarDen <= 0 {
		return float64(width)
	}
	return float64(width) * float64(sarNum) / float64(sarDen)
}

func approxEqualIntFloat(actual int, expected float64, tolerance float64) bool {
	return math.Abs(float64(actual)-expected) <= tolerance
}

func formatDimensions(w, h int) string {
	return fmt.Sprintf("%dx%d", w, h)
}

func roundForReport(value float64, digits int) float64 {
	scale := math.Pow10(digits)
	return math.Round(value*scale) / scale
}
