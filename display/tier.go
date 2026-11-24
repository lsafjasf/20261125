package display

import (
	"fmt"
	"math"
)

// TierID identifies one row of the capability-matrix conversion strategy.
type TierID string

const (
	// TierHDRPQ: HDR path using the SMPTE ST.2084 PQ transfer function.
	TierHDRPQ TierID = "hdr-pq"
	// TierSDRDevice: SDR path calibrated to the device's own peak and black.
	TierSDRDevice TierID = "sdr-srgb-device"
	// TierSDRSafe: fixed, hard-coded baseline that is always verifiable.
	TierSDRSafe TierID = "sdr-srgb-safe"
)

// TransferFunction names the opto-electronic transfer function of a tier.
type TransferFunction string

const (
	TransferPQ   TransferFunction = "PQ-ST2084"
	TransferSRGB TransferFunction = "sRGB-IEC61966"
)

// Profile is the fully resolved, verifiable conversion configuration a tier
// produces. All quantities are fixed numbers, so every constraint can be
// re-checked by Verify at any time.
type Profile struct {
	Tier        TierID
	Transfer    TransferFunction
	PeakNits    float64
	BlackNits   float64
	KneeNits    float64
	Gamut       Primaries
	Bits        int
	CodeMax     int
	GamutMatrix Mat3
}

const (
	// SafeProfile constants define the immutable baseline fallback tier.
	SafePeakNits  = 100.0
	SafeBlackNits = 0.0
	SafeBits      = 8
)

// SafeProfile returns the hard-coded baseline. Its values are constants and
// Verify is asserted in tests, so callers always have a known-good target.
func SafeProfile() Profile {
	gamut, _ := StandardGamut("sRGB")
	identity := Mat3{1, 0, 0, 0, 1, 0, 0, 0, 1}
	return Profile{
		Tier:        TierSDRSafe,
		Transfer:    TransferSRGB,
		PeakNits:    SafePeakNits,
		BlackNits:   SafeBlackNits,
		KneeNits:    SafePeakNits,
		Gamut:       gamut,
		Bits:        SafeBits,
		CodeMax:     (1 << SafeBits) - 1,
		GamutMatrix: identity,
	}
}

// Verify re-checks every numeric and structural invariant of a resolved
// profile. A profile that fails Verify must never be used for conversion.
func (p Profile) Verify() error {
	checks := []struct {
		ok   bool
		msg  string
		args []any
	}{
		{finite(p.PeakNits) && p.PeakNits > 0, "peak %g nits must be finite and positive", []any{p.PeakNits}},
		{finite(p.BlackNits) && p.BlackNits >= 0 && p.BlackNits < p.PeakNits,
			"black %g nits must satisfy 0 <= black < peak %g", []any{p.BlackNits, p.PeakNits}},
		{p.KneeNits > 0 && p.KneeNits <= p.PeakNits, "knee %g nits must be within (0, peak %g]", []any{p.KneeNits, p.PeakNits}},
		{p.Bits >= 6 && p.Bits <= 16, "bits %d outside 6..16", []any{p.Bits}},
		{p.CodeMax == (1<<p.Bits)-1, "code max %d does not match bits %d", []any{p.CodeMax, p.Bits}},
		{math.Abs(p.GamutMatrix.determinant()) > 1e-9, "gamut matrix is singular (det=%g)", []any{p.GamutMatrix.determinant()}},
	}
	for _, c := range checks {
		if !c.ok {
			return fmt.Errorf("tier %s failed verification: "+c.msg, append([]any{p.Tier}, c.args...)...)
		}
	}
	switch p.Tier {
	case TierHDRPQ:
		if p.Transfer != TransferPQ {
			return fmt.Errorf("tier %s requires PQ transfer, got %s", p.Tier, p.Transfer)
		}
		if p.PeakNits < 200 {
			return fmt.Errorf("tier %s needs >=200 nits peak, got %g", p.Tier, p.PeakNits)
		}
		if p.Bits < 10 {
			return fmt.Errorf("tier %s needs >=10 bit depth, got %d", p.Tier, p.Bits)
		}
	case TierSDRDevice, TierSDRSafe:
		if p.Transfer != TransferSRGB {
			return fmt.Errorf("tier %s requires sRGB transfer, got %s", p.Tier, p.Transfer)
		}
	default:
		return fmt.Errorf("unknown tier %q", p.Tier)
	}
	if p.Tier == TierSDRSafe {
		safe := SafeProfile()
		if p.PeakNits != safe.PeakNits || p.BlackNits != safe.BlackNits || p.Bits != safe.Bits {
			return fmt.Errorf("safe tier constants were altered: %+v", p)
		}
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
