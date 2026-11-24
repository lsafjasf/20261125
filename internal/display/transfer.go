// Package display contains the scene-to-display transfer used by the repair.
package display

import (
	"fmt"
	"math"
)

// SRGBEncode applies the IEC 61966-2-1 sRGB opto-electronic transfer.
func SRGBEncode(linear float64) float64 {
	linear = clamp01(linear)
	if linear <= 0.0031308 {
		return 12.92 * linear
	}
	return 1.055*math.Pow(linear, 1.0/2.4) - 0.055
}

// SRGBDecode is the inverse of SRGBEncode.
func SRGBDecode(code float64) float64 {
	code = clamp01(code)
	if code <= 0.04045 {
		return code / 12.92
	}
	return math.Pow((code+0.055)/1.055, 2.4)
}

// Shoulder is a monotonic highlight roll-off. Input at or below Knee is passed
// through byte-for-byte, so middle tones are not altered by highlight recovery.
// Above the knee it uses a rational shoulder with a finite output ceiling and
// enough slope to survive 16-bit display quantization.
type Shoulder struct {
	Knee              float64
	Ceiling           float64
	CalibrationInput  float64
	CalibrationOutput float64
}

// DefaultShoulder is calibrated so scene-linear 0.70 remains 0.70, scene-linear
// 128 maps to 0.88, and the asymptote is 0.90. The last value leaves about 3.4%
// of an SDR code word above the brightest explainable highlight.
func DefaultShoulder() Shoulder {
	return Shoulder{
		Knee:              0.70,
		Ceiling:           0.90,
		CalibrationInput:  128,
		CalibrationOutput: 0.88,
	}
}

// Validate checks that the shoulder cannot fold or clip its input.
func (s Shoulder) Validate() error {
	if !(s.Knee >= 0 && s.Knee < s.Ceiling && s.Ceiling <= 1) {
		return fmt.Errorf("shoulder needs 0 <= knee < ceiling <= 1, got %.4f / %.4f", s.Knee, s.Ceiling)
	}
	if !(s.CalibrationInput > s.Knee && s.CalibrationOutput > s.Knee && s.CalibrationOutput < s.Ceiling) {
		return fmt.Errorf("shoulder calibration must lie between knee and ceiling")
	}
	return nil
}

// Scale is the rational shoulder distance constant implied by the calibration
// pair.
func (s Shoulder) Scale() float64 {
	if err := s.Validate(); err != nil {
		panic(err)
	}
	ratio := (s.CalibrationOutput - s.Knee) / (s.Ceiling - s.Knee)
	return (s.CalibrationInput - s.Knee) * (1 - ratio) / ratio
}

// Apply maps scene-linear radiance to display-linear output.
func (s Shoulder) Apply(input float64) float64 {
	if input <= 0 {
		return 0
	}
	if input <= s.Knee {
		return input
	}
	scale := s.Scale()
	u := input - s.Knee
	return s.Knee + (s.Ceiling-s.Knee)*u/(u+scale)
}

func clamp01(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
