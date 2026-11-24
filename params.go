package cutluma

import (
	"errors"
	"math"
)

// ConvertParams is the per-segment color conversion recipe.
//
// The forward model (normalized linear-light source x in [0,1]) is
//
//	u = x * 2^ExposureEV
//	s = clamp((u - Black) / (White - Black), 0, 1)
//	y = s ^ TransferPower
//
// Black/White are input signal levels, ExposureEV is the pre-stretch
// exposure in stops and TransferPower is the encode gamma exponent.
type ConvertParams struct {
	Black         float64 `json:"black"`
	White         float64 `json:"white"`
	ExposureEV    float64 `json:"exposureEV"`
	TransferPower float64 `json:"transferPower"`
}

// DefaultParams is the shared recipe every segment should have used.
func DefaultParams() ConvertParams {
	return ConvertParams{Black: 0.02, White: 0.96, ExposureEV: 0, TransferPower: 1}
}

// Validate rejects degenerate recipes.
func (p ConvertParams) Validate() error {
	if !(p.White-p.Black > 1e-4) {
		return errors.New("white must exceed black by more than 1e-4")
	}
	if p.TransferPower <= 1e-3 || p.TransferPower > 10 {
		return errors.New("transfer power out of sane range")
	}
	if math.Abs(p.ExposureEV) > 6 {
		return errors.New("exposure out of sane range")
	}
	return nil
}

// Apply runs the forward conversion.
func (p ConvertParams) Apply(x float64) float64 {
	gain := math.Pow(2, p.ExposureEV)
	u := x * gain
	s := clamp01((u - p.Black) / (p.White - p.Black))
	return clamp01(math.Pow(s, p.TransferPower))
}

// Inverse maps a converted luma y back to normalized source light.
func (p ConvertParams) Inverse(y float64) float64 {
	if err := p.Validate(); err != nil {
		return math.NaN()
	}
	s := math.Pow(clamp01(y), 1/p.TransferPower)
	u := p.Black + (p.White-p.Black)*s
	x := u / math.Pow(2, p.ExposureEV)
	return clamp01(x)
}

// ReconversionMap returns a LUT mapping luma produced by src into the
// luma that tgt would have produced from the same source: tgt(src^-1(v)).
func ReconversionMap(src, tgt ConvertParams, size int) ([]float64, error) {
	if size < 2 {
		size = 257
	}
	if err := src.Validate(); err != nil {
		return nil, err
	}
	if err := tgt.Validate(); err != nil {
		return nil, err
	}
	lut := make([]float64, size)
	for i := 0; i < size; i++ {
		v := float64(i) / float64(size-1)
		lut[i] = clamp01(tgt.Apply(src.Inverse(v)))
	}
	return lut, nil
}

// ApplyLUT evaluates an evenly spaced [0,1] LUT with linear interpolation.
func ApplyLUT(lut []float64, v float64) float64 {
	n := len(lut)
	if n == 0 {
		return v
	}
	if n == 1 {
		return lut[0]
	}
	x := clamp01(v) * float64(n-1)
	i := int(x)
	if i >= n-1 {
		return lut[n-1]
	}
	f := x - float64(i)
	return lut[i]*(1-f) + lut[i+1]*f
}

// ParamDiff summarizes the deviation of one recipe from a reference.
type ParamDiff struct {
	BlackDelta      float64 `json:"blackDelta"`
	WhiteDelta      float64 `json:"whiteDelta"`
	ExposureEVDelta float64 `json:"exposureEvDelta"`
	PowerDelta      float64 `json:"powerDelta"`
	MaxAbs          float64 `json:"maxAbs"`
}

// DiffParams computes raw deltas (segment minus reference).
func DiffParams(p, ref ConvertParams) ParamDiff {
	d := ParamDiff{
		BlackDelta:      p.Black - ref.Black,
		WhiteDelta:      p.White - ref.White,
		ExposureEVDelta: p.ExposureEV - ref.ExposureEV,
		PowerDelta:      p.TransferPower - ref.TransferPower,
	}
	d.MaxAbs = math.Max(math.Abs(d.BlackDelta),
		math.Max(math.Abs(d.WhiteDelta),
			math.Max(math.Abs(d.ExposureEVDelta), math.Abs(d.PowerDelta))))
	return d
}

// Consistent reports whether two recipes agree within per-field tolerances.
// Tolerances are expressed in their natural units (levels / stops / gamma).
func (d ParamDiff) Consistent(tol ParamTolerance) bool {
	return math.Abs(d.BlackDelta) <= tol.Black &&
		math.Abs(d.WhiteDelta) <= tol.White &&
		math.Abs(d.ExposureEVDelta) <= tol.ExposureEV &&
		math.Abs(d.PowerDelta) <= tol.Power
}

// ParamTolerance holds per-field consistency tolerances.
type ParamTolerance struct {
	Black      float64 `json:"black"`
	White      float64 `json:"white"`
	ExposureEV float64 `json:"exposureEv"`
	Power      float64 `json:"power"`
}

// DefaultTolerance gives practical agreement limits: sub-code level points,
// about 1/20 stop exposure and ~2% transfer exponent.
func DefaultTolerance() ParamTolerance {
	return ParamTolerance{Black: 0.004, White: 0.004, ExposureEV: 0.05, Power: 0.03}
}
