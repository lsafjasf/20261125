package display

import "fmt"

// Capability-matrix thresholds. They are the single source of truth for which
// tier a capability row is eligible for.
const (
	HDRMinPeakNits = 200.0
	HDRMinBits     = 10
)

// Attempt records what happened when one tier of the matrix was considered.
type Attempt struct {
	Tier        TierID
	Eligible    bool
	Reason      string
	VerifyError string
}

// Decision is the auditable result of resolving one capability row.
type Decision struct {
	DisplayID        string
	ConfigValid      bool
	ValidationErrors []ValidationError
	Selected         TierID
	Profile          Profile
	Fallback         bool
	FallbackReasons  []string
	Attempts         []Attempt
}

// Resolve runs the capability matrix for one display against the given source
// gamut. Illegal or incomplete rows never reach tier construction: they fall
// straight to the safe baseline and every reason is recorded. Each candidate
// tier is built and must pass Profile.Verify before it may be selected.
func Resolve(d Display, source Primaries) Decision {
	dec := Decision{DisplayID: d.ID}
	verrs := d.Validate()
	if len(verrs) > 0 {
		dec.ConfigValid = false
		dec.ValidationErrors = verrs
		dec.Fallback = true
		for _, e := range verrs {
			dec.FallbackReasons = append(dec.FallbackReasons, e.Error())
		}
		dec.FallbackReasons = append(dec.FallbackReasons,
			fmt.Sprintf("illegal/incomplete configuration: selecting verifiable baseline tier %s", TierSDRSafe))
		safe := SafeProfile()
		if err := safe.Verify(); err != nil {
			panic("safe baseline profile failed its own invariants: " + err.Error())
		}
		dec.Attempts = append(dec.Attempts, Attempt{Tier: TierSDRSafe, Eligible: true, Reason: "fixed baseline"})
		dec.Selected, dec.Profile = TierSDRSafe, safe
		return dec
	}
	dec.ConfigValid = true

	for _, tier := range []TierID{TierHDRPQ, TierSDRDevice} {
		eligible, reason := eligible(tier, d)
		att := Attempt{Tier: tier, Eligible: eligible, Reason: reason}
		if !eligible {
			dec.Attempts = append(dec.Attempts, att)
			continue
		}
		prof, err := buildProfile(tier, d, source)
		if err != nil {
			att.VerifyError = err.Error()
			dec.Attempts = append(dec.Attempts, att)
			dec.Fallback = true
			dec.FallbackReasons = append(dec.FallbackReasons,
				fmt.Sprintf("tier %s built but failed verification: %s", tier, err.Error()))
			continue
		}
		dec.Attempts = append(dec.Attempts, att)
		dec.Selected, dec.Profile = tier, prof
		return dec
	}

	// Valid rows that match neither matrix row still get a device-independent
	// baseline rather than an error. This is normal matrix resolution, not an
	// invalid-config fallback.
	safe := SafeProfile()
	if err := safe.Verify(); err != nil {
		panic("safe baseline profile failed its own invariants: " + err.Error())
	}
	dec.Attempts = append(dec.Attempts, Attempt{Tier: TierSDRSafe, Eligible: true, Reason: "matrix default"})
	dec.Selected, dec.Profile = TierSDRSafe, safe
	return dec
}

func eligible(tier TierID, d Display) (bool, string) {
	switch tier {
	case TierHDRPQ:
		switch {
		case d.PeakNits < HDRMinPeakNits:
			return false, fmt.Sprintf("peak %g nits below HDR minimum %g", d.PeakNits, HDRMinPeakNits)
		case d.Bits < HDRMinBits:
			return false, fmt.Sprintf("bit depth %d below HDR minimum %d", d.Bits, HDRMinBits)
		default:
			return true, "peak and bit depth satisfy HDR matrix row"
		}
	case TierSDRDevice:
		if d.PeakNits <= 0 || d.Bits <= 0 {
			return false, "missing luminance or bit-depth capability"
		}
		return true, "valid SDR capability row"
	default:
		return false, "unknown tier"
	}
}

func buildProfile(tier TierID, d Display, source Primaries) (Profile, error) {
	matrix, ok := GamutMapMatrix(source, d.Gamut)
	if !ok {
		return Profile{}, fmt.Errorf("cannot derive gamut matrix from supplied primaries")
	}
	p := Profile{
		PeakNits:    d.PeakNits,
		BlackNits:   d.BlackNits,
		Gamut:       d.Gamut,
		Bits:        d.Bits,
		CodeMax:     (1 << d.Bits) - 1,
		GamutMatrix: matrix,
	}
	switch tier {
	case TierHDRPQ:
		p.Tier, p.Transfer = TierHDRPQ, TransferPQ
		p.KneeNits = d.PeakNits
	case TierSDRDevice:
		p.Tier, p.Transfer = TierSDRDevice, TransferSRGB
		p.KneeNits = min(SafePeakNits, d.PeakNits)
	default:
		return Profile{}, fmt.Errorf("cannot build tier %q", tier)
	}
	if err := p.Verify(); err != nil {
		return Profile{}, err
	}
	return p, nil
}
